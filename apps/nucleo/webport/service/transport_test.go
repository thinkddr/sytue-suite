// SPDX-License-Identifier: Apache-2.0
package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/webport"
)

func TestServiceTransportReusesNativeSourceValidationAndOnlyFixedOperations(t *testing.T) {
	identity := fixtureIdentity()
	sources := webport.NewSources()
	state := storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: identity.Scope, Mode: "legacy"}, PrivateIngressReady: true, MaxBodyBytes: 1 << 20}
	body, _ := json.Marshal(state)
	stream := strings.Repeat("b", 64)
	offset := int64(0)
	closed := false
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method webport.Method, raw []byte) ([]byte, error) {
		switch method {
		case webport.ServiceRequest:
			if len(raw) < 4 {
				return nil, webport.ErrRejected
			}
			size := int(binary.BigEndian.Uint32(raw))
			var input serviceRequest
			if size > len(raw)-4 || json.Unmarshal(raw[4:4+size], &input) != nil || input.Operation != "mail.source.state" || len(raw) != 4+size {
				return nil, webport.ErrRejected
			}
			return json.Marshal(serviceResponse{Status: 200, Stream: stream, ContentType: "application/json", ContentLength: int64(len(body))})
		case webport.ServiceRead:
			var request struct {
				Stream string
				Offset int64
				Length int
			}
			if json.Unmarshal(raw, &request) != nil || request.Stream != stream || request.Offset != offset || request.Length > webport.ChunkBytes {
				return nil, webport.ErrRejected
			}
			end := min(int64(len(body)), offset+int64(request.Length))
			out := bytes.Clone(body[offset:end])
			offset = end
			return out, nil
		case webport.ServiceClose:
			closed = true
			return nil, nil
		default:
			return nil, webport.ErrRejected
		}
	}}
	owner, _ := webport.OpenStorage(t.Context(), port, sources)
	transport, _ := NewServiceTransport(owner, "mail")
	got, err := transport.Mail().PrivateStorageCapabilities(t.Context(), identity.Scope)
	if err != nil || got.Scope != identity.Scope || !got.PrivateIngressReady || !closed {
		t.Fatal("native Go capability parser bypassed", got, err, closed)
	}
	scopeHeader, _ := storage.EncodeScopeHeader(identity.Scope)
	for _, path := range []string{
		"https://mail.invalid/api/v1/accounts",
		"https://mail.invalid/api/v1/send",
		"https://outside.test/api/v1/private-storage/migration",
		"https://mail.invalid/api/v1/private-storage/migration?evil=1",
		"https://mail.invalid/api/v1/private-storage/migration/22222222-2222-4222-8222-222222222222/rows?evil=1",
		"https://mail.invalid/api/v1/private-storage/migration/../../accounts",
	} {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
		req.Header.Set(storage.ScopeHeader, scopeHeader)
		if _, err := transport.RoundTrip(req); !errors.Is(err, webport.ErrRejected) {
			t.Fatalf("unrestricted route admitted: %s: %v", path, err)
		}
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://mail.invalid/api/v1/private-storage/migration", http.NoBody)
	req.Header.Set(storage.ScopeHeader, scopeHeader)
	req.Header.Set("Authorization", "synthetic-forbidden")
	if _, err := transport.RoundTrip(req); !errors.Is(err, webport.ErrRejected) {
		t.Fatal("worker credential/header admitted", err)
	}
}

func TestServiceTransportStreamsConsentMIMEOnlyOnceAndRetiresSource(t *testing.T) {
	identity := fixtureIdentity()
	sources := webport.NewSources()
	original := bytes.Repeat([]byte("From: owner@example.test\r\n\r\noriginal private MIME\r\n"), 12000)
	input := model.Submission{Operation: "11111111-1111-4111-8111-111111111111", Epoch: "22222222-2222-4222-8222-222222222222", From: "owner@example.test", Recipients: []string{"recipient@external.test"}, Consent: model.SMTPConsent, Generation: 1, Control: strings.Repeat("c", 64)}
	var port storagePort
	calls := 0
	retained := ""
	port = storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method webport.Method, raw []byte) ([]byte, error) {
		switch method {
		case webport.ServiceRequest:
			calls++
			size := int(binary.BigEndian.Uint32(raw))
			var request serviceRequest
			if size > len(raw)-4 || json.Unmarshal(raw[4:4+size], &request) != nil || request.Operation != "mail.submission.send" || request.Size != int64(len(original)) || len(raw) != 4+size || !webport.ValidID(request.Source) || request.Submission == "" {
				return nil, webport.ErrRejected
			}
			retained = request.Source
			for offset := int64(0); offset < request.Size; {
				length := int(min(int64(webport.ChunkBytes), request.Size-offset))
				actual, err := sources.Read(t.Context(), port, identity, request.Source, offset, length)
				if err != nil || !bytes.Equal(actual, original[offset:offset+int64(length)]) {
					return nil, webport.ErrRejected
				}
				clear(actual)
				offset += int64(length)
			}
			return json.Marshal(serviceResponse{Status: 401, Stream: strings.Repeat("b", 64), ContentType: "application/json", ContentLength: 0})
		case webport.ServiceRead:
			return nil, nil
		case webport.ServiceClose:
			return nil, nil
		default:
			return nil, webport.ErrRejected
		}
	}}
	owner, _ := webport.OpenStorage(t.Context(), port, sources)
	bridge, _ := NewServiceTransport(owner, "mail")
	submission, err := bridge.Mail().PrivateSubmissions(identity.Scope, input.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = submission.Submit(t.Context(), input, bytes.NewReader(original), int64(len(original))); err == nil || calls != 1 {
		t.Fatal("consented SMTP replayed or unread", calls, err)
	}
	if _, err = sources.Read(t.Context(), port, identity, retained, 0, 1); !errors.Is(err, webport.ErrRejected) {
		t.Fatal("MIME source remained exposed after response", err)
	}
	clear(original)
}

type sourcePort struct {
	id       string
	identity webport.Identity
}

func (p sourcePort) ID() string { return p.id }
func (p sourcePort) Current(ctx context.Context) (webport.Identity, error) {
	return p.identity, ctx.Err()
}

type storagePort struct {
	sourcePort
	handle func(webport.Method, []byte) ([]byte, error)
}

func (p storagePort) Call(ctx context.Context, _ storage.Scope, method webport.Method, raw []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.handle(method, raw)
}

func fixtureIdentity() webport.Identity {
	return webport.Identity{Version: webport.Version, Binding: webport.Binding{MatrixUserID: "@owner:example.test", DeviceID: "DEVICE"}, Scope: storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}}
}
