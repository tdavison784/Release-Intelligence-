package bundle

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

const valuesApp = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: NAME
spec:
  sources:
    - repoURL: https://charts.example.com
      chart: NAME
      targetRevision: 1.0.0
      helm:
        valueFiles:
          - $values/platform/NAME/values.yaml
          - $values/platform/NAME/values-prod.yaml
    - repoURL: https://git.example.com/gitops.git
      ref: values
`

// Values are per release: the product's own release, fully read, makes the
// repo input complete for it even with several values files in the repo
// (formerly "repo mode found N values files" — always partial); another
// product's unreadable values are not this product's gap; and two releases
// of the product with different values stay a gap with the reason.
func TestSnapshotValuesArePerRelease(t *testing.T) {
	repo := t.TempDir()
	for _, p := range []string{"cert-manager", "ingress-nginx"} {
		write(t, repo, "clusters/prod/"+p+".yaml", strings.ReplaceAll(valuesApp, "NAME", p))
		write(t, repo, "platform/"+p+"/values.yaml", "replicaCount: 1\n")
		write(t, repo, "platform/"+p+"/values-prod.yaml", "replicaCount: 2\n")
	}
	write(t, repo, "flux/other.yaml", `apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: redis
spec:
  chart:
    spec:
      chart: redis
  valuesFrom:
    - kind: Secret
      name: redis-values
`)
	build := func(product string) (domain.CollectionStatus, string) {
		res, err := NewSnapshotBuilder(newSt(t)).BuildSnapshot(context.Background(), investigate.EnvironmentInputs{Name: "e", Repo: repo, Product: product})
		if err != nil {
			t.Fatal(err)
		}
		return repoStatus(t, res)
	}
	if st, why := build("cert-manager"); st != domain.CollectionOK {
		t.Fatalf("cert-manager: repo status = %s (%s), want ok", st, why)
	}
	if st, why := build("redis"); st != domain.CollectionPartial || !strings.Contains(why, "valuesFrom") {
		t.Fatalf("redis: repo status = %s (%s), want partial (its valuesFrom is a cluster Secret)", st, why)
	}
	write(t, repo, "clusters/dev/cert-manager.yaml", strings.ReplaceAll(strings.ReplaceAll(valuesApp, "values-prod.yaml", "values-dev.yaml"), "NAME", "cert-manager"))
	write(t, repo, "platform/cert-manager/values-dev.yaml", "replicaCount: 0\n")
	if st, why := build("cert-manager"); st != domain.CollectionPartial || !strings.Contains(why, "2 releases deploy cert-manager with different effective values") {
		t.Fatalf("two cert-manager releases: repo status = %s (%s), want partial with the reason", st, why)
	}
}
