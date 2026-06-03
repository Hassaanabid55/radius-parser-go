package main

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// =========================
// CONSTANTS
// =========================

const maxQueueSize = 65536 // power-of-two for fast modulo via mask
const maxQueueMask = maxQueueSize - 1

// =========================
// GLOBAL QUEUE + THREAD HANDLES
// =========================

var globalQueue = NewGlobalQueue()

// =========================
// TASK QUEUE
// =========================

// GlobalQueue is a bounded MPMC ring-buffer backed by a channel.
// Using a buffered channel is idiomatic Go and avoids manual mutex/cond
// management while preserving the same backpressure semantics.
type GlobalQueue struct {
	ch       chan Task
	shutdown chan struct{}
	once     sync.Once
}

func NewGlobalQueue() *GlobalQueue {
	return &GlobalQueue{
		ch:       make(chan Task, maxQueueSize),
		shutdown: make(chan struct{}),
	}
}

// Push enqueues a task, blocking until space is available or shutdown.
// Returns false if the queue is shutting down.
func (q *GlobalQueue) Push(task Task) bool {
	select {
	case <-q.shutdown:
		return false
	case q.ch <- task:
		return true
	}
}

// Pop dequeues a task, blocking until one is available or shutdown.
// Returns false when the queue is shut down and drained.
func (q *GlobalQueue) Pop() (Task, bool) {
	select {
	case t, ok := <-q.ch:
		return t, ok
	case <-q.shutdown:
		// Drain any remaining tasks before reporting empty.
		select {
		case t := <-q.ch:
			return t, true
		default:
			return Task{}, false
		}
	}
}

// Shutdown signals all goroutines to stop and drains pending tasks.
func (q *GlobalQueue) Shutdown() {
	q.once.Do(func() {
		close(q.shutdown)
	})
}

// Close drains and frees any tasks still in the queue.
func (q *GlobalQueue) Close() {
	q.Shutdown()
	for {
		select {
		case t := <-q.ch:
			_ = t // Data is a []byte; GC handles it.
		default:
			return
		}
	}
}

// =========================
// SUBMIT TASK
// =========================

func submitTask(task *Task) {
	atomic.AddInt64(&gInflight, 1)
	if !globalQueue.Push(*task) {
		atomic.AddInt64(&gInflight, -1)
		task.Data = nil
	}
}

// =========================
// CORE ID PARSER
// =========================

// parseCoreIDs parses a comma-separated list of core IDs or ranges
// (e.g. "0,2,4-7") and returns a deduplicated slice.
func parseCoreIDs(coreList string) ([]int, error) {
	if strings.TrimSpace(coreList) == "" {
		return nil, nil
	}

	seen := make(map[int]struct{})
	var cores []int

	for _, token := range strings.Split(coreList, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		if dash := strings.IndexByte(token, '-'); dash >= 0 {
			startStr := strings.TrimSpace(token[:dash])
			endStr := strings.TrimSpace(token[dash+1:])
			start, err1 := strconv.Atoi(startStr)
			end, err2 := strconv.Atoi(endStr)
			if err1 != nil || err2 != nil || start < 0 || end < 0 {
				return nil, fmt.Errorf("invalid core range: %s", token)
			}
			if start > end {
				return nil, fmt.Errorf("invalid core range: %d-%d", start, end)
			}
			for i := start; i <= end; i++ {
				if _, dup := seen[i]; !dup {
					seen[i] = struct{}{}
					cores = append(cores, i)
				}
			}
		} else {
			core, err := strconv.Atoi(token)
			if err != nil || core < 0 {
				return nil, fmt.Errorf("invalid core id: %s", token)
			}
			if _, dup := seen[core]; !dup {
				seen[core] = struct{}{}
				cores = append(cores, core)
			}
		}
	}
	return cores, nil
}

// =========================
// WORKER THREAD
// =========================

func workerThread(coreID int, wg *sync.WaitGroup, cfg *Config) {
	defer wg.Done()

	// Pin goroutine to OS thread; set CPU affinity via runtime.
	// Go's scheduler will keep this goroutine on this OS thread.
	// True CPU pinning requires cgo+sched_setaffinity; this is the
	// pure-Go equivalent. Replace with a cgo call if hard pinning is needed.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if cfg.Verbosity > 0 {
		sysInfo(fmt.Sprintf("Worker thread started on core %d", coreID))
	}

	var (
		totalPackets      uint64
		totalNs           uint64
		parseFailures     uint64
		attributeFailures uint64
	)

	for atomic.LoadInt32(&gRunning) == 1 {
		task, ok := globalQueue.Pop()
		if !ok {
			break
		}

		start := time.Now()

		rp, err := parseRadiusPkt(task.Data)
		if err == nil {
			session, err2 := readRadiusAttributes(rp, cfg)
			if err2 == nil {
				if cfg.Verbosity > 1 {
					printUserSession(session, cfg)
				}
				switch session.AccountStatusType {
				case SessionStart:
					RabbitMQPublishSessionStart(session)
					RabbitMQPublishSessionSync(session)
				case SessionStop:
					RabbitMQPublishSessionStop(session)
					RabbitMQPublishSessionDelete(session.AcAccountSessionId)
				}
			} else {
				attributeFailures++
			}
		} else {
			parseFailures++
		}

		elapsed := uint64(time.Since(start).Nanoseconds())
		totalNs += elapsed
		totalPackets++
		atomic.AddInt64(&gInflight, -1)
		task.Data = nil // let GC reclaim
	}

	if cfg.Verbosity > 1 {
		totalMs := float64(totalNs) / 1e6
		avgUs := 0.0
		if totalPackets > 0 {
			avgUs = float64(totalNs) / float64(totalPackets) / 1000.0
		}
		throughput := 0.0
		if totalNs > 0 {
			throughput = float64(totalPackets) / (float64(totalNs) / 1e9)
		}
		sysInfo(fmt.Sprintf(
			"Worker %d exiting | Packets=%d | ParseFail=%d | AttrFail=%d"+
				" | Total=%.3f ms | Avg=%.3f us/pkt | Throughput=%.2f pkt/sec",
			coreID, totalPackets, parseFailures, attributeFailures,
			totalMs, avgUs, throughput,
		))
	}
}

// =========================
// STATS GOROUTINE
// =========================

func sessionStatsLoop() {
	for atomic.LoadInt32(&gRunning) == 1 {
		time.Sleep(5 * time.Second)
		sessionPrintStats()
	}
}

// =========================
// START WORKER THREADS
// =========================

// StartWorkerThreads parses core IDs, spawns workers, and launches
// background goroutines. The WaitGroup is signalled when all workers exit.
func StartWorkerThreads(wg *sync.WaitGroup, queue *GlobalQueue, cfg *Config) {
	cores, err := parseCoreIDs(cfg.ThreadsStr)
	if err != nil {
		sysErr("parse_core_ids: " + err.Error())
	}

	// Fall back to one worker per logical CPU if no list was given.
	if len(cores) == 0 {
		for i := 0; i < runtime.NumCPU(); i++ {
			cores = append(cores, i)
		}
	}

	for _, coreID := range cores {
		wg.Add(1)
		go workerThread(coreID, wg, cfg)
	}

	// Background goroutines (no WaitGroup: they exit on gRunning == 0).
	go sessionTimeoutLoop(cfg)
	go RabbitMQStatsWorker()

	if cfg.Verbosity > 1 {
		go sessionStatsLoop()
	}

	go roleElectionLoop()
}