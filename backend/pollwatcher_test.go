package backend

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPollWatcherReadErrorRetries: a change whose read fails must be retried
// on the next tick (the hash is rolled back), not permanently swallowed.
func TestPollWatcherReadErrorRetries(t *testing.T) {
	path := writeTempConfig(t, "first")
	calls := 0
	read := func() (*Content, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("transient read error")
		}
		return &Content{Source: "test"}, nil
	}
	w, err := NewPollWatcher(path, 10*time.Millisecond, read)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	ch := w.Watch()

	if err := os.WriteFile(path, []byte("second, longer content"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case c := <-ch:
		if c.Source != "test" {
			t.Fatalf("unexpected content: %+v", c)
		}
		if calls < 2 {
			t.Fatalf("expected read to be retried after the first error, got %d call(s)", calls)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("change was never delivered: failed read swallowed the update")
	}
}

// TestPollWatcherZeroIntervalDefaulted: a zero (or negative) interval must not
// busy-loop; it falls back to the default.
func TestPollWatcherZeroIntervalDefaulted(t *testing.T) {
	path := writeTempConfig(t, "content")
	w, err := NewPollWatcher(path, 0, func() (*Content, error) { return &Content{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	if got := w.(*pollWatcher).interval; got != defaultPollInterval {
		t.Fatalf("expected zero interval to default to %s, got %s", defaultPollInterval, got)
	}
}

// TestPollWatcherEmptyPathDormant: an empty path yields a watcher that never
// delivers and whose Stop returns cleanly.
func TestPollWatcherEmptyPathDormant(t *testing.T) {
	w, err := NewPollWatcher("", 10*time.Millisecond, func() (*Content, error) { return &Content{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	ch := w.Watch()
	select {
	case c := <-ch:
		t.Fatalf("dormant watcher delivered content: %+v", c)
	case <-time.After(100 * time.Millisecond):
	}
	w.Stop()
}
