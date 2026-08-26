# herrscher-llm-extractor

**The open, reference memory curator.** A generic, LLM-driven
`orchestrator.Extractor`. It turns a session's call journal and transcript into
durable memory nodes: shared **facts** under the project, and private **skills**
under the agent. That is how herrscher self-populates its vault.

It is not the nudge loop. The every-N-turns `Consolidate` is owned by the
orchestrator, which calls this extractor.

Domain-specific curation heuristics live in separate, closed extractors. This is
the reusable default that ships in the open.

Status: live.

## What it is, in the plugin model

It is a library rather than a plugin: it registers no `contracts.Plugin`, it
extends the orchestrator.

It registers itself as `orchestrator.RegisterExtractor("llm", …)`, so a session
selects it with `--extractor llm`.

It implements `orchestrator.Extractor`, which is
`Extract(ctx, journal, transcript) ([]orchestrator.Candidate, error)`.

It consumes `contracts.Backend`: the first backend plugin in
`contracts.Default.Backends()`, built lazily on the first `Extract`.

## Install

```bash
herrscher plugin add github.com/Herrscherd/herrscher-llm-extractor
```

Blank-import the package into a herrscher host, the xcaddy pattern:

```go
import _ "github.com/Herrscherd/herrscher-llm-extractor"
```

Then opt a session into auto-capture:

```bash
session create --extractor llm --journal <worktree>/.neublox/calls.log --consolidate-every 10
```

## Configuration

`HERRSCHER_CURATION_MODEL` is optional. It overrides *any* env key ending in
`MODEL` that the registered backend reads, so curation can run on a cheaper model
than the conversation does. Every other key of that backend's own manifest config
(API keys, endpoints) is read unchanged from the process environment.

Two knobs have no environment equivalent and are set in code:
`New(backend, WithThreshold(f), WithMax(n))`. The confidence threshold defaults to
`0.6`, and the number of candidates kept per pass defaults to `8`.

## What it writes

Each candidate becomes a `contracts.Node` with a **stable key**:
`facts/<kind>/<slug>` for shared facts, `skills/<slug>` for agent-private ones. So
re-extraction upserts instead of duplicating. A title that slugs to nothing falls
back to a deterministic FNV hash rather than colliding on an empty key.

`Meta["capturedBy"]="llm-extractor"` marks every node for human audit and pruning.
`domain` and `tags` are carried through when the model supplies them. A kind
outside the allowed set degrades to `session` instead of dropping the candidate.

## Failure behaviour

Extraction is best-effort and never breaks a session. No registered backend, an
empty journal *and* transcript, a backend build error, or a malformed JSON reply
all yield zero candidates and no error.

The journal and the transcript are fenced between a per-call random sentinel and
declared untrusted, so instructions embedded in captured output cannot hijack the
curation prompt.

## Further reading

- [Herrscher docs](https://github.com/Herrscherd/herrscher-docs), page
  `architecture/learning`
- [contracts](https://github.com/Herrscherd/herrscher-contracts), for the port
  signatures
- [orchestrator](https://github.com/Herrscherd/herrscher-orchestrator), the
  `Learner` that drives this extractor
