package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/easyp-tech/deckhouse-harness/internal/k8s"
	pb "github.com/easyp-tech/deckhouse-harness/proto/deckhouse/v1"
)

// errNotImplemented is returned by GREEN-stub handler methods awaiting full implementation.
var errNotImplemented = errors.New("not implemented")

// errEmptyModuleSettings is returned when UpdateModuleSettings receives an empty patch.
var errEmptyModuleSettings = errors.New("settings must be a non-empty object")

// moduleMaintenanceModeActive is the Deckhouse-defined string value for
// ModuleConfig.spec.maintenance that pauses enable/disable transitions while
// allowing settings/version updates to continue. Confirmed via Deckhouse public
// docs (kubernetes-platform/documentation/v1/cr.html and module-development).
const moduleMaintenanceModeActive = "NoResourceReconciliation"

// ModulesHandler implements pb.ModulesAPIToolHandler.
type ModulesHandler struct {
	client k8s.Client
}

// NewModulesHandler creates a new ModulesHandler.
func NewModulesHandler(client k8s.Client) *ModulesHandler {
	return &ModulesHandler{client: client}
}

func (h *ModulesHandler) ListModuleConfigs(
	ctx context.Context,
	req *pb.ListModuleConfigsRequest,
) (*pb.ListModuleConfigsResponse, error) {
	configs, err := h.client.ListModuleConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing module configs: %w", err)
	}

	var result []*pb.ModuleConfigInfo

	for _, moduleConfig := range configs {
		name := unstructuredNestedString(moduleConfig.Object, "metadata", "name")
		enabled, _ := nestedField(moduleConfig.Object, "spec", "enabled")
		// spec.version is an integer in the CR; read it as int64 and render as a
		// string. Fall back to a string value if some source stores it as text.
		version := unstructuredNestedString(moduleConfig.Object, "spec", "version")
		if version == "" {
			if v := unstructuredNestedInt64(moduleConfig.Object, "spec", "version"); v != 0 {
				version = strconv.FormatInt(v, 10)
			}
		}
		source := unstructuredNestedString(moduleConfig.Object, "status", "source")
		updatePolicy := unstructuredNestedString(moduleConfig.Object, "status", "updatePolicy")
		statusMsg := unstructuredNestedString(moduleConfig.Object, "status", "message")

		isEnabled := false
		if b, ok := enabled.(bool); ok {
			isEnabled = b
		}

		if req.Enabled != nil && isEnabled != req.GetEnabled() {
			continue
		}

		result = append(result, &pb.ModuleConfigInfo{
			Name:          name,
			Enabled:       isEnabled,
			Version:       version,
			Source:        source,
			UpdatePolicy:  updatePolicy,
			StatusMessage: statusMsg,
		})
	}

	return &pb.ListModuleConfigsResponse{Modules: result}, nil
}

// GetModuleConfig returns detailed configuration for a specific module.
func (h *ModulesHandler) GetModuleConfig(
	ctx context.Context,
	req *pb.GetModuleConfigRequest,
) (*pb.GetModuleConfigResponse, error) {
	moduleConfig, err := h.client.GetModuleConfig(ctx, req.GetName())
	if err != nil {
		return nil, fmt.Errorf("getting module config %s: %w", req.GetName(), err)
	}

	name := unstructuredNestedString(moduleConfig.Object, "metadata", "name")
	enabledVal, _ := nestedField(moduleConfig.Object, "spec", "enabled")
	statusMsg := unstructuredNestedString(moduleConfig.Object, "status", "message")

	isEnabled := false
	if b, ok := enabledVal.(bool); ok {
		isEnabled = b
	}

	// Read version as int.
	var version *int32

	v := unstructuredNestedInt64(moduleConfig.Object, "spec", "version")
	if v != 0 {
		v32 := int32(v) //nolint:gosec // version numbers fit in int32
		version = &v32
	}

	// Read settings as map[string]string (best-effort — values may be of any type).
	settings := make(map[string]string)

	settingsVal, _ := nestedField(moduleConfig.Object, "spec", "settings")
	if m, ok := settingsVal.(map[string]any); ok {
		for k, val := range m {
			settings[k] = fmt.Sprintf("%v", val)
		}
	}

	return &pb.GetModuleConfigResponse{
		Name:          name,
		Enabled:       isEnabled,
		Version:       version,
		Settings:      settings,
		StatusMessage: statusMsg,
	}, nil
}

// EnableModule sets spec.enabled=true on a ModuleConfig resource.
func (h *ModulesHandler) EnableModule(
	ctx context.Context,
	req *pb.EnableModuleRequest,
) (*pb.EnableModuleResponse, error) {
	return h.setModuleEnabled(ctx, req.GetName(), true)
}

// DisableModule sets spec.enabled=false on a ModuleConfig resource.
func (h *ModulesHandler) DisableModule(
	ctx context.Context,
	req *pb.DisableModuleRequest,
) (*pb.DisableModuleResponse, error) {
	resp, err := h.setModuleEnabled(ctx, req.GetName(), false)
	if err != nil {
		return nil, err
	}

	return &pb.DisableModuleResponse{
		Success:       resp.GetSuccess(),
		PreviousState: resp.GetPreviousState(),
	}, nil
}

// ListModules returns all Deckhouse Module runtime resources.
func (h *ModulesHandler) ListModules(
	ctx context.Context,
	_ *pb.ListModulesRequest,
) (*pb.ListModulesResponse, error) {
	modules, err := h.client.ListModules(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing modules: %w", err)
	}

	result := make([]*pb.ModuleInfo, 0, len(modules))

	for _, module := range modules {
		name := unstructuredNestedString(module.Object, "metadata", "name")
		weight := unstructuredNestedInt64(module.Object, "spec", "weight")
		source := unstructuredNestedString(module.Object, "spec", "source")
		state := unstructuredNestedString(module.Object, "status", "phase")

		result = append(result, &pb.ModuleInfo{
			Name:   name,
			Weight: clampInt32(weight),
			Source: source,
			State:  state,
		})
	}

	return &pb.ListModulesResponse{Modules: result}, nil
}

// setModuleEnabled is the shared implementation for Enable/Disable.
func (h *ModulesHandler) setModuleEnabled(
	ctx context.Context,
	name string,
	enable bool,
) (*pb.EnableModuleResponse, error) {
	moduleConfig, err := h.client.GetModuleConfig(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("getting module config %s: %w", name, err)
	}

	// Read previous state.
	prevVal, _ := nestedField(moduleConfig.Object, "spec", "enabled")

	prevEnabled := false
	if b, ok := prevVal.(bool); ok {
		prevEnabled = b
	}

	// Ensure spec map exists.
	spec, ok := moduleConfig.Object["spec"].(map[string]any)
	if !ok {
		spec = make(map[string]any)
		moduleConfig.Object["spec"] = spec
	}

	spec["enabled"] = enable

	updated, err := h.client.UpdateModuleConfig(ctx, moduleConfig)
	if err != nil {
		return nil, fmt.Errorf("updating module config %s: %w", name, err)
	}

	_ = updated

	return &pb.EnableModuleResponse{
		Success:       true,
		PreviousState: prevEnabled,
	}, nil
}

// UpdateModuleSettings performs a deep merge (RFC 7396 JSON Merge Patch semantics)
// of the supplied settings into the ModuleConfig's spec.settings.
//
// Rules:
//   - patch[k] == nil      → delete k from target (RFC 7396 explicit deletion)
//   - patch[k] is map      → recurse into target[k]
//   - patch[k] is anything → replace target[k] entirely (arrays, scalars)
//
// An empty patch is rejected before any cluster call to avoid surprise no-ops.
func (h *ModulesHandler) UpdateModuleSettings(
	ctx context.Context,
	req *pb.UpdateModuleSettingsRequest,
) (*pb.UpdateModuleSettingsResponse, error) {
	patch := req.GetSettings().AsMap()
	if len(patch) == 0 {
		return nil, errEmptyModuleSettings
	}

	moduleConfig, err := h.client.GetModuleConfig(ctx, req.GetName())
	if err != nil {
		return nil, fmt.Errorf("getting module config %s: %w", req.GetName(), err)
	}

	spec, ok := moduleConfig.Object["spec"].(map[string]any)
	if !ok {
		spec = make(map[string]any)
		moduleConfig.Object["spec"] = spec
	}

	current, _ := spec["settings"].(map[string]any)
	if current == nil {
		current = make(map[string]any)
	}

	merged := mergeJSONPatch(current, patch)
	spec["settings"] = merged

	_, err = h.client.UpdateModuleConfig(ctx, moduleConfig)
	if err != nil {
		return nil, fmt.Errorf("updating module config %s: %w", req.GetName(), err)
	}

	return &pb.UpdateModuleSettingsResponse{Updated: true}, nil
}

// SetModuleMaintenance toggles ModuleConfig.spec.maintenance using a JSON merge
// patch. When enabled=true, sets spec.maintenance="NoResourceReconciliation"
// (Deckhouse stops applying enable/disable transitions). When enabled=false,
// emits {"spec":{"maintenance":null}} — RFC 7396 removes the field.
//
// REQ-4.2: idempotent. REQ-4.4: not-found propagates as wrapped error.
func (h *ModulesHandler) SetModuleMaintenance(
	ctx context.Context,
	req *pb.SetModuleMaintenanceRequest,
) (*pb.SetModuleMaintenanceResponse, error) {
	name := req.GetName()

	var patch map[string]any
	if req.GetEnabled() {
		patch = map[string]any{
			"spec": map[string]any{"maintenance": moduleMaintenanceModeActive},
		}
	} else {
		patch = map[string]any{
			"spec": map[string]any{"maintenance": nil},
		}
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("marshalling maintenance patch for %s: %w", name, err)
	}

	_, err = h.client.PatchModuleConfig(ctx, name, patchBytes)
	if err != nil {
		return nil, fmt.Errorf("patching module config %s: %w", name, err)
	}

	return &pb.SetModuleMaintenanceResponse{
		MaintenanceEnabled: req.GetEnabled(),
		Name:               name,
	}, nil
}

// mergeJSONPatch applies RFC 7396 JSON Merge Patch semantics: explicit nil in
// patch deletes the key in target; nested maps are merged recursively; everything
// else replaces the value.
func mergeJSONPatch(target, patch map[string]any) map[string]any {
	if target == nil {
		target = make(map[string]any)
	}

	for k, v := range patch {
		if v == nil {
			delete(target, k)

			continue
		}

		patchMap, patchIsMap := v.(map[string]any)
		if !patchIsMap {
			target[k] = v

			continue
		}

		existing, existingIsMap := target[k].(map[string]any)
		if !existingIsMap {
			existing = make(map[string]any)
		}

		target[k] = mergeJSONPatch(existing, patchMap)
	}

	return target
}
