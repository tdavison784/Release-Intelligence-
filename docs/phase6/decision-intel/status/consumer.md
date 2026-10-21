# Lane consumer — status (Milestone 1, ledger D-0001)

Branch `p6di/consumer`. Brief: `briefs/consumer.md`; plan: `PLAN-M1.md` §3 (changes 1–8), §4 tests.

## Done

- Read FLEET (P5, P6), _common, brief, MISSION, PLAN-M1. Code read: relations builder/edges, agent session,
  tools, submit_findings, verdicts, dossier, prompts, per-item loop, pipeline, replay driver.

## In progress (exact next step)

- Implementing domain additions, then relations consumer mode.

## Decisions (with why)

1. **Consumer edges are opt-in and live in a separate session graph (v9 only).** The pipeline's stored graph,
   traversal and therefore every policy input are unchanged. Why: (a) the plan forbids policy changes, and new
   (unresolved) edges in the stored graph would change impact paths and so classification; (b) any change to
   the stored graph (new edges, or extra fields on existing edges) changes `expand_graph` results and dossier
   bytes of v1–v8 sessions, so historical transcripts (e.g. `B0-K0-…-v8`) would no longer replay. The agent's
   v9 session builds `relations.NewBuilder(st, catalog…, relations.WithConsumers())` over the bundle at the TO
   (and, for `trace_consumers side=from`, the FROM) side, keeps the stored graph for every other tool and for
   validators, and stores the consumer graph as a run artifact so the session is auditable.
2. **Mount detail is a structured additive field (`GraphEdge.Mount`), not text in `Detail`.** `Detail` is part
   of the edge id; changing it would rename every references edge and break the traversal's edge ids. The plan
   says "in Detail"; the content (container, mountPath, subPath, items key→path, projected source) is the same.
3. **`reads-file` edge direction follows the graph convention** (From = provider, To = consumer): the mounted
   Secret/ConfigMap → the ConfigMap whose file names a path under its mountPath. The plan writes it as
   "ConfigMap → Secret key"; the graph's propagation direction is the reverse.
