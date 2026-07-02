package llmextractor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Herrscherd/herrscher-contracts"
	"github.com/Herrscherd/herrscher-orchestrator"
)

type fakeBackend struct {
	mu     sync.Mutex
	reply  string
	err    error
	got    contracts.Prompt
	closed bool
}

func (f *fakeBackend) Respond(_ context.Context, p contracts.Prompt, _ func(contracts.BackendEvent)) (string, error) {
	f.mu.Lock()
	f.got = p
	f.mu.Unlock()
	return f.reply, f.err
}

func (f *fakeBackend) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

// compile-time proof the type satisfies the seam.
var _ orchestrator.Extractor = (*LLMExtractor)(nil)

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
	cs, _ := e.Extract(context.Background(), "j", "t")
	if len(cs) != 1 {
		t.Fatalf("threshold via Extract: want 1, got %d", len(cs))
	}
}

func TestExtract_ConcurrentLazyInitIsRaceFree(t *testing.T) {
	e := &LLMExtractor{
		newBackend: func() (contracts.Backend, error) { return &fakeBackend{reply: twoValid}, nil },
		threshold:  defaultThreshold,
		max:        defaultMax,
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
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
