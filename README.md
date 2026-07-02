# herrscher-llm-extractor

The open, reference **memory curator** for [herrscher](https://github.com/Herrscherd/herrscher):
a generic, LLM-driven `orchestrator.Extractor`. It turns a session's call journal
and transcript into durable memory nodes — shared **facts** (under the project)
and private **skills** (under the agent) — so herrscher self-populates its vault.

## Wire it in

Blank-import the package into a herrscher host (xcaddy pattern):

    import _ "github.com/Herrscherd/herrscher-llm-extractor"

Then opt a session into auto-capture:

    session create --extractor llm --journal <worktree>/.neublox/calls.log --consolidate-every 10

The nudge (every-N-turns `Consolidate`) is owned by the
[orchestrator](https://github.com/Herrscherd/herrscher-orchestrator); this package
only supplies the extractor it calls.

## Config

- `HERRSCHER_CURATION_MODEL` (optional) — run curation on a cheaper/faster model
  than the conversation backend. Overrides any `*_MODEL` env key the registered
  backend reads.

## What it writes

Each candidate becomes a `contracts.Node` with a **stable Key**
(`facts/<kind>/<slug>` or `skills/<slug>`) so re-extraction upserts instead of
duplicating, and `Meta["capturedBy"]="llm-extractor"` for human audit and pruning.

The Roblox-specific curation heuristics are a separate, **closed** extractor; this
is the reusable default that ships in the open.
