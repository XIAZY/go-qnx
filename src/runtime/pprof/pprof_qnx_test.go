// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pprof

import (
	"bytes"
	"internal/profile"
	"testing"
	"time"
)

// cpuSamples profiles f at the default rate and returns the number of
// samples in the profile, weights included.
func cpuSamples(t *testing.T, f func()) int64 {
	var buf bytes.Buffer
	if err := StartCPUProfile(&buf); err != nil {
		t.Fatal(err)
	}
	f()
	StopCPUProfile()
	p, err := profile.Parse(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	for _, s := range p.Sample {
		n += s.Value[0]
	}
	return n
}

// On qnx a profiling thread signals the threads whose CPU time grew,
// weighting each sample by the periods it covers. A second of CPU time
// at 100 Hz must come out near 100 samples, and a second of sleep near
// none.
func TestQNXCPUProfileWeights(t *testing.T) {
	busy := cpuSamples(t, func() {
		for end := time.Now().Add(time.Second); time.Now().Before(end); {
		}
	})
	if busy < 70 || busy > 130 {
		t.Errorf("1 s of CPU at 100 Hz gave %d samples, want about 100", busy)
	}
	idle := cpuSamples(t, func() { time.Sleep(time.Second) })
	if idle > 10 {
		t.Errorf("1 s of sleep gave %d samples, want about 0", idle)
	}
	t.Logf("busy %d samples, idle %d", busy, idle)
}
