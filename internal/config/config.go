package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// General
	CGNATFilePath     string
	WhitelistFilePath string
	InterfaceName     string
	Threads           []int
	ExtractAll        bool
	Verbosity         int
	CapLen            int
	UpdateTimeout     int
	RingBufferSize    int

	InputFile string

	// RabbitMQ
	RabbitMQHost     string
	RabbitMQPort     int
	RabbitMQUser     string
	RabbitMQPassword string
	RabbitMQVHost    string
	RabbitMQExchange string

	SiteName  string
	NodeName  string
}

func ParseThreads(s string) []int {
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))

	for _, p := range parts {
		var v int
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		fmt.Sscanf(p, "%d", &v)
		if v > 0 {
			out = append(out, v)
		}
	}

	return out
}

// LoadConfig parses key=value file
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config: %w", err)
	}
	defer f.Close()

	cfg := &Config{
		InterfaceName:  "lo",
		CapLen:         3200,
		UpdateTimeout:  300,
		RingBufferSize: 1048576,
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// remove inline comments
		if idx := strings.Index(val, "#"); idx != -1 {
			val = strings.TrimSpace(val[:idx])
		}

		cfg.apply(key, val)
	}

	return cfg, scanner.Err()
}

func (c *Config) apply(key, val string) {
	switch key {

	// general
	case "cgnat_file_path":
		c.CGNATFilePath = val
	case "whitelist_file_path":
		c.WhitelistFilePath = val
	case "interface_name":
		c.InterfaceName = val
	case "extract_all":
		c.ExtractAll = parseBool(val)
	case "threads":
		c.Threads = ParseThreads(val)
	case "verbosity":
		c.Verbosity = parseInt(val)
	case "caplen":
		c.CapLen = parseInt(val)
	case "update_timeout":
		c.UpdateTimeout = parseInt(val)
	case "ring_buffer_size":
		c.RingBufferSize = parseInt(val)

	// input
	case "input_file":
		c.InputFile = val

	// rabbitmq
	case "rabbitmq_host":
		c.RabbitMQHost = val
	case "rabbitmq_port":
		c.RabbitMQPort = parseInt(val)
	case "rabbitmq_user":
		c.RabbitMQUser = val
	case "rabbitmq_password":
		c.RabbitMQPassword = val
	case "rabbitmq_vhost":
		c.RabbitMQVHost = val
	case "rabbitmq_exchange":
		c.RabbitMQExchange = val
	case "site_name":
		c.SiteName = val
	case "node_name":
		c.NodeName = val
	}
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}
