package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// Key is the idempotency key of a stage invocation (DESIGN.md §5.1): derived
// from content digests and ids only, so a retry, a resubmission or a second
// worker computes the same key and dedupes side effects. A new evidence
// round changes the inputs and therefore the keys. Pure: the workflow calls
// it too.
//
// A memo hit returns the first call's whole StageOutput, refs included, so a
// key covers every input the stage reads AND every ref its output carries
// forward (passed). Otherwise a second run that shares one stage's inputs
// with an earlier run in the same store (another config, another change)
// would continue on the earlier run's refs: its investigation, its change.
// Keys are workspace-local and never stored in a run record, so changing
// them changes no id or digest.
func Key(in StageInput) string {
	r, rf := in.Request, in.Refs
	cfg := r.Config.Digest()
	switch in.Stage {
	case domain.StageCollectChange:
		// The collector also renders the release when the config asks for it.
		k := investigate.ChangeKey(r.Product, r.From, r.To, in.Definition)
		if r.Config.Render {
			k += ":render"
		}
		return k + passed(rf)
	case domain.StageBuildSnapshot:
		return investigate.SnapshotKey(r.Environment) + ":" + domain.ShortHash(r.Product) + ":" + domain.ShortHash(strings.Join(in.Extra, "\n"), itoa(in.Round)) + passed(rf)
	case domain.StageAssemble:
		// Assemble writes rf.Pending into the record it creates and passes none
		// on; on a hit that record exists already, so Pending is not an input.
		rf.Pending = nil
		return "bundle:" + domain.ShortHash(rf.Change, rf.Snapshot, cfg, itoa(in.Round)) + passed(rf)
	case domain.StageBuildGraph:
		// It writes the graph into the run record, so the record is an input.
		return "graph:" + domain.ShortHash(rf.Bundle, relations.BuilderName, rf.Investigation, rf.Record) + passed(rf)
	case domain.StageTraverse:
		return "traverse:" + domain.ShortHash(rf.Investigation, in.Phase, digestOf(r.Config.Bound), rf.Record) + passed(rf)
	case domain.StageValidate:
		return "validate:" + domain.ShortHash(rf.Investigation, in.Phase, strings.Join(r.Config.Validators, ","), rf.Record) + passed(rf)
	case domain.StageInvestigate:
		return "investigate:" + domain.ShortHash(rf.Investigation, itoa(in.Round), rf.Record) + passed(rf)
	case domain.StageClassify:
		return "classify:" + domain.ShortHash(rf.Investigation, itoa(in.Round), rf.Record) + passed(rf)
	}
	return string(in.Stage) + ":" + domain.ShortHash(rf.Investigation, itoa(in.Round), rf.Record) + passed(rf)
}

// passed is the key part for the refs a stage's output carries forward
// unchanged. The pending audit events (stages before the record exists) count
// by content — stage, event, actor, refs, detail — not by time, so a retry or
// resubmission keeps its key.
func passed(rf Refs) string {
	pending := make([]domain.AuditEvent, len(rf.Pending))
	for i, e := range rf.Pending {
		e.At, e.Prev = zeroTime, ""
		pending[i] = e
	}
	return ":" + domain.ShortHash(rf.Change, rf.Snapshot, rf.Bundle, rf.Investigation, rf.Previous, digestOf(pending))
}

// ReviewKey is the idempotency key of applying a review decision.
func ReviewKey(in ReviewInput) string {
	return "review:" + domain.ShortHash(in.Investigation, itoa(in.Round), digestOf(in.Decision))
}

func itoa(n int) string { return strconv.Itoa(n) }

func digestOf(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ContentDigest digests the run record's content: no audit trail, no
// timestamps (CreatedAt, the impact report's stamps, validators' CheckedAt). It feeds idempotency keys, which must not change when a retry
// adds an audit event.
func ContentDigest(inv *domain.Investigation) string {
	c := *inv
	c.Audit, c.CreatedAt, c.ReportStamps = nil, zeroTime, nil
	c.Validations = untimed(inv.Validations)
	return digestOf(c)
}

// untimed copies validation records with CheckedAt zeroed: when a validator
// ran is provenance of the run (kept in the stored record), not identity —
// like CreatedAt and audit times, it must not change the record digest, or
// two identical runs would give different digests.
func untimed(vs []domain.ValidationRecord) []domain.ValidationRecord {
	if vs == nil {
		return nil
	}
	out := make([]domain.ValidationRecord, len(vs))
	for i, v := range vs {
		v.CheckedAt = zeroTime
		out[i] = v
	}
	return out
}

// Digest digests the run record including the audit sequence (stage, event,
// actor, refs, detail — not times or the hash chain, which derive from
// times, nor the impact report's wall-clock stamps or the validators'
// CheckedAt). Same inputs, config and scripted/replay agent ⇒ same digest,
// in-process or under Temporal.
func Digest(inv *domain.Investigation) string {
	c := *inv
	c.CreatedAt, c.ReportStamps = zeroTime, nil
	c.Validations = untimed(inv.Validations)
	c.Audit = make([]domain.AuditEvent, len(inv.Audit))
	for i, e := range inv.Audit {
		e.At, e.Prev = zeroTime, ""
		c.Audit[i] = e
	}
	return digestOf(c)
}

var zeroTime time.Time
