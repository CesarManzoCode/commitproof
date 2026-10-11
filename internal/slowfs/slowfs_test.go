package slowfs

import (
	"os"
	"testing"
	"time"
)

func TestControlFile(t *testing.T) {
	for in, want := range map[string]time.Duration{"delay_ms=200\n": 200 * time.Millisecond, " delay_ms = 0 ": 0} {
		if d, err := ParseControl([]byte(in)); err != nil || d != want {
			t.Errorf("%q: %v %v", in, d, err)
		}
	}
	for _, bad := range []string{"", "delay=5", "delay_ms=-1", "delay_ms=1.5", "delay_ms=600001"} {
		if _, err := ParseControl([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if d, _ := ParseControl(FormatControl(150 * time.Millisecond)); d != 150*time.Millisecond {
		t.Error("format/parse round trip")
	}
}

func TestMedian(t *testing.T) {
	if Median([]int64{5, 1, 3, 2, 4}) != 3 || Median([]int64{4, 1, 3, 2}) != 2 || Median(nil) != 0 {
		t.Fatal("median")
	}
}

func TestReadProcSelf(t *testing.T) {
	p, err := ReadProc(os.Getpid())
	if err != nil || p.PID != os.Getpid() || p.PPID != os.Getppid() || p.State == "" {
		t.Fatalf("%+v %v", p, err)
	}
}
