package k8s

import (
	"errors"
	"fmt"
	"testing"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsCRDNotRegistered(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("connection refused"), false},
		{"discovery message", errors.New("the server could not find the requested resource"), true},
		{"wrapped discovery", fmt.Errorf("listing: %w", errors.New("could not find the requested resource")), true},
		{"api not found", kerrors.NewNotFound(schema.GroupResource{Group: "deckhouse.io", Resource: "nodegroups"}, ""), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsCRDNotRegistered(tc.err); got != tc.want {
				t.Errorf("IsCRDNotRegistered(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
