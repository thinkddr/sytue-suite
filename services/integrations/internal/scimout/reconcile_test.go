// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package scimout

import (
	"context"
	"log/slog"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// --- Dobles en memoria (sin base de datos ni red hacia Sytue ID) --------------

type fakeDir struct {
	users  []directory.User
	groups []directory.Group
}

func (f *fakeDir) ListUsers(context.Context, string) ([]directory.User, error) { return f.users, nil }

func (f *fakeDir) ListGroups(context.Context, string) ([]directory.Group, error) {
	return f.groups, nil
}

type fakeMap struct {
	entries    map[string]domain.ScimMapEntry
	lastCounts domain.SyncCounts
	lastStatus string
}

func newFakeMap() *fakeMap { return &fakeMap{entries: map[string]domain.ScimMapEntry{}} }

func mapKey(kind domain.ScimKind, sytueID string) string { return string(kind) + "/" + sytueID }

func (f *fakeMap) LoadScimMap(context.Context, string) ([]domain.ScimMapEntry, error) {
	out := make([]domain.ScimMapEntry, 0, len(f.entries))
	for _, e := range f.entries {
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeMap) UpsertScimMap(_ context.Context, _ string, e domain.ScimMapEntry) error {
	f.entries[mapKey(e.Kind, e.SytueID)] = e
	return nil
}

func (f *fakeMap) DeleteScimMap(_ context.Context, _ string, kind domain.ScimKind, sytueID string) error {
	delete(f.entries, mapKey(kind, sytueID))
	return nil
}

func (f *fakeMap) StartSyncRun(context.Context, string) (string, error) { return "run-1", nil }

func (f *fakeMap) FinishSyncRun(_ context.Context, _ string, c domain.SyncCounts, status string) error {
	f.lastCounts, f.lastStatus = c, status
	return nil
}

// allowLoopback relaja la barrera SSRF para llegar al mock en 127.0.0.1 a TRAVÉS
// del egress central (policy.BuildRuntime). Se restaura al terminar.
func allowLoopback(t *testing.T) {
	t.Helper()
	restore := ssrfhttp.SetIPPolicyForTest(func(net.IP) bool { return false })
	t.Cleanup(restore)
}

// setup arranca el mock SCIM y construye las Deps con el cliente cableado sobre el
// Runtime.HTTP de la pasarela (egress central): así el test prueba también que TODO
// el tráfico al SaaS sale por el cliente de políticas, no por uno propio.
func setup(t *testing.T, dir *fakeDir) (Deps, *scimMock, connector.Instance) {
	t.Helper()
	allowLoopback(t)
	m := newSCIMMock()
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)

	rt := policy.BuildRuntime(policy.RuntimeParams{OrgID: "acme", InstanceID: "inst-1", Log: slog.Default()})
	deps := Deps{Dir: dir, Map: newFakeMap(), SCIM: NewClient(rt.HTTP, srv.URL)}
	inst := connector.Instance{ID: "inst-1", OrgID: "acme"}
	return deps, m, inst
}

func TestProvisionNewUsersAndGroup(t *testing.T) {
	dir := &fakeDir{
		users: []directory.User{
			{ID: "u1", Email: "a@acme.test", Name: "Alice", Active: true},
			{ID: "u2", Email: "b@acme.test", Name: "Bob", Active: true},
		},
		groups: []directory.Group{{ID: "g1", Name: "Engineering", Members: []string{"u1", "u2"}}},
	}
	deps, m, inst := setup(t, dir)

	counts, err := Reconcile(t.Context(), deps, inst)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if counts.Created != 3 || counts.Updated != 0 || counts.Deactivated != 0 || counts.Errors != 0 {
		t.Fatalf("contadores = %+v, quiero Created=3 (2 usuarios + 1 grupo)", counts)
	}
	if m.posts != 3 {
		t.Fatalf("POSTs = %d, quiero 3", m.posts)
	}
	// El grupo se creó con sus dos miembros (ids remotos resueltos).
	fm := deps.Map.(*fakeMap)
	ge := fm.entries[mapKey(domain.ScimGroup, "g1")]
	m.mu.Lock()
	members, _ := m.groups[ge.RemoteID]["members"].([]any)
	m.mu.Unlock()
	if len(members) != 2 {
		t.Fatalf("miembros del grupo remoto = %d, quiero 2", len(members))
	}
}

func TestAdoptsPreexistingUserByFilter(t *testing.T) {
	dir := &fakeDir{users: []directory.User{{ID: "u1", Email: "a@acme.test", Active: true}}}
	deps, m, inst := setup(t, dir)
	existingID := m.seedUser("a@acme.test") // ya existe en el SaaS con ese userName.

	counts, err := Reconcile(t.Context(), deps, inst)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// Adopción: se ALINEA el recurso preexistente (PUT), NO se duplica (0 POST).
	if m.posts != 0 {
		t.Fatalf("POSTs = %d, quiero 0 (no duplicar; adoptar)", m.posts)
	}
	if m.puts != 1 {
		t.Fatalf("PUTs = %d, quiero 1 (alinear el adoptado)", m.puts)
	}
	if counts.Created != 0 || counts.Updated != 1 {
		t.Fatalf("contadores = %+v, quiero Updated=1 (adoptado)", counts)
	}
	fm := deps.Map.(*fakeMap)
	if got := fm.entries[mapKey(domain.ScimUser, "u1")].RemoteID; got != existingID {
		t.Fatalf("remote_id mapeado = %q, quiero el preexistente %q", got, existingID)
	}
}

func TestUpdatesOnDrift(t *testing.T) {
	dir := &fakeDir{users: []directory.User{{ID: "u1", Email: "a@acme.test", Name: "Alice", Active: true}}}
	deps, m, inst := setup(t, dir)
	ctx := t.Context()

	if _, err := Reconcile(ctx, deps, inst); err != nil {
		t.Fatalf("Reconcile 1: %v", err)
	}
	// Cambia el nombre en el directorio ⇒ el hash difiere ⇒ segundo pase actualiza.
	dir.users[0].Name = "Alice Cooper"
	m.resetCounts()

	counts, err := Reconcile(ctx, deps, inst)
	if err != nil {
		t.Fatalf("Reconcile 2: %v", err)
	}
	if counts.Updated != 1 || counts.Created != 0 {
		t.Fatalf("contadores = %+v, quiero Updated=1", counts)
	}
	if m.puts != 1 || m.posts != 0 {
		t.Fatalf("PUTs=%d POSTs=%d, quiero PUT=1 POST=0", m.puts, m.posts)
	}
}

func TestDeactivatesRemovedUser(t *testing.T) {
	dir := &fakeDir{users: []directory.User{
		{ID: "u1", Email: "a@acme.test", Active: true},
		{ID: "u2", Email: "b@acme.test", Active: true},
	}}
	deps, m, inst := setup(t, dir)
	ctx := t.Context()

	if _, err := Reconcile(ctx, deps, inst); err != nil {
		t.Fatalf("Reconcile 1: %v", err)
	}
	fm := deps.Map.(*fakeMap)
	u2remote := fm.entries[mapKey(domain.ScimUser, "u2")].RemoteID

	// u2 se da de baja en Sytue (desaparece del directorio).
	dir.users = dir.users[:1]
	m.resetCounts()

	counts, err := Reconcile(ctx, deps, inst)
	if err != nil {
		t.Fatalf("Reconcile 2: %v", err)
	}
	if counts.Deactivated != 1 {
		t.Fatalf("contadores = %+v, quiero Deactivated=1", counts)
	}
	// DESACTIVAR, no BORRAR: PATCH active=false, cero DELETE.
	if m.patches != 1 || m.deletes != 0 {
		t.Fatalf("PATCHes=%d DELETEs=%d, quiero PATCH=1 DELETE=0", m.patches, m.deletes)
	}
	m.mu.Lock()
	active := m.users[u2remote]["active"]
	m.mu.Unlock()
	if active != false {
		t.Fatalf("el usuario remoto sigue activo = %v, quiero desactivado (false)", active)
	}
	if _, ok := fm.entries[mapKey(domain.ScimUser, "u2")]; ok {
		t.Fatal("el mapeo de u2 debería haberse borrado tras desactivar")
	}
}

func TestIdempotentSecondPass(t *testing.T) {
	dir := &fakeDir{
		users:  []directory.User{{ID: "u1", Email: "a@acme.test", Active: true}, {ID: "u2", Email: "b@acme.test", Active: true}},
		groups: []directory.Group{{ID: "g1", Name: "Eng", Members: []string{"u1", "u2"}}},
	}
	deps, m, inst := setup(t, dir)
	ctx := t.Context()

	if _, err := Reconcile(ctx, deps, inst); err != nil {
		t.Fatalf("Reconcile 1: %v", err)
	}
	m.resetCounts()

	counts, err := Reconcile(ctx, deps, inst)
	if err != nil {
		t.Fatalf("Reconcile 2: %v", err)
	}
	if (counts != domain.SyncCounts{}) {
		t.Fatalf("segundo pase sin cambios: contadores = %+v, quiero todo 0", counts)
	}
	// Idempotencia fuerte: CERO peticiones al SaaS en el segundo pase.
	if m.reqs != 0 {
		t.Fatalf("peticiones SCIM en el pase idempotente = %d, quiero 0", m.reqs)
	}
}

func TestRetriesOn429DuringReconcile(t *testing.T) {
	dir := &fakeDir{users: []directory.User{{ID: "u1", Email: "a@acme.test", Active: true}}}
	deps, m, inst := setup(t, dir)
	m.fails = []failResp{{status: 429, retryAfter: "1"}}

	var slept time.Duration
	deps.SCIM.sleep = func(d time.Duration) { slept = d }

	counts, err := Reconcile(t.Context(), deps, inst)
	if err != nil {
		t.Fatalf("Reconcile con 429: %v", err)
	}
	if slept != time.Second {
		t.Fatalf("espera = %v, quiero 1s (Retry-After respetado en el reconcile)", slept)
	}
	if counts.Created != 1 || counts.Errors != 0 {
		t.Fatalf("contadores = %+v, quiero Created=1 Errors=0 (reintento transparente)", counts)
	}
}
