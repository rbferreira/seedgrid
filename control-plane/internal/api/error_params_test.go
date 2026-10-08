package api

import (
	"sort"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/messages"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
)

// The error.params of numbered errors must be the ones messages.Registry promises: it is from them
// that the Portal builds the message (ADR-041 §4).
func TestRecordErrorParams(t *testing.T) {
	cases := map[string]map[string]any{
		"no_node_with_space": noNodeWithSpaceParams(&scheduler.NoNodeWithSpaceError{NeedBytes: 3, MinFreeBytes: 2, BestFreeBytes: 1}),
		"no_eligible_node":   noEligibleNodeParams(&scheduler.NoEligibleNodeError{BlockedByStorage: []string{"node-01"}}),
	}
	for code, params := range cases {
		var want []string
		for _, m := range messages.Registry {
			if m.Kind == messages.Error && m.Code == code {
				want = append(want, m.Payload...)
			}
		}
		var got []string
		for k := range params {
			got = append(got, k)
		}
		sort.Strings(want)
		sort.Strings(got)
		if len(want) == 0 || len(got) != len(want) {
			t.Errorf("%s: params %v, the record promises %v", code, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: params %v, the record promises %v", code, got, want)
				break
			}
		}
	}
}
