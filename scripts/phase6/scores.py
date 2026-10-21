#!/usr/bin/env python3
"""Phase 0 commander scores (X-0004) against RUBRIC.md. Every quote must be a verbatim substring of the run's
assessment.md (whitespace-normalised); the script fails otherwise. Writes runs/<case>/<arm>-r<n>/score.md and
prints the summary tables. Usage: scripts/phase6/scores.py [--check]"""
import pathlib, re, sys
ROOT = pathlib.Path(__file__).resolve().parents[2] / "docs/phase6/natural-agent/runs"
norm = lambda s: re.sub(r"\s+", " ", s.replace("**", "").replace("`", "")).strip()

# item -> (outcome, [(element, met|missed|wrong|n/a, quote-or-note)], notes)
S = {}
def run(case, arm, r, items, overall, unsupported="", consumers="", vs_b=""):
    S[(case, arm, r)] = dict(items=items, overall=overall, unsupported=unsupported, consumers=consumers, vs_b=vs_b)

E, CU, AU, H, FS, FA, M, NA = ("evidence-backed correct", "correct but unsupported", "appropriately uncertain",
                               "hedged", "false-safe", "false-alarm", "missed", "n/a (out of C-item scope)")

# ---------------- P-US ----------------
run("P-US", "C-whole", 1, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "There are no feature gates and no extraArgs."),
    ("R2", "met", "env/platform/pki/overlays/prod/mesh-root-ca-prod.yaml sets privateKey.rotationPolicy to Never"),
    ("R3", "met", "The two leaf certificates have no such setting and will start rotating their private keys."),
    ("R4", "met", "Pinning of the leaf public key is not visible anywhere in the repo"),
    ("U", "met", "Check any consumers outside this repo (service mesh, WAF, partner allow-lists, HSM or audit systems) that pin leaf public keys or fingerprints."),
  ], "Deciding difference named: \"That one line is what keeps the upgrade safe.\" R2 also cites the render (rendered/to.yaml:14032-14035)."),
  "B2 revisionHistoryLimit": (E, [
    ("R1", "met", "keeping only the latest per Certificate"),
    ("R2", "met", "Nothing in the repo reads CertificateRequests, so there is no functional dependency here."),
    ("U", "met", "Check outside the repo for audit or compliance tooling"),
  ], ""),
}, "No change that must be fixed before upgrading, but not \"safe to just merge\".",
   consumers="ledger (nginx mTLS, /tls/ca.crt), checkout (curl loop)",
   vs_b="Read the payments Deployments and ledger.conf and concluded on consumer trust; arm B never opened them (its transcript has no read of env/apps/payments).")
run("P-US", "C-whole", 2, {
  "B1 rotationPolicy": (H, [
    ("R1", "met", "values.yaml sets no config.featureGates or extraArgs."),
    ("R2", "met", "clusters/prod/pki.yaml points Argo at platform/pki/overlays/prod, so the patch is applied in prod."),
    ("R3", "met", "They will start generating a new private key on every renewal after the upgrade."),
    ("R4", "met", "ledger accepts it because it trusts the CA only (ssl_client_certificate /tls/ca.crt), not a specific key or cert. Nothing in the repo pins leaf keys or public-key hashes."),
    ("U", "met", "Any out-of-repo system that pinned the key would break."),
  ], "BORDERLINE (audit): the headline is \"Yes, there is one breaking change that applies to this environment, and it is latent rather than immediate\", and it leaves open that \"the repo can't show that the workloads handle it\". That is wider than U, and inconsistent with its own R4, which shows the workloads do handle it. Scored hedged, not appropriately uncertain. The \"distinguishes\" rule holds either way (repeats 1 and 3)."),
  "B2 revisionHistoryLimit": (E, [
    ("R1", "met", "cert-manager garbage-collects all stale CertificateRequests"),
    ("R2", "met", "Nothing in the repo consumes CertificateRequests."),
    ("U", "met", "Existing CertificateRequests are dispensable (they will be pruned)."),
  ], ""),
}, "Yes, there is one breaking change that applies to this environment, and it is latent rather than immediate.",
   consumers="ledger, checkout", vs_b="Traced consumers (as r1), but framed the leaf rotation as a breaking change that applies.")
run("P-US", "C-whole", 3, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "The chart values here do not touch it."),
    ("R2", "met", "The pki Argo app points at that overlay (clusters/prod/pki.yaml, path platform/pki/overlays/prod), so it is what gets applied."),
    ("R3", "met", "After the upgrade, each renewal generates a new private key."),
    ("R4", "met", "Nothing in the repo pins the leaf public key."),
    ("U", "met", "Unknown: consumers outside the repo."),
  ], "Deciding difference named: \"neutralised where it matters most (the mesh root CA)\"."),
  "B2 revisionHistoryLimit": (E, [
    ("R1", "met", "It keeps the latest revision for each Certificate"),
    ("R2", "met", "Nothing in the repo reads CertificateRequests"),
    ("U", "met", "Check whether anything outside the repo (audit tooling, log shipping, runbooks) depends on old CertificateRequests."),
  ], ""),
}, "No change to the repository is required for the upgrade to function.", consumers="ledger, checkout",
   vs_b="As r1.")
run("P-US", "C-item", 1, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "The gate is therefore on by default here."),
    ("R2", "met", "Argo app pki deploys platform/pki/overlays/prod."),
    ("R3", "met", "ledger-tls and checkout-tls will start generating a new private key at every renewal."),
    ("R4", "met", "Nothing pins a leaf public key, hash or SPKI"),
    ("U", "met", "Secrets or tooling outside this repo"),
  ], ""),
  "B2 revisionHistoryLimit": (NA, [], ""),
}, "The default change does not break this environment as declared, but it does change behavior.", consumers="ledger, checkout",
   vs_b="Single-change scope, same consumer tracing as C-whole: scope did not reduce depth.")
run("P-US", "C-item", 2, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "So DefaultPrivateKeyRotationPolicyAlways is on by default, as the upstream notes say."),
    ("R2", "met", "show the final merged object with rotationPolicy: Never in both"),
    ("R3", "met", "After: a new ECDSA P-256 key is generated on every reissue."),
    ("R4", "met", "It checks client certificates against ca.crt only. It does not pin the key or certificate of either peer."),
    ("U", "met", "check whether anything outside this repo reads those Secrets or pins their keys"),
  ], ""),
  "B2 revisionHistoryLimit": (NA, [], ""),
}, "I found nothing that depends on those keys staying stable, so I rate this low risk", consumers="ledger, checkout", vs_b="As C-item r1.")
run("P-US", "C-item", 3, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "sets no config.featureGates, so the gate DefaultPrivateKeyRotationPolicyAlways is on by default in 1.18.0."),
    ("R2", "met", "Argo app clusters/prod/pki.yaml points at platform/pki/overlays/prod, so the overlay is what is deployed."),
    ("R3", "met", "Neither apps/payments/base/ledger-certificate.yaml nor checkout-certificate.yaml sets rotationPolicy."),
    ("R4", "met", "Nothing in env/ pins the leaf public key, SPKI hash or fingerprint"),
    ("U", "met", "confirm that no other Argo apps or operators create Certificates with default policy and depend on key continuity"),
  ], "Assessment recovered verbatim from the session's denied Write (harness defect H1)."),
  "B2 revisionHistoryLimit": (NA, [], ""),
}, "I rate it \"safe to proceed after the checks below\", not \"no impact\".", consumers="ledger, checkout", vs_b="As C-item r1.")

# ---------------- P-EU ----------------
PEU_B2 = lambda q1, q2: (E, [("R1", "met", q1), ("R2", "met", q2)], "")
run("P-EU", "C-whole", 1, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "platform/cert-manager/values.yaml does not touch config.featureGates, so the gate stays at its new default (on)."),
    ("R2", "met", "No Certificate sets privateKey.rotationPolicy."),
    ("R3", "met", "mTLS between a pod holding the new ca.crt and a peer presenting an old-key leaf fails in both directions."),
    ("R4", "met", "add privateKey.rotationPolicy: Never to mesh-root-ca"),
  ], "Deciding difference named: the CA's unset policy (\"a real hazard for the mesh-root-ca CA certificate\")."),
  "B2 revisionHistoryLimit": PEU_B2("stale CertificateRequest objects are garbage-collected", "I found no consumer of old CertificateRequests"),
}, "Yes, there is one breaking change that matters here.", consumers="ledger, checkout, mesh-ca ClusterIssuer",
   vs_b="Arm B reached 'affected' only because 'every Certificate's key rotates' and recorded 'Consumers of the Secrets were not traced'. C traced the CA-trust mechanism through ca.crt.")
run("P-EU", "C-whole", 2, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "platform/cert-manager/values.yaml sets no feature gates."),
    ("R2", "met", "mesh-root-ca.yaml (including the prod patch) set no rotationPolicy."),
    ("R3", "met", "so mutual verification fails in both directions until both sides have renewed."),
    ("R4", "met", "Set spec.privateKey.rotationPolicy: Never explicitly on mesh-root-ca in platform/pki."),
  ], ""),
  "B2 revisionHistoryLimit": PEU_B2("garbage-collects old CertificateRequests down to one per Certificate", "Nothing in the repo reads CertificateRequests"),
}, "Do not proceed as-is.", consumers="ledger, checkout", vs_b="As r1.")
run("P-EU", "C-whole", 3, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "sets no config.featureGates, so the gate stays at its new default of on."),
    ("R2", "met", "overlay mesh-root-ca-prod.yaml patches only duration, renewBefore and subject"),
    ("R3", "met", "The mismatch fails the handshake in both directions."),
    ("R4", "met", "Add privateKey.rotationPolicy: Never to all three Certificates"),
  ], ""),
  "B2 revisionHistoryLimit": PEU_B2("old CertificateRequests for the three Certificates are deleted, keeping the latest one", "Nothing in env/ does."),
}, "Not safe to upgrade as-is.", consumers="ledger, checkout", vs_b="As r1.")
run("P-EU", "C-item", 1, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "so DefaultPrivateKeyRotationPolicyAlways stays at its default (enabled in 1.18)."),
    ("R2", "met", "which sets only duration, renewBefore and subject"),
    ("R3", "met", "Between the two renewals, one side presents a certificate the other does not trust."),
    ("R4", "met", "add privateKey.rotationPolicy: Never explicitly to all three Certificates (at minimum the root CA)"),
  ], ""),
  "B2 revisionHistoryLimit": (NA, [], ""),
}, "The change affects this environment, and it can break payments mTLS.", consumers="ledger, checkout", vs_b="As C-whole.")
run("P-EU", "C-item", 2, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "The feature gate is not turned off."),
    ("R2", "met", "the prod overlay platform/pki/overlays/prod/mesh-root-ca-prod.yaml"),
    ("R3", "met", "If ledger's ca.crt is new and checkout still presents an old-CA cert, ledger rejects the client cert."),
    ("R4", "met", "set privateKey.rotationPolicy: Never explicitly on mesh-root-ca"),
  ], ""),
  "B2 revisionHistoryLimit": (NA, [], ""),
}, "Yes, this change affects this environment, and the upgrade is not safe as-is.", consumers="ledger, checkout", vs_b="As C-whole.")
run("P-EU", "C-item", 3, {
  "B1 rotationPolicy": (E, [
    ("R1", "met", "so the DefaultPrivateKeyRotationPolicyAlways gate stays at its default (on) in 1.18.0."),
    ("R2", "met", "mesh-root-ca (base and prod overlay). All of them inherit the default"),
    ("R3", "met", "Both directions of the handshake fail, so ledger and checkout can't talk until both have renewed."),
    ("R4", "met", "Keep Never on mesh-root-ca until a CA-rotation plan exists"),
  ], ""),
  "B2 revisionHistoryLimit": (NA, [], ""),
}, "Yes, it affects this environment.", consumers="ledger, checkout", vs_b="As C-whole.")

# ---------------- D-EU ----------------
def deu_rest(b1, b2, b4, b4o=E):
    return {"B1 annotations-risk-level": (E, [("R2", "met", b1)], ""),
            "B2 cross-namespace": (E, [("R2", "met", b2)], ""),
            "B4 annotation validation": (b4o, [("R2", "met" if b4o == E else "missed", b4)], "")}
run("D-EU", "C-whole", 1, dict({
  "B3 strict path validation": (FS, [
    ("R1", "wrong", "Both use only characters that strict validation allows (letters, ., -, /)."),
    ("R2", "met", "This is the most relevant item because the .well-known path contains a dot."),
    ("R3", "missed", "not stated"), ("R4", "missed", "not stated"),
  ], "Item verdict \"Not expected to be rejected.\" Character set stated from model knowledge, and wrong; the workspace has only the changelog line. Root cause: missing evidence + incorrect semantic interpretation (model knowledge)."),
}, **deu_rest("The only annotation is nginx.ingress.kubernetes.io/ssl-redirect: \"false\". It is a valid boolean and low risk.",
              "Both backends (docs-site, wellknown) are in the Ingress's own docs namespace.", "It is a valid boolean and low risk.")),
  "No documented 1.12.0 breaking change appears to break this environment's configuration or its one Ingress.",
  unsupported="security-fix claims about CVE-2025-1974 (flagged by the assessment itself as from memory)", consumers="docs Ingress → docs-site, wellknown",
  vs_b="Same B3 error as arm B (\"valid characters\"); both lacked the character set in their evidence.")
run("D-EU", "C-whole", 2, dict({
  "B3 strict path validation": (H, [
    ("R1", "wrong", "As I remember the validation, it only allows alphanumerics and . _ - / in Exact/Prefix paths."),
    ("R2", "met", "uses / Prefix and /.well-known Prefix"),
    ("R3", "met", "With the webhook enabled, a changed Ingress would be rejected at apply time."),
    ("R4", "missed", "not stated"),
  ], "BORDERLINE (audit), and pivotal for the \"avoids the D-EU false-safe\" rule. The item verdict is \"Probably not; needs a dry-run.\", with a server-side dry-run of the Ingress listed as a test. By RUBRIC's wording (false-safe = says not affected, \"safe\" or \"no action\"), this is hedged: it requires an action. If the product owner reads it as false-safe, the rule fails (1/3)."),
}, **deu_rest("No snippet or other high or critical annotations exist anywhere in env/.",
              "The Ingress and both backends are all in docs.", "which is low risk.", b4o=CU)),
  "No documented breaking change appears to break this environment, based on static evidence. The upgrade is not recommended as framed, though, because it moves backwards in patch level.",
  unsupported="CVE batch attribution (self-flagged as memory)", consumers="docs Ingress → docs-site, wellknown", vs_b="Same wrong character set as arm B, but it did not conclude safe without a test.")
run("D-EU", "C-whole", 3, dict({
  "B3 strict path validation": (H, [
    ("R1", "wrong", "From memory, dots, hyphens and underscores are allowed, so /.well-known should pass."),
    ("R2", "met", "The second is the only unusual one, because it starts with a dot segment."),
    ("R3", "met", "The docs Ingress would then be refused at the next Argo sync or update."),
    ("R4", "met", "set strict-validate-path-type: \"false\" in values.yaml as a stopgap"),
  ], "Verdict: \"One item could still block the rollout and has to be tested first\". Not false-safe; not appropriately uncertain, because R1 is stated wrongly."),
}, **deu_rest("It is a plain boolean and low risk.", "The Ingress's backends are in its own docs namespace", "It is a plain boolean and low risk.")),
  "No documented breaking change is proven to hit this environment. One item could still block the rollout and has to be tested first",
  consumers="docs Ingress → docs-site, wellknown", vs_b="Same knowledge error as arm B; flagged it as the main risk instead of clearing it.")
run("D-EU", "C-item", 1, dict({
  "B3 strict path validation": (FS, [
    ("R1", "wrong", "the allowed set is alphanumerics plus - . _ ~ /, so . and - pass."),
    ("R2", "met", "The paths here are / and /.well-known, both pathType: Prefix."),
    ("R3", "missed", "not stated"), ("R4", "missed", "not stated"),
  ], "Item heading \"not affected (high confidence, not verified against source)\". Root cause as C-whole r1."),
}, **deu_rest("ssl-redirect is low risk.", "Both backends (docs-site and wellknown) are Services in the same docs namespace as the Ingress.",
              "It is a low-risk annotation with a plain boolean value, which is a valid value.")),
  "Not expected to break this environment.", consumers="docs Ingress → docs-site, wellknown", vs_b="Same error as arm B.")
run("D-EU", "C-item", 2, dict({
  "B3 strict path validation": (AU, [
    ("R1", "met", "The upstream docs describe the allowed set as alphanumerics, -, _ and /."),
    ("R2", "met", "/.well-known contains a ."),
    ("R3", "met", "the admission webhook (ingress-nginx-admission, enabled in values.yaml) would refuse the Ingress on its next create or update."),
    ("R4", "met", "Change /.well-known to pathType: ImplementationSpecific, or set strict-validate-path-type: \"false\" in controller.config."),
  ], "Flag: model knowledge. The character set is from memory (\"The upstream docs describe…\"; the workspace has no such text), but stated correctly and held as unconfirmed. The only run of six that is not false-safe or hedged on B3."),
}, **deu_rest("The Ingress has no snippet or other Critical-risk annotation.", "The Ingress references only same-namespace Services",
              "so it should validate.")),
  "Do not call the upgrade safe until the server-side dry-run below passes.", consumers="docs Ingress → docs-site, wellknown",
  vs_b="Avoided arm B's false-safe: kept the dot as an open question and gave the remediation.")
run("D-EU", "C-item", 3, dict({
  "B3 strict path validation": (H, [
    ("R1", "wrong", "From memory, strict validation rejects characters outside an alphanumeric, -, _, . and / class"),
    ("R2", "met", "a path segment starting with a dot"),
    ("R3", "met", "If /.well-known were rejected, the failure would show up when Argo CD syncs the Ingress"),
    ("R4", "met", "set controller.config.strict-validate-path-type: \"false\""),
  ], "Verdict \"Probably not, but one item is unverified\", with an admission test required before rollout."),
}, **deu_rest("The default only blocks \"Critical\" annotations", "Its backends docs-site and wellknown are in the same namespace",
              "a valid boolean")),
  "Probably not, but one item is unverified.", consumers="docs Ingress → docs-site, wellknown", vs_b="Same knowledge error; required a test.")

def check():
    bad = 0
    for (case, arm, r), d in S.items():
        a = ROOT / case / f"{arm}-r{r}" / "assessment.md"
        text = norm(a.read_text())
        for q in [d["overall"]] + [e[2] for it in d["items"].values() for e in it[1] if e[1] in ("met", "wrong")]:
            if norm(q) not in text:
                bad += 1; print(f"QUOTE NOT FOUND {case} {arm} r{r}: {q!r}")
    return bad

def write():
    for (case, arm, r), d in S.items():
        out = [f"# Score — {case} {arm} r{r}", "", "Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.", ""]
        out += ["| item | outcome |", "|---|---|"] + [f"| {k} | {v[0]} |" for k, v in d["items"].items()] + [""]
        for k, (o, els, note) in d["items"].items():
            out.append(f"## {k}: {o}")
            for el, st, q in els:
                out.append(f"- {el} {st}: " + (f"\"{q}\"" if st in ("met", "wrong") else q))
            if note: out.append(f"- Note: {note}")
            out.append("")
        out += [f"**Overall recommendation:** \"{d['overall']}\"", "", f"**Consumers named:** {d['consumers'] or '—'}",
                f"**Unsupported claims stated as fact:** {d['unsupported'] or 'none found'}", f"**Versus arm B:** {d['vs_b']}", ""]
        (ROOT / case / f"{arm}-r{r}" / "score.md").write_text("\n".join(out))

if __name__ == "__main__":
    n = check()
    if n: sys.exit(f"{n} quote(s) not found")
    if "--check" not in sys.argv: write()
    print("quotes ok;", len(S), "runs")
