package llmextractor

import (
	"strings"

	"github.com/Herrscherd/herrscher-contracts"
)

// extractionPrompt builds the curation prompt: it hands the model the call
// journal and session transcript and asks for a JSON array of memory candidates,
// mirroring herrscher's own memory-writing guidance (one fact per node,
// Why/How-to-apply for decisions, link liberally, mark agent-private skills).
func extractionPrompt(journal, transcript string) contracts.Prompt {
	var b strings.Builder
	b.WriteString(instructions)
	b.WriteString("\n\n## Call journal\n\n")
	b.WriteString(journal)
	b.WriteString("\n\n## Session transcript\n\n")
	b.WriteString(transcript)
	return contracts.Prompt{Content: b.String()}
}

const instructions = `You are herrscher's memory curator. From the work below,
distill only what is worth remembering for future sessions. Return ONLY a JSON array (no prose, no code fence) of candidate memory nodes:

[{"kind":"decision|architecture|user|production|organization|project|repo|server|agent|domain|session",
  "title":"short stable title",
  "body":"the fact in markdown; for a decision add **Why:** and **How to apply:**",
  "domain":"dev",
  "tags":["nats","transport"],
  "links":[{"to":"projects/x/index","rel":"applies-to"}],
  "private":false,
  "confidence":0.9}]

Rules:
- One durable fact per element; concise, stable titles.
- private=true for a skill this agent learned; private=false for a fact every
  agent of the project should share.
- confidence is 0..1; omit anything you are not sure is durable.
- Prefer decisions, architecture, user preferences, production facts over
  transient chatter.
- If nothing is worth remembering, return [].`
