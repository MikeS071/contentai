## ContentAI (Go CLI — `contentai`)
- **Binary:** `contentai` — AI-powered content pipeline: ideas -> draft -> QA -> hero -> publish -> social.
- **Skill:** `contentai` — read SKILL.md for full command reference.

### When to use ContentAI
- User asks to write a blog post, article, or insight
- User wants article ideas or content suggestions
- User says "publish" or "schedule" for an existing draft
- Morning content scan / idea generation routine
- **DO NOT use for:** social-only posts (no article), quick one-line responses, non-content tasks

### How to use it
1. **Generate ideas:** `contentai ideas --from-kb --from-conversations --count 5` -> present to user
2. **New article:** `contentai new <slug> --title "..."` (or `--from-idea N`)
3. **Draft:** `contentai draft <slug>` -> present draft -> iterate with user feedback
4. **QA:** `contentai qa <slug>` -> show diff of auto-fixes -> user approves
5. **Hero image:** `contentai hero <slug>`
6. **Publish:** ask user for explicit approval -> `contentai publish <slug> --approve`
7. **Social copy:** `contentai social <slug>` -> present copy -> user approves
8. **Schedule:** ask user for explicit approval -> `contentai schedule <slug> --approve`

### Hard rules
- **Never publish or schedule without explicit user approval.** Always ask first.
- **Always run QA before presenting to user.** Don't show raw drafts.
- **Include voice.md + blueprint.md context in all content LLM calls.**
- **Present ideas, don't auto-execute.** User picks which ideas to develop.
