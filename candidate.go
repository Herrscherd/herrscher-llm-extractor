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

// parseCandidates tolerantly turns a model reply into candidates: it extracts the
// JSON array (ignoring any prose or code-fence wrapper), drops entries with no
// title or below threshold, caps the result at max (0 = uncapped), and never
// errors — a garbage reply yields nil so Consolidate stays best-effort.
func parseCandidates(reply string, threshold float64, max int) []orchestrator.Candidate {
	arr := extractJSONArray(reply)
	if arr == "" {
		return nil
	}
	var raws []rawCandidate
	if err := json.Unmarshal([]byte(arr), &raws); err != nil {
		return nil
	}
	var out []orchestrator.Candidate
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
	return out
}

// extractJSONArray returns the substring from the first '[' to the last ']'
// inclusive, so a fenced or prose-wrapped array still parses. Empty when absent.
func extractJSONArray(s string) string {
	i := strings.IndexByte(s, '[')
	j := strings.LastIndexByte(s, ']')
	if i < 0 || j < i {
		return ""
	}
	return s[i : j+1]
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

// mapKind maps the model's kind string to a NodeKind, defaulting unknown or blank
// to KindSession (transient) rather than dropping the candidate.
func mapKind(s string) contracts.NodeKind {
	switch contracts.NodeKind(strings.ToLower(strings.TrimSpace(s))) {
	case contracts.KindOrganization:
		return contracts.KindOrganization
	case contracts.KindProject:
		return contracts.KindProject
	case contracts.KindRepo:
		return contracts.KindRepo
	case contracts.KindServer:
		return contracts.KindServer
	case contracts.KindArchitecture:
		return contracts.KindArchitecture
	case contracts.KindProduction:
		return contracts.KindProduction
	case contracts.KindDecision:
		return contracts.KindDecision
	case contracts.KindUser:
		return contracts.KindUser
	case contracts.KindAgent:
		return contracts.KindAgent
	case contracts.KindDomain:
		return contracts.KindDomain
	default:
		return contracts.KindSession
	}
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
