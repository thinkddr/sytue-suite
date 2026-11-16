//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func TestPreparedPublicationRealSDKBacksUpBeforeHookAndNeverSendsEarly(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PUBLICATION_PREPARE") != "1" {
		t.Skip("isolated remote Synapse required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	target, err := url.Parse(os.Getenv("SYTUE_MATRIX_PUBLICATION_PREPARE_URL"))
	if err != nil || target.Host == "" {
		t.Fatal("isolated Synapse URL required")
	}
	for _, product := range []string{"", "notes"} {
		name := "rooms"
		if product != "" {
			name = product
		}
		t.Run(name, func(t *testing.T) {
			reverse := httputil.NewSingleHostReverseProxy(target)
			var sends atomic.Int64
			proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/send/m.room.encrypted/") {
					sends.Add(1)
				}
				reverse.ServeHTTP(w, r)
			}))
			defer proxy.Close()
			actor, channel := uuid.NewString(), uuid.NewString()
			c, room, _, _ := publicationSDKDeviceFixture(ctx, t, proxy.URL, proxy.Client(), actor, "prepared-fixture", channel, product)
			defer c.Close()
			c.stateMu.Lock()
			c.cfg.RequirePublicationAdmission = true
			c.stateMu.Unlock()
			body := "original private SDK message before durable admission"
			txn := "prepared-" + uuid.NewString()
			prepared, err := c.PrepareEncryptedPublication(ctx, room, event.EventMessage, event.MessageEventContent{MsgType: event.MsgText, Body: body}, txn)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			if sends.Load() != 0 || bytes.Contains(prepared.Wire(), []byte(body)) {
				t.Fatal("preparation sent or exposed unencrypted body")
			}
			if !prepared.Provenance().Valid() || prepared.Binding().TransactionID != txn || prepared.Binding().DeviceID != c.DeviceID() {
				t.Fatal("preparation lost actual SDK binding")
			}
			if _, err := prepared.Send(ctx, nil); !errors.Is(err, ErrPublicationAdmissionRequired) || sends.Load() != 0 {
				t.Fatal("missing per-operation admission sent event", err)
			}
			if product != "" {
				scope := c.storageScope()
				roots, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
				if err != nil || roots == nil {
					t.Fatal(err)
				}
				authority := matrixproof.Authority{Version: 1, Scope: scope, UserID: c.session.UserID.String(), RootKey: roots.MasterKey.String(), Generation: 1, Status: "active"}
				// The fixture supplies a synthetic scope witness; crypto, SQLite, SSSS and
				// key backup are actual SDK/Synapse. Production uses the persistent broker.
				var leases atomic.Int64
				lease := func(ctx context.Context) (func(), error) {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					if leases.Add(1) != 1 {
						leases.Add(-1)
						return nil, errors.New("nested publication session lease")
					}
					return func() { leases.Add(-1) }, nil
				}
				session, err := c.StoragePublicationSession(ctx, scope, authority, lease)
				if err != nil {
					t.Fatal(err)
				}
				// A real resource Provider callback acquires the same persistent lease.
				// It must run outside the signer's own key-access lease.
				session.validate = func(ctx context.Context) error {
					release, err := lease(ctx)
					if err != nil {
						return err
					}
					defer release()
					return ctx.Err()
				}
				request, err := prepared.SignAdmission(ctx, session)
				if err != nil {
					t.Fatal(err)
				}
				if leases.Load() != 0 {
					t.Fatal("signer leaked its session witness")
				}
				keys, err := c.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{c.session.UserID: {c.session.DeviceID}}})
				if err != nil {
					t.Fatal(err)
				}
				_, server, err := c.session.UserID.Parse()
				if err != nil {
					t.Fatal(err)
				}
				origin := matrixproof.Origin{Scope: scope, ServerName: server, UserID: c.session.UserID, DeviceID: c.session.DeviceID, AccountActive: true, DeviceActive: true, Keys: *keys}
				device, err := matrixproof.Verify(ctx, origin, roots.MasterKey)
				if err != nil {
					t.Fatal(err)
				}
				hash, _, err := publication.WireDigest(prepared.Wire(), false)
				if err != nil {
					t.Fatal(err)
				}
				expected := publication.Request{Version: 1, Domain: "org.sytue.publication.admission.v1", Namespace: "storage:notes", Scope: scope, Target: storage.PublicationTarget{RoomID: room, DeviceID: c.DeviceID()}, TransactionID: txn, CipherSHA256: hash, AuthorityGeneration: 1}
				if _, err := publication.VerifyRequest(ctx, request, expected, device); err != nil {
					t.Fatal("native SDK signed a different ciphertext admission", err)
				}
				expectation := sessionproof.CertificateExpectation{Scope: scope, Resource: model.Resource{Tenant: scope.Tenant, Product: scope.Product, Kind: "fixture", ID: "resource1"}, Epoch: "epoch1", Generation: "1", PolicySHA256: strings.Repeat("5", 64), Session: prepared.Provenance(), DeviceID: c.DeviceID(), RootKey: roots.MasterKey.String()}
				certificate, err := prepared.SignSessionCertificate(ctx, session, expectation)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(certificate)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := sessionproof.VerifyCertificateOrigin(ctx, raw, expectation, origin); err != nil {
					t.Fatal("native SDK certificate did not bind its actual session", err)
				}
				if _, err := prepared.SignAdmission(ctx, nil); err == nil {
					t.Fatal("raw scope replaced verified publication session")
				}
			}
			rejected := errors.New("synthetic admission denied")
			if _, err := prepared.Send(ctx, func(context.Context, *PreparedEncryptedPublication) (publication.Marker, error) {
				return publication.Marker{}, rejected
			}); !errors.Is(err, rejected) || sends.Load() != 0 {
				t.Fatal("failed durable admission sent event", err)
			}
			marker := publication.Marker{Version: 1, ID: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))}
			calls := 0
			reference, err := prepared.Send(ctx, func(ctx context.Context, p *PreparedEncryptedPublication) (publication.Marker, error) {
				calls++
				if sends.Load() != 0 {
					t.Fatal("HTTP happened before durable hook")
				}
				if err := p.Revalidate(ctx); err != nil {
					t.Fatal(err)
				}
				return marker, nil
			})
			if err != nil || calls != 1 || sends.Load() != 1 {
				t.Fatal("exact prepared SDK event did not send once", err)
			}
			wire, err := c.cli.GetEvent(ctx, id.RoomID(room), id.EventID(reference.EventID))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(&wire.Content)
			if err != nil {
				t.Fatal(err)
			}
			_, actual, err := publication.WireDigest(raw, true)
			if err != nil || actual != marker || bytes.Contains(raw, []byte(body)) {
				t.Fatal("SDK wire lost marker or leaked content", err)
			}
			decrypted, err := c.Read(ctx, reference)
			if err != nil {
				t.Fatal(err)
			}
			var content event.MessageEventContent
			if json.Unmarshal(decrypted.Content, &content) != nil || content.Body != body {
				t.Fatal("actual SDK decrypt changed original message")
			}
			originalRef := reference
			reference, err = prepared.Send(ctx, nil)
			if err != nil || reference != originalRef || sends.Load() != 2 {
				t.Fatal("retry did not reuse exact transaction and SDK wire", err)
			}
			different := publication.Marker{Version: 1, ID: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))}
			if _, err := prepared.Send(ctx, func(context.Context, *PreparedEncryptedPublication) (publication.Marker, error) {
				return different, nil
			}); err == nil || sends.Load() != 2 {
				t.Fatal("retry replaced durable admission marker")
			}
			prepared.Close()
			if _, err := prepared.Send(ctx, nil); err == nil {
				t.Fatal("retired prepared operation sent again")
			}
		})
	}
}
