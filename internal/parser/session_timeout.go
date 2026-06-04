package parser

import (
	"log"
	"sync"
	"time"

	"radius-parser/internal/session"
	"radius-parser/internal/stats"
)

var (
	timeoutStopCh = make(chan struct{})
	timeoutOnce   sync.Once
	shard         uint32
)

func StartSessionTimeoutWorker() {
	timeoutOnce.Do(func() {
		go sessionTimeoutLoop()
	})
}

func StopSessionTimeoutWorker() {
	close(timeoutStopCh)
}

func sessionTimeoutLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutStopCh:
			return

		case <-ticker.C:
			now := uint32(time.Now().Unix())

			session.Mu.Lock()

			idx := 0

			for id, node := range session.Map {

				// shard distribution (matches C: idx++ % 10)
				if (idx % 10) != int(shard) {
					idx++
					continue
				}
				idx++

				if node.Entry.DestroyTime == 0 {
					continue
				}

				if node.Entry.DestroyTime > now {
					continue
				}

				if OptVerbosity.Load() > 2 {
					log.Printf("Session expired: %s", id)
				}

				// mimic C behavior
				session.End(&node.Entry)

				// IMPORTANT: delete safely under lock
				delete(session.Map, id)

				// update stats (if you want C parity)
				if stats.GetSessionCount() > 0 {
					stats.DecSessionCount()
				}
				stats.IncTotalDeletes()
			}

			shard = (shard + 1) % 10

			session.Mu.Unlock()
		}
	}
}
