package workers

import (
	"log"
	"sync"
	"time"

	"radius-parser/internal/capture"
	"radius-parser/internal/liveness"
	"radius-parser/internal/parser"
	"radius-parser/internal/stats"
)

type WorkerConfig struct {
	CoreIDs []int
	Verbose int
}

var wg sync.WaitGroup

// StartWorkers
func StartWorkers(cfg WorkerConfig) {

	if len(cfg.CoreIDs) == 0 {
		log.Fatal("no cores provided")
	}

	for i, core := range cfg.CoreIDs {
		wg.Add(1)
		go worker(core, i, cfg.Verbose)
	}
	liveness.StartAliveNodeCounter(20 * time.Second)
	parser.StartSessionTimeoutWorker()
	if cfg.Verbose > 2 {
		stats.StartSessionStatsThread()
	}

	log.Printf("Workers started: %d", len(cfg.CoreIDs))
}

func Stop() {
	log.Println("Stopping workers...")
	parser.StopSessionTimeoutWorker()
	stats.StopSessionStatsThread()
	liveness.StopAliveNodeCounter()
	close(capture.TaskQueue)
	wg.Wait()
	log.Println("Workers stopped")
}

func worker(coreID int, idx int, verbose int) {
	if verbose > 0 {
		log.Printf("Worker started core=%d idx=%d\n", coreID, idx)
	}

	var processed uint64

	for task := range capture.TaskQueue {

		radiusData, err := parser.ExtractRadiusFromTask(task.Data)
		if err != nil {
			log.Printf("drop: %v", err)
			continue
		}

		pkt, err := parser.ParseRadiusPacket(radiusData)
		if err == nil {

			s, err := parser.BuildSession(pkt)
			if err == nil {
				if verbose > 2 {
					log.Printf("Session built: %+v\n================================================", s)
				}
			} else {
				log.Printf("Session build failed: %v\n", err)
			}
		} else {
			log.Printf("Packet parse failed: %v\n", err)
		}

		processed++
		capture.Inflight.Add(^uint64(0))
	}

	if verbose > 0 {
		log.Printf("Worker %d exiting | processed=%d\n", coreID, processed)
	}
}
