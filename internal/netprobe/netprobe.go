// Package netprobe holds the network probes the nemesis helpers run (CONTRACT
// §8: F1 connect probes, F2 readiness, F7 connect RTT). The cpnet command
// prints their results as one JSON line, the raw probe output recorded in
// commands.jsonl and copied into the fault event's evidence (§15 K3.4). The
// runner parses the same types to evaluate the confirmation predicates.
package netprobe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Error kinds of a connect attempt.
const (
	KindTimeout     = "timeout"
	KindRefused     = "refused"
	KindUnreachable = "unreachable"
	KindOther       = "other"
)

// Attempt is one TCP connect.
type Attempt struct {
	OK  bool   `json:"ok"`
	Err string `json:"err,omitempty"` // a Kind* value
	Msg string `json:"msg,omitempty"`
	US  int64  `json:"us"` // duration of connect() in microseconds
}

// Target is the result of Count connects to one address.
type Target struct {
	Addr     string    `json:"addr"`
	Attempts []Attempt `json:"attempts"`
}

// Connect is the output of `cpnet connect`.
type Connect struct {
	TimeoutMS int64    `json:"timeout_ms"`
	Count     int      `json:"count"`
	Results   []Target `json:"results"`
}

// Classify maps a dial error to a Kind.
func Classify(err error) string {
	var ne net.Error
	switch {
	case err == nil:
		return ""
	case errors.As(err, &ne) && ne.Timeout():
		return KindTimeout
	case errors.Is(err, syscall.ECONNREFUSED):
		return KindRefused
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return KindUnreachable
	}
	return KindOther
}

// RunConnect connects to every address concurrently, count times each in
// sequence, each attempt bounded by timeout.
func RunConnect(addrs []string, timeout time.Duration, count int) Connect {
	out := Connect{TimeoutMS: timeout.Milliseconds(), Count: count, Results: make([]Target, len(addrs))}
	var wg sync.WaitGroup
	for i, a := range addrs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := Target{Addr: a}
			for range count {
				d := net.Dialer{Timeout: timeout}
				start := time.Now()
				c, err := d.Dial("tcp", a)
				at := Attempt{US: time.Since(start).Microseconds(), OK: err == nil}
				if err != nil {
					at.Err, at.Msg = Classify(err), err.Error()
				} else {
					c.Close()
				}
				t.Attempts = append(t.Attempts, at)
			}
			out.Results[i] = t
		}()
	}
	wg.Wait()
	return out
}

// Result returns the target for addr (nil if absent).
func (c Connect) Result(addr string) *Target {
	for i := range c.Results {
		if c.Results[i].Addr == addr {
			return &c.Results[i]
		}
	}
	return nil
}

// AllFailedWith reports whether every attempt failed with kind.
func (t *Target) AllFailedWith(kind string) bool {
	if t == nil || len(t.Attempts) == 0 {
		return false
	}
	for _, a := range t.Attempts {
		if a.OK || a.Err != kind {
			return false
		}
	}
	return true
}

// AllOK reports whether every attempt connected.
func (t *Target) AllOK() bool {
	if t == nil || len(t.Attempts) == 0 {
		return false
	}
	for _, a := range t.Attempts {
		if !a.OK {
			return false
		}
	}
	return true
}

// MedianUS is the median connect duration of the successful attempts, and
// whether every attempt succeeded.
func (t *Target) MedianUS() (int64, bool) {
	if t == nil {
		return 0, false
	}
	var xs []int64
	for _, a := range t.Attempts {
		if a.OK {
			xs = append(xs, a.US)
		}
	}
	if len(xs) == 0 || len(xs) != len(t.Attempts) {
		return 0, false
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	return xs[(len(xs)-1)/2], true
}

// HTTP is the output of `cpnet http`.
type HTTP struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
	Body   string `json:"body,omitempty"` // first 256 bytes
	Err    string `json:"err,omitempty"`
	US     int64  `json:"us"`
}

// RunHTTP performs one GET bounded by timeout.
func RunHTTP(url string, timeout time.Duration) HTTP {
	out := HTTP{URL: url}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	out.US = time.Since(start).Microseconds()
	if err != nil {
		out.Err = fmt.Sprintf("%s: %v", Classify(err), err)
		return out
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	out.Status, out.Body = resp.StatusCode, string(b)
	return out
}
