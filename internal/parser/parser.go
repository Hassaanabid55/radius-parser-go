package parser

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"radius-parser/internal/cgnat"
	"radius-parser/internal/rabbitmq"
	"radius-parser/internal/session"
	"radius-parser/internal/stats"
	"radius-parser/internal/whitelist"
)

/* =========================================================
 * GLOBAL CONFIG (set from main)
 * ========================================================= */

var (
	OptExtractAll    atomic.Bool
	OptUpdateTimeout atomic.Uint32
	OptVerbosity     atomic.Int32
)

/* =========================================================
 * INIT FUNCTION (called from main)
 * ========================================================= */

func InitParser(extractAll bool, updateTimeout int, verbosity int) {
	OptExtractAll.Store(extractAll)
	OptUpdateTimeout.Store(uint32(updateTimeout))
	OptVerbosity.Store(int32(verbosity))
}

/* =========================================================
 * SESSION STATUS
 * ========================================================= */

const (
	SessionStart  = 1
	SessionStop   = 2
	SessionUpdate = 3
)

const (
	AcctStatusType     = 40
	AcctSessionID      = 44
	AcctMultiSessionID = 50
	EventTimestamp     = 55
	CallingStationID   = 31
	FramedIPAddress    = 8
	FramedIPv6Prefix   = 97
)

const (
	RadiusHeaderLen = 20
)

/* =========================================================
 * RADIUS PACKET
 * ========================================================= */

type RadiusPacket struct {
	Code       uint8
	Identifier uint8
	Length     uint16

	Attributes []byte
}

/* =========================================================
 * PARSE PACKET (FAST VALIDATION ONLY)
 * ========================================================= */

func ParseRadiusPacket(data []byte) (*RadiusPacket, error) {

	if len(data) < 20 {
		return nil, errors.New("radius too small")
	}

	code := data[0]

	// VALID RADIUS CODES ONLY
	if code < 1 {
		return nil, errors.New("invalid radius code")
	}

	length := binary.BigEndian.Uint16(data[2:4])

	if length < 20 || int(length) > len(data) {
		return nil, fmt.Errorf(
			"invalid radius length: hdr=%d pkt=%d",
			length, len(data),
		)
	}

	return &RadiusPacket{
		Code:       code,
		Identifier: data[1],
		Length:     length,
		Attributes: data[20:length],
	}, nil
}

func ExtractRadiusFromTask(data []byte) ([]byte, error) {

	if len(data) < 42 {
		return nil, errors.New("packet too small")
	}

	// Ethernet
	ethLen := 14
	if len(data) < ethLen+20 {
		return nil, errors.New("no ip layer")
	}

	ip := data[ethLen:]

	// IP header length
	ipHeaderLen := int((ip[0] & 0x0F) * 4)
	if len(ip) < ipHeaderLen+8 {
		return nil, errors.New("no udp layer")
	}

	udp := ip[ipHeaderLen:]

	// MUST be port 1813 (accounting)
	srcPort := binary.BigEndian.Uint16(udp[0:2])
	dstPort := binary.BigEndian.Uint16(udp[2:4])

	if srcPort != 1813 && dstPort != 1813 {
		return nil, errors.New("not radius accounting packet")
	}

	radius := udp[8:]

	return radius, nil
}

/* =========================================================
 * SESSION BUILD
 * ========================================================= */
func BuildSession(pkt *RadiusPacket) (*session.UserSession, error) {

	s := &session.UserSession{}
	var node *session.SessionNode

	offset := 0

	for offset+2 <= len(pkt.Attributes) {

		attrType := pkt.Attributes[offset]
		attrLen := int(pkt.Attributes[offset+1])

		if attrLen < 2 || offset+attrLen > len(pkt.Attributes) {
			return nil, errors.New("invalid avp")
		}

		value := pkt.Attributes[offset+2 : offset+attrLen]

		switch attrType {

		case AcctStatusType:
			if len(value) == 4 {
				v := binary.BigEndian.Uint32(value)
				s.AccountStatusType = uint8(v)

				switch s.AccountStatusType {

				case SessionStart:
					stats.IncStarts()

				case SessionUpdate:
					stats.IncTotalUpdates()

				case SessionStop:
					stats.IncTotalDeletes()
				}
			}

		case AcctSessionID:
			s.AccountSessionID = string(value)
			node = session.Find(s.AccountSessionID)

		case CallingStationID:
			s.CallingStationID = string(value)

			if wl, ok := whitelist.Lookup(s.CallingStationID); ok {
				s.IsWhitelist = wl.Status
			}

		case EventTimestamp:
			if len(value) == 4 {
				s.EventTimestamp = binary.BigEndian.Uint32(value)
			}

		case FramedIPAddress:
			if len(value) == 4 {
				s.FramedIPv4 = parseIPv4(value)
				if nat, ok := cgnat.Lookup(s.FramedIPv4); ok {
					s.PublicIPv4 = nat.NatIP
					s.PortStart = nat.StartPort
					s.PortEnd = nat.EndPort
				}
			}

		case FramedIPv6Prefix:
			s.FramedIPv6 = parseIPv6Prefix(value)

		case AcctMultiSessionID:
			s.MultiSessionID = string(value)
		}

		offset += attrLen
	}

	// ===============================
	// SESSION LOGIC (POST PARSE FIX)
	// ===============================

	if node != nil && s.AccountStatusType != 0 {

		switch s.AccountStatusType {

		case SessionUpdate:
			session.Lock()
			session.End(&node.Entry)
			node.Entry.DestroyTime = uint32(time.Now().Unix()) + OptUpdateTimeout.Load()
			*s = node.Entry
			session.Unlock()

			stats.IncUpdates()
			return s, nil

		case SessionStop:
			session.Lock()
			session.End(&node.Entry)
			*s = node.Entry
			session.DeleteNode(node)
			session.Unlock()

			rabbitmq.PublishSessionStop(s)

			stats.DecSessionCount()
			stats.IncDeletes()
			return s, nil
		}
	}

	// ===============================
	// INSERT LOGIC
	// ===============================

	if s.AccountStatusType == SessionStart || s.AccountStatusType == SessionUpdate {

		session.SetStartTime(s)
		s.DestroyTime = uint32(time.Now().Unix()) + OptUpdateTimeout.Load()

		rc := session.Insert(s)

		rabbitmq.PublishSessionStart(s)

		if rc == 0 {
			stats.IncSessionCount()
			stats.IncInserts()
		}
	}

	return s, nil
}

func parseIPv4(b []byte) string {
    if len(b) != 4 {
        return ""
    }

    return net.IP(b).String()
}

func parseIPv6Prefix(b []byte) string {

    if len(b) < 2 {
        return ""
    }

    prefixLen := b[1]

    ipBytes := make([]byte, 16)

    copy(ipBytes, b[2:])

    return fmt.Sprintf(
        "%s/%d",
        net.IP(ipBytes).String(),
        prefixLen,
    )
}