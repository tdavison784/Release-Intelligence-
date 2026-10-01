package upgrade

import "testing"

func TestDedupeKeys(t *testing.T) {
	a := alnumKey(normalizeNoteText("When configuring sidecar proxies if a hostname exists"))
	b := alnumKey(normalizeNoteText("When configuring sidecar proxies, if a hostname exists."))
	if a != b {
		t.Fatalf("punctuation must not matter: %q vs %q", a, b)
	}
	if headingTitle("Untaint controller: If you enabled the untaint controller") != headingTitle("Untaint controller: The `PILOT_ENABLE...` variable is now automatic") {
		t.Fatal("same heading title must produce the same key")
	}
	if headingTitle("Fix a bug. Then: more") != "" || headingTitle("Single: word") != "" {
		t.Fatal("non-heading prefixes must not produce a title key")
	}
}
