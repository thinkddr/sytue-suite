// SPDX-License-Identifier: Apache-2.0
package model

import (
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
)

func TestOfficeResourceActionsPreserveOriginalRoleCapabilities(t *testing.T) {
	for _, tc := range []struct {
		role    Role
		allowed []resource.Action
	}{
		{RoleViewer, []resource.Action{resource.Read}},
		{RoleCommenter, []resource.Action{resource.Comment, resource.Read}},
		{RoleEditor, []resource.Action{resource.Comment, resource.Publish, resource.Read}},
		{RoleOwner, []resource.Action{resource.Comment, resource.Manage, resource.Publish, resource.Read, resource.Validate}},
		{"admin", nil},
		{"", nil},
	} {
		for _, action := range []resource.Action{resource.Comment, resource.Manage, resource.Publish, resource.Read, resource.Validate, "unknown"} {
			want := false
			for _, allowed := range tc.allowed {
				want = want || action == allowed
			}
			if tc.role.AllowsResourceAction(action) != want {
				t.Errorf("original role %q changed capability %q", tc.role, action)
			}
		}
	}
}
