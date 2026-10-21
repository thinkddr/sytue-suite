// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// randKeyB64 genera una KEK AES-256 en base64.
func randKeyB64(t *testing.T) string {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(k)
}

// secretosSembrados son los cuatro sitios donde Sytue Cloud guarda algo cifrado.
// Si aparece un quinto y no se añade aquí, la rotación lo dejará atrás y retirar
// la clave vieja lo hará ilegible: por eso el test los enumera explícitamente.
type secretosSembrados struct {
	proyecto   uuid.UUID
	webhook    string // projects.webhook_secret_enc
	deployKey  string // projects.repo_creds_enc
	envSecreto string // env_vars.value_enc
	credsSvc   string // managed_services.creds_enc
}

// sembrarSecretos deja un secreto de cada tipo cifrado con el sellador dado.
func sembrarSecretos(t *testing.T, ctx context.Context, st *store.Store, tenant string, sealer *crypto.Sealer) secretosSembrados {
	t.Helper()
	s := secretosSembrados{
		webhook:    "test_webhook_prev",
		deployKey:  "-----BEGIN PRIVATE KEY-----\nfalsa\n-----END PRIVATE KEY-----",
		envSecreto: "postgres://app:s3cr3t@db:5432/x",
		credsSvc:   `{"dsn":"postgres://svc:pwd@db:5432/y"}`,
	}
	sellar := func(plain string) []byte {
		t.Helper()
		enc, err := sealer.SealString(plain)
		if err != nil {
			t.Fatalf("sellar %q: %v", plain, err)
		}
		return enc
	}

	p, err := st.CreateProject(ctx, store.NewProjectRecord{
		Slug: "web", Name: "web", GitProvider: domain.GitGeneric,
		ProductionRef: "main", Framework: domain.FrameworkDetect,
		WebhookSecretEnc: sellar(s.webhook),
	}, 0)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	s.proyecto = p.ID

	creds := sellar(s.deployKey)
	if _, err := st.UpdateProject(ctx, p.ID, store.UpdateProjectRecord{RepoCredsEnc: &creds}); err != nil {
		t.Fatalf("UpdateProject(deploy key): %v", err)
	}

	if _, err := st.UpsertEnvVar(ctx, store.EnvVarRecord{
		ProjectID: p.ID, Key: "DATABASE_URL", IsSecret: true, ValueEnc: sellar(s.envSecreto),
	}); err != nil {
		t.Fatalf("UpsertEnvVar: %v", err)
	}

	m, err := st.ProvisionService(ctx, store.NewServiceRecord{
		ProjectID: p.ID, Kind: domain.ServicePostgres, Name: "db",
	}, 0)
	if err != nil {
		t.Fatalf("ProvisionService: %v", err)
	}
	if err := st.MarkServiceReady(ctx, tenant, m.ID, map[string]string{"host": "db"}, sellar(s.credsSvc)); err != nil {
		t.Fatalf("MarkServiceReady: %v", err)
	}
	return s
}

// comprobarLegibles lee los cuatro secretos por los mismos caminos que el
// controlador/builder en producción y exige que el sellador dado los abra.
func comprobarLegibles(t *testing.T, ctx context.Context, st *store.Store, tenant string, sealer *crypto.Sealer, s secretosSembrados, fase string) {
	t.Helper()
	abrir := func(qué string, enc []byte, quiero string) {
		t.Helper()
		got, err := sealer.OpenString(enc)
		if err != nil {
			t.Fatalf("%s: %s ilegible: %v", fase, qué, err)
		}
		if got != quiero {
			t.Fatalf("%s: %s = %q, se esperaba %q", fase, qué, got, quiero)
		}
	}

	enc, err := st.GetProjectWebhookSecretEnc(ctx, s.proyecto)
	if err != nil {
		t.Fatalf("%s: GetProjectWebhookSecretEnc: %v", fase, err)
	}
	abrir("secreto de webhook", enc, s.webhook)

	enc, err = st.GetProjectRepoCredsEnc(ctx, tenant, s.proyecto)
	if err != nil {
		t.Fatalf("%s: GetProjectRepoCredsEnc: %v", fase, err)
	}
	abrir("deploy key", enc, s.deployKey)

	vars, err := st.ResolveEnvForDeploy(ctx, tenant, s.proyecto, domain.TargetProduction)
	if err != nil {
		t.Fatalf("%s: ResolveEnvForDeploy: %v", fase, err)
	}
	if len(vars) != 1 || !vars[0].IsSecret {
		t.Fatalf("%s: se esperaba 1 variable secreta, hay %d", fase, len(vars))
	}
	abrir("variable secreta", vars[0].ValueEnc, s.envSecreto)

	creds, err := st.ResolveServiceCreds(ctx, tenant, s.proyecto, domain.TargetProduction)
	if err != nil {
		t.Fatalf("%s: ResolveServiceCreds: %v", fase, err)
	}
	abrir("credenciales del servicio", creds[string(domain.ServicePostgres)], s.credsSvc)
}

// TestRotateSecretKeys recorre una rotación de KEK COMPLETA contra PostgreSQL real:
// se siembra un secreto de cada clase con la clave 1, se añade la 2 al anillo, se
// re-sella en caliente y se retira la 1. Lo que se exige en cada fase es que NINGUNA
// lectura falle — es la única prueba de que la rotación no necesita ventana de
// parada— y que al final la clave vieja ya no haga falta.
//
// Corre con el rol de aplicación de pgtest, SIN BYPASSRLS: el re-sellado es
// cross-tenant, así que también demuestra que las políticas de mantenimiento (la de
// env_vars llega en la 0025) están puestas. Sin ellas el UPDATE afectaría a 0 filas
// y el censo se quedaría clavado en la versión vieja.
func TestRotateSecretKeys(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	const tenant = "acme"
	ctx := tenantCtx(tenant)

	k1, k2 := randKeyB64(t), randKeyB64(t)

	// Fase 1 — antes de rotar: una sola clave, formato histórico (⇒ versión 1).
	vieja, err := crypto.NewSealer(k1)
	if err != nil {
		t.Fatalf("NewSealer(k1): %v", err)
	}
	s := sembrarSecretos(t, ctx, st, tenant, vieja)
	comprobarLegibles(t, ctx, st, tenant, vieja, s, "antes de rotar")

	if censo := censoDeClaves(t, st); censo[1] != 4 {
		t.Fatalf("antes de rotar: censo %v, se esperaban 4 blobs en la versión 1", censo)
	}
	// Con una sola clave no hay nada que rotar: la pasada no debe escribir nada.
	if stats, err := st.RotateSecretKeys(context.Background(), vieja.Version(), vieja.Reseal); err != nil || stats.Total() != 0 {
		t.Fatalf("rotación sin clave nueva: stats %+v err %v (se esperaba no-op)", stats, err)
	}

	// Fase 2 — clave nueva AÑADIDA al anillo, aún sin re-sellar. Es el momento
	// crítico: todo sigue cifrado con la 1 y todo tiene que leerse igual.
	anillo, err := crypto.NewSealer("1:" + k1 + ",2:" + k2)
	if err != nil {
		t.Fatalf("NewSealer(anillo): %v", err)
	}
	if anillo.Version() != 2 {
		t.Fatalf("anillo: primaria %d, se esperaba 2", anillo.Version())
	}
	comprobarLegibles(t, ctx, st, tenant, anillo, s, "con la clave nueva añadida")

	// Fase 3 — re-sellado incremental. Debe tocar las 3 tablas.
	stats, err := st.RotateSecretKeys(context.Background(), anillo.Version(), anillo.Reseal)
	if err != nil {
		t.Fatalf("RotateSecretKeys: %v", err)
	}
	if stats.EnvVars != 1 || stats.Services != 1 || stats.Projects != 1 {
		t.Fatalf("re-sellado: stats %+v, se esperaba 1 fila por tabla", stats)
	}
	comprobarLegibles(t, ctx, st, tenant, anillo, s, "recién re-sellado")

	censo := censoDeClaves(t, st)
	if censo[2] != 4 || censo[1] != 0 {
		t.Fatalf("tras re-sellar: censo %v, se esperaban los 4 blobs en la versión 2", censo)
	}

	// Idempotencia: repetir la pasada no vuelve a escribir (si escribiera, un cron
	// horario estaría re-cifrando la base entera para siempre).
	if again, err := st.RotateSecretKeys(context.Background(), anillo.Version(), anillo.Reseal); err != nil || again.Total() != 0 {
		t.Fatalf("segunda pasada: stats %+v err %v (se esperaba no-op)", again, err)
	}

	// Fase 4 — clave vieja RETIRADA del anillo. Si el re-sellado se hubiera dejado
	// un solo blob, aquí falla la lectura: eso es exactamente lo que se quiere.
	nueva, err := crypto.NewSealer("2:" + k2)
	if err != nil {
		t.Fatalf("NewSealer(solo k2): %v", err)
	}
	comprobarLegibles(t, ctx, st, tenant, nueva, s, "con la clave vieja retirada")

	// Y lo que se escriba a partir de ahora se cifra con la 2 y también se lee.
	nuevoEnc, err := nueva.SealString("valor-post-rotacion")
	if err != nil {
		t.Fatalf("sellar tras rotar: %v", err)
	}
	if _, err := st.UpsertEnvVar(ctx, store.EnvVarRecord{
		ProjectID: s.proyecto, Key: "API_KEY", IsSecret: true, ValueEnc: nuevoEnc,
	}); err != nil {
		t.Fatalf("UpsertEnvVar tras rotar: %v", err)
	}
	// La fila nace con key_version=1 (DEFAULT) aunque ya esté cifrada con la 2:
	// la columna es una cota inferior. La siguiente pasada la re-sella sin efecto
	// real y deja el censo limpio — que es lo que mira quien decide retirar claves.
	if _, err := st.RotateSecretKeys(context.Background(), nueva.Version(), nueva.Reseal); err != nil {
		t.Fatalf("pasada tras escribir con la clave nueva: %v", err)
	}
	if censo := censoDeClaves(t, st); censo[2] != 5 || censo[1] != 0 {
		t.Fatalf("censo final %v, se esperaban 5 blobs en la versión 2 y ninguno en la 1", censo)
	}
}

func censoDeClaves(t *testing.T, st *store.Store) map[int]int64 {
	t.Helper()
	censo, err := st.SecretKeyVersions(context.Background())
	if err != nil {
		t.Fatalf("SecretKeyVersions: %v", err)
	}
	return censo
}
