package llmextractor

import (
	"context"
	"os"
	"strings"

	"github.com/Herrscherd/herrscher-contracts"
)

// curationEnv wraps a base getenv so any model key resolves to
// HERRSCHER_CURATION_MODEL when that override is set — letting curation run on a
// cheaper/faster model than the conversation backend. Every other key passes
// through unchanged. A model key is any env var whose name ends in "MODEL"
// (e.g. CLAUDE_MODEL), matching how backends name their model setting.
func curationEnv(base func(string) string) func(string) string {
	override := base("HERRSCHER_CURATION_MODEL")
	return func(key string) string {
		if override != "" && strings.HasSuffix(key, "MODEL") {
			return override
		}
		return base(key)
	}
}

// backendFrom builds the first backend plugin in plugins from its resolved env
// config, mirroring the host's firstBackend (Resolve(manifest.Config, getenv) →
// factory). Returns (nil, nil) when no backend plugin is registered, so curation
// degrades to a clean no-op and recall keeps working.
func backendFrom(plugins []contracts.Plugin, getenv func(string) string) (contracts.Backend, error) {
	for _, p := range plugins {
		if p.Backend == nil {
			continue
		}
		cfg, err := contracts.Resolve(p.Manifest.Config, getenv)
		if err != nil {
			return nil, err
		}
		return p.Backend(context.Background(), cfg)
	}
	return nil, nil
}

// lazyBackend builds a curation backend from the global plugin registry using a
// model-overriding view of the process environment. Used by the registered
// default extractor on first Extract.
func lazyBackend() (contracts.Backend, error) {
	return backendFrom(contracts.Default.Backends(), curationEnv(os.Getenv))
}
