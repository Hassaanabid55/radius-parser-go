package whitelist

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"radius-parser/internal/stats"
)

type WhitelistInfo struct {
	MSISDN string
	Status bool
}

var (
	whitelistMap   = make(map[string]WhitelistInfo)
	whitelistMutex sync.RWMutex
)

func LoadWhitelistFromFile(path string) error {

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open whitelist file: %w", err)
	}
	
	defer f.Close()
	tmp := make(map[string]WhitelistInfo)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) != 2 {
			log.Printf("Invalid whitelist line: %s", line)
			continue
		}

		msisdn := strings.TrimSpace(parts[0])

		statusInt, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			log.Printf("Invalid status: %s", line)
			continue
		}

		tmp[msisdn] = WhitelistInfo{
			MSISDN: msisdn,
			Status: statusInt != 0,
		}

		stats.IncWhitelistEntries()
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	whitelistMutex.Lock()
	whitelistMap = tmp
	whitelistMutex.Unlock()

	return nil
}

func Lookup(msisdn string) (WhitelistInfo, bool) {

	whitelistMutex.RLock()
	defer whitelistMutex.RUnlock()

	v, ok := whitelistMap[msisdn]
	return v, ok
}

func LoadFromBytes(entries []WhitelistInfo) {

	whitelistMutex.Lock()
	defer whitelistMutex.Unlock()

	for _, e := range entries {

		old, exists := whitelistMap[e.MSISDN]

		// insert OR update detection
		if !exists || old != e {
			whitelistMap[e.MSISDN] = e
			stats.IncWhitelistEntries()
		}
	}
}