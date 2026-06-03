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

// WhitelistInfo mirrors the C WhitelistInfo struct.
type WhitelistInfo struct {
	MSISDN string
	Status bool
}

// =========================
// WHITELIST MAP
// =========================

// gWlMap is a sync.Map keyed by msisdn (string) → WhitelistInfo.
// Replaces uthash g_wl_map + g_wl_mutex; reads are lock-free.
var gWlMap sync.Map

// =========================
// LOAD FROM FILE
// =========================

// WLLoadFromFile reads a whitelist file (msisdn,status per line)
// and populates gWlMap.
func WLLoadFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		sysErr("Failed to open whitelist file: " + path)
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}
		msisdn := strings.TrimSpace(parts[0])
		statusRaw := strings.TrimSpace(parts[1])
		if msisdn == "" {
			continue
		}
		status, err := strconv.Atoi(statusRaw)
		if err != nil {
			continue
		}

		info := WhitelistInfo{
			MSISDN: msisdn,
			Status: status != 0,
		}
		gWlMap.Store(msisdn, info)
		atomic.AddUint64(&wlTableSize, 1)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("whitelist scan error: %w", err)
	}

	sysInfo("Whitelist loaded into memory")
	return nil
}

// =========================
// FAST LOOKUP  (hot path)
// =========================

// wlLookup looks up an MSISDN and fills out if found.
// Returns true on hit, false on miss.
func wlLookup(msisdn string, out *WhitelistInfo) bool {
	if msisdn == "" {
		return false
	}
	v, ok := gWlMap.Load(msisdn)
	if !ok {
		return false
	}
	if out != nil {
		*out = v.(WhitelistInfo)
	}
	return true
}