# Linked PR / commit evidence (`ri evidence link`)

Most remaining `need-more-evidence` review closes are one-line changelog entries or PR titles: the
setting, the old and new default, and who breaks are in the **referenced pull request**, which release
ingestion does not read. `ri evidence link` fetches that text and attaches it to the knowledge
candidates so reviewers and prompts can show it.

```
ri evidence link [-dir knowledge] [-dry-run] [-edges FILE] [-report FILE] <product> <from> <to>
```

## What it does

1. Builds the upgrade edge (cached) and takes every change's `References` (extracted from the note
   text): pull requests, issues, bare `#N`, and commit URLs on github.com. Advisory ids, CVEs and other
   hosts are ignored; commit abbreviations without a URL cannot be resolved and are skipped.
2. Fetches each item once through the cached GitHub client (`GITHUB_TOKEN`; replayable with `-offline`):
   `issues/N` (title, state, labels, description; says whether it is a PR), then
   `pulls/N/files` (first page, ≤ 100 files) for PRs, or `commits/SHA` for commits. Bounds: description
   8 000 bytes, ≤ 4 items per change (`-max-per-change`), ≤ 2 000 items per edge (`-max-fetches`).
3. Records `domain.Evidence` of kind **`linked-pr`** (PRs and issues) or **`linked-commit`**: URI = the
   item's page, producer `linkedev@v1`, content digest over the shown content (title, state, labels,
   description, files — not volatile API fields). Each record stays within `domain.MaxExcerpt`:
   *title and description* (HTML comments of PR templates removed), *description (part 2)* when the
   description is longer, and *changed files*. A note that cites `#N` bare gets "(cited in the notes as
   #N)" in the first record.
4. Adds the records to the stored candidates whose members cite those changes (member change id, falling
   back to the member's statement anchor since change ids shift with note text) —
   `knowledge.ExtendCandidateEvidence`.

Evidence is **context, never a conclusion**: it is not a fact, a validation or a decision, and no model
is called. Candidate ids derive from members, so they do not move; proposals cite evidence ids they were
shown, which growth never removes.

## Bare `#N` references

Notes read from a documentation repository (cert-manager's website) carry that repository on a bare
`#N`, but the number belongs to the code repository. A bare reference is therefore looked up in the
product's **version-source repositories** first (`github-releases` / `git-tags` locators of the catalog:
`linkedev.CodeReposOf`), then in the repository it was extracted with. Explicit URLs and `owner/repo#N`
references are never re-targeted. This is a heuristic; the item's URI shows where it was found.

## Store semantics

A candidate's evidence snapshot may **grow** (CONTRACT-CHANGE(prtext), `knowledge.mutableChange`): every
stored record must remain, in order; anything else must be identical. Re-running with unchanged content
adds nothing (content-derived ids; retrieval time is not part of the id). An edited PR description
produces new records, appended after the old ones. Environment evidence is still refused.

## Throttling and failures

- GitHub rate limiting (`fetch.ErrThrottled`: 429, 503 with `Retry-After`, exhausted quota) **stops the
  run**: nothing further is requested, the remaining targets are reported `throttled`, what was gathered
  is stored, the server's `Retry-After` is printed, and the command exits non-zero. Run it again later;
  the fetch cache resumes.
- A missing/private item is `not-found`, any other failure `error`; both are recorded in the report and
  skipped. `-dry-run` fetches and reports without writing the store.
- `-report FILE` writes the per-edge JSON (every link with its status, the candidates it reached).

Review items are never read or changed; "needs-evidence" counts in the report are by `ReviewItem.Status`
only.
