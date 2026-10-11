// Command cpnet runs inside the nemesis helpers (CONTRACT §5.1 nem-*, §8):
//
//	cpnet idle                                         # the helper's main process
//	cpnet connect [--timeout 1s] [--count 1] host:port ...
//	cpnet http [--timeout 2s] <url>
//
// connect and http print one JSON line on stdout: the raw probe output that
// the nemesis records (CONTRACT §15 K3.4).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/netprobe"
)

func main() { os.Exit(run(os.Args[1:])) }

func emit(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(string(b))
	return 0
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cpnet idle | connect [--timeout d] [--count n] addr... | http [--timeout d] url")
		return 3
	}
	fs := flag.NewFlagSet("cpnet "+args[0], flag.ContinueOnError)
	switch args[0] {
	case "idle":
		// PID 1 of a helper: exit promptly on SIGTERM so teardown does
		// not wait for the stop timeout.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
		<-ch
		return 0
	case "connect":
		timeout := fs.Duration("timeout", time.Second, "per-attempt connect timeout")
		count := fs.Int("count", 1, "sequential attempts per address")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() == 0 || *count < 1 {
			return 3
		}
		return emit(netprobe.RunConnect(fs.Args(), *timeout, *count))
	case "http":
		timeout := fs.Duration("timeout", 2*time.Second, "request timeout")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 1 {
			return 3
		}
		return emit(netprobe.RunHTTP(fs.Arg(0), *timeout))
	}
	fmt.Fprintln(os.Stderr, "cpnet: unknown subcommand "+args[0])
	return 3
}
