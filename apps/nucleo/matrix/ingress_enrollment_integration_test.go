// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func TestIngressEnrollmentRecoversExactPublicPlanWithoutCreatingCandidateOnFreshDevice(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_TRANSFER_DOCUMENT_PROOF") != "1" {
		t.Skip("requires disposable Synapse")
	}
	endpoint, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_MATRIX_TRANSFER_DOCUMENT_URL"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	forward := httputil.NewSingleHostReverseProxy(endpoint)
	var writes atomic.Int32
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/account_data/org.sytue.ingress.key.v1.") {
			writes.Add(1)
		}
		forward.ServeHTTP(w, r)
	}))
	defer func() { cancel(); proxy.Close() }()
	scope := storage.Scope{Tenant: "enrollment-proof", Subject: uuid.NewString(), Product: "mail"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	plan, err := device.Client.CreateIngressKey(ctx, scope, uuid.NewString(), 1)
	if err != nil {
		t.Fatal(err)
	}
	seedPrivateMailboxRecord(ctx, t, device.Client, "mailbox", "inbox", []byte(`{"id":"inbox","name":"Private inbox"}`))
	if err := device.Client.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := freshDocumentDevice(ctx, t, proxy, device, scope)
	defer fresh.Close()
	before := writes.Load()
	if before < 1 {
		t.Fatal("fixture never observed the original encrypted SSSS recipient write")
	}
	for range 2 {
		recovered, err := fresh.RecoverIngressEnrollment(ctx, plan)
		if err != nil || recovered != plan {
			t.Fatal("fresh recovered device changed the original enrollment", recovered, err)
		}
	}
	for _, fault := range []string{"scope", "generation", "recipient", "fingerprint"} {
		t.Run(fault, func(t *testing.T) {
			changed := plan
			switch fault {
			case "scope":
				changed.Scope.Subject = "other"
			case "generation":
				changed.Generation++
			case "recipient":
				identity, err := age.GenerateX25519Identity()
				if err != nil {
					t.Fatal(err)
				}
				changed.Recipient = identity.Recipient().String()
				changed.KeyID = ingress.PublicFingerprint(changed.Recipient)
			case "fingerprint":
				changed.KeyID = strings.Repeat("a", 64)
			}
			if _, err := fresh.RecoverIngressEnrollment(ctx, changed); err == nil {
				t.Fatal("changed public plan recovered another identity")
			}
		})
	}
	if writes.Load() != before {
		t.Fatal("missing/mismatched recovery silently created another recipient", before, writes.Load())
	}
	missing := plan
	missing.Generation++
	if _, err := fresh.RecoverIngressEnrollment(ctx, missing); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("missing recovery lost its explicit failure", err)
	}
	t.Log("INGRESS_ENROLLMENT_FRESH_SSSS_EXACT_PLAN_NO_CREATE_PASS")
}
