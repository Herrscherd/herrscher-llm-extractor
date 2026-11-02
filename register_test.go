package llmextractor

import (
	"context"
	"testing"

	orchestrator "github.com/Herrscherd/herrscher-orchestrator"
)

func TestNewDefault_IsLazyAndNoOpsWithoutBackend(t *testing.T) {
	e := newDefault()
	if e.threshold != defaultThreshold || e.max != defaultMax {
		t.Fatalf("defaults not applied: %v %v", e.threshold, e.max)
	}
	if e.newBackend == nil {
		t.Fatal("registered default must have a lazy backend source")
	}
	cs, err := e.Extract(context.Background(), "journal", "transcript")
	if err != nil || cs != nil {
		t.Fatalf("want clean no-op, got (%v,%v)", cs, err)
	}
}

var _ orchestrator.Extractor = newDefault()
