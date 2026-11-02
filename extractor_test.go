package llmextractor

import (
	"context"
	"errors"
	"fmt"
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
		callerCtx  func() context.Context
		wantCalls  int
		wantReport bool
	}{
		{name: "permanent error is memorised and reported", err: buildErr, callerCtx: context.Background, wantCalls: 1, wantReport: true},
		{name: "cancellation is retried and not reported", err: context.Canceled, callerCtx: cancelledContext, wantCalls: 2, wantReport: false},
		{name: "deadline is retried and not reported", err: context.DeadlineExceeded, callerCtx: expiredContext, wantCalls: 2, wantReport: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertLazyBuildFailure(t, tc.callerCtx, tc.err, tc.wantCalls, tc.wantReport)
		})
	}
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func expiredContext() context.Context {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	cancel()
	return ctx
}

func TestExtract_BuildErrorWrappingDeadlineWithLiveContextIsMemorised(t *testing.T) {
	buildErr := fmt.Errorf("plugin handshake: %w", context.DeadlineExceeded)
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
	if calls != 1 {
		t.Fatalf("newBackend calls: want 1 (memorised despite the deadline error), got %d", calls)
	}
	got := e.BackendErr()
	if !errors.Is(got, buildErr) {
		t.Fatalf("BackendErr: want wrapping %v, got %v", buildErr, got)
	}
	if !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("BackendErr: want the deadline reachable through errors.Is, got %v", got)
	}
}

func assertLazyBuildFailure(t *testing.T, callerCtx func() context.Context, buildErr error, wantCalls int, wantReport bool) {
	t.Helper()
	ctx := callerCtx()
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
		cs, err := e.Extract(ctx, "j", "t")
		if err != nil || cs != nil {
			t.Fatalf("want clean no-op, got (%v,%v)", cs, err)
		}
	}
	if calls != wantCalls {
		t.Fatalf("newBackend calls: want %d, got %d", wantCalls, calls)
	}
	got := e.BackendErr()
	if wantReport && !errors.Is(got, buildErr) {
		t.Fatalf("BackendErr: want wrapping %v, got %v", buildErr, got)
	}
	if !wantReport && got != nil {
		t.Fatalf("BackendErr: want nil for an interrupted build, got %v", got)
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
	var calls atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	e := &LLMExtractor{
		newBackend: func(context.Context) (contracts.Backend, error) {
			calls.Add(1)
			entered <- struct{}{}
			<-release
			return &fakeBackend{reply: twoValid}, nil
		},
		threshold: defaultThreshold,
		max:       defaultMax,
	}
	var callers sync.WaitGroup
	for range 8 {
		callers.Go(func() {
			cs, err := e.Extract(context.Background(), "j", "t")
			if err != nil || len(cs) != 2 {
				t.Errorf("Extract: want 2 candidates, got (%v,%v)", cs, err)
			}
		})
	}
	awaitWithin(t, entered, "the backend build never started")

	bailed := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	go func() {
		defer close(bailed)
		if cs, err := e.Extract(ctx, "j", "t"); err != nil || cs != nil {
			t.Errorf("waiter with a done context: want (nil,nil), got (%v,%v)", cs, err)
		}
	}()
	awaitWithin(t, bailed, "a waiter with a done context stayed blocked behind the build")

	close(release)
	awaitWithin(t, allDone(&callers), "callers still blocked after the build was released")
	if builds := calls.Load(); builds != 1 {
		t.Fatalf("newBackend calls: want 1, got %d", builds)
	}
}

func TestExtract_WaiterTakesOverWhenBuilderIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		builderCtx, cancelBuilder := context.WithCancel(context.Background())
		defer cancelBuilder()
		var calls atomic.Int32
		e := &LLMExtractor{
			newBackend: func(ctx context.Context) (contracts.Backend, error) {
				if calls.Add(1) == 1 {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return &fakeBackend{reply: twoValid}, nil
			},
			threshold: defaultThreshold,
			max:       defaultMax,
		}
		var builder sync.WaitGroup
		builder.Go(func() {
			if cs, err := e.Extract(builderCtx, "j", "t"); err != nil || cs != nil {
				t.Errorf("cancelled builder: want (nil,nil), got (%v,%v)", cs, err)
			}
		})
		synctest.Wait()

		var waiter sync.WaitGroup
		waiter.Go(func() {
			if cs, err := e.Extract(context.Background(), "j", "t"); err != nil || len(cs) != 2 {
				t.Errorf("waiter: want 2 candidates after takeover, got (%v,%v)", cs, err)
			}
		})
		synctest.Wait()
		cancelBuilder()
		builder.Wait()
		waiter.Wait()
		if got := calls.Load(); got != 2 {
			t.Fatalf("newBackend calls: want 2 (cancelled builder, then waiter), got %d", got)
		}
		if err := e.BackendErr(); err != nil {
			t.Fatalf("BackendErr after a cancelled build: want nil, got %v", err)
		}
	})
}

func TestExtract_PanickingBuildIsRetried(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		e := &LLMExtractor{
			newBackend: func(context.Context) (contracts.Backend, error) {
				calls++
				if calls == 1 {
					panic("factory bug")
				}
				return &fakeBackend{reply: twoValid}, nil
			},
			threshold: defaultThreshold,
			max:       defaultMax,
		}
		func() {
			defer func() {
				if got := recover(); got != "factory bug" {
					t.Errorf("recovered %v, want the factory panic", got)
				}
			}()
			cs, err := e.Extract(context.Background(), "j", "t")
			t.Errorf("Extract returned (%v,%v), want the factory panic", cs, err)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cs, err := e.Extract(ctx, "j", "t")
		if err != nil || len(cs) != 2 {
			t.Fatalf("Extract after a panicking build: want 2 candidates, got (%v,%v)", cs, err)
		}
		if calls != 2 {
			t.Fatalf("newBackend calls: want 2 (panic, then success), got %d", calls)
		}
	})
}

func TestExtract_WaiterLeavesWhenItsContextEnds(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	e := &LLMExtractor{
		newBackend: func(context.Context) (contracts.Backend, error) {
			entered <- struct{}{}
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
	awaitWithin(t, entered, "the backend build never started")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(10*time.Millisecond, cancel)
	left := make(chan struct{})
	go func() {
		defer close(left)
		if cs, err := e.Extract(ctx, "j", "t"); err != nil || cs != nil {
			t.Errorf("waiter past its deadline: want (nil,nil), got (%v,%v)", cs, err)
		}
	}()
	awaitWithin(t, left, "the waiter stayed blocked behind the build after its context ended")

	close(release)
	awaitWithin(t, allDone(&builder), "the builder stayed blocked after the build was released")
}

func awaitWithin(t *testing.T, ready <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal(failure)
	}
}

func allDone(wg *sync.WaitGroup) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	return done
}
