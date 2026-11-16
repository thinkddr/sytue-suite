// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func TestProductSessionEnrollmentRequiresHybridPossessionAndExactSDKConsent(t *testing.T) {
	f := resourcetest.New(t, false)
	recipient := resource.SignedRecipient{RecipientEnrollment: f.Enrolled.Value()}
	recipient.Signatures = matrixprooftest.Sign(t, f.Chain.Device, recipient, f.Chain.Origin.UserID, f.Actor.DeviceID)
	binding := resource.SessionBinding{Scope: f.Scope, Namespace: "resources:" + f.Scope.Product, UserID: f.Actor.UserID, DeviceID: f.Actor.DeviceID, RootKey: f.Actor.RootKey, Recipient: recipient.Recipient, ProductKeyID: recipient.KeyID, EnrollmentSHA256: f.Enrolled.SHA256(), Generation: "9223372036854775807", RootGeneration: "1", AccountVersion: "0", TenantVersion: "0", ConsentID: uuid.NewString()}
	now := time.Now()
	challenge, err := resource.NewSessionChallenge(t.Context(), binding, true, now.Add(time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := resource.OpenSessionChallenge(t.Context(), challenge, f.RecipientIdentity, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.PrepareSessionEnrollment(challenge, nonce, "", now); err == nil {
		t.Fatal("first root auto-enrolled without explicit SDK-signed decision")
	}
	hash, err := resource.SessionChallengeSHA256(challenge)
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := resource.PrepareSessionEnrollment(challenge, nonce, "confirm_current_root", now)
	if err != nil {
		t.Fatal(err)
	}
	if unsigned.ChallengeSHA256 != hash {
		t.Fatal("Go transcript changed challenge hash")
	}
	proof := resource.SignedSessionEnrollment{SessionEnrollment: unsigned}
	proof.Signatures = matrixprooftest.Sign(t, f.Chain.Device, proof, f.Chain.Origin.UserID, f.Actor.DeviceID)
	verified, err := resource.VerifySessionEnrollment(t.Context(), proof, recipient, challenge, f.Device, now)
	if err != nil || verified.Binding() != binding {
		t.Fatal("original product consent could not enroll", err)
	}
	if strings.Contains(string(verified.Bytes()), f.RecipientIdentity.String()) {
		t.Fatal("public enrollment leaked private product recipient")
	}
	wrongKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resource.OpenSessionChallenge(t.Context(), challenge, wrongKey, now); err == nil {
		t.Fatal("Mail without Docs private recipient opened challenge")
	}
	for name, change := range map[string]func(*resource.SignedSessionEnrollment){
		"product":     func(p *resource.SignedSessionEnrollment) { p.Binding.Scope.Product = "mail" },
		"namespace":   func(p *resource.SignedSessionEnrollment) { p.Binding.Namespace = "storage:docs" },
		"consent":     func(p *resource.SignedSessionEnrollment) { p.Binding.ConsentID = uuid.NewString() },
		"device":      func(p *resource.SignedSessionEnrollment) { p.Binding.DeviceID = "MAIL_DEVICE" },
		"root":        func(p *resource.SignedSessionEnrollment) { p.Binding.RootKey = f.Chain.Self.PublicKey().String() },
		"challenge":   func(p *resource.SignedSessionEnrollment) { p.ChallengeID = uuid.NewString() },
		"cipher-hash": func(p *resource.SignedSessionEnrollment) { p.ChallengeSHA256 = strings.Repeat("1", 64) },
		"nonce": func(p *resource.SignedSessionEnrollment) {
			p.Nonce = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
		},
		"root-decision": func(p *resource.SignedSessionEnrollment) { p.RootDecision = "" },
		"signature":     func(p *resource.SignedSessionEnrollment) { p.Signatures = nil },
	} {
		t.Run(name, func(t *testing.T) {
			copied := proof
			change(&copied)
			if name != "signature" {
				copied.Signatures = matrixprooftest.Sign(t, f.Chain.Device, copied, f.Chain.Origin.UserID, f.Actor.DeviceID)
			}
			if _, err := resource.VerifySessionEnrollment(t.Context(), copied, recipient, challenge, f.Device, now); err == nil {
				t.Fatal("cross-bound session enrollment accepted")
			}
		})
	}
	if _, err := resource.VerifySessionEnrollment(t.Context(), proof, recipient, challenge, f.Device, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expired challenge accepted")
	}
	mutated := challenge
	mutated.Binding.Namespace = "resources:mail"
	if _, err := resource.OpenSessionChallenge(t.Context(), mutated, f.RecipientIdentity, now); err == nil {
		t.Fatal("challenge context not authenticated by age")
	}
	if _, err := resource.NewSessionChallenge(t.Context(), binding, true, now.Add(11*time.Minute), now); err == nil {
		t.Fatal("unbounded challenge lifetime")
	}
}
