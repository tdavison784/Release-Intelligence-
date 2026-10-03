# proxy-incl-shadow: evaluation view, NOT the knowledge store

The real `knowledge/` store merged with the proxy shadow pass (`docs/phase3/learning-loop/proxy-shadow/knowledge/`). In the shadow pass a blind AI proxy (claude-opus-5-5) decided the high-priority review items that the product owner reviews in the real store. Every decision from the shadow pass is `reviewerKind: proxy`, so the facts it produced are at most proxy level (capped at REVIEW by the trust ladder). Evaluate this view at `-min-verification proxy` and report it as **proxy-incl-shadow**, never as human-verified.

Built by `scripts/proxy-shadow-merge.py` (three-way, per file; the real store wins every conflict).

```json
{
  "stats": {
    "real": 0,
    "shadow": 538,
    "same": 11011,
    "conflict-real-wins": 0,
    "real-only": 0,
    "shadow-only": 626
  },
  "conflicts": []
}
```
