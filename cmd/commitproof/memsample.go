package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/cluster"
	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/host"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

func msDuration(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

// memSampleCmd samples `docker stats --no-stream` for the project's
// containers every P24 interval (K1.5) until --stop-file exists, writing one
// JSON line per sample with the raw MemUsage strings (the gate parses them
// itself). Samples start on the tick even if an earlier one is still
// running, so the 1 s cadence holds although one `docker stats` call can
// take longer than 1 s.
func memSampleCmd(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("__memsample", flag.ContinueOnError)
	out := fs.String("out", "", "output JSONL")
	stop := fs.String("stop-file", "", "stop when this file exists")
	maxMS := fs.Int64("max-ms", int64(6*params.HealthyDeadline/time.Millisecond), "safety limit")
	if err := fs.Parse(args); err != nil || *out == "" || *stop == "" {
		return usageErr("__memsample", "--out and --stop-file are required")
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return summary(map[string]any{"command": "memsample", "error": err.Error()}, 3)
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	write := func(m map[string]any) {
		b, err := canonjson.Marshal(m)
		if err != nil {
			return
		}
		mu.Lock()
		f.Write(append(b, '\n'))
		mu.Unlock()
	}
	t := time.NewTicker(params.ObservationInterval)
	defer t.Stop()
	end := time.Now().Add(msDuration(*maxMS))
	seq := 0
	for {
		if _, err := os.Stat(*stop); err == nil || time.Now().After(end) || ctx.Err() != nil {
			break
		}
		seq++
		wg.Add(1)
		go func(seq int, tick int64) {
			defer wg.Done()
			write(sampleOnce(seq, tick))
		}(seq, cmdlog.MonoNS())
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
	wg.Wait()
	f.Close()
	return summary(map[string]any{"command": "memsample", "samples": seq, "out": *out}, 0)
}

func sampleOnce(seq int, tick int64) map[string]any {
	m := map[string]any{"seq": seq, "tick_mono_ns": tick, "wall": time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}
	ctx, cancel := context.WithTimeout(context.Background(), host.QuickTimeout)
	defer cancel()
	ids, err := exec.CommandContext(ctx, "docker", "ps", "-q", "--filter", "label="+cluster.ProjectLabel).Output()
	if err != nil {
		m["error"] = "docker ps: " + err.Error()
		return m
	}
	idList := strings.Fields(string(ids))
	m["start_mono_ns"] = cmdlog.MonoNS()
	containers := []any{}
	if len(idList) > 0 {
		argv := append([]string{"stats", "--no-stream", "--format", "{{json .}}"}, idList...)
		outB, err := exec.CommandContext(ctx, "docker", argv...).Output()
		if err != nil {
			m["error"] = "docker stats: " + err.Error()
			m["end_mono_ns"] = cmdlog.MonoNS()
			return m
		}
		for _, l := range strings.Split(strings.TrimSpace(string(outB)), "\n") {
			var row map[string]any
			if json.Unmarshal([]byte(l), &row) != nil {
				continue
			}
			containers = append(containers, map[string]any{"name": row["Name"], "mem_usage": row["MemUsage"], "mem_perc": row["MemPerc"]})
		}
	}
	m["end_mono_ns"] = cmdlog.MonoNS()
	m["argv"] = "docker stats --no-stream --format {{json .}} <project containers>"
	m["containers"] = containers
	return m
}
