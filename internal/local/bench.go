package local

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Bench is what this computer's processor and memory do, measured in about
// a second: what llama.cpp's speed on it follows. Writing a token reads
// every weight of the model once, so it goes as fast as memory is read;
// reading a prompt is arithmetic, as fast as the cores multiply.
type Bench struct {
	// Bandwidth is how fast all the cores together read memory, in GB/s.
	Bandwidth float64 `json:"bandwidth"`
	// Compute is how many int8 multiply-adds a second Hardware.Cores do
	// together, in billions: a quantized model's arithmetic, done one at
	// a time (llama.cpp does many at once with SIMD).
	Compute float64 `json:"compute"`
}

// benchMemory is the buffer read for Bandwidth: well past any CPU's cache,
// smaller on a computer with little memory.
func benchMemory(ram uint64) int {
	if ram > 0 && ram < 4<<30 {
		return 64 << 20
	}
	return 256 << 20
}

// measureBench measures this computer, with cores goroutines for Compute.
func measureBench(ram uint64, cores int) Bench {
	return Bench{Bandwidth: bandwidth(benchMemory(ram)), Compute: compute(max(cores, 1))}
}

// bandwidth reads a buffer of size bytes with every CPU at once, a few
// times, and is the best rate in GB/s.
func bandwidth(size int) float64 {
	buf := make([]uint64, size/8)
	for i := range buf {
		buf[i] = uint64(i)
	}
	n := runtime.NumCPU()
	chunk := len(buf) / n
	best := 0.0
	for range 3 {
		var wg sync.WaitGroup
		t0 := time.Now()
		for g := range n {
			wg.Add(1)
			go func(s []uint64) {
				defer wg.Done()
				for range 2 {
					sink(sumWords(s))
				}
			}(buf[g*chunk : (g+1)*chunk])
		}
		wg.Wait()
		if gbs := float64(chunk*n*8*2) / time.Since(t0).Seconds() / 1e9; gbs > best {
			best = gbs
		}
	}
	return best
}

// sumWords reads s, four sums at a time so that the adds don't wait on
// each other and memory is what limits it.
func sumWords(s []uint64) uint64 {
	var a, b, c, d uint64
	for i := 0; i+8 <= len(s); i += 8 {
		x := s[i : i+8 : i+8]
		a += x[0] + x[4]
		b += x[1] + x[5]
		c += x[2] + x[6]
		d += x[3] + x[7]
	}
	return a + b + c + d
}

// compute runs int8 dot products on n goroutines for a fifth of a second,
// twice, and is the best total rate in billions of multiply-adds a second.
func compute(n int) float64 {
	best := 0.0
	for range 2 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		total := 0.0
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a, b := make([]int8, 4096), make([]int8, 4096)
				for i := range a {
					a[i], b[i] = int8(i), int8(i*7)
				}
				ops := 0
				t0 := time.Now()
				for time.Since(t0) < 200*time.Millisecond {
					for range 64 {
						sink(uint64(dot8(a, b)))
					}
					ops += 64 * len(a)
				}
				mu.Lock()
				total += float64(ops) / time.Since(t0).Seconds()
				mu.Unlock()
			}()
		}
		wg.Wait()
		best = max(best, total/1e9)
	}
	return best
}

// dot8 is an int8 dot product widened to int32, as a quantized matrix
// multiply does it.
func dot8(a, b []int8) int32 {
	var s0, s1, s2, s3 int32
	for i := 0; i+4 <= len(a) && i+4 <= len(b); i += 4 {
		x, y := a[i:i+4:i+4], b[i:i+4:i+4]
		s0 += int32(x[0]) * int32(y[0])
		s1 += int32(x[1]) * int32(y[1])
		s2 += int32(x[2]) * int32(y[2])
		s3 += int32(x[3]) * int32(y[3])
	}
	return s0 + s1 + s2 + s3
}

// sinkVal keeps the compiler from dropping work whose result isn't used.
var sinkVal struct {
	sync.Mutex
	v uint64
}

func sink(v uint64) {
	if v == 1<<63 { // never, but the compiler can't know
		sinkVal.Lock()
		sinkVal.v = v
		sinkVal.Unlock()
	}
}

// existing is path, or the nearest folder above it that exists: where a
// folder not made yet will be.
func existing(path string) string {
	for p := path; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil || p == filepath.Dir(p) {
			return p
		}
	}
}
