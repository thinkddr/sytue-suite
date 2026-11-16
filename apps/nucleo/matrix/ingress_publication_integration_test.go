// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
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

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// The maintained state store is delegated unchanged. Only this fixture's
// ingress read queues a real writer while sendEventWithTxn holds stateMu.RLock.
// TryRLock cannot fail for that reader until the writer is actually waiting.
type ingressWaitingWriterStore struct {
	mautrix.StateStore
	client *Client
	armed  atomic.Bool
	done   chan struct{}
}

func (s *ingressWaitingWriterStore) GetRoomJoinedOrInvitedMembers(ctx context.Context, room id.RoomID) ([]id.UserID, error) {
	members, err := s.StateStore.GetRoomJoinedOrInvitedMembers(ctx, room)
	if err != nil || ctx.Value(storagePublicationContext{}) == nil || !s.armed.CompareAndSwap(true, false) {
		return members, err
	}
	go func() {
		s.client.stateMu.Lock()
		s.client.cfg.Subject = "changed-after-encryption"
		s.client.stateMu.Unlock()
		close(s.done)
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for s.client.stateMu.TryRLock() {
		s.client.stateMu.RUnlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
	return members, nil
}

func testIngressWaitingWriterRevalidation(ctx context.Context, t *testing.T, c *Client, target storage.PublicationTarget, record storage.Record) {
	t.Helper()
	c.stateMu.Lock()
	original := c.cli.StateStore
	wrapped := &ingressWaitingWriterStore{StateStore: original, client: c, done: make(chan struct{})}
	wrapped.armed.Store(true)
	c.cli.StateStore = wrapped
	c.stateMu.Unlock()
	defer func() {
		c.stateMu.Lock()
		c.cfg.Subject = record.Scope.Subject
		c.cli.StateStore = original
		c.stateMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := c.PutStorageRecordAt(ctx, target, "writer_revalidation", record); !errors.Is(err, ErrIdentity) {
		t.Fatal("writer did not progress and invalidate ingress before Send", err)
	}
	select {
	case <-wrapped.done:
	case <-ctx.Done():
		t.Fatal("pending writer deadlocked with synchronous backup")
	}
}

// This is the recovery prerequisite for durable ingress. The queue/worker
// vertical separately owns claims and ACKs; this fixture never enables Begin.
func TestIngressPublicationBacksUpBeforeSendAndFreshDeviceRecoversWithoutRepublishing(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_INGRESS") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	target, err := url.Parse(os.Getenv("SYTUE_INGRESS_SYNAPSE_URL"))
	if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" || target.Port() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		t.Fatal("fixture requires explicit remote loopback")
	}
	var failBackup, dropSend atomic.Bool
	var sends, secretLeaks atomic.Int32
	accepted := make(chan storage.Reference, 1)
	upstream := httputil.NewSingleHostReverseProxy(target)
	upstream.ModifyResponse = func(response *http.Response) error {
		if response.Request.Method == http.MethodPut && strings.Contains(response.Request.URL.Path, "/rooms/") && strings.Contains(response.Request.URL.Path, "/send/m.room.encrypted/") && dropSend.CompareAndSwap(true, false) {
			raw, err := io.ReadAll(io.LimitReader(response.Body, 2048))
			_ = response.Body.Close()
			if err != nil {
				return err
			}
			defer clear(raw)
			var body struct {
				EventID string `json:"event_id"`
			}
			if json.Unmarshal(raw, &body) != nil || body.EventID == "" {
				return errors.New("fixture missing accepted receipt")
			}
			accepted <- storage.Reference{EventID: body.EventID}
			return errors.New("synthetic lost accepted ingress ACK")
		}
		return nil
	}
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/account_data/org.sytue.ingress.key.v1.") {
			raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
			_ = r.Body.Close()
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if bytes.Contains(raw, []byte("AGE-SECRET-KEY")) || bytes.Contains(raw, []byte("\"identity\"")) {
				secretLeaks.Add(1)
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/room_keys/keys") && failBackup.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/rooms/") && strings.Contains(r.URL.Path, "/send/m.room.encrypted/") {
			sends.Add(1)
		}
		upstream.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	scope := storage.Scope{Tenant: "ingress-fixture", Subject: "owner", Product: "mail"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	a := device.Client
	defer func() { _ = a.Close() }()
	epoch := "11111111-1111-4111-8111-111111111111"
	plan, err := a.CreateIngressKey(ctx, scope, epoch, 1)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Profile != ingress.HybridProfile {
		t.Fatal("new ingress enrollment did not select post-quantum hybrid encryption")
	}
	proofAction := testIngressSDKPublicAuthorization(ctx, t, device)
	if secretLeaks.Load() != 0 {
		t.Fatal("private ingress identity reached the server")
	}
	body := bytes.Repeat([]byte("artificial private SMTP input / 漢字 é\r\n"), 40000)
	metadata := []byte(`{"mailbox_id":"synthetic","future":9007199254740993}`)
	var ciphertext bytes.Buffer
	task, err := agecipher.Seal(ctx, &ciphertext, plan, "input_01", "mail.receive", metadata, bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	publication, err := a.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	work, err := a.RequireRecoverableIngress(ctx, scope, publication)
	if err != nil {
		t.Fatal(err)
	}
	content := storage.Record{Version: 1, Scope: scope, ObjectID: "ingress_input_01", RevisionID: "content_01", Role: "content", Payload: []byte(`{"kind":"ingress_commit.v1","task_id":"input_01"}`)}
	failBackup.Store(true)
	// Ordinary storage callers must receive the same guarantee without opting in
	// to the ingress context. In particular this covers sidecar/FileLink records.
	if _, err := a.PutStorageRecordAt(ctx, publication, "generic_storage", content); err == nil || sends.Load() != 0 {
		t.Fatal("ordinary storage bypassed backup before Send", err, sends.Load())
	}
	if _, err := a.PutStorageRecordAt(work, publication, "ingress_content", content); err == nil || sends.Load() != 0 {
		t.Fatal("failed backup permitted a publication", err, sends.Load())
	}
	failBackup.Store(false)
	testStoragePublicationPolicyAfterBackup(ctx, t, a, publication, content, &sends)
	testIngressWaitingWriterRevalidation(work, t, a, publication, content)
	if sends.Load() != 0 {
		t.Fatal("changed state reached Send")
	}
	contentRef, err := a.PutStorageRecordAt(work, publication, "ingress_content", content)
	if err != nil {
		t.Fatal(err)
	}
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{"kind":"ingress_commit.v1","task_id":"input_01"}`), Content: &storage.ContentPointer{Kind: "record", RevisionID: content.RevisionID, Reference: contentRef}}
	dropSend.Store(true)
	if _, err := a.PutStorageObjectAt(work, publication, "ingress_index", content.ObjectID, "index_01", commit); err == nil {
		t.Fatal("accepted commit ACK was not interrupted")
	}
	var receipt storage.Reference
	select {
	case receipt = <-accepted:
		receipt.RoomID = device.Room.String()
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	before := sends.Load()
	login, err := device.Matrix.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: device.Matrix.UserID.String()}, Password: device.Password, DeviceID: "INGRESS_FRESH_DEVICE"})
	if err != nil {
		t.Fatal(err)
	}
	config := device.Config
	config.StoreDir = t.TempDir()
	config.Secrets = &memorySecrets{values: map[string][]byte{}}
	b := storageSynapseSession(ctx, t, config, credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: scope.Subject, Tenant: scope.Tenant})
	defer func() { _ = b.Close() }()
	go func() { _ = b.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := b.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.OpenIngressBody(ctx, task, bytes.NewReader(ciphertext.Bytes())); err == nil {
		t.Fatal("token-only device opened the ingress identity")
	}
	unverifiedAction := proofAction
	unverifiedAction.Target.DeviceID = login.DeviceID.String()
	if _, err := b.SignIngressAction(ctx, ingressFixtureWorkLease, unverifiedAction.Intent(), unverifiedAction); err == nil {
		t.Fatal("token-only device signed an ingress enrollment")
	}
	if err := b.Recover(ctx, device.Recovery); err != nil {
		t.Fatal(err)
	}
	testIngressSDKPublicAuthorization(ctx, t, storageSynapseDevice{Client: b, Matrix: b.cli})
	actual, opened, err := b.OpenIngressBody(ctx, task, bytes.NewReader(ciphertext.Bytes()))
	if err != nil {
		t.Fatal("fresh SSSS recipient recovery failed", err)
	}
	defer clear(actual)
	if !bytes.Equal(actual, metadata) {
		t.Fatal("fresh metadata changed")
	}
	var restored bytes.Buffer
	if _, err := io.Copy(&restored, opened); err != nil || !bytes.Equal(restored.Bytes(), body) {
		t.Fatal("fresh original MIME changed", err)
	}
	defer clear(restored.Bytes())
	if _, err := device.Matrix.Logout(ctx); err != nil {
		t.Fatal("real device fence failed", err)
	}
	if _, err := a.PutStorageRecordAt(work, publication, "fenced_ingress", content); err == nil {
		t.Fatal("revoked device retained publication access")
	}
	if _, err := b.StorageObject(ctx, receipt, content.ObjectID); err != nil {
		t.Fatal("fresh device could not reconcile the pre-fence commit", err)
	}
	snapshot, err := storage.LoadCatalog(ctx, b, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Heads) != 1 || snapshot.Heads[0].Reference != receipt {
		t.Fatal("handoff duplicated or lost the accepted commit")
	}
	if sends.Load() != before {
		t.Fatal("reconciliation published the task twice", before, sends.Load())
	}
	if _, err := device.Matrix.SendMessageEvent(ctx, device.Room, event.EventEncrypted, map[string]string{"algorithm": "m.megolm.v1.aes-sha2"}, mautrix.ReqSendEvent{TransactionID: "fenced_device_probe"}); err == nil {
		t.Fatal("homeserver still accepted the retired device")
	}
	t.Log("INGRESS_SDK_RECOVERY_PREREQUISITE_PASS backup-failure zeroSend, lost accepted ACK, ciphertext-only SSSS custody, token-only denial, fresh recovery, real prior-device fence and reconciliation without republication")
}

// The native SDK supplies the trusted own master independently from the queried
// server keys. Production authority additionally owns the durable root pin and
// one-use challenge transaction; this checks actual SDK wire interoperability.
func testIngressSDKPublicAuthorization(ctx context.Context, t *testing.T, device storageSynapseDevice) matrixproof.Action {
	t.Helper()
	c := device.Client
	scope := c.storageScope()
	target, err := c.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	keys, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil || keys == nil {
		t.Fatal("own SDK trust root missing", err)
	}
	query, err := device.Matrix.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{device.Matrix.UserID: mautrix.DeviceIDList{device.Matrix.DeviceID}}})
	if err != nil {
		t.Fatal(err)
	}
	_, server, _ := strings.Cut(device.Matrix.UserID.String(), ":")
	verified, err := matrixproof.Verify(ctx, matrixproof.Origin{Scope: scope, ServerName: server, UserID: device.Matrix.UserID, DeviceID: device.Matrix.DeviceID, AccountActive: true, DeviceActive: true, Keys: *query}, keys.MasterKey)
	if err != nil {
		t.Fatal("real SDK device chain rejected", err)
	}
	localRoot, err := c.StorageRootConsentRequest(ctx, ingressFixtureWorkLease, scope, target, "root.enroll", nil)
	if err != nil || localRoot.RootKey != keys.MasterKey.String() || localRoot.UserID != device.Matrix.UserID.String() || localRoot.Scope != scope || localRoot.Target != target || localRoot.Generation != 1 {
		t.Fatal("SDK root consent did not retain original verified identity", err)
	}
	requestDigest, err := matrixproof.RequestDigest(map[string]any{"generation": 1, "recipient": "synthetic_public_request"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	action := matrixproof.Action{Version: 1, Generation: 1, Scope: scope, Epoch: "11111111-1111-4111-8111-111111111111", Kind: "key.enroll", Target: target, RootKey: keys.MasterKey.String(), RequestSHA256: requestDigest, Nonce: "independent_fixture_challenge", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
	signed, err := c.SignIngressAction(ctx, ingressFixtureWorkLease, action.Intent(), action)
	if err != nil {
		t.Fatal("verified SDK did not sign exact action", err)
	}
	if err := verified.VerifyAction(ctx, signed, action, time.Now()); err != nil {
		t.Fatal("real SDK signature did not verify", err)
	}
	substituted := action
	substituted.RequestSHA256, _ = matrixproof.RequestDigest(map[string]any{"recipient": "different"})
	if _, err := c.SignIngressAction(ctx, ingressFixtureWorkLease, action.Intent(), substituted); err == nil {
		t.Fatal("SDK signed server-substituted request without client intent")
	}

	altered := action
	altered.Epoch = "22222222-2222-4222-8222-222222222222"
	if err := verified.VerifyAction(ctx, signed, altered, time.Now()); err == nil {
		t.Fatal("server accepted changed SDK transcript")
	}
	t.Log("INGRESS_REAL_SDK_PUBLIC_CHAIN_AND_ACTION_PASS")
	return action
}

func ingressFixtureWorkLease(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }

// This policy deliberately needs the SDK writer lock, as a real signer or
// lifecycle check can. It also denies only after backup has completed.
func testStoragePublicationPolicyAfterBackup(ctx context.Context, t *testing.T, client *Client, target storage.PublicationTarget, record storage.Record, sends *atomic.Int32) {
	t.Helper()
	denied := errors.New("synthetic authority revoked after backup")
	var calls atomic.Int32
	policy := func(ctx context.Context) error {
		wait, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for !client.stateMu.TryLock() {
			select {
			case <-wait.Done():
				return errors.New("policy called under SDK state lock")
			case <-tick.C:
			}
		}
		client.stateMu.Unlock()
		if calls.Add(1) == 3 {
			return denied
		}
		return nil
	}
	bound, err := client.RequireAuthorizedStoragePublication(ctx, record.Scope, target, policy)
	if err != nil {
		t.Fatal(err)
	}
	bound, err = client.RequireRecoverableStorage(bound, record.Scope, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.PutStorageRecordAt(bound, target, "denied_after_backup", record); !errors.Is(err, denied) || calls.Load() != 3 || sends.Load() != 0 {
		t.Fatal("post-backup revocation permitted Send or lost policy", err, calls.Load(), sends.Load())
	}
}
