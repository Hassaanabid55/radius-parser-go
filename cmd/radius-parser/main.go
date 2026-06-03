package main

import (
	"bufio"
	"flag"
	"log/syslog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

// =========================
// CONFIGURATION OPTIONS
// =========================

type Config struct {
	ConfigFile      string
	CGNATFilePath   string
	WhitelistFile   string
	InterfaceName   string
	InputFiles      string
	ThreadsStr      string
	Verbosity       uint8
	Caplen          uint16
	ByeTimeout      uint16
	RabbitMQPort    uint16
	UpdateTimeout   uint32
	RingBufferSize  uint32
	ExtractAll      bool

	// RabbitMQ
	RabbitMQHost     string
	RabbitMQVHost    string
	RabbitMQUser     string
	RabbitMQPassword string
	RabbitMQExchange string
}

// =========================
// GLOBALS
// =========================

var (
	gRunning int32 = 1 // atomic
	log      *syslog.Writer
)

// =========================
// PARSERS
// =========================

func parseU8(s string) uint8 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 8)
	return uint8(v)
}

func parseU16(s string) uint16 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	return uint16(v)
}

func parseU32(s string) uint32 {
	v, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	return uint32(v)
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}

// =========================
// CONFIG LOADER
// =========================

func loadConfig(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		log.Err("Failed opening config: " + path)
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" || val == "" {
			continue
		}

		switch key {
		// General
		case "verbosity":
			cfg.Verbosity = parseU8(val)
		case "cgnat_file_path":
			cfg.CGNATFilePath = val
		case "whitelist_file_path":
			cfg.WhitelistFile = val
		case "interface_name":
			cfg.InterfaceName = val
		case "threads":
			cfg.ThreadsStr = val
		case "extract_all":
			cfg.ExtractAll = parseBool(val)

		// Capture
		case "caplen":
			cfg.Caplen = parseU16(val)
		case "update_timeout":
			cfg.UpdateTimeout = parseU32(val)
		case "bye_timeout":
			cfg.ByeTimeout = parseU16(val)
		case "ring_buffer_size":
			cfg.RingBufferSize = parseU32(val)

		// Input
		case "input_file":
			cfg.InputFiles = val

		// RabbitMQ
		case "rabbitmq_host":
			cfg.RabbitMQHost = val
		case "rabbitmq_vhost":
			cfg.RabbitMQVHost = val
		case "rabbitmq_user":
			cfg.RabbitMQUser = val
		case "rabbitmq_password":
			cfg.RabbitMQPassword = val
		case "rabbitmq_exchange":
			cfg.RabbitMQExchange = val
		case "rabbitmq_port":
			cfg.RabbitMQPort = parseU16(val)
		}
	}

	return scanner.Err()
}

// =========================
// SIGNAL HANDLER / CLEANUP
// =========================

// App holds runtime state shared across goroutines.
type App struct {
	cfg     *Config
	queue   *GlobalQueue
	pcap    PcapHandle // interface defined in capture.go
	once    sync.Once
}

func (app *App) cleanup() {
	app.once.Do(func() {
		app.queue.Shutdown()
		if app.pcap != nil && app.cfg.InputFiles == "" {
			app.pcap.BreakLoop()
		}
		atomic.StoreInt32(&gRunning, 0)
	})
}

func (app *App) handleSignals() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-ch
		if app.cfg.Verbosity > 0 {
			log.Info("Shutdown signal received: " + sig.String())
		}
		app.cleanup()
	}()
}

// =========================
// MAIN
// =========================

func main() {
	var err error
	log, err = syslog.New(syslog.LOG_USER|syslog.LOG_PID, "radius_parser")
	if err != nil {
		panic("syslog: " + err.Error())
	}
	defer log.Close()

	cfg := &Config{
		InterfaceName:  "lo",
		Caplen:         3200,
		ByeTimeout:     43200,
		UpdateTimeout:  900,
		RingBufferSize: 1048576,
	}

	// -------------------------
	// Parse flags
	// -------------------------
	fs := flag.NewFlagSet("radius_parser", flag.ExitOnError)

	fs.StringVar(&cfg.ConfigFile, "c", "", "Config file path")
	fs.StringVar(&cfg.ConfigFile, "config-file", "", "Config file path")

	verbosity := fs.Uint("v", 0, "Verbosity level")
	fs.Uint("verbose", 0, "Verbosity level")

	fs.StringVar(&cfg.InterfaceName, "i", cfg.InterfaceName, "Network interface")
	fs.StringVar(&cfg.InterfaceName, "interface", cfg.InterfaceName, "Network interface")

	fs.StringVar(&cfg.ThreadsStr, "t", "", "Thread list")
	fs.StringVar(&cfg.ThreadsStr, "threads", "", "Thread list")

	fs.StringVar(&cfg.InputFiles, "input-file", "", "Input PCAP file")

	fs.StringVar(&cfg.WhitelistFile, "whitelist-file", "", "Whitelist file")
	fs.StringVar(&cfg.CGNATFilePath, "cgnat-file", "", "CGNAT CSV file")

	extractAll := fs.Bool("extract-all", false, "Extract all flows")

	caplen := fs.Uint("caplen", uint(cfg.Caplen), "Capture length")
	updateTimeout := fs.Uint("update-timeout", uint(cfg.UpdateTimeout), "Update timeout (s)")
	byeTimeout := fs.Uint("bye-timeout", uint(cfg.ByeTimeout), "Bye timeout (s)")
	ringBuf := fs.Uint("ring-buffer-size", uint(cfg.RingBufferSize), "Ring buffer size")

	// RabbitMQ
	fs.StringVar(&cfg.RabbitMQHost, "rabbitmq-host", "", "RabbitMQ host")
	fs.StringVar(&cfg.RabbitMQVHost, "rabbitmq-vhost", "", "RabbitMQ vhost")
	fs.StringVar(&cfg.RabbitMQUser, "rabbitmq-user", "", "RabbitMQ user")
	fs.StringVar(&cfg.RabbitMQPassword, "rabbitmq-password", "", "RabbitMQ password")
	fs.StringVar(&cfg.RabbitMQExchange, "rabbitmq-exchange", "", "RabbitMQ exchange")
	rabbitmqPort := fs.Uint("rabbitmq-port", 0, "RabbitMQ port")

	_ = fs.Parse(os.Args[1:])

	// Apply numeric flags back into cfg
	cfg.Verbosity = uint8(*verbosity)
	cfg.ExtractAll = *extractAll
	cfg.Caplen = uint16(*caplen)
	cfg.UpdateTimeout = uint32(*updateTimeout)
	cfg.ByeTimeout = uint16(*byeTimeout)
	cfg.RingBufferSize = uint32(*ringBuf)
	cfg.RabbitMQPort = uint16(*rabbitmqPort)

	// -------------------------
	// Load config file first, then CLI overrides take precedence
	// (flag.Parse already applied CLI values; re-parse config below
	//  only fills keys that were not explicitly set on CLI)
	// -------------------------
	if cfg.ConfigFile != "" {
		fileCfg := *cfg // snapshot CLI values
		if err := loadConfig(cfg.ConfigFile, cfg); err != nil {
			os.Exit(1)
		}
		// Restore CLI overrides (non-zero / non-empty CLI values win)
		mergeConfig(cfg, &fileCfg)
	}

	if cfg.Verbosity > 0 {
		log.Info("Radius parser started")
	}

	// -------------------------
	// Build app
	// -------------------------
	queue := NewGlobalQueue()
	app := &App{cfg: cfg, queue: queue}
	app.handleSignals()

	// -------------------------
	// RabbitMQ init
	// -------------------------
	rmqCfg := RabbitMQConfig{
		Host:     cfg.RabbitMQHost,
		VHost:    cfg.RabbitMQVHost,
		User:     cfg.RabbitMQUser,
		Password: cfg.RabbitMQPassword,
		Exchange: cfg.RabbitMQExchange,
		Port:     cfg.RabbitMQPort,
	}
	RabbitMQInit(&rmqCfg)

	// -------------------------
	// Bootstrap state
	// -------------------------
	RabbitMQBootstrapState()

		if cfg.Verbosity > 0 {
			log.Info("Loading data from files")
		}
		if cfg.WhitelistFile == "" || cfg.CGNATFilePath == "" {
			log.Err("Whitelist/CGNAT file missing")
			os.Exit(1)
		}
		if err := WLLoadFromFile(cfg.WhitelistFile); err != nil {
			log.Err("Failed loading whitelist file")
			os.Exit(1)
		}
		if err := CGNATLoadFromCSV(cfg.CGNATFilePath); err != nil {
			log.Err("Failed loading CGNAT CSV")
			os.Exit(1)
		}
	

	// -------------------------
	// Start workers
	// -------------------------
	var wg sync.WaitGroup
	StartWorkerThreads(&wg, queue, cfg)

	// -------------------------
	// Start capture
	// -------------------------
	if cfg.InputFiles != "" {
		if cfg.Verbosity > 0 {
			log.Info("Processing input file: " + cfg.InputFiles)
		}
		StartFileCapture(cfg.InputFiles, queue, cfg)
		app.cleanup()
	} else {
		handle := StartInterfaceCapture(queue, cfg)
		app.pcap = handle
	}

	// -------------------------
	// Wait for shutdown
	// -------------------------
	for atomic.LoadInt32(&gRunning) == 1 {
		syscall.Nanosleep(&syscall.Timespec{Sec: 1}, nil)
	}

	if cfg.Verbosity > 0 {
		log.Info("Waiting for worker threads...")
	}

	wg.Wait()

	log.Info("All threads exited, performing cleanup")

	DBClose()
	RabbitMQCleanup()
	queue.Close()
}

// mergeConfig copies non-zero CLI values back over config-file values so CLI wins.
func mergeConfig(base, cli *Config) {
	if cli.Verbosity != 0 {
		base.Verbosity = cli.Verbosity
	}
	if cli.CGNATFilePath != "" {
		base.CGNATFilePath = cli.CGNATFilePath
	}
	if cli.WhitelistFile != "" {
		base.WhitelistFile = cli.WhitelistFile
	}
	if cli.InterfaceName != "" && cli.InterfaceName != "lo" {
		base.InterfaceName = cli.InterfaceName
	}
	if cli.InputFiles != "" {
		base.InputFiles = cli.InputFiles
	}
	if cli.ThreadsStr != "" {
		base.ThreadsStr = cli.ThreadsStr
	}
	if cli.ExtractAll {
		base.ExtractAll = true
	}
	if cli.Caplen != 0 && cli.Caplen != 3200 {
		base.Caplen = cli.Caplen
	}
	if cli.UpdateTimeout != 0 && cli.UpdateTimeout != 900 {
		base.UpdateTimeout = cli.UpdateTimeout
	}
	if cli.ByeTimeout != 0 && cli.ByeTimeout != 43200 {
		base.ByeTimeout = cli.ByeTimeout
	}
	if cli.RingBufferSize != 0 && cli.RingBufferSize != 1048576 {
		base.RingBufferSize = cli.RingBufferSize
	}

	if cli.RabbitMQHost != "" {
		base.RabbitMQHost = cli.RabbitMQHost
	}
	if cli.RabbitMQVHost != "" {
		base.RabbitMQVHost = cli.RabbitMQVHost
	}
	if cli.RabbitMQUser != "" {
		base.RabbitMQUser = cli.RabbitMQUser
	}
	if cli.RabbitMQPassword != "" {
		base.RabbitMQPassword = cli.RabbitMQPassword
	}
	if cli.RabbitMQExchange != "" {
		base.RabbitMQExchange = cli.RabbitMQExchange
	}
	if cli.RabbitMQPort != 0 {
		base.RabbitMQPort = cli.RabbitMQPort
	}
}