# ContentAI

ContentAI is a Go CLI for end-to-end content production:

- initialize project context (`voice.md`, `blueprint.md`)
- ingest knowledge sources from RSS + notes
- generate ideas
- draft articles
- run QA with optional auto-fix
- generate hero images
- publish through adapters
- generate social copy
- schedule lifecycle status

## Install

### Prerequisites

- Go 1.22+

### Build from source

```bash
go build ./...
```

Run locally:

```bash
go run . --help
```

## Quickstart

```bash
# 1) Initialize project (interactive wizard)
contentai init my-project

# 2) Add and sync feeds
contentai kb add-feed https://example.com/feed.xml
contentai kb sync

# 3) Generate idea outlines
contentai ideas --count 3

# 4) Create article scaffold
contentai new my-first-post --from-idea 1

# 5) Draft, QA, hero, publish, social, schedule
contentai draft my-first-post
contentai qa my-first-post --auto-fix --approve
contentai hero my-first-post
contentai publish my-first-post --approve
contentai social my-first-post
contentai schedule my-first-post
```

## Configuration

ContentAI reads `contentai.toml` (use `--config` to override path).

Primary sections:

- `[project]`: content directory and gates
- `[llm]`: provider/model/api key command/base URL
- `[images]`: image provider/model/size/api key command
- `[publish]`: publish adapter configuration (`http` or `static`)
- `[schedule]`: scheduling defaults
- `[qa]`: QA defaults

Environment variables commonly used in development/testing:

- `CONTENTAI_LLM_API_KEY`
- `CONTENTAI_IMAGE_API_KEY`
- `OPENAI_API_KEY`
- `CONTENTAI_LLM_BASE_URL` (useful for local/mock LLM endpoints)
- `CONTENTAI_IMAGE_BASE_URL` (useful for local/mock image endpoints)

## Command Reference

### `contentai init [name]`

Run initialization wizard in the current directory.

### `contentai kb`

Knowledge-base operations.

- `contentai kb add-feed [url] [--opml path]`
- `contentai kb list-feeds`
- `contentai kb sync`
- `contentai kb add-note <path>`
- `contentai kb search <query> [--limit N]`

### `contentai ideas`

Generate structured idea outlines.

Flags:

- `--from-kb` (default true)
- `--from-conversations`
- `--count N`

### `contentai new <slug>`

Create `content/<slug>/` scaffold.

Flags:

- `--title "..."`
- `--from-idea N`

### `contentai draft <slug>`

Generate or refine article markdown.

Flags:

- `--source path.md`
- `--interactive`

### `contentai qa <slug>`

Run QA checks, optional LLM auto-fix, and optional approval.

Flags:

- `--auto-fix`
- `--approve`

### `contentai hero <slug>`

Generate hero and LinkedIn images.

Flags:

- `--regenerate`

### `contentai publish <slug>`

Publish via configured adapter.

Flags:

- `--approve`
- `--dry-run`

### `contentai social <slug>`

Generate and save X + LinkedIn copy.

Flags:

- `--no-interactive`

### `contentai schedule <slug>`

Mark an item as scheduled.

Flags:

- `--immediate` (mark as posted immediately)

### `contentai templates`

Template management.

- `contentai templates export [--dir path] [--force]`

### Utility Commands

- `contentai list`
- `contentai version`

## Testing

```bash
go test ./... -count=1
```

End-to-end lifecycle test:

- `TestE2ELifecycle` in `e2e_test.go`
- runs in a temp workspace
- uses mocked RSS, LLM, image, and publish HTTP services
