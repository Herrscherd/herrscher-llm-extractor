package llmextractor

import (
	"context"
	"os"
	"strings"

	contracts "github.com/Herrscherd/herrscher-contracts"
)

func curationEnv(base func(string) string) func(string) string {
	override := base("HERRSCHER_CURATION_MODEL")
	return func(key string) string {
		if override != "" && strings.HasSuffix(key, "MODEL") {
			return override
		}
		return base(key)
	}
}

func backendFrom(ctx context.Context, plugins []contracts.Plugin, getenv func(string) string) (contracts.Backend, error) {
	for _, p := range plugins {
		if p.Backend == nil {
			continue
		}
		cfg, err := contracts.Resolve(p.Manifest.Config, getenv)
		if err != nil {
			return nil, err
		}
		return p.Backend(ctx, cfg)
	}
	return nil, nil
}

func lazyBackend(ctx context.Context) (contracts.Backend, error) {
	return backendFrom(ctx, contracts.Default.Backends(), curationEnv(os.Getenv))
}
