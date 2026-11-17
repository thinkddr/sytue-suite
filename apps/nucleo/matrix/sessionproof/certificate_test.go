// SPDX-License-Identifier: Apache-2.0
package sessionproof_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func certificateFixture(t *testing.T) (*matrixprooftest.Fixture, sessionproof.CertificateExpectation, []byte) {
	t.Helper()
	scope := storage.Scope{Tenant: "fixture", Subject: "sender", Product: "rooms"}
	f := matrixprooftest.NewUser(t, scope, "matrix.test", "@sender:matrix.test", "VERIFIED_SENDER")
	expected := sessionproof.CertificateExpectation{
		Scope: scope, Resource: resource.Resource{Tenant: "fixture", Product: "rooms", Kind: "channel", ID: "fixture-channel"}, Epoch: "epoch1", Generation: "9223372036854775807", PolicySHA256: strings.Repeat("1", 64),
		Session: sessionproof.Session{RoomID: "!resource:matrix.test", SessionID: matrixprooftest.NewSigning(t).PublicKey().String(), SenderKey: f.Self.PublicKey().String(), SenderUser: f.Origin.UserID.String(), SigningKey: f.Device.PublicKey().String()}, DeviceID: f.Origin.DeviceID.String(), RootKey: f.Master.PublicKey().String(),
	}
	encoded, err := sessionproof.EncodeCertificate(expected)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := sessionproof.DecodeCertificate(encoded.Payload)
	if err != nil {
		t.Fatal(err)
	}
	certificate.Signatures = matrixprooftest.Sign(t, f.Device, certificate, f.Origin.UserID, f.Origin.DeviceID.String())
	raw, err := matrixproof.CanonicalBytes(certificate)
	if err != nil {
		t.Fatal(err)
	}
	return f, expected, raw
}

func TestPublicSessionCertificateBindsExactSDKChainAndResource(t *testing.T) {
	f, expected, raw := certificateFixture(t)
	verified, err := sessionproof.VerifyCertificateOrigin(t.Context(), raw, expected, f.Origin)
	if err != nil || !verified.Valid() {
		t.Fatal("SDK signed certificate rejected", err)
	}
	recovered, err := verified.Expectation()
	if err != nil || recovered != expected || !strings.HasPrefix(verified.ID(), "sessioncert1_") {
		t.Fatal("verified certificate lost binding", err)
	}
	for _, fault := range []string{"tenant", "subject", "product", "resource", "kind", "epoch", "generation", "policy", "room", "session", "sender", "curve-key", "signing-key", "device", "root", "signature", "unknown", "duplicate", "inactive", "revoked-root", "foreign-device", "trailing"} {
		t.Run(fault, func(t *testing.T) {
			e := expected
			origin := f.Origin
			wire := append([]byte(nil), raw...)
			switch fault {
			case "tenant":
				e.Scope.Tenant = "another"
				e.Resource.Tenant = "another"
			case "subject":
				e.Scope.Subject = "another"
			case "product":
				e.Scope.Product = "mail"
				e.Resource.Product = "mail"
			case "resource":
				e.Resource.ID = "another"
			case "kind":
				e.Resource.Kind = "document"
			case "epoch":
				e.Epoch = "epoch2"
			case "generation":
				e.Generation = "2"
			case "policy":
				e.PolicySHA256 = strings.Repeat("2", 64)
			case "room":
				e.Session.RoomID = "!other:matrix.test"
			case "session":
				e.Session.SessionID = matrixprooftest.NewSigning(t).PublicKey().String()
			case "sender":
				e.Session.SenderUser = "@another:matrix.test"
			case "curve-key":
				e.Session.SenderKey = matrixprooftest.NewSigning(t).PublicKey().String()
			case "signing-key":
				e.Session.SigningKey = matrixprooftest.NewSigning(t).PublicKey().String()
			case "device":
				e.DeviceID = "OTHER"
			case "root":
				e.RootKey = matrixprooftest.NewSigning(t).PublicKey().String()
			case "signature":
				c, err := sessionproof.DecodeCertificate(wire)
				if err != nil {
					t.Fatal(err)
				}
				c.Signatures = nil
				wire, _ = json.Marshal(c)
			case "unknown":
				wire = append([]byte("{\"unrecognized\":true,"), wire[1:]...)
			case "duplicate":
				wire = append([]byte("{\"v\":1,"), wire[1:]...)
			case "inactive":
				origin.DeviceActive = false
			case "revoked-root":
				other := matrixprooftest.NewUser(t, e.Scope, "matrix.test", id.UserID(e.Session.SenderUser), id.DeviceID(e.DeviceID))
				origin.Keys = other.Origin.Keys
			case "foreign-device":
				origin.DeviceID = "OTHER"
			case "trailing":
				wire = append(wire, []byte("{}")...)
			}
			if _, err := sessionproof.VerifyCertificateOrigin(t.Context(), wire, e, origin); err == nil {
				t.Fatal("changed binding accepted", fault)
			}
		})
	}
}

func TestPublicSessionCertificateCannotBeSynthesizedFromPublicBackup(t *testing.T) {
	f, expected, raw := certificateFixture(t)
	certificate, err := sessionproof.DecodeCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	// An attacker can upload a complete Megolm backup, including these claimed
	// fields, but has neither the device signing key nor the pinned account root.
	attacker := matrixprooftest.NewSigning(t)
	certificate.Signatures = matrixprooftest.Sign(t, attacker, certificate, f.Origin.UserID, f.Origin.DeviceID.String())
	forged, _ := json.Marshal(certificate)
	if _, err := sessionproof.VerifyCertificateOrigin(t.Context(), forged, expected, f.Origin); err == nil {
		t.Fatal("public backup impersonated verified sender")
	}
	// Even a genuine sender's signature cannot certify a different device curve
	// key; compare the tuple with independently verified device keys as well.
	certificate.Session.SenderKey = attacker.PublicKey().String()
	expected.Session.SenderKey = certificate.Session.SenderKey
	certificate.Signatures = matrixprooftest.Sign(t, f.Device, certificate, f.Origin.UserID, f.Origin.DeviceID.String())
	forged, _ = json.Marshal(certificate)
	if _, err := sessionproof.VerifyCertificateOrigin(t.Context(), forged, expected, f.Origin); err == nil {
		t.Fatal("sender certificate changed SDK encryption identity")
	}
}

// TestPublicSessionCertificateSDKInterop exchanges public test vectors with the
// real Rust OlmMachine probe. It exports no private signing/session material.
func TestPublicSessionCertificateSDKInterop(t *testing.T) {
	if output := os.Getenv("SYTUE_MATRIX_SESSION_CERTIFICATE_GO_VECTOR"); output != "" {
		f, expected, raw := certificateFixture(t)
		origin := map[string]any{"scope": f.Origin.Scope, "server_name": f.Origin.ServerName, "user_id": f.Origin.UserID, "device_id": f.Origin.DeviceID, "account_active": f.Origin.AccountActive, "device_active": f.Origin.DeviceActive, "keys": f.Origin.Keys}
		vector, err := json.Marshal(map[string]any{"expectation": expected, "certificate": json.RawMessage(raw), "origin": origin})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, vector, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	input := os.Getenv("SYTUE_MATRIX_SESSION_CERTIFICATE_RUST_VECTOR")
	if input == "" {
		t.Skip("opt-in public Rust SDK vector")
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Expectation json.RawMessage `json:"expectation"`
		Certificate json.RawMessage `json:"certificate"`
		Origin      json.RawMessage `json:"origin"`
	}
	if json.Unmarshal(raw, &vector) != nil {
		t.Fatal("invalid public Rust vector")
	}
	expected, err := sessionproof.DecodeCertificateExpectation(vector.Expectation)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := sessionproof.DecodeCertificateOrigin(vector.Origin)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := sessionproof.VerifyCertificateOrigin(t.Context(), vector.Certificate, expected, origin)
	if err != nil || !verified.Valid() {
		t.Fatal("native rejected actual Rust SDK signature", err)
	}
}
