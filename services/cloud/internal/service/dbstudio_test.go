// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// El DSN de un Postgres gestionado tal cual lo genera provisionPostgres.
const testDSN = "postgres://app:s3cr3t-pass@svc-abc-postgres.tenant-acme.svc.cluster.local:5432/svc_abc?sslmode=disable"

// TestResolveDbTargetSinPostgresListo: sin un servicio postgres READY no hay
// nada real que enseñar → 404 (la UI degrada), nunca datos inventados.
func TestResolveDbTargetSinPostgresListo(t *testing.T) {
	t.Parallel()
	cases := map[string][]domain.ManagedService{
		"sin servicios": nil,
		"postgres aprovisionándose": {{
			Kind: domain.ServicePostgres, Status: domain.ServiceProvisioning,
		}},
		"otro tipo de servicio": {{
			Kind: domain.ServiceStorage, Status: domain.ServiceReady,
		}},
	}
	for name, svcs := range cases {
		t.Run(name, func(t *testing.T) {
			s := &Service{store: &fakeStore{svcs: svcs}}
			if _, err := s.resolveDbTarget(context.Background(), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("esperado ErrNotFound, obtenido %v", err)
			}
		})
	}
}

// TestResolveDbTargetSinCifrador: con el store concreto o el sealer sin cablear
// no se pueden descifrar credenciales; debe dar 404 y NO reventar (la ruta se
// detiene antes de tocar tenancy/creds).
func TestResolveDbTargetSinCifrador(t *testing.T) {
	t.Parallel()
	s := &Service{store: &fakeStore{svcs: []domain.ManagedService{{
		Kind: domain.ServicePostgres, Status: domain.ServiceReady,
	}}}}
	if _, err := s.resolveDbTarget(context.Background(), uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("esperado ErrNotFound, obtenido %v", err)
	}
}

// TestRunDbQueryEntradaInvalida: la validación de entrada ocurre ANTES de tocar
// la base de datos del cliente (por eso el Service va vacío: si la tocara,
// petaría).
func TestRunDbQueryEntradaInvalida(t *testing.T) {
	t.Parallel()
	s := &Service{}
	for name, sql := range map[string]string{
		"vacía":       "   \n\t ",
		"desmesurada": strings.Repeat("-", dbStudioMaxSQLLen+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.RunDbQuery(context.Background(), uuid.New(), sql); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("esperado ErrInvalidInput, obtenido %v", err)
			}
		})
	}
}

// TestDbStudioConnConfigLimites: los topes que protegen la plataforma tienen que
// viajar en el arranque de la conexión. Si esto se cae, una consulta del cliente
// puede tener un worker de su Postgres ocupado indefinidamente.
func TestDbStudioConnConfigLimites(t *testing.T) {
	t.Parallel()
	cfg, err := dbStudioConnConfig(testDSN)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if got := cfg.RuntimeParams["statement_timeout"]; got != "8000" {
		t.Fatalf("statement_timeout = %q", got)
	}
	if got := cfg.RuntimeParams["idle_in_transaction_session_timeout"]; got != "8000" {
		t.Fatalf("idle_in_transaction_session_timeout = %q", got)
	}
	if cfg.ConnectTimeout != dbStudioConnectTimeout {
		t.Fatalf("ConnectTimeout = %v", cfg.ConnectTimeout)
	}
	if cfg.RuntimeParams["application_name"] != "sytue-db-studio" {
		t.Fatalf("application_name = %q", cfg.RuntimeParams["application_name"])
	}
	if _, err := dbStudioConnConfig("esto no es un DSN://"); err == nil {
		t.Fatal("un DSN corrupto debería fallar")
	}
}

// TestMaskDSN: la contraseña NUNCA puede salir por la API.
func TestMaskDSN(t *testing.T) {
	t.Parallel()
	got := maskDSN(testDSN)
	if strings.Contains(got, "s3cr3t-pass") {
		t.Fatalf("la contraseña se ha filtrado: %s", got)
	}
	if !strings.Contains(got, dbStudioMask) || !strings.Contains(got, "app:") ||
		!strings.Contains(got, "svc_abc") {
		t.Fatalf("cadena enmascarada inservible: %s", got)
	}
	// Un DSN que no se puede parsear se enmascara ENTERO (fail-closed).
	if got := maskDSN("host=x password=abc"); strings.Contains(got, "abc") {
		t.Fatalf("fail-closed roto: %s", got)
	}
}

// TestDbAPIInfoNoRevelaClaves: el preview de las claves es una máscara; el
// secreto JWT del proyecto no se firma ni se devuelve aquí.
func TestDbAPIInfoNoRevelaClaves(t *testing.T) {
	t.Parallel()
	info := DbAPIInfo{
		RestURL:           "https://p1.db.sytue.app",
		AnonKeyPreview:    "JWT HS256 " + dbStudioMask,
		ServiceKeyPreview: "JWT HS256 " + dbStudioMask,
		ConnectionString:  maskDSN(testDSN),
	}
	blob, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	for _, secreto := range []string{"s3cr3t-pass", "eyJ"} {
		if strings.Contains(string(blob), secreto) {
			t.Fatalf("credencial en claro en %s", blob)
		}
	}
}

func TestParseQualifiedTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in           string
		schema, name string
		wantErr      bool
	}{
		{in: "public.clientes", schema: "public", name: "clientes"},
		{in: "ventas.facturas_2026", schema: "ventas", name: "facturas_2026"},
		{in: "clientes", schema: "public", name: "clientes"}, // sin esquema → público
		{in: "  public.clientes  ", schema: "public", name: "clientes"},
		{in: `a."b.c"`, schema: "a", name: `"b.c"`}, // el primer punto separa
		{in: "", wantErr: true},
		{in: ".clientes", wantErr: true},
		{in: "public.", wantErr: true},
		{in: "public." + strings.Repeat("x", 64), wantErr: true}, // > NAMEDATALEN
	}
	for _, c := range cases {
		schema, name, err := parseQualifiedTable(c.in)
		if c.wantErr {
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("%q: esperado ErrInvalidInput, obtenido %v", c.in, err)
			}
			continue
		}
		if err != nil || schema != c.schema || name != c.name {
			t.Fatalf("%q → (%q,%q,%v)", c.in, schema, name, err)
		}
	}
}

// TestJSONSafeValue: uuid y bytea llegan de pgx como arrays de bytes y sin
// normalizar se serializarían como listas de números en el navegador.
func TestJSONSafeValue(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("6f6f2b3a-0e6e-4f2a-9f0e-2a7b1c3d4e5f")
	if got := jsonSafeValue([16]byte(id)); got != id.String() {
		t.Fatalf("uuid = %v", got)
	}
	if got := jsonSafeValue([]byte("hola")); got != "hola" {
		t.Fatalf("texto = %v", got)
	}
	if got := jsonSafeValue([]byte{0xff, 0x00}); got != `\xff00` {
		t.Fatalf("bytea = %v", got)
	}
	if got := jsonSafeValue(nil); got != nil {
		t.Fatalf("nil = %v", got)
	}
	if got := jsonSafeValue(42); got != 42 {
		t.Fatalf("passthrough = %v", got)
	}
}

// TestSQLErrorMessage: el timeout se explica como límite de Sytue (lo es); el
// resto del error de Postgres es del propio cliente sobre su BD.
func TestSQLErrorMessage(t *testing.T) {
	t.Parallel()
	got := sqlErrorMessage(errors.New(`ERROR: canceling statement due to statement timeout (SQLSTATE 57014)`))
	if !strings.Contains(got, "superó el límite") {
		t.Fatalf("timeout sin traducir: %s", got)
	}
	if got := sqlErrorMessage(errors.New(`syntax error at or near "SELCT"`)); !strings.Contains(got, "SELCT") {
		t.Fatalf("error de sintaxis perdido: %s", got)
	}
}

// TestContratoJSONDelDashboard fija los nombres de campo EXACTOS que valida
// apps/cloud-dashboard/src/lib/cloud.ts: si cambian, la UI se cae a datos de
// demostración sin dar ni un error visible.
func TestContratoJSONDelDashboard(t *testing.T) {
	t.Parallel()
	cases := []struct {
		valor  any
		claves []string
	}{
		{DbTable{Columns: []DbColumn{{}}}, []string{`"schema"`, `"name"`, `"rowCount"`, `"columns"`, `"apiExposed"`, `"nullable"`, `"isPrimaryKey"`}},
		{DbRows{Columns: []string{}, Rows: []map[string]any{}}, []string{`"columns"`, `"rows"`, `"totalRows"`}},
		{SQLResult{Columns: []string{}, Rows: []map[string]any{}}, []string{`"columns"`, `"rows"`, `"rowCount"`, `"elapsedMs"`}},
		{DbAPIInfo{}, []string{`"restUrl"`, `"anonKeyPreview"`, `"serviceKeyPreview"`, `"connectionString"`}},
	}
	for _, c := range cases {
		blob, err := json.Marshal(c.valor)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range c.claves {
			if !strings.Contains(string(blob), k) {
				t.Fatalf("falta %s en %s", k, blob)
			}
		}
		// Listas vacías como [] y nunca null: la UI itera sobre ellas sin comprobar.
		if strings.Contains(string(blob), `"rows":null`) || strings.Contains(string(blob), `"columns":null`) {
			t.Fatalf("colección nula en %s", blob)
		}
	}
}

// CLOUD-09 — una sola celda gigante (p. ej. repeat('a', 500000000)) agotaba la
// memoria de cloud-api dentro de rows.Values() antes de que el acumulador size
// pudiera cortar en la siguiente iteración. decodeRowBounded inspecciona los
// bytes crudos de la fila ANTES de llamar a decode() y trunca cualquier celda
// que supere dbStudioMaxCellBytes sin materializarla con rows.Values().
func TestDecodeRowBoundedTruncaCeldaGiganteSinDecodificarFila(t *testing.T) {
	t.Parallel()
	decodeCalled := false
	giant := make([]byte, 1024*1024) // 1 MiB (> 64 KiB)
	for i := range giant {
		giant[i] = 'a'
	}
	raw := [][]byte{[]byte("42"), giant}
	rec, wireSize, err := decodeRowBounded(raw, func() ([]any, error) {
		decodeCalled = true
		return []any{42, string(giant)}, nil
	}, []string{"id", "payload"})
	if err != nil {
		t.Fatalf("decodeRowBounded: %v", err)
	}
	if decodeCalled {
		t.Fatal("decode() (rows.Values) NO debe invocarse cuando una celda cruda supera el tope")
	}
	if got := rec["id"]; got != "42" {
		t.Errorf("celda normal id = %v, quiero \"42\"", got)
	}
	wantTrunc := "[truncado: 1048576 bytes]"
	if got := rec["payload"]; got != wantTrunc {
		t.Errorf("celda gigante payload = %v, quiero %q", got, wantTrunc)
	}
	if wireSize < len(giant) {
		t.Errorf("wireSize = %d, debe reflejar los bytes crudos (%d) para cortar la siguiente fila", wireSize, len(giant))
	}
}
