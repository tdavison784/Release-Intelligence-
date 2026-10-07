package app

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/render"
)

func TestValidatorsRegistersRenderedDiff(t *testing.T) {
	a := newReplayApp(t, fixedNow)
	names := map[string]bool{}
	for _, v := range a.Validators("1.31") {
		if names[v.Name()] {
			t.Errorf("validator %s registered twice", v.Name())
		}
		names[v.Name()] = true
	}
	for _, want := range []string{"semvalidate.crd@v1", "semvalidate.canonical@v1", render.ValidatorName} {
		if !names[want] {
			t.Errorf("validator %s is not registered (have %v)", want, names)
		}
	}
}
