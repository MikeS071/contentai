# ContentAI

AI-powered content creation, publishing, and social scheduling CLI. Built as a portable [OpenClaw](https://openclaw.ai) skill.

ContentAI handles the full content lifecycle: voice profiling → idea generation → drafting → QA → hero images → publishing → social copy → scheduling. All from the command line, all configurable, no hardcoded anything.

## Install

```bash
go install github.com/MikeS071/contentai@latest
```

Or build from source:

```bash
git clone https://github.com/MikeS071/contentai.git
cd contentai
go build -o contentai .
```

### OpenClaw skill install

```bash
contentai install --openclaw
```

This copies SKILL.md and workspace snippets into your OpenClaw workspace.

## Quick Start

```bash
# 1. Create a workspace
mkdir my-content && cd my-content

# 2. Initialize — guided voice discovery + config
contentai init

# 3. Add knowledge sources (optional)
contentai kb add-feed https://simonwillison.net/atom/everything
contentai kb sync

# 4. Generate article ideas
contentai ideas --count 5

# 5. Create a new content item
contentai new "my-first-article" --title "My First Article"

# 6. Draft the article
contentai draft "my-first-article"

# 7. Run QA checks
contentai qa "my-first-article"

# 8. Generate hero image
contentai hero "my-first-article"

# 9. Publish
contentai publish "my-first-article" --approve

# 10. Generate social copy + schedule
contentai social "my-first-article"
contentai schedule "my-first-article"
```

## Commands

| Command | Description |
|---------|-------------|
| `init` | Guided setup — Perspective Architect voice discovery, generates `voice.md` + `blueprint.md` |
| `kb add-feed <url>` | Add RSS/Atom feed to knowledge base |
| `kb add-note <file>` | Add markdown note to knowledge base |
| `kb sync` | Fetch latest articles from all feeds |
| `kb search <query>` | Search knowledge base |
| `ideas` | Generate article ideas from voice + blueprint + KB |
| `new <slug>` | Create `content/<slug>/` with metadata scaffold |
| `draft <slug>` | Write article using Creative Thought Partner + Blog Writer prompts |
| `qa <slug>` | Run 7 quality checks, optional LLM auto-fix |
| `hero <slug>` | Generate hero image with 8 rotating color palettes |
| `publish <slug>` | Push to configured publisher (HTTP or static) |
| `social <slug>` | Generate platform-specific social copy (X + LinkedIn) |
| `schedule <slug>` | Queue for next available posting slot |
| `post <slug>` | Fire a scheduled post immediately |
| `list` | Show all content items with status |
| `templates export` | Export embedded prompts for local customization |
| `install` | Install OpenClaw skill assets to workspace |

## Configuration

ContentAI uses `contentai.toml` in the working directory:

```toml
[project]
name = "my-blog"

[llm]
provider = "openai"           # openai | anthropic
model = "gpt-4o-mini"         # default model for ideas, QA, social
api_key_cmd = "pass show apis/openai-api-key"  # shell command to get API key

[llm.draft]
model = "gpt-4o"              # stronger model for article writing

[content]
dir = "content"               # where content items live

[hero]
provider = "openai"
model = "gpt-image-1"
api_key_cmd = "pass show apis/openai-api-key"
width = 1200
height = 630
title_overlay = true          # overlay article title on hero image
linkedin_resize = true        # generate LinkedIn-sized variant

[publish]
type = "http"                 # http | static
url = "https://cms.example.com/api/articles"
auth_header = "Authorization"
auth_cmd = "pass show apis/blog-api-key"
auth_prefix = "Bearer "
# field_mapping = { title = "title", slug = "slug", content = "content" }

[social.x]
enabled = true
api_key_cmd = "pass show apis/x-api-key"
api_secret_cmd = "pass show apis/x-api-secret"
access_token_cmd = "pass show apis/x-access-token"
access_token_secret_cmd = "pass show apis/x-access-token-secret"

[social.linkedin]
enabled = true
access_token_cmd = "pass show apis/linkedin-access-token"

[schedule]
timezone = "UTC"
days = ["Mon", "Tue", "Wed", "Thu", "Fri"]
window_start = "09:00"
window_end = "09:30"

[qa]
auto_fix = true               # attempt LLM-powered fixes
max_fix_rounds = 2            # max auto-fix iterations
```

### Secret management

All credentials use the `api_key_cmd` pattern — ContentAI shells out to your command and reads stdout. This works with:

- **pass**: `pass show apis/openai-api-key`
- **Environment variables**: `echo $OPENAI_API_KEY`
- **1Password CLI**: `op read op://vault/item/field`
- **AWS Secrets Manager**: `aws secretsmanager get-secret-value --secret-id my-key --query SecretString --output text`

No API keys are ever stored in config files.

## Content Lifecycle

```
idea → new → draft → qa → qa_passed → hero → publish → published → social → schedule → posted
```

Each content item lives in `content/<slug>/` with:

```
content/my-article/
├── meta.json        # status, title, dates, category
├── article.md       # the article
├── qa.json          # QA results
├── hero.png         # hero image (1200x630)
├── hero-linkedin.png # LinkedIn variant
└── social.json      # generated social copy
```

## Voice System

ContentAI's voice system has two parts:

### voice.md
Your writing voice — tone, style, anti-patterns, benchmark articles. Generated during `contentai init` via the Perspective Architect prompt (a guided 5-phase discovery process) or written manually.

### blueprint.md
Your intellectual blueprint — core ideas, content pathways, themes. This ensures every article connects back to your broader worldview, not just the topic at hand.

Both files are included in every LLM call. The voice profile is never truncated.

## Prompt Templates

ContentAI ships with 8 embedded prompts:

| Template | Used by |
|----------|---------|
| `perspective-architect` | `init` — guided voice discovery |
| `voice-extractor` | `init` — extract voice from example articles |
| `deep-post-ideas` | `ideas` — generate article outlines |
| `creative-thought-partner` | `draft` — develop the article angle |
| `blog-writer` | `draft` — write the article |
| `qa-checklist` | `qa` — quality check rules |
| `hero-prompt` | `hero` — image generation prompt |
| `social-copy` | `social` — platform-specific copy |

Export and customize:

```bash
contentai templates export    # writes to templates/ directory
# Edit templates/*.md to your liking
# ContentAI uses local templates when present, falls back to embedded
```

## QA Checks

The `qa` command runs 7 built-in checks:

1. **no_secrets** — scans for API keys, tokens, passwords
2. **voice_consistency** — checks article against voice.md rules
3. **accuracy** — flags unsupported claims and vague statements
4. **dash_cleanup** — catches unnecessary mid-sentence dashes
5. **dedup** — detects repeated phrases and restated points
6. **reading_level** — flags overly long sentences
7. **length** — validates word count (default 500-1000)

With `auto_fix = true`, ContentAI uses the LLM to propose fixes and shows a diff for each. Fixes are applied only with confirmation or `--auto-approve`.

## Publisher Adapters

### HTTP Publisher
Posts to any REST API. Configure URL, auth, and field mapping in `contentai.toml`.

### Static Publisher
Writes formatted output to a local directory. Useful for static site generators.

### Custom Publishers
Implement the publisher interface in Go and register it. The adapter pattern is open for community additions.

## Social Adapters

### X (Twitter)
OAuth 1.0a authentication. Generates tweet-length copy with optional thread support.

### LinkedIn
OAuth 2.0 authentication. Generates professional-format posts.

Both adapters respect the hard rule: **no auto-posting without explicit approval, ever.** This is not configurable.

## Hard Rules

- **No auto-posting** — social posts always require explicit `--approve` or `contentai post`. Never fires automatically.
- **QA before publish** — `publish` requires QA pass (skip with `--skip-qa` if you know what you're doing).
- **No secrets in config** — all credentials via `api_key_cmd` shell commands.
- **Voice is sacred** — `voice.md` is included in full in every LLM call, never truncated or summarized.

## Development

```bash
# Run tests
go test ./...

# Run tests with coverage
go test ./... -cover

# Build
go build -o contentai .
```

## License

MIT
