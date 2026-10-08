package api

import (
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/node"
)

// The trail only stores the text: the old alias no longer exists anywhere after the
// UPDATE, so the description needs to include the before and after.
func TestRenameAuditText(t *testing.T) {
	cases := []struct {
		name string
		res  node.RenameResult
		want string
	}{
		{
			name: "change alias",
			res:  node.RenameResult{Hostname: "seedgrid-node-02", Before: "Room", After: "Bedroom"},
			want: "Renamed Node «Room» to «Bedroom»",
		},
		{
			name: "first alias",
			res:  node.RenameResult{Hostname: "seedgrid-node-02", After: "Room"},
			want: "Gave Node «seedgrid-node-02» the nickname «Room»",
		},
		{
			name: "remove alias",
			res:  node.RenameResult{Hostname: "seedgrid-node-02", Before: "Room"},
			want: "Removed the nickname «Room» from Node «seedgrid-node-02»",
		},
		{
			name: "remove when none existed",
			res:  node.RenameResult{Hostname: "seedgrid-node-02"},
			want: "Removed the nickname of Node «seedgrid-node-02» (it had none)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := renameAuditText(c.res); got != c.want {
				t.Errorf("renameAuditText(%+v) = %q, want %q", c.res, got, c.want)
			}
		})
	}
}
