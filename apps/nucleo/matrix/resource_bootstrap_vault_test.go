// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"testing"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestBootstrapRecoveryVaultRequiresExactProductOwnerAndMatrixServer(t *testing.T) {
	canonical := id.UserID("@sytue_00000000-0000-4000-8000-000000000001:matrix.fixture:8448")
	for _, product := range storage.Products() {
		scope := storage.Scope{Tenant: "tenant", Subject: "00000000-0000-4000-8000-000000000001", Product: product}
		user := id.UserID(storage.IsolatedMatrixUserPrefix(scope) + "matrix.fixture:8448")
		if !bootstrapRecoveryVaultUser(scope, canonical, user) {
			t.Fatal("same product isolated vault denied", product)
		}
		for _, wrong := range []id.UserID{canonical, id.UserID(storage.IsolatedMatrixUserPrefix(scope) + "other.fixture:8448"), id.UserID(storage.IsolatedMatrixUserPrefix(scope) + "matrix.fixture"), "@sytue_storage_wrong:matrix.fixture:8448", "malformed"} {
			if bootstrapRecoveryVaultUser(scope, canonical, wrong) {
				t.Fatal("foreign recovery vault accepted", product)
			}
		}
		for _, field := range []string{"tenant", "subject", "product"} {
			wrong := scope
			switch field {
			case "tenant":
				wrong.Tenant = "other"
			case "subject":
				wrong.Subject = "other"
			case "product":
				wrong.Product = "another"
			}
			if bootstrapRecoveryVaultUser(wrong, canonical, user) {
				t.Fatal("another vault scope accepted", product, field)
			}
		}
	}
	if bootstrapRecoveryVaultUser(storage.Scope{}, canonical, "@sytue_storage_:matrix.fixture:8448") {
		t.Fatal("empty owner scope accepted")
	}
}
