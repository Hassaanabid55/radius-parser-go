package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"radius-parser/internal/capture"
	"radius-parser/internal/cgnat"
	"radius-parser/internal/config"
	"radius-parser/internal/parser"
	"radius-parser/internal/whitelist"
	"radius-parser/internal/workers"
)

func printConfig(cfg *config.Config) {
	fmt.Println("\n================ CONFIG DUMP ================")

	fmt.Println("GENERAL")
	fmt.Println("  CGNATFilePath    :", cfg.CGNATFilePath)
	fmt.Println("  WhitelistFilePath:", cfg.WhitelistFilePath)
	fmt.Println("  InterfaceName    :", cfg.InterfaceName)
	fmt.Println("  Threads          :", cfg.Threads)
	fmt.Println("  ExtractAll       :", cfg.ExtractAll)
	fmt.Println("  Verbosity        :", cfg.Verbosity)
	fmt.Println("  CapLen           :", cfg.CapLen)
	fmt.Println("  UpdateTimeout    :", cfg.UpdateTimeout)
	fmt.Println("  RingBufferSize   :", cfg.RingBufferSize)

	fmt.Println("\nINPUT")
	fmt.Println("  InputFile        :", cfg.InputFile)

	fmt.Println("\nMYSQL")
	fmt.Println("  Host             :", cfg.MySQLHost)
	fmt.Println("  Database         :", cfg.MySQLDatabase)
	fmt.Println("  User             :", cfg.MySQLUser)
	fmt.Println("  Password         :", cfg.MySQLPassword)
	fmt.Println("  Port             :", cfg.MySQLPort)

	fmt.Println("\nRABBITMQ")
	fmt.Println("  Host             :", cfg.RabbitMQHost)
	fmt.Println("  Port             :", cfg.RabbitMQPort)
	fmt.Println("  User             :", cfg.RabbitMQUser)
	fmt.Println("  Password         :", cfg.RabbitMQPassword)
	fmt.Println("  VHost            :", cfg.RabbitMQVHost)
	fmt.Println("  Exchange         :", cfg.RabbitMQExchange)

}

func atoi(s string) int {
	var v int
	fmt.Sscanf(s, "%d", &v)
	return v
}

func overrideCLI(cfg *config.Config) {

	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {

		case "-v", "--verbose":
			if i+1 < len(args) {
				cfg.Verbosity = atoi(args[i+1])
				i++
			}

		case "-i", "--interface":
			if i+1 < len(args) {
				cfg.InterfaceName = args[i+1]
				i++
			}

		case "-t", "--threads":
			if i+1 < len(args) {
				cfg.Threads = config.ParseThreads(args[i+1])
				i++
			}

		case "--input-file":
			if i+1 < len(args) {
				cfg.InputFile = args[i+1]
				i++
			}

		case "--caplen":
			if i+1 < len(args) {
				cfg.CapLen = atoi(args[i+1])
				i++
			}

		case "--update-timeout":
			if i+1 < len(args) {
				cfg.UpdateTimeout = atoi(args[i+1])
				i++
			}

		case "--ring-buffer-size":
			if i+1 < len(args) {
				cfg.RingBufferSize = atoi(args[i+1])
				i++
			}
		}
	}
}

type Runtime struct {
	Config *config.Config
}

func main() {

	// =========================
	// STEP 1: FIRST PASS CLI (ONLY config path)
	// =========================
	configPath := flag.String("c", "", "config file path")
	flag.Parse()

	if *configPath == "" {
		log.Fatal("config file path is required (-c)")
	}

	// =========================
	// STEP 2: LOAD CONFIG FILE
	// =========================
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// =========================
	// STEP 3: SECOND PASS CLI OVERRIDES
	// (rebuild flag set manually for full control like C)
	// =========================
	overrideCLI(cfg)

	// =========================
	// DEBUG OUTPUT (optional)
	// =========================
	if cfg.Verbosity > 0 {
		fmt.Println("=== CONFIG LOADED ===")
		fmt.Printf("Interface: %s\n", cfg.InterfaceName)
		fmt.Printf("Threads: %v\n", cfg.Threads)
		fmt.Printf("Verbosity: %d\n", cfg.Verbosity)
		fmt.Printf("InputFile: %s\n", cfg.InputFile)
	}

	// =========================
	// RUNTIME OBJECT
	// =========================
	rt := &Runtime{Config: cfg}

	printConfig(cfg)
	start(rt)
}

func start(rt *Runtime) {

	cfg := rt.Config

	// =========================
	// 1. INIT CAPTURE FIRST (QUEUE OWNER)
	// =========================
	capture.InitCapture(
		cfg.RingBufferSize,
		cfg.CapLen,
		cfg.Verbosity,
	)

	parser.InitParser(
		cfg.ExtractAll,    // extract all
		cfg.UpdateTimeout, // timeout
		cfg.Verbosity,     // verbosity
	)

	// =========================
	// 2. START WORKERS (CONSUMERS)
	// =========================
	workers.StartWorkers(workers.WorkerConfig{
		CoreIDs: cfg.Threads,
		Verbose: cfg.Verbosity,
	})

	// =========================
	// LOAD DATA (BEFORE WORKERS)
	// =========================
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

	// =========================
	// 3. START CAPTURE (PRODUCER)
	// =========================
	if cfg.InputFile != "" {
		capture.StartFileCapture(cfg.InputFile)
	} else {
		capture.StartInterfaceCapture(cfg.InterfaceName)
	}
}
