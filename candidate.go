package llmextractor

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	contracts "github.com/Herrscherd/herrscher-contracts"
	orchestrator "github.com/Herrscherd/herrscher-orchestrator"
)

const capturedBy = "llm-extractor"

const (
	maxCandidateScan     = 1 << 20
	maxCandidateAttempts = 64
)

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
		return nil
	}
	return out
}

func candidateArray(reply string) []rawCandidate {
	if len(reply) > maxCandidateScan {
		reply = reply[:maxCandidateScan]
	}
	attempts := 0
	for i := 0; i < len(reply); i++ {
		if reply[i] != '[' {
			continue
		}
		if attempts >= maxCandidateAttempts {
			return nil
		}
		attempts++
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

func mapKind(s string) contracts.NodeKind {
	k := contracts.NodeKind(strings.ToLower(strings.TrimSpace(s)))
	if allowedKinds[k] {
		return k
	}
	return contracts.KindSession
}

func stableKey(kind contracts.NodeKind, title string, private bool) string {
	prefix := "facts/" + string(kind)
	if private {
		prefix = "skills"
	}
	return prefix + "/" + slugOrHash(title)
}

func slugOrHash(title string) string {
	if s := slug(title); s != "" {
		return s
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(title)))
	return fmt.Sprintf("n%08x", h.Sum32())
}

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
