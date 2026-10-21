// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/crypto"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// --- Hash / verify de tokens de API ---

func TestGenerateAndHashAPIToken(t *testing.T) {
	plain, hashHex, err := GenerateAPIToken()
	if err != nil {
		t.Fatalf("GenerateAPIToken: %v", err)
	}
	if !strings.HasPrefix(plain, TokenPrefix) {
		t.Fatalf("token sin prefijo %q: %q", TokenPrefix, plain)
	}
	// El hash es determinista sobre el claro (clave de búsqueda) y de 64 hex (SHA-256).
	if got := HashAPIToken(plain); got != hashHex || len(got) != 64 {
		t.Fatalf("hash no determinista/longitud: got=%q want=%q", got, hashHex)
	}
	// El hash NO revela el claro y difiere de él.
	if hashHex == plain || strings.Contains(hashHex, strings.TrimPrefix(plain, TokenPrefix)) {
		t.Fatal("el hash no debe contener/igualar el secreto en claro")
	}
	// Dos tokens generados difieren (entropía real).
	plain2, hash2, _ := GenerateAPIToken()
	if plain == plain2 || hashHex == hash2 {
		t.Fatal("dos tokens generados no deben coincidir")
	}
}

func TestResolveAPITokenRejectsNonPrefixed(t *testing.T) {
	// Un Bearer sin el prefijo de token no debe llegar al store: se responde
	// ErrNotFound (401) sin consultar. resolveTokErr probaría que SÍ se consultó.
	fs := &fakeStore{resolveTokErr: errBoom}
	s := newTestService(fs, nil)
	if _, err := s.ResolveAPIToken(tenantCtx(t), "not-a-token"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("token sin prefijo debe dar ErrNotFound, got %v", err)
	}
	if _, err := s.ResolveAPIToken(tenantCtx(t), ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("token vacío debe dar ErrNotFound, got %v", err)
	}
	// Un token con prefijo sí consulta el store (aquí devuelve el error simulado).
	if _, err := s.ResolveAPIToken(tenantCtx(t), TokenPrefix+"abc"); !errors.Is(err, errBoom) {
		t.Fatalf("token con prefijo debe consultar el store, got %v", err)
	}
}

func TestScopeLadder(t *testing.T) {
	if !domain.ScopeDeploy.AtLeast(domain.ScopeWrite) || !domain.ScopeWrite.AtLeast(domain.ScopeRead) {
		t.Fatal("la escalera read<write<deploy no se respeta")
	}
	if domain.ScopeRead.AtLeast(domain.ScopeWrite) || domain.ScopeWrite.AtLeast(domain.ScopeDeploy) {
		t.Fatal("un scope inferior no debe alcanzar a uno superior")
	}
	if domain.TokenScope("bogus").AtLeast(domain.ScopeRead) {
		t.Fatal("un scope desconocido no alcanza ni read")
	}
}

func TestCreateAPITokenValidatesAndReturnsSecretOnce(t *testing.T) {
	fs := &fakeStore{apiTok: domain.APIToken{Name: "ci"}}
	s := newTestService(fs, nil)
	ctx := tenantCtx(t)

	if _, err := s.CreateAPIToken(ctx, CreateTokenInput{Name: "  "}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("nombre vacío debe fallar: %v", err)
	}
	if _, err := s.CreateAPIToken(ctx, CreateTokenInput{Name: "ci", Scope: "root"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("scope inválido debe fallar: %v", err)
	}
	// Un token hook sin proyecto publicaría todos los del tenant (CLOUD-17).
	if _, err := s.CreateAPIToken(ctx, CreateTokenInput{Name: "hook", Scope: domain.ScopeHook}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("hook sin proyecto debe fallar: %v", err)
	}
	past := time.Now().Add(-time.Hour)
	if _, err := s.CreateAPIToken(ctx, CreateTokenInput{Name: "ci", ExpiresAt: &past}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expiración pasada debe fallar: %v", err)
	}

	tok, err := s.CreateAPIToken(ctx, CreateTokenInput{Name: "ci", Scope: domain.ScopeDeploy})
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if tok.Plaintext == "" || !strings.HasPrefix(tok.Plaintext, TokenPrefix) {
		t.Fatalf("debe devolver el claro una vez: %q", tok.Plaintext)
	}
	// El store recibe SOLO el hash, nunca el claro.
	if fs.newTok == nil || fs.newTok.TokenHash != HashAPIToken(tok.Plaintext) {
		t.Fatalf("el store debe recibir el hash del claro: %+v", fs.newTok)
	}
	if fs.newTok.TokenHash == tok.Plaintext {
		t.Fatal("el store no debe recibir el secreto en claro")
	}
	if fs.newTok.Scope != domain.ScopeDeploy {
		t.Fatalf("scope no propagado: %q", fs.newTok.Scope)
	}
}

func TestCreateAPITokenProjectConfinement(t *testing.T) {
	ctx := tenantCtx(t)
	pid := uuid.New()

	// Proyecto de otro tenant / inexistente: GetProject bajo RLS devuelve ErrNotFound
	// => la emisión se rechaza y NADA se persiste.
	fs := &fakeStore{projectErr: domain.ErrNotFound}
	s := newTestService(fs, nil)
	if _, err := s.CreateAPIToken(ctx, CreateTokenInput{Name: "ci", ProjectID: &pid}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("project_id inexistente/de otro tenant debe rechazarse: %v", err)
	}
	if fs.newTok != nil {
		t.Fatalf("no debe persistirse un token con proyecto inválido: %+v", fs.newTok)
	}

	// Proyecto válido del tenant: se persiste con project_id propagado.
	fs = &fakeStore{apiTok: domain.APIToken{Name: "ci"}, project: domain.Project{ID: pid}}
	s = newTestService(fs, nil)
	if _, err := s.CreateAPIToken(ctx, CreateTokenInput{Name: "ci", ProjectID: &pid}); err != nil {
		t.Fatalf("project_id válido debe aceptarse: %v", err)
	}
	if fs.newTok == nil || fs.newTok.ProjectID == nil || *fs.newTok.ProjectID != pid {
		t.Fatalf("project_id no propagado al store: %+v", fs.newTok)
	}
}

// --- Resolución de rol efectivo ---

func TestEffectiveRole(t *testing.T) {
	// Con membership explícito manda el rol de la tabla.
	fs := &fakeStore{role: domain.RoleAdmin}
	s := newTestService(fs, nil)
	role, fromM, err := s.EffectiveRole(tenantCtx(t), "sub-1")
	if err != nil || !fromM || role != domain.RoleAdmin {
		t.Fatalf("membership explícito: role=%q fromM=%v err=%v", role, fromM, err)
	}

	// Sin membership (ErrNotFound) => fromMembership=false (el llamante cae al claim).
	fs = &fakeStore{roleErr: domain.ErrNotFound}
	s = newTestService(fs, nil)
	if _, fromM, err := s.EffectiveRole(tenantCtx(t), "sub-1"); err != nil || fromM {
		t.Fatalf("sin membership: fromM=%v err=%v", fromM, err)
	}

	// Un error de BD se propaga (no se degrada a "sin rol").
	fs = &fakeStore{roleErr: errBoom}
	s = newTestService(fs, nil)
	if _, _, err := s.EffectiveRole(tenantCtx(t), "sub-1"); !errors.Is(err, errBoom) {
		t.Fatalf("error de BD debe propagarse, got %v", err)
	}

	// Sub vacío no consulta el store.
	fs = &fakeStore{roleErr: errBoom}
	s = newTestService(fs, nil)
	if _, fromM, err := s.EffectiveRole(tenantCtx(t), "  "); err != nil || fromM {
		t.Fatalf("sub vacío: fromM=%v err=%v", fromM, err)
	}
}

func TestAddMemberValidatesRole(t *testing.T) {
	s := newTestService(&fakeStore{}, nil)
	if _, err := s.AddMember(tenantCtx(t), "sub-1", domain.Role("root")); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("rol inválido debe fallar: %v", err)
	}
	if _, err := s.AddMember(tenantCtx(t), "  ", domain.RoleMember); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("sub vacío debe fallar: %v", err)
	}
	fs := &fakeStore{membership: domain.Membership{Sub: "sub-1", Role: domain.RoleAdmin}}
	s = newTestService(fs, nil)
	if _, err := s.AddMember(tenantCtx(t), "sub-1", domain.RoleAdmin); err != nil {
		t.Fatalf("AddMember válido: %v", err)
	}
	if fs.upsertSub != "sub-1" || fs.upsertRole != domain.RoleAdmin {
		t.Fatalf("no propagado al store: sub=%q role=%q", fs.upsertSub, fs.upsertRole)
	}
}

// La guarda de «último owner» ya no se prueba aquí: dejó de ser una comprobación
// de la capa de servicio (donde solo podía ser un TOCTOU sobre un fake) y vive en
// el store, dentro de la transacción que escribe. Su test es
// store/lastowner_integration_test.go, contra PostgreSQL real y con la
// concurrencia que era el fallo de verdad.

// --- Verificación de firma de webhook POR PROYECTO ---

func TestVerifyWebhookPerProjectSecret(t *testing.T) {
	sealer, err := crypto.NewSealer(testKeyB64)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"ref":"refs/heads/main","after":"abc","repository":{"clone_url":"https://x/y.git"}}`)
	const projSecret = "per-project-secret"
	enc, err := sealer.SealString(projSecret)
	if err != nil {
		t.Fatal(err)
	}

	fs := &fakeStore{resolved: store.ResolvedRepo{Tenant: "acme", WebhookSecretEnc: enc}}
	s := newTestService(fs, nil)
	ctx := tenantCtx(t)

	// Firma correcta con el secreto DEL PROYECTO => válida.
	if _, err := s.VerifyWebhook(ctx, "https://x/y.git", body, ghSig256(projSecret, body), "", ""); err != nil {
		t.Fatalf("firma por proyecto válida rechazada: %v", err)
	}
	// Firma con el secreto global (u otro) NO vale contra el secreto del proyecto.
	if _, err := s.VerifyWebhook(ctx, "https://x/y.git", body, ghSig256("otro", body), "", ""); !errors.Is(err, domain.ErrSignatureInvalid) {
		t.Fatalf("firma de otro secreto aceptada: %v", err)
	}
	// SHA-1 legado del proyecto como alternativa.
	if _, err := s.VerifyWebhook(ctx, "https://x/y.git", body, "", ghSig1(projSecret, body), ""); err != nil {
		t.Fatalf("firma sha1 por proyecto válida rechazada: %v", err)
	}

	// Repo no conectado => ErrNotFound (el handler responde 202).
	fs2 := &fakeStore{resolveErr: domain.ErrNotFound}
	s2 := newTestService(fs2, nil)
	if _, err := s2.VerifyWebhook(ctx, "https://x/y.git", body, ghSig256(projSecret, body), "", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("repo no conectado: %v", err)
	}

	// Proyecto SIN secreto propio (enc nil) => fallback al secreto global de config.
	fs3 := &fakeStore{resolved: store.ResolvedRepo{Tenant: "acme"}}
	s3 := newTestService(fs3, nil)
	s3.cfg.WebhookSecret = "global-legacy"
	if _, err := s3.VerifyWebhook(ctx, "https://x/y.git", body, ghSig256("global-legacy", body), "", ""); err != nil {
		t.Fatalf("fallback a secreto global rechazado: %v", err)
	}
	if _, err := s3.VerifyWebhook(ctx, "https://x/y.git", body, ghSig256("mal", body), "", ""); !errors.Is(err, domain.ErrSignatureInvalid) {
		t.Fatalf("fallback: firma mala aceptada: %v", err)
	}
}
