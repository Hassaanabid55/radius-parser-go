package session

import (
	"sync"
	"time"
)

const MaxExtraAVPs = 128

type ExtraAVP struct {
	Type  uint8
	Len   uint8
	Value []byte
}

type UserSession struct {
	EventTimestamp uint32
	PacketCount    uint32
	DestroyTime    uint32

	AccountStatusType uint8
	IsWhitelist       bool

	AccountSessionID string
	MultiSessionID   string
	CallingStationID string

	FramedIPv4 [4]byte
	PublicIPv4 [4]byte
	FramedIPv6 [64]byte

	PortStart uint16
	PortEnd   uint16

	SessionStart time.Time
	SessionEnd   time.Time

	ExtraAVPs []ExtraAVP
}

type SessionNode struct {
	Entry UserSession
}

var (
	Map = make(map[string]*SessionNode)
	Mu  sync.RWMutex
)

func Lock() {
	Mu.Lock()
}

func Unlock() {
	Mu.Unlock()
}

func Find(id string) *SessionNode {

	Mu.RLock()
	defer Mu.RUnlock()

	return Map[id]
}

func Insert(s *UserSession) int {

	Mu.Lock()
	defer Mu.Unlock()

	Map[s.AccountSessionID] =
		&SessionNode{
			Entry: *s,
		}

	return 0
}

func DeleteNode(node *SessionNode) {

	delete(
		Map,
		node.Entry.AccountSessionID,
	)
}

func SetStartTime(s *UserSession) {
	s.SessionStart = time.Now()
}

func End(s *UserSession) {
	s.SessionEnd = time.Now()
}

func AddExtraAVP(
	s *UserSession,
	t byte,
	v []byte,
) {

	if len(s.ExtraAVPs) >= MaxExtraAVPs {
		return
	}

	cp := make([]byte, len(v))
	copy(cp, v)

	s.ExtraAVPs = append(
		s.ExtraAVPs,
		ExtraAVP{
			Type:  t,
			Len:   uint8(len(v)),
			Value: cp,
		},
	)
}

func UpdatePacketCount(sessionID string, delta uint32) {
	Mu.RLock()
	node := Map[sessionID]
	Mu.RUnlock()

	if node == nil {
		return
	}

	Mu.Lock()
	node.Entry.PacketCount += delta
	Mu.Unlock()
}