package auth

import "testing"

func TestUserHasPermission(t *testing.T) {
	tests := []struct {
		name string
		user User
		perm string
		want bool
	}{
		{
			name: "admin resolves as wildcard",
			user: User{Roles: []string{"Administrator"}},
			perm: "users:write",
			want: true,
		},
		{
			name: "exact permission matches",
			user: User{Permissions: []string{"torrents:read"}},
			perm: "torrents:read",
			want: true,
		},
		{
			name: "different permission does not match",
			user: User{Permissions: []string{"torrents:read"}},
			perm: "torrents:delete",
			want: false,
		},
		{
			name: "wildcard resource covers action",
			user: User{Permissions: []string{"torrents:*"}},
			perm: "torrents:write",
			want: true,
		},
		{
			name: "wildcard action covers resource",
			user: User{Permissions: []string{"*:read"}},
			perm: "nodes:read",
			want: true,
		},
		{
			name: "full wildcard covers everything",
			user: User{Permissions: []string{"*:*"}},
			perm: "settings:write",
			want: true,
		},
		{
			name: "different resource with same action does not match",
			user: User{Permissions: []string{"torrents:read"}},
			perm: "nodes:read",
			want: false,
		},
		{
			name: "no permissions denies",
			user: User{},
			perm: "torrents:read",
			want: false,
		},
		{
			name: "viewer does not have write permission",
			user: User{Roles: []string{"Viewer"}, Permissions: []string{"torrents:read"}},
			perm: "torrents:write",
			want: false,
		},
		{
			name: "route requesting wildcard does not grant partial permission",
			user: User{Permissions: []string{"torrents:read"}},
			perm: "*:*",
			want: false,
		},
		{
			name: "route requesting resource wildcard does not grant concrete action",
			user: User{Permissions: []string{"torrents:read"}},
			perm: "torrents:*",
			want: false,
		},
		{
			name: "route requesting action wildcard does not grant concrete resource",
			user: User{Permissions: []string{"torrents:read"}},
			perm: "*:read",
			want: false,
		},
		{
			name: "user wildcard covers concrete route",
			user: User{Permissions: []string{"*:*"}},
			perm: "users:write",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.HasPermission(tt.perm); got != tt.want {
				t.Errorf("HasPermission(%q) = %v, want %v", tt.perm, got, tt.want)
			}
		})
	}
}
