package workers

import (
	"log"
	"runtime"

	"radius-parser/internal/capture"
	"radius-parser/internal/parser"
	"radius-parser/internal/stats"
)

type WorkerConfig struct {
	CoreIDs []int
	Verbose int
}

// StartWorkers = equivalent of start_worker_threads()
func StartWorkers(cfg WorkerConfig) {

	if len(cfg.CoreIDs) == 0 {
		log.Fatal("no cores provided")
	}

	for i, core := range cfg.CoreIDs {
		go worker(core, i, cfg.Verbose)
	}
	parser.StartSessionTimeoutWorker()
	if cfg.Verbose > 2 {
		stats.StartSessionStatsThread()
	}

	log.Printf("Workers started: %d\n", len(cfg.CoreIDs))
}

func worker(coreID int, idx int, verbose int) {

	runtime.LockOSThread()

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
				log.Printf("Session built: %+v\n================================================", s)
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
