package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// =========================
// STATS GLOBALS
// =========================

var (
	gSessionCount         uint64 // atomic
	gSessionInserts       uint64 // atomic
	gSessionDeletes       uint64 // atomic
	gSessionUpdates       uint64 // atomic
	cgnatTableSize        uint64 // atomic
	wlTableSize           uint64 // atomic
	gSessionTotalRestores uint64 // atomic
	gSessionTotalStarts   uint64 // atomic
	gSessionTotalUpdates  uint64 // atomic
	gSessionTotalDeletes  uint64 // atomic
)

// =========================
// SESSION MAP
// =========================

// gSessionMap is a sync.Map keyed by acAccountSessionId (string) → *SessionNode.
// It replaces uthash's HASH_MAP + g_session_mutex.
var gSessionMap sync.Map

// =========================
// STATS PRINTER
// =========================

func sessionPrintStats() {
	sysInfo("================ SESSION MAP STATS ================")
	sysInfo(fmt.Sprintf("Active sessions          : %d", atomic.LoadUint64(&gSessionCount)))
	sysInfo(fmt.Sprintf("Active cgnat entries     : %d", atomic.LoadUint64(&cgnatTableSize)))
	sysInfo(fmt.Sprintf("Active whitelist entries : %d", atomic.LoadUint64(&wlTableSize)))
	sysInfo(fmt.Sprintf("Inserts                  : %d", atomic.LoadUint64(&gSessionInserts)))
	sysInfo(fmt.Sprintf("Deletes                  : %d", atomic.LoadUint64(&gSessionDeletes)))
	sysInfo(fmt.Sprintf("Updates                  : %d", atomic.LoadUint64(&gSessionUpdates)))
	sysInfo(fmt.Sprintf("Total Starts             : %d", atomic.LoadUint64(&gSessionTotalStarts)))
	sysInfo(fmt.Sprintf("Total Updates            : %d", atomic.LoadUint64(&gSessionTotalUpdates)))
	sysInfo(fmt.Sprintf("Total Deletes            : %d", atomic.LoadUint64(&gSessionTotalDeletes)))
	sysInfo(fmt.Sprintf("Total Restores           : %d", atomic.LoadUint64(&gSessionTotalRestores)))

	if count := atomic.LoadUint64(&gSessionCount); count > 0 {
		approxMem := count * uint64(sessionNodeSize)
		sysInfo(fmt.Sprintf("Approx memory            : %d KB", approxMem/1024))
	}
	sysInfo("===================================================")
}

// =========================
// INLINE HELPERS  (mirrors parser.h static inlines)
// =========================

func logInvalidAvp(typ, length uint8, offset uint32) error {
	msg := fmt.Sprintf("Invalid AVP - Type=%d Len=%d Offset=%d", typ, length, offset)
	sysErr(msg)
	return fmt.Errorf("%s", msg)
}

func ipv4ToStr(ip [4]byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])
}

// sessionFind looks up a session by ID. Returns nil if not found.
func sessionFind(id string) *SessionNode {
	if id == "" {
		return nil
	}
	v, ok := gSessionMap.Load(id)
	if !ok {
		return nil
	}
	return v.(*SessionNode)
}

// sessionInsert inserts or updates a session.
// Returns 0 on fresh insert, 1 if an existing entry was updated, -1 on error.
func sessionInsert(s *UserSessionInfo) int {
	if s == nil {
		return -1
	}
	node := &SessionNode{
		AcAccountSessionId: s.AcAccountSessionId,
		Entry:              *s,
	}
	// LoadOrStore: if key already present, update in place and return 1.
	actual, loaded := gSessionMap.LoadOrStore(s.AcAccountSessionId, node)
	if loaded {
		existing := actual.(*SessionNode)
		existing.Entry = *s
		return 1
	}
	return 0
}

// sessionDelete removes a node from the map and releases it.
func sessionDelete(node *SessionNode) {
	if node == nil {
		return
	}
	gSessionMap.Delete(node.AcAccountSessionId)
}

// =========================
// LAYER WALKERS
// =========================

// getIPLayer returns the slice starting at the IPv4 header, or nil.
func getIPLayer(pkt []byte) []byte {
	if len(pkt) < etherHdrLen {
		return nil
	}
	ethertype := binary.BigEndian.Uint16(pkt[12:14])
	if ethertype != uint16(etherTypeIPv4) {
		return nil
	}
	return pkt[etherHdrLen:]
}

// getUDPLayer returns the slice starting at the UDP header, or nil.
func getUDPLayer(ip []byte) []byte {
	if len(ip) < int(ipHdrMinLen) {
		return nil
	}
	hdrLen := uint32((ip[0] & 0x0F) << 2)
	if hdrLen < ipHdrMinLen || int(hdrLen) > len(ip) {
		return nil
	}
	if ip[9] != ipProtoUDP {
		return nil
	}
	return ip[hdrLen:]
}

// getUDPDstPort returns the destination port from a UDP slice.
func getUDPDstPort(udp []byte) uint16 {
	if len(udp) < 4 {
		return 0
	}
	return binary.BigEndian.Uint16(udp[2:4])
}

// getRadiusAcctLayer validates and returns the RADIUS accounting payload, or nil.
func getRadiusAcctLayer(pkt []byte) []byte {
	ip := getIPLayer(pkt)
	if ip == nil {
		return nil
	}
	udp := getUDPLayer(ip)
	if udp == nil {
		return nil
	}
	if getUDPDstPort(udp) != RadiusAcctPort2 {
		return nil
	}
	if len(udp) < int(udpHdrLen) {
		return nil
	}
	radius := udp[udpHdrLen:]
	if len(radius) < int(radiusHdrLen) {
		return nil
	}
	if radius[0] != radiusCodeAcctReq {
		return nil
	}
	radiusLen := binary.BigEndian.Uint16(radius[2:4])
	if radiusLen < uint16(radiusHdrLen) || int(radiusLen) > len(radius) {
		return nil
	}
	return radius
}

// =========================
// TIME HELPERS
// =========================

func setCurrentLocalTime() time.Time { return time.Now() }

func sessionStart(s *UserSessionInfo) {
	s.SessionStartTime = setCurrentLocalTime()
}

func sessionEnd(s *UserSessionInfo) {
	s.SessionEndTime = setCurrentLocalTime()
}

// =========================
// RADIUS PACKET
// =========================

// RadiusPacket mirrors the C struct.
type RadiusPacket struct {
	Data       []byte
	Payload    []byte
	Length     uint16
	Code       uint8
	Identifier uint8
}

func parseRadiusPkt(pkt []byte) (*RadiusPacket, error) {
	if len(pkt) == 0 {
		return nil, fmt.Errorf("nil packet")
	}
	radius := getRadiusAcctLayer(pkt)
	if radius == nil {
		return nil, fmt.Errorf("no RADIUS accounting layer")
	}
	radiusLen := binary.BigEndian.Uint16(radius[2:4])
	if radiusLen < uint16(radiusHdrLen) {
		return nil, fmt.Errorf("radius length too short: %d", radiusLen)
	}
	rp := &RadiusPacket{
		Data:       radius,
		Length:     radiusLen,
		Code:       radius[0],
		Identifier: radius[1],
		Payload:    radius[radiusHdrLen:radiusLen],
	}
	return rp, nil
}

// =========================
// SESSION TIMEOUT GOROUTINE  (mirrors session_timeout_thread)
// =========================

func sessionTimeoutLoop(cfg *Config) {
	var shard uint8
	for atomic.LoadInt32(&gRunning) == 1 {
		time.Sleep(200 * time.Millisecond)
		now := uint32(time.Now().Unix())
		idx := 0

		gSessionMap.Range(func(_, v any) bool {
			node := v.(*SessionNode)
			idx++
			if (idx%10) != int(shard) {
				return true
			}
			if node.Entry.DestroyTime == 0 || node.Entry.DestroyTime > now {
				return true
			}
			if cfg.Verbosity > 2 {
				sysInfo("Session expired: " + node.AcAccountSessionId)
			}
			RabbitMQPublishSessionStop(&node.Entry)
			RabbitMQPublishSessionDelete(node.AcAccountSessionId)
			sessionDelete(node)
			atomic.AddUint64(&gSessionCount, ^uint64(0)) // decrement
			atomic.AddUint64(&gSessionDeletes, 1)
			return true
		})

		shard = (shard + 1) % 10
	}
}

// =========================
// READ RADIUS ATTRIBUTES
// =========================

func readRadiusAttributes(rp *RadiusPacket, cfg *Config) (*UserSessionInfo, error) {
	if rp == nil || rp.Payload == nil {
		return nil, fmt.Errorf("nil radius packet")
	}

	pSession := &UserSessionInfo{}
	payload := rp.Payload
	payloadLen := uint32(len(payload))
	var node *SessionNode
	offset := uint32(0)

	for offset+2 <= payloadLen {
		// If we already found the node and have the status type, handle
		// updates/stops before parsing further attributes.
		if node != nil && pSession.AccountStatusType != 0 {
			if cfg.Verbosity > 2 {
				sysInfo(fmt.Sprintf("Old node found: node=%s current=%s",
					node.AcAccountSessionId, pSession.AcAccountSessionId))
			}
			switch pSession.AccountStatusType {
			case SessionUpdate:
				sessionEnd(&node.Entry)
				node.Entry.DestroyTime = uint32(time.Now().Unix()) + cfg.UpdateTimeout
				if cfg.Verbosity > 2 {
					sysInfo(fmt.Sprintf("Updated session=%s destroy_time=%d",
						node.AcAccountSessionId, node.Entry.DestroyTime))
				}
				*pSession = node.Entry
				atomic.AddUint64(&gSessionUpdates, 1)

			case SessionStop:
				if cfg.Verbosity > 2 {
					sysInfo("Stopping session_id=" + node.AcAccountSessionId)
				}
				sessionEnd(&node.Entry)
				*pSession = node.Entry
				sessionDelete(node)
				atomic.AddUint64(&gSessionCount, ^uint64(0))
				atomic.AddUint64(&gSessionDeletes, 1)
			}
			return pSession, nil
		}

		p := payload[offset:]
		typ := p[0]
		length := p[1]

		if typ == 0 {
			return nil, logInvalidAvp(typ, length, offset)
		}
		if length < 2 {
			return nil, logInvalidAvp(typ, length, offset)
		}
		if offset+uint32(length) > payloadLen {
			return nil, logInvalidAvp(typ, length, offset)
		}

		value := p[2:length]
		valueLen := uint16(length - 2)

		switch typ {
		// ---------------------------------------------------------
		// ACCT STATUS TYPE
		// ---------------------------------------------------------
		case avpAcctStatusType:
			if valueLen != 4 {
				return nil, logInvalidAvp(typ, length, offset)
			}
			v := binary.BigEndian.Uint32(value)
			pSession.AccountStatusType = uint8(v)
			switch pSession.AccountStatusType {
			case SessionStart:
				atomic.AddUint64(&gSessionTotalStarts, 1)
			case SessionUpdate:
				atomic.AddUint64(&gSessionTotalUpdates, 1)
			case SessionStop:
				atomic.AddUint64(&gSessionTotalDeletes, 1)
			}
			pSession.ValidAttributes |= validAcctStatusType

		// ---------------------------------------------------------
		// SESSION ID
		// ---------------------------------------------------------
		case avpAcctSessionID:
			copyLen := valueLen
			if int(copyLen) >= len(pSession.AcAccountSessionId) {
				copyLen = uint16(len(pSession.AcAccountSessionId) - 1)
			}
			pSession.AcAccountSessionId = string(value[:copyLen])
			node = sessionFind(pSession.AcAccountSessionId)
			pSession.ValidAttributes |= validAcctSessionID

		// ---------------------------------------------------------
		// CALLING STATION ID
		// ---------------------------------------------------------
		case avpCallingStationID:
			if valueLen < 3 || int(valueLen) >= len(pSession.AcCallingStationId) {
				return nil, fmt.Errorf("calling station id length invalid: %d", valueLen)
			}
			pSession.AcCallingStationId = string(value[:valueLen])
			pSession.ValidAttributes |= validCallingStationID

			var wlInfo WhitelistInfo
			if wlLookup(pSession.AcCallingStationId, &wlInfo) {
				if wlInfo.Status {
					pSession.IsWL = 1
				} else {
					pSession.IsWL = 0
				}
			}

		// ---------------------------------------------------------
		// EVENT TIMESTAMP
		// ---------------------------------------------------------
		case avpEventTimestamp:
			if valueLen != 4 {
				return nil, logInvalidAvp(typ, length, offset)
			}
			pSession.EventTimestamp = binary.BigEndian.Uint32(value)
			pSession.ValidAttributes |= validEventTimestamp

		// ---------------------------------------------------------
		// FRAMED IPv4
		// ---------------------------------------------------------
		case avpFramedIPAddress:
			if valueLen != 4 {
				return nil, fmt.Errorf("framed ip address length invalid: %d", valueLen)
			}
			copy(pSession.FramedIPv4Address[:], value[:4])
			pSession.ValidAttributes |= validFramedIPv4

			ipStr := ipv4ToStr(pSession.FramedIPv4Address)
			var entry CgnatEntry
			if cgnatLookup(ipStr, &entry) {
				if addr := net.ParseIP(entry.NatIP).To4(); addr != nil {
					copy(pSession.FramedIPv4PubAddress[:], addr)
					pSession.PortStart = entry.StartPort
					pSession.PortEnd = entry.EndPort
				}
			}

		// ---------------------------------------------------------
		// FRAMED IPv6 PREFIX
		// ---------------------------------------------------------
		case avpFramedIPv6Prefix:
			if valueLen < 2 || valueLen > ipv6PrefixMaxLen {
				return nil, fmt.Errorf("framed ipv6 prefix length invalid: %d", valueLen)
			}
			copy(pSession.FramedIPv6Prefix[:], value[:valueLen])
			pSession.ValidAttributes |= validFramedIPv6Prefix

		// ---------------------------------------------------------
		// MULTI SESSION ID
		// ---------------------------------------------------------
		case avpAcctMultiSessionID:
			copyLen := valueLen
			if int(copyLen) >= len(pSession.AcMultiSessionId) {
				copyLen = uint16(len(pSession.AcMultiSessionId) - 1)
			}
			pSession.AcMultiSessionId = string(value[:copyLen])
			pSession.ValidAttributes |= validAcctMultiSessionID

		// ---------------------------------------------------------
		// EXTRA AVPs
		// ---------------------------------------------------------
		default:
			if cfg.ExtractAll && pSession.ExtraAvpCount < maxExtraAvps {
				avp := &pSession.ExtraAvps[pSession.ExtraAvpCount]
				avp.Type = typ
				avp.Len = length
				copyLen := valueLen
				if copyLen > maxAvpValue {
					copyLen = maxAvpValue
				}
				copy(avp.Value[:], value[:copyLen])
				pSession.ExtraAvpCount++
			}
		}

		offset += uint32(length)
	}

	// ---------------------------------------------------------
	// SESSION START / UPDATE (new node path)
	// ---------------------------------------------------------
	if pSession.AccountStatusType == SessionStart || pSession.AccountStatusType == SessionUpdate {
		if cfg.Verbosity > 2 {
			sysInfo(fmt.Sprintf("Inserting session_id=%s status=%d",
				pSession.AcAccountSessionId, pSession.AccountStatusType))
		}
		sessionStart(pSession)
		pSession.DestroyTime = uint32(time.Now().Unix()) + cfg.UpdateTimeout

		rc := sessionInsert(pSession)
		if pSession.AccountStatusType == SessionUpdate {
			RabbitMQPublishSessionStart(pSession)
			RabbitMQPublishSessionSync(pSession)
		}
		if rc == 0 {
			atomic.AddUint64(&gSessionCount, 1)
			atomic.AddUint64(&gSessionInserts, 1)
		} else if rc < 0 {
			sysErr("Failed to insert session_id=" + pSession.AcAccountSessionId)
		}
	}

	return pSession, nil
}