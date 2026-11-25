// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package pg_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

// El esquema reproduce, en pequeño, todo lo que una purga se encuentra en la
// suite. El orden alfabético —el del catálogo— está elegido para que salgan mal
// a la primera: el padre va antes que su hija sin cascada, y la cola que llena un
// disparador al borrar va antes que la tabla que la llena.
const purgaFixtureDDL = `
-- sytue:scope-bound-merge-v1
-- sytue:tenant-purge-v1
CREATE TABLE a_padre (tenant text NOT NULL, id int, PRIMARY KEY (tenant, id));
CREATE TABLE b_hija (tenant text NOT NULL, id int, padre int,
    FOREIGN KEY (tenant, padre) REFERENCES a_padre (tenant, id));
CREATE TABLE a_cola (tenant text NOT NULL, que text);
CREATE TABLE d_origen (tenant text NOT NULL, id int);
CREATE FUNCTION d_origen_encolar() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO a_cola (tenant, que) VALUES (OLD.tenant, 'borrado'); RETURN OLD; END $$;
CREATE TRIGGER d_origen_encolar AFTER DELETE ON d_origen FOR EACH ROW EXECUTE FUNCTION d_origen_encolar();
CREATE TABLE c_inmutable (tenant text NOT NULL, asiento int);
CREATE FUNCTION c_inmutable_guardia() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'asiento inmutable'; END $$;
CREATE TRIGGER c_inmutable_guardia BEFORE DELETE ON c_inmutable FOR EACH ROW EXECUTE FUNCTION c_inmutable_guardia();
CREATE TABLE e_ligada (tenant text NOT NULL, clave text);
CREATE TABLE legal_holds (tenant text NOT NULL, subject text, released_at timestamptz);
DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['a_padre','b_hija','a_cola','d_origen','c_inmutable','e_ligada','legal_holds'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY %I ON %I USING (tenant = current_setting(''app.tenant'', true)) WITH CHECK (tenant = current_setting(''app.tenant'', true))', t || '_aislamiento', t);
    IF t <> 'e_ligada' THEN
      EXECUTE format('CREATE POLICY %I ON %I USING (current_setting(''app.merge'', true) = ''on'') WITH CHECK (current_setting(''app.merge'', true) = ''on'')', t || '_platform_merge', t);
    END IF;
  END LOOP;
END $$;
SELECT sytue_scope_merge_register('e_ligada'::regclass);
-- Como Sytue Integrations: el slug en org_id, sin política de fusión.
CREATE TABLE f_por_org (org_id text NOT NULL, dato text);
ALTER TABLE f_por_org ENABLE ROW LEVEL SECURITY;
ALTER TABLE f_por_org FORCE ROW LEVEL SECURITY;
CREATE POLICY f_por_org_aislamiento ON f_por_org
    USING (org_id = current_setting('app.tenant', true)) WITH CHECK (org_id = current_setting('app.tenant', true));
`

func purgaHarness(t *testing.T, gancho string) (*pgtest.Harness, *pg.Pool) {
	t.Helper()
	fsys := pg.WithTenantPurgeDDL(pg.WithScopeBoundMergeDDL(fstest.MapFS{
		"migrations/0001_fixture.up.sql": {Data: []byte(purgaFixtureDDL + gancho)},
	}))
	h := pgtest.New(t, fsys, "migrations")
	admin := h.PoolSinRLS(t)
	for _, slug := range []string{"p-se-va", "se-queda"} {
		sembrarPurga(t, admin, slug)
	}
	return h, admin
}

func sembrarPurga(t *testing.T, admin *pg.Pool, slug string) {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO a_padre VALUES ($1, 1), ($1, 2)`,
		`INSERT INTO b_hija VALUES ($1, 1, 1)`,
		`INSERT INTO d_origen VALUES ($1, 1), ($1, 2)`,
		`INSERT INTO c_inmutable VALUES ($1, 7)`,
		`INSERT INTO e_ligada VALUES ($1, 'k')`,
		`INSERT INTO f_por_org VALUES ($1, 'x')`,
	} {
		if _, err := admin.Exec(ctx, q, slug); err != nil {
			t.Fatalf("sembrar %q: %v", q, err)
		}
	}
}

func filasDe(t *testing.T, admin *pg.Pool, tabla, slug string) int {
	t.Helper()
	columna := "tenant"
	if tabla == "f_por_org" {
		columna = "org_id"
	}
	var n int
	if err := admin.QueryRow(context.Background(), `SELECT count(*) FROM `+pgx.Identifier{tabla}.Sanitize()+` WHERE `+columna+` = $1`, slug).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPurgarTenantBorraTodoMenosLoQueGuardaUnGuardian(t *testing.T) {
	h, admin := purgaHarness(t, "")
	ctx := context.Background()

	res, err := pg.PurgarTenant(ctx, h.Pool, "p-se-va")
	if err != nil {
		t.Fatal(err)
	}
	// La FK sin cascada, la cola rellenada por un disparador y la tabla de ámbito
	// ligado: las tres se vacían aunque el catálogo las dé en mal orden.
	for _, tabla := range []string{"a_padre", "b_hija", "a_cola", "d_origen", "e_ligada", "f_por_org"} {
		if n := filasDe(t, admin, tabla, "p-se-va"); n != 0 {
			t.Errorf("%s: quedan %d filas del tenant purgado", tabla, n)
		}
	}
	// El guardián no se salta: su tabla queda entera y se informa con el motivo.
	if len(res.Retenidas) != 1 || res.Retenidas[0].Tabla != "c_inmutable" || res.Retenidas[0].Filas != 1 ||
		!strings.Contains(res.Retenidas[0].Motivo, "asiento inmutable") {
		t.Fatalf("retenidas = %+v; quiero solo c_inmutable con su motivo", res.Retenidas)
	}
	if res.Completa() || res.ObjetosEncolados {
		t.Fatal("con algo retenido no se puede ordenar el borrado de objetos")
	}
	// Lo de la otra organización, intacto.
	for tabla, quiero := range map[string]int{"a_padre": 2, "b_hija": 1, "d_origen": 2, "c_inmutable": 1, "e_ligada": 1, "a_cola": 0, "f_por_org": 1} {
		if n := filasDe(t, admin, tabla, "se-queda"); n != quiero {
			t.Errorf("%s de otra organización: %d filas, quiero %d", tabla, n, quiero)
		}
	}
	// Idempotente: repetirla no borra nada más y vuelve a decir lo mismo.
	otra, err := pg.PurgarTenant(ctx, h.Pool, "p-se-va")
	if err != nil || otra.Total != 0 || len(otra.Retenidas) != 1 {
		t.Fatalf("repetir: %+v, %v", otra, err)
	}
}

func TestPurgarTenantCompletaEncolaLosObjetos(t *testing.T) {
	h, admin := purgaHarness(t, "")
	ctx := context.Background()
	// Sin el asiento inmutable la base se puede vaciar entera.
	if _, err := admin.Exec(ctx, `ALTER TABLE c_inmutable DISABLE TRIGGER c_inmutable_guardia`); err != nil {
		t.Fatal(err)
	}
	res, err := pg.PurgarTenant(ctx, h.Pool, "p-se-va")
	if err != nil || !res.Completa() || !res.ObjetosEncolados {
		t.Fatalf("purga completa: %+v, %v", res, err)
	}
	estado, err := pg.ConsultarPurgaDeObjetos(ctx, h.Pool, "p-se-va")
	if err != nil || !estado.Pendiente || estado.Hecho() {
		t.Fatalf("recién encolada: %+v, %v", estado, err)
	}
	reclamadas, err := pg.ReclamarPurgasDeObjetos(ctx, h.Pool, 10, time.Minute)
	if err != nil || len(reclamadas) != 1 || reclamadas[0].Slug != "p-se-va" {
		t.Fatalf("reclamar: %+v, %v", reclamadas, err)
	}
	// Otra réplica que pregunta a la vez no se la lleva: el intento es reciente.
	if otra, err := pg.ReclamarPurgasDeObjetos(ctx, h.Pool, 10, time.Minute); err != nil || len(otra) != 0 {
		t.Fatalf("doble reclamo: %+v, %v", otra, err)
	}
	// Si Sytue ID reabre la orden mientras se barre (borró filas nuevas), marcar
	// con la generación vieja no la cierra.
	sembrarPurga(t, admin, "p-se-va")
	if _, err := admin.Exec(ctx, `UPDATE c_inmutable SET asiento = 8`); err != nil {
		t.Fatal(err)
	}
	if res, err := pg.PurgarTenant(ctx, h.Pool, "p-se-va"); err != nil || !res.Completa() || res.Total == 0 {
		t.Fatalf("segunda purga: %+v, %v", res, err)
	}
	if err := pg.MarcarObjetosPurgados(ctx, h.Pool, reclamadas[0]); err != nil {
		t.Fatal(err)
	}
	if estado, _ := pg.ConsultarPurgaDeObjetos(ctx, h.Pool, "p-se-va"); estado.Hecho() {
		t.Fatal("una generación vieja cerró la orden reabierta")
	}
	if _, err := admin.Exec(ctx, `UPDATE platform_tenant_purges SET last_attempt_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	nuevas, err := pg.ReclamarPurgasDeObjetos(ctx, h.Pool, 10, time.Minute)
	if err != nil || len(nuevas) != 1 {
		t.Fatalf("reclamar la reabierta: %+v, %v", nuevas, err)
	}
	if err := pg.AnotarFalloPurgaDeObjetos(ctx, h.Pool, "p-se-va", errors.New(strings.Repeat("é", 400))); err != nil {
		t.Fatal(err)
	}
	if err := pg.MarcarObjetosPurgados(ctx, h.Pool, nuevas[0]); err != nil {
		t.Fatal(err)
	}
	estado, err = pg.ConsultarPurgaDeObjetos(ctx, h.Pool, "p-se-va")
	if err != nil || !estado.Hecho() || estado.UltimoErr != "" {
		t.Fatalf("cerrada: %+v, %v", estado, err)
	}
	// Una base sin cola no espera a nadie.
	sinCola := pgtest.New(t, fstest.MapFS{"migrations/0001_x.up.sql": {Data: []byte(`CREATE TABLE x (tenant text)`)}}, "migrations")
	if e, err := pg.ConsultarPurgaDeObjetos(ctx, sinCola.Pool, "p-se-va"); err != nil || !e.SinCola || !e.Hecho() {
		t.Fatalf("sin cola: %+v, %v", e, err)
	}
}

func TestPurgarTenantRespetaLaRetencionLegalYElGancho(t *testing.T) {
	gancho := `
CREATE FUNCTION platform_tenant_purge_prepare(t text) RETURNS text LANGUAGE plpgsql AS $$
BEGIN
  IF t = 'p-retenido' THEN
    RAISE EXCEPTION 'quedan facturas en plazo legal' USING ERRCODE = 'PTP01';
  END IF;
  RETURN 'preparado ' || t;
END $$;`
	h, admin := purgaHarness(t, gancho)
	ctx := context.Background()

	if _, err := admin.Exec(ctx, `INSERT INTO legal_holds VALUES ('p-se-va', NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	_, err := pg.PurgarTenant(ctx, h.Pool, "p-se-va")
	var ret *pg.PurgaRetenidaError
	if !errors.Is(err, pg.ErrPurgaRetenida) || !errors.As(err, &ret) || !strings.Contains(ret.Motivo, "retenciones legales") {
		t.Fatalf("con retención legal activa: %v", err)
	}
	if n := filasDe(t, admin, "a_padre", "p-se-va"); n != 2 {
		t.Fatalf("una retención legal no deja borrar nada: quedan %d", n)
	}
	// Liberada, sigue; el gancho habla.
	if _, err := admin.Exec(ctx, `UPDATE legal_holds SET released_at = now()`); err != nil {
		t.Fatal(err)
	}
	res, err := pg.PurgarTenant(ctx, h.Pool, "p-se-va")
	if err != nil || res.Preparado != "preparado p-se-va" {
		t.Fatalf("tras liberar: %+v, %v", res, err)
	}
	sembrarPurga(t, admin, "p-retenido")
	if _, err := pg.PurgarTenant(ctx, h.Pool, "p-retenido"); !errors.Is(err, pg.ErrPurgaRetenida) || !strings.Contains(err.Error(), "plazo legal") {
		t.Fatalf("el gancho retiene: %v", err)
	}
	if n := filasDe(t, admin, "a_padre", "p-retenido"); n != 2 {
		t.Fatal("el gancho retuvo y aun así se borró")
	}
	if _, err := pg.PurgarTenant(ctx, h.Pool, "NO-VALE"); err == nil {
		t.Fatal("un slug inválido no se purga")
	}
}
