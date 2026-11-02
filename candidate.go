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

func parseCandidates(reply string, threshold float64, limit int) []orchestrator.Candidate {
	raws := candidateArray(reply)
	count := len(raws)
	if limit > 0 && limit < count {
		count = limit
	}
	out := make([]orchestrator.Candidate, 0, count)
	for index := range raws {
		raw := &raws[index]
		title := strings.TrimSpace(raw.Title)
		if title == "" || raw.Confidence < threshold {
			continue
		}
		out = append(out, orchestrator.Candidate{Node: toNode(*raw, title), Private: raw.Private})
		if limit > 0 && len(out) >= limit {
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
	for start := range len(reply) {
		if reply[start] != '[' {
			continue
		}
		if attempts >= maxCandidateAttempts {
			return nil
		}
		attempts++
		var raws []rawCandidate
		if err := json.NewDecoder(strings.NewReader(reply[start:])).Decode(&raws); err != nil {
			continue
		}
		for index := range raws {
			if strings.TrimSpace(raws[index].Title) != "" {
				return raws
			}
		}
	}
	return nil
}

func toNode(raw rawCandidate, title string) contracts.Node {
	meta := map[string]string{"capturedBy": capturedBy}
	if d := strings.TrimSpace(raw.Domain); d != "" {
		meta["domain"] = d
	}
	if len(raw.Tags) > 0 {
		meta["tags"] = strings.Join(raw.Tags, ",")
	}
	var links []contracts.Link
	for _, link := range raw.Links {
		target := strings.TrimSpace(link.To)
		if target == "" {
			continue
		}
		rel := strings.TrimSpace(link.Rel)
		if rel == "" {
			rel = contracts.RelAppliesTo
		}
		links = append(links, contracts.Link{To: target, Rel: rel})
	}
	kind := mapKind(raw.Kind)
	return contracts.Node{
		Key:   stableKey(kind, title, raw.Private),
		Kind:  kind,
		Title: title,
		Body:  raw.Body,
		Links: links,
		Meta:  meta,
	}
}

func allowedKinds() map[contracts.NodeKind]bool {
	return map[contracts.NodeKind]bool{
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
		contracts.KindSession:      false,
		contracts.KindTranscript:   false,
	}
}

func mapKind(s string) contracts.NodeKind {
	kind := contracts.NodeKind(strings.ToLower(strings.TrimSpace(s)))
	if allowedKinds()[kind] {
		return kind
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
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.ToLower(title)))
	return fmt.Sprintf("n%08x", hasher.Sum32())
}

func slug(text string) string {
	var out []byte
	prevDash := false
	for _, char := range strings.ToLower(text) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			out = append(out, byte(char))
			prevDash = false
		default:
			if !prevDash {
				out = append(out, '-')
				prevDash = true
			}
		}
	}
	return strings.Trim(string(out), "-")
}
