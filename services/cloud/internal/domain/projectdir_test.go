// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// CLOUD-15: root_dir y output_dir no salen del repositorio.
func TestValidateProjectDir(t *testing.T) {
	for _, ok := range []string{"", "apps/web", "./web", "dist", "a/b..c"} {
		if err := ValidateProjectDir(ok); err != nil {
			t.Errorf("%q debería valer: %v", ok, err)
		}
	}
	for _, mal := range []string{"..", "../x", "a/../../b", "/etc", "a\\..\\b", "web\n", "x\x00"} {
		if err := ValidateProjectDir(mal); err == nil {
			t.Errorf("%q no debería valer", mal)
		}
	}
}
