// SPDX-License-Identifier: Apache-2.0
package storage

import "testing"

func TestIdentityVersionDoesNotDowngradeUnsupportedAccounts(t *testing.T) {
	for _, v := range []IdentityVersion{-1, 3, 256} {
		if v.Valid() || v.Normalized() != 0 {
			t.Fatal("unknown account namespace fell back to legacy", v)
		}
	}
	if IdentityVersion(0).Normalized() != LegacyIdentity || LegacyIdentity.Normalized() != LegacyIdentity || IsolatedIdentity.Normalized() != IsolatedIdentity {
		t.Fatal("supported identity mapping changed")
	}
}
