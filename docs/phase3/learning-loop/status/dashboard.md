# Lane `dashboard` — status

**State: in progress**

## Plan
1. `internal/reviewui`: server, session guard, decision/correction forms, bulk flow, templates, static CSS/JS (embed).
2. `reviewui.DemoQueue`: in-memory `knowledge.Queue` with realistic fixtures (fakes the knowledge lane until its FileStore lands).
3. `cmd/ri/review.go`: `ri review serve [-addr] [-demo|-knowledge DIR]`.
4. Tests for every route/action, invalid decisions, bulk paths. Docs (`docs/REVIEW_UI.md`) + HTML snapshot.
