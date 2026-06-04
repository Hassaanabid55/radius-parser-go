package stats

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"radius-parser/internal/session"
)

/* =========================================================
 * GLOBAL STATS (C equivalent)
 * ========================================================= */

var (
	gSessionCount         uint64
	gSessionInserts       uint64
	gSessionDeletes       uint64
	gSessionUpdates       uint64
	cgnatTableSize        uint64
	whitelistTableSize    uint64
	gSessionTotalRestores uint64
	gSessionTotalStarts   uint64
	gSessionTotalUpdates  uint64
	gSessionTotalDeletes  uint64
)

/* =========================================================
 * ATOMIC HELPERS (safe for workers)
 * ========================================================= */

func IncSessionCount() {
	atomic.AddUint64(&gSessionCount, 1)
}

func DecSessionCount() {
	atomic.AddUint64(&gSessionCount, ^uint64(0))
}

func GetSessionCount() uint64 {
	return atomic.LoadUint64(&gSessionCount)
}

func IncCGNATEntries() {
	atomic.AddUint64(&cgnatTableSize, 1)
}

func DecCGNATEntries() {
	atomic.AddUint64(&cgnatTableSize, ^uint64(0))
}

func IncWhitelistEntries() {
	atomic.AddUint64(&whitelistTableSize, 1)
}

func DecWhitelistEntries() {
	atomic.AddUint64(&whitelistTableSize, ^uint64(0))
}

func IncInserts() {
	atomic.AddUint64(&gSessionInserts, 1)
}

func IncDeletes() {
	atomic.AddUint64(&gSessionDeletes, 1)
}

func IncUpdates() {
	atomic.AddUint64(&gSessionUpdates, 1)
}

func IncStarts() {
	atomic.AddUint64(&gSessionTotalStarts, 1)
}

func IncTotalUpdates() {
	atomic.AddUint64(&gSessionTotalUpdates, 1)
}

func IncTotalDeletes() {
	atomic.AddUint64(&gSessionTotalDeletes, 1)
}

func IncRestores() {
	atomic.AddUint64(&gSessionTotalRestores, 1)
}

func sessionPrintStats() {
	log.Println("================ SESSION MAP STATS ================")

	log.Printf("Active sessions          : %d", atomic.LoadUint64(&gSessionCount))
	log.Printf("Active cgnat entries     : %d", atomic.LoadUint64(&cgnatTableSize))
	log.Printf("Active whitelist entries : %d", atomic.LoadUint64(&whitelistTableSize))

	log.Printf("Inserts                  : %d", atomic.LoadUint64(&gSessionInserts))
	log.Printf("Deletes                  : %d", atomic.LoadUint64(&gSessionDeletes))
	log.Printf("Updates                  : %d", atomic.LoadUint64(&gSessionUpdates))

	log.Printf("Total Starts             : %d", atomic.LoadUint64(&gSessionTotalStarts))
	log.Printf("Total Updates            : %d", atomic.LoadUint64(&gSessionTotalUpdates))
	log.Printf("Total Deletes            : %d", atomic.LoadUint64(&gSessionTotalDeletes))
	log.Printf("Total Restores           : %d", atomic.LoadUint64(&gSessionTotalRestores))

	count := atomic.LoadUint64(&gSessionCount)
	if count > 0 {
		approxMem := count * uint64(unsafeSizeOfSessionNode())
		log.Printf("Approx memory            : %d KB", approxMem/1024)
	}

	log.Println("===================================================")
}

func unsafeSizeOfSessionNode() uintptr {
	return unsafe.Sizeof(session.SessionNode{})
}

var statsStopCh = make(chan struct{})
var statsOnce sync.Once

func StartSessionStatsThread() {
	statsOnce.Do(func() {
		go sessionStatsLoop()
	})
}

func StopSessionStatsThread() {
	close(statsStopCh)
}

func sessionStatsLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-statsStopCh:
			return

		case <-ticker.C:
			sessionPrintStats()
		}
	}
}
