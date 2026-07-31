package llmextractor

import (
	"context"
	"testing"

	"github.com/Herrscherd/herrscher-contracts"
	"github.com/Herrscherd/herrscher-orchestrator"
)

// memStub is a minimal in-memory Memory: enough for the Learner's Consolidate
// (Recall the session transcript; Record + Links candidates under scope roots).
type memStub struct {
	nodes map[string]contracts.Node
	links [][3]string // {from, to, rel}
}

func newMemStub(transcript string) *memStub {
	// NewScoped (inside NewLearner) prefixes "sessions/" to the session key,
	// so Consolidate's Recall will look up "sessions/sess-1", not "sess-1".
	return &memStub{nodes: map[string]contracts.Node{
		"sessions/sess-1": {Key: "sessions/sess-1", Kind: contracts.KindSession, Body: transcript},
	}}
}

func (m *memStub) Recall(_ context.Context, key string, _ int) (contracts.Subgraph, error) {
	return contracts.Subgraph{Root: m.nodes[key]}, nil
}
func (m *memStub) Record(_ context.Context, n contracts.Node) error {
	if m.nodes == nil {
		m.nodes = map[string]contracts.Node{}
	}
	m.nodes[n.Key] = n
	return nil
}
func (m *memStub) Search(context.Context, contracts.Query) ([]contracts.Node, error) {
	return nil, nil
}
func (m *memStub) Links(_ context.Context, from, to, rel string) error {
	m.links = append(m.links, [3]string{from, to, rel})
	return nil
}

// Unlink is the inverse of Links: identity is the (from, to) pair — no rel — so
// every relation targeting `to` goes. Idempotent, an absent edge is not an error.
func (m *memStub) Unlink(_ context.Context, from, to string) error {
	kept := m.links[:0]
	for _, l := range m.links {
		if l[0] != from || l[1] != to {
			kept = append(kept, l)
		}
	}
	m.links = kept
	return nil
}
func (m *memStub) Close() error { return nil }

func TestLearnerConsolidate_RecordsScopedNodes(t *testing.T) {
	mem := newMemStub("we decided to use NATS; agent learned a flock trick")
	scope := contracts.MemoryScope{Project: "projects/game", Agent: "agents/bob"}
	ex := New(&fakeBackend{reply: twoValid})

	// journal="" is fine: transcript alone is non-empty, so Extract runs.
	learner := orchestrator.NewLearner(mem, "sess-1", scope, ex, "/nonexistent/journal.log", 0)

	if err := learner.Consolidate(context.Background()); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	// Shared fact recorded and linked under the project root.
	if _, ok := mem.nodes["facts/decision/use-nats"]; !ok {
		t.Fatalf("shared fact not recorded; nodes: %v", keys(mem.nodes))
	}
	// Private skill recorded and linked under the agent root.
	if _, ok := mem.nodes["skills/learned-flock-trick"]; !ok {
		t.Fatalf("private skill not recorded; nodes: %v", keys(mem.nodes))
	}
	assertLink(t, mem.links, "projects/game", "facts/decision/use-nats", contracts.RelContains)
	assertLink(t, mem.links, "agents/bob", "skills/learned-flock-trick", contracts.RelContains)
}

func keys(m map[string]contracts.Node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func assertLink(t *testing.T, links [][3]string, from, to, rel string) {
	t.Helper()
	for _, l := range links {
		if l[0] == from && l[1] == to && l[2] == rel {
			return
		}
	}
	t.Fatalf("missing link %s -%s-> %s in %v", from, rel, to, links)
}
