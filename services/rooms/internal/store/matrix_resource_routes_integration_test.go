//go:build integration

// SPDX-License-Identifier: Apache-2.0
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

// Only the ORIGINAL product metadata witness and directory liveness are seams.
// The separate Docs proof tests the real frozen source. Product consent,
// recipient challenge, SDK chains/signatures, current Provider, SQL and RLS here
// are production code. This is not a composed HTTP/Synapse-origin activation.
type roomRouteOriginalSource struct {
	snapshot resource.BootstrapSourceSnapshot
	active   bool
}

func (s *roomRouteOriginalSource) BootstrapSource(ctx context.Context, key resource.Resource) (resource.BootstrapSourceSnapshot, error) {
	if ctx.Err() != nil {
		return resource.BootstrapSourceSnapshot{}, ctx.Err()
	}
	if !s.active || s.snapshot.Resource != key {
		return resource.BootstrapSourceSnapshot{}, resource.ErrRevoked
	}
	return s.snapshot, nil
}

func (s *roomRouteOriginalSource) CheckBootstrapSource(ctx context.Context, expected resource.BootstrapSourceSnapshot) error {
	current, err := s.BootstrapSource(ctx, expected.Resource)
	if err != nil {
		return err
	}
	if current != expected {
		return resource.ErrRevoked
	}
	return nil
}

func TestResourceBootstrapRoomRegistrationUsesActualProviderAndImmutableSource(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool, testCipher(t))
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "docs"}
	ctx := tenantCtx(scope.Tenant)
	a := matrixprooftest.NewUser(t, scope, "matrix.test", id.UserID("@sytue_"+scope.Subject+":matrix.test"), "DOCS_ROUTE_A")
	directory := &resourceRegistryDirectory{devices: map[string]*matrixprooftest.Fixture{a.Origin.DeviceID.String(): a}}
	st.SetMatrixGate(directory)
	leaf, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	recipient := registryRecipient(t, a, leaf)
	consent, err := st.CreateMatrixResourceWebConsent(ctx, scope.Subject, scope.Product, "sytue-docus")
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := st.ResourceSessionChallenge(ctx, scope, consent.Grant, "sytue-docus", recipient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.ConfirmResourceSession(ctx, scope, "sytue-docus", registryEnrollment(t, a, leaf, challenge)); err != nil {
		t.Fatal(err)
	}
	provider, err := st.ResourceSessionProvider()
	if err != nil {
		t.Fatal(err)
	}
	current, err := provider.Resolve(ctx, scope, a.Origin.DeviceID.String())
	if err != nil {
		t.Fatal(err)
	}
	sign := func(value any) signatures.Signatures {
		return matrixprooftest.Sign(t, a.Device, value, a.Origin.UserID, a.Origin.DeviceID.String())
	}
	encrypted, err := json.Marshal(&event.EncryptedEventContent{Algorithm: id.AlgorithmMegolmV1, SessionID: id.SessionID(a.Master.PublicKey()), SenderKey: id.Curve25519(a.Self.PublicKey()), DeviceID: a.Origin.DeviceID, MegolmCiphertext: []byte("opaque fixture only")})
	if err != nil {
		t.Fatal(err)
	}
	f := resourcetest.NewWithSDKRecipient(t, false, a.Origin, a.Master.PublicKey(), sign, "!registered-resource:matrix.test", encrypted, leaf)
	f.Chain = a
	key := resource.Resource{Tenant: scope.Tenant, Product: scope.Product, Kind: "document", ID: uuid.NewString()}
	source := &roomRouteOriginalSource{active: true, snapshot: resource.BootstrapSourceSnapshot{Resource: key, OwnerScope: scope, SourceEpoch: uuid.NewString(), SourceSeq: "9007199254740993", ACLVersion: "9007199254740994", History: resource.HistoryFull}}
	bootstrap := resourcetest.NewBootstrap(t, ctx, f, current, source, key)
	plan := bootstrap.Verified
	if err = st.RegisterMatrixResourceBootstrapRoom(ctx, resource.VerifiedBootstrapPlan{}); err == nil {
		t.Fatal("zero plan registered a room")
	}
	if err = st.RegisterMatrixResourceBootstrapRoom(ctx, plan); err != nil {
		t.Fatal("verified route", err)
	}
	if err = st.RegisterMatrixResourceBootstrapRoom(ctx, plan); err != nil {
		t.Fatal("lost registration ACK", err)
	}
	route, managed, err := st.MatrixPublicationRoomRoute(ctx, plan.Value().RoomID)
	if err != nil || !managed || route.Resource != key || route.Scope.Subject != "" || route.Scope.Tenant != key.Tenant || route.ServiceID != "docs" || route.PlanSHA256 != plan.SHA256() {
		t.Fatal("resource misrouted as producer/owner storage", err, route)
	}
	if _, _, err = st.MatrixPublicationRoomScope(ctx, plan.Value().RoomID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("resource route became personal storage authority", err)
	}
	for _, lookup := range []string{"", "!unregistered:matrix.test", plan.Value().RoomID} {
		var count int
		if err = h.Pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_resource_route_room',$1,true)`, lookup); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `SELECT count(*) FROM matrix_resource_routes`).Scan(&count)
		}); err != nil {
			t.Fatal(err)
		}
		expected := 0
		if lookup == plan.Value().RoomID {
			expected = 1
		}
		if count != expected {
			t.Fatal("maintenance lookup widened beyond exact room", lookup, count)
		}
	}
	other := resourcetest.NewBootstrap(t, ctx, f, current, source, key)
	if err = st.RegisterMatrixResourceBootstrapRoom(ctx, other.Verified); err == nil {
		t.Fatal("another plan silently replaced original route")
	}
	// Plain SQL replicas cannot register even valid-looking source metadata. The
	// narrow new contract is required, and its independent fields are checked.
	rawSQL := func(ctx context.Context, q string, args ...any) error {
		return h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.resource_session_subject',$1,true),set_config('app.resource_session_product','docs',true)`, scope.Subject); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, q, args...)
			return err
		})
	}
	if err = rawSQL(ctx, `INSERT INTO matrix_resource_routes(room_id,tenant,product,kind,resource_id,service_id,registration,registration_sha256)VALUES('!old-binary:matrix.test',$1,'docs','document',$2,'docs',$3,$4)`, scope.Tenant, uuid.NewString(), plan.PublicPlanBytes(), route.RegistrationSHA256); err == nil {
		t.Fatal("old binary bypassed verified route contract")
	}
	// Both the existing general guard and the new scope/plan guard preserve the
	// original bytes and prevent a client from changing a service into a URL.
	for _, query := range []string{
		`UPDATE matrix_resource_routes SET service_id='https://attacker.invalid' WHERE tenant=$1 AND product='docs' AND room_id=$2`,
		`UPDATE matrix_resource_routes SET actor_subject='other' WHERE tenant=$1 AND product='docs' AND room_id=$2`,
		`DELETE FROM matrix_resource_routes WHERE tenant=$1 AND product='docs' AND room_id=$2`,
	} {
		if err = rawSQL(ctx, query, scope.Tenant, plan.Value().RoomID); err == nil {
			t.Fatal("registered source route mutated")
		}
	}
	var foreign int
	if err = h.Pool.InTenantTx(tenantCtx("foreign"), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.resource_session_product','docs',true)`); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM matrix_resource_routes WHERE tenant=$1 AND product='docs'`, scope.Tenant).Scan(&foreign)
	}); err != nil || foreign != 0 {
		t.Fatal("resource route crossed tenant RLS", err)
	}
	source.active = false
	if err = st.RegisterMatrixResourceBootstrapRoom(ctx, plan); err == nil {
		t.Fatal("withdrawn original source registered")
	}
	source.active = true
	a.Origin.DeviceActive = false
	if err = st.RegisterMatrixResourceBootstrapRoom(ctx, plan); err == nil {
		t.Fatal("historical plan revived retired SDK producer")
	}
	still, managed, err := st.MatrixPublicationRoomRoute(ctx, plan.Value().RoomID)
	if err != nil || !managed || still != route {
		t.Fatal("retirement deleted managed route evidence", err)
	}
	svc := service.New(config.Config{}, st, nil, nil)
	opaque := publication.OriginEvent{Version: 1, Reference: storage.Reference{RoomID: plan.Value().RoomID, EventID: "$registered-resource-origin"}, Sender: current.Actor().UserID, Type: "m.room.encrypted", Content: encrypted}
	for _, accepted := range []bool{false, true} {
		var decision publication.OriginDecision
		if accepted {
			decision, err = svc.AcceptedMatrixPublicationOrigin(ctx, opaque)
		} else {
			decision, err = svc.BeforeMatrixPublicationOrigin(ctx, opaque)
		}
		if err != nil || !decision.Managed || decision.Allow {
			t.Fatal("registered resource fell through unmanaged/personal origin", accepted, err, decision)
		}
	}
}
