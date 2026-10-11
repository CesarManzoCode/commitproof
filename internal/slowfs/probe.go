package slowfs

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// IOProbe is the F4a probe (P17): Count sequential write+fsync of Size bytes
// at offset 0 of a scratch file in Dir, each timed from the write call to the
// end of fsync. Durations are integer microseconds (canonical JSON).
type IOProbe struct {
	Dir       string  `json:"dir"`
	Size      int     `json:"size"`
	SamplesUS []int64 `json:"samples_us"`
	MedianUS  int64   `json:"median_us"`
}

// ProbeIO runs the probe and removes its scratch file.
func ProbeIO(dir string, size, count int) (IOProbe, error) {
	p := IOProbe{Dir: dir, Size: size}
	if count < 1 || size < 1 {
		return p, fmt.Errorf("count and size must be positive")
	}
	path := filepath.Join(dir, fmt.Sprintf(".slowfs-probe-%d", os.Getpid()))
	defer os.Remove(path)
	buf := bytes.Repeat([]byte{0x5a}, size)
	for range count {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return p, err
		}
		start := time.Now()
		_, werr := f.WriteAt(buf, 0)
		serr := f.Sync()
		d := time.Since(start)
		f.Close()
		if werr != nil || serr != nil {
			return p, fmt.Errorf("write %v, fsync %v", werr, serr)
		}
		p.SamplesUS = append(p.SamplesUS, d.Microseconds())
	}
	p.MedianUS = Median(p.SamplesUS)
	return p, nil
}

// Median returns the median (the lower middle element for even lengths).
func Median(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64{}, xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[(len(s)-1)/2]
}

// Proc is one process as read from /proc/<pid>/stat.
type Proc struct {
	PID   int    `json:"pid"`
	PPID  int    `json:"ppid"`
	State string `json:"state"`
	Comm  string `json:"comm"`
}

// ReadProc parses /proc/<pid>/stat. The comm field may contain spaces and
// parentheses, so the state is read after the last ')'.
func ReadProc(pid int) (Proc, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Proc{}, err
	}
	s := string(b)
	l, r := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if l < 0 || r < l {
		return Proc{}, fmt.Errorf("/proc/%d/stat: malformed", pid)
	}
	rest := strings.Fields(s[r+1:])
	if len(rest) < 2 {
		return Proc{}, fmt.Errorf("/proc/%d/stat: malformed", pid)
	}
	ppid, _ := strconv.Atoi(rest[1])
	return Proc{PID: pid, PPID: ppid, State: rest[0], Comm: s[l+1 : r]}, nil
}

// PGProcs is the F2a/F3a node probe: the postmaster PID recorded in
// PGDATA/postmaster.pid, every process whose comm is "postgres", and the
// state of each PID asked for ("absent" when /proc has no entry).
type PGProcs struct {
	PIDFile       string            `json:"pid_file"`
	PostmasterPID int               `json:"postmaster_pid"` // 0: no postmaster.pid
	PIDFileError  string            `json:"pid_file_error,omitempty"`
	Postgres      []Proc            `json:"postgres"`
	Asked         map[string]string `json:"asked"`
}

// ProbePG reads the process table.
func ProbePG(pgdata string, asked []int) PGProcs {
	out := PGProcs{PIDFile: filepath.Join(pgdata, "postmaster.pid"), Postgres: []Proc{}, Asked: map[string]string{}}
	if f, err := os.Open(out.PIDFile); err != nil {
		out.PIDFileError = err.Error()
	} else {
		sc := bufio.NewScanner(f)
		if sc.Scan() {
			out.PostmasterPID, _ = strconv.Atoi(strings.TrimSpace(sc.Text()))
		}
		f.Close()
	}
	ents, _ := os.ReadDir("/proc")
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if p, err := ReadProc(pid); err == nil && p.Comm == "postgres" {
			out.Postgres = append(out.Postgres, p)
		}
	}
	sort.Slice(out.Postgres, func(i, j int) bool { return out.Postgres[i].PID < out.Postgres[j].PID })
	for _, pid := range asked {
		if p, err := ReadProc(pid); err == nil {
			out.Asked[strconv.Itoa(pid)] = p.State
		} else {
			out.Asked[strconv.Itoa(pid)] = "absent"
		}
	}
	return out
}
