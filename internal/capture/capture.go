package main

import (
	"encoding/binary"
	"net"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/google/gopacket/pcap"
)

// =========================
// RADIUS PORT CONSTANTS
// =========================

const (
	RadiusAcctPort1 uint16 = 1812 // auth  (RADIUS_ACCT_PORT_1)
	RadiusAcctPort2 uint16 = 1813 // acct  (RADIUS_ACCT_PORT_2)
)

// =========================
// PACKET TYPE
// =========================

// RadiusPacketType mirrors the C PacketType enum.
type RadiusPacketType uint8

const (
	PKTUnknown    RadiusPacketType = 0
	PKTRadiusAuth RadiusPacketType = 1 // PKT_RADIUS_AUTH
	PKTRadiusAcct RadiusPacketType = 2 // PKT_RADIUS_ACCT
)

// =========================
// LAYER SIZE CONSTANTS
// =========================

const (
	etherHdrLen   uint32 = 14
	ipHdrMinLen   uint32 = 20
	udpHdrLen     uint32 = 8
	radiusHdrLen  uint16 = 20
	etherTypeIPv4 uint16 = 0x0800
	ipProtoUDP    uint8  = 17
)

// =========================
// GLOBALS
// =========================

var (
	gPcapHandle *pcap.Handle
	gInflight   int64 // atomic — mirrors g_inflight_tasks
)

// =========================
// TASK
// =========================

// Task mirrors the C Task struct. Sub-slices are recomputed after the
// packet buffer is copied so they point into owned memory.
type Task struct {
	Data         []byte
	Timestamp    time.Time
	PacketLength uint32
	EtherType    uint16
	IPProtocol   uint8
	IPVersion    uint8
	IPHeaderLen  uint8
	PacketType   RadiusPacketType

	SrcIP   uint32
	DstIP   uint32
	SrcPort uint16
	DstPort uint16

	// Offsets into Data
	EthernetOffset uint32
	IPOffset       uint32
	UDPOffset      uint32
	RadiusOffset   uint32
	RadiusLength   uint16

	// Convenience sub-slices (valid after rebase)
	PEthernet []byte
	PIP       []byte
	PUDP      []byte
	PRadius   []byte
}

// =========================
// INLINE HELPERS
// =========================

func isIPv4Packet(ethertype uint16) bool { return ethertype == etherTypeIPv4 }
func isUDPPacket(proto uint8) bool       { return proto == ipProtoUDP }

// isRadiusPort mirrors is_radius_port.
// Sets *pt and returns true when src or dst is a known RADIUS port.
func isRadiusPort(src, dst uint16, pt *RadiusPacketType) bool {
	// Auth port (less common path)
	if src == RadiusAcctPort1 || dst == RadiusAcctPort1 {
		*pt = PKTRadiusAuth
		return true
	}
	// Accounting port (hot path)
	if src == RadiusAcctPort2 || dst == RadiusAcctPort2 {
		*pt = PKTRadiusAcct
		return true
	}
	return false
}

// =========================
// BUILD RADIUS TASK
// =========================

func buildRadiusTask(caplen uint32, ts time.Time, pkt []byte) (Task, bool) {
	var task Task
	task.PacketLength = caplen
	task.Timestamp = ts

	// Ethernet
	if caplen < etherHdrLen {
		return task, false
	}
	task.PEthernet = pkt
	ethertype := binary.BigEndian.Uint16(pkt[12:14])
	task.EtherType = ethertype
	if !isIPv4Packet(ethertype) {
		return task, false
	}

	// IPv4
	task.EthernetOffset = 0
	task.IPOffset = etherHdrLen
	if caplen < task.IPOffset+ipHdrMinLen {
		return task, false
	}
	ipSlice := pkt[task.IPOffset:]
	ipHdrLen := uint32((ipSlice[0] & 0x0F) << 2)
	if ipHdrLen < ipHdrMinLen {
		return task, false
	}
	task.IPHeaderLen = uint8(ipHdrLen)
	task.IPVersion = (ipSlice[0] >> 4) & 0x0F
	task.IPProtocol = ipSlice[9]
	if !isUDPPacket(task.IPProtocol) {
		return task, false
	}
	task.SrcIP = binary.BigEndian.Uint32(ipSlice[12:16])
	task.DstIP = binary.BigEndian.Uint32(ipSlice[16:20])
	task.PIP = ipSlice

	// UDP
	task.UDPOffset = task.IPOffset + ipHdrLen
	if caplen < task.UDPOffset+udpHdrLen {
		return task, false
	}
	udpSlice := pkt[task.UDPOffset:]
	task.PUDP = udpSlice
	task.SrcPort = binary.BigEndian.Uint16(udpSlice[0:2])
	task.DstPort = binary.BigEndian.Uint16(udpSlice[2:4])

	if !isRadiusPort(task.SrcPort, task.DstPort, &task.PacketType) {
		return task, false
	}

	// RADIUS
	task.RadiusOffset = task.UDPOffset + udpHdrLen
	if caplen < task.RadiusOffset+uint32(radiusHdrLen) {
		return task, false
	}
	radiusSlice := pkt[task.RadiusOffset:]
	task.PRadius = radiusSlice

	radiusLen := binary.BigEndian.Uint16(radiusSlice[2:4])
	if radiusLen < uint16(radiusHdrLen) {
		return task, false
	}
	if task.RadiusOffset+uint32(radiusLen) > caplen {
		return task, false
	}
	task.RadiusLength = radiusLen
	return task, true
}

// =========================
// PACKET HANDLER
// =========================

func packetHandler(caplen uint32, ts time.Time, pkt []byte) {
	if atomic.LoadInt32(&gRunning) == 0 {
		return
	}

	task, ok := buildRadiusTask(caplen, ts, pkt)
	if !ok {
		return
	}

	// Own a copy of the packet so the caller can reuse its buffer.
	buf := make([]byte, caplen)
	copy(buf, pkt)
	task.Data = buf

	// Rebase sub-slices into owned copy.
	task.PEthernet = buf[task.EthernetOffset:]
	task.PIP = buf[task.IPOffset:]
	task.PUDP = buf[task.UDPOffset:]
	task.PRadius = buf[task.RadiusOffset:]

	submitTask(&task)
}

// =========================
// INTERFACE CAPTURE
// =========================

// PcapHandle is the control interface returned to main.go.
type PcapHandle interface {
	BreakLoop()
}

type livePcapHandle struct{ h *pcap.Handle }

func (l *livePcapHandle) BreakLoop() { l.h.Close() }

// StartInterfaceCapture opens a live capture loop in a goroutine and
// returns a handle so main can stop it on shutdown.
func StartInterfaceCapture(queue *GlobalQueue, cfg *Config) PcapHandle {
	lph := &livePcapHandle{}
	go func() {
		for atomic.LoadInt32(&gRunning) == 1 {
			handle, err := pcap.OpenLive(cfg.InterfaceName, int32(cfg.Caplen), true, pcap.BlockForever)
			if err != nil {
				sysErr("pcap_create failed: " + err.Error())
				time.Sleep(time.Second)
				continue
			}
			// TODO: set kernel ring-buffer size via inactive handle API
			// (pcap.NewInactiveHandle) when buffer tuning is needed.

			gPcapHandle = handle
			lph.h = handle

			if cfg.Verbosity > 0 {
				sysInfo("Listening on interface: " + cfg.InterfaceName)
			}

			for atomic.LoadInt32(&gRunning) == 1 {
				data, ci, err := handle.ReadPacketData()
				if err != nil {
					if atomic.LoadInt32(&gRunning) == 0 {
						break
					}
					sysErr("ReadPacketData error: " + err.Error())
					break
				}
				packetHandler(uint32(ci.CaptureLength), ci.Timestamp, data)
			}

			if cfg.Verbosity > 0 {
				sysInfo("Capture stopped")
			}
			handle.Close()
			gPcapHandle = nil

			if atomic.LoadInt32(&gRunning) == 1 {
				time.Sleep(time.Second)
			}
		}
	}()
	return lph
}

// StopInterfaceCapture closes the live handle (mirrors stop_interface_capture).
func StopInterfaceCapture() {
	if h := gPcapHandle; h != nil {
		gPcapHandle = nil
		h.Close()
	}
}

// =========================
// PRINT SESSION MAP
// =========================

func printSessionMap() {
	sysInfo("========== SESSION MAP DUMP ==========")
	count := 0
	gSessionMap.Range(func(_, v any) bool {
		s := v.(*SessionNode)
		sysInfo("SessionID: " + s.AcAccountSessionId)
		count++
		return true
	})
	sysInfo("Total sessions: " + itoa(count))
	sysInfo("======================================")
}

// =========================
// FILE CAPTURE
// =========================

func StartFileCapture(filePath string, queue *GlobalQueue, cfg *Config) {
	if cfg.Verbosity > 0 {
		sysInfo("Opening pcap file: " + filePath)
	}

	handle, err := pcap.OpenOffline(filePath)
	if err != nil {
		sysErr("pcap_open_offline failed: " + err.Error())
		return
	}
	gPcapHandle = handle
	defer func() {
		handle.Close()
		gPcapHandle = nil
	}()

	if cfg.Verbosity > 0 {
		sysInfo("Processing pcap file: " + filePath)
	}

	var pktNum uint64
	for atomic.LoadInt32(&gRunning) == 1 {
		data, ci, err := handle.ReadPacketData()
		if err != nil {
			if err.Error() == "EOF" {
				sysInfo("End of pcap file reached")
				break
			}
			pktNum++
			sysErr("pcap error at packet #" + uitoa(pktNum) + ": " + err.Error())
			continue
		}
		pktNum++
		packetHandler(uint32(ci.CaptureLength), ci.Timestamp, data)
	}

	if cfg.Verbosity > 0 {
		sysInfo("PCAP finished. Waiting for task queue to drain...")
	}

	for atomic.LoadInt64(&gInflight) > 0 {
		time.Sleep(time.Millisecond)
	}

	if cfg.Verbosity > 2 {
		printSessionMap()
	}

	if cfg.Verbosity > 0 {
		sysInfo("Task queue drained. File capture completed")
	}
}

// =========================
// HELPERS
// =========================

// itoa / uitoa avoid pulling in fmt on hot paths.
func itoa(n int) string        { return strconv.Itoa(n) }
func uitoa(n uint64) string    { return strconv.FormatUint(n, 10) }

// sysInfo / sysErr are thin wrappers so call sites stay readable.
func sysInfo(msg string) { log.Info(msg) }
func sysErr(msg string)  { log.Err(msg) }

// Suppress unused-import errors for packages used only via gopacket internals.
var _ = net.IPv4len
var _ = unsafe.Sizeof(0)