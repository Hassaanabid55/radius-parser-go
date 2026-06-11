package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"radius-parser/internal/capture"
	"radius-parser/internal/cgnat"
	"radius-parser/internal/config"
	"radius-parser/internal/parser"
	"radius-parser/internal/rabbitmq"
	"radius-parser/internal/whitelist"
	"radius-parser/internal/workers"
)

type Runtime struct {
	Config *config.Config
}

func main() {

	// STEP 1: PASS CLI (ONLY config path)
	configPath := flag.String("c", "", "config file path")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("config file path is required (-c)")
	}

	// STEP 2: LOAD CONFIG FILE
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	go waitForShutdown()

	// RUNTIME OBJECT
	rt := &Runtime{Config: cfg}
	start(rt)
}

func waitForShutdown() {

	sigCh := make(chan os.Signal, 1)

	signal.Notify(
		sigCh,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGSEGV,
	)

	sig := <-sigCh

	log.Printf("Received signal: %v", sig)

	shutdown()

	os.Exit(0)
}

func shutdown() {

	log.Println("Stopping Radius Parser")

	capture.Stop()
	workers.Stop()
	rabbitmq.Close()

	log.Println("Shutdown complete")
}

func start(rt *Runtime) {

	cfg := rt.Config

	// 1. INIT CAPTURE
	capture.InitCapture(cfg.RingBufferSize, cfg.CapLen, cfg.Verbosity)
	parser.InitParser(cfg.ExtractAll, cfg.UpdateTimeout, cfg.Verbosity)

	// 2. INIT RABBITMQ
	err := rabbitmq.Init(
		rabbitmq.Config{
			Host:     cfg.RabbitMQHost,
			Port:     cfg.RabbitMQPort,
			User:     cfg.RabbitMQUser,
			Password: cfg.RabbitMQPassword,
			Vhost:    cfg.RabbitMQVHost,
			Exchange: cfg.RabbitMQExchange,
		},
	)
	if err != nil {
		log.Fatalf("rabbitmq init failed: %v", err)
	}

	if err := rabbitmq.StartConsumers(); err != nil {
		log.Fatalf("rabbitmq consumers failed: %v", err)
	}

	// 3. LOAD LOCAL FALLBACK DATA
	if cfg.CGNATFilePath != "" {
		if err := cgnat.LoadCGNATFromCSV(cfg.CGNATFilePath); err != nil {
			log.Fatalf("CGNAT load failed: %v", err)
		}
	}

	if cfg.WhitelistFilePath != "" {
		if err := whitelist.LoadWhitelistFromFile(cfg.WhitelistFilePath); err != nil {
			log.Fatalf("Whitelist load failed: %v", err)
		}
	}

	// 4. START WORKERS
	workers.StartWorkers(workers.WorkerConfig{
		CoreIDs: cfg.Threads,
		Verbose: cfg.Verbosity,
	})

	// 5. START CAPTURE
	if cfg.InputFile != "" {
		capture.StartFileCapture(cfg.InputFile)
	} else {
		capture.StartInterfaceCapture(cfg.InterfaceName)
	}
}
