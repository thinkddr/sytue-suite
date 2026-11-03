// SPDX-License-Identifier: Apache-2.0
package storage

// IsolatedUserPrefix identifies private Matrix accounts across the suite.
const IsolatedUserPrefix = "@sytue_storage_"

// IsolatedMatrixUserPrefix is the routing tuple shared by provisioning and
// clients. The caller supplies the homeserver name and its identity validation.
func IsolatedMatrixUserPrefix(scope Scope) string {
	if !scope.Valid() {
		return ""
	}
	return IsolatedUserPrefix + scope.Product + "_" + scope.Tenant + "_" + scope.Subject + ":"
}

// IdentityVersion selects the Matrix account namespace, not the record codec.
// Legacy accounts are shared across products; isolated accounts are scoped to
// one person, tenant and product. Existing clients keep the legacy default.
type IdentityVersion int

const (
	LegacyIdentity   IdentityVersion = 1
	IsolatedIdentity IdentityVersion = 2
)

func (v IdentityVersion) Valid() bool { return v == LegacyIdentity || v == IsolatedIdentity }

// Normalized maps an omitted wire value to the historical identity. Unsupported
// values remain invalid (zero); they must never silently select another account.
func (v IdentityVersion) Normalized() IdentityVersion {
	if v == 0 {
		return LegacyIdentity
	}
	if v.Valid() {
		return v
	}
	return 0
}
