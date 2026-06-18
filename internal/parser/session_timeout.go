package parser

import (
	"log"
	"sync"
	"time"

	"radius-parser/internal/rabbitmq"
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

			var toStop []*session.UserSession

			// PHASE 1: LOCK ONLY FOR READ/MUTATION
			session.Mu.Lock()

			idx := 0

			for _, node := range session.Map {

				if (idx % 10) != int(shard) {
					idx++
					continue
				}
				idx++

				if node.DestroyTime == 0 || node.DestroyTime > now {
					continue
				}

				// already handled STOP
				if node.StopSent {
					rabbitmq.PublishSessionFinal(node)
					session.DeleteNode(node)
					continue
				}

				// mark stop
				node.StopSent = true
				node.DestroyTime = uint32(time.Now().Unix()) + OptUpdateTimeout.Load()
				session.End(node)
				toStop = append(toStop, node)
			}
			shard = (shard + 1) % 10
			session.Mu.Unlock()

			// PHASE 2: OUTSIDE LOCK (IMPORTANT)
			for _, node := range toStop {

				if OptVerbosity.Load() > 2 {
					log.Printf("Session expired: %s", node.AccountSessionID)
				}

				rabbitmq.PublishSessionStop(node)
				if OptVerbosity.Load() > 2 {
					if stats.GetSessionCount() > 0 {
						stats.DecSessionCount()
					}
					stats.IncDeletes()
				}
			}
		}
	}
}
