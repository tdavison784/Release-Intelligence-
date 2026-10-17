# Lane `refcheck` — exposure is not failure; dangling references are (DESIGN R1 vs R3 consistency) — Opus

Branch `p4ei/refcheck`, worktree `.claude/worktrees/p4ei-refcheck`. Read FLEET.md, `_wave2-common.md`, DESIGN §2.6
and §3 (R1, R3), `docs/INVESTIGATE.md` ("Validators", "Relationships"), the relationships catalog construct.
You own `internal/investigate/validate`, `internal/investigate/relations`, the `relationships:` entries in
`products/*.yaml` for this. Never open `eval/investigations/*/{expected.yaml,NOTES.md,observed/}`; generic only;
every change has its own synthetic test.

## Inconsistency to fix

DESIGN: a rendered change correlated to an item is a **plausible-impact (R3)** signal; `affected` (R1) needs a
failing validation that **names a concrete failure**. Today `validate.RenderDiff` returns `fail` when a breaking
item's rendered change merely reaches a subject — exposure is reported as failure, so two environments with the same
rendered change but different consequences get the same `affected` class.

## Do

1. `RenderDiff`: a correlated rendered change reaching a subject ⇒ `error` (inconclusive, "exposure: <change> reaches
   <object>") — never `pass`, never `fail` by itself. `fail` only with a concrete failure: the TO object is invalid
   against its (rendered) CRD schema, a referenced object the change removes, a dangling reference (see 2), an
   apiVersion no longer served, etc. Keep every existing concrete-failure path.
2. `StaticReference`: evaluate references **in the TO render** (and FROM, to tell new from pre-existing) using the
   declarative relationship rules, including **namespace semantics** (same-namespace refs resolve in the referrer's
   namespace; cluster-scoped kinds; explicit namespace fields). A reference that resolved at FROM and dangles at TO,
   or a new reference that dangles at TO, ⇒ `fail` naming referrer, field path, target kind/namespace/name. A ref to an
   object the environment provides outside the rendered chart (env resources) resolves. Unknown target ⇒ `not-run`.
3. Relationship rules (declarative, cited upstream API docs, one URL per rule) for Secret references in
   prometheus-operator monitors — ServiceMonitor/PodMonitor `endpoints[].authorization.credentials`,
   `bearerTokenSecret`, `basicAuth`, `tlsConfig` secret/configmap refs (same namespace as the monitor) — and check
   the existing cert-manager / ingress-nginx rules for namespace semantics.
4. Look at eval's note: two runtime passes on an item did not clear it because a join `impact:not-joined` gap blocks
   it — determine whether that gap is legitimate (keep) or a classify defect (fix with a test); you own
   `internal/investigate/classify` for this one point only.

Done: tests (exposure ⇒ inconclusive; each concrete-failure path; dangling-ref same-namespace vs other-namespace;
env-provided target resolves; unknown target not-run), build/vet/test green, status, `LANE DONE:`.
