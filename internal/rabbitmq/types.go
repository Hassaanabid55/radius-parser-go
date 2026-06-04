package rabbitmq

import "radius-parser/internal/session"

type StatsMessage struct {
	SessionID  string `json:"session_id"`
	PacketCount uint32 `json:"packet_count"`
	ByeSeen    bool   `json:"bye_seen"`
}

type SessionMessage struct {
	session.UserSession
}