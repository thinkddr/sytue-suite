//go:build integration && cgo

// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// The control-plane lifecycle witness and its delayed callback are synthetic.
// Signatures use the production Matrix verifier and actual SDK device chain.
// Ciphertext, SSSS, backup, encrypted SQLite, HTTP sends and reads are REAL.
// Separate Rooms/PG/origin tests cover the actual durable admission service.
type (
	nativeAdmissionRow struct {
		request  publication.Request
		proof    publication.SignedRequest
		receipt  publication.Receipt
		accepted *storage.Reference
	}
	nativeAdmissionFixture struct {
		mu           sync.Mutex
		device       matrixproof.Device
		authority    matrixproof.Authority
		target       storage.PublicationTarget
		rows         map[string]*nativeAdmissionRow
		sends        int
		invalid      int
		loseReserve  bool
		loseHTTP     bool
		dropBefore   atomic.Bool
		reserveCalls int
		swapRecovery *storage.Reference
	}
)

func (s *nativeAdmissionFixture) ReserveMatrixStoragePublication(ctx context.Context, p publication.SignedRequest) (publication.Receipt, error) {
	s.mu.Lock()
	s.reserveCalls++
	s.mu.Unlock()
	expected := p.Request
	expected.Scope = s.authority.Scope
	expected.Namespace = "storage:" + expected.Scope.Product
	expected.Target = s.target
	expected.AuthorityGeneration = s.authority.Generation
	if _, err := publication.VerifyRequest(ctx, p, expected, s.device); err != nil {
		return publication.Receipt{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.rows[p.TransactionID]
	if row == nil {
		token := make([]byte, 32)
		if _, err := rand.Read(token); err != nil {
			return publication.Receipt{}, err
		}
		marker := publication.Marker{Version: 1, ID: base64.RawURLEncoding.EncodeToString(token)}
		clear(token)
		hash, err := matrixproof.RequestDigest(p.Request)
		if err != nil {
			return publication.Receipt{}, err
		}
		row = &nativeAdmissionRow{request: p.Request, proof: p, receipt: publication.Receipt{Version: 1, Marker: marker, Status: "reserved", RequestSHA256: hash}}
		s.rows[p.TransactionID] = row
	} else if row.request != p.Request {
		return publication.Receipt{}, ErrIdentity
	}
	if s.loseReserve {
		s.loseReserve = false
		return publication.Receipt{}, errors.New("synthetic lost reserve ACK")
	}
	return row.receipt, nil
}

func (s *nativeAdmissionFixture) ReadMatrixStoragePublication(ctx context.Context, p publication.Request) (publication.Receipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.rows[p.TransactionID]
	if row == nil || row.request != p {
		return publication.Receipt{}, ErrIdentity
	}
	return row.receipt, ctx.Err()
}

func (s *nativeAdmissionFixture) ReadMatrixStoragePublicationRecovery(ctx context.Context, selection publication.RecoverySelection) (publication.RecoveryHint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.rows[selection.TransactionID]
	if row == nil || selection.Scope != s.authority.Scope {
		return publication.RecoveryHint{}, ErrIdentity
	}
	state, err := publication.RecoveryFromStored(row.proof, row.receipt)
	if err != nil {
		return publication.RecoveryHint{}, err
	}
	if s.swapRecovery != nil {
		copy := *s.swapRecovery
		state.Reference = &copy
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return publication.RecoveryHint{}, err
	}
	defer clear(raw)
	return publication.DecodeRecoveryHint(raw, selection)
}

func (s *nativeAdmissionFixture) acknowledge() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.rows {
		if row.accepted != nil {
			row.receipt.Status = "accepted"
			row.receipt.Reference = row.accepted
		}
	}
}

func TestNativeStorageStrictAdmissionPreservesFragmentWireAcrossLostACKAndRestart(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PUBLICATION_PREPARE") != "1" {
		t.Skip("isolated remote Synapse required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	target, err := url.Parse(os.Getenv("SYTUE_MATRIX_PUBLICATION_PREPARE_URL"))
	if err != nil || target.Host == "" {
		t.Fatal("isolated fixture URL required")
	}
	fixture := &nativeAdmissionFixture{rows: make(map[string]*nativeAdmissionRow), loseReserve: true, loseHTTP: true}
	reverse := httputil.NewSingleHostReverseProxy(target)
	reverse.ModifyResponse = func(response *http.Response) error {
		if !strings.Contains(response.Request.URL.Path, "/send/m.room.encrypted/") {
			return nil
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		_ = response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(body))
		if response.StatusCode != http.StatusOK {
			return nil
		}
		var ref struct {
			EventID string `json:"event_id"`
		}
		if json.Unmarshal(body, &ref) != nil || ref.EventID == "" {
			return ErrIdentity
		}
		txn, _ := url.PathUnescape(response.Request.URL.Path[strings.LastIndex(response.Request.URL.Path, "/")+1:])
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		row := fixture.rows[txn]
		if row == nil {
			return ErrIdentity
		}
		row.accepted = &storage.Reference{RoomID: row.request.Target.RoomID, EventID: ref.EventID}
		if fixture.loseHTTP {
			fixture.loseHTTP = false
			return errors.New("synthetic lost Matrix ACK")
		}
		row.receipt.Status = "accepted"
		row.receipt.Reference = row.accepted
		return nil
	}
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/send/m.room.encrypted/") {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "fixture", 400)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(raw))
			if fixture.dropBefore.CompareAndSwap(true, false) {
				http.Error(w, "synthetic pre-origin disconnect", http.StatusServiceUnavailable)
				return
			}
			hash, marker, err := publication.WireDigest(raw, true)
			txn := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			fixture.mu.Lock()
			row := fixture.rows[txn]
			valid := err == nil && row != nil && marker == row.receipt.Marker && hash == row.request.CipherSHA256 && row.receipt.Status == "reserved"
			if valid {
				row.receipt.Status = "sending"
				fixture.sends++
			} else {
				fixture.invalid++
			}
			fixture.mu.Unlock()
			if !valid {
				http.Error(w, "missing exact synthetic origin admission", http.StatusForbidden)
				return
			}
		}
		reverse.ServeHTTP(w, r)
	}))
	defer proxy.Close()
	scope := storage.Scope{Tenant: "native-strict-fixture", Subject: uuid.NewString(), Product: "notes"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	c := device.Client
	defer func() { _ = c.Close() }()
	c.stateMu.Lock()
	c.cfg.RequirePublicationAdmission = true
	cfg := c.cfg
	c.stateMu.Unlock()
	roots, err := c.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil || roots == nil {
		t.Fatal(err)
	}
	keys, err := c.cli.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: map[id.UserID]mautrix.DeviceIDList{c.session.UserID: {c.session.DeviceID}}})
	if err != nil {
		t.Fatal(err)
	}
	_, server, _ := c.session.UserID.Parse()
	fixture.device, err = matrixproof.Verify(ctx, matrixproof.Origin{Scope: scope, ServerName: server, UserID: c.session.UserID, DeviceID: c.session.DeviceID, AccountActive: true, DeviceActive: true, Keys: *keys}, roots.MasterKey)
	if err != nil {
		t.Fatal(err)
	}
	fixture.authority = matrixproof.Authority{Version: 1, Scope: scope, UserID: c.session.UserID.String(), RootKey: roots.MasterKey.String(), Generation: 1, Status: "active"}
	fixture.target, err = c.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(strings.Repeat("N", 28<<10))
	record := storage.Record{Version: 1, Scope: scope, ObjectID: "original_note", RevisionID: "revision1", Role: "content", Payload: payload}
	if err := record.Validate(scope); err != nil {
		t.Fatal("fixture original record invalid", err)
	}
	if _, err := c.PutStorageRecordAt(ctx, fixture.target, "no_admission_context", record); !errors.Is(err, ErrPublicationAdmissionRequired) {
		t.Fatal("strict unbound native storage sent", err)
	}
	var leases atomic.Int64
	var scopeLock sync.RWMutex
	lease := offline.ReentrantWorkLease(func(ctx context.Context) (func(), error) {
		for !scopeLock.TryRLock() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Millisecond):
			}
		}
		leases.Add(1)
		return func() { leases.Add(-1); scopeLock.RUnlock() }, nil
	})
	gate := offline.NewWorkGateWithLease(ctx, lease)
	defer gate.Close()
	var done func()
	defer func() {
		if done != nil {
			done()
		}
	}()
	start := func(bind func(context.Context) (context.Context, error)) context.Context {
		bound, release, err := gate.StartBound(ctx, bind)
		if err != nil {
			t.Fatal(err)
		}
		done = release
		return bound
	}

	var active atomic.Bool
	active.Store(true)
	policy := func(ctx context.Context) error {
		if !active.Load() {
			return ErrIdentity
		}
		return ctx.Err()
	}
	journalDir := t.TempDir()
	journal, err := offline.OpenWithSecrets(journalDir, cfg.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	bind, err := c.StoragePublicationBinder(scope, fixture.authority, lease, fixture, journal, policy)
	if err != nil {
		t.Fatal(err)
	}
	bound := start(bind)
	// A reserved operation whose HTTP never reached Origin must keep exactly
	// the same randomized hybrid AND SDK wire when the process restarts.
	reservedRecord := storage.Record{Version: 1, Scope: scope, ObjectID: "reserved_note", RevisionID: "revision1", Role: "content", Payload: []byte(`{"body":"private data before origin"}`)}
	fixture.mu.Lock()
	fixture.loseHTTP = false
	fixture.mu.Unlock()
	fixture.dropBefore.Store(true)
	writerRequested := make(chan struct{})
	writerDone := make(chan struct{})
	go func() { close(writerRequested); scopeLock.Lock(); scopeLock.Unlock(); close(writerDone) }()
	<-writerRequested
	for scopeLock.TryRLock() {
		scopeLock.RUnlock()
		select {
		case <-ctx.Done():
			t.Fatal("writer did not queue")
		case <-time.After(time.Millisecond):
		}
	}

	borrowCtx, borrowCancel := context.WithTimeout(bound, 15*time.Second)
	if _, err := c.PutStorageRecordAt(borrowCtx, fixture.target, "reserved_before_origin", reservedRecord); err == nil {
		borrowCancel()
		t.Fatal("pre-origin disconnect was acknowledged")
	}
	borrowCancel()
	if fixture.dropBefore.Load() {
		t.Fatal("signer reacquired the caller lease instead of reaching the reserved HTTP attempt")
	}
	select {
	case <-writerDone:
		t.Fatal("signer released original caller lease early")
	default:
	}
	done()
	done = nil
	select {
	case <-writerDone:
	case <-ctx.Done():
		t.Fatal("writer stayed blocked after original operation completed")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	journal, err = offline.OpenWithSecrets(journalDir, cfg.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	bind, err = c.StoragePublicationBinder(scope, fixture.authority, lease, fixture, journal, policy)
	if err != nil {
		t.Fatal(err)
	}
	bound = start(bind)
	if _, err := c.PutStorageRecordAt(bound, fixture.target, "reserved_before_origin", reservedRecord); err != nil {
		var pending retainedStoragePublication
		key := fixture.target.RoomID + "\x00" + fixture.target.DeviceID + "\x00reserved_before_origin"
		_, _ = journal.Read(scope.Tenant+"\x00"+scope.Subject+"\x00"+scope.Product, storage.NativePublicationNamespace, key, &pending)
		for _, frame := range pending.Frames {
			outbound, _ := c.crypto.Machine().CryptoStore.GetOutboundGroupSession(ctx, id.RoomID(fixture.target.RoomID))
			inbound, _ := c.crypto.Machine().CryptoStore.GetGroupSession(ctx, id.RoomID(fixture.target.RoomID), id.SessionID(frame.Binding.SessionID))
			t.Log("reserved restart flags", c.DeviceID() == fixture.target.DeviceID, outbound != nil, outbound != nil && outbound.ID().String() == frame.Binding.SessionID, inbound != nil, frame.Binding.MatrixUserID == c.session.UserID.String())
		}
		t.Fatal("reserved wire restart changed original ciphertext", err)
	}

	fixture.mu.Lock()
	baselineSends := fixture.sends
	fixture.loseHTTP = true
	fixture.mu.Unlock()
	txn := "strict_fragmented_record"
	if _, err := c.PutStorageRecordAt(bound, fixture.target, txn, record); err == nil {
		t.Fatal("lost Matrix ACK was falsely acknowledged")
	}
	fixture.mu.Lock()
	firstSends := fixture.sends
	fixture.mu.Unlock()
	if firstSends != baselineSends+1 {
		t.Fatal("fixture did not lose first fragment ACK", firstSends)
	}
	if _, err := c.PutStorageRecordAt(bound, fixture.target, txn, record); !errors.Is(err, ErrStoragePublicationPending) {
		t.Fatal("sending hold did not remain pending", err)
	}
	fixture.mu.Lock()
	if fixture.sends != firstSends {
		t.Fatal("ambiguous held native frame resent")
	}
	fixture.mu.Unlock()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	fixture.acknowledge()
	journal, err = offline.OpenWithSecrets(journalDir, cfg.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	bind, err = c.StoragePublicationBinder(scope, fixture.authority, lease, fixture, journal, policy)
	if err != nil {
		t.Fatal(err)
	}
	bound, err = bind(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := c.PutStorageRecordAt(bound, fixture.target, txn, record)
	if err != nil {
		t.Fatal("strict immutable native fragment restart", err)
	}
	accepted, err := c.ReadStorageRecord(bound, ref, record.ObjectID, record.Role)
	if err != nil || !bytes.Equal(accepted.Payload, record.Payload) {
		t.Fatal("accepted original record changed", err)
	}
	clear(accepted.Payload)
	fixture.mu.Lock()
	sends := fixture.sends
	rows := len(fixture.rows)
	invalid := fixture.invalid
	fixture.mu.Unlock()
	if sends != rows || rows < 3 || invalid != 0 {
		t.Fatal("native fragmented admission duplicated/omitted frame", sends, rows, invalid)
	}
	again, err := c.PutStorageRecordAt(bound, fixture.target, txn, record)
	if err != nil || again != ref {
		t.Fatal("accepted history failed exact record readback", err)
	}
	fixture.mu.Lock()
	if fixture.sends != sends {
		t.Fatal("accepted history republished")
	}
	fixture.mu.Unlock()
	owner := scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product
	pending, err := journal.RecordIDs(owner, storage.NativePublicationNamespace)
	if err != nil || len(pending) != 0 {
		t.Fatal("successful WAL stayed pending", err)
	}
	history, err := journal.RecordIDs(owner, storage.NativePublicationHistoryNamespace)
	if err != nil || len(history) != 2 {
		t.Fatal("accepted SDK history lost", err)
	}
	changed := record
	changed.Payload = []byte(`"changed private data"`)
	if _, err := c.PutStorageRecordAt(bound, fixture.target, txn, changed); !errors.Is(err, ErrIdentity) {
		t.Fatal("same txn replaced original semantic record", err)
	}
	// A different real SDK ciphertext can encode exactly the same record. A
	// forged recovery hint pointing there must still fail the ORIGINAL wire hash.
	equivalent, err := c.PutStorageRecordAt(bound, fixture.target, "equivalent_other_transaction", record)
	if err != nil {
		t.Fatal(err)
	}
	done()
	done = nil
	if err := c.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Matrix.Whoami(ctx); err == nil {
		t.Fatal("original producer was not actually retired")
	}
	loginClient, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	loginClient.Client = proxy.Client()
	login, err := loginClient.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: cfg.Bootstrap.MatrixUserID}, Password: device.Password, DeviceID: "RECOVERY_DEVICE_B"})
	if err != nil {
		t.Fatal(err)
	}
	cfgB := cfg
	cfgB.StoreDir = t.TempDir()
	cfgB.Secrets = &memorySecrets{values: map[string][]byte{}}
	c = storageSynapseSession(ctx, t, cfgB, credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: scope.Subject, Tenant: scope.Tenant})
	if c.DeviceID() == fixture.target.DeviceID {
		t.Fatal("recovery reused producer identity")
	}
	go func() { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := c.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Recover(ctx, device.Recovery); err != nil {
		t.Fatal("fresh SDK storage recovery", err)
	}
	journal, err = offline.OpenWithSecrets(t.TempDir(), cfgB.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	bind, err = c.StoragePublicationBinder(scope, fixture.authority, lease, fixture, journal, policy)
	if err != nil {
		t.Fatal(err)
	}
	bound = start(bind)
	fixture.mu.Lock()
	beforeRecoverySends := fixture.sends
	beforeRecoveryReserves := fixture.reserveCalls
	fixture.swapRecovery = &storage.Reference{RoomID: equivalent.RoomID, EventID: equivalent.EventID}
	fixture.mu.Unlock()
	if _, err := c.PutStorageRecordAt(bound, fixture.target, txn, record); !errors.Is(err, ErrIdentity) {
		t.Fatal("different equivalent ciphertext substituted original recovery ACK", err)
	}
	fixture.mu.Lock()
	fixture.swapRecovery = nil
	fixture.mu.Unlock()
	recovered, err := c.PutStorageRecordAt(bound, fixture.target, txn, record)
	if err != nil || recovered != ref {
		t.Fatal("fresh B failed ACK-only recovery after A retirement", err)
	}
	fixture.mu.Lock()
	if fixture.sends != beforeRecoverySends || fixture.reserveCalls != beforeRecoveryReserves {
		t.Fatal("B tried to reserve or publish as retired producer A")
	}
	fixture.mu.Unlock()
	done()
	done = nil
	active.Store(false)
	if _, release, err := gate.StartBound(ctx, bind); !errors.Is(err, ErrIdentity) {
		if release != nil {
			release()
		}
		t.Fatal("withdrawn original writer rebound", err)
	}
	if leases.Load() != 0 {
		t.Fatal("native strict signer leaked lease")
	}
	t.Log("actual SDK strict marker/signature; reserve-ACK recovery; Sending never resent; near-limit original-record fragments survive Matrix ACK loss and restart with same wire; accepted history Go readback and zero pending")
}
