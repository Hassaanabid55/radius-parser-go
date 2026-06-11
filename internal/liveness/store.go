package liveness

import (
	"sync"
	"time"

	"radius-parser/internal/session"
)

var (
	Map = make(map[string]time.Time)
	Mu  sync.RWMutex

	stopCh = make(chan struct{})
	once   sync.Once
)

func StartAliveNodeCounter(interval time.Duration) {

	once.Do(func() {
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stopCh:
					return

				case <-ticker.C:
					updateAliveNodes()
				}
			}
		}()
	})
}

func StopAliveNodeCounter() {
	close(stopCh)
}

func updateAliveNodes() {
	Mu.RLock()
	defer Mu.RUnlock()

	count := 0
	now := time.Now()

	for _, lastSeen := range Map {
		if now.Sub(lastSeen) < 20*time.Second {
			count++
		}
	}
	session.ActiveNode = count
}
