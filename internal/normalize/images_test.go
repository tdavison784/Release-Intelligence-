package normalize

import (
	"reflect"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestParseImageRef(t *testing.T) {
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tests := []struct {
		in      string
		want    domain.ImageRef
		wantErr bool
	}{
		{in: "quay.io/jetstack/cert-manager-controller:v1.18.0", want: domain.ImageRef{Repository: "quay.io/jetstack/cert-manager-controller", Tag: "v1.18.0"}},
		{in: "quay.io/jetstack/cert-manager-controller", want: domain.ImageRef{Repository: "quay.io/jetstack/cert-manager-controller"}},
		{in: "nginx", want: domain.ImageRef{Repository: "nginx"}},
		{in: "nginx:1.25", want: domain.ImageRef{Repository: "nginx", Tag: "1.25"}},
		{in: "localhost:5000/app", want: domain.ImageRef{Repository: "localhost:5000/app"}},
		{in: "localhost:5000/app:dev", want: domain.ImageRef{Repository: "localhost:5000/app", Tag: "dev"}},
		{in: "registry.example.com:8443/team/app/sub:1.0.0-rc.1", want: domain.ImageRef{Repository: "registry.example.com:8443/team/app/sub", Tag: "1.0.0-rc.1"}},
		{in: "repo/app@" + digest, want: domain.ImageRef{Repository: "repo/app", Digest: digest}},
		{in: "repo/app:v1@" + digest, want: domain.ImageRef{Repository: "repo/app", Tag: "v1", Digest: digest}},
		{in: "  docker.io/library/busybox:latest  ", want: domain.ImageRef{Repository: "docker.io/library/busybox", Tag: "latest"}},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "repo/app@notadigest", wantErr: true},
		{in: "repo/app@sha256:abc", wantErr: true},
		{in: "repo/app:", wantErr: true},
		{in: "repo//app", wantErr: true},
		{in: "/leading", wantErr: true},
		{in: "has space/app", wantErr: true},
		{in: "UPPER/app:tag with space", wantErr: true},
		{in: "http://example.com/app", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseImageRef(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("got %+v err=%v, want %+v", got, err, tt.want)
			}
		})
	}
}

func TestImageRefs(t *testing.T) {
	src := `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: a
          image: "quay.io/jetstack/cert-manager-controller:v1.18.0"
          args:
          - --v=2
          - --acme-http01-solver-image=quay.io/jetstack/cert-manager-acmesolver:v1.18.0
          - "--cluster-resource-namespace=ns"
        - name: b
          image: busybox:1.36   # trailing comment
        - image: 'docker.io/library/nginx@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'
      initContainers:
      - name: c
        image: registry.local:5000/init:latest
      # image: commented/out:1
      imagePullPolicy: Always
      imagePullSecrets: []
      defaultImage: not/an/image:key
      templated: 1
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
        image: ${IMAGE}
        image: $(IMG):tag
        image:
        image: ""
  json: {"image": "ghcr.io/example/json:v2", "name": "x"}
  duplicate:
    image: busybox:1.36
args2: ["--webhook-image=quay.io/example/webhook:v9", "--image=example/plain:1"]
`
	got := ImageRefs([]byte(src))
	want := []domain.ImageRef{
		{Repository: "busybox", Tag: "1.36"},
		{Repository: "docker.io/library/nginx", Digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		{Repository: "example/plain", Tag: "1"},
		{Repository: "ghcr.io/example/json", Tag: "v2"},
		{Repository: "quay.io/example/webhook", Tag: "v9"},
		{Repository: "quay.io/jetstack/cert-manager-acmesolver", Tag: "v1.18.0"},
		{Repository: "quay.io/jetstack/cert-manager-controller", Tag: "v1.18.0"},
		{Repository: "registry.local:5000/init", Tag: "latest"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%+v\nwant:\n%+v", got, want)
	}
	if got := ImageRefs(nil); len(got) != 0 {
		t.Errorf("nil input: %v", got)
	}
}

func TestImageRefsInstallManifestFixture(t *testing.T) {
	got := ImageRefs(fixture(t, "certmanager/install-deployments.yaml"))
	want := []domain.ImageRef{
		{Repository: "quay.io/jetstack/cert-manager-acmesolver", Tag: "v1.18.0"},
		{Repository: "quay.io/jetstack/cert-manager-cainjector", Tag: "v1.18.0"},
		{Repository: "quay.io/jetstack/cert-manager-controller", Tag: "v1.18.0"},
		{Repository: "quay.io/jetstack/cert-manager-webhook", Tag: "v1.18.0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// the chart values keep repository and tag apart: no `image: <ref>` lines
	if got := ImageRefs(fixture(t, "certmanager/values.yaml")); len(got) != 0 {
		t.Errorf("values.yaml: %+v", got)
	}
}
