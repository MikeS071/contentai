# Architecture

## Overview

ContentAI is a modular CLI built around command handlers (`cmd/`) and domain packages (`internal/`).

Core design points:

- local-first filesystem storage (`content/`)
- explicit lifecycle transitions via metadata status
- external integrations behind package-level abstractions
- prompt templates embedded and optionally overridden locally

## Package Structure

### CLI Layer (`cmd/`)

- Constructs Cobra commands.
- Loads config and dependencies for each command.
- Delegates to domain services in `internal/*`.

Primary commands:

- `init`, `kb`, `ideas`, `new`, `draft`, `qa`, `hero`, `publish`, `social`, `schedule`

### Domain Packages (`internal/`)

- `internal/config`: TOML config model, defaults, validation.
- `internal/content`: filesystem-backed content store and status transitions.
- `internal/init`: initialization wizard and feed bootstrap.
- `internal/kb`: feed registry, RSS/Atom sync, KB search.
- `internal/ideas`: outline generation and batch persistence.
- `internal/draft`: prompt-chain drafting pipeline.
- `internal/qa`: rule checks and auto-fix workflow.
- `internal/hero`: image generation, overlays, and resizing.
- `internal/publish`: publish service and adapters (`http`, `static`).
- `internal/social`: social copy generation and persistence.
- `internal/llm`: provider clients (OpenAI/Anthropic) and prompt context assembly.
- `internal/templates`: embedded templates + local override export/lookup.

## Data Model and Storage

Default project content root: `content/`.

Key files/directories:

- `content/voice.md`
- `content/blueprint.md`
- `content/examples/*.md`
- `content/kb/blogs/**.md`
- `content/kb/notes/*.md`
- `content/ideas/*-batch.md`
- `content/<slug>/meta.json`
- `content/<slug>/article.md`
- `content/<slug>/qa.json`
- `content/<slug>/hero.png`
- `content/<slug>/hero-linkedin.png`
- `content/<slug>/social.json`

## Lifecycle Flow

Content status is stored in `meta.json` and validated by transition rules:

1. `draft`
2. `qa_passed`
3. `published`
4. `social_generated`
5. `scheduled`
6. `posted`

Typical pipeline:

1. `init`
2. `kb add-feed` + `kb sync`
3. `ideas`
4. `new`
5. `draft`
6. `qa --approve`
7. `hero`
8. `publish --approve`
9. `social`
10. `schedule`

## Request/Response Integration Points

### LLM

- Created through `internal/llm.NewClient`.
- Used by `init`, `ideas`, `draft`, `qa`, `social`.
- Supports configurable base URL for test/mocked environments.

### Image Generation

- `internal/hero.OpenAIImageGenerator` calls image API endpoint.
- Writes normalized PNG outputs and updates content metadata.

### Publishing

- `internal/publish.Service` enforces lifecycle gates.
- Adapter interface:
  - `HTTPPublisher`
  - `StaticPublisher`

## Extension Points

### Add a New LLM Provider

1. Implement `internal/llm.LLMClient`.
2. Register provider in `internal/llm.NewClient`.
3. Update config validation allowed providers.

### Add a New Publish Adapter

1. Implement `internal/publish.Publisher`.
2. Add constructor wiring in `NewPublisherFromConfig`.
3. Extend config schema/validation for adapter-specific options.

### Add New Commands

1. Add command constructor in `cmd/` with `Use/Short/Long`.
2. Wire command in `cmd/root.go`.
3. Add tests at unit level and lifecycle coverage where applicable.

## Testing Strategy

- Unit tests live with packages (`*_test.go`).
- Integration coverage includes command and adapter paths.
- `TestE2ELifecycle` exercises the full workflow in a temporary directory with all external services mocked.
