// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

type enrollmentHTTPProbe struct {
	PrivateIngressPort
	result ingress.EnrollmentState
	err    error
	calls  int
}

func (p *enrollmentHTTPProbe) Enrollment(context.Context, storage.Scope, string) (ingress.EnrollmentState, error) {
	p.calls++
	return p.result, p.err
}

func TestPrivateIngressEnrollmentCannotConvertFailureToAbsence(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "11111111-1111-4111-8111-111111111111"
	for _, fault := range []string{"absent", "foreign", "malformed", "unavailable"} {
		t.Run(fault, func(t *testing.T) {
			port := &enrollmentHTTPProbe{result: ingress.EnrollmentState{Version: 1, Scope: scope, Epoch: epoch, Status: "absent"}}
			switch fault {
			case "foreign":
				port.result.Scope.Subject = "bob"
			case "malformed":
				port.result.Generation = 1
			case "unavailable":
				port.err = ingress.ErrRevoked
			}
			h := NewPrivateIngressHandlers(PrivateIngressHTTPConfig{Product: "mail", Port: port, MaxBodyBytes: 1024, Identity: func(context.Context) (PrivateSourceIdentity, bool) {
				return PrivateSourceIdentity{Tenant: scope.Tenant, Subject: scope.Subject}, true
			}, MapError: func(error) *Error { return ErrUnavailable }})
			w := httptest.NewRecorder()
			h.Enrollment(w, ingressHTTPRequest(t, scope, ingress.EnrollmentRequest{Epoch: epoch}))
			if fault == "absent" {
				if w.Code != http.StatusOK {
					t.Fatal(w.Code)
				}
			} else if w.Code < 400 {
				t.Fatal("failure represented as absence", w.Code)
			}
			if port.calls != 1 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("enrollment cached or route skipped", port.calls, w.Header())
			}
		})
	}
}
