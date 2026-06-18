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
}

// GLOBAL STATE
var TaskQueue chan Task
var Inflight atomic.Uint64

var (
	handle    *pcap.Handle
	gRunning  atomic.Bool
	snapLen   int32
	verbosity int

	stopCh = make(chan struct{})
)

const (
	RadiusAcct = 1813
)

func InitCapture(queueSize int, snap int, v int) {
	TaskQueue = make(chan Task, queueSize)
	snapLen = int32(snap)
	verbosity = v
	gRunning.Store(true)
}

// PACKET PROCESSOR
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

	udp := udpLayer.(*layers.UDP)

	if !isRadiusPort(uint16(udp.SrcPort), uint16(udp.DstPort)) {
		return
	}

	task := Task{
		Timestamp: packet.Metadata().Timestamp,
		Data:      packet.Data(),
	}

	select {
	case TaskQueue <- task:
		Inflight.Add(1)
	default:
		log.Println("Task queue full, dropping packet")
	}
}

// INTERFACE CAPTURE
func StartInterfaceCapture(iface string) {

	var err error

	handle, err = pcap.OpenLive(iface, snapLen, true, pcap.BlockForever)
	if err != nil {
		log.Fatalf("pcap open live failed: %v", err)
	}

	if verbosity > 0 {
		log.Println("Listening on interface:", iface)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	for gRunning.Load() {
		packet, err := packetSource.NextPacket()
		if err != nil {
			if !gRunning.Load() {
				break
			}
			continue
		}
		processPacket(packet)
	}
	if verbosity > 1 {
		log.Printf("Closing Interface Capture.")
	}
}

// PCAP FILE CAPTURE
func StartFileCapture(file string) {

	handle, err := pcap.OpenOffline(file)
	if err != nil {
		log.Fatalf("pcap open offline failed: %v", err)
	}

	if verbosity > 0 {
		log.Println("Processing pcap file:", file)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	for gRunning.Load() {
		packet, err := packetSource.NextPacket()
		if err != nil {
			if !gRunning.Load() {
				break
			}
			continue
		}
		processPacket(packet)
	}
	if verbosity > 1 {
		log.Printf("Closing File Capture.")
	}
}

// STOP CAPTURE
func Stop() {
	gRunning.Store(false)
}

// RADIUS FILTER
func isRadiusPort(src, dst uint16) bool {
	return src == RadiusAcct || dst == RadiusAcct
}
