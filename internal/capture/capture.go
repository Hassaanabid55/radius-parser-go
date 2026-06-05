package capture

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type Task struct {
	Timestamp time.Time
	Data      []byte
	SrcIP     string
	DstIP     string
	SrcPort   uint16
	DstPort   uint16
	IsRadius  bool
}

// -----------------------------
// GLOBAL STATE (cleaned)
// -----------------------------
var TaskQueue chan Task
var Inflight atomic.Uint64

var (
	handle    *pcap.Handle
	gRunning  atomic.Bool
	snapLen   int32
	verbosity int
)

func InitCapture(queueSize int, snap int, v int) {
	TaskQueue = make(chan Task, queueSize)
	snapLen = int32(snap)
	verbosity = v
	gRunning.Store(true)
}

// -----------------------------
// PACKET PROCESSOR (equivalent to packet_handler)
// -----------------------------
func processPacket(packet gopacket.Packet) {

	if !gRunning.Load() {
		return
	}

	ethLayer := packet.Layer(layers.LayerTypeEthernet)
	if ethLayer == nil {
		return
	}

	ipLayer := packet.Layer(layers.LayerTypeIPv4)
	if ipLayer == nil {
		return
	}

	udpLayer := packet.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		return
	}

	ip := ipLayer.(*layers.IPv4)
	udp := udpLayer.(*layers.UDP)

	if !isRadiusPort(uint16(udp.SrcPort), uint16(udp.DstPort)) {
		return
	}

	task := Task{
		Timestamp: packet.Metadata().Timestamp,
		Data:      packet.Data(),

		SrcIP:   ip.SrcIP.String(),
		DstIP:   ip.DstIP.String(),
		SrcPort: uint16(udp.SrcPort),
		DstPort: uint16(udp.DstPort),

		IsRadius: true,
	}

	select {
	case TaskQueue <- task:
		Inflight.Add(1)
	default:
		log.Println("Task queue full, dropping packet")
	}
}

// -----------------------------
// INTERFACE CAPTURE
// -----------------------------
func StartInterfaceCapture(iface string) {

	var err error

	handle, err = pcap.OpenLive(iface, snapLen, true, pcap.BlockForever)
	if err != nil {
		log.Fatalf("pcap open live failed: %v", err)
	}
	defer handle.Close()

	if verbosity > 0 {
		log.Println("Listening on interface:", iface)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packets := packetSource.Packets()

	for pkt := range packets {
		if !gRunning.Load() {
			break
		}
		processPacket(pkt)
	}
}

// -----------------------------
// PCAP FILE CAPTURE
// -----------------------------
func StartFileCapture(file string) {

	handle, err := pcap.OpenOffline(file)
	if err != nil {
		log.Fatalf("pcap open offline failed: %v", err)
	}
	defer handle.Close()

	if verbosity > 0 {
		log.Println("Processing pcap file:", file)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	for pkt := range packetSource.Packets() {
		if !gRunning.Load() {
			break
		}
		processPacket(pkt)
	}

	if verbosity > 0 {
		log.Println("PCAP file processing complete")
	}
}

// -----------------------------
// STOP CAPTURE
// -----------------------------
func Stop() {
	gRunning.Store(false)

	if handle != nil {
		handle.Close()
	}
}

// -----------------------------
// RADIUS FILTER (equivalent logic)
// -----------------------------
func isRadiusPort(src, dst uint16) bool {
	const (
		RadiusAuth = 1812
		RadiusAcct = 1813
	)

	return src == RadiusAuth || dst == RadiusAuth ||
		src == RadiusAcct || dst == RadiusAcct
}
