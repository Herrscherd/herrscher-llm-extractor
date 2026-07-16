package llmextractor

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/Herrscherd/herrscher-contracts"
	"github.com/Herrscherd/herrscher-orchestrator"
)

// capturedBy stamps every auto-recorded node so human curators can audit and
// prune what the extractor wrote.
const capturedBy = "llm-extractor"

type rawLink struct {
	To  string `json:"to"`
	Rel string `json:"rel"`
}

type rawCandidate struct {
	Kind       string    `json:"kind"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Domain     string    `json:"domain"`
	Tags       []string  `json:"tags"`
	Links      []rawLink `json:"links"`
	Private    bool      `json:"private"`
	Confidence float64   `json:"confidence"`
}

// parseCandidates tolerantly turns a model reply into candidates: it decodes the
// first embedded JSON array (ignoring any prose or code-fence wrapper), drops
// entries with no title or below threshold, caps the result at max (0 = uncapped),
// and never errors — a garbage reply yields nil so Consolidate stays best-effort.
func parseCandidates(reply string, threshold float64, max int) []orchestrator.Candidate {
	raws := candidateArray(reply)
	n := len(raws)
	if max > 0 && max < n {
		n = max
	}
	out := make([]orchestrator.Candidate, 0, n)
	for _, r := range raws {
		title := strings.TrimSpace(r.Title)
		if title == "" || r.Confidence < threshold {
			continue
		}
		out = append(out, orchestrator.Candidate{Node: toNode(r, title), Private: r.Private})
		if max > 0 && len(out) >= max {
			break
		}
	}
	if len(out) == 0 {
		return nil // keep the documented nil-on-garbage contract, not a 0-len slice
	}
	return out
}

// candidateArray decodes the first JSON array in reply that looks like candidates —
// one with at least one titled element. It scans from each '[' with a json.Decoder,
// which reads a single value and ignores trailing text, so an array wrapped in prose
// still parses even when the surrounding prose contains its own brackets. Requiring a
// titled element means a stray title-less array such as `[]` or `[{}]` appearing in
// the prose before the real one no longer masks it. Returns nil when no such array
// decodes, keeping Consolidate best-effort on garbage or truncated replies.
func candidateArray(reply string) []rawCandidate {
	for i := 0; i < len(reply); i++ {
		if reply[i] != '[' {
			continue
		}
		var raws []rawCandidate
		if err := json.NewDecoder(strings.NewReader(reply[i:])).Decode(&raws); err != nil {
			continue
		}
		for _, r := range raws {
			if strings.TrimSpace(r.Title) != "" {
				return raws
			}
		}
	}
	return nil
}

func toNode(r rawCandidate, title string) contracts.Node {
	meta := map[string]string{"capturedBy": capturedBy}
	if d := strings.TrimSpace(r.Domain); d != "" {
		meta["domain"] = d
	}
	if len(r.Tags) > 0 {
		meta["tags"] = strings.Join(r.Tags, ",")
	}
	var links []contracts.Link
	for _, l := range r.Links {
		to := strings.TrimSpace(l.To)
		if to == "" {
			continue
		}
		rel := strings.TrimSpace(l.Rel)
		if rel == "" {
			rel = contracts.RelAppliesTo
		}
		links = append(links, contracts.Link{To: to, Rel: rel})
	}
	kind := mapKind(r.Kind)
	return contracts.Node{
		Key:   stableKey(kind, title, r.Private),
		Kind:  kind,
		Title: title,
		Body:  r.Body,
		Links: links,
		Meta:  meta,
	}
}

// allowedKinds is the set of NodeKinds the model may name directly; anything else
// (including blank) falls through to KindSession. Kept as one table so adding a
// contracts.NodeKind is a single-line change rather than a new switch arm.
var allowedKinds = map[contracts.NodeKind]bool{
	contracts.KindOrganization: true,
	contracts.KindProject:      true,
	contracts.KindRepo:         true,
	contracts.KindServer:       true,
	contracts.KindArchitecture: true,
	contracts.KindProduction:   true,
	contracts.KindDecision:     true,
	contracts.KindUser:         true,
	contracts.KindAgent:        true,
	contracts.KindDomain:       true,
}

// mapKind maps the model's kind string to a NodeKind, defaulting unknown or blank
// to KindSession (transient) rather than dropping the candidate.
func mapKind(s string) contracts.NodeKind {
	k := contracts.NodeKind(strings.ToLower(strings.TrimSpace(s)))
	if allowedKinds[k] {
		return k
	}
	return contracts.KindSession
}

// stableKey derives a deterministic Key so the same fact re-extracted in a later
// session upserts by Key instead of duplicating. Shared facts live under facts/,
// private skills under skills/.
func stableKey(kind contracts.NodeKind, title string, private bool) string {
	prefix := "facts/" + string(kind)
	if private {
		prefix = "skills"
	}
	return prefix + "/" + slugOrHash(title)
}

// slugOrHash returns the readable slug of title, or a deterministic hash fallback
// when the title has no [a-z0-9] runes (non-ASCII or punctuation-only). Without
// the fallback such titles slug to "" and each collides on a degenerate key like
// "facts/decision/", silently overwriting distinct facts on upsert.
func slugOrHash(title string) string {
	if s := slug(title); s != "" {
		return s
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(title)))
	return fmt.Sprintf("n%08x", h.Sum32())
}

// slug lowercases title and keeps [a-z0-9], collapsing every other run into a
// single hyphen, so a Key is filesystem- and wikilink-safe.
func slug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
