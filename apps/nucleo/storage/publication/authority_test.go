// SPDX-License-Identifier: Apache-2.0
package publication_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func TestActiveAuthorityRawBytesKeepExistingGenerationAndScopeLimits(t *testing.T) {
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}
	original := matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "matrix.test", RootKey: base64.RawStdEncoding.EncodeToString(make([]byte, 32)), Generation: 9007199254740991, Status: "active"}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := publication.DecodeActiveAuthority(raw, scope)
	if err != nil || parsed != original {
		t.Fatal("valid raw authority changed", err)
	}
	for _, fault := range []string{"unsafe-generation", "revoked", "foreign-scope", "foreign-user", "invalid-root"} {
		changed := original
		switch fault {
		case "unsafe-generation":
			changed.Generation = 9007199254740992
		case "revoked":
			changed.Status = "revoked"
		case "foreign-scope":
			changed.Scope.Product = "docs"
		case "foreign-user":
			changed.UserID = "@other:matrix.test"
		case "invalid-root":
			changed.RootKey = "invalid"
		}
		encoded, _ := json.Marshal(changed)
		if _, err := publication.DecodeActiveAuthority(encoded, scope); err == nil {
			t.Fatal("unusable authority accepted", fault)
		}
	}
}
