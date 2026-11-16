//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
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
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/crypto/olm"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

type resourceTransportFence struct{}

func (resourceTransportFence) Revalidate(ctx context.Context, _ resource.VerifiedAccess) error {
	return ctx.Err()
}

func TestResourceRelayRealSDKUsesStableDeviceAndReconcilesAfterRevocation(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_RESOURCE_RELAY") != "1" {
		t.Skip("disposable remote AS Synapse required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	target, err := url.Parse(os.Getenv("SYTUE_MATRIX_RESOURCE_RELAY_URL"))
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.Port() == "" {
		t.Fatal("disposable loopback Synapse required")
	}
	token := os.Getenv("SYTUE_MATRIX_RESOURCE_RELAY_AS_TOKEN")
	rotatedToken := os.Getenv("SYTUE_MATRIX_RESOURCE_RELAY_ROTATED_TOKEN")
	upstream := httputil.NewSingleHostReverseProxy(target)
	var requests, stableDevice, wrongToken atomic.Int64
	var drop atomic.Bool
	var observedRef atomic.Value
	upstream.ModifyResponse = func(response *http.Response) error {
		if strings.Contains(response.Request.URL.Path, "/send/m.room.encrypted/") && drop.CompareAndSwap(true, false) {
			raw, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				return err
			}
			var receipt mautrix.RespSendEvent
			if json.Unmarshal(raw, &receipt) != nil || receipt.EventID == "" {
				return errors.New("missing synthetic receipt")
			}
			observedRef.Store(storage.Reference{RoomID: response.Request.URL.Query().Get("fixture_room"), EventID: receipt.EventID.String()})
			return errors.New("synthetic accepted ACK loss")
		}
		return nil
	}
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/send/m.room.encrypted/") {
			requests.Add(1)
			if r.URL.Query().Get("device_id") != "" && r.URL.Query().Get("org.matrix.msc3202.device_id") == "" {
				stableDevice.Add(1)
			}
			if (r.Header.Get("Authorization") != "Bearer "+token && (rotatedToken == "" || r.Header.Get("Authorization") != "Bearer "+rotatedToken)) || r.URL.Query().Get("access_token") != "" {
				wrongToken.Add(1)
			}
		}
		upstream.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	cli, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cli.Client = proxy.Client()
	actor := uuid.NewString()
	password := uuid.NewString()
	registered := matrixprooftest.Register(ctx, t, cli, "sytue_"+actor, password, "RESOURCE_DEVICE")
	room, err := cli.CreateRoom(ctx, &mautrix.ReqCreateRoom{Preset: "private_chat", CreationContent: map[string]any{"m.federate": false}, InitialState: []*event.Event{{Type: event.StateEncryption, StateKey: new(""), Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, server, err := registered.UserID.Parse()
	if err != nil {
		t.Fatal(err)
	}
	// Actual maintained Olm account and Megolm stream. Current account/device
	// activity is a synthetic fixture witness; product enrollment remains a
	// separate provider integration, not a permission fabricated by this test.
	account := crypto.NewOlmAccount()
	scope := storage.Scope{Tenant: "resource-fixture", Subject: actor, Product: "rooms"}
	chain := matrixprooftest.NewUser(t, scope, server, registered.UserID, registered.DeviceID)
	public := chain.Origin.Keys.DeviceKeys[registered.UserID][registered.DeviceID]
	public.Keys[id.NewDeviceKeyID(id.KeyAlgorithmEd25519, registered.DeviceID)] = account.SigningKey().String()
	public.Keys[id.NewDeviceKeyID(id.KeyAlgorithmCurve25519, registered.DeviceID)] = account.IdentityKey().String()
	public.Signatures = matrixprooftest.Sign(t, chain.Self, &public, registered.UserID, chain.Self.PublicKey().String())
	own, err := account.SignJSON(&public)
	if err != nil {
		t.Fatal(err)
	}
	public.Signatures[registered.UserID][id.NewKeyID(id.KeyAlgorithmEd25519, registered.DeviceID.String())] = own
	chain.Origin.Keys.DeviceKeys[registered.UserID][registered.DeviceID] = public
	if _, err := cli.UploadKeys(ctx, &mautrix.ReqUploadKeys{DeviceKeys: &public}); err != nil {
		t.Fatal(err)
	}
	signer := func(value any) signatures.Signatures {
		signature, err := account.SignJSON(value)
		if err != nil {
			t.Fatal(err)
		}
		return signatures.NewSingleSignature(registered.UserID, id.KeyAlgorithmEd25519, registered.DeviceID.String(), signature)
	}
	outbound, err := olm.NewOutboundGroupSession()
	if err != nil {
		t.Fatal(err)
	}
	inbound, err := olm.NewInboundGroupSession([]byte(outbound.Key()))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := outbound.Encrypt([]byte("unpublished synthetic setup"))
	if err != nil {
		t.Fatal(err)
	}
	encrypted := event.EncryptedEventContent{Algorithm: id.AlgorithmMegolmV1, SenderKey: account.IdentityKey(), DeviceID: registered.DeviceID, SessionID: outbound.ID(), MegolmCiphertext: initial}
	raw, err := json.Marshal(&encrypted)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SDK route diagnostics scope=%v room_colon=%v room_len=%d", scope.Valid(), strings.Contains(room.RoomID.String(), ":"), len(room.RoomID.String()))
	fixture := resourcetest.NewWithSDK(t, false, chain.Origin, chain.Master.PublicKey(), signer, room.RoomID.String(), raw)
	access, err := resource.Authorize(fixture.Epoch, fixture.Cap, scope.Subject, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	window, err := resource.CurrentEpoch(fixture.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := resource.BindOwnedEpoch(window, fixture.EpochIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	original := []byte("{\"seq\":9223372036854775807,\"text\":\"private original\"}")
	envelope, err := handle.SealFrame(ctx, fixture.Candidate.Proof.Publication, original, access, resourceTransportFence{})
	if err != nil {
		t.Fatal(err)
	}
	inner, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	encrypted.MegolmCiphertext, err = outbound.Encrypt(inner)
	if err != nil {
		t.Fatal(err)
	}
	candidate := fixture.Candidate
	candidate.EncryptedEvent, err = json.Marshal(&encrypted)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Proof.InnerCipherSHA256, err = resource.CipherJSONDigest(inner)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Proof.OuterCipherSHA256, err = resource.OuterCipherDigest(candidate.EncryptedEvent)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Proof.Signatures = signer(candidate.Proof)
	verified, err := resource.VerifyCandidate(ctx, candidate, access, fixture.Device, nil)
	if err != nil {
		t.Fatal(err)
	}
	relay, err := NewResourceRelay(ResourceRelayConfig{InternalURL: proxy.URL, ServerName: server, ASToken: token}, proxy.Client())
	if err != nil {
		t.Fatal(err)
	}
	marker := publication.Marker{Version: 1, ID: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{13}, 32))}
	if _, err := relay.Send(ctx, candidate, resource.VerifiedPublication{}, marker); err == nil || requests.Load() != 0 {
		t.Fatal("raw candidate substituted for verified producer")
	}
	rebound := candidate
	rebound.Proof.Actor.DeviceID = "DIFFERENT_DEVICE"
	if _, err := relay.Send(ctx, rebound, verified, marker); err == nil || requests.Load() != 0 {
		t.Fatal("transport rebound verified SDK producer")
	}
	drop.Store(true)
	if _, err := relay.Send(ctx, candidate, verified, marker); err == nil {
		t.Fatal("lost accepted ACK was falsely confirmed")
	}
	// This direct transport retry checks the server's stable-device dedup only.
	// Production dispatch MUST reconcile its durable hold rather than retry here.
	first := observedRef.Load()
	if first == nil {
		t.Fatal("fixture did not observe accepted event")
	}
	accepted := first.(storage.Reference)
	accepted.RoomID = room.RoomID.String()
	expectedRequests := int64(2)
	if signal := os.Getenv("SYTUE_MATRIX_RESOURCE_RELAY_ROTATION_SIGNAL"); signal != "" {
		if !strings.HasPrefix(signal, "/tmp/sytue-resource-relay-synapse-20261102.") || len(rotatedToken) < 32 {
			t.Fatal("isolated rotation fixture required")
		}
		if err := os.WriteFile(signal, []byte("rotate"), 0o600); err != nil {
			t.Fatal(err)
		}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, err := os.Stat(signal + ".done"); err == nil {
				break
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				t.Fatal("AS fixture rotation did not complete")
			}
		}
		if _, err := relay.Send(ctx, candidate, verified, marker); err == nil {
			t.Fatal("retired AS credential remained active")
		}
		expectedRequests++
		relay, err = NewResourceRelay(ResourceRelayConfig{InternalURL: proxy.URL, ServerName: server, ASToken: rotatedToken}, proxy.Client())
		if err != nil {
			t.Fatal(err)
		}
	}
	ref, err := relay.Send(ctx, candidate, verified, marker)
	if err != nil || ref != accepted {
		t.Fatal("SDK device/txn dedup changed accepted event", err)
	}
	if requests.Load() != expectedRequests || stableDevice.Load() != expectedRequests || wrongToken.Load() != 0 {
		t.Fatal("AS transport lost exact device or leaked credential into query")
	}
	readerAccess, err := resource.Authorize(fixture.Epoch, fixture.Cap, scope.Subject, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	source, err := relay.AcceptanceReader(readerAccess, resourceTransportFence{})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := source.AcceptedEvent(ctx, ref)
	if err != nil {
		t.Fatal("authorized AS reader could not read exact event without asserting producer device", err)
	}
	acceptedWire, err := json.Marshal(&wire.Content)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(acceptedWire, original) {
		t.Fatal("AS saw plaintext")
	}
	var cipher event.EncryptedEventContent
	if json.Unmarshal(acceptedWire, &cipher) != nil {
		t.Fatal("accepted SDK wire invalid")
	}
	plain, _, err := inbound.Decrypt(cipher.MegolmCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	var actual pqc.Envelope
	if json.Unmarshal(plain, &actual) != nil {
		t.Fatal("inner hybrid frame missing")
	}
	pending, err := resource.VerifyProducerCandidate(ctx, candidate, access, fixture.Device)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := resource.Authorize(fixture.Epoch, fixture.Cap, scope.Subject, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := handle.OpenFrame(ctx, pending, actual, reader, resourceTransportFence{})
	if err != nil || !bytes.Equal(recovered, original) {
		t.Fatal("real SDK/AS changed hybrid payload", err)
	}
	if _, err := resource.ConfirmAcceptance(ctx, verified, ref, source); err != nil {
		t.Fatal(err)
	}
	// Revoke the producer's actual Synapse device. ACK-only observer access still
	// works, while a retained attempt cannot publish under a nonexistent device.
	if _, err := cli.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := relay.Send(ctx, candidate, verified, marker); err == nil {
		t.Fatal("AS published as revoked device")
	}
	if _, err := resource.ConfirmAcceptance(ctx, verified, ref, source); err != nil {
		t.Fatal("accepted receipt became unrecoverable after device revocation", err)
	}
	t.Log("real SDK Megolm + hybrid resource bytes, stable AS device/txn dedup, lost ACK, revoked-device rejection and authorized reader receipt after actual AS credential rotation verified")
}
