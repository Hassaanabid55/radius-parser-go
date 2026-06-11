# Radius Parser Go

High-performance RADIUS Accounting parser written in Go.

The application captures RADIUS Accounting packets from live interfaces or PCAP files, maintains an in-memory session database, performs CGNAT and whitelist lookups, publishes session lifecycle events through RabbitMQ, consumes bootstrap data from Task Manager, and exchanges statistics with Filtering Applications.

## Features

### Packet Processing

- Live interface capture
- Offline PCAP replay
- High-speed packet ingestion
- Multi-core packet processing
- RADIUS Accounting packet parsing

### Session Management

- Session Start handling
- Session Update handling
- Session Stop handling
- Session Finalization
- Session timeout management
- In-memory session tracking

### Correlation

- CGNAT correlation
- MSISDN whitelist lookup
- IPv4 and IPv6 session support

### RabbitMQ Integration

- Session lifecycle publishing
- Statistics consumption
- Bootstrap synchronization
- Node heartbeat publishing

### Monitoring

- Runtime statistics collection
- Session monitoring
- CGNAT cache monitoring
- Whitelist cache monitoring

## Architecture

```text
                     ┌──────────────────────┐
                     │    Task Manager      │
                     │                      │
              ┌──────│ DB Owner             │
              |      │ RabbitMQ Owner       │
              |      └──────────┬───────────┘
              |                 │
              ▲          bootstrap.cgnat
              |          bootstrap.whitelist
       session.final            |
              |                 │
              |                 ▼
┌───────────────────────────────────────────────────┐
│                    Radius Parser                  │
│                                                   │
│ Capture → Parse → Session Engine → RabbitMQ       │
│                                                   │
└─────────────┬──────────────────────┬──────────────┘
              │                      │
              ▼                      ▲
      session.start                  |
      session.stop             node.heartbeat
              |                session.stats
              │                      │
              ▼                      │
     ┌─────────────────────┐         │
     │ Filtering Apps      │─────────┘
     │ (1..N instances)    │
     └─────────────────────┘
              │
              ▼

        Task Manager
```

## Session Lifecycle

### Start Packet

When a RADIUS Accounting Start packet arrives:

1. Session is created.
2. Session inserted into memory.
3. Session start timestamp recorded.
4. Session published to RabbitMQ.

Routing Key:

```text
session.start
```

### Update Packet

When a RADIUS Accounting Update packet arrives:

1. Existing session located.
2. Session timeout refreshed.
3. Session updated in memory.

No start or stop events are generated.

### Stop Packet

When a RADIUS Accounting Stop packet arrives:

1. Session located.
2. Session end timestamp recorded.
3. Stop event published.
4. Session remains in memory awaiting statistics updates.
5. Filtering applications continue reporting packet counts.

Routing Key:

```text
session.stop
```

### Final Session Export

After statistics aggregation completes:

1. Packet counts finalized.
2. Final session exported.
3. Session removed from memory.

Routing Key:

```text
session.final
```

## RabbitMQ Topology

### Exchange

```text
radius_exchange
```

### Exchange Type

```text
topic
```

### Queues

| Queue               | Producer               | Consumer               |
|---------------------|------------------------|------------------------|
| session.start       | Radius Parser          | Filtering Applications |
| session.stop        | Radius Parser          | Filtering Applications |
| session.final       | Radius Parser          | Task Manager           |
| session.stats       | Filtering Applications | Radius Parser          |
| bootstrap.cgnat     | Task Manager           | Radius Parser          |
| bootstrap.whitelist | Task Manager           | Radius Parser          |
| node.heartbeat      | Filtering Applications | Radius Parser          |

### Queue Purposes

| Queue               | Purpose                               |
|---------------------|---------------------------------------|
| session.start       | New subscriber session                |
| session.stop        | Session termination notification      |
| session.final       | Finalized session export              |
| session.stats       | Packet counters and BYE notifications |
| bootstrap.cgnat     | CGNAT lookup synchronization          |
| bootstrap.whitelist | Whitelist synchronization             |
| node.heartbeat      | Filtering node liveness monitoring    |

## Message Structures

### session.start

Routing Key:

```text
session.start
```

Structure:

```go
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
```

Example:

```json
{
  "account_session_id": "abc123",
  "framed_ipv4": "10.250.41.153",
  "public_ipv4": "5.38.72.0",
  "framed_ipv6": "2001:db8::1",
  "port_start": 1,
  "port_end": 6666,
  "is_whitelist": true,
  "framed_ipv6_len": 64
}
```

### session.stop

Routing Key:

```text
session.stop
```

Structure:

```go
type StopSessionMessage struct {
    AccountSessionID string
}
```

Example:

```json
{
  "account_session_id": "abc123"
}
```

### session.final

Routing Key:

```text
session.final
```

Structure:

```go
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

    byeAcks int

    ExtraAVPs []ExtraAVP
}
```

### session.stats

Routing Key:

```text
session.stats
```

Structure:

```go
type StatsMessage struct {
    SessionID   string `json:"session_id"`
    PacketCount uint32 `json:"packet_count"`
    ByeSeen     bool   `json:"bye_seen"`
}
```

Example:

```json
{
  "session_id": "abc123",
  "packet_count": 1500,
  "bye_seen": true
}
```

Used by Radius Parser to update packet counters and determine when a session can be finalized.

### bootstrap.cgnat

Routing Key:

```text
bootstrap.cgnat
```

Structure:

```go
type CgnatEntry struct {
    InsideIP  string
    NatIP     string
    StartPort uint16
    EndPort   uint16
    delete    bool
}
```

Example:

```json
{
  "inside_ip": "10.250.41.153",
  "nat_ip": "5.38.72.0",
  "start_port": 1,
  "end_port": 6666,
  "delete": false
}
```

Used to populate and update the in-memory CGNAT lookup cache.

### bootstrap.whitelist

Routing Key:

```text
bootstrap.whitelist
```

Structure:

```go
type WhitelistInfo struct {
    MSISDN string
    Status bool
    delete bool
}
```

Example:

```json
{
  "msisdn": "971501234567",
  "status": true,
  "delete": false
}
```

Used to populate and update the in-memory whitelist cache.

### node.heartbeat

Routing Key:

```text
node.heartbeat
```

Structure:

```go
type HeartbeatMessage struct {
    NodeID    string    `json:"node_id"`
    TimeStamp time.Time `json:"time_stamp"`
}
```

Example:

```json
{
  "node_id": "radius-parser-siteA",
  "time_stamp": "2026-06-11T12:30:00Z"
}
```

Used by MDF applications to advertise liveness and health status to Radius Parser.

## Session Structure

```go
type ExtraAVP struct {
    Type  uint8
    Len   uint8
    Value []byte
}

type UserSession struct {
    EventTimestamp uint32

    PacketCount uint32
    DestroyTime uint32

    AccountStatusType uint8
    IsWhitelist bool

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

    ExtraAVPs []ExtraAVP
}
```

## Configuration

### Example Configuration

```ini
# =========================
# GENERAL SETTINGS
# =========================

interface_name=dummy0
threads=2,4,6,8,10,12,14,16,18
extract_all=no
verbosity=3
caplen=3200
update_timeout=300
ring_buffer_size=1048576

# =========================
# INPUT FILES
# =========================

# input_file=./pcap/RADIUS_input.pcap

# =========================
# RabbitMQ CONFIG
# =========================

rabbitmq_host=127.0.0.1
rabbitmq_port=5672
rabbitmq_user=radius_user
rabbitmq_password=radius_pass
rabbitmq_vhost=radius
rabbitmq_exchange=radius_exchange
```

## Building

### Requirements

- Go 1.24+
- RabbitMQ 3.13+
- Linux

### Build

```bash
go mod tidy

go build -o radius-parser ./cmd/radius-parser
```

## Running

The application can operate in either:

- Live capture mode
- Offline PCAP replay mode

Run:

```bash
./radius-parser -c <path/to/radius_parser.conf>
```

## Live Monitoring

The parser maintains runtime statistics:

```text
Active Sessions
CGNAT Entries
Whitelist Entries

Session Inserts
Session Updates
Session Deletes

Total Starts
Total Updates
Total Stops
Total Restores
```

Statistics are periodically logged by the monitoring thread.

## Performance Goals

- Multi-core packet processing
- Lock-efficient session lookups
- O(1) whitelist lookups
- O(1) CGNAT lookups
- RabbitMQ topic-based fanout
- High-throughput session lifecycle processing
- Suitable for ISP-scale RADIUS accounting environments
