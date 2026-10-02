# Judge brief — "worth a look" proxy review (SCORING.md §G, re-run)

You are a judge. You score AI review suggestions that a release-intelligence tool attached to
UNKNOWN upgrade findings for **cert-manager v1.17.0 → v1.18.0** on one environment. The suggestions
were written by several different models; you are not told which wrote which, and you must not try
to find out.

## Inputs (read only these)
- `docs/phase3/model-comparison/judging/packet-r2.json`: 26 blinded items. Each one has the finding
  (title, detail, breaking flag, category), the suggestion (title, content, confidence) and the
  evidence it cites (URI, locator, excerpt).
- The environment the suggestions talk about, in `docs/phase3/review-packet/example-env/report4-env/`
  (values.yaml, manifests/, crds/, images.txt). The cluster is Kubernetes 1.28.

Do NOT read `judging/key.json`, `judging/key-r2.json`, `judging/packet.json`, any `judging/answers-*.json`, anything under `docs/phase3/model-comparison/runs/`, `.ri/`,
`docs/phase3/review-packet/proxy-reviews/`, `docs/phase3/review-packet/reports/`, repo source code,
or other judges answers. Do not use the web, other LLMs or sub-agents. Score each item on its own merits.

## Personas (score every item under BOTH)
- **platform-engineer**: senior platform engineer. Owns Helm charts, image mirroring and the upgrade
  calendar for ~40 clusters. Has never seen this tool or its code. Reads the report cold with the
  environment inputs open alongside it.
- **sre**: SRE on an on-call rotation for a production fleet. Cares about blast radius, security
  exposure, alert surface, and not getting paged because an upgrade note went unread. Optimizes for
  "what can I safely skip". Reads asking two things: would this page me for nothing, and would it
  stay silent about something that pages me at 3am.

## What to score
For each item and each persona:
- `worth_look`: **yes** / **no** / **maybe**. Is this suggestion worth a reviewer's attention during
  this upgrade review, or is it dismissible without looking? (The question asked of the original proxy
  reviewers was "agree the suggestion is worth a reviewer's attention (vs. dismissible without looking)?")
- `comment`: optional, one short line.

Once per item (persona-independent):
- `grounding`: **supported** / **overstated** / **unsupported**. Judge it against the cited excerpt(s)
  and the environment inputs. *supported*: what the suggestion claims about the change is in the cited
  excerpt, and what it claims about the environment is true of the inputs. *overstated*: there is a
  basis for it, but it claims more certainty, scope or impact than the evidence shows. *unsupported*:
  a key claim is not in the cited evidence or contradicts the environment.
- `grounding_note`: one short line saying why, required unless the item is supported.

## Output
Write `docs/phase3/model-comparison/judging/answers-<your judge name>-r2.json`:
```json
{"judge":"<judge name>","model":"<your model id>","generatedAt":"<RFC3339>",
 "items":[{"item":"R01","grounding":"supported","grounding_note":"",
           "platform-engineer":{"worth_look":"yes","comment":""},
           "sre":{"worth_look":"no","comment":""}}, ...]}
```
Score all 26 items. Write the file with the Write tool. No script may produce scores.
Reply with ONLY the path of the file you wrote and the yes/no/maybe counts for each persona.
