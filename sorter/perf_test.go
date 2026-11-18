package sorter

import (
	"context"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestPlanPerf times each planning stage on a real photo folder:
//
//	PHOTOS_DIR=~/Pictures go test ./sorter -run TestPlanPerf -v -count=1
func TestPlanPerf(t *testing.T) {
	dir := os.Getenv("PHOTOS_DIR")
	if dir == "" {
		t.Skip("PHOTOS_DIR not set")
	}
	for _, mode := range []struct {
		name           string
		dedupe, simila bool
	}{{"exif", false, false}, {"dedupe", true, false}, {"similar", false, true}, {"both", true, true}} {
		start := time.Now()
		marks := map[string]time.Duration{}
		engine := Engine{SkipDuplicates: mode.dedupe, GroupSimilar: mode.simila, ByLocation: true,
			Progress: func(stage string, done, total int) {
				if done == total {
					marks[stage] = time.Since(start)
				}
			}}
		cpu := func() time.Duration {
			var u syscall.Rusage
			syscall.Getrusage(syscall.RUSAGE_SELF, &u)
			return time.Duration(u.Utime.Nano() + u.Stime.Nano())
		}
		cpuBefore := cpu()
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		plan, err := engine.Plan(context.Background(), []string{dir}, t.TempDir())
		runtime.ReadMemStats(&after)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%-8s total=%v entries=%d skipped=%d dups=%d marks=%v cpu=%v alloc=%dMB",
			mode.name, time.Since(start).Round(time.Millisecond), len(plan.Entries), len(plan.Skipped), len(plan.Duplicates), marks,
			(cpu() - cpuBefore).Round(time.Millisecond), (after.TotalAlloc-before.TotalAlloc)>>20)
	}
}
