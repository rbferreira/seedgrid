package commitmsg

import (
	"reflect"
	"testing"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want []string
	}{
		{
			name: "simple valid subject",
			msg:  "feat: add webhook support\n",
			want: nil,
		},
		{
			name: "subject with scope",
			msg:  "fix(deploy): fix volume reattach\n",
			want: nil,
		},
		{
			name: "subject with breaking change",
			msg:  "feat(api)!: change response payload\n",
			want: nil,
		},
		{
			name: "GitHub squash with PR",
			msg:  "chore: x (#8)\n",
			want: nil,
		},
		{
			name: "scope with slash and hash",
			msg:  "feat(deploy/app#2): support multiple nodes\n",
			want: nil,
		},
		{
			name: "exempt Merge",
			msg:  "Merge branch 'main' into feature\n",
			want: nil,
		},
		{
			name: "exempt git Revert",
			msg:  `Revert "feat: something"` + "\n",
			want: nil,
		},
		{
			name: "empty message",
			msg:  "",
			want: []string{"empty commit message"},
		},
		{
			name: "only spaces and line breaks",
			msg:  " \n\t\n",
			want: []string{"empty commit message"},
		},
		{
			name: "subject out of format",
			msg:  "Added a new feature\n",
			want: []string{"subject not in the format <type>[(<scope>)][!]: <description> (types: feat, fix, chore, docs, refactor, test, perf, ci, build, style, revert)"},
		},
		{
			name: "subject with unknown type",
			msg:  "feature: add something\n",
			want: []string{"subject not in the format <type>[(<scope>)][!]: <description> (types: feat, fix, chore, docs, refactor, test, perf, ci, build, style, revert)"},
		},
		{
			name: "subject without space after colon",
			msg:  "feat:no space\n",
			want: []string{"subject not in the format <type>[(<scope>)][!]: <description> (types: feat, fix, chore, docs, refactor, test, perf, ci, build, style, revert)"},
		},
		{
			name: "Co-Authored-By no final",
			msg:  "feat: something\n\nBody.\nCo-authored-by: X <x@y>\n",
			want: []string{"Co-Authored-By trailer is not allowed"},
		},
		{
			name: "Co-Authored-By in uppercase",
			msg:  "feat: something\n\nCO-AUTHORED-BY: X <x@y>\n",
			want: []string{"Co-Authored-By trailer is not allowed"},
		},
		{
			name: "Co-Authored-By mixed case",
			msg:  "feat: something\n\nCo-Authored-By: X <x@y>\n",
			want: []string{"Co-Authored-By trailer is not allowed"},
		},
		{
			name: "Co-Authored-By in the middle of the body",
			msg:  "feat: something\n\nText.\nCo-authored-by: X <x@y>\nMore.\n",
			want: []string{"Co-Authored-By trailer is not allowed"},
		},
		{
			name: "invalid subject and trailer together",
			msg:  "Added something\n\nCo-authored-by: X <x@y>\n",
			want: []string{
				"subject not in the format <type>[(<scope>)][!]: <description> (types: feat, fix, chore, docs, refactor, test, perf, ci, build, style, revert)",
				"Co-Authored-By trailer is not allowed",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Check(tt.msg)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Check(%q) = %#v, want %#v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestStripComments(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "removes git comment lines",
			msg:  "feat: something\n# git comment\n\nBody.\n",
			want: "feat: something\n\nBody.\n",
		},
		{
			name: "Co-Authored-By inside comment disappears",
			msg:  "feat: something\n# Co-authored-by: X <x@y>\n",
			want: "feat: something\n",
		},
		{
			name: "hash in the middle of a line is not a comment",
			msg:  "chore: fixes (#12)\n",
			want: "chore: fixes (#12)\n",
		},
		{
			name: "no comments",
			msg:  "fix: fixes bug\n",
			want: "fix: fixes bug\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripComments(tt.msg); got != tt.want {
				t.Errorf("StripComments(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}
