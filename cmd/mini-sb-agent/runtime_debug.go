package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type runtimeDebugSample struct {
	pid            int
	utimeTicks     uint64
	stimeTicks     uint64
	startTimeTicks uint64
	threads        int
	vmRSSKB        uint64
	vmHWMKB        uint64
	vmDataKB       uint64
	cgroupMemory   uint64
	cgroupOOM      uint64
	cgroupOOMKill  uint64
	cgroupPids     uint64
	cgroupPidsMax  string
	memAvailableKB uint64
	goroutines     int
	heapAlloc      uint64
	heapSys        uint64
	heapIdle       uint64
	heapReleased   uint64
	stackInuse     uint64
	otherSys       uint64
	numGC          uint32
	pauseTotalNs   uint64
	lastGCPauseNs  uint64
}

func startRuntimeDebugLogger(ctx context.Context, path string, every time.Duration) error {
	if path == "" {
		return nil
	}
	if every <= 0 {
		every = time.Second
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if st, err := f.Stat(); err == nil && st.Size() == 0 {
		fmt.Fprintln(f, "ts,pid,utime_ticks,stime_ticks,threads,vmrss_kb,vmhwm_kb,vmdata_kb,cgroup_memory,cgroup_oom,cgroup_oom_kill,cgroup_pids,cgroup_pids_max,mem_available_kb,goroutines,heap_alloc,heap_sys,heap_idle,heap_released,stack_inuse,other_sys,num_gc,pause_total_ns,last_gc_pause_ns")
	}
	go func() {
		defer f.Close()
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			s := collectRuntimeDebugSample()
			fmt.Fprintf(f, "%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%s,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
				time.Now().UnixNano(), s.pid, s.utimeTicks, s.stimeTicks, s.threads, s.vmRSSKB, s.vmHWMKB, s.vmDataKB,
				s.cgroupMemory, s.cgroupOOM, s.cgroupOOMKill, s.cgroupPids, csvSafe(s.cgroupPidsMax), s.memAvailableKB,
				s.goroutines, s.heapAlloc, s.heapSys, s.heapIdle, s.heapReleased, s.stackInuse, s.otherSys, s.numGC,
				s.pauseTotalNs, s.lastGCPauseNs)
			_ = f.Sync()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func collectRuntimeDebugSample() runtimeDebugSample {
	s := runtimeDebugSample{pid: os.Getpid(), cgroupPidsMax: readTrim("/sys/fs/cgroup/pids.max")}
	fillProcStatus(&s)
	fillProcStat(&s)
	s.cgroupMemory = readUintFirst("/sys/fs/cgroup/memory.current", "/sys/fs/cgroup/memory/memory.usage_in_bytes")
	fillCgroupMemoryEvents(&s)
	s.cgroupPids = readUintFirst("/sys/fs/cgroup/pids.current")
	s.memAvailableKB = readMemAvailableKB()
	s.goroutines = runtime.NumGoroutine()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s.heapAlloc = ms.HeapAlloc
	s.heapSys = ms.HeapSys
	s.heapIdle = ms.HeapIdle
	s.heapReleased = ms.HeapReleased
	s.stackInuse = ms.StackInuse
	s.otherSys = ms.OtherSys
	s.numGC = ms.NumGC
	s.pauseTotalNs = ms.PauseTotalNs
	if ms.NumGC > 0 {
		s.lastGCPauseNs = ms.PauseNs[(ms.NumGC+255)%256]
	}
	return s
}

func fillProcStatus(s *runtimeDebugSample) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "Threads:":
			s.threads = atoi(fields[1])
		case "VmRSS:":
			s.vmRSSKB = atou(fields[1])
		case "VmHWM:":
			s.vmHWMKB = atou(fields[1])
		case "VmData:":
			s.vmDataKB = atou(fields[1])
		}
	}
}

func fillProcStat(s *runtimeDebugSample) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return
	}
	text := string(data)
	idx := strings.LastIndex(text, ")")
	if idx < 0 || idx+2 >= len(text) {
		return
	}
	fields := strings.Fields(text[idx+2:])
	if len(fields) < 20 {
		return
	}
	// fields[0] is original field 3 (state), so original field 14 is index 11.
	s.utimeTicks = atou(fields[11])
	s.stimeTicks = atou(fields[12])
	s.startTimeTicks = atou(fields[19])
}

func fillCgroupMemoryEvents(s *runtimeDebugSample) {
	data, err := os.ReadFile("/sys/fs/cgroup/memory.events")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "oom":
			s.cgroupOOM = atou(fields[1])
		case "oom_kill":
			s.cgroupOOMKill = atou(fields[1])
		}
	}
}

func readMemAvailableKB() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemAvailable:" {
			return atou(fields[1])
		}
	}
	return 0
}

func readUintFirst(paths ...string) uint64 {
	for _, path := range paths {
		text := readTrim(path)
		if text == "" || text == "max" {
			continue
		}
		if v, err := strconv.ParseUint(strings.Fields(text)[0], 10, 64); err == nil {
			return v
		}
	}
	return 0
}

func readTrim(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func atou(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func csvSafe(s string) string {
	return strings.ReplaceAll(s, ",", "_")
}
