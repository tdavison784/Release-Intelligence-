package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// KindDriver runs throwaway clusters with kind and talks to them with
// kubectl. Each cluster gets its own kubeconfig file (the user's
// ~/.kube/config is never touched), removed with the cluster.
type KindDriver struct {
	Kind, Kubectl, Docker string // binaries (looked up on PATH)
	// Dir holds the kubeconfig files ("" = a fresh directory under the OS temp dir).
	Dir string
	// DiskPath is where free disk is measured ("" = the user's home, where
	// Docker Desktop keeps its disk image; the docker root dir on Linux).
	DiskPath string
}

// NewKindDriver returns the kind driver with the default binaries.
func NewKindDriver() *KindDriver {
	return &KindDriver{Kind: "kind", Kubectl: "kubectl", Docker: "docker"}
}

func run(ctx context.Context, stdin []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if ctx.Err() != nil {
			return out.Bytes(), fmt.Errorf("%s %s: %w (%s)", name, firstArgs(args), ctx.Err(), clip(msg, 300))
		}
		if msg == "" {
			msg = err.Error()
		}
		return out.Bytes(), errors.New(msg)
	}
	return out.Bytes(), nil
}

func firstArgs(args []string) string {
	if len(args) > 3 {
		args = args[:3]
	}
	return strings.Join(args, " ")
}

// Preflight checks the binaries and the docker daemon.
func (d *KindDriver) Preflight(ctx context.Context) (map[string]string, error) {
	tools := map[string]string{}
	for _, bin := range []string{d.Kind, d.Kubectl, d.Docker} {
		if _, err := exec.LookPath(bin); err != nil {
			return tools, fmt.Errorf("%s is not on PATH", bin)
		}
	}
	vctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if out, err := run(vctx, nil, d.Kind, "version"); err == nil {
		tools["kind"] = strings.TrimSpace(string(out))
	}
	if out, err := run(vctx, nil, d.Kubectl, "version", "--client", "-o", "json"); err == nil {
		var v struct {
			ClientVersion struct {
				GitVersion string `json:"gitVersion"`
			} `json:"clientVersion"`
		}
		if json.Unmarshal(out, &v) == nil {
			tools["kubectl"] = v.ClientVersion.GitVersion
		}
	}
	out, err := run(vctx, nil, d.Docker, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return tools, fmt.Errorf("the docker daemon is not reachable: %v", err)
	}
	tools["docker"] = strings.TrimSpace(string(out))
	return tools, nil
}

// FreeDisk measures free bytes at DiskPath.
func (d *KindDriver) FreeDisk(ctx context.Context) (uint64, error) {
	p := d.DiskPath
	if p == "" {
		if h, err := os.UserHomeDir(); err == nil {
			p = h
		} else {
			p = os.TempDir()
		}
	}
	return freeDisk(p)
}

// Clusters lists kind clusters.
func (d *KindDriver) Clusters(ctx context.Context) ([]string, error) {
	out, err := run(ctx, nil, d.Kind, "get", "clusters")
	if err != nil {
		if strings.Contains(err.Error(), "No kind clusters found") {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "No kind clusters") {
			names = append(names, l)
		}
	}
	return names, nil
}

func (d *KindDriver) kubeconfig(name string) (string, error) {
	dir := d.Dir
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "ri-runtime-kubeconfig")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".kubeconfig"), nil
}

// Create creates the cluster and waits for its control plane.
func (d *KindDriver) Create(ctx context.Context, spec ClusterSpec) (Cluster, error) {
	kc, err := d.kubeconfig(spec.Name)
	if err != nil {
		return nil, err
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	args := []string{"create", "cluster", "--name", spec.Name, "--kubeconfig", kc, "--wait", "120s"}
	if spec.NodeImage != "" {
		args = append(args, "--image", spec.NodeImage)
	}
	if _, err := run(ctx, nil, d.Kind, args...); err != nil {
		return nil, err
	}
	return &kindCluster{name: spec.Name, kubeconfig: kc, kubectl: d.Kubectl}, nil
}

// Delete deletes the cluster and its kubeconfig file.
func (d *KindDriver) Delete(ctx context.Context, name string) error {
	kc, _ := d.kubeconfig(name)
	_, err := run(ctx, nil, d.Kind, "delete", "cluster", "--name", name, "--kubeconfig", kc)
	if kc != "" {
		_ = os.Remove(kc)
	}
	return err
}

type kindCluster struct {
	name, kubeconfig, kubectl string
}

func (c *kindCluster) Name() string { return c.name }

func (c *kindCluster) k(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	return run(ctx, stdin, c.kubectl, append([]string{"--kubeconfig", c.kubeconfig, "--request-timeout=60s"}, args...)...)
}

// resource is the kubectl type argument of a ref: "<kind>.<version>.<group>"
// (fully qualified, so a kind served by two groups is never ambiguous), or
// the kind for the core group.
func resource(g GVK) string {
	if g.Group == "" {
		return strings.ToLower(g.Kind)
	}
	return strings.ToLower(g.Kind) + "." + g.Version + "." + g.Group
}

func nsArgs(ns string) []string {
	if ns == "" {
		return nil
	}
	return []string{"-n", ns}
}

func (c *kindCluster) Apply(ctx context.Context, m Manifest, namespace string) (map[string]any, error) {
	doc, err := m.YAML()
	if err != nil {
		return nil, err
	}
	args := []string{"apply", "--server-side", "--force-conflicts", "--field-manager=ri-runtime", "-o", "json", "-f", "-"}
	if m.Ref.Namespace == "" {
		args = append(args, nsArgs(namespace)...)
	}
	out, err := c.k(ctx, doc, args...)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		return nil, fmt.Errorf("apply %s: unreadable response: %w", m.Ref, err)
	}
	return obj, nil
}

func (c *kindCluster) Delete(ctx context.Context, ref ObjectRef) error {
	args := append([]string{"delete", resource(ref.GVK()), ref.Name, "--ignore-not-found", "--wait=true", "--timeout=60s"}, nsArgs(ref.Namespace)...)
	_, err := c.k(ctx, nil, args...)
	if err != nil && unservedApply.MatchString(err.Error()) {
		return nil
	}
	return err
}

func (c *kindCluster) List(ctx context.Context, g GVK) ([]map[string]any, error) {
	out, err := c.k(ctx, nil, "get", resource(g), "-A", "-o", "json")
	if err != nil {
		if strings.Contains(err.Error(), "doesn't have a resource type") || unservedApply.MatchString(err.Error()) {
			return nil, ErrUnservedKind
		}
		return nil, err
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("list %s: %w", g, err)
	}
	return list.Items, nil
}

func (c *kindCluster) Patch(ctx context.Context, ref ObjectRef, subresource string, patch []byte) error {
	args := append([]string{"patch", resource(ref.GVK()), ref.Name, "--type=merge", "-p", string(patch)}, nsArgs(ref.Namespace)...)
	if subresource != "" {
		args = append(args, "--subresource="+subresource)
	}
	_, err := c.k(ctx, nil, args...)
	return err
}

func (c *kindCluster) Logs(ctx context.Context, namespace, pod string, since time.Time) (string, error) {
	out, err := c.k(ctx, nil, "logs", "-n", namespace, pod, "--all-containers=true", "--ignore-errors=true",
		"--since-time="+since.UTC().Format(time.RFC3339))
	return string(out), err
}
