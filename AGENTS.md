# AGENTS.md — ai-filter

MuxCore sidecar module (`ai-filter`).

## Module identity

| Field | Value |
|-------|-------|
| Directory | `ai-filter` |
| Role | `ai` |
| Capability | `ai.filter` |

## Build

```bash
cd ai-filter
go test ./...
make lint
```

## Agent rules

- Modules run as gRPC sidecars; capabilities are the security boundary.
- TLS required in production (`MUXCORE_INSECURE_DISABLE_TLS` is dev-only).
- Match existing Go patterns; run `gofmt` and package tests before finishing.
- Roadmaps, task lists, and remaining-work checklists live in workspace [`MASTER-ROADMAP.md`](../MASTER-ROADMAP.md) and umbrella GitHub Issues. Do not add `ROADMAP.md` / `TASKS.md` in this repo.
