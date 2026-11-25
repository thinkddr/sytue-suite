// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// La RLS de Sytue ID (migración 0127): sin ámbito declarado, todo como antes; con
// ámbito, la base niega lo de fuera aunque el SQL lo pida. Las pruebas de SQL crudo
// hablan con h.Pool sin pasar por el store, así que lo único que puede esconder una
// fila es la política (y el arnés conecta con un rol NOBYPASSRLS: un superusuario no
// evaluaría ninguna).
package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// dosOrganizaciones siembra A y B, una persona en cada una y un dispositivo de cada
// persona. Todo SIN ámbito, que es como escriben los flujos del IdP.
type dosOrganizaciones struct {
	a, b       domain.Organization
	ana, bruno domain.User
	devA, devB domain.Device
}

func sembrarDos(ctx context.Context, t *testing.T, st *store.Store) dosOrganizaciones {
	t.Helper()
	var d dosOrganizaciones
	var err error
	if d.a, err = st.CreateOrg(ctx, nil, domain.Organization{Slug: "ambito-a", Name: "A"}); err != nil {
		t.Fatalf("CreateOrg A: %v", err)
	}
	if d.b, err = st.CreateOrg(ctx, nil, domain.Organization{Slug: "ambito-b", Name: "B"}); err != nil {
		t.Fatalf("CreateOrg B: %v", err)
	}
	d.ana = seedUser(ctx, t, st, "ana@ambito.test")
	d.bruno = seedUser(ctx, t, st, "bruno@ambito.test")
	for _, m := range []domain.Membership{
		{OrgID: d.a.ID, UserID: d.ana.ID, Role: domain.RoleAdmin},
		{OrgID: d.b.ID, UserID: d.bruno.ID, Role: domain.RoleMember},
	} {
		if err := st.UpsertMembership(ctx, nil, m); err != nil {
			t.Fatalf("UpsertMembership: %v", err)
		}
	}
	if d.devA, err = st.UpsertDevice(ctx, nil, domain.Device{OrgID: d.a.ID, UserID: d.ana.ID, DeviceID: "dev-ana", Name: "Ana"}); err != nil {
		t.Fatalf("UpsertDevice A: %v", err)
	}
	if d.devB, err = st.UpsertDevice(ctx, nil, domain.Device{OrgID: d.b.ID, UserID: d.bruno.ID, DeviceID: "dev-bruno", Name: "Bruno"}); err != nil {
		t.Fatalf("UpsertDevice B: %v", err)
	}
	return d
}

func esViolacionDeRLS(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "42501"
}

// TestRLSForzadaEnSytueID: toda tabla con organización (`org_id` o `tenant`) lleva la
// RLS habilitada Y forzada. Sin excepciones: si una tabla nueva nace sin ella, falla
// el día que se crea.
func TestRLSForzadaEnSytueID(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	pgtest.ExigirRLSForzada(t, h, nil)
}

// TestCadaTablaConDuenoTieneSuAmbito: lo que ExigirRLSForzada no mira. Cada columna
// de ámbito (`org_id`, `tenant`, `user_id`) tiene que estar en la política de su
// tabla con SU función: una tabla por persona sin política sería un hueco silencioso,
// y una con la RLS forzada pero sin política dejaría al IdP sin poder leerla.
func TestCadaTablaConDuenoTieneSuAmbito(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	// Tablas que tienen columnas de ámbito y no se confinan, con su porqué.
	excepciones := map[string]string{
		"usernames_retirados": "reserva global de @usuario: cambiar el propio pregunta por las de OTRAS personas (0126)",
	}
	rows, err := h.Pool.Query(context.Background(), `
		SELECT c.relname, a.attname,
		       coalesce((SELECT string_agg(coalesce(p.qual, '') || ' ' || coalesce(p.with_check, ''), ' ')
		                   FROM pg_policies p
		                  WHERE p.schemaname = 'public' AND p.tablename = c.relname), '')
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		 WHERE c.relkind IN ('r', 'p') AND NOT c.relispartition AND n.nspname = 'public'
		   AND a.attname IN ('org_id', 'tenant', 'user_id')
		 ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	funcion := map[string]string{
		"org_id":  "id_ambito_org(org_id)",
		"tenant":  "id_ambito_tenant(tenant)",
		"user_id": "id_ambito_persona(user_id)",
	}
	vistas := 0
	for rows.Next() {
		var tabla, columna, politicas string
		if err := rows.Scan(&tabla, &columna, &politicas); err != nil {
			t.Fatal(err)
		}
		vistas++
		if _, ok := excepciones[tabla]; ok {
			continue
		}
		quiere := funcion[columna]
		// Dos veces: en USING y en WITH CHECK. Solo en USING dejaría ESCRIBIR fuera
		// del ámbito lo que luego no se puede leer.
		if n := contar(politicas, quiere); n < 2 {
			t.Errorf("%s.%s: la política no aplica %s en USING y WITH CHECK (políticas: %q)", tabla, columna, quiere, politicas)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if vistas < 50 {
		t.Fatalf("solo %d columnas de ámbito en el catálogo: la consulta no mira lo que cree", vistas)
	}
}

func contar(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

// TestAmbitoConSQLCrudo: la política es la que esconde, no el store. Sin ámbito se ve
// todo; con el de A, ni leer ni tocar ni insertar en B; y al acabar la transacción la
// conexión vuelve a no tener ámbito (la variable queda vacía, no con el valor viejo).
func TestAmbitoConSQLCrudo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()
	d := sembrarDos(ctx, t, st)

	conn, err := h.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	contarFilas := func(q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	}, sql string, args ...any,
	) int {
		t.Helper()
		var n int
		if err := q.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}

	if n := contarFilas(conn, `SELECT count(*) FROM memberships`); n != 2 {
		t.Fatalf("sin ámbito: %d membresías, quiero 2", n)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.org_id', $1, true), set_config('app.tenant', $2, true)`, d.a.ID, d.a.Slug); err != nil {
		t.Fatal(err)
	}
	if n := contarFilas(tx, `SELECT count(*) FROM memberships`); n != 1 {
		t.Errorf("con ámbito A: %d membresías, quiero 1", n)
	}
	if n := contarFilas(tx, `SELECT count(*) FROM devices WHERE org_id = $1`, d.b.ID); n != 0 {
		t.Errorf("con ámbito A se ven %d dispositivos de B", n)
	}
	tag, err := tx.Exec(ctx, `UPDATE memberships SET role = 'owner' WHERE org_id = $1`, d.b.ID)
	if err != nil || tag.RowsAffected() != 0 {
		t.Errorf("con ámbito A, UPDATE sobre B: filas=%d err=%v, quiero 0 filas", tag.RowsAffected(), err)
	}
	if _, err := tx.Exec(ctx, `SAVEPOINT s`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'owner')`, d.b.ID, d.ana.ID)
	if !esViolacionDeRLS(err) {
		t.Errorf("con ámbito A, INSERT en B: err=%v, quiero 42501 (WITH CHECK)", err)
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT s`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Misma conexión, transacción nueva: la variable sigue definida pero vacía.
	if n := contarFilas(conn, `SELECT count(*) FROM memberships`); n != 2 {
		t.Fatalf("tras el COMMIT: %d membresías, quiero 2 (el ámbito no puede sobrevivir a su transacción)", n)
	}

	// Por persona: solo lo de Ana, también en tablas sin organización.
	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.user_id', $1, true)`, d.ana.ID); err != nil {
		t.Fatal(err)
	}
	if n := contarFilas(tx, `SELECT count(*) FROM devices`); n != 1 {
		t.Errorf("con ámbito de Ana: %d dispositivos, quiero 1", n)
	}
	if n := contarFilas(tx, `SELECT count(*) FROM memberships WHERE user_id = $1`, d.bruno.ID); n != 0 {
		t.Errorf("con ámbito de Ana se ven %d membresías de Bruno", n)
	}
}

// TestAmbitoEnElStore: el ámbito del contexto llega a la base por los cuatro caminos
// del store —Query, QueryRow y Exec sueltos, y InTx— aunque el método pida
// expresamente la otra organización.
func TestAmbitoEnElStore(t *testing.T) {
	st, ctx := newStore(t)
	d := sembrarDos(ctx, t, st)
	enA := store.ConAmbitoDeOrganizacion(ctx, d.a.ID, d.a.Slug)

	// Query (filas en memoria).
	if devs, err := st.ListDevicesForOrg(enA, d.b.ID); err != nil || len(devs) != 0 {
		t.Errorf("ListDevicesForOrg(B) con ámbito A = %d (%v), quiero 0", len(devs), err)
	}
	devs, err := st.ListDevicesForOrg(enA, d.a.ID)
	if err != nil || len(devs) != 1 || devs[0].ID != d.devA.ID || devs[0].UserEmail != d.ana.Email {
		t.Errorf("ListDevicesForOrg(A) con ámbito A = %+v (%v)", devs, err)
	}
	// QueryRow: el error de «no hay fila» sale igual que sin ámbito.
	if _, err := st.GetMembership(enA, d.b.ID, d.bruno.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetMembership(B) con ámbito A = %v, quiero ErrNotFound", err)
	}
	if m, err := st.GetMembership(enA, d.a.ID, d.ana.ID); err != nil || m.Role != domain.RoleAdmin {
		t.Errorf("GetMembership(A) con ámbito A = %+v (%v)", m, err)
	}
	// Exec: borrar el dispositivo de B pidiéndolo por su organización no hace nada.
	if borrado, err := st.DeleteDeviceForOrg(enA, d.b.ID, d.devB.ID); err != nil || borrado {
		t.Errorf("DeleteDeviceForOrg(B) con ámbito A: borrado=%v err=%v", borrado, err)
	}
	if _, err := st.GetDeviceForUser(ctx, d.bruno.ID, d.devB.ID); err != nil {
		t.Fatalf("el dispositivo de B ya no está: %v", err)
	}
	// InTx: escribir en B dentro de una transacción con ámbito A falla en la base.
	err = st.InTx(enA, func(q pg.Querier) error {
		_, err := q.Exec(enA, `INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'owner')`, d.b.ID, d.ana.ID)
		return err
	})
	if !esViolacionDeRLS(err) {
		t.Errorf("InTx con ámbito A, INSERT en B: %v, quiero 42501", err)
	}

	// Por persona.
	deAna := store.ConAmbitoDePersona(ctx, d.ana.ID)
	if devs, err := st.ListDevicesForUser(deAna, d.bruno.ID); err != nil || len(devs) != 0 {
		t.Errorf("ListDevicesForUser(Bruno) con ámbito de Ana = %d (%v), quiero 0", len(devs), err)
	}
	if devs, err := st.ListDevicesForUser(deAna, d.ana.ID); err != nil || len(devs) != 1 {
		t.Errorf("ListDevicesForUser(Ana) con ámbito de Ana = %d (%v), quiero 1", len(devs), err)
	}

	// Un ámbito mal formado no se ignora: la consulta falla.
	roto := store.ConAmbitoDeOrganizacion(ctx, "no-es-un-uuid", d.a.Slug)
	if _, err := st.ListDevicesForOrg(roto, d.a.ID); err == nil {
		t.Error("con un ámbito inválido se sirvió la consulta: tiene que fallar cerrado")
	}
	if _, err := st.ListDevicesForOrg(store.ConAmbitoDeOrganizacion(ctx, d.a.ID, ""), d.a.ID); err == nil {
		t.Error("con la organización a medias (uuid sin slug) se sirvió la consulta")
	}
}

// TestLosDisparadoresVenTodasLasOrganizaciones: suspender a una persona desde el
// ámbito de UNA organización deja la valla de llamadas de documentos (0117) en TODAS
// las suyas. Con la política aplicada dentro del disparador solo se escribiría la de
// la organización activa: una revocación a medias que no falla.
func TestLosDisparadoresVenTodasLasOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()
	d := sembrarDos(ctx, t, st)
	// Ana también es de B.
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: d.b.ID, UserID: d.ana.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	versiones := func() map[string]int64 {
		t.Helper()
		rows, err := h.Pool.Query(ctx, `SELECT org_slug, version FROM identity_call_access_fences WHERE subject = $1`, d.ana.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]int64{}
		for rows.Next() {
			var slug string
			var v int64
			if err := rows.Scan(&slug, &v); err != nil {
				t.Fatal(err)
			}
			out[slug] = v
		}
		return out
	}
	antes := versiones()

	enA := store.ConAmbitoDeOrganizacion(ctx, d.a.ID, d.a.Slug)
	if _, err := st.DB().Exec(enA, `UPDATE users SET status = 'suspended' WHERE id = $1`, d.ana.ID); err != nil {
		t.Fatalf("suspender con ámbito A: %v", err)
	}
	despues := versiones()
	for _, slug := range []string{d.a.Slug, d.b.Slug} {
		if despues[slug] <= antes[slug] {
			t.Errorf("la valla de %s no avanzó (%d → %d): el disparador no vio esa organización", slug, antes[slug], despues[slug])
		}
	}
}
