package runtime

import (
	"context"
	"errors"
	"time"
)

// Driver creates and deletes throwaway clusters (the kind implementation is
// KindDriver; tests use a fake). It is the only place that knows about
// docker, kind or kubectl.
type Driver interface {
	// Preflight reports why no cluster can be created here (a missing tool,
	// an unreachable docker daemon) as an error, and the tool versions used.
	Preflight(ctx context.Context) (tools map[string]string, err error)
	// FreeDisk returns the free bytes where the cluster's images and
	// volumes live.
	FreeDisk(ctx context.Context) (uint64, error)
	// Clusters lists the existing clusters by name.
	Clusters(ctx context.Context) ([]string, error)
	// Create creates a cluster and waits until its API server answers. On
	// error the caller still calls Delete (a half-created cluster).
	Create(ctx context.Context, spec ClusterSpec) (Cluster, error)
	// Delete deletes a cluster; deleting an absent cluster is not an error.
	Delete(ctx context.Context, name string) error
}

// ClusterSpec describes the cluster to create.
type ClusterSpec struct {
	Name string
	// NodeImage pins the node image (and so the Kubernetes version); "" is
	// the driver's default.
	NodeImage string
	Timeout   time.Duration
}

// Cluster is one running cluster. Every call is bounded by ctx.
type Cluster interface {
	Name() string
	// Apply server-side applies one object (field manager ri-runtime,
	// conflicts forced, as a GitOps sync would) and returns the object as
	// the API server stored it. namespace is used for a namespaced object
	// that names none. The error text is the API server's (admission
	// rejections, schema errors, webhook failures).
	Apply(ctx context.Context, m Manifest, namespace string) (map[string]any, error)
	// Delete deletes an object and waits for it to be gone; absent is fine.
	Delete(ctx context.Context, ref ObjectRef) error
	// List lists every object of a type across all namespaces;
	// ErrUnservedKind when the cluster does not serve the type.
	List(ctx context.Context, gvk GVK) ([]map[string]any, error)
	// Patch applies a JSON merge patch to an object or (subresource
	// "status") to its status.
	Patch(ctx context.Context, ref ObjectRef, subresource string, mergePatch []byte) error
	// Logs returns the logs of every container of a pod since a time.
	Logs(ctx context.Context, namespace, pod string, since time.Time) (string, error)
}

// ErrUnservedKind reports a type the cluster does not serve (no built-in API
// and no CRD defines it).
var ErrUnservedKind = errors.New("runtime: the cluster serves no such type")
