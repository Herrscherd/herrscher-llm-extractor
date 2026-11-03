package llmextractor

import (
	"context"
	"errors"
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

var errNoBackend = errors.New("no curation backend registered")

func backendFrom(ctx context.Context, plugins []contracts.Plugin, getenv func(string) string) (contracts.Backend, error) {
	for index := range plugins {
		plugin := &plugins[index]
		if plugin.Backend == nil {
			continue
		}
		cfg, err := contracts.Resolve(plugin.Manifest.Config, getenv)
		if err != nil {
			return nil, err
		}
		return plugin.Backend(ctx, cfg)
	}
	return nil, errNoBackend
}

func lazyBackend(ctx context.Context) (contracts.Backend, error) {
	return backendFrom(ctx, contracts.Default.Backends(), curationEnv(os.Getenv))
}
