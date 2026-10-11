// Command slowfs runs inside every pg-nX container (CONTRACT §8, ADR-0006):
//
//	slowfs run --backing <dir> --mount <dir> --control <file> [--bypass d,...] --uid N --gid N -- <command...>
//	slowfs probe --dir <dir> [--size 4096] [--count 5]   # F4a write+fsync latency (P17)
//	slowfs pgprocs --pgdata <dir> [--pid N ...]          # F2a/F3a process states
//	slowfs control --file <file> --set-ms <n>            # F4a injection / heal
//
// Every subcommand except run prints one JSON line on stdout: the raw probe
// output the nemesis records (CONTRACT §15 K3.4).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/slowfs"
)

func main() { os.Exit(run(os.Args[1:])) }

type intList []int

func (l *intList) String() string { return fmt.Sprint(*l) }
func (l *intList) Set(v string) error {
	n, err := strconv.Atoi(v)
	*l = append(*l, n)
	return err
}

func emit(v any, code int) int {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(string(b))
	return code
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: slowfs run|probe|pgprocs|control ...")
		return 3
	}
	fs := flag.NewFlagSet("slowfs "+args[0], flag.ContinueOnError)
	switch args[0] {
	case "run":
		backing := fs.String("backing", "", "backing directory (the node's data volume)")
		mount := fs.String("mount", "", "mount point")
		control := fs.String("control", "", "control file (outside the mount)")
		bypass := fs.String("bypass", "", "comma-separated subdirectories bind-mounted from the backing volume")
		uid := fs.Int("uid", -1, "uid of the command")
		gid := fs.Int("gid", -1, "gid of the command")
		if err := fs.Parse(args[1:]); err != nil {
			return 3
		}
		if *backing == "" || *mount == "" || *control == "" || *uid < 0 || *gid < 0 {
			fmt.Fprintln(os.Stderr, "slowfs run: --backing, --mount, --control, --uid, --gid are required")
			return 3
		}
		var by []string
		for _, d := range strings.Split(*bypass, ",") {
			if d = strings.TrimSpace(d); d != "" {
				by = append(by, d)
			}
		}
		return slowfs.Launch(slowfs.LaunchConfig{Backing: *backing, Mount: *mount, Control: *control,
			Bypass: by, UID: *uid, GID: *gid, Argv: fs.Args()})
	case "probe":
		dir := fs.String("dir", "", "directory on the mount")
		size := fs.Int("size", 4096, "bytes per write")
		count := fs.Int("count", 5, "samples")
		if err := fs.Parse(args[1:]); err != nil || *dir == "" {
			return 3
		}
		p, err := slowfs.ProbeIO(*dir, *size, *count)
		if err != nil {
			return emit(map[string]any{"dir": *dir, "error": err.Error()}, 2)
		}
		return emit(p, 0)
	case "pgprocs":
		pgdata := fs.String("pgdata", "", "PGDATA")
		var pids intList
		fs.Var(&pids, "pid", "PID whose state to report (repeatable)")
		if err := fs.Parse(args[1:]); err != nil || *pgdata == "" {
			return 3
		}
		return emit(slowfs.ProbePG(*pgdata, pids), 0)
	case "control":
		file := fs.String("file", "", "control file")
		ms := fs.Int64("set-ms", -1, "delay in milliseconds")
		if err := fs.Parse(args[1:]); err != nil || *file == "" || *ms < 0 {
			return 3
		}
		if err := slowfs.SetControl(*file, time.Duration(*ms)*time.Millisecond); err != nil {
			return emit(map[string]any{"control": *file, "error": err.Error()}, 2)
		}
		b, _ := os.ReadFile(*file)
		return emit(map[string]any{"control": *file, "content": string(b)}, 0)
	}
	fmt.Fprintln(os.Stderr, "slowfs: unknown subcommand "+args[0])
	return 3
}
