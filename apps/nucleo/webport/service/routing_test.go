// SPDX-License-Identifier: Apache-2.0
package service

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func TestOriginalRoutingAndCipherUsageAPIsTraverseOnlyFixedScopedOperations(t *testing.T) {
	identity := fixtureIdentity()
	stream := strings.Repeat("b", 64)
	alias := api.MailAlias{ID: "11111111-1111-4111-8111-111111111111", Address: "alias@example.test"}
	var body []byte
	offset := int64(0)
	seen := []string{}
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method webport.Method, raw []byte) ([]byte, error) {
		switch method {
		case webport.ServiceRequest:
			if len(raw) < 4 {
				return nil, webport.ErrRejected
			}
			n := int(binary.BigEndian.Uint32(raw))
			var request serviceRequest
			if n > len(raw)-4 || json.Unmarshal(raw[4:4+n], &request) != nil {
				return nil, webport.ErrRejected
			}
			seen = append(seen, request.Operation)
			status := 200
			var value any
			switch request.Operation {
			case "mail.routing.get":
				value = api.MailRoutingSnapshot{Primary: "owner@example.test", Aliases: []api.MailAlias{alias}}
			case "mail.routing.create":
				var input struct{ Address string }
				if json.Unmarshal(raw[4+n:], &input) != nil || input.Address != alias.Address {
					return nil, webport.ErrRejected
				}
				value = alias
			case "mail.routing.delete":
				if len(request.Arguments) != 1 || request.Arguments[0] != alias.ID {
					return nil, webport.ErrRejected
				}
				status = 204
			case "mail.routing.plan":
				value = api.MailQuota{UsedBytes: 999, QuotaBytes: 1000}
			case "drive.cipher.usage":
				value = storage.CipherUsage{Version: 1, Scope: identity.Scope, Bytes: 80, SytueBytes: 60, ExternalBytes: 20, Blobs: 2}
			default:
				return nil, webport.ErrRejected
			}
			body, _ = json.Marshal(value)
			if status == 204 {
				body = nil
			}
			offset = 0
			return json.Marshal(serviceResponse{Status: status, Stream: stream, ContentType: "application/json", ContentLength: int64(len(body))})
		case webport.ServiceRead:
			var input struct {
				Stream string
				Offset int64
				Length int
			}
			if json.Unmarshal(raw, &input) != nil || input.Stream != stream || input.Offset != offset {
				return nil, webport.ErrRejected
			}
			end := min(int64(len(body)), offset+int64(input.Length))
			out := bytes.Clone(body[offset:end])
			offset = end
			return out, nil
		case webport.ServiceClose:
			return nil, nil
		default:
			return nil, webport.ErrRejected
		}
	}}
	owner, err := webport.OpenStorage(t.Context(), port, webport.NewSources())
	if err != nil {
		t.Fatal(err)
	}
	mail, err := NewServiceTransport(owner, "mail")
	if err != nil {
		t.Fatal(err)
	}
	drive, err := NewServiceTransport(owner, "drive")
	if err != nil {
		t.Fatal(err)
	}
	source := mail.Mail()
	if value, err := source.RoutingMetadata(t.Context(), identity.Scope); err != nil || value.Primary != "owner@example.test" {
		t.Fatal("original routing read did not traverse admitted operation", err)
	}
	if value, err := source.CreateRoutingAlias(t.Context(), identity.Scope, alias.Address); err != nil || value != alias {
		t.Fatal("original alias create did not traverse admitted operation", err)
	}
	if err := source.DeleteRoutingAlias(t.Context(), identity.Scope, alias.ID); err != nil {
		t.Fatal(err)
	}
	if value, err := source.RoutingQuota(t.Context(), identity.Scope); err != nil || value.QuotaBytes != 1000 {
		t.Fatal(err)
	}
	if value, err := drive.Drive().CipherUsage(t.Context(), identity.Scope); err != nil || value.Bytes != 80 || value.SytueBytes != 60 {
		t.Fatal("original ciphertext counter did not traverse admitted operation", err)
	}
	if len(seen) != 5 {
		t.Fatal("metadata request count changed", len(seen))
	}
	wrong := identity.Scope
	wrong.Subject = "other"
	if _, err := source.RoutingMetadata(t.Context(), wrong); err == nil || len(seen) != 5 {
		t.Fatal("changed metadata scope reached host", err)
	}
}
