# herrscher-llm-extractor

**The open, reference memory curator.** A generic, LLM-driven
`orchestrator.Extractor`: it turns a session's call journal and transcript into
durable memory nodes — shared **facts** (under the project) and private
**skills** (under the agent) — so herrscher self-populates its vault. It is not
the nudge loop: the every-N-turns `Consolidate` is owned by the orchestrator,
which calls this extractor. The Roblox-specific curation heuristics are a
separate, **closed** extractor; this is the reusable default that ships in the open.

## Role · Category · Ports · Config · Status · Repo

| Aspect | Value |
|--------|-------|
| **Role** | Distills a session's journal and transcript into memory candidates |
| **Category** | Library (orchestrator extension — it registers no `contracts.Plugin`) |
| **Registered as** | `orchestrator.RegisterExtractor("llm", …)` — selected with `--extractor llm` |
| **Ports implemented** | `orchestrator.Extractor` (`Extract(ctx, journal, transcript) ([]orchestrator.Candidate, error)`) |
| **Ports consumed** | `contracts.Backend` — the first backend plugin in `contracts.Default.Backends()`, built lazily on first `Extract` |
| **Config & env** | `HERRSCHER_CURATION_MODEL` (optional) — overrides *any* env key ending in `MODEL` that the registered backend reads, so curation runs on a cheaper model than the conversation. Every other key of that backend's own manifest config (API keys, endpoints) is read unchanged from the process env |
| **Code-level tuning** | `New(backend, WithThreshold(f), WithMax(n))` — confidence threshold (default `0.6`) and candidates kept per pass (default `8`). No env equivalent |
| **Dependencies** | contracts `v0.1.9`, orchestrator `v0.1.4` |
| **Status** | live |
| **Repo** | [herrscher-llm-extractor](https://github.com/Herrscherd/herrscher-llm-extractor) |

## Install

```bash
herrscher plugin add github.com/Herrscherd/herrscher-llm-extractor
```

Blank-import the package into a herrscher host (xcaddy pattern):

```go
import _ "github.com/Herrscherd/herrscher-llm-extractor"
```

Then opt a session into auto-capture:

```bash
session create --extractor llm --journal <worktree>/.neublox/calls.log --consolidate-every 10
```

## What it writes

Each candidate becomes a `contracts.Node` with a **stable Key** —
`facts/<kind>/<slug>` for shared facts, `skills/<slug>` for agent-private ones —
so re-extraction upserts instead of duplicating. Titles that slug to nothing fall
back to a deterministic FNV hash rather than colliding on an empty key.
`Meta["capturedBy"]="llm-extractor"` marks every node for human audit and
pruning; `domain` and `tags` are carried through when the model supplies them. A
kind outside the allowed set degrades to `session` instead of dropping the
candidate.

## Failure behaviour

Extraction is best-effort and never breaks a session: no registered backend,
empty journal *and* transcript, a backend build error, or a malformed JSON reply
all yield zero candidates without an error. Journal and transcript are fenced
between a per-call random sentinel and declared untrusted, so instructions
embedded in captured output cannot hijack the curation prompt.

## Further reading

- [Herrscher docs](https://github.com/Herrscherd/herrscher-docs) — `architecture/learning`
- [contracts](https://github.com/Herrscherd/herrscher-contracts) — port signatures
- [orchestrator](https://github.com/Herrscherd/herrscher-orchestrator) — the `Learner` that drives this extractor
