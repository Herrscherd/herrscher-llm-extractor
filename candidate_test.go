package llmextractor

import (
	"strings"
	"testing"

	"github.com/Herrscherd/herrscher-contracts"
)

const twoValid = `[
 {"kind":"decision","title":"Use NATS","body":"**Why:** decoupling","tags":["nats","transport"],"private":false,"confidence":0.9},
 {"kind":"session","title":"Learned flock trick","body":"skill","private":true,"confidence":0.8}
]`

func TestParseCandidates_MapsFieldsAndScope(t *testing.T) {
	cs := parseCandidates(twoValid, 0.6, 0)
	if len(cs) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(cs))
	}
	if cs[0].Private || !cs[1].Private {
		t.Fatalf("scope routing wrong: %v / %v", cs[0].Private, cs[1].Private)
	}
	if cs[0].Node.Kind != contracts.KindDecision {
		t.Fatalf("kind: got %q", cs[0].Node.Kind)
	}
	if cs[0].Node.Meta["capturedBy"] != "llm-extractor" {
		t.Fatalf("provenance missing: %v", cs[0].Node.Meta)
	}
	if cs[0].Node.Meta["tags"] != "nats,transport" {
		t.Fatalf("tags: got %q", cs[0].Node.Meta["tags"])
	}
}

func TestParseCandidates_StableKeyIsDeterministic(t *testing.T) {
	a := parseCandidates(twoValid, 0.6, 0)
	b := parseCandidates(twoValid, 0.6, 0)
	if a[0].Node.Key != b[0].Node.Key || a[0].Node.Key == "" {
		t.Fatalf("keys not stable: %q vs %q", a[0].Node.Key, b[0].Node.Key)
	}
	if a[0].Node.Key != "facts/decision/use-nats" {
		t.Fatalf("shared key shape: got %q", a[0].Node.Key)
	}
	if a[1].Node.Key != "skills/learned-flock-trick" {
		t.Fatalf("private key shape: got %q", a[1].Node.Key)
	}
}

func TestParseCandidates_DropsBelowThresholdAndCaps(t *testing.T) {
	if got := parseCandidates(twoValid, 0.85, 0); len(got) != 1 {
		t.Fatalf("threshold: want 1 (0.9 kept, 0.8 dropped), got %d", len(got))
	}
	if got := parseCandidates(twoValid, 0.0, 1); len(got) != 1 {
		t.Fatalf("cap: want 1, got %d", len(got))
	}
}

func TestParseCandidates_TolerantOfWrapperAndGarbage(t *testing.T) {
	fenced := "Sure! Here you go:\n```json\n" + twoValid + "\n```\nHope that helps."
	if got := parseCandidates(fenced, 0.6, 0); len(got) != 2 {
		t.Fatalf("fenced/prose-wrapped: want 2, got %d", len(got))
	}
	if got := parseCandidates("not json at all", 0.6, 0); got != nil {
		t.Fatalf("garbage: want nil, got %v", got)
	}
	if got := parseCandidates(`[{"title":"","confidence":1}]`, 0.6, 0); got != nil {
		t.Fatalf("empty title: want nil, got %v", got)
	}
	if got := parseCandidates("[ {kind: decision, title: x} ]", 0.6, 0); got != nil {
		t.Fatalf("bracketed-but-invalid array: want nil, got %v", got)
	}
	if got := parseCandidates(`[{"title":"x"`, 0.6, 0); got != nil {
		t.Fatalf("truncated array: want nil, got %v", got)
	}
}

func TestParseCandidates_IgnoresBracketsInProse(t *testing.T) {
	// A valid array wrapped in prose that itself contains brackets must still extract.
	reply := "Here are the facts [as requested]:\n" + twoValid + "\nHope this helps ]"
	cs := parseCandidates(reply, 0.6, 0)
	if len(cs) != 2 {
		t.Fatalf("bracketed-prose wrapper: want 2 candidates, got %d", len(cs))
	}
}

func TestParseCandidates_StampsDomainMeta(t *testing.T) {
	in := `[{"kind":"decision","title":"Has domain","confidence":1,"domain":"dev"},
	 {"kind":"decision","title":"Blank domain","confidence":1,"domain":"  "}]`
	cs := parseCandidates(in, 0.6, 0)
	if len(cs) != 2 {
		t.Fatalf("want 2, got %d", len(cs))
	}
	if cs[0].Node.Meta["domain"] != "dev" {
		t.Fatalf("domain meta not stamped: %v", cs[0].Node.Meta)
	}
	if _, ok := cs[1].Node.Meta["domain"]; ok {
		t.Fatalf("whitespace-only domain should be skipped: %v", cs[1].Node.Meta)
	}
}

func TestParseCandidates_NormalizesLinks(t *testing.T) {
	in := `[{"kind":"decision","title":"Linked","confidence":1,"links":[
	 {"to":"projects/x/index"},
	 {"to":"","rel":"contains"},
	 {"to":"y","rel":"relates-to"},
	 {"to":"  z  ","rel":"  "}]}]`
	cs := parseCandidates(in, 0.6, 0)
	if len(cs) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cs))
	}
	links := cs[0].Node.Links
	if len(links) != 3 {
		t.Fatalf("want 3 links (empty-To dropped), got %d", len(links))
	}
	if links[0].To != "projects/x/index" || links[0].Rel != contracts.RelAppliesTo {
		t.Fatalf("default rel not applied: %+v", links[0])
	}
	if links[1].To != "y" || links[1].Rel != "relates-to" {
		t.Fatalf("explicit rel not preserved: %+v", links[1])
	}
	if links[2].To != "z" || links[2].Rel != contracts.RelAppliesTo {
		t.Fatalf("whitespace not trimmed / rel default: %+v", links[2])
	}
}

func TestStableKey_NonSluggableTitlesGetDistinctKeys(t *testing.T) {
	in := `[{"kind":"decision","title":"決定事項","confidence":1},
	 {"kind":"decision","title":"日本語","confidence":1},
	 {"kind":"decision","title":"!!!","confidence":1}]`
	cs := parseCandidates(in, 0.6, 0)
	if len(cs) != 3 {
		t.Fatalf("want 3, got %d", len(cs))
	}
	seen := map[string]bool{}
	for _, c := range cs {
		k := c.Node.Key
		if strings.HasSuffix(k, "/") {
			t.Fatalf("degenerate key with empty slug: %q", k)
		}
		if seen[k] {
			t.Fatalf("colliding key for distinct title: %q", k)
		}
		seen[k] = true
	}
}

func TestMapKind_UnknownFallsBackToSession(t *testing.T) {
	if mapKind("architecture") != contracts.KindArchitecture {
		t.Fatal("known kind lost")
	}
	if mapKind("wibble") != contracts.KindSession {
		t.Fatal("unknown kind should fall back to session")
	}
}
