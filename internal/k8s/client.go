package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client abstracts Kubernetes API operations for both core resources and Deckhouse CRDs.
type Client interface {
	// Core resources (typed).
	ListNodes(ctx context.Context) ([]corev1.Node, error)
	GetNode(ctx context.Context, name string) (*corev1.Node, error)
	CordonNode(ctx context.Context, name string) error
	UncordonNode(ctx context.Context, name string) error
	ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error)
	DeletePod(ctx context.Context, namespace, name string) error
	EvictPod(ctx context.Context, namespace, name string) error
	ListNodeEvents(ctx context.Context, nodeName string) ([]corev1.Event, error)
	GetPodLogs(ctx context.Context, namespace, pod, container string, tail *int64, since *string) (string, error)
	GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error)
	UpdateSecret(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error)

	// Deckhouse CRDs (dynamic/unstructured).
	ListNodeGroups(ctx context.Context) ([]unstructured.Unstructured, error)
	GetNodeGroup(ctx context.Context, name string) (*unstructured.Unstructured, error)
	CreateNodeGroup(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
	DeleteNodeGroup(ctx context.Context, name string) error
	ListStaticInstances(ctx context.Context) ([]unstructured.Unstructured, error)
	GetStaticInstance(ctx context.Context, name string) (*unstructured.Unstructured, error)
	CreateStaticInstance(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
	DeleteStaticInstance(ctx context.Context, name string) error
	ListModuleConfigs(ctx context.Context) ([]unstructured.Unstructured, error)
	GetModuleConfig(ctx context.Context, name string) (*unstructured.Unstructured, error)
	UpdateModuleConfig(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
	ListDeckhouseReleases(ctx context.Context) ([]unstructured.Unstructured, error)
	GetDeckhouseRelease(ctx context.Context, name string) (*unstructured.Unstructured, error)
	PatchDeckhouseRelease(ctx context.Context, name string, patch []byte) (*unstructured.Unstructured, error)
	CreateSSHCredentials(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
	DeleteSSHCredentials(ctx context.Context, name string) error
	ListModules(ctx context.Context) ([]unstructured.Unstructured, error)
	ListModuleSources(ctx context.Context) ([]unstructured.Unstructured, error)
	CreateModuleSource(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
	DeleteModuleSource(ctx context.Context, name string) error
	ListModuleUpdatePolicies(ctx context.Context) ([]unstructured.Unstructured, error)
	CreateModuleUpdatePolicy(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
	ListModuleReleases(ctx context.Context, moduleName string) ([]unstructured.Unstructured, error)
	CreateNodeGroupConfiguration(ctx context.Context, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)
	PatchModuleConfig(ctx context.Context, name string, patch []byte) (*unstructured.Unstructured, error)
}

// GVR constants for Deckhouse CRDs.
var (
	NodeGroupGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1",
		Resource: "nodegroups",
	}
	StaticInstanceGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha2",
		Resource: "staticinstances",
	}
	SSHCredentialsGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha2",
		Resource: "sshcredentials",
	}
	ModuleConfigGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha1",
		Resource: "moduleconfigs",
	}
	DeckhouseReleaseGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha1",
		Resource: "deckhousereleases",
	}
	ModuleGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha1",
		Resource: "modules",
	}
	ModuleSourceGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha1",
		Resource: "modulesources",
	}
	ModuleUpdatePolicyGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha1",
		Resource: "moduleupdatepolicies",
	}
	ModuleReleaseGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha1",
		Resource: "modulereleases",
	}
	NodeGroupConfigurationGVR = schema.GroupVersionResource{
		Group:    "deckhouse.io",
		Version:  "v1alpha1",
		Resource: "nodegroupconfigurations",
	}
)

type client struct {
	typed   kubernetes.Interface
	dynamic dynamic.Interface
}

// New creates a new Client from the given rest.Config.
func New(cfg *rest.Config) (Client, error) {
	typedClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating typed client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	return &client{
		typed:   typedClient,
		dynamic: dynamicClient,
	}, nil
}

// ListNodes returns all cluster nodes.
func (c *client) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	list, err := c.typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	return list.Items, nil
}

// ListPods returns pods in the given namespace (empty string means all namespaces).
func (c *client) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	list, err := c.typed.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing pods in %q: %w", namespace, err)
	}

	return list.Items, nil
}

// IsCRDNotRegistered reports whether err indicates that a Deckhouse CRD is not
// installed in the cluster — typically because its owning module (e.g.
// node-manager) is disabled. Callers can use this to degrade gracefully instead
// of failing a whole aggregate operation.
func IsCRDNotRegistered(err error) bool {
	if err == nil {
		return false
	}

	return kerrors.IsNotFound(err) ||
		strings.Contains(err.Error(), "could not find the requested resource")
}

// ListNodeGroups returns all NodeGroup resources.
//
// Errors are returned without an operation prefix; the calling handler adds one
// ("listing node groups: %w") so the message is not wrapped twice.
func (c *client) ListNodeGroups(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.dynamic.Resource(NodeGroupGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		// Make the common "CRD not installed" case more actionable for users.
		if IsCRDNotRegistered(err) {
			return nil, fmt.Errorf("CRD deckhouse.io/v1/nodegroups not registered (is node-manager module enabled?): %w", err)
		}
		return nil, err
	}

	return list.Items, nil
}

// ListStaticInstances returns all StaticInstance resources.
//
// Errors are returned without an operation prefix; the calling handler adds one.
func (c *client) ListStaticInstances(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.dynamic.Resource(StaticInstanceGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if IsCRDNotRegistered(err) {
			return nil, fmt.Errorf("CRD deckhouse.io/v1alpha2/staticinstances not registered (is node-manager module enabled?): %w", err)
		}
		return nil, err
	}

	return list.Items, nil
}

// GetStaticInstance returns a single StaticInstance by name.
//
// The error is returned unwrapped; the calling handler adds the operation prefix.
func (c *client) GetStaticInstance(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	obj, err := c.dynamic.Resource(StaticInstanceGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// CreateStaticInstance creates a StaticInstance resource.
func (c *client) CreateStaticInstance(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	created, err := c.dynamic.Resource(StaticInstanceGVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating static instance: %w", err)
	}

	return created, nil
}

// ListModuleConfigs returns all ModuleConfig resources.
func (c *client) ListModuleConfigs(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.dynamic.Resource(ModuleConfigGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing module configs: %w", err)
	}

	return list.Items, nil
}

// ListDeckhouseReleases returns all DeckhouseRelease resources.
func (c *client) ListDeckhouseReleases(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.dynamic.Resource(DeckhouseReleaseGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing deckhouse releases: %w", err)
	}

	return list.Items, nil
}

// CreateSSHCredentials creates an SSHCredentials resource.
func (c *client) CreateSSHCredentials(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	created, err := c.dynamic.Resource(SSHCredentialsGVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating ssh credentials: %w", err)
	}

	return created, nil
}

// GetNode returns a single Node by name.
func (c *client) GetNode(ctx context.Context, name string) (*corev1.Node, error) {
	node, err := c.typed.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting node %q: %w", name, err)
	}

	return node, nil
}

// CordonNode marks a node as unschedulable.
func (c *client) CordonNode(ctx context.Context, name string) error {
	node, err := c.typed.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting node %q for cordon: %w", name, err)
	}

	node.Spec.Unschedulable = true

	_, err = c.typed.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating node %q: %w", name, err)
	}

	return nil
}

// GetPodLogs returns logs for a specific pod.
// container selects the container (empty = default).
// tail limits lines; since limits by duration (e.g. "30m").
func (c *client) GetPodLogs(
	ctx context.Context,
	namespace, pod, container string,
	tail *int64,
	since *string,
) (string, error) {
	opts := &corev1.PodLogOptions{Container: container}
	if tail != nil {
		opts.TailLines = tail
	}

	if since != nil {
		d, err := time.ParseDuration(*since)
		if err != nil {
			return "", fmt.Errorf("parsing since duration %q: %w", *since, err)
		}

		secs := int64(d.Seconds())
		opts.SinceSeconds = &secs
	}

	req := c.typed.CoreV1().Pods(namespace).GetLogs(pod, opts)

	logStream, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("opening log stream for pod %q: %w", pod, err)
	}

	defer func() { _ = logStream.Close() }()

	var buf bytes.Buffer

	_, err = io.Copy(&buf, logStream)
	if err != nil {
		return "", fmt.Errorf("reading log stream: %w", err)
	}

	return buf.String(), nil
}

// DeletePod deletes a single pod by namespace and name.
func (c *client) DeletePod(ctx context.Context, namespace, name string) error {
	err := c.typed.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("deleting pod %q/%q: %w", namespace, name, err)
	}

	return nil
}

const nodeEventListLimit = 10

// ListNodeEvents returns the last 10 events for a node (involvedObject.name=<nodeName>).
func (c *client) ListNodeEvents(ctx context.Context, nodeName string) ([]corev1.Event, error) {
	list, err := c.typed.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + nodeName,
		Limit:         nodeEventListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing events for node %q: %w", nodeName, err)
	}

	return list.Items, nil
}

// GetSecret returns a single Secret by namespace and name.
func (c *client) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	secret, err := c.typed.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting secret %q/%q: %w", namespace, name, err)
	}

	return secret, nil
}

// GetNodeGroup returns a single NodeGroup by name.
func (c *client) GetNodeGroup(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	obj, err := c.dynamic.Resource(NodeGroupGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting node group %q: %w", name, err)
	}

	return obj, nil
}

// CreateNodeGroup creates a NodeGroup resource.
func (c *client) CreateNodeGroup(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	created, err := c.dynamic.Resource(NodeGroupGVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating node group: %w", err)
	}

	return created, nil
}

// DeleteStaticInstance deletes a StaticInstance by name.
func (c *client) DeleteStaticInstance(ctx context.Context, name string) error {
	err := c.dynamic.Resource(StaticInstanceGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("deleting static instance %q: %w", name, err)
	}

	return nil
}

// GetModuleConfig returns a single ModuleConfig by name.
func (c *client) GetModuleConfig(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	obj, err := c.dynamic.Resource(ModuleConfigGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting module config %q: %w", name, err)
	}

	return obj, nil
}

// UpdateModuleConfig replaces a ModuleConfig resource.
func (c *client) UpdateModuleConfig(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	updated, err := c.dynamic.Resource(ModuleConfigGVR).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("updating module config: %w", err)
	}

	return updated, nil
}

// GetDeckhouseRelease returns a single DeckhouseRelease by name.
func (c *client) GetDeckhouseRelease(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	obj, err := c.dynamic.Resource(DeckhouseReleaseGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting deckhouse release %q: %w", name, err)
	}

	return obj, nil
}

// PatchDeckhouseRelease applies a merge patch to a DeckhouseRelease.
func (c *client) PatchDeckhouseRelease(
	ctx context.Context,
	name string,
	patch []byte,
) (*unstructured.Unstructured, error) {
	obj, err := c.dynamic.Resource(DeckhouseReleaseGVR).Patch(
		ctx, name, types.MergePatchType, patch, metav1.PatchOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("patching deckhouse release %q: %w", name, err)
	}

	return obj, nil
}

// ListModules returns all Module resources (runtime view of installed modules).
func (c *client) ListModules(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.dynamic.Resource(ModuleGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing modules: %w", err)
	}

	return list.Items, nil
}

// UncordonNode marks a node as schedulable (Spec.Unschedulable = false).
func (c *client) UncordonNode(ctx context.Context, name string) error {
	node, err := c.typed.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting node %q for uncordon: %w", name, err)
	}

	node.Spec.Unschedulable = false

	_, err = c.typed.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating node %q: %w", name, err)
	}

	return nil
}

// EvictPod issues a policy/v1 Eviction request for a pod, respecting any
// PodDisruptionBudget. Callers should classify the returned error via
// k8s.io/apimachinery/pkg/api/errors helpers.
func (c *client) EvictPod(ctx context.Context, namespace, name string) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}

	err := c.typed.PolicyV1().Evictions(namespace).Evict(ctx, eviction)
	if err != nil {
		return fmt.Errorf("evicting pod %q/%q: %w", namespace, name, err)
	}

	return nil
}

// UpdateSecret replaces an existing Secret. Caller must supply the same
// ResourceVersion they observed via GetSecret to enable optimistic concurrency.
func (c *client) UpdateSecret(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error) {
	updated, err := c.typed.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("updating secret %q/%q: %w", secret.Namespace, secret.Name, err)
	}

	return updated, nil
}

// DeleteSSHCredentials deletes an SSHCredentials resource by name.
func (c *client) DeleteSSHCredentials(ctx context.Context, name string) error {
	err := c.dynamic.Resource(SSHCredentialsGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("deleting ssh credentials %q: %w", name, err)
	}

	return nil
}

// DeleteNodeGroup deletes a NodeGroup resource by name.
func (c *client) DeleteNodeGroup(ctx context.Context, name string) error {
	err := c.dynamic.Resource(NodeGroupGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("deleting node group %q: %w", name, err)
	}

	return nil
}

// ListModuleSources returns all ModuleSource resources.
func (c *client) ListModuleSources(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.dynamic.Resource(ModuleSourceGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing module sources: %w", err)
	}

	return list.Items, nil
}

// CreateModuleSource creates a ModuleSource resource.
func (c *client) CreateModuleSource(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	created, err := c.dynamic.Resource(ModuleSourceGVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating module source: %w", err)
	}

	return created, nil
}

// ListModuleUpdatePolicies returns all ModuleUpdatePolicy resources.
func (c *client) ListModuleUpdatePolicies(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.dynamic.Resource(ModuleUpdatePolicyGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing module update policies: %w", err)
	}

	return list.Items, nil
}

// CreateModuleUpdatePolicy creates a ModuleUpdatePolicy resource.
func (c *client) CreateModuleUpdatePolicy(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	created, err := c.dynamic.Resource(ModuleUpdatePolicyGVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating module update policy: %w", err)
	}

	return created, nil
}

// DeleteModuleSource deletes a ModuleSource resource by name.
func (c *client) DeleteModuleSource(ctx context.Context, name string) error {
	err := c.dynamic.Resource(ModuleSourceGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("deleting module source %q: %w", name, err)
	}

	return nil
}

// ListModuleReleases returns ModuleRelease resources. When moduleName is
// non-empty, results are filtered by the label selector "module=<moduleName>".
// An empty moduleName returns all releases — useful for label-agnostic
// post-filtering (e.g. by status or labels["source"]).
func (c *client) ListModuleReleases(
	ctx context.Context,
	moduleName string,
) ([]unstructured.Unstructured, error) {
	opts := metav1.ListOptions{}
	if moduleName != "" {
		opts.LabelSelector = "module=" + moduleName
	}

	list, err := c.dynamic.Resource(ModuleReleaseGVR).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("listing module releases for %q: %w", moduleName, err)
	}

	return list.Items, nil
}

// CreateNodeGroupConfiguration creates a NodeGroupConfiguration resource.
func (c *client) CreateNodeGroupConfiguration(
	ctx context.Context,
	obj *unstructured.Unstructured,
) (*unstructured.Unstructured, error) {
	created, err := c.dynamic.Resource(NodeGroupConfigurationGVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating node group configuration: %w", err)
	}

	return created, nil
}

// PatchModuleConfig applies a JSON merge patch to a ModuleConfig.
func (c *client) PatchModuleConfig(
	ctx context.Context,
	name string,
	patch []byte,
) (*unstructured.Unstructured, error) {
	obj, err := c.dynamic.Resource(ModuleConfigGVR).Patch(
		ctx, name, types.MergePatchType, patch, metav1.PatchOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("patching module config %q: %w", name, err)
	}

	return obj, nil
}
