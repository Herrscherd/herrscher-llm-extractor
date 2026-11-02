package llmextractor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	contracts "github.com/Herrscherd/herrscher-contracts"
)

type fakeBackend struct {
	mu    sync.Mutex
	reply string
	err   error
	got   contracts.Prompt
}

func (f *fakeBackend) Respond(_ context.Context, p contracts.Prompt, _ func(contracts.BackendEvent)) (string, error) {
	f.mu.Lock()
	f.got = p
	f.mu.Unlock()
	return f.reply, f.err
}

func (f *fakeBackend) Close() error {
	return nil
}

func TestExtract_HappyPath(t *testing.T) {
	fb := &fakeBackend{reply: twoValid}
	cs, err := New(fb).Extract(context.Background(), "journal", "transcript")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("want 2, got %d", len(cs))
	}
	if !strings.Contains(fb.got.Content, "transcript") {
		t.Fatal("transcript not passed to backend")
	}
}

func TestExtract_EmptyInputsIsNoOp(t *testing.T) {
	fb := &fakeBackend{reply: twoValid}
	cs, err := New(fb).Extract(context.Background(), "  ", "")
	if err != nil || cs != nil {
		t.Fatalf("empty in → want (nil,nil), got (%v,%v)", cs, err)
	}
	if fb.got.Content != "" {
		t.Fatal("backend should not be called on empty input")
	}
}

func TestExtract_BackendErrorPropagates(t *testing.T) {
	cs, err := New(&fakeBackend{err: errors.New("boom")}).Extract(context.Background(), "j", "t")
	if err == nil || cs != nil {
		t.Fatalf("want error, got (%v,%v)", cs, err)
	}
}

func TestExtract_NilBackendIsNoOp(t *testing.T) {
	cs, err := (&LLMExtractor{}).Extract(context.Background(), "j", "t")
	if err != nil || cs != nil {
		t.Fatalf("nil backend → want (nil,nil), got (%v,%v)", cs, err)
	}
}

func TestOptions_ThresholdAndMax(t *testing.T) {
	e := New(&fakeBackend{reply: twoValid}, WithThreshold(0.85), WithMax(5))
	if e.threshold != 0.85 || e.max != 5 {
		t.Fatalf("options not applied: %v %v", e.threshold, e.max)
	}
	cs, err := e.Extract(context.Background(), "j", "t")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(cs) != 1 {
		t.Fatalf("threshold via Extract: want 1, got %d", len(cs))
	}
}

func TestExtract_ConcurrentLazyInitIsRaceFree(t *testing.T) {
	e := &LLMExtractor{
		newBackend: func(context.Context) (contracts.Backend, error) { return &fakeBackend{reply: twoValid}, nil },
		threshold:  defaultThreshold,
		max:        defaultMax,
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.Extract(context.Background(), "j", "t"); err != nil {
				t.Errorf("Extract: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestExtract_LazyBackendBuildFailure(t *testing.T) {
	buildErr := errors.New("missing env")
	cases := []struct {
		name       string
		err        error
		wantCalls  int
		wantReport bool
	}{
		{name: "permanent error is memorised", err: buildErr, wantCalls: 1, wantReport: true},
		{name: "cancellation is retried", err: context.Canceled, wantCalls: 2, wantReport: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertLazyBuildFailure(t, tc.err, tc.wantCalls, tc.wantReport)
		})
	}
}

func assertLazyBuildFailure(t *testing.T, buildErr error, wantCalls int, wantReport bool) {
	t.Helper()
	calls := 0
	e := &LLMExtractor{
		newBackend: func(context.Context) (contracts.Backend, error) {
			calls++
			return nil, buildErr
		},
		threshold: defaultThreshold,
		max:       defaultMax,
	}
	for range 2 {
		cs, err := e.Extract(context.Background(), "j", "t")
		if err != nil || cs != nil {
			t.Fatalf("want clean no-op, got (%v,%v)", cs, err)
		}
	}
	if calls != wantCalls {
		t.Fatalf("newBackend calls: want %d, got %d", wantCalls, calls)
	}
	if got := e.BackendErr(); wantReport && !errors.Is(got, buildErr) {
		t.Fatalf("BackendErr: want wrapping %v, got %v", buildErr, got)
	}
}

func TestExtract_BackendBuildNotAttemptedOnEmptyInputs(t *testing.T) {
	calls := 0
	e := &LLMExtractor{
		newBackend: func(context.Context) (contracts.Backend, error) {
			calls++
			return &fakeBackend{reply: twoValid}, nil
		},
		threshold: defaultThreshold,
		max:       defaultMax,
	}
	if _, err := e.Extract(context.Background(), " ", ""); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if calls != 0 {
		t.Fatalf("backend built on empty inputs: %d calls", calls)
	}
}

func TestExtract_LazyBackendReceivesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var seenCancelled bool
	e := &LLMExtractor{
		newBackend: func(c context.Context) (contracts.Backend, error) {
			seenCancelled = c.Err() != nil
			return nil, c.Err()
		},
		threshold: defaultThreshold,
		max:       defaultMax,
	}
	if _, err := e.Extract(ctx, "j", "t"); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !seenCancelled {
		t.Fatal("caller context not propagated to the backend factory")
	}
}

func TestExtract_BackendErrorIsWrapped(t *testing.T) {
	boom := errors.New("boom")
	_, err := New(&fakeBackend{err: boom}).Extract(context.Background(), "j", "t")
	if !errors.Is(err, boom) {
		t.Fatalf("want wrapped boom, got %v", err)
	}
}

func TestExtract_ConcurrentCallersBuildBackendOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		release := make(chan struct{})
		e := &LLMExtractor{
			newBackend: func(context.Context) (contracts.Backend, error) {
				calls.Add(1)
				<-release
				return &fakeBackend{reply: twoValid}, nil
			},
			threshold: defaultThreshold,
			max:       defaultMax,
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				cs, err := e.Extract(context.Background(), "j", "t")
				if err != nil || len(cs) != 2 {
					t.Errorf("Extract: want 2 candidates, got (%v,%v)", cs, err)
				}
			})
		}
		synctest.Wait()
		close(release)
		wg.Wait()
		if got := calls.Load(); got != 1 {
			t.Fatalf("newBackend calls: want 1, got %d", got)
		}
	})
}

func TestExtract_WaiterLeavesWhenItsContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		e := &LLMExtractor{
			newBackend: func(context.Context) (contracts.Backend, error) {
				<-release
				return &fakeBackend{reply: twoValid}, nil
			},
			threshold: defaultThreshold,
			max:       defaultMax,
		}
		var builder sync.WaitGroup
		builder.Go(func() {
			if cs, err := e.Extract(context.Background(), "j", "t"); err != nil || len(cs) != 2 {
				t.Errorf("builder: want 2 candidates, got (%v,%v)", cs, err)
			}
		})
		synctest.Wait()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cs, err := e.Extract(ctx, "j", "t")
		if err != nil || cs != nil {
			t.Fatalf("waiter past its deadline: want (nil,nil), got (%v,%v)", cs, err)
		}
		close(release)
		builder.Wait()
	})
}
