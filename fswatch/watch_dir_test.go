/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fswatch_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/utils/fswatch"
)

// TestWatchDirFiresOnFileCreate verifies that creating a file in the
// watched directory fires onChange.
func TestWatchDirFiresOnFileCreate(t *testing.T) {
	dir := t.TempDir()

	var changes atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fswatch.WatchDir(ctx, dir, func() { changes.Add(1) })
	}()
	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	if !waitFor(2*time.Second, func() bool { return changes.Load() >= 1 }) {
		t.Fatal("onChange not invoked within 2s for file create")
	}
}

// TestWatchDirRecheckFiresOnTick verifies that WithDirRecheckInterval
// drives onChange callbacks at a regular cadence even when no
// filesystem events occur.
func TestWatchDirRecheckFiresOnTick(t *testing.T) {
	dir := t.TempDir()

	var ticks atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	_ = fswatch.WatchDir(ctx, dir, func() {
		ticks.Add(1)
	}, fswatch.WithDirRecheckInterval(100*time.Millisecond))

	if got := ticks.Load(); got < 3 {
		t.Errorf("recheck-on-tick fired %d times in 600ms with 100ms interval, want >= 3", got)
	}
}

// TestWatchDirRetriesAddOnRecheck verifies that an initial Add(dir)
// failure does not stop the loop: the error handler is invoked, the
// recheck ticker keeps firing onChange, and once the directory exists
// Add succeeds and event-driven updates resume.
func TestWatchDirRetriesAddOnRecheck(t *testing.T) {
	parent := t.TempDir()
	missing := filepath.Join(parent, "later")

	var (
		ticks atomic.Int32
		errs  atomic.Int32
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = fswatch.WatchDir(ctx, missing, func() {
			ticks.Add(1)
		},
			fswatch.WithDirRecheckInterval(100*time.Millisecond),
			fswatch.WithDirErrorHandler(func(error) { errs.Add(1) }),
		)
	}()

	// First few ticks should report the Add failure but keep ticking.
	if !waitFor(2*time.Second, func() bool { return errs.Load() >= 1 && ticks.Load() >= 1 }) {
		t.Fatalf("expected error handler + ticks before dir exists; got errs=%d ticks=%d", errs.Load(), ticks.Load())
	}

	// Create the directory; the next recheck should bring the watch
	// online and a file create should produce a tick.
	if err := os.Mkdir(missing, 0755); err != nil {
		t.Fatal(err)
	}
	// Wait long enough for the next recheck to retry Add.
	time.Sleep(250 * time.Millisecond)
	preTicks := ticks.Load()
	if err := os.WriteFile(filepath.Join(missing, "f"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return ticks.Load() > preTicks }) {
		t.Fatalf("expected event-driven tick after directory recreated; ticks=%d (was %d)", ticks.Load(), preTicks)
	}

	cancel()
	<-done
}
