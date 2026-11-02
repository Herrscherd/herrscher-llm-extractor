package llmextractor

import orchestrator "github.com/Herrscherd/herrscher-orchestrator"

func init() {
	orchestrator.RegisterExtractor("llm", newDefault())
}

func newDefault() *LLMExtractor {
	return &LLMExtractor{newBackend: lazyBackend, threshold: defaultThreshold, max: defaultMax}
}
