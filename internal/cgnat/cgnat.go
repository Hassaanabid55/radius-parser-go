package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// =========================
// TYPES
// =========================

// CgnatEntry mirrors the C CgnatEntry struct.
type CgnatEntry struct {
	InsideIP  string
	NatIP     string
	StartPort uint16
	EndPort   uint16
}

// =========================
// CGNAT MAP
// =========================

// gCgnatMap is a sync.Map keyed by inside_ip (string) → CgnatEntry.
// Replaces uthash g_cgnat_map + g_cgnat_mutex; reads are lock-free.
var gCgnatMap sync.Map

// =========================
// LOAD FROM CSV
// =========================

// CGNATLoadFromCSV reads a CGNAT mapping CSV (nat_ip,inside_ip,start_port,end_port)
// and populates gCgnatMap. The first line (header) is skipped.
func CGNATLoadFromCSV(path string) error {
	f, err := os.Open(path)
	if err != nil {
		sysErr("Failed to open CGNAT CSV: " + path)
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// Skip header line.
	if !scanner.Scan() {
		return fmt.Errorf("cgnat csv empty or unreadable: %s", path)
	}

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ",", 4)
		if len(parts) != 4 {
			continue
		}
		natIP := strings.TrimSpace(parts[0])
		insideIP := strings.TrimSpace(parts[1])
		startPort, err1 := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 16)
		endPort, err2 := strconv.ParseUint(strings.TrimSpace(parts[3]), 10, 16)
		if err1 != nil || err2 != nil {
			continue
		}

		entry := CgnatEntry{
			InsideIP:  insideIP,
			NatIP:     natIP,
			StartPort: uint16(startPort),
			EndPort:   uint16(endPort),
		}
		gCgnatMap.Store(insideIP, entry)
		atomic.AddUint64(&cgnatTableSize, 1)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	sysInfo("CGNAT mappings loaded into memory")
	return nil
}

// =========================
// FAST LOOKUP  (hot path)
// =========================

// cgnatLookup looks up an inside IP and fills out if found.
// Returns true on hit, false on miss.
func cgnatLookup(insideIP string, out *CgnatEntry) bool {
	if insideIP == "" {
		return false
	}
	v, ok := gCgnatMap.Load(insideIP)
	if !ok {
		return false
	}
	if out != nil {
		*out = v.(CgnatEntry)
	}
	return true
}