package download

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// --- newID ---

func TestNewID_Format(t *testing.T) {
	id := newID("t")
	if !strings.HasPrefix(id, "t-") {
		t.Fatalf("expected prefix 't-', got %q", id)
	}
	// "t-" + 16 hex chars = 18 total
	if len(id) != 18 {
		t.Fatalf("expected length 18, got %d (%q)", len(id), id)
	}
}

func TestNewID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := newID("t")
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate ID on iteration %d: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewID_Prefix(t *testing.T) {
	for _, prefix := range []string{"t", "b", "x"} {
		id := newID(prefix)
		if !strings.HasPrefix(id, prefix+"-") {
			t.Errorf("prefix %q: got %q", prefix, id)
		}
	}
}

// --- HTTPError ---

func TestHTTPError_Error(t *testing.T) {
	e := &HTTPError{StatusCode: 503}
	if e.Error() != "http status 503" {
		t.Fatalf("unexpected: %q", e.Error())
	}
}

// --- isRetryable ---

func TestIsRetryable_Nil(t *testing.T) {
	if isRetryable(nil) {
		t.Fatal("nil error should not be retryable")
	}
}

func TestIsRetryable_EmptyURL(t *testing.T) {
	if isRetryable(errEmptyURL) {
		t.Fatal("errEmptyURL should not be retryable")
	}
}

func TestIsRetryable_HTTP5xx(t *testing.T) {
	for _, code := range []int{500, 502, 503, 504, 520, 599} {
		err := &HTTPError{StatusCode: code}
		if !isRetryable(err) {
			t.Errorf("HTTP %d should be retryable", code)
		}
	}
}

func TestIsRetryable_HTTP429(t *testing.T) {
	err := &HTTPError{StatusCode: 429}
	if !isRetryable(err) {
		t.Fatal("HTTP 429 should be retryable")
	}
}

func TestIsRetryable_HTTP4xx_NotRetryable(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 410, 451} {
		err := &HTTPError{StatusCode: code}
		if isRetryable(err) {
			t.Errorf("HTTP %d should NOT be retryable", code)
		}
	}
}

func TestIsRetryable_NetTimeout(t *testing.T) {
	err := &net.DNSError{IsTimeout: true}
	if !isRetryable(err) {
		t.Fatal("net timeout should be retryable")
	}
}

func TestIsRetryable_NetOpError(t *testing.T) {
	err := &net.OpError{Op: "dial", Err: fmt.Errorf("connection refused")}
	if !isRetryable(err) {
		t.Fatal("net.OpError should be retryable")
	}
}

func TestIsRetryable_WrappedHTTPError(t *testing.T) {
	inner := &HTTPError{StatusCode: 503}
	wrapped := fmt.Errorf("get url: %w", inner)
	if !isRetryable(wrapped) {
		t.Fatal("wrapped HTTPError 503 should be retryable")
	}
}

func TestIsRetryable_GenericError(t *testing.T) {
	err := errors.New("some random error")
	if isRetryable(err) {
		t.Fatal("generic error should not be retryable")
	}
}

// --- withRetry ---

func TestWithRetry_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	err := withRetry(3, 2, func() error {
		calls++
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestWithRetry_SuccessAfterRetries(t *testing.T) {
	calls := 0
	err := withRetry(3, 1, func() error {
		calls++
		if calls < 3 {
			return &HTTPError{StatusCode: 503}
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestWithRetry_AllFail(t *testing.T) {
	calls := 0
	err := withRetry(3, 1, func() error {
		calls++
		return &HTTPError{StatusCode: 500}
	}, nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestWithRetry_NonRetryableExitsImmediately(t *testing.T) {
	calls := 0
	err := withRetry(3, 1, func() error {
		calls++
		return &HTTPError{StatusCode: 404}
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry for 404), got %d", calls)
	}
}

func TestWithRetry_OnRetryCallback(t *testing.T) {
	var attempts []int
	_ = withRetry(3, 1, func() error {
		return &HTTPError{StatusCode: 502}
	}, func(attempt int, waitMs int64, err error) {
		attempts = append(attempts, attempt)
	})
	// onRetry is called for attempts 1 and 2 (not the final attempt 3).
	if len(attempts) != 2 || attempts[0] != 1 || attempts[1] != 2 {
		t.Fatalf("expected onRetry for attempts [1,2], got %v", attempts)
	}
}

func TestWithRetry_BackoffCap(t *testing.T) {
	// With backoffBase=100 and attempt=2, raw = 100^1 = 100s > 60s cap.
	// Verify it doesn't take > 61s.
	start := time.Now()
	_ = withRetry(2, 100, func() error {
		return &HTTPError{StatusCode: 500}
	}, nil)
	elapsed := time.Since(start)
	// Should be capped at ~60s for the single sleep between attempt 1 and 2.
	// But since maxRetries=2, there's 1 sleep. With cap 60s, elapsed < 62s.
	if elapsed > 62*time.Second {
		t.Fatalf("backoff should be capped at 60s, took %v", elapsed)
	}
}

func TestWithRetry_EmptyURLNoRetry(t *testing.T) {
	calls := 0
	err := withRetry(3, 1, func() error {
		calls++
		return errEmptyURL
	}, nil)
	if !errors.Is(err, errEmptyURL) {
		t.Fatalf("expected errEmptyURL, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry for empty URL), got %d", calls)
	}
}

// --- Manager basic ---

func TestManager_EnqueueAndGetTask(t *testing.T) {
	// Use a no-op manager (no real downloads).
	m := NewManager(Config{
		MusicDir:     t.TempDir(),
		Concurrency:  1,
		MaxRetries:   1,
		RetryBackoff: 1,
	}, nil)

	// Won't actually download because providers is nil and getURL will panic,
	// but we can test task creation before the goroutine runs.
	// Instead test via LoadTasks.
	m.LoadTasks([]*Task{
		{ID: "t-abc123", Source: "test", Status: StatusDone},
		{ID: "t-def456", Source: "test", Status: StatusFailed},
	})

	task, ok := m.GetTask("t-abc123")
	if !ok {
		t.Fatal("expected task t-abc123")
	}
	if task.Status != StatusDone {
		t.Fatalf("expected done, got %s", task.Status)
	}

	// Batch IDs should not be found (never loaded as tasks).
	_, ok = m.GetTask("b-nonexistent")
	if ok {
		t.Fatal("should not find non-existent batch ID")
	}
}

func TestManager_ListTasks_ExcludesBatchPrefix(t *testing.T) {
	m := NewManager(Config{MusicDir: t.TempDir(), Concurrency: 1, MaxRetries: 1, RetryBackoff: 1}, nil)
	m.LoadTasks([]*Task{
		{ID: "t-001", Source: "test", Status: StatusDone},
		{ID: "b-001", Source: "test", Status: StatusDone}, // should be excluded
		{ID: "t-002", Source: "test", Status: StatusPending, BatchID: "b-001"},
	})
	tasks := m.ListTasks()
	for _, task := range tasks {
		if strings.HasPrefix(task.ID, "b-") {
			t.Fatalf("ListTasks should exclude batch entries, found %q", task.ID)
		}
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestManager_LoadBatchNames(t *testing.T) {
	m := NewManager(Config{MusicDir: t.TempDir(), Concurrency: 1, MaxRetries: 1, RetryBackoff: 1}, nil)
	m.LoadBatchNames(map[string]string{
		"b-001": "My Playlist",
		"b-002": "Another",
	})
	m.LoadTasks([]*Task{
		{ID: "t-001", Source: "test", Status: StatusDone, BatchID: "b-001"},
		{ID: "t-002", Source: "test", Status: StatusFailed, BatchID: "b-001"},
		{ID: "t-003", Source: "test", Status: StatusDone, BatchID: "b-002"},
	})

	batches := m.ListBatches()
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	if batches[0].Name != "My Playlist" {
		t.Fatalf("expected 'My Playlist', got %q", batches[0].Name)
	}
	if batches[0].Done != 1 || batches[0].Failed != 1 {
		t.Fatalf("batch 0: expected done=1 failed=1, got done=%d failed=%d", batches[0].Done, batches[0].Failed)
	}
	if batches[1].Name != "Another" {
		t.Fatalf("expected 'Another', got %q", batches[1].Name)
	}
}
