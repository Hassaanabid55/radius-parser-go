package session

import (
	"log"
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
	CallingStationID string

	FramedIPv4    string
	PublicIPv4    string
	FramedIPv6    string
	FramedIPv6Len int

	PortStart uint16
	PortEnd   uint16

	SessionStart time.Time
	SessionEnd   time.Time

	byeAcks  int
	StopSent bool

	ExtraAVPs []ExtraAVP
}

var (
	Map = make(map[string]*UserSession)
	Mu  sync.RWMutex
)

var ActiveNode int

func Lock() {
	Mu.Lock()
}

func Unlock() {
	Mu.Unlock()
}

func Find(id string) *UserSession {

	Mu.RLock()
	defer Mu.RUnlock()

	return Map[id]
}

func Insert(s *UserSession) bool {

	Mu.Lock()
	defer Mu.Unlock()

	id := s.AccountSessionID

	if _, exists := Map[id]; exists {
		return false
	}

	Map[id] = s
	return true
}

func DeleteNode(node *UserSession) {
	delete(Map, node.AccountSessionID)
}

func SetStartTime(s *UserSession) {
	s.SessionStart = time.Now()
}

func End(s *UserSession) {
	s.SessionEnd = time.Now()
}

func AddExtraAVP(s *UserSession, t byte, v []byte) {

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

func UpdatePacketCount(sessionID string, delta uint32, byeSeen bool) (bool, UserSession) {
	Mu.Lock()
	defer Mu.Unlock()

	node, ok := Map[sessionID]
	if !ok || node == nil {
		return false, UserSession{}
	}

	if byeSeen {
		node.byeAcks++
	}
	node.PacketCount += delta
	log.Printf("stats Updated: %+v",node)

	if node.byeAcks >= ActiveNode {
		session := *node
		DeleteNode(node)
		return true, session
	}

	return false, UserSession{}
}
