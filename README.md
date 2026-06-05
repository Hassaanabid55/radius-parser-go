# Radius Parser Go

High-performance RADIUS Accounting parser written in Go.

The application captures RADIUS Accounting packets from live interfaces or PCAP files, maintains an in-memory session database, performs CGNAT and whitelist lookups, publishes session lifecycle events through RabbitMQ, and consumes external statistics and bootstrap data from other applications.

---

# Features

* High-speed packet capture

  * Live interface capture
  * Offline PCAP replay
* RADIUS Accounting packet parsing
* Session tracking

  * Start
  * Update
  * Stop
* In-memory session database
* CGNAT correlation
* MSISDN whitelist lookup
* RabbitMQ integration
* Session timeout handling
* Multi-core worker processing
* Statistics monitoring
* Hot-reload bootstrap data via RabbitMQ

---

# Architecture

```text
                   +--------------------+
                   | Task Management    |
                   | Application        |
                   +---------+----------+
                             |
                             |
                  bootstrap.cgnat
                  bootstrap.whitelist
                             |
                             v

+----------------------------------------------------+
|                  Radius Parser                     |
|                                                    |
|  Capture -> Parse -> Session Engine -> RabbitMQ    |
|                                                    |
+----------------------------------------------------+
           |                    ^
           |                    |
           |                    |
           v                    |
   session.start          session.stats
   session.stop                 |
   session.final                |
           |                    |
           v                    |
+----------------------+        |
| Filtering Apps       |--------+
| (1..N instances)     |
+----------------------+
```

---

# Session Lifecycle

## Start Packet

When a RADIUS Accounting Start packet arrives:

1. Session is created.
2. Session inserted into memory.
3. Session start timestamp recorded.
4. Session published to RabbitMQ.

Routing Key:

```text
session.start
```

---

## Update Packet

When a RADIUS Accounting Update packet arrives:

1. Existing session located.
2. Session timeout refreshed.
3. Session updated in memory.

No start/stop events are generated.

---

## Stop Packet

When a RADIUS Accounting Stop packet arrives:

1. Session located.
2. Session end timestamp recorded.
3. Stop event published.
4. Session remains temporarily in memory awaiting final statistics.
5. Filtering applications continue sending packet counters.

Routing Key:

```text
session.stop
```

---

## Final Session Export

After all statistics are aggregated:

1. Session packet count finalized.
2. Final session exported.
3. Session removed from memory.

Routing Key:

```text
session.final
```

---

# RabbitMQ Topology

Exchange:

```text
radius_exchange
```

Type:

```text
topic
```

---

## Published Events

### Session Start

Routing Key:

```text
session.start
```

Payload:

```json
{
  "event_timestamp": 1722423063,
  "is_whitelist": true,
  "account_session_id": "abc123",
  "multi_session_id": "xyz456",
  "calling_station_id": "971501234567",
  "framed_ipv4": "10.250.41.153",
  "public_ipv4": "5.38.72.0",
  "framed_ipv6": "2001:db8::/64",
  "port_start": 1,
  "port_end": 6666
}
```

---

### Session Stop

Routing Key:

```text
session.stop
```

Payload:

```json
{
  "account_session_id": "abc123"
}
```

---

### Final Session

Routing Key:

```text
session.final
```

Payload:

```json
{
  "account_session_id": "abc123",
  "packet_count": 123456,
  "session_start": "2026-06-04T12:00:00Z",
  "session_end": "2026-06-04T14:00:00Z"
}
```

---

## Consumed Events

### Statistics Updates

Routing Key:

```text
session.stats
```

Payload:

```json
{
  "account_session_id": "abc123",
  "packet_count": 1500,
  "bye_seen": true
}
```

Used to update session counters maintained by filtering applications.

---

### CGNAT Bootstrap

Routing Key:

```text
cgnat.load
```

Payload:

```json
[
  {
    "InsideIP":"10.250.41.153",
    "NatIP":"5.38.72.0",
    "StartPort":1,
    "EndPort":6666
  }
]
```

Used to populate the CGNAT lookup table.

---

### Whitelist Bootstrap

Routing Key:

```text
whitelist.load
```

Payload:

```json
[
  {
    "MSISDN":"971501234567",
    "Status":true
  }
]
```

Used to populate the whitelist lookup table.

---

# Configuration

Example:

```ini
interface=ens160

threads=2

ring_buffer_size=4096
caplen=2048

extract_all=false
update_timeout=86400

rabbitmq_host=127.0.0.1
rabbitmq_port=5672
rabbitmq_user=radius_user
rabbitmq_password=radius_pass
rabbitmq_vhost=radius
rabbitmq_exchange=radius_exchange

verbosity=2
```

---

# Session Structure

```go
type UserSession struct {
    EventTimestamp uint32

    PacketCount uint32
    DestroyTime uint32

    AccountStatusType uint8
    IsWhitelist bool

    AccountSessionID string
    MultiSessionID string
    CallingStationID string

    FramedIPv4 string
    PublicIPv4 string
    FramedIPv6 string

    PortStart uint16
    PortEnd uint16

    SessionStart time.Time
    SessionEnd time.Time

    ExtraAVPs []ExtraAVP
}
```

---

# Building

Requirements:

* Go 1.24+
* RabbitMQ 3.13+
* Linux

Build:

```bash
go mod tidy

go build -o radius-parser ./cmd/radius-parser
```

---

# Running

Live Capture

```bash
./radius-parser \
  --interface ens160 \
  --config radius.conf
```

PCAP Replay

```bash
./radius-parser \
  --file sample.pcap \
  --config radius.conf
```

---

# Monitoring

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

---

# Performance Goals

* Multi-core packet processing
* Lock-efficient session lookups
* O(1) whitelist lookups
* O(1) CGNAT lookups
* RabbitMQ topic-based fanout
* Suitable for ISP-scale RADIUS accounting environments

---

# License

Internal/Private Project.
