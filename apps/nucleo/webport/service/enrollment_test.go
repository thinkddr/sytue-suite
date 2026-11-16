// SPDX-License-Identifier: Apache-2.0
package service

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func TestOriginalEnrollmentAPITraversesOnlyFixedServiceOperationAndNeverInfersAbsence(t *testing.T) {
	identity := fixtureIdentity()
	epoch := "22222222-2222-4222-8222-222222222222"
	state := ingress.EnrollmentState{Version: 1, Scope: identity.Scope, Epoch: epoch, Status: "absent"}
	body, _ := json.Marshal(state)
	status := 200
	offset := int64(0)
	requests, closed := 0, 0
	stream := strings.Repeat("b", 64)
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method webport.Method, raw []byte) ([]byte, error) {
		switch method {
		case webport.ServiceRequest:
			if len(raw) < 4 {
				return nil, webport.ErrRejected
			}
			size := int(binary.BigEndian.Uint32(raw))
			var input serviceRequest
			if size > len(raw)-4 || json.Unmarshal(raw[4:4+size], &input) != nil || input.Operation != "mail.ingress.enrollment" || input.ContentType != "application/json" || len(input.Arguments) != 0 {
				return nil, webport.ErrRejected
			}
			var request ingress.EnrollmentRequest
			if json.Unmarshal(raw[4+size:], &request) != nil || request.Epoch != epoch {
				return nil, webport.ErrRejected
			}
			requests++
			offset = 0
			return json.Marshal(serviceResponse{Status: status, Stream: stream, ContentType: "application/json", ContentLength: int64(len(body))})
		case webport.ServiceRead:
			var request struct {
				Stream string
				Offset int64
				Length int
			}
			if json.Unmarshal(raw, &request) != nil || request.Stream != stream || request.Offset != offset {
				return nil, webport.ErrRejected
			}
			end := min(int64(len(body)), offset+int64(request.Length))
			output := bytes.Clone(body[offset:end])
			offset = end
			return output, nil
		case webport.ServiceClose:
			closed++
			return nil, nil
		default:
			return nil, webport.ErrRejected
		}
	}}
	owner, _ := webport.OpenStorage(t.Context(), port, webport.NewSources())
	transport, _ := NewServiceTransport(owner, "mail")
	api := transport.Mail().PrivateIngress()
	result, err := api.Enrollment(t.Context(), identity.Scope, epoch)
	if err != nil || result.Status != "absent" || result.Scope != identity.Scope || result.Epoch != epoch || requests != 1 || closed != 1 {
		t.Fatal("original enrollment API could not read an explicit authenticated state", err, requests, closed)
	}
	for _, fault := range []string{"http404", "malformed", "foreign-scope", "different-epoch", "active-without-plan"} {
		t.Run(fault, func(t *testing.T) {
			status = 200
			invalid := state
			switch fault {
			case "http404":
				status = 404
			case "malformed":
				invalid.Version = 0
			case "foreign-scope":
				invalid.Scope.Subject = "another"
			case "different-epoch":
				invalid.Epoch = "33333333-3333-4333-8333-333333333333"
			case "active-without-plan":
				invalid.Status = "active"
				invalid.Generation = 1
			}
			body, _ = json.Marshal(invalid)
			before := requests
			if _, err := api.Enrollment(t.Context(), identity.Scope, epoch); err == nil || requests != before+1 {
				t.Fatal("missing or invalid enrollment became an empty private account", fault, err, requests-before)
			}
		})
	}
}
