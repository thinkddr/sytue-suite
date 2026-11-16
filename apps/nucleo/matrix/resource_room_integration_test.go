//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
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
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

// The approval/root lifecycle witness is synthetic. Device keys, signatures,
// encryption, SDK SQLite, SSSS, backup ACK and all content HTTP are REAL Synapse.
// Native purpose consent/Registry has a separate real-PG/SDK integration proof.
type nativeResourceRoomFixtureSource struct {
	client     *Client
	record     resource.SessionRecord
	active     atomic.Bool
	fenceCalls atomic.Int64
	failAt     atomic.Int64
}

func (s *nativeResourceRoomFixtureSource) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, error) {
	if err := s.CheckResourceSession(ctx, s.record.Binding); err != nil {
		return resource.SessionRecord{}, err
	}
	if scope != s.record.Binding.Scope || device != s.record.Binding.DeviceID {
		return resource.SessionRecord{}, ErrIdentity
	}
	return s.record, nil
}

func (s *nativeResourceRoomFixtureSource) CheckResourceSession(ctx context.Context, b resource.SessionBinding) error {
	if !s.active.Load() || b != s.record.Binding {
		return resource.ErrRevoked
	}
	return ctx.Err()
}

func (s *nativeResourceRoomFixtureSource) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	if !s.active.Load() {
		return matrixproof.Device{}, resource.ErrRevoked
	}
	c := s.client
	who, err := c.cli.Whoami(ctx)
	if err != nil || who.UserID != c.session.UserID || who.DeviceID != c.session.DeviceID {
		return matrixproof.Device{}, ErrIdentity
	}
	keys, err := c.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: map[id.UserID]mautrix.DeviceIDList{c.session.UserID: {c.session.DeviceID}}})
	if err != nil {
		return matrixproof.Device{}, err
	}
	_, server, err := c.session.UserID.Parse()
	if err != nil {
		return matrixproof.Device{}, err
	}
	return matrixproof.VerifyUser(ctx, matrixproof.Origin{Scope: scope, ServerName: server, UserID: c.session.UserID, DeviceID: id.DeviceID(device), AccountActive: true, DeviceActive: true, Keys: *keys}, c.session.UserID, id.Ed25519(root))
}

func (s *nativeResourceRoomFixtureSource) Revalidate(ctx context.Context, a resource.VerifiedAccess) error {
	if s.fenceCalls.Add(1) == s.failAt.Load() {
		s.active.Store(false)
	}
	if !s.active.Load() || a.Scope() != s.record.Binding.Scope || a.Actor() != s.record.Binding.Actor() {
		return resource.ErrRevoked
	}
	return ctx.Err()
}

func TestNativeResourceRoomRealSDKRetainsCiphertextAcrossRestartAndFencesReads(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PUBLICATION_PREPARE") != "1" {
		t.Skip("isolated remote Synapse required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	target, err := url.Parse(os.Getenv("SYTUE_MATRIX_PUBLICATION_PREPARE_URL"))
	if err != nil || target.Host == "" {
		t.Fatal("isolated fixture URL required")
	}
	var missingProof atomic.Value
	missingProof.Store("")
	reverse := httputil.NewSingleHostReverseProxy(target)
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := missingProof.Load().(string)
		if name != "" && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, name) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errcode":"M_NOT_FOUND","error":"synthetic absent device-local provenance"}`))
			return
		}
		reverse.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	actor := uuid.NewString()
	scope := storage.Scope{Subject: actor, Tenant: "resource-frame-fixture", Product: "docs"}
	raw, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	raw.Client = proxy.Client()
	password := uuid.NewString()
	registered := matrixprooftest.Register(ctx, t, raw, "sytue_"+actor, password, "RESOURCE_TRANSPORT")
	roomID := newPrivateStorageTestRoom(ctx, t, raw)
	_, server, _ := registered.UserID.Parse()
	cfg := Config{ResourceProduct: "docs", Bootstrap: Bootstrap{Actor: actor, Tenant: scope.Tenant, HomeserverURL: proxy.URL, MatrixUserID: registered.UserID.String(), ControllerUserID: "@rooms_controller:" + server, Encryption: string(id.AlgorithmMegolmV1)}, Subject: actor, Tenant: scope.Tenant, StoreDir: t.TempDir(), Secrets: &memorySecrets{values: map[string][]byte{}}, HTTPClient: proxy.Client()}
	c := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: proxy.URL, UserID: registered.UserID, DeviceID: registered.DeviceID, AccessToken: registered.AccessToken, Subject: actor, Tenant: scope.Tenant, ResourceProduct: "docs"})
	defer func() { _ = c.Close() }()
	canonicalRecovery, err := c.BootstrapRecovery(ctx, storageFixtureUIA(registered.UserID, password))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	roots, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil || roots == nil {
		t.Fatal(err)
	}
	leaf, err := c.resourceLeaf(true)
	if err != nil {
		t.Fatal(err)
	}
	source := &nativeResourceRoomFixtureSource{client: c}
	source.active.Store(true)
	device, err := source.ResourceSessionDevice(ctx, scope, c.DeviceID(), roots.MasterKey.String())
	if err != nil {
		t.Fatal(err)
	}
	sign := func(value any) signatures.Signatures {
		signatures, err := c.signResourceJSON(ctx, value)
		if err != nil {
			t.Fatal(err)
		}
		return signatures
	}
	fixtureEncrypted, err := c.crypto.Encrypt(ctx, roomID, event.EventMessage, event.MessageEventContent{MsgType: event.MsgText, Body: "bootstrap-fixture-not-published"})
	if err != nil {
		t.Fatal(err)
	}
	fixtureCipher, err := json.Marshal(fixtureEncrypted)
	if err != nil {
		t.Fatal(err)
	}
	query, err := c.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: map[id.UserID]mautrix.DeviceIDList{c.session.UserID: {c.session.DeviceID}}})
	if err != nil {
		t.Fatal(err)
	}
	fixture := resourcetest.NewWithSDKRecipient(t, false, matrixproof.Origin{Scope: scope, ServerName: server, UserID: c.session.UserID, DeviceID: c.session.DeviceID, AccountActive: true, DeviceActive: true, Keys: *query}, roots.MasterKey, sign, roomID.String(), fixtureCipher, leaf)
	recipient := resource.SignedRecipient{RecipientEnrollment: fixture.Enrolled.Value()}
	recipient.Signatures = sign(recipient)
	binding := resource.SessionBinding{Scope: scope, Namespace: "resources:docs", UserID: c.session.UserID.String(), DeviceID: c.DeviceID(), RootKey: roots.MasterKey.String(), Recipient: leaf.Recipient().String(), ProductKeyID: fixture.Enrolled.Value().KeyID, EnrollmentSHA256: fixture.Enrolled.SHA256(), Generation: "1", RootGeneration: "1", AccountVersion: "1", TenantVersion: "1", ConsentID: uuid.NewString()}
	now := time.Now()
	challenge, err := resource.NewSessionChallenge(ctx, binding, true, now.Add(5*time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := resource.OpenSessionChallenge(ctx, challenge, leaf, now)
	if err != nil {
		t.Fatal(err)
	}
	challengeHash, _ := resource.SessionChallengeSHA256(challenge)
	enrollment := resource.SignedSessionEnrollment{SessionEnrollment: resource.SessionEnrollment{Version: 1, Domain: "org.sytue.resource.session.enrollment.v1", ChallengeID: challenge.ID, ChallengeSHA256: challengeHash, Binding: binding, Nonce: nonce, RootDecision: "confirm_current_root"}}
	enrollment.Signatures = sign(enrollment)
	verifiedEnrollment, err := resource.VerifySessionEnrollment(ctx, enrollment, recipient, challenge, device, now)
	if err != nil {
		t.Fatal(err)
	}
	source.record = resource.SessionRecord{Binding: binding, PinnedRoot: binding.RootKey, RootGeneration: "1", Recipient: recipient, Challenge: challenge, Proof: enrollment, ProofSHA256: verifiedEnrollment.SHA256(), ConfirmedAt: now}
	provider, err := resource.NewSessionProvider(source, source)
	if err != nil {
		t.Fatal(err)
	}
	session, err := provider.Resolve(ctx, scope, c.DeviceID())
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
	journalDir := t.TempDir()
	journal, err := offline.OpenWithSecrets(journalDir, cfg.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	var work atomic.Int64
	lease := func(ctx context.Context) (func(), error) {
		if work.Add(1) != 1 {
			work.Add(-1)
			return nil, errors.New("nested resource work lease")
		}
		return func() { work.Add(-1) }, ctx.Err()
	}
	exerciseNativeResourceSourceQuery(t, ctx, c, session, fixture.Epoch.Value().Resource, roomID.String(), lease)
	bootstrapSaved := exerciseNativeResourceBootstrap(ctx, t, c, session, fixture, provider, lease, journal)
	access, err := resource.AuthorizeSession(ctx, fixture.Epoch, fixture.Cap, session, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	room, err := c.BindResourceRoom(ctx, session, access, handle, source, lease, journal)
	if err != nil {
		t.Fatal(err)
	}
	header := fixture.Candidate.Proof.Publication
	header.TransactionID = uuid.NewString()
	header.OpID = "actual-sdk-resource-op"
	record := storage.Record{Version: 1, Scope: room.ActorScope(), ObjectID: "document-object", RevisionID: "revision-1", Role: "content", Payload: []byte(`{"seq":9007199254740993,"operations":"original private document"}`)}
	payload, err := resource.EncodeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	foreignRecord := record
	foreignRecord.Scope.Subject = "another-account"
	if prepared, err := room.PrepareRecord(ctx, header, foreignRecord); err == nil || prepared != nil {
		t.Fatal("foreign private scope reached resource SDK preparation")
	}
	prepared, err := room.PrepareRecord(ctx, header, record)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := prepared.Candidate()
	if err != nil {
		t.Fatal(err)
	}
	candidateRaw, _ := json.Marshal(candidate)
	if bytes.Contains(candidateRaw, payload) {
		t.Fatal("candidate exposed plaintext")
	}
	if _, err := prepared.prepared.Send(ctx, nil); !errors.Is(err, ErrPublicationAdmissionRequired) {
		t.Fatal("resource escaped common CAS relay", err)
	}
	request, err := prepared.SignAdmission(ctx)
	if err != nil || request.Request.Namespace != "resources:docs" {
		t.Fatal("resource admission lost namespace", err)
	}
	pending, err := resource.VerifyProducerCandidate(ctx, candidate, access, session.Device())
	if err != nil {
		t.Fatal(err)
	}
	// This is an opaque transport fixture, not a claim that ResourceRegistry0073,
	// the AS/origin admission and product ACL gateway have been composed yet.
	response, err := c.cli.SendMessageEvent(ctx, roomID, event.EventEncrypted, json.RawMessage(candidate.EncryptedEvent), mautrix.ReqSendEvent{TransactionID: header.TransactionID})
	if err != nil {
		t.Fatal(err)
	}
	ref := storage.Reference{RoomID: roomID.String(), EventID: response.EventID.String()}
	if _, err := c.Read(ctx, EventRef{RoomID: ref.RoomID, EventID: ref.EventID}); !errors.Is(err, ErrIdentity) {
		t.Fatal("raw resource read bypassed typed room", err)
	}
	readAccess, err := resource.AuthorizeSession(ctx, fixture.Epoch, fixture.Cap, session, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := c.BindResourceRoom(ctx, session, readAccess, handle, source, lease, journal)
	if err != nil {
		t.Fatal(err)
	}
	cleartext, err := reader.Read(ctx, ref, pending, candidate)
	if err != nil || !bytes.Equal(cleartext, payload) {
		t.Fatal("actual resource SDK read failed", err)
	}
	clear(cleartext)
	checkNativeCommittedRead(t, ctx, reader, access, candidate, ref, payload)
	// A second real event can carry the SAME authenticated inner frame but a
	// different SDK ciphertext. Its receipt must not substitute the staged wire.
	originalEvent, err := c.cli.GetEvent(ctx, roomID, response.EventID)
	if err != nil {
		t.Fatal(err)
	}
	originalEvent.RoomID = roomID
	if err := originalEvent.Content.ParseRaw(originalEvent.Type); err != nil {
		t.Fatal(err)
	}
	originalClear, err := c.crypto.Decrypt(ctx, originalEvent)
	if err != nil {
		t.Fatal(err)
	}
	originalFrame, err := eventContentBytes(originalClear)
	if err != nil {
		t.Fatal(err)
	}
	alternateCipher, err := c.crypto.Encrypt(ctx, roomID, event.Type{Type: resource.FrameEventType, Class: event.MessageEventType}, json.RawMessage(originalFrame))
	clear(originalFrame)
	if err != nil {
		t.Fatal(err)
	}
	alternate, err := c.cli.SendMessageEvent(ctx, roomID, event.EventEncrypted, alternateCipher, mautrix.ReqSendEvent{TransactionID: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := reader.Read(ctx, storage.Reference{RoomID: roomID.String(), EventID: alternate.EventID.String()}, pending, candidate); err == nil || len(out) != 0 {
		t.Fatal("different SDK ciphertext substituted staged receipt")
	}
	source.failAt.Store(source.fenceCalls.Load() + 3)
	if out, err := reader.Read(ctx, ref, pending, candidate); !errors.Is(err, resource.ErrRevoked) || len(out) != 0 {
		t.Fatal("post-decrypt revocation released plaintext", err)
	}
	source.failAt.Store(0)
	source.active.Store(true)
	// A receiver may have the legitimate SDK backup while its own SSSSv2 witness
	// is absent. Public candidate/session signatures must be sufficient, without
	// treating arbitrary anonymous backups as authenticated publications.
	proofName, err := sessionproof.Encode(c.vaultScope(), prepared.prepared.Provenance())
	if err != nil {
		t.Fatal(err)
	}
	clear(proofName.Payload)
	missingProof.Store(ssssVaultEventPrefix + proofName.ID)
	c.mu.Lock()
	c.attested = make(map[attestedStorageSession]struct{})
	c.mu.Unlock()
	if err := c.verifyRoomSessionProof(ctx, prepared.prepared.Provenance()); !errors.Is(err, ErrSessionProvenanceMissing) {
		t.Fatal("fixture did not remove local provenance", err)
	}
	if err := c.crypto.Machine().CryptoStore.RedactGroupSession(ctx, roomID, id.SessionID(prepared.prepared.Provenance().SessionID), "resource public certificate recovery test"); err != nil {
		t.Fatal(err)
	}
	if err := reader.RestoreBackup(ctx); err != nil {
		t.Fatal(err)
	}
	imported, err := c.crypto.Machine().CryptoStore.GetGroupSession(ctx, roomID, id.SessionID(prepared.prepared.Provenance().SessionID))
	if err != nil || imported == nil || imported.KeySource != id.KeySourceBackup {
		t.Fatal("resource recovery reused original direct key", err)
	}
	cleartext, err = reader.Read(ctx, ref, pending, candidate)
	if err != nil || !bytes.Equal(cleartext, payload) {
		t.Fatal("public certificate receiver-only recovery failed", err)
	}
	clear(cleartext)
	missingProof.Store("")
	bad := candidate
	bad.EncryptedEvent = bytes.ReplaceAll(candidate.EncryptedEvent, []byte("ciphertext"), []byte("ciphertext_changed"))
	if out, err := reader.Read(ctx, ref, pending, bad); err == nil || len(out) != 0 {
		t.Fatal("tampered candidate released plaintext")
	}
	prepared.Close()
	room.Close()
	reader.Close()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	source.client = c
	go func() { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	session, err = provider.Resolve(ctx, scope, c.DeviceID())
	if err != nil {
		t.Fatal(err)
	}
	access, err = resource.AuthorizeSession(ctx, fixture.Epoch, fixture.Cap, session, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	journal, err = offline.OpenWithSecrets(journalDir, cfg.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	exerciseNativeBootstrapAfterSDKRestart(ctx, t, c, session, lease, journal, bootstrapSaved)
	room, err = c.BindResourceRoom(ctx, session, access, handle, source, lease, journal)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := room.Prepare(ctx, header, payload)
	if err != nil {
		t.Fatal("durable exact resource restart failed", err)
	}
	defer resumed.Close()
	recovered, err := resumed.Candidate()
	if err != nil {
		t.Fatal(err)
	}
	completed, done, err := resumed.AcceptedReceipt(ctx)
	if err != nil || !done || completed.Reference != ref {
		t.Fatal("SDK reopen lost exact accepted resource receipt", done, err)
	}
	if _, err := resumed.SignAdmission(ctx); !errors.Is(err, ErrResourcePublicationAcknowledged) {
		t.Fatal("accepted resource retry created another admission", err)
	}
	recoveredRaw, _ := json.Marshal(recovered)
	if !bytes.Equal(candidateRaw, recoveredRaw) {
		t.Fatal("restart re-encrypted immutable SDK wire")
	}
	changed := header
	changed.TransactionID = uuid.NewString()
	if _, err := room.Prepare(ctx, changed, payload); !errors.Is(err, resource.ErrConflict) {
		t.Fatal("same logical op replaced retained txn", err)
	}
	source.active.Store(false)
	if _, err := resumed.SignAdmission(ctx); !errors.Is(err, resource.ErrRevoked) {
		t.Fatal("retained candidate bypassed withdrawn enrollment", err)
	}
	// The fresh-B branch receives only restarted public Source bytes, its own
	// SDK/vault recovery and the owner's sealed original work. No held policy or
	// capability is available to restore the bootstrap authority.
	bootstrapSaved.plan = resource.VerifiedBootstrapPlan{}
	fixture.Epoch, fixture.Cap = resource.VerifiedEpoch{}, resource.VerifiedCapability{}
	resumed.Close()
	room.Close()
	exerciseNativeFreshBootstrapPolicy(ctx, t, c, cfg, password, canonicalRecovery, bootstrapSaved.vault, bootstrapSaved.policy, lease, journal)
	if work.Load() != 0 {
		t.Fatal("resource transport leaked work lease")
	}
	t.Log("real SDK age+Megolm/public cert/backup; durable immutable txn/cipher restart; raw-room, tamper and revoked-enrollment gates")
}
