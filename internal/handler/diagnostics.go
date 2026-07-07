package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"

	"github.com/easyp-tech/deckhouse-harness/internal/k8s"
	pb "github.com/easyp-tech/deckhouse-harness/proto/deckhouse/v1"
)

// DiagnosticsHandler implements pb.DiagnosticsAPIToolHandler.
type DiagnosticsHandler struct {
	client k8s.Client
}

// NewDiagnosticsHandler creates a new DiagnosticsHandler.
func NewDiagnosticsHandler(client k8s.Client) *DiagnosticsHandler {
	return &DiagnosticsHandler{client: client}
}

// Phase string constants used across handlers.
const (
	phaseRunning = "Running"
	phasePending = "Pending"
)

var errDeckhousePodNotFound = errors.New("deckhouse pod not found in d8-system namespace")

// clampInt32 converts int64 to int32, clamping at MaxInt32 to prevent overflow.
func clampInt32(value int64) int32 {
	const maxInt32 int64 = 1<<31 - 1
	if value > maxInt32 {
		return 1<<31 - 1
	}

	return int32(value) //nolint:gosec // value is bounded by prior check
}

func (h *DiagnosticsHandler) GetClusterStatus(
	ctx context.Context,
	_ *emptypb.Empty,
) (*pb.GetClusterStatusResponse, error) {
	nodeSummary, err := h.collectNodeSummary(ctx)
	if err != nil {
		return nil, err
	}

	ngStatuses, err := h.collectNodeGroupStatuses(ctx)
	if err != nil {
		return nil, err
	}

	erroredModules, err := h.collectErroredModules(ctx)
	if err != nil {
		return nil, err
	}

	pendingReleases, deckhouseVersion, err := h.collectReleaseInfo(ctx)
	if err != nil {
		return nil, err
	}

	unhealthyCount, err := h.countUnhealthyPods(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.GetClusterStatusResponse{
		Nodes:              nodeSummary,
		NodeGroups:         ngStatuses,
		ErroredModules:     erroredModules,
		PendingReleases:    pendingReleases,
		UnhealthyPodsCount: unhealthyCount,
		DeckhouseVersion:   deckhouseVersion,
	}, nil
}

func (h *DiagnosticsHandler) ListNodes(
	ctx context.Context,
	req *pb.ListNodesRequest,
) (*pb.ListNodesResponse, error) {
	nodes, err := h.client.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var result []*pb.NodeInfo

	for _, n := range nodes {
		info := nodeToInfo(&n)
		if matchesNodeFilter(info, req) {
			result = append(result, info)
		}
	}

	return &pb.ListNodesResponse{Nodes: result}, nil
}

func matchesNodeFilter(info *pb.NodeInfo, req *pb.ListNodesRequest) bool {
	if req.NodeGroup != nil && info.GetNodeGroup() != req.GetNodeGroup() {
		return false
	}

	if req.Status != nil && !matchesNodeStatusFilter(info.GetStatus(), req.GetStatus()) {
		return false
	}

	if req.Role != nil && info.GetRole() != req.GetRole() {
		return false
	}

	return true
}

func matchesNodeStatusFilter(status string, filter pb.NodeStatusFilter) bool {
	switch filter {
	case pb.NodeStatusFilter_NODE_STATUS_FILTER_READY:
		return status == "Ready"
	case pb.NodeStatusFilter_NODE_STATUS_FILTER_NOT_READY:
		return status == "NotReady"
	case pb.NodeStatusFilter_NODE_STATUS_FILTER_UNSPECIFIED,
		pb.NodeStatusFilter_NODE_STATUS_FILTER_ALL:
		return true
	}

	return true
}

func (h *DiagnosticsHandler) ListNodeGroups(
	ctx context.Context,
	_ *emptypb.Empty,
) (*pb.ListNodeGroupsResponse, error) {
	nodeGroups, err := h.client.ListNodeGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing node groups: %w", err)
	}

	var result []*pb.NodeGroupInfo

	for _, nodeGroup := range nodeGroups {
		name := unstructuredNestedString(nodeGroup.Object, "metadata", "name")
		nodeType := unstructuredNestedString(nodeGroup.Object, "spec", "nodeType")
		ngReady := unstructuredNestedInt64(nodeGroup.Object, "status", "ready")
		ngTotal := unstructuredNestedInt64(nodeGroup.Object, "status", "nodes")
		upToDate := unstructuredNestedInt64(nodeGroup.Object, "status", "upToDate")
		ngError := unstructuredNestedString(nodeGroup.Object, "status", "error")

		var conditions []*pb.Condition

		condSlice := unstructuredNestedSlice(nodeGroup.Object, "status", "conditions")
		for _, c := range condSlice {
			if cMap, ok := c.(map[string]any); ok {
				conditions = append(conditions, &pb.Condition{
					Type:    getString(cMap, "type"),
					Status:  getString(cMap, "status"),
					Message: getString(cMap, "message"),
				})
			}
		}

		result = append(result, &pb.NodeGroupInfo{
			Name:       name,
			NodeType:   nodeType,
			Ready:      clampInt32(ngReady),
			Total:      clampInt32(ngTotal),
			UpToDate:   clampInt32(upToDate),
			Conditions: conditions,
			Error:      ngError,
		})
	}

	return &pb.ListNodeGroupsResponse{NodeGroups: result}, nil
}

func (h *DiagnosticsHandler) ListStaticInstances(
	ctx context.Context,
	req *pb.ListStaticInstancesRequest,
) (*pb.ListStaticInstancesResponse, error) {
	instances, err := h.client.ListStaticInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing static instances: %w", err)
	}

	var result []*pb.StaticInstanceInfo

	for _, instance := range instances {
		name := unstructuredNestedString(instance.Object, "metadata", "name")
		address := unstructuredNestedString(instance.Object, "spec", "address")
		phase := unstructuredNestedString(instance.Object, "status", "currentStatus", "phase")
		nodeRef := unstructuredNestedString(instance.Object, "status", "nodeRef", "name")
		lastUpdate := unstructuredNestedString(instance.Object, "status", "currentStatus", "lastUpdateTime")

		labels := unstructuredNestedStringMap(instance.Object, "metadata", "labels")
		nodeGroup := labels["node.deckhouse.io/group"]

		if req.NodeGroup != nil && nodeGroup != req.GetNodeGroup() {
			continue
		}

		if req.Phase != nil && req.GetPhase() != pb.StaticInstancePhase_STATIC_INSTANCE_PHASE_UNSPECIFIED {
			if !strings.EqualFold(phase, phaseToString(req.GetPhase())) {
				continue
			}
		}

		result = append(result, &pb.StaticInstanceInfo{
			Name:           name,
			Address:        address,
			Phase:          phase,
			NodeRef:        nodeRef,
			LastUpdateTime: lastUpdate,
		})
	}

	return &pb.ListStaticInstancesResponse{Instances: result}, nil
}

func (h *DiagnosticsHandler) ListUnhealthyPods(
	ctx context.Context,
	req *pb.ListUnhealthyPodsRequest,
) (*pb.ListUnhealthyPodsResponse, error) {
	namespace := ""
	if req.Namespace != nil {
		namespace = req.GetNamespace()
	}

	pods, err := h.client.ListPods(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}

	excludeCompleted := req.ExcludeCompleted != nil && req.GetExcludeCompleted()

	var result []*pb.UnhealthyPodInfo

	for _, pod := range pods {
		info := toUnhealthyPodInfo(&pod, excludeCompleted)
		if info != nil {
			result = append(result, info)
		}
	}

	return &pb.ListUnhealthyPodsResponse{Pods: result}, nil
}

func toUnhealthyPodInfo(pod *corev1.Pod, excludeCompleted bool) *pb.UnhealthyPodInfo {
	phase := string(pod.Status.Phase)
	if phase == string(corev1.PodRunning) || phase == string(corev1.PodSucceeded) {
		return nil
	}

	if excludeCompleted && phase == string(corev1.PodSucceeded) {
		return nil
	}

	var restartCount int32
	for _, cs := range pod.Status.ContainerStatuses {
		restartCount += cs.RestartCount
	}

	reason := phase
	if pod.Status.Reason != "" {
		reason = pod.Status.Reason
	}

	age := time.Since(pod.CreationTimestamp.Time).Truncate(time.Second).String()

	return &pb.UnhealthyPodInfo{
		Name:         pod.Name,
		Namespace:    pod.Namespace,
		Status:       phase,
		Reason:       reason,
		RestartCount: restartCount,
		Age:          age,
	}
}

// Helper: determine if a node is Ready.
func isNodeReady(node *corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}

	return false
}

// Helper: determine if a pod is healthy.
func isPodHealthy(pod *corev1.Pod) bool {
	phase := pod.Status.Phase

	return phase == corev1.PodRunning || phase == corev1.PodSucceeded
}

// Helper: convert Node to NodeInfo proto.
func nodeToInfo(node *corev1.Node) *pb.NodeInfo {
	status := "NotReady"
	if isNodeReady(node) {
		status = "Ready"
	}

	var role string

	for label := range node.Labels {
		if after, ok := strings.CutPrefix(label, "node-role.kubernetes.io/"); ok {
			role = after

			break
		}
	}

	var internalIP string

	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			internalIP = addr.Address

			break
		}
	}

	age := time.Since(node.CreationTimestamp.Time).Truncate(time.Second).String()
	nodeGroup := node.Labels["node.deckhouse.io/group"]

	return &pb.NodeInfo{
		Name:           node.Name,
		Status:         status,
		Role:           role,
		InternalIp:     internalIP,
		OsImage:        node.Status.NodeInfo.OSImage,
		KubeletVersion: node.Status.NodeInfo.KubeletVersion,
		Age:            age,
		NodeGroup:      nodeGroup,
	}
}

// Helper: convert StaticInstancePhase enum to string.
func phaseToString(phase pb.StaticInstancePhase) string {
	switch phase {
	case pb.StaticInstancePhase_STATIC_INSTANCE_PHASE_PENDING:
		return phasePending
	case pb.StaticInstancePhase_STATIC_INSTANCE_PHASE_BOOTSTRAPPING:
		return "Bootstrapping"
	case pb.StaticInstancePhase_STATIC_INSTANCE_PHASE_RUNNING:
		return phaseRunning
	case pb.StaticInstancePhase_STATIC_INSTANCE_PHASE_CLEANING:
		return "Cleaning"
	case pb.StaticInstancePhase_STATIC_INSTANCE_PHASE_ERROR:
		return "Error"
	case pb.StaticInstancePhase_STATIC_INSTANCE_PHASE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

// Unstructured helpers to avoid repeating type assertions.
func unstructuredNestedString(obj map[string]any, fields ...string) string {
	val, found := nestedField(obj, fields...)
	if !found {
		return ""
	}

	s, _ := val.(string)

	return s
}

func unstructuredNestedInt64(obj map[string]any, fields ...string) int64 {
	val, found := nestedField(obj, fields...)
	if !found {
		return 0
	}

	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func unstructuredNestedSlice(obj map[string]any, fields ...string) []any {
	val, found := nestedField(obj, fields...)
	if !found {
		return nil
	}

	s, _ := val.([]any)

	return s
}

func unstructuredNestedStringMap(obj map[string]any, fields ...string) map[string]string {
	val, found := nestedField(obj, fields...)
	if !found {
		return nil
	}

	mapVal, ok := val.(map[string]any)
	if !ok {
		return nil
	}

	result := make(map[string]string, len(mapVal))
	for k, v := range mapVal {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}

	return result
}

func nestedField(obj map[string]any, fields ...string) (any, bool) {
	var current any = obj

	for _, field := range fields {
		cur, isMap := current.(map[string]any)
		if !isMap {
			return nil, false
		}

		val, exists := cur[field]
		if !exists {
			return nil, false
		}

		current = val
	}

	return current, true
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}

	return ""
}

// GetNode returns detailed information about a specific node.
func (h *DiagnosticsHandler) GetNode(ctx context.Context, req *pb.GetNodeRequest) (*pb.GetNodeResponse, error) {
	node, err := h.client.GetNode(ctx, req.GetName())
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", req.GetName(), err)
	}

	info := nodeToInfo(node)

	// Map node conditions.
	var conditions []*pb.NodeCondition
	for _, c := range node.Status.Conditions {
		conditions = append(conditions, &pb.NodeCondition{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Message: c.Message,
		})
	}

	// Map allocatable and capacity resources.
	allocatable := make(map[string]string)
	for res, qty := range node.Status.Allocatable {
		allocatable[string(res)] = qty.String()
	}

	capacity := make(map[string]string)
	for res, qty := range node.Status.Capacity {
		capacity[string(res)] = qty.String()
	}

	// Try to get StaticInstance phase for this node (may not exist for cloud nodes).
	var siPhase *string

	si, err := h.client.GetStaticInstance(ctx, req.GetName())
	if err == nil && si != nil {
		phase := unstructuredNestedString(si.Object, "status", "currentStatus", "phase")
		if phase != "" {
			siPhase = &phase
		}
	}
	// Ignore error — cloud nodes have no StaticInstance.

	// Fetch last 10 events for this node.
	rawEvents, _ := h.client.ListNodeEvents(ctx, req.GetName())

	var events []*pb.NodeEvent

	for _, event := range rawEvents {
		events = append(events, &pb.NodeEvent{
			Reason:   event.Reason,
			Message:  event.Message,
			Type:     event.Type,
			LastTime: event.LastTimestamp.UTC().Format(time.RFC3339),
			Count:    event.Count,
		})
	}

	return &pb.GetNodeResponse{
		Node:                info,
		Conditions:          conditions,
		Allocatable:         allocatable,
		Capacity:            capacity,
		StaticInstancePhase: siPhase,
		Events:              events,
	}, nil
}

// GetNodeGroup returns detailed information about a specific NodeGroup including member node names.
func (h *DiagnosticsHandler) GetNodeGroup(
	ctx context.Context,
	req *pb.GetNodeGroupRequest,
) (*pb.GetNodeGroupResponse, error) {
	nodeGroup, err := h.client.GetNodeGroup(ctx, req.GetName())
	if err != nil {
		return nil, fmt.Errorf("getting node group %s: %w", req.GetName(), err)
	}

	name := unstructuredNestedString(nodeGroup.Object, "metadata", "name")
	nodeType := unstructuredNestedString(nodeGroup.Object, "spec", "nodeType")
	ngReady := unstructuredNestedInt64(nodeGroup.Object, "status", "ready")
	ngTotal := unstructuredNestedInt64(nodeGroup.Object, "status", "nodes")
	upToDate := unstructuredNestedInt64(nodeGroup.Object, "status", "upToDate")
	statusMsg := unstructuredNestedString(nodeGroup.Object, "status", "error")

	// Collect node names that belong to this group.
	nodes, err := h.client.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing nodes for group %s: %w", req.GetName(), err)
	}

	var nodeNames []string

	for _, n := range nodes {
		if n.Labels["node.deckhouse.io/group"] == name {
			nodeNames = append(nodeNames, n.Name)
		}
	}

	return &pb.GetNodeGroupResponse{
		Name:          name,
		NodeType:      nodeType,
		Ready:         clampInt32(ngReady),
		Total:         clampInt32(ngTotal),
		UpToDate:      clampInt32(upToDate),
		StatusMessage: statusMsg,
		NodeNames:     nodeNames,
	}, nil
}

// GetDeckhouseLogs retrieves the Deckhouse controller pod logs with optional filtering.
func (h *DiagnosticsHandler) GetDeckhouseLogs(
	ctx context.Context,
	req *pb.GetDeckhouseLogsRequest,
) (*pb.GetDeckhouseLogsResponse, error) {
	pods, err := h.client.ListPods(ctx, "d8-system")
	if err != nil {
		return nil, fmt.Errorf("listing pods in d8-system: %w", err)
	}

	podName, err := findDeckhousePod(pods)
	if err != nil {
		return nil, err
	}

	var tailLines *int64

	if req.Tail != nil {
		lines := int64(req.GetTail())
		tailLines = &lines
	}

	var since *string
	if req.Since != nil {
		since = req.Since
	}

	logs, err := h.client.GetPodLogs(ctx, "d8-system", podName, "deckhouse", tailLines, since)
	if err != nil {
		return nil, fmt.Errorf("getting pod logs for %s: %w", podName, err)
	}

	return &pb.GetDeckhouseLogsResponse{Logs: applyGrepFilter(logs, req)}, nil
}

func findDeckhousePod(pods []corev1.Pod) (string, error) {
	for _, pod := range pods {
		if pod.Labels["app"] == "deckhouse" {
			return pod.Name, nil
		}
	}

	return "", errDeckhousePodNotFound
}

func applyGrepFilter(logs string, req *pb.GetDeckhouseLogsRequest) string {
	if req.Grep == nil || req.GetGrep() == "" {
		return logs
	}

	pattern := req.GetGrep()

	var filtered []string

	for line := range strings.SplitSeq(logs, "\n") {
		if strings.Contains(line, pattern) {
			filtered = append(filtered, line)
		}
	}

	var builder strings.Builder

	for _, line := range filtered {
		builder.WriteString(line)
		builder.WriteString("\n")
	}

	return builder.String()
}

// GetNodeEvents returns Kubernetes Events whose involvedObject.name matches the given node name.
func (h *DiagnosticsHandler) GetNodeEvents(
	ctx context.Context,
	req *pb.GetNodeEventsRequest,
) (*pb.GetNodeEventsResponse, error) {
	rawEvents, err := h.client.ListNodeEvents(ctx, req.GetName())
	if err != nil {
		return nil, fmt.Errorf("listing events for node %s: %w", req.GetName(), err)
	}

	events := make([]*pb.NodeEvent, 0, len(rawEvents))
	for _, event := range rawEvents {
		events = append(events, &pb.NodeEvent{
			Reason:   event.Reason,
			Message:  event.Message,
			Type:     event.Type,
			LastTime: event.LastTimestamp.UTC().Format(time.RFC3339),
			Count:    event.Count,
		})
	}

	return &pb.GetNodeEventsResponse{Events: events}, nil
}

// GetStaticInstance returns detailed information for a single StaticInstance resource.
func (h *DiagnosticsHandler) GetStaticInstance(
	ctx context.Context,
	req *pb.GetStaticInstanceRequest,
) (*pb.GetStaticInstanceResponse, error) {
	instance, err := h.client.GetStaticInstance(ctx, req.GetName())
	if err != nil {
		return nil, fmt.Errorf("getting static instance %s: %w", req.GetName(), err)
	}

	name := unstructuredNestedString(instance.Object, "metadata", "name")
	address := unstructuredNestedString(instance.Object, "spec", "address")
	phase := unstructuredNestedString(instance.Object, "status", "currentStatus", "phase")
	credentialsRef := unstructuredNestedString(instance.Object, "spec", "credentialsRef", "name")
	nodeRef := unstructuredNestedString(instance.Object, "status", "nodeRef", "name")
	labels := unstructuredNestedStringMap(instance.Object, "metadata", "labels")

	return &pb.GetStaticInstanceResponse{
		Name:           name,
		Address:        address,
		Phase:          phase,
		CredentialsRef: credentialsRef,
		NodeRef:        nodeRef,
		Labels:         labels,
	}, nil
}

// GetPodLogs fetches logs for a specific pod and optional container.
func (h *DiagnosticsHandler) GetPodLogs(
	ctx context.Context,
	req *pb.GetPodLogsRequest,
) (*pb.GetPodLogsResponse, error) {
	container := ""
	if req.Container != nil {
		container = req.GetContainer()
	}

	var tailLines *int64

	if req.Tail != nil {
		lines := int64(req.GetTail())
		tailLines = &lines
	}

	var since *string
	if req.Since != nil {
		since = req.Since
	}

	logs, err := h.client.GetPodLogs(ctx, req.GetNamespace(), req.GetPod(), container, tailLines, since)
	if err != nil {
		return nil, fmt.Errorf("getting pod logs for %s/%s: %w", req.GetNamespace(), req.GetPod(), err)
	}

	return &pb.GetPodLogsResponse{Logs: logs}, nil
}

func (h *DiagnosticsHandler) collectNodeSummary(ctx context.Context) (*pb.NodeSummary, error) {
	nodes, err := h.client.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var ready, notReady int32

	for _, n := range nodes {
		if isNodeReady(&n) {
			ready++
		} else {
			notReady++
		}
	}

	return &pb.NodeSummary{
		Total:    clampInt32(int64(len(nodes))),
		Ready:    ready,
		NotReady: notReady,
	}, nil
}

func (h *DiagnosticsHandler) collectNodeGroupStatuses(ctx context.Context) ([]*pb.NodeGroupStatus, error) {
	nodeGroups, err := h.client.ListNodeGroups(ctx)
	if err != nil {
		// node-manager is optional: when it is disabled the nodegroups CRD is
		// absent. Degrade to an empty node-group list rather than failing the
		// whole cluster status.
		if k8s.IsCRDNotRegistered(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing node groups: %w", err)
	}

	var statuses []*pb.NodeGroupStatus

	for _, nodeGroup := range nodeGroups {
		name := unstructuredNestedString(nodeGroup.Object, "metadata", "name")
		ngReady := unstructuredNestedInt64(nodeGroup.Object, "status", "ready")
		ngTotal := unstructuredNestedInt64(nodeGroup.Object, "status", "nodes")

		statuses = append(statuses, &pb.NodeGroupStatus{
			Name:  name,
			Ready: clampInt32(ngReady),
			Total: clampInt32(ngTotal),
		})
	}

	return statuses, nil
}

func (h *DiagnosticsHandler) collectErroredModules(ctx context.Context) ([]string, error) {
	moduleConfigs, err := h.client.ListModuleConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing module configs: %w", err)
	}

	var errored []string

	for _, moduleConfig := range moduleConfigs {
		msg := unstructuredNestedString(moduleConfig.Object, "status", "message")
		if msg != "" {
			name := unstructuredNestedString(moduleConfig.Object, "metadata", "name")
			errored = append(errored, name)
		}
	}

	return errored, nil
}

func (h *DiagnosticsHandler) collectReleaseInfo(ctx context.Context) ([]*pb.PendingRelease, string, error) {
	releases, err := h.client.ListDeckhouseReleases(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("listing releases: %w", err)
	}

	var (
		pendingReleases  []*pb.PendingRelease
		deckhouseVersion string
	)

	for _, release := range releases {
		phase := unstructuredNestedString(release.Object, "status", "phase")
		name := unstructuredNestedString(release.Object, "metadata", "name")
		version := unstructuredNestedString(release.Object, "spec", "version")

		if phase == phasePending {
			pendingReleases = append(pendingReleases, &pb.PendingRelease{
				Name:    name,
				Version: version,
			})
		}

		if phase == "Deployed" {
			deckhouseVersion = version
		}
	}

	return pendingReleases, deckhouseVersion, nil
}

func (h *DiagnosticsHandler) countUnhealthyPods(ctx context.Context) (int32, error) {
	pods, err := h.client.ListPods(ctx, "d8-system")
	if err != nil {
		return 0, fmt.Errorf("listing pods: %w", err)
	}

	var count int32

	for _, pod := range pods {
		if !isPodHealthy(&pod) {
			count++
		}
	}

	return count, nil
}
