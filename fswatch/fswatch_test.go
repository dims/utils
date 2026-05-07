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
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"k8s.io/utils/fswatch"
)

func TestNewWatcher(t *testing.T) {
	w, err := fswatch.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherCloseIdempotent(t *testing.T) {
	w, err := fswatch.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close: got %v, want nil", err)
	}
}

func TestWatcherAddRemove(t *testing.T) {
	dir := t.TempDir()
	w, err := fswatch.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Remove(dir); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := w.Remove(dir); !errors.Is(err, fswatch.ErrNonExistentWatch) {
		t.Errorf("second Remove: got %v, want ErrNonExistentWatch", err)
	}
}

func TestWatcherAddAfterClose(t *testing.T) {
	dir := t.TempDir()
	w, err := fswatch.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Add(dir); !errors.Is(err, fswatch.ErrClosed) {
		t.Errorf("Add after Close: got %v, want ErrClosed", err)
	}
}

// TestWatcherDeliversCreate exercises the end-to-end happy path: a
// fresh file in the watched directory produces an event with the
// portable Op set.
func TestWatcherDeliversCreate(t *testing.T) {
	dir := t.TempDir()
	w, err := fswatch.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "f")
	if err := os.WriteFile(target, []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-w.Events():
			if ev.Name == target && ev.Has(fswatch.Create) {
				return
			}
		case err := <-w.Errors():
			t.Fatalf("unexpected error: %v", err)
		case <-deadline:
			t.Fatal("Create event for target not received within 2s")
		}
	}
}

// TestWatcherCloseNoLeakUnderTraffic exercises the shutdown path under
// active event traffic. Without proper coordination between the read
// goroutine and Close, this test would deadlock or leak.
func TestWatcherCloseNoLeakUnderTraffic(t *testing.T) {
	dir := t.TempDir()
	w, err := fswatch.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Add(dir); err != nil {
		t.Fatal(err)
	}

	// Pump events from a goroutine; we deliberately don't drain the
	// public channels.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				name := filepath.Join(dir, "f"+itoa(i))
				_ = os.WriteFile(name, []byte("x"), 0644)
				_ = os.Remove(name)
				i++
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- w.Close() }()

	select {
	case err := <-done:
		close(stop)
		wg.Wait()
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		close(stop)
		wg.Wait()
		t.Fatal("Close deadlocked under traffic")
	}
}

// TestWatcherAddCloseRace exercises Add and Close racing each other.
// Add must either succeed cleanly or return ErrClosed; the underlying
// inotify FD must never be closed mid-syscall (which would surface as
// a syscall errno or a panic under -race).
func TestWatcherAddCloseRace(t *testing.T) {
	for i := 0; i < 50; i++ {
		dir := t.TempDir()
		w, err := fswatch.NewWatcher()
		if err != nil {
			t.Fatal(err)
		}

		var addErrs sync.Map
		var wg sync.WaitGroup
		for j := 0; j < 4; j++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				err := w.Add(dir)
				addErrs.Store(id, err)
			}(j)
		}
		// Race Close against the Adds.
		_ = w.Close()
		wg.Wait()

		addErrs.Range(func(_, v any) bool {
			if v == nil {
				return true
			}
			err, _ := v.(error)
			if err != nil && !errors.Is(err, fswatch.ErrClosed) {
				t.Errorf("Add returned non-ErrClosed error during Close race: %v", err)
			}
			return true
		})
	}
}

func TestOpString(t *testing.T) {
	cases := []struct {
		op   fswatch.Op
		want string
	}{
		{0, ""},
		{fswatch.Create, "Create"},
		{fswatch.Write, "Write"},
		{fswatch.Create | fswatch.Write, "Create|Write"},
	}
	for _, c := range cases {
		if got := c.op.String(); got != c.want {
			t.Errorf("Op(%d).String() = %q, want %q", c.op, got, c.want)
		}
	}
}

// itoa is a tiny helper avoiding strconv import in test loops.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}

func waitFor(d time.Duration, cond func() bool) bool {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}
