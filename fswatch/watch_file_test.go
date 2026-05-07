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
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/utils/fswatch"
)

// TestWatchFileFiresOnInPlaceWrite verifies that an in-place write
// triggers onChange.
func TestWatchFileFiresOnInPlaceWrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	if err := os.WriteFile(target, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	var changes atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- fswatch.WatchFile(ctx, target, func() {
			changes.Add(1)
		})
	}()
	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(target, []byte("v22"), 0644); err != nil {
		t.Fatal(err)
	}

	if !waitFor(2*time.Second, func() bool { return changes.Load() >= 1 }) {
		t.Fatal("onChange not invoked within 2s")
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

// TestWatchFileAtomicRename verifies that rename-into-place fires
// onChange.
func TestWatchFileAtomicRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	if err := os.WriteFile(target, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	var changes atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = fswatch.WatchFile(ctx, target, func() { changes.Add(1) })
	}()
	time.Sleep(100 * time.Millisecond)

	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte("v2-different-size"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatal(err)
	}

	if !waitFor(2*time.Second, func() bool { return changes.Load() >= 1 }) {
		t.Fatal("onChange not invoked within 2s for atomic rename")
	}
}

// TestWatchFileInitialCallback verifies that WithInitialCallback fires
// onChange once at startup.
func TestWatchFileInitialCallback(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	if err := os.WriteFile(target, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	var changes atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = fswatch.WatchFile(ctx, target, func() {
		changes.Add(1)
	}, fswatch.WithInitialCallback())

	if got := changes.Load(); got != 1 {
		t.Errorf("onChange invoked %d times, want 1 (initial only)", got)
	}
}

// TestWatchFileRecheckFiresUnconditionally verifies that
// WithRecheckInterval fires onChange on every tick regardless of
// whether the file changed. Callers rely on this for retry-on-failure
// semantics that match the original WatchUntil eventHandler-on-tick
// behavior.
func TestWatchFileRecheckFiresUnconditionally(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config")
	if err := os.WriteFile(target, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	var changes atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	_ = fswatch.WatchFile(ctx, target, func() {
		changes.Add(1)
	}, fswatch.WithRecheckInterval(100*time.Millisecond))

	if got := changes.Load(); got < 3 {
		t.Errorf("recheck-on-tick fired %d times in 700ms with 100ms interval, want >= 3", got)
	}
}

// TestWatchFileErrorHandlerCalledOnInitFailure verifies that the
// error handler is invoked when the parent directory does not exist.
func TestWatchFileErrorHandlerCalledOnInitFailure(t *testing.T) {
	target := "/nonexistent-fswatch-dir/some/file"

	var hits atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = fswatch.WatchFile(ctx, target, func() {},
		fswatch.WithErrorHandler(func(error) { hits.Add(1) }),
		fswatch.WithFallbackPolling(50*time.Millisecond),
	)
	if hits.Load() == 0 {
		t.Errorf("error handler not invoked on init failure")
	}
}
