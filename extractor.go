package llmextractor

import (
	"context"
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
	newBackend func() (contracts.Backend, error) // lazy source when backend is nil
	once       sync.Once
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
	b := e.resolveBackend()
	if b == nil || (strings.TrimSpace(journal) == "" && strings.TrimSpace(transcript) == "") {
		return nil, nil
	}
	raw, err := b.Respond(ctx, extractionPrompt(journal, transcript), nil)
	if err != nil {
		return nil, err
	}
	return parseCandidates(raw, e.threshold, e.max), nil
}

// resolveBackend returns the backend, building it lazily exactly once when a
// lazy source is set. A build error leaves the backend nil (no-op degrade).
func (e *LLMExtractor) resolveBackend() contracts.Backend {
	// Injected via New(): the field is set at construction and never mutated,
	// so this read needs no synchronization.
	if e.newBackend == nil {
		return e.backend
	}
	// Lazy: every caller goes through once.Do, whose internal barrier serializes
	// the write below and the subsequent read across concurrent sessions.
	e.once.Do(func() {
		if b, err := e.newBackend(); err == nil {
			e.backend = b
		}
	})
	return e.backend
}
