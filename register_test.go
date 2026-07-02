package llmextractor

import (
	"context"
	"testing"

	"github.com/Herrscherd/herrscher-orchestrator"
)

// The registry is package-private in orchestrator; we verify registration by
// observing that newDefault produces a usable, lazily-backed extractor that
// no-ops cleanly with no backend registered.
func TestNewDefault_IsLazyAndNoOpsWithoutBackend(t *testing.T) {
	e := newDefault()
	if e.threshold != defaultThreshold || e.max != defaultMax {
		t.Fatalf("defaults not applied: %v %v", e.threshold, e.max)
	}
	if e.newBackend == nil {
		t.Fatal("registered default must have a lazy backend source")
	}
	// With no backend plugin registered in this test binary, lazyBackend yields
	// nil, so Extract is a clean no-op rather than a panic.
	cs, err := e.Extract(context.Background(), "journal", "transcript")
	if err != nil || cs != nil {
		t.Fatalf("want clean no-op, got (%v,%v)", cs, err)
	}
}

// compile-time proof the registered value satisfies the seam.
var _ orchestrator.Extractor = newDefault()
