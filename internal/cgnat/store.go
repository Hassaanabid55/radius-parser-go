package cgnat

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

type CgnatEntry struct {
	InsideIP  string // Private IP
	NatIP     string // Public IP
	StartPort uint16
	EndPort   uint16
	delete    bool
}

var (
	cgnatMap   = make(map[string]CgnatEntry)
	cgnatMutex sync.RWMutex
)

func LoadCGNATFromCSV(path string) error {

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open CGNAT CSV: %w", err)
	}
	defer f.Close()

	tmp := make(map[string]CgnatEntry)

	scanner := bufio.NewScanner(f)

	if scanner.Scan() {
	}

	for scanner.Scan() {

		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) != 4 {
			log.Printf("Invalid CGNAT line: %s", line)
			continue
		}

		natIP := strings.TrimSpace(parts[0])
		insideIP := strings.TrimSpace(parts[1])

		startPort64, err1 := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 16)
		endPort64, err2 := strconv.ParseUint(strings.TrimSpace(parts[3]), 10, 16)

		if err1 != nil || err2 != nil {
			log.Printf("Invalid CGNAT ports: %s", line)
			continue
		}

		tmp[insideIP] = CgnatEntry{
			InsideIP:  insideIP,
			NatIP:     natIP,
			StartPort: uint16(startPort64),
			EndPort:   uint16(endPort64),
		}
		// For debugging
		stats.IncCGNATEntries()
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	cgnatMutex.Lock()
	cgnatMap = tmp
	cgnatMutex.Unlock()

	return nil
}

func Lookup(ip string) (CgnatEntry, bool) {

	cgnatMutex.RLock()
	defer cgnatMutex.RUnlock()

	v, ok := cgnatMap[ip]
	return v, ok
}

func DeleteNode(node *CgnatEntry) {
	delete(cgnatMap, node.InsideIP)
}

func LoadFromBytes(entries []CgnatEntry) {

	cgnatMutex.Lock()
	defer cgnatMutex.Unlock()

	for _, e := range entries {
		old, exists := cgnatMap[e.InsideIP]
		if exists && e.delete {
			DeleteNode(&e)
			continue
		}

		// insert OR update detection
		if !exists || old != e {
			cgnatMap[e.InsideIP] = e
			stats.IncCGNATEntries()
		}
	}
}
