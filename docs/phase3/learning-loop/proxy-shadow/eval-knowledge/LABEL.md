# proxy-incl-shadow: evaluation view, NOT the knowledge store

The real `knowledge/` store merged with the proxy shadow pass (`docs/phase3/learning-loop/proxy-shadow/knowledge/`). In the shadow pass a blind AI proxy (claude-opus-5-5) decided the high-priority review items that the product owner reviews in the real store. Every decision from the shadow pass is `reviewerKind: proxy`, so the facts it produced are at most proxy level (capped at REVIEW by the trust ladder). Evaluate this view at `-min-verification proxy` and report it as **proxy-incl-shadow**, never as human-verified.

Built by `scripts/proxy-shadow-merge.py` (three-way, per file; the real store wins every conflict; with --base-git=7b493755, conflicts touching different fields are merged field-wise).

```json
{
  "stats": {
    "real": 315,
    "shadow": 636,
    "same": 14291,
    "conflict-real-wins": 0,
    "real-only": 544,
    "shadow-only": 775,
    "merged-fields": 0
  },
  "conflicts": [],
  "mergedFields": []
}
```
