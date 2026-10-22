// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration
// +build integration

// Tests de integracion del store del Sytue Integration Gateway contra un
// PostgreSQL real y efimero (platform/pkg/pgtest). Detras del build-tag
// `integration` (no corren en el CI por defecto). Verifican:
//   - que las migraciones aplican (esquema + RLS ENABLE+FORCE en las tablas por-org),
//   - AISLAMIENTO por org (RLS): org A no ve las instancias de org B, ni por id,
//   - que los VALORES de secretos JAMAS aparecen en lecturas de instancia (solo
//     HasSecrets), y que LoadInstanceWithSecrets los abre solo del org activo.
package store_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// newCipher construye un Cipher con KEK aleatoria de 32 bytes.
func newCipher(t *testing.T) *keycrypto.Cipher {
	t.Helper()
	k := make([]byte, keycrypto.KeyLen)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("aleatoriedad: %v", err)
	}
	c, err := keycrypto.New(k)
	if err != nil {
		t.Fatalf("keycrypto.New: %v", err)
	}
	return c
}

// seedTemplate inserta una plantilla global (sin RLS) directamente.
func seedTemplate(t *testing.T, h *pgtest.Harness, key string) {
	t.Helper()
	if _, err := h.Pool.Exec(context.Background(),
		`INSERT INTO app_templates (key, manifest, tier, enabled) VALUES ($1, '{}'::jsonb, 'standard', true)`,
		key); err != nil {
		t.Fatalf("seed plantilla %q: %v", key, err)
	}
}

func TestMigrationsApplyRLS(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	ctx := context.Background()

	for _, tbl := range []string{"app_instances", "instance_secrets", "egress_audit"} {
		var enable, force bool
		err := h.Pool.QueryRow(ctx,
			`SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname = $1 AND relkind = 'r'`, tbl).
			Scan(&enable, &force)
		if err != nil {
			t.Fatalf("tabla %q no encontrada tras migrar: %v", tbl, err)
		}
		if !enable || !force {
			t.Fatalf("tabla %q sin RLS ENABLE+FORCE (enable=%v force=%v)", tbl, enable, force)
		}
	}
}

func TestOrgIsolation(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "demo")

	ctxA := tenantCtx("org-a")
	ctxB := tenantCtx("org-b")

	instA, err := st.CreateInstance(ctxA, "demo", "CRM de A", json.RawMessage(`{"region":"eu"}`), []string{"read"})
	if err != nil {
		t.Fatalf("A CreateInstance: %v", err)
	}
	if instA.Status != domain.StatusDraft || instA.OrgID != "org-a" {
		t.Fatalf("instancia inesperada: %+v", instA)
	}

	// Org B no ve la instancia de A (listado vacio).
	listB, err := st.ListInstances(ctxB)
	if err != nil {
		t.Fatalf("B ListInstances: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("aislamiento roto: B ve %d instancias de A", len(listB))
	}
	// Ni por id directo.
	if _, err := st.GetInstance(ctxB, instA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B obtiene la instancia de A (err=%v)", err)
	}
	// Ni puede borrarla.
	if err := st.DeleteInstance(ctxB, instA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B borra la instancia de A (err=%v)", err)
	}

	// A sigue viendo la suya.
	listA, err := st.ListInstances(ctxA)
	if err != nil {
		t.Fatalf("A ListInstances: %v", err)
	}
	if len(listA) != 1 || listA[0].ID != instA.ID {
		t.Fatalf("A perdio su instancia: %+v", listA)
	}
}

func TestSecretsNeverExposed(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "demo")

	ctxA := tenantCtx("org-a")
	ctxB := tenantCtx("org-b")

	inst, err := st.CreateInstance(ctxA, "demo", "con secretos", nil, nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if inst.HasSecrets {
		t.Fatal("una instancia recien creada no debe tener secretos")
	}

	const token = "super-secret-value"
	if err := st.PutSecrets(ctxA, inst.ID, map[string]string{"bearer": token, "api_key": "k2"}); err != nil {
		t.Fatalf("PutSecrets: %v", err)
	}

	// GetInstance refleja HasSecrets pero NUNCA el valor.
	got, err := st.GetInstance(ctxA, inst.ID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if !got.HasSecrets {
		t.Fatal("HasSecrets deberia ser true tras PutSecrets")
	}
	blob, _ := json.Marshal(got)
	if strings.Contains(string(blob), token) {
		t.Fatalf("el secreto se filtro en la lectura de instancia: %s", blob)
	}

	// La unica via que abre secretos los descifra correctamente, solo del org activo.
	full, err := st.LoadInstanceWithSecrets(ctxA, inst.ID)
	if err != nil {
		t.Fatalf("LoadInstanceWithSecrets: %v", err)
	}
	if full.Secrets["bearer"] != token || full.Secrets["api_key"] != "k2" {
		t.Fatalf("secretos descifrados incorrectos: %+v", full.Secrets)
	}

	// Org B no puede abrir ni escribir los secretos de A (RLS).
	if _, err := st.LoadInstanceWithSecrets(ctxB, inst.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B abre secretos de A (err=%v)", err)
	}
	if err := st.PutSecrets(ctxB, inst.ID, map[string]string{"bearer": "hijack"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B escribe secretos en la instancia de A (err=%v)", err)
	}

	// El valor sellado en la tabla NO esta en claro (defensa en profundidad). Va
	// dentro de InTenantTx porque el Pool del harness ya no tiene BYPASSRLS: sin el
	// GUC `app.tenant` la política esconde la fila. Sigue siendo SQL crudo, que es lo
	// que da valor a la comprobación: lee la columna tal cual está en disco.
	var enc string
	if err := h.Pool.InTenantTx(ctxA, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT value_enc FROM instance_secrets WHERE instance_id = $1 AND name = 'bearer'`, inst.ID).Scan(&enc)
	}); err != nil {
		t.Fatalf("leer value_enc: %v", err)
	}
	if strings.Contains(enc, token) || !strings.HasPrefix(enc, "encv1:") {
		t.Fatalf("el secreto no esta sellado at-rest: %q", enc)
	}
}

// TestMergeSecretsPreservaLosDemas: escribir una credencial NO puede llevarse por
// delante las otras de la misma conexión. Es el fallo que tenía la migración de
// Google: conectar por consentimiento guardaba el token y borraba la cuenta de
// servicio con delegación (y al revés), y la conexión dejaba de funcionar sin que
// nada lo dijera.
func TestMergeSecretsPreservaLosDemas(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "demo")

	ctxA := tenantCtx("org-a")
	ctxB := tenantCtx("org-b")
	inst, err := st.CreateInstance(ctxA, "demo", "google", nil, nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if err := st.PutSecrets(ctxA, inst.ID, map[string]string{"service_account_key": "sa-json"}); err != nil {
		t.Fatalf("PutSecrets: %v", err)
	}

	// El canje del consentimiento añade su token: la cuenta de servicio sigue ahí.
	if err := st.MergeSecrets(ctxA, inst.ID, map[string]string{"bearer": "tok-1", "oauth_refresh": "ref-1"}); err != nil {
		t.Fatalf("MergeSecrets: %v", err)
	}
	full, err := st.LoadInstanceWithSecrets(ctxA, inst.ID)
	if err != nil {
		t.Fatalf("LoadInstanceWithSecrets: %v", err)
	}
	if full.Secrets["service_account_key"] != "sa-json" || full.Secrets["bearer"] != "tok-1" {
		t.Fatalf("la fusión perdió credenciales: %+v", full.Secrets)
	}

	// Renovar el token reescribe SU clave y no toca las demás.
	if err := st.MergeSecrets(ctxA, inst.ID, map[string]string{"bearer": "tok-2"}); err != nil {
		t.Fatalf("MergeSecrets (renovación): %v", err)
	}
	full, err = st.LoadInstanceWithSecrets(ctxA, inst.ID)
	if err != nil {
		t.Fatalf("LoadInstanceWithSecrets: %v", err)
	}
	if full.Secrets["bearer"] != "tok-2" || full.Secrets["service_account_key"] != "sa-json" ||
		full.Secrets["oauth_refresh"] != "ref-1" {
		t.Fatalf("la renovación dejó el vault inconsistente: %+v", full.Secrets)
	}

	// Y sigue aislada por organización, como PutSecrets.
	if err := st.MergeSecrets(ctxB, inst.ID, map[string]string{"bearer": "hijack"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B fusiona secretos en la instancia de A (err=%v)", err)
	}
}
