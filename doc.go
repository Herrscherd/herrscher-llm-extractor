// Package llmextractor is herrscher's open, reference memory curator: a generic
// LLM-driven orchestrator.Extractor. A blank import registers it under "llm"; the
// host then opts a session into auto-capture with
// `session create --extractor llm --journal <path> --consolidate-every N`.
//
// The Roblox-specific curation heuristics are a separate, closed extractor; this
// package is the reusable default that ships in the open.
package llmextractor
