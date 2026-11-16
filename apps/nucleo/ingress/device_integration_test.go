// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package ingress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

const ingressProofPassword = "artificial-disposable-ingress-password"

type liveIngressDevice struct {
	Client    *matrix.Client
	Secrets   *authtest.Secrets
	Config    matrix.Config
	Proxy     *httptest.Server
	Recovery  string
	Sends     *atomic.Int32
	KeyWrites *atomic.Int32
	Barrier   *atomic.Pointer[liveIngressSendBarrier]
}

// The disposable server has no real SSO provider. Only its initial SSO flow and
// one-use login response are fixtures. The registered device, Whoami, all SDK
// crypto, cross-signing, backup, SSSS, room events and revocation use Synapse.
func openLiveIngressDevice(ctx context.Context, t *testing.T, vector matrixprooftest.IngressDeviceVector, device, recovery string) *liveIngressDevice {
	t.Helper()
	sends := &atomic.Int32{}
	keyWrites := &atomic.Int32{}
	barrier := &atomic.Pointer[liveIngressSendBarrier]{}
	bridge, err := matrixprooftest.NewLoginProxy(t, matrixprooftest.LoginProxyConfig{SynapseURL: vector.SynapseURL, UserID: vector.UserID, Password: ingressProofPassword, RoomID: vector.RoomID, Intercept: func(w http.ResponseWriter, r *http.Request, forward *httputil.ReverseProxy) bool {
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/send/m.room.encrypted/") {
			sends.Add(1)
			if current := barrier.Load(); current != nil && current.handle(w, r, forward) {
				return true
			}
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/account_data/org.sytue.ingress.key.v1.") {
			keyWrites.Add(1)
		}
		return false
	}})
	if err != nil {
		t.Fatal(err)
	}
	proxy := bridge.Server
	secrets := &authtest.Secrets{}
	server := strings.TrimPrefix(vector.UserID, storage.IsolatedMatrixUserPrefix(vector.Scope))
	config := matrix.Config{Bootstrap: matrix.Bootstrap{Actor: vector.Scope.Subject, Tenant: vector.Scope.Tenant, HomeserverURL: proxy.URL, MatrixUserID: vector.UserID, ControllerUserID: "@rooms_controller:" + server, Encryption: string(id.AlgorithmMegolmV1), StorageProduct: vector.Scope.Product, StorageIdentityVersion: storage.IsolatedIdentity, StorageRoomID: vector.RoomID, StorageRoomIDs: []string{vector.RoomID}}, Subject: vector.Scope.Subject, Tenant: vector.Scope.Tenant, DeviceName: device, StoreDir: t.TempDir(), Secrets: secrets, HTTPClient: proxy.Client()}
	client, err := matrix.Login(ctx, config, func(raw string) error { return bridge.OpenURL(ctx, raw) })
	if err != nil {
		t.Fatal("SDK public login fixture", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if recovery == "" {
		recovery, err = client.BootstrapRecovery(ctx, func(challenge *mautrix.RespUserInteractive) any {
			return map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": vector.UserID}, "password": ingressProofPassword, "session": challenge.Session}
		})
	} else {
		target, targetErr := client.StoragePublicationTarget()
		if targetErr != nil {
			t.Fatal(targetErr)
		}
		if _, err := client.StorageRootConsentRequest(ctx, liveIngressLease(t), vector.Scope, target, "root.enroll", nil); err == nil {
			t.Fatal("token-only fresh device could sign without recovering its own SSSS root")
		}
		err = client.Recover(ctx, recovery)
	}
	if err != nil {
		t.Fatal("maintained SDK recovery", err)
	}
	go func() { _ = client.Sync(ctx, func(matrix.DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	return &liveIngressDevice{Client: client, Secrets: secrets, Config: config, Proxy: proxy, Recovery: recovery, Sends: sends, KeyWrites: keyWrites, Barrier: barrier}
}

func liveIngressLease(t *testing.T) offline.WorkLease {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session-work")
	return func(ctx context.Context) (func(), error) {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, err
		}
		return ajustes.CerrojoFicheroCompartido(ctx, file)
	}
}

type liveIngressSession struct{}

func (liveIngressSession) AccessToken(ctx context.Context) (string, error) {
	return "synthetic-live-device-session", ctx.Err()
}
func (liveIngressSession) Invalidate(string) {}

type liveSDKExecutor struct {
	device  *matrix.Client
	journal *offline.Store
	scope   storage.Scope
}

func (e liveSDKExecutor) payload(ctx context.Context, metadata []byte, source io.ReaderAt, size int64) ([]byte, error) {
	if size < 1 || size > 1<<20 {
		return nil, contract.ErrContract
	}
	body, err := io.ReadAll(io.NewSectionReader(source, 0, size))
	if err != nil {
		return nil, err
	}
	defer clear(body)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(struct{ Metadata, Body []byte }{metadata, body})
}

func (e liveSDKExecutor) Reconcile(ctx context.Context, claim contract.Claim, metadata []byte, source io.ReaderAt, size int64) (*storage.Reference, error) {
	return e.reconcile(ctx, claim.Task, claim.Target, metadata, source, size)
}

func (e liveSDKExecutor) ReconcileHandoff(ctx context.Context, task contract.Task, previous contract.Assignment, metadata []byte, source io.ReaderAt, size int64) (*storage.Reference, error) {
	if !previous.Valid() {
		return nil, contract.ErrClaim
	}
	target, err := e.device.StoragePublicationTarget()
	if err != nil {
		return nil, err
	}
	record := storage.Record{Version: 1, Scope: e.scope, ObjectID: "handoff_read_probe", RevisionID: "handoff_read_probe", Role: "content", Payload: []byte(`{"probe":"read-only"}`)}
	if _, err := e.device.PutStorageRecordAt(ctx, target, "handoff_read_probe", record); !errors.Is(err, contract.ErrClaim) {
		return nil, errors.New("read-only handoff could publish through real SDK")
	}
	return e.reconcile(ctx, task, previous.Target, metadata, source, size)
}

func (e liveSDKExecutor) reconcile(ctx context.Context, task contract.Task, target storage.PublicationTarget, metadata []byte, source io.ReaderAt, size int64) (*storage.Reference, error) {
	payload, err := e.payload(ctx, metadata, source, size)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	snapshot, err := storage.LoadCatalog(ctx, e.device, e.scope)
	if err != nil {
		return nil, err
	}
	var accepted *storage.Reference
	for _, head := range snapshot.Heads {
		if head.ObjectID != "ingress_"+task.ID {
			continue
		}
		if accepted != nil || head.Commit.Content == nil || head.Commit.Content.Kind != "record" {
			return nil, contract.ErrReceipt
		}
		record, err := e.device.ReadStorageRecord(ctx, head.Commit.Content.Reference, head.ObjectID, "content")
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(record.Payload, payload) || head.Reference.RoomID != target.RoomID {
			return nil, contract.ErrReceipt
		}
		ref := head.Reference
		accepted = &ref
	}
	return accepted, nil
}

func (e liveSDKExecutor) Execute(ctx context.Context, claim contract.Claim, metadata []byte, source io.ReaderAt, size int64) (storage.Reference, error) {
	payload, err := e.payload(ctx, metadata, source, size)
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(payload)
	object := "ingress_" + claim.Task.ID
	record := storage.Record{Version: 1, Scope: e.scope, ObjectID: object, RevisionID: "content_" + claim.Task.ID, Role: "content", Payload: payload}
	owner := e.scope.Tenant + "\x00" + e.scope.Subject + "\x00" + e.scope.Product
	if err := e.journal.Write(owner, "live-ingress-publication-fixture", claim.Task.ID, record); err != nil {
		return storage.Reference{}, err
	}
	content, err := e.device.PutStorageRecordAt(ctx, claim.Target, "content_"+claim.Task.ID, record)
	if err != nil {
		return storage.Reference{}, err
	}
	details, err := json.Marshal(map[string]string{"kind": "live-ingress-commit.v1", "task_id": claim.Task.ID, "cipher_sha256": claim.Task.Cipher.SHA256})
	if err != nil {
		return storage.Reference{}, err
	}
	version, err := e.device.PutStorageObjectAt(ctx, claim.Target, "index_"+claim.Task.ID, object, "index_"+claim.Task.ID, storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: details, Content: &storage.ContentPointer{Kind: "record", RevisionID: record.RevisionID, Reference: content}})
	return version.Reference, err
}

type liveIngressFixture struct {
	ctx          context.Context
	vector       matrixprooftest.IngressDeviceVector
	device       *liveIngressDevice
	lease        offline.WorkLease
	authority    matrixproof.Authority
	sourceConfig HTTPSourceConfig
	source       *HTTPSource
	plan         contract.KeyPlan
	task         contract.Task
}

type liveIngressKeySetup struct {
	device        *liveIngressDevice
	source        *HTTPSource
	configuration HTTPSourceConfig
	target        storage.PublicationTarget
	authority     matrixproof.Authority
}

func newLiveIngressFixture(t *testing.T, initialize ...func(context.Context, liveIngressKeySetup) (contract.KeyPlan, error)) liveIngressFixture {
	t.Helper()
	if os.Getenv("SYTUE_INGRESS_DEVICE_PROOF") != "1" {
		t.Skip("requires disposable live ingress fixture")
	}
	endpoint, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_INGRESS_DEVICE_API_URL"))
	if err != nil {
		t.Fatal(err)
	}
	var vector matrixprooftest.IngressDeviceVector
	if json.Unmarshal([]byte(os.Getenv("SYTUE_INGRESS_DEVICE_VECTOR")), &vector) != nil || !vector.Scope.Valid() || vector.Scope.Product != "mail" || !storage.MigrationEpoch(vector.Epoch) || !strings.HasPrefix(vector.UserID, storage.IsolatedMatrixUserPrefix(vector.Scope)) || !strings.HasPrefix(vector.RoomID, "!") {
		t.Fatal("invalid disposable device routing")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	t.Cleanup(cancel)
	device := openLiveIngressDevice(ctx, t, vector, "LIVE_INGRESS_A", "")
	lease := liveIngressLease(t)
	rooms := api.NewRooms(endpoint.String(), liveIngressSession{}, nil)
	target, err := device.Client.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	consent := RootConsent{Scope: vector.Scope, Target: target, Device: device.Client, Service: rooms, Lease: lease, Authorize: func(ctx context.Context) error { return ctx.Err() }, Approve: func(_ context.Context, request matrixproof.RootEnrollment) error {
		if request.Scope != vector.Scope || request.UserID != vector.UserID || request.Target != target {
			return matrixproof.ErrUnverified
		}
		return nil
	}}
	authority, err := consent.Ensure(ctx)
	if err != nil {
		t.Fatal("explicit fixture root consent against independent origin", err)
	}
	control, err := api.NewPrivateIngress(endpoint.String(), vector.Scope.Product, liveIngressSession{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceConfig := HTTPSourceConfig{Scope: vector.Scope, Epoch: vector.Epoch, Control: control, Authority: rooms, Signer: device.Client, Lease: lease}
	source, err := NewHTTPSource(sourceConfig)
	if err != nil {
		t.Fatal(err)
	}
	var plan contract.KeyPlan
	if len(initialize) > 1 {
		t.Fatal("only one explicit enrollment initializer may own this fixture")
	}
	if len(initialize) == 1 {
		plan, err = initialize[0](ctx, liveIngressKeySetup{device: device, source: source, configuration: sourceConfig, target: target, authority: authority})
	} else {
		plan, err = device.Client.CreateIngressKey(ctx, vector.Scope, vector.Epoch, 1)
		if err == nil {
			err = source.Enroll(ctx, plan, target, authority)
		}
	}
	if err != nil {
		t.Fatal("live SDK public recipient enrollment", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String()+"/fixture/seal", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer synthetic-live-device-session")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("ciphertext admission failed", response.StatusCode)
	}
	pending, err := source.Pending(ctx, "", 10)
	if err != nil || len(pending.Tasks) != 1 {
		t.Fatal("durable input not discoverable", err)
	}
	task := pending.Tasks[0]
	return liveIngressFixture{ctx, vector, device, lease, authority, sourceConfig, source, plan, task}
}

func TestIngressLiveDeviceQueueSDKRecoveryFixture(t *testing.T) {
	f := newLiveIngressFixture(t)
	ctx, vector, device, lease, sourceConfig, source, plan, task := f.ctx, f.vector, f.device, f.lease, f.sourceConfig, f.source, f.plan, f.task
	var err error
	root := t.TempDir()
	journal, err := offline.OpenWithSecrets(root, device.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	executor := liveSDKExecutor{device: device.Client, journal: journal, scope: vector.Scope}
	config := Config{Context: ctx, Scope: vector.Scope, Epoch: vector.Epoch, Journal: journal, Lease: lease, Source: source, Device: device.Client, Executor: executor}
	worker, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Process(ctx, task.ID); err == nil {
		t.Fatal("accepted ACK response loss was not exercised")
	}
	count := device.Sends.Load()
	if count != 2 {
		t.Fatal("expected one content and one index event", count)
	}
	var retained intent
	found, err := journal.Read(worker.owner(), storage.PrivateIngressNamespace, task.ID, &retained)
	if err != nil || !found || retained.Commit == nil || retained.Claim == nil || retained.Cipher == nil {
		t.Fatal("lost ACK did not retain encrypted worker intent", err)
	}
	cipher, err := journal.OpenStagedFile(ctx, worker.owner(), storage.PrivateIngressStagingNamespace, *retained.Cipher)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := io.ReadAll(io.NewSectionReader(cipher, 0, retained.Cipher.SizeBytes))
	_ = cipher.Close()
	if err != nil {
		t.Fatal(err)
	}
	worker.Close()
	source, err = NewHTTPSource(sourceConfig)
	if err != nil {
		t.Fatal(err)
	}
	config.Source = source
	worker, err = New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	if err := worker.Process(ctx, task.ID); err != nil {
		t.Fatal("restart did not reconcile accepted encrypted commit and ACK", err)
	}
	if device.Sends.Load() != count {
		t.Fatal("restart republished accepted delivery")
	}
	if pending, err := worker.Pending(ctx); err != nil || pending.Count != 0 {
		t.Fatal("accepted ACK did not finish WAL", err)
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		defer clear(raw)
		if bytes.Contains(raw, []byte("PRIVATE-LIVE")) || bytes.Contains(raw, []byte("AGE-SECRET-KEY")) {
			return errors.New("plaintext reached local ingress journal")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	recovery := device.Recovery
	_ = device.Client.Close()
	fresh := openLiveIngressDevice(ctx, t, vector, "LIVE_INGRESS_B", recovery)
	if recovered, err := fresh.Client.RecoverIngressKeyPlan(ctx, task); err != nil || recovered != plan {
		t.Fatal("fresh device did not recover the exact original public recipient", err)
	}
	for _, field := range []string{"owner", "epoch", "key"} {
		changed := task
		switch field {
		case "owner":
			changed.Scope.Subject = "22222222-2222-4222-8222-222222222222"
		case "epoch":
			changed.Epoch = "22222222-2222-4222-8222-222222222222"
		case "key":
			changed.KeyID = strings.Repeat("b", 64)
		}
		if _, err := fresh.Client.RecoverIngressKeyPlan(ctx, changed); err == nil {
			t.Fatal("recipient recovery accepted a substituted task binding", field)
		}
	}
	if fresh.KeyWrites.Load() != 0 {
		t.Fatal("recipient recovery created substitute SSSS material")
	}
	freshJournal, err := offline.OpenWithSecrets(t.TempDir(), fresh.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := freshJournal.RecordIDs(worker.owner(), storage.PrivateIngressNamespace)
	if err != nil || len(ids) != 0 {
		t.Fatal("fresh recovery inherited A's journal", err)
	}
	if err := fresh.Client.RestoreBackup(ctx); err != nil {
		t.Fatal(err)
	}
	catalog, err := storage.LoadCatalog(ctx, fresh.Client, vector.Scope)
	if err != nil || len(catalog.Heads) != 1 {
		t.Fatal("fresh recovered SDK did not see exactly one committed head", err)
	}
	head := catalog.Heads[0]
	record, err := fresh.Client.ReadStorageRecord(ctx, head.Commit.Content.Reference, head.ObjectID, "content")
	if err != nil {
		t.Fatal(err)
	}
	var original struct{ Metadata, Body []byte }
	if json.Unmarshal(record.Payload, &original) != nil || !bytes.Contains(original.Metadata, []byte("9007199254740993")) || !bytes.Contains(original.Body, []byte("PRIVATE-LIVE-MIME-MARKER")) {
		t.Fatal("fresh recovery lost original metadata or MIME")
	}
	metadata, body, err := fresh.Client.OpenIngressBody(ctx, task, bytes.NewReader(ciphertext))
	if err != nil {
		t.Fatal("fresh SSSS did not recover the age recipient", err)
	}
	recovered, err := io.ReadAll(body)
	if err != nil || !bytes.Equal(metadata, original.Metadata) || !bytes.Equal(recovered, original.Body) {
		t.Fatal("fresh age recipient did not recover exact input", err)
	}
	if fresh.Sends.Load() != 0 {
		t.Fatal("fresh recovery republished accepted data")
	}
	t.Log("INGRESS_LIVE_QUEUE_SDK_SSSS_PASS; A public login/consent, real pin/queue HTTP, encrypted WAL, lost ACK restart, verified fresh B with no inherited journal, one SDK head; pending-device handoff remains separate")
}
