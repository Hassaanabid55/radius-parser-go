package main

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// =========================
// SESSION TYPE STRING
// =========================

func sessionTypeStr(t uint8) string {
	switch t {
	case SessionStart:
		return "START"
	case SessionStop:
		return "STOP"
	case SessionUpdate:
		return "UPDATE"
	default:
		return "UNKNOWN"
	}
}

// =========================
// PRINT USER SESSION
// =========================

func printUserSession(s *UserSessionInfo, cfg *Config) {
	if s == nil {
		return
	}

	// IPv4 private
	ipv4Priv := "[not present]"
	if s.ValidAttributes&validFramedIPv4 != 0 {
		ipv4Priv = ipv4ToStr(s.FramedIPv4Address)
	}

	// IPv4 public (present when CGNAT ports are set)
	ipv4Pub := "[not present]"
	if s.PortStart != 0 || s.PortEnd != 0 {
		ipv4Pub = ipv4ToStr(s.FramedIPv4PubAddress)
	}

	// IPv6 prefix
	ipv6 := "[not present]"
	if s.ValidAttributes&validFramedIPv6Prefix != 0 {
		// The prefix bytes start at offset 2 (reserved + prefix-length octets).
		addr := net.IP(s.FramedIPv6Prefix[2:])
		if len(addr) >= net.IPv6len {
			ipv6 = addr[:net.IPv6len].String()
		}
	}

	// Event timestamp
	tsbuf := "[not present]"
	if s.ValidAttributes&validEventTimestamp != 0 {
		tsbuf = time.Unix(int64(s.EventTimestamp), 0).Local().
			Format("2006-01-02 15:04:05")
	}

	sessionID := "[not present]"
	if s.ValidAttributes&validAcctSessionID != 0 {
		sessionID = s.AcAccountSessionId
	}

	multiID := "[not present]"
	if s.ValidAttributes&validAcctMultiSessionID != 0 {
		multiID = s.AcMultiSessionId
	}

	callingStation := "[not present]"
	if s.ValidAttributes&validCallingStationID != 0 {
		callingStation = s.AcCallingStationId
	}

	wl := "NO"
	if s.IsWL != 0 {
		wl = "YES"
	}

	var b strings.Builder
	fmt.Fprintf(&b,
		"[SESSION] Type=%s | SessionId=%s | MultiSessionId=%s | CallingStation=%s"+
			" | IPv4Private=%s | IPv4Public=%s | IPv6=%s"+
			" | Ports=%d-%d | Timestamp=%d | Time=%s | WL=%s",
		sessionTypeStr(s.AccountStatusType),
		sessionID, multiID, callingStation,
		ipv4Priv, ipv4Pub, ipv6,
		s.PortStart, s.PortEnd,
		s.EventTimestamp, tsbuf,
		wl,
	)

	// Extra AVPs
	if cfg.ExtractAll && s.ExtraAvpCount > 0 {
		b.WriteString(" | AVPs=[")
		for i := uint16(0); i < s.ExtraAvpCount; i++ {
			avp := &s.ExtraAvps[i]
			payloadLen := uint16(avp.Len) - 2
			if payloadLen > maxAvpValue {
				payloadLen = maxAvpValue
			}

			var hex strings.Builder
			for j := uint16(0); j < payloadLen; j++ {
				fmt.Fprintf(&hex, "%02x", avp.Value[j])
				if j+1 < payloadLen {
					hex.WriteByte(':')
				}
			}

			fmt.Fprintf(&b, "{Type=%d,Len=%d,Value=%s}", avp.Type, avp.Len, hex.String())
			if i+1 < s.ExtraAvpCount {
				b.WriteByte(',')
			}
		}
		b.WriteByte(']')
	}

	sysInfo(b.String())
}