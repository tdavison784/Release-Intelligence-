package reviewui

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// seedStore writes the demo fixtures into a real file-backed knowledge store.
func seedStore(t *testing.T, s knowledge.Store, d *DemoQueue) {
	t.Helper()
	put := func(e any) {
		rec, err := domain.NewRecord(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Put(context.Background(), rec); err != nil {
			t.Fatalf("put %T: %v", e, err)
		}
	}
	for _, c := range d.candidates {
		put(c)
	}
	for _, p := range d.proposals {
		put(p)
	}
	for _, v := range d.validations {
		put(v)
	}
	for _, id := range d.order {
		put(d.items[id])
	}
}

// The UI against the knowledge lane's real FileStore-backed Queue: inbox,
// item page, an individual decision and a bulk batch (one Decide call).
func TestAgainstTheFileStoreQueue(t *testing.T) {
	demo := NewDemoQueue()
	store := knowledge.NewFileStore(t.TempDir())
	seedStore(t, store, demo)
	q := knowledge.NewQueue(store, func() time.Time { return now })
	srv := httptest.NewServer(NewServer(q, Options{Now: func() time.Time { return now }, Logf: t.Logf}))
	defer srv.Close()
	r := &rig{t: t, q: demo, srv: srv, c: newRig(t).c}

	st, body := r.get("/")
	if st != 200 {
		t.Fatal(st, body)
	}
	contains(t, body, "The default privateKey.rotationPolicy is now Always", "Default of --max-concurrent-challenges")
	id := r.id("rotationPolicy")
	if st, body := r.get("/items/" + id); st != 200 {
		t.Fatal(st, body)
	} else {
		contains(t, body, "claude-opus-5-5", "glm-5.3-flash", "semvalidate.crd@v1")
	}
	ids := r.routine()
	if st, body, _ := r.post("/items/"+ids[0]+"/decision", decisionForm("reject", "reason", "no")); st != 303 {
		t.Fatalf("%d %s", st, body)
	}
	if st, body, _ := r.post("/bulk", bulkForm("defer", ids[1:4], "confirm", "1")); st != 303 {
		t.Fatalf("%d %s", st, body)
	}
	snap, err := store.Load(context.Background(), knowledge.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Decisions) != 4 {
		t.Fatalf("%d decisions stored", len(snap.Decisions))
	}
	batch := ""
	for _, d := range snap.Decisions {
		if d.BatchID != "" {
			if batch != "" && batch != d.BatchID {
				t.Error("two batch ids")
			}
			batch = d.BatchID
			if d.BatchSize != 3 || d.ReviewerKind != domain.ReviewerHuman {
				t.Errorf("%+v", d)
			}
		}
	}
	if batch == "" {
		t.Error("no batch recorded")
	}
	// a correction through the real queue (the domain checks the original against the item)
	cid := r.id("RBAC default policy")
	f := decisionForm("correct", "reason", "config key", "c_subject_family", "config-key", "c_subject_product", "argo-cd", "c_subject_path", "server.rbac.policy.default",
		"c_change_type", "renamed", "c_change_replacedby", `{"family":"config-key","product":"argo-cd","path":"configs.rbac.policy.default"}`)
	if st, body, _ := r.post("/items/"+cid+"/decision", f); st != 303 {
		t.Fatalf("%d %s", st, body)
	}
}

// A prose-only correction through the real queue (contract-5).
func TestProseOnlyCorrectionAgainstTheFileStoreQueue(t *testing.T) {
	demo := NewDemoQueue()
	store := knowledge.NewFileStore(t.TempDir())
	seedStore(t, store, demo)
	q := knowledge.NewQueue(store, func() time.Time { return now })
	srv := httptest.NewServer(NewServer(q, Options{Now: func() time.Time { return now }, Logf: t.Logf}))
	defer srv.Close()
	r := &rig{t: t, q: demo, srv: srv, c: newRig(t).c}
	id := r.id("rotationPolicy")
	it, _ := demo.ItemRecord(id)
	c := it.Proposed.Consequence
	f := decisionForm("correct", "reason", "better words", "c_cons_kind", string(c.Kind), "c_cons_severity", string(c.Severity),
		"c_cons_statement", "Keys rotate on every renewal; update pinned clients first.", "c_cons_remediation", c.Remediation)
	if st, body, _ := r.post("/items/"+id+"/decision", f); st != 303 {
		t.Fatalf("%d %s", st, body)
	}
	snap, _ := store.Load(context.Background(), knowledge.Query{})
	if len(snap.Decisions) != 1 || len(snap.Decisions[0].Labels) != 2 || snap.Decisions[0].Labels[1] != domain.LabelImprovedStatement {
		t.Fatalf("%+v", snap.Decisions)
	}
}
