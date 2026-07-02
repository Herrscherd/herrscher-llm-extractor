package llmextractor

import (
	"context"
	"testing"

	"github.com/Herrscherd/herrscher-contracts"
)

func TestCurationEnv_OverridesModelKeysOnly(t *testing.T) {
	base := map[string]string{"CLAUDE_MODEL": "expensive", "CLAUDE_CMD": "claude"}
	env := curationEnv(func(k string) string {
		if k == "HERRSCHER_CURATION_MODEL" {
			return "cheap"
		}
		return base[k]
	})
	if got := env("CLAUDE_MODEL"); got != "cheap" {
		t.Fatalf("model key not overridden: %q", got)
	}
	if got := env("CLAUDE_CMD"); got != "claude" {
		t.Fatalf("non-model key changed: %q", got)
	}
}

func TestCurationEnv_NoOverridePassesThrough(t *testing.T) {
	env := curationEnv(func(k string) string {
		if k == "CLAUDE_MODEL" {
			return "default-model"
		}
		return ""
	})
	if got := env("CLAUDE_MODEL"); got != "default-model" {
		t.Fatalf("passthrough broken: %q", got)
	}
}

func TestBackendFrom_BuildsFirstBackendPlugin(t *testing.T) {
	built := &fakeBackend{}
	plugins := []contracts.Plugin{
		{Manifest: contracts.Manifest{Category: contracts.CategoryBackend, Config: []contracts.Setting{
			{Key: "model", Env: "CLAUDE_MODEL"},
		}}, Backend: func(_ context.Context, cfg contracts.PluginConfig) (contracts.Backend, error) {
			if cfg.Get("model") != "cheap" {
				t.Fatalf("curation model not resolved into cfg: %q", cfg.Get("model"))
			}
			return built, nil
		}},
	}
	env := curationEnv(func(k string) string {
		if k == "HERRSCHER_CURATION_MODEL" {
			return "cheap"
		}
		return ""
	})
	b, err := backendFrom(plugins, env)
	if err != nil {
		t.Fatalf("backendFrom: %v", err)
	}
	if b != built {
		t.Fatal("did not return the built backend")
	}
}

func TestBackendFrom_NoBackendRegisteredIsNoOp(t *testing.T) {
	b, err := backendFrom(nil, curationEnv(func(string) string { return "" }))
	if b != nil || err != nil {
		t.Fatalf("want (nil,nil), got (%v,%v)", b, err)
	}
}

func TestBackendFrom_SkipsPluginsWithoutBackendFactory(t *testing.T) {
	built := &fakeBackend{}
	plugins := []contracts.Plugin{
		{Manifest: contracts.Manifest{Category: contracts.CategoryBackend}}, // no Backend factory
		{Manifest: contracts.Manifest{Category: contracts.CategoryBackend}, Backend: func(_ context.Context, _ contracts.PluginConfig) (contracts.Backend, error) {
			return built, nil
		}},
	}
	b, err := backendFrom(plugins, func(string) string { return "" })
	if err != nil || b != built {
		t.Fatalf("want built backend via skip, got (%v,%v)", b, err)
	}
}

func TestBackendFrom_ResolveErrorPropagates(t *testing.T) {
	plugins := []contracts.Plugin{
		{Manifest: contracts.Manifest{Category: contracts.CategoryBackend, Config: []contracts.Setting{
			{Key: "model", Env: "CLAUDE_MODEL", Required: true},
		}}, Backend: func(_ context.Context, _ contracts.PluginConfig) (contracts.Backend, error) {
			return &fakeBackend{}, nil
		}},
	}
	b, err := backendFrom(plugins, func(string) string { return "" }) // required env unset → Resolve error
	if err == nil || b != nil {
		t.Fatalf("want (nil, error) on unresolved required setting, got (%v,%v)", b, err)
	}
}
