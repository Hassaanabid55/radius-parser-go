package rabbitmq

import "time"

//For Consumers

type StatsMessage struct {
	SessionID   string `json:"session_id"`
	PacketCount uint32 `json:"packet_count"`
	ByeSeen     bool   `json:"bye_seen"`
}

type HeartbeatMessage struct {
	NodeID    string    `json:"node_id"`
	TimeStamp time.Time `json:"time_stamp"`
}

//For Producers

type StartSessionMessage struct {
	AccountSessionID string
	FramedIPv4       string
	PublicIPv4       string
	FramedIPv6       string
	PortStart        uint16
	PortEnd          uint16
	IsWhitelist      bool
	FramedIPv6Len    int
}

type StopSessionMessage struct {
	AccountSessionID string
}
