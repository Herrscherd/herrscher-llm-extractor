package llmextractor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	contracts "github.com/Herrscherd/herrscher-contracts"
	orchestrator "github.com/Herrscherd/herrscher-orchestrator"
)

// LLMExtractor is the open, reference Extractor: it asks a contracts.Backend to
// distill a stretch of work (journal + transcript) into memory candidates. The
// backend is either injected (New) or built lazily from the plugin registry on
// first use (the registered default, see register.go and backend.go).
type LLMExtractor struct {
	backend    contracts.Backend
	newBackend func(context.Context) (contracts.Backend, error)
	mu         sync.Mutex
	building   chan struct{}
	built      bool
	backendErr error
	threshold  float64
	max        int
}

var _ orchestrator.Extractor = (*LLMExtractor)(nil)

// Option configures an LLMExtractor.
type Option func(*LLMExtractor)

// WithThreshold drops candidates below the given confidence (default 0.6).
func WithThreshold(t float64) Option { return func(e *LLMExtractor) { e.threshold = t } }

// WithMax caps how many candidates one Extract call returns, counted after the
// threshold filter; candidates past the cap are dropped, not deferred. Zero
// disables the cap. The default is 8.
func WithMax(m int) Option { return func(e *LLMExtractor) { e.max = m } }

// New builds an extractor over an explicit backend (tests and callers that
// already hold a model edge).
func New(backend contracts.Backend, opts ...Option) *LLMExtractor {
	extractor := &LLMExtractor{backend: backend, threshold: defaultThreshold, max: defaultMax}
	for _, o := range opts {
		o(extractor)
	}
	return extractor
}

// Extract asks the curation backend to distill journal + transcript into
// candidates. It is best-effort: blank inputs, no registered backend, or a
// backend that failed to build yield (nil, nil), and BackendErr reports the
// build failure. A reply without a usable JSON array also yields (nil, nil).
// The reply is scanned for its first array holding a titled candidate, and only
// within its first 1 MiB and before its 65th '[' byte: an array past either
// bound is dropped without an error. The only error returned is a failed Respond
// call, wrapped with %w, so errors.Is and errors.As reach the backend's own
// error. The package defines no sentinel for Extract.
//
// A lazy backend build that fails with an error other than a context error
// (errors.Is reports neither context.Canceled nor context.DeadlineExceeded) is
// memorised while the context of its Extract call is still live: no later Extract
// builds again, BackendErr reports the failure, and Extract stays a no-op. A build
// that fails with a context error, or fails once the context of its Extract call
// is done, is an interruption: it is not memorised, BackendErr does not report it,
// and the next Extract builds again. A build that panics is not memorised either:
// the panic reaches the Extract call that started the build, and the next Extract
// builds again. A factory that keeps failing with a context error while its
// caller's context is live is therefore rebuilt by every Extract with no report:
// the extractor has no logger, and BackendErr does not show it.
func (e *LLMExtractor) Extract(ctx context.Context, journal, transcript string) ([]orchestrator.Candidate, error) {
	if strings.TrimSpace(journal) == "" && strings.TrimSpace(transcript) == "" {
		return nil, nil
	}
	backend := e.resolveBackend(ctx)
	if backend == nil {
		return nil, nil
	}
	raw, err := backend.Respond(ctx, extractionPrompt(journal, transcript), nil)
	if err != nil {
		return nil, fmt.Errorf("llmextractor: curation respond: %w", err)
	}
	return parseCandidates(raw, e.threshold, e.max), nil
}

// BackendErr returns the permanent error of the lazy backend creation, or nil when no backend failed permanently.
func (e *LLMExtractor) BackendErr() error {
	if e.newBackend == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.backendErr == nil {
		return nil
	}
	return fmt.Errorf("llmextractor: build curation backend: %w", e.backendErr)
}

func (e *LLMExtractor) resolveBackend(ctx context.Context) contracts.Backend {
	if e.newBackend == nil {
		return e.backend
	}
	for {
		e.mu.Lock()
		if e.built {
			backend := e.backend
			e.mu.Unlock()
			return backend
		}
		if e.building == nil {
			done := make(chan struct{})
			e.building = done
			e.mu.Unlock()
			return e.build(ctx, done)
		}
		done := e.building
		e.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil
		}
	}
}

func (e *LLMExtractor) build(ctx context.Context, done chan struct{}) contracts.Backend {
	completed := false
	defer func() {
		if !completed {
			e.abandon(done)
		}
	}()
	backend, err := e.newBackend(ctx)
	completed = true
	return e.settle(ctx, done, backend, err)
}

func (e *LLMExtractor) abandon(done chan struct{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.building = nil
	close(done)
}

func (e *LLMExtractor) settle(ctx context.Context, done chan struct{}, backend contracts.Backend, err error) contracts.Backend {
	if errors.Is(err, errNoBackend) {
		err = nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.building = nil
	close(done)
	switch {
	case err != nil && (ctx.Err() != nil || isContextError(err)):
		return nil
	case err != nil:
		e.backendErr, e.built = err, true
		return nil
	}
	e.backend, e.built = backend, true
	return backend
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
