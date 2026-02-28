## ContentAI CLI
- **Binary:** `contentai` — content creation, publishing, and social scheduling pipeline
- **Config:** `contentai.toml` in project root (LLM provider, publish endpoint, social accounts)
- **Content dir:** `content/` — voice.md, blueprint.md, KB, and per-slug article directories
- **Posting calendar:** `content/posting-calendar.json` — managed by `contentai schedule/post`
- **Cron:** `contentai post --check` should run every 5 min to fire due scheduled posts
