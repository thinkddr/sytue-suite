// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type consentIdentity struct{ user, tenant, device string }

func (c consentIdentity) Identity() (string, string) { return c.user, c.tenant }
func (c consentIdentity) DeviceID() string           { return c.device }
func TestStorageConsentChecksDeviceSeparatelyFromTenant(t *testing.T) {
	request := matrixproof.RootEnrollment{Scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}, UserID: "@alice:storage.test", Target: storage.PublicationTarget{RoomID: "!mail:storage.test", DeviceID: "DEVICE"}}
	identity := consentIdentity{user: request.UserID, tenant: "acme", device: "DEVICE"}
	if !storageConsentIdentity(identity, request) {
		t.Fatal("verified device was compared with its tenant")
	}
	for _, change := range []consentIdentity{{user: "@other:storage.test", tenant: "acme", device: "DEVICE"}, {user: request.UserID, tenant: "other", device: "DEVICE"}, {user: request.UserID, tenant: "acme", device: "other"}} {
		if storageConsentIdentity(change, request) {
			t.Fatal("foreign identity permitted consent")
		}
	}
	if err := nativeStorageApproval(nil, &Panel{})(context.Background(), request); err == nil {
		t.Fatal("missing bound resources approved consent")
	}
}
