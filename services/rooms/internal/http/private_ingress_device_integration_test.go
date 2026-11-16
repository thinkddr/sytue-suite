// SPDX-License-Identifier: Apache-2.0
//go:build integration

package chathttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/storagetest"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/mailtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	backendmatrix "git.sytue.eu/sytue/one/services/rooms/internal/matrix"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

type liveIngressAuthority struct {
	*store.Store
	origin *backendmatrix.Client
}

func (a liveIngressAuthority) ReadStorageAuthority(ctx context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	return a.MatrixStorageAuthority(ctx, scope.Subject, scope.Product)
}

func (a liveIngressAuthority) VerifyStorageAction(ctx context.Context, expected matrixproof.Action, proof matrixproof.SignedAction) (matrixproof.Authorization, error) {
	return a.VerifyMatrixStorageAction(ctx, expected.Scope.Subject, expected.Scope.Product, expected, proof)
}

func (a liveIngressAuthority) VerifyStorageReceipt(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, receipt storage.Reference) error {
	return a.VerifyMatrixStorageReceipt(ctx, scope.Subject, scope.Product, target, receipt)
}

func (a liveIngressAuthority) FenceStorageDevice(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	if _, err := a.PlanMatrixStorageDeviceFence(ctx, proof); err != nil {
		return ingress.DeviceFence{}, err
	}
	return a.ConfirmMatrixStorageDeviceFence(ctx, proof, func(ctx context.Context, scope storage.Scope, previous ingress.Assignment, revoke bool) error {
		if revoke {
			if err := a.origin.RevokeStorageDevice(ctx, scope, previous.Target, previous.SigningKey); err != nil {
				return err
			}
		}
		return a.origin.RequireStorageDeviceAbsent(ctx, scope, previous.Target)
	})
}

func (a liveIngressAuthority) ReadStorageDeviceFence(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	return a.ReadMatrixStorageDeviceFence(ctx, proof, a.origin.RequireStorageDeviceAbsent)
}

func (a liveIngressAuthority) VerifyMatrixStorageReceipt(ctx context.Context, actor, product string, target storage.PublicationTarget, receipt storage.Reference) error {
	return a.Store.VerifyMatrixStorageReceipt(ctx, actor, product, target, receipt, func(ctx context.Context, scope storage.Scope, ref storage.Reference) error {
		return a.origin.ValidateStorageEvent(ctx, domain.MatrixEventRef{RoomID: ref.RoomID, EventID: ref.EventID}, scope)
	})
}

type lostLiveIngressACK struct {
	pg.PrivateIngressQueue
	lost          atomic.Bool
	lose          bool
	loseRejection bool
	lostRejection atomic.Bool
}

func (q *lostLiveIngressACK) Apply(ctx context.Context, scope storage.Scope, request ingress.ControlRequest, proof matrixproof.SignedAction) (ingress.ControlResult, error) {
	result, err := q.PrivateIngressQueue.Apply(ctx, scope, request, proof)
	if err == nil && q.loseRejection && result.Handoff != nil && result.Handoff.Status == "not_started" && q.lostRejection.CompareAndSwap(false, true) {
		return ingress.ControlResult{}, io.ErrUnexpectedEOF
	}
	if err == nil && q.lose && request.Kind == "task.ack" && q.lost.CompareAndSwap(false, true) {
		return ingress.ControlResult{}, io.ErrUnexpectedEOF
	}
	return result, err
}

func liveIngressMigrations(t *testing.T) fs.FS {
	t.Helper()
	files := fstest.MapFS{}
	if err := fs.WalkDir(db.Migrations, db.MigrationsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(db.Migrations, path)
		if err != nil {
			return err
		}
		files[path] = &fstest.MapFile{Data: raw}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	files["migrations/9999_private_ingress_fixture.up.sql"] = &fstest.MapFile{Data: []byte(`-- sytue:private-ingress-v1
-- sytue:private-ingress-bodies-v1
-- sytue:private-ingress-handoff-v1
-- sytue:private-ingress-rejections-v1
-- sytue:private-ingress-epoch-v1
-- sytue:private-ingress-pq-v1
SELECT sytue_scope_merge_register('private_ingress_key_plans'::regclass);
SELECT sytue_scope_merge_register('private_ingress_devices'::regclass);
SELECT sytue_scope_merge_register('private_ingress_tasks'::regclass);
SELECT sytue_scope_merge_register('private_ingress_actions'::regclass);
SELECT sytue_scope_admission_register('private_ingress_key_plans'::regclass);
SELECT sytue_scope_admission_register('private_ingress_devices'::regclass);
SELECT sytue_scope_admission_register('private_ingress_tasks'::regclass);
SELECT sytue_scope_admission_register('private_ingress_actions'::regclass);
SELECT sytue_scope_admission_register('private_ingress_bodies'::regclass);
`)}
	return pg.WithPrivateIngressDDL(files)
}

func TestPrivateIngressLiveAuthorityQueueWorkerSDKRecovery(t *testing.T) {
	runLivePrivateIngressDeviceProof(t, "")
}

func TestPrivateIngressLiveEnrollmentRuntime(t *testing.T) {
	runLivePrivateIngressDeviceProof(t, "enrollment")
}

func TestPrivateIngressLiveNativeMailApplication(t *testing.T) {
	runLivePrivateIngressDeviceProof(t, "native-mail")
}

func TestPrivateIngressLiveNativeMailPostgresSource(t *testing.T) {
	runLivePrivateIngressDeviceProof(t, "native-mail-sql")
}

func TestPrivateIngressLiveNativeMailSharedOutbound(t *testing.T) {
	t.Setenv("SYTUE_MAIL_SHARED_OUTBOUND_PROOF", "1")
	runLivePrivateIngressDeviceProof(t, "native-mail-sql")
}

func TestPrivateIngressLiveNativeMailBrowserFullSource(t *testing.T) {
	t.Setenv("SYTUE_MAIL_SHARED_OUTBOUND_PROOF", "1")
	runLivePrivateIngressDeviceProof(t, "native-mail-sql")
}

func TestPrivateIngressLiveDeviceHandoff(t *testing.T) {
	for _, mode := range []string{"absent", "committed", "renewal"} {
		t.Run(mode, func(t *testing.T) { runLivePrivateIngressDeviceProof(t, mode) })
	}
}

func runLivePrivateIngressDeviceProof(t *testing.T, mode string) {
	t.Helper()
	if os.Getenv("SYTUE_INGRESS_DEVICE_PROOF") != "1" {
		t.Skip("requires disposable Synapse and PostgreSQL")
	}
	endpoint, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_INGRESS_DEVICE_SYNAPSE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	browserSource := t.Name() == "TestPrivateIngressLiveNativeMailBrowserFullSource"
	webComposite := os.Getenv("SYTUE_MAIL_WEB_PROOF") == "1"
	if browserSource && !webComposite {
		t.Fatal("BrowserFull requires its exact real web fixture")
	}
	if webComposite && ((t.Name() != "TestPrivateIngressLiveNativeMailSharedOutbound" && !browserSource) || mode != "native-mail-sql" || os.Getenv("SYTUE_MAIL_SHARED_OUTBOUND_PROOF") != "1") {
		t.Fatal("web composite requires its exact native Mail parent")
	}
	if debug := os.Getenv("SYTUE_MAIL_WEB_DEBUG"); debug != "" && (debug != "1" || !webComposite) {
		t.Fatal("browser debug hold requires the exact web composite")
	}
	if strict := os.Getenv("SYTUE_MAIL_NATIVE_STRICT_PROOF"); strict != "" && (strict != "1" || !webComposite) {
		t.Fatal("native strict proof requires the exact web composite")
	}
	proofLimit := 8 * time.Minute
	if os.Getenv("SYTUE_MAIL_SHARED_OUTBOUND_PROOF") == "1" {
		proofLimit = 12 * time.Minute
	}
	if webComposite {
		proofLimit = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(t.Context(), proofLimit)
	defer cancel()
	h := pgtest.New(t, liveIngressMigrations(t), db.MigrationsDir)
	st := store.New(h.Pool, nil)
	admin, err := mautrix.NewClient(endpoint.String(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	login, err := admin.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: "rooms_controller"}, Password: "artificial-identity-admin-password"})
	if err != nil {
		t.Fatal("disposable controller login", err)
	}
	admin.UserID, admin.DeviceID, admin.AccessToken = login.UserID, login.DeviceID, login.AccessToken
	_, matrixServerName, ok := strings.Cut(admin.UserID.String(), ":")
	if !ok {
		t.Fatal("disposable Matrix controller has no server name")
	}
	publicationSecret := ""
	if os.Getenv("SYTUE_MAIL_WEB_PROOF") == "1" {
		publicationSecret = mailBrowserPublicationSecret
	}
	origin := backendmatrix.New(backendmatrix.Config{PublicationSecret: publicationSecret, InternalURL: endpoint.String(), HomeserverURL: "https://" + matrixServerName, ServerName: matrixServerName, SSOProviderID: "oidc-sytue", AdminToken: login.AccessToken})
	origin.SetIdentityGate(st)
	st.SetMatrixGate(origin)
	if err := origin.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	tenant := "ingress-proof"
	if mode != "" {
		tenant = "ingress-handoff-" + mode
	}
	scope := storage.Scope{Tenant: tenant, Subject: "11111111-1111-4111-8111-111111111111", Product: "mail"}
	nativeMailMode := mode == "native-mail" || mode == "native-mail-sql"
	var snapshot *storagetest.MailSnapshot
	if mode == "native-mail-sql" {
		raw, err := os.ReadFile(os.Getenv("SYTUE_MAIL_PRIVATE_VECTOR"))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := storagetest.DecodeMailSnapshot(raw)
		clear(raw)
		if err != nil {
			t.Fatal(err)
		}
		snapshot = &decoded
		scope = decoded.State.Scope
	}
	epoch := uuid.New()
	if nativeMailMode {
		candidate, err := storage.InitialSourceEpoch(scope)
		if err != nil {
			t.Fatal(err)
		}
		epoch = uuid.MustParse(candidate)
		if snapshot != nil {
			epoch = uuid.MustParse(snapshot.State.Epoch)
		}
	}
	tenantCtx := tenancy.Into(ctx, tenancy.Tenant{Slug: scope.Tenant})
	rooms, err := st.EnsureMatrixStorageRoomsForIdentity(tenantCtx, scope.Subject, scope.Product, storage.IsolatedIdentity, func(ctx context.Context, room string) (string, error) {
		return origin.EnsureIsolatedStorageRoom(ctx, scope, room)
	})
	if err != nil || len(rooms) != 1 {
		t.Fatal("disposable isolated room", err)
	}
	user, err := origin.StorageUserID(scope)
	if err != nil {
		t.Fatal(err)
	}
	_, err = admin.MakeRequest(ctx, http.MethodPut, endpoint.String()+"/_synapse/admin/v2/users/"+url.PathEscape(user), map[string]any{"password": "artificial-disposable-ingress-password", "logout_devices": false}, nil)
	if err != nil {
		t.Fatal("disposable user setup", err)
	}
	var nativeMail *nativeMailSQLFixture
	if nativeMailMode {
		nativeMail, err = prepareNativeMailSQLFixture(tenantCtx, t, st, origin, admin, endpoint.String(), scope, snapshot)
		if err != nil {
			t.Fatal("native Mail retained SQL fixture", err)
		}
	}
	if os.Getenv("SYTUE_MAIL_SHARED_OUTBOUND_PROOF") == "1" {
		if nativeMail == nil || snapshot == nil {
			t.Fatal("shared outbound proof requires the retained SQL native fixture")
		}
		nativeMail.outbound = mailtest.NewRelay(t, *snapshot)
	}
	authority := liveIngressAuthority{Store: st, origin: origin}
	cfg := pg.PrivateIngressConfig{Scope: scope, Epoch: epoch, VerifyReceipt: func(ctx context.Context, claim ingress.Claim, receipt ingress.Receipt) error {
		return authority.VerifyMatrixStorageReceipt(ctx, scope.Subject, scope.Product, claim.Target, receipt.Commit)
	}, Sealed: func(ctx context.Context, tx pgx.Tx, id string) error {
		event, err := events.NewEvent("private_ingress", "private_ingress.sealed", scope.Tenant, map[string]string{"task_id": id})
		if err != nil {
			return err
		}
		return events.Publish(ctx, tx, event)
	}}
	inTx := func(ctx context.Context, operation func(pgx.Tx) error) error {
		return h.Pool.InTenantTx(ctx, operation)
	}
	queue := &lostLiveIngressACK{lose: mode == "" || nativeMailMode, loseRejection: mode == "renewal", PrivateIngressQueue: pg.PrivateIngressQueue{Transaction: func(ctx context.Context, selected storage.Scope, selectedEpoch, _ string, operation func(pgx.Tx, pg.AuthorizedPrivateIngressConfig) error) error {
		if selected != scope || selectedEpoch != epoch.String() {
			return ingress.ErrContract
		}
		return inTx(ctx, func(tx pgx.Tx) error {
			return operation(tx, pg.AuthorizedPrivateIngressConfig{PrivateIngressConfig: cfg, Authority: authority})
		})
	}, Body: func(ctx context.Context, selected storage.Scope, blob string) (io.ReadCloser, error) {
		if selected != scope {
			return nil, ingress.ErrContract
		}
		var body io.ReadCloser
		err := inTx(ctx, func(tx pgx.Tx) error {
			var err error
			body, err = pg.ReadPrivateIngressInlineBody(ctx, tx, cfg, blob)
			return err
		})
		return body, err
	}}}
	rootHandlers := storageAuthorityHandlers{svc: authority}
	ingressHandlers := httpx.NewPrivateIngressHandlers(httpx.PrivateIngressHTTPConfig{Product: scope.Product, Port: queue, MaxBodyBytes: 2 << 20, Identity: authn.PrivateStorageIdentity, MapError: func(err error) *httpx.Error {
		if shared := httpx.PrivateIngressError(err); shared != nil {
			return shared
		}
		return httpx.ErrInternal
	}})
	var browserOrigin *mailBrowserOrigin
	if os.Getenv("SYTUE_MAIL_WEB_PROOF") == "1" {
		if nativeMail == nil {
			t.Fatal("Mail browser origin requires native source")
		}
		browserOrigin = newMailBrowserOrigin(nativeMail, st)
	}
	router := chi.NewRouter()
	if browserOrigin != nil {
		browserOrigin.mount(router)
	}
	router.Route("/api/v1", func(api chi.Router) {
		api.Get("/matrix/storage/{product}/authority", rootHandlers.get)
		api.Post("/matrix/storage/{product}/authority/challenge", rootHandlers.challenge)
		api.Post("/matrix/storage/{product}/authority/confirm", rootHandlers.confirm)
		if browserOrigin != nil {
			browserOrigin.mountPublications(api)
		}
		ingressHandlers.Mount(api)
		if nativeMail != nil {
			nativeMail.mount(api)
		}
	})
	if nativeMail != nil {
		router.Post("/fixture/retire-source", func(w http.ResponseWriter, r *http.Request) {
			state, err := st.MatrixStorageCutover(r.Context(), scope.Subject, scope.Product)
			if err != nil || state.Phase != "isolated" {
				httpx.Fail(w, r, httpx.ErrConflict)
				return
			}
			nativeMail.mu.Lock()
			nativeMail.retired = true
			nativeMail.mu.Unlock()
			httpx.NoContent(w)
		})
	}
	router.Post("/fixture/seal", func(w http.ResponseWriter, r *http.Request) {
		if err := inTx(r.Context(), func(tx pgx.Tx) error {
			plan, err := pg.ReadPrivateIngressKey(r.Context(), tx, cfg, 1)
			if err != nil {
				return err
			}
			return pg.PlanPrivateIngress(r.Context(), tx, cfg, plan, "live_task_01", "mail.receive")
		}); err != nil {
			httpx.Fail(w, r, httpx.ErrInternal)
			return
		}
		raw := []byte("From: synthetic@example.invalid\r\nSubject: PRIVATE-LIVE-MIME-MARKER\r\n\r\nOriginal MIME / 漢字 é\r\n")
		defer clear(raw)
		metadata := []byte(`{"future":9007199254740993,"subject":"PRIVATE-LIVE-METADATA-MARKER"}`)
		if nativeMail != nil {
			metadata, _ = json.Marshal(map[string]any{"id": "native_received", "mailbox_id": nativeMail.inbox, "thread_id": "native_received", "size": len(raw), "subject": "PRIVATE-LIVE-METADATA-MARKER", "received_at": "2026-10-30T00:00:00Z"})
		}
		err := inTx(r.Context(), func(tx pgx.Tx) error {
			plan, err := pg.ReadPrivateIngressKey(r.Context(), tx, cfg, 1)
			if err != nil {
				return err
			}
			_, _, err = pg.SealPrivateIngressInline(r.Context(), tx, cfg, plan, "live_task_01", "mail.receive", metadata, bytes.NewReader(raw), int64(len(raw)), 1<<20)
			return err
		})
		if err != nil {
			httpx.Fail(w, r, httpx.ErrInternal)
			return
		}
		httpx.NoContent(w)
	})
	router.Post("/fixture/expire", func(w http.ResponseWriter, r *http.Request) {
		err := inTx(r.Context(), func(tx pgx.Tx) error {
			if err := pg.BindPrivateIngressScope(r.Context(), tx, cfg); err != nil {
				return err
			}
			query := `UPDATE private_ingress_tasks SET lease_until=now()-interval '1 minute' WHERE id='live_task_01' AND device_id IS NOT NULL AND acknowledged_at IS NULL`
			if r.URL.Query().Get("renew") == "1" {
				query = `UPDATE private_ingress_tasks SET lease_until=now()-interval '30 seconds' WHERE id='live_task_01' AND device_id IS NOT NULL AND acknowledged_at IS NULL`
			}
			result, err := tx.Exec(r.Context(), query)
			if err != nil {
				return err
			}
			if result.RowsAffected() != 1 {
				return ingress.ErrClaim
			}
			return nil
		})
		if err != nil {
			httpx.Fail(w, r, httpx.ErrConflict)
			return
		}
		httpx.NoContent(w)
	})
	if nativeMail != nil && nativeMail.outbound != nil {
		router.Get("/fixture/outbound-proof", nativeMail.outbound.Proof)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if browserOrigin != nil && strings.HasPrefix(r.URL.Path, "/internal/matrix/publications/") {
			router.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-live-device-session" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		requestCtx := authn.Into(tenancy.Into(r.Context(), tenancy.Tenant{Slug: scope.Tenant}), authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject, Email: "synthetic@example.invalid"})
		router.ServeHTTP(w, r.WithContext(requestCtx))
	}))
	if browserOrigin != nil {
		if err = server.Listener.Close(); err != nil {
			t.Fatal(err)
		}
		server.Listener, err = net.Listen("tcp", "127.0.0.1:18747")
		if err != nil {
			t.Fatal(err)
		}
	}
	server.Start()
	defer server.Close()
	routingVector := matrixprooftest.IngressDeviceVector{Scope: scope, Epoch: epoch.String(), UserID: user, RoomID: rooms[0], SynapseURL: endpoint.String()}
	if nativeMail != nil {
		routingVector = nativeMail.extend(routingVector)
	}
	vector, err := json.Marshal(routingVector)
	if err != nil {
		t.Fatal(err)
	}
	nativeTest, marker := "TestIngressLiveDeviceQueueSDKRecoveryFixture", "INGRESS_LIVE_QUEUE_SDK_SSSS_PASS"
	if mode == "enrollment" {
		nativeTest, marker = "TestIngressLiveEnrollmentRuntimeFixture", "INGRESS_LIVE_ENROLLMENT_RUNTIME_WAL_PASS"
	} else if mode != "" {
		nativeTest, marker = "TestIngressLiveDeviceHandoffFixture", "INGRESS_LIVE_DEVICE_HANDOFF_PASS"
	}
	module, packageName := "../../../../apps/nucleo", "./ingress"
	if nativeMail != nil {
		module, packageName, nativeTest, marker = "../../../../apps/fyne", "./cifrado", "TestNativeMailApplicationAgainstAuthorityQueueAndSynapse", "NATIVE_MAIL_AUTHORITY_QUEUE_SDK_MIME_PASS"
	}
	if browserSource {
		nativeTest = "TestNativeMailBrowserFullSourceAgainstAuthorityAndSynapse"
		marker = "NATIVE_MAIL_BROWSER_FULL_SOURCE_MIME_RELOAD_RETAINED_ACK_PASS"
	}
	fixtureEnv := map[string]string{"SYTUE_INGRESS_DEVICE_PROOF": "1", "SYTUE_INGRESS_HANDOFF_CASE": mode, "SYTUE_INGRESS_DEVICE_API_URL": server.URL, "SYTUE_INGRESS_DEVICE_VECTOR": string(vector)}
	if snapshot != nil {
		fixtureEnv["SYTUE_MAIL_PRIVATE_VECTOR"] = os.Getenv("SYTUE_MAIL_PRIVATE_VECTOR")
	}
	if os.Getenv("SYTUE_MAIL_WEB_PROOF") == "1" {
		fixtureEnv["SYTUE_MAIL_WEB_PROOF"] = "1"
		fixtureEnv["SYTUE_MAIL_WEB_PROOF_PARENT"] = t.Name()
		if os.Getenv("SYTUE_MAIL_NATIVE_STRICT_PROOF") == "1" {
			fixtureEnv["SYTUE_MAIL_NATIVE_STRICT_PROOF"] = "1"
		}
		if os.Getenv("SYTUE_MAIL_WEB_DEBUG") == "1" {
			fixtureEnv["SYTUE_MAIL_WEB_DEBUG"] = "1"
		}
		fixtureEnv["SYTUE_MATRIX_BROWSER_EXECUTABLE"] = os.Getenv("SYTUE_MATRIX_BROWSER_EXECUTABLE")
	}
	fixtureTimeout := 6 * time.Minute
	if nativeMail != nil && nativeMail.outbound != nil {
		fixtureEnv["SYTUE_MAIL_SHARED_OUTBOUND_PROOF"] = "1"
		fixtureTimeout = 10 * time.Minute
	}
	if webComposite {
		fixtureTimeout = 14 * time.Minute
	}
	pgtest.RunNativeFixture(ctx, t, pgtest.NativeFixture{Module: module, Package: packageName, Test: nativeTest, Marker: marker, Timeout: fixtureTimeout, Env: fixtureEnv})
	if !browserSource && nativeMail != nil && nativeMail.outbound != nil && nativeMail.outbound.SMTP.Accepted() != 1 {
		t.Fatal("fresh SDK relay duplicated SMTP or sent the canceled job", nativeMail.outbound.SMTP.Accepted())
	}
	if !browserSource && (mode == "" || nativeMail != nil) && !queue.lost.Load() {
		t.Fatal("accepted ACK response was never lost")
	}
	if mode == "renewal" && !queue.lostRejection.Load() {
		t.Fatal("durable rejection response was never lost")
	}
	if err := runLiveMailPostAudit(t.Name(), func() error { return browserOrigin.auditBrowserPublications(tenantCtx, scope) }, func() error {
		return inTx(tenantCtx, func(tx pgx.Tx) error {
			delivery, err := pg.ReadPrivateIngressDelivery(tenantCtx, tx, cfg, "live_task_01")
			if err != nil {
				return err
			}
			if delivery.Receipt == nil {
				return errors.New("live SDK receipt did not commit")
			}
			if mode != "" && mode != "enrollment" && nativeMail == nil {
				expectedGeneration := int64(1)
				if mode != "committed" {
					expectedGeneration = 2
				}
				if delivery.ClaimGeneration != expectedGeneration || delivery.Receipt.ClaimGeneration != expectedGeneration {
					return errors.New("handoff changed the committed claim generation")
				}
				var history, fences int
				if err := tx.QueryRow(tenantCtx, `SELECT count(*) FROM private_ingress_claim_history WHERE task_id='live_task_01' AND claim_generation=1`).Scan(&history); err != nil {
					return err
				}
				if _, err := tx.Exec(tenantCtx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, scope.Subject, scope.Product); err != nil {
					return err
				}
				if err := tx.QueryRow(tenantCtx, `SELECT count(*) FROM matrix_storage_device_fences WHERE tenant=$1 AND subject=$2 AND product=$3 AND epoch=$4 AND status='confirmed'`, scope.Tenant, scope.Subject, scope.Product, epoch).Scan(&fences); err != nil {
					return err
				}
				if history != 1 || fences != 1 {
					return errors.New("handoff lost original claim or authoritative fence evidence")
				}
				if mode == "renewal" {
					var rejected int
					if err := tx.QueryRow(tenantCtx, `SELECT count(*) FROM private_ingress_handoff_rejections WHERE task_id='live_task_01'`).Scan(&rejected); err != nil {
						return err
					}
					if rejected != 1 {
						return errors.New("renewal lost its permanently rejected begin")
					}
				}
			}
			var rows string
			if err := tx.QueryRow(ctx, `SELECT json_agg(row_to_json(t))::text FROM private_ingress_tasks t`).Scan(&rows); err != nil {
				return err
			}
			if strings.Contains(rows, "PRIVATE-LIVE") || strings.Contains(rows, "AGE-SECRET-KEY") {
				return errors.New("plaintext reached durable queue metadata")
			}
			var cipher []byte
			var notifications string
			if err := tx.QueryRow(tenantCtx, `SELECT ciphertext FROM private_ingress_bodies WHERE task_id='live_task_01'`).Scan(&cipher); err != nil {
				return err
			}
			if !bytes.HasPrefix(cipher, []byte("age-encryption.org/v1")) || bytes.Contains(cipher, []byte("PRIVATE-LIVE")) || bytes.Contains(cipher, []byte("AGE-SECRET-KEY")) {
				return errors.New("plaintext reached durable body")
			}
			if err := tx.QueryRow(tenantCtx, `SELECT json_agg(payload)::text FROM events_outbox WHERE type='private_ingress.sealed'`).Scan(&notifications); err != nil {
				return err
			}
			if notifications != `[{"task_id": "live_task_01"}]` {
				return errors.New("ingress notification exposed anything beyond its opaque task ID")
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}

	switch mode {
	case "native-mail", "native-mail-sql":
		t.Log("INGRESS_LIVE_NATIVE_MAIL_AUTHORITY_PG_HTTP_FACTORY_SDK_MIME_PASS; retained SQL source is a fixture; production readiness remains disabled")
	case "enrollment":
		t.Log("INGRESS_LIVE_ENROLLMENT_RUNTIME_PG_HTTP_SDK_PASS; readiness remains disabled pending production domain execution")
	case "":
		t.Log("INGRESS_LIVE_AUTHORITY_QUEUE_WORKER_SDK_RECOVERY_PASS; readiness remains disabled pending production domain execution")
	default:
		t.Log("INGRESS_LIVE_REAL_FENCE_EMPTY_B_SDK_HANDOFF_PASS", mode, "; readiness remains disabled pending production domain execution")
	}
}
