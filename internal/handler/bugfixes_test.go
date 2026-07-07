package handler

import (
	"context"
	"errors"
	"testing"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	pb "github.com/easyp-tech/deckhouse-harness/proto/deckhouse/v1"
)

// GetClusterStatus must degrade gracefully (empty node groups, no error) when the
// nodegroups CRD is absent — e.g. the node-manager module is disabled.
func TestGetClusterStatus_NodeGroupsCRDAbsent(t *testing.T) {
	mc := &mockClient{
		listNodeGroupsFunc: func(_ context.Context) ([]unstructured.Unstructured, error) {
			return nil, errors.New("nodegroups.deckhouse.io: the server could not find the requested resource")
		},
	}

	h := NewDiagnosticsHandler(mc)

	resp, err := h.GetClusterStatus(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("expected graceful degradation, got error: %v", err)
	}
	if len(resp.GetNodeGroups()) != 0 {
		t.Errorf("expected empty node groups, got %d", len(resp.GetNodeGroups()))
	}
}

// ListModuleConfigs must render the integer spec.version as a string ("2"),
// not drop it because the raw value is not a string.
func TestListModuleConfigs_VersionFromInt(t *testing.T) {
	mc := &mockClient{
		listModuleConfigsFunc: func(_ context.Context) ([]unstructured.Unstructured, error) {
			return []unstructured.Unstructured{{
				Object: map[string]any{
					"apiVersion": "deckhouse.io/v1alpha1",
					"kind":       "ModuleConfig",
					"metadata":   map[string]any{"name": "cert-manager"},
					"spec":       map[string]any{"enabled": true, "version": int64(2)},
				},
			}}, nil
		},
	}

	h := NewModulesHandler(mc)

	resp, err := h.ListModuleConfigs(context.Background(), &pb.ListModuleConfigsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetModules()) != 1 {
		t.Fatalf("expected 1 module, got %d", len(resp.GetModules()))
	}
	if got := resp.GetModules()[0].GetVersion(); got != "2" {
		t.Errorf("expected version %q, got %q", "2", got)
	}
}
