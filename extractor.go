package llmextractor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Herrscherd/herrscher-contracts"
	"github.com/Herrscherd/herrscher-orchestrator"
)

// LLMExtractor is the open, reference Extractor: it asks a contracts.Backend to
// distill a stretch of work (journal + transcript) into memory candidates. The
// backend is either injected (New) or built lazily from the plugin registry on
// first use (the registered default — see register.go / backend.go).
type LLMExtractor struct {
	backend    contracts.Backend
	newBackend func(context.Context) (contracts.Backend, error)
	mu         sync.Mutex
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

// WithMax caps candidates recorded per Consolidate (0 = uncapped; default 8).
func WithMax(m int) Option { return func(e *LLMExtractor) { e.max = m } }

// New builds an extractor over an explicit backend (tests and callers that
// already hold a model edge).
func New(b contracts.Backend, opts ...Option) *LLMExtractor {
	e := &LLMExtractor{backend: b, threshold: defaultThreshold, max: defaultMax}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Extract asks the curation backend to distill journal + transcript into
// candidates. It is best-effort: no backend or empty inputs yield a clean no-op;
// a bad JSON reply yields no candidates without erroring.
func (e *LLMExtractor) Extract(ctx context.Context, journal, transcript string) ([]orchestrator.Candidate, error) {
	if strings.TrimSpace(journal) == "" && strings.TrimSpace(transcript) == "" {
		return nil, nil
	}
	b := e.resolveBackend(ctx)
	if b == nil {
		return nil, nil
	}
	raw, err := b.Respond(ctx, extractionPrompt(journal, transcript), nil)
	if err != nil {
		return nil, fmt.Errorf("llmextractor: curation respond: %w", err)
	}
	return parseCandidates(raw, e.threshold, e.max), nil
}

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
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.built {
		return e.backend
	}
	b, err := e.newBackend(ctx)
	e.backendErr = err
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		e.built = true
		return nil
	}
	e.backend, e.built = b, true
	return e.backend
}
