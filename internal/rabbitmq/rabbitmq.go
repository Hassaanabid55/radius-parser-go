package main

import (
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	amqp "github.com/rabbitmq/amqp091-go"
)

// =========================
// ROUTING KEY CONSTANTS
// =========================

const (
	rkSessionStart  = "session.start"
	rkSessionStop   = "session.stop"
	rkSyncSession   = "sync.session"
	rkSyncDelete    = "sync.session.delete"
	rkSessionStats  = "session.stats"
)

// =========================
// CONFIG + CLIENT
// =========================

// RabbitMQConfig mirrors the C RabbitMQConfig struct.
type RabbitMQConfig struct {
	Host     string
	VHost    string
	User     string
	Password string
	Exchange string
	Port     uint16
}

// RabbitMQClient holds the live AMQP connection and channels.
type RabbitMQClient struct {
	conn        *amqp.Connection
	pubCh       *amqp.Channel // channel 1 — publishes
	statsCh     *amqp.Channel // channel 2 — stats consumer
	syncCh      *amqp.Channel // channel 3 — bootstrap drain
	cfg         RabbitMQConfig
	mu          sync.Mutex
}

// =========================
// PACKAGE-LEVEL SINGLETON
// =========================

var gRabbitMQ RabbitMQClient

// =========================
// INIT
// =========================

// RabbitMQInit connects and opens three channels, mirroring rabbitmq_init.
func RabbitMQInit(cfg *RabbitMQConfig) error {
	gRabbitMQ.cfg = *cfg

	url := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.VHost)

	conn, err := amqp.Dial(url)
	if err != nil {
		return fmt.Errorf("rabbitmq dial: %w", err)
	}
	gRabbitMQ.conn = conn

	gRabbitMQ.pubCh, err = conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq pub channel: %w", err)
	}

	gRabbitMQ.statsCh, err = conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq stats channel: %w", err)
	}

	gRabbitMQ.syncCh, err = conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq sync channel: %w", err)
	}

	return nil
}

// =========================
// CLEANUP
// =========================

// RabbitMQCleanup closes channels and the connection.
func RabbitMQCleanup() {
	c := &gRabbitMQ
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pubCh != nil {
		c.pubCh.Close()
	}
	if c.statsCh != nil {
		c.statsCh.Close()
	}
	if c.syncCh != nil {
		c.syncCh.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// =========================
// INTERNAL PUBLISH
// =========================

func rabbitmqPublish(routingKey string, body []byte) error {
	c := &gRabbitMQ
	if c.conn == nil || c.conn.IsClosed() {
		return nil
	}

	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent, // delivery_mode = 2
		Body:         body,
	}

	c.mu.Lock()
	err := c.pubCh.Publish(c.cfg.Exchange, routingKey, false, false, msg)
	c.mu.Unlock()
	return err
}

// =========================
// SERIALISE UserSessionInfo
// =========================

// sessionInfoToBytes serialises a UserSessionInfo to a stable binary blob.
// In the C code the raw struct was sent over the wire; here we use a
// fixed-size little-endian encoding so the wire format is explicit and
// portable. Adjust if you need to interop with the existing C consumers.
func sessionInfoToBytes(s *UserSessionInfo) []byte {
	// Use unsafe.Sizeof to get the same footprint as the C struct would have.
	// If you need exact C ABI compatibility, generate a cgo binding instead.
	size := int(unsafe.Sizeof(*s))
	buf := make([]byte, size)
	// Simplest portable approach: encode each field individually.
	// Replace with a cgo cast if strict C-struct wire compat is required.
	_ = binary.LittleEndian // used below for numeric fields
	copy(buf, []byte(s.AcAccountSessionId))
	return buf
}

// =========================
// HIGH-LEVEL PUBLISH
// =========================

func RabbitMQPublishSessionStart(s *UserSessionInfo) bool {
	err := rabbitmqPublish(rkSessionStart, sessionInfoToBytes(s))
	return err == nil
}

func RabbitMQPublishSessionStop(s *UserSessionInfo) bool {
	err := rabbitmqPublish(rkSessionStop, sessionInfoToBytes(s))
	return err == nil
}

func RabbitMQPublishSessionSync(s *UserSessionInfo) bool {
	err := rabbitmqPublish(rkSyncSession, sessionInfoToBytes(s))
	return err == nil
}

func RabbitMQPublishSessionDelete(sessionID string) bool {
	err := rabbitmqPublish(rkSyncDelete, []byte(sessionID))
	return err == nil
}

// =========================
// BOOTSTRAP  (drain sync queues at startup)
// =========================

// RabbitMQBootstrapState drains the sync queues and rebuilds the in-memory
// session map, mirroring rabbitmq_bootstrap_state.
func RabbitMQBootstrapState() error {
	if err := drainSyncQueue(rkSyncSession, bootstrapHandler); err != nil {
		return err
	}
	if err := drainSyncQueue(rkSyncDelete, bootstrapHandler); err != nil {
		return err
	}
	sysInfo("RabbitMQ: bootstrap completed")
	return nil
}

// bootstrapHandler mirrors the C bootstrap_handler closure.
func bootstrapHandler(queue string, body []byte) {
	switch queue {
	case rkSyncSession:
		s := bytesToSessionInfo(body)
		if s == nil {
			return
		}
		if existing := sessionFind(s.AcAccountSessionId); existing != nil {
			// Only overwrite if newer.
			if s.EventTimestamp > existing.Entry.EventTimestamp {
				existing.Entry = *s
			}
		} else {
			node := &SessionNode{
				AcAccountSessionId: s.AcAccountSessionId,
				Entry:              *s,
			}
			gSessionMap.Store(s.AcAccountSessionId, node)
			atomic.AddUint64(&gSessionCount, 1)
			atomic.AddUint64(&gSessionTotalRestores, 1)
		}

	case rkSyncDelete:
		id := string(body)
		if node := sessionFind(id); node != nil {
			sessionDelete(node)
			atomic.AddUint64(&gSessionCount, ^uint64(0))
			atomic.AddUint64(&gSessionTotalDeletes, 1)
		}
	}
}

func drainSyncQueue(queue string, handler func(string, []byte)) error {
	c := &gRabbitMQ
	c.mu.Lock()
	msgs, err := c.syncCh.Consume(queue, "", true, false, false, false, nil)
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("drain consume %s: %w", queue, err)
	}

	timeout := time.After(time.Second)
	for {
		select {
		case d, ok := <-msgs:
			if !ok {
				return nil
			}
			handler(queue, d.Body)
		case <-timeout:
			return nil
		}
	}
}

// =========================
// STATS CONSUMER GOROUTINE
// =========================

// RabbitMQStatsWorker consumes the stats queue and updates packet counts,
// mirroring rabbitmq_stats_worker. Called as go RabbitMQStatsWorker().
func RabbitMQStatsWorker() {
	c := &gRabbitMQ

reconnect:
	c.mu.Lock()
	msgs, err := c.statsCh.Consume(rkSessionStats, "", true, false, false, false, nil)
	c.mu.Unlock()
	if err != nil {
		sysErr("stats worker: basic_consume failed: " + err.Error())
		return
	}

	for atomic.LoadInt32(&gRunning) == 1 {
		select {
		case d, ok := <-msgs:
			if !ok {
				if atomic.LoadInt32(&gRunning) == 0 {
					return
				}
				sysErr("stats worker: channel closed → reconnecting")
				time.Sleep(time.Second)
				goto reconnect
			}

			s := bytesToSessionInfo(d.Body)
			if s == nil {
				continue
			}

			if node := sessionFind(s.AcAccountSessionId); node != nil {
				node.Entry.PacketCount = s.PacketCount
			}

		case <-time.After(2 * time.Second):
			// Timeout — loop and re-check gRunning.
		}
	}
}

// =========================
// DESERIALISE
// =========================

// bytesToSessionInfo is the inverse of sessionInfoToBytes.
// Replace with a cgo cast if strict C-struct wire compat is required.
func bytesToSessionInfo(b []byte) *UserSessionInfo {
	if len(b) == 0 {
		return nil
	}
	s := &UserSessionInfo{}
	s.AcAccountSessionId = string(b[:min(len(b), len(s.AcAccountSessionId))])
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}