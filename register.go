package llmextractor

import "github.com/Herrscherd/herrscher-orchestrator"

// init registers the open reference extractor under "llm" so a host that blank-
// imports this package and passes `--extractor llm` opts into auto-capture. This
// is the xcaddy pattern: the orchestrator's register.go looks the name up at
// session construction.
func init() {
	orchestrator.RegisterExtractor("llm", newDefault())
}

// newDefault builds the registered extractor: tuning defaults, with its curation
// backend resolved lazily from the plugin registry on first use.
func newDefault() *LLMExtractor {
	return &LLMExtractor{newBackend: lazyBackend, threshold: defaultThreshold, max: defaultMax}
}
