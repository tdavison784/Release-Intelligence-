# End-to-end reproducibility fixtures

`internal/app/e2e_test.go` runs the real pipeline (`app.App.Upgrade`) for three
upgrades completely offline and compares the result with golden files:

| product | upgrade |
|---|---|
| cert-manager | v1.17.0 → v1.18.0 |
| istio | 1.29.2 → 1.30.1 |
| argo-cd | v2.14.11 → v3.0.6 |

The run is a pure function of three inputs, all checked in or injected:

1. **The recording** (`state/cache/`): every HTTP response and every git result
   the pipeline needed, as written by `internal/fetch` and `internal/gitsrc`.
2. **The product definitions** (`../../../../products/*.yaml`, loaded directly
   from the repository, not copied).
3. **The clock** (`app.Config.Now`, fixed by the test).

## Layout

```
state/cache/http/<host>/<key>.json + .body.gz    fetch cache (bodies gzip-compressed)
state/cache/git/tags/*.json.gz                   git ls-remote --tags results
state/cache/git/log/<repo>/*.json.gz             git-log documents
state/cache/git/dir/<repo>/*.json.gz             repo-dir listings with file contents
golden/<product>_<from>_<to>.txt                 upgrade.RenderText output (no colour)
golden/<product>_<from>_<to>.json                the UpgradeEdge as `ri upgrade -o json`
```

Not part of the recording (see `.gitignore`): `state/store` (output of a run)
and `state/cache/git/{mirrors,work}` (git clones). The clones only accelerate
online runs; offline replay is served exclusively from the `*.json.gz` result
files, and the tests prove it by running with an empty `PATH` and an HTTP
transport that fails the test on any use.

The recording is 2.8 MiB (2,922,153 bytes; budget: 8 MiB, enforced by
`TestE2EFixtureHygiene`) in 215 files: 1.76 MB of HTTP cache (1.35 MB from
`raw.githubusercontent.com`, 0.39 MB release downloads from `github.com`,
0.02 MB registry manifests), 1.09 MB of git `repo-dir` results (Argo CD's
`manifests/crds` at each of the eight releases, about 130 KB each, is the
bulk), 0.03 MB of commit logs and 0.05 MB of tag listings. The largest single
entry is the `argo-helm` chart index
(`raw.githubusercontent.com/argoproj/argo-helm/gh-pages/index.yaml`, 150 KB
compressed). Sizes are on-disk bytes; bodies and git results are stored
gzip-compressed. The goldens add 1.4 MB (Argo CD's JSON edge alone is 0.75 MB).

Services that were unreachable when recording (`api.github.com`, `quay.io`,
`charts.jetstack.io`, `argoproj.github.io`) are replayed as `unavailable`
sources ("not in cache (offline mode)"). That is the intended "honest about
gaps" behaviour; the semantic assertions rely on it (for example, cert-manager's
controller image is `referenced` by the install manifest, never `verified`).

## What the tests check

- `TestE2EUpgrade`: no error; `edge.Validate()`; key semantic facts per product
  (see `check*` in `e2e_test.go`); the rendered text and the JSON edge equal
  the golden files byte for byte.
- `TestE2EDeterminism`: running the same upgrade again on the same App, and on
  a fresh App with a different clock after the wall clock crossed a second
  boundary, yields byte-identical JSON (only `generatedAt` is allowed to follow
  the injected clock).
- `TestE2EFixtureHygiene`: size budget, no store or clones in the fixture.

Tests never modify `testdata`: each run replays from a temporary copy of
`state/`.

## When the goldens must be updated

Goldens are derived from the recording **and** from the code and definitions
that interpret it. Update them (and review the diff) when you change

- `products/*.yaml` (the edge contains `definitionDigest`, so *any* edit of a
  loaded product's definition changes its goldens),
- the normalisation, ingestion or upgrade logic, or the text renderer.

```
go test ./internal/app -run TestE2E -update
git diff internal/app/testdata/e2e/golden
```

The semantic assertions in `e2e_test.go` are not regenerated; if one fails
after a legitimate change, adjust it by hand.

## Re-recording the fixtures

Needs network access (git, `raw.githubusercontent.com`, GitHub release
downloads, Docker Hub, `ghcr.io`, ...) and `git` in `PATH`. It replaces
`state/` with a fresh recording and, with `-update`, rewrites the goldens:

```
go test ./internal/app -record -update -count=1
```

(`-record` and `-update` are flags of this package's test binary: run them on
`./internal/app` only, not on `./...`. `TestRecordFixtures` is the first test
of the file, so it runs before the golden tests.)

The recording is made into a scratch directory and replaces `state/` only if it
succeeds, with the store and the clones left out.

By default the hosts that were blocked in the original recording environment
are treated as unreachable while recording (`-record-deny`), so that a fresh
recording has the same shape as the checked-in one wherever it is made. Use
`-record-deny=` to record everything, and a `GITHUB_TOKEN` to use the GitHub API
adapters; expect different goldens (more sources become `ok`) and adjust the
assertions in `e2e_test.go` that expect `unavailable`/`referenced`/`expected`.

The `retrievedAt` timestamps in the JSON goldens are those of the recording, so
every re-recording rewrites them; apart from them (and the `unavailable`
details) two independent recordings produced identical goldens when this was
checked.

Upstream data also moves: some sources are read from a branch (for example
cert-manager's `supported-releases` page of the website repository), so a new
recording can legitimately change the goldens even without a code change.

## Adding an upgrade

Append a case to `e2eCases` in `e2e_test.go` with its semantic checks, re-record
(this refreshes the whole recording) and run with `-update`.
