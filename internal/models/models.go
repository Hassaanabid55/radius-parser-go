package models

import (
	"sync"
	"sync/atomic"
	"time"

	"radius-parser/internal/constants"
)

/* =========================================================
 * GLOBAL COUNTERS (extern replacements)
 * ========================================================= */

var (
	SessionCount       uint64
	SessionInserts     uint64
	SessionDeletes     uint64
	SessionUpdates     uint64
	CGNATTableSize     uint64
	WLTableSize        uint64
	SessionTotalRestores uint64
	SessionTotalStarts   uint64
	SessionTotalUpdates  uint64
	SessionTotalDeletes  uint64
)

/* =========================================================
 * CGNAT
 * ========================================================= */

type CgnatEntry struct {
	InsideIP  string
	NatIP     string
	StartPort uint16
	EndPort   uint16
}

/*
Instead of UT_hash_handle (C hash table),
Go uses native maps:
	map[string]*CgnatNode
*/

type CgnatNode struct {
	InsideIP string
	Entry    CgnatEntry
}

/* =========================================================
 * WHITELIST
 * ========================================================= */

type WhitelistInfo struct {
	MSISDN string
	Status bool
}

type WlNode struct {
	MSISDN string
	Info   WhitelistInfo
}

/* =========================================================
 * EXTRA AVPS
 * ========================================================= */

type ExtraAVP struct {
	Type  uint8
	Len   uint8
	Value [constants.MaxAVPValue]byte
}

/* =========================================================
 * USER SESSION
 * ========================================================= */

type UserSessionInfo struct {
	/* FAST MODE FIELDS */

	ValidAttributes uint64

	EventTimestamp uint32
	PacketCount    uint32
	DestroyTime    uint32

	AccountStatusType uint8
	IsWL              bool

	AccountSessionID   string
	MultiSessionID     string
	CallingStationID   string

	FramedIPv4Address     [constants.IPv4Octets]byte
	FramedIPv4PubAddress  [constants.IPv4Octets]byte
	FramedIPv6Prefix      [constants.IPv6PrefixMaxLen]byte

	PortStart uint16
	PortEnd   uint16

	SessionStartTime time.Time
	SessionEndTime   time.Time

	/* OPTIONAL AVPS */

	ExtraAVPCount uint16
	ExtraAVPs     [constants.MaxExtraAVPs]ExtraAVP
}

type SessionNode struct {
	AccountSessionID string
	Entry            UserSessionInfo
}

/* =========================================================
 * RABBITMQ CONFIGURATION
 * ========================================================= */

type RabbitMQConfig struct {
	Host     string
	VHost    string
	User     string
	Password string
	Exchange string
	Port     uint16
}

/* =========================================================
 * RABBITMQ CLIENT
 * ========================================================= */

// NOTE: real amqp types should come from your RabbitMQ library
type RabbitMQClient struct {
	Conn         interface{}
	Socket       interface{}
	PublishCh    int
	StatsCh      int
	SyncCh       int
	Cfg          RabbitMQConfig
}

/* =========================================================
 * RABBITMQ BOOTSTRAP
 * ========================================================= */

type RabbitMQSyncHandler func(
	routingKey string,
	body []byte,
	ctx interface{},
)

type RabbitMQBootstrapCtx struct {
	SessionMap    map[string]*SessionNode
	SessionsLoaded int
}

/* =========================================================
 * PUBLISH QUEUE
 * ========================================================= */

type RabbitPublishEvent struct {
	RoutingKey string
	Session    UserSessionInfo
	SessionID  string
	Len        uint
	Next       *RabbitPublishEvent
}

type RabbitPublishQueue struct {
	Head *RabbitPublishEvent
	Tail *RabbitPublishEvent

	Mutex sync.Mutex
	Cond  *sync.Cond
}

/* =========================================================
 * DATABASE CONFIG
 * ========================================================= */

type DBConfig struct {
	Enabled  bool
	Host     string
	User     string
	Password string
	Database string
	Port     uint16
}

/* =========================================================
 * RADIUS PACKET
 * ========================================================= */

type RadiusPacket struct {
	Data       []byte
	Payload    []byte
	Length     uint16
	PayloadLen uint16
	Code       uint8
	Identifier uint8
}

type RadiusAttributeMap struct {
	Type uint16
	Name string
}

/* =========================================================
 * TASK / PACKET CONTEXT
 * ========================================================= */

type Task struct {
	/* RAW PACKET */
	Data         []byte
	PacketLength uint32

	/* TIMESTAMP */
	Timestamp time.Time

	/* OFFSETS */
	EthernetOffset uint16
	IPOffset       uint16
	UDPOffset      uint16
	RadiusOffset   uint16

	/* LENGTHS */
	IPHeaderLength uint16
	UDPLength      uint16
	RadiusLength   uint16

	/* PROTOCOL INFO */
	Ethertype  uint16
	IPVersion  uint8
	IPProtocol uint8

	SrcIP uint32
	DstIP uint32

	SrcPort uint16
	DstPort uint16

	PacketType constants.PacketType

	/* POINTERS */
	Ethernet []byte
	IP       []byte
	UDP      []byte
	Radius   []byte
}

/* =========================================================
 * TASK QUEUE (thread-safe replacement of pthread queue)
 * ========================================================= */

type TaskQueue struct {
	Tasks []Task

	Head  int
	Tail  int
	Count int

	Mutex    sync.Mutex
	NotEmpty *sync.Cond
	NotFull  *sync.Cond

	Shutdown bool
}

/* Helper constructor for TaskQueue */

func NewTaskQueue(size int) *TaskQueue {
	q := &TaskQueue{
		Tasks: make([]Task, size),
	}
	q.NotEmpty = sync.NewCond(&q.Mutex)
	q.NotFull = sync.NewCond(&q.Mutex)
	return q
}

/* =========================================================
 * OPTIONAL ATOMIC HELPERS (replacement for C counters)
 * ========================================================= */

func IncSessionCount() {
	atomic.AddUint64(&SessionCount, 1)
}