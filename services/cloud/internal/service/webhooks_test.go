// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // se prueba la compatibilidad con la firma sha1 de GitHub.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func ghSig256(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func ghSig1(secret string, body []byte) string {
	m := hmac.New(sha1.New, []byte(secret))
	m.Write(body)
	return "sha1=" + hex.EncodeToString(m.Sum(nil))
}

func TestVerifyWebhookSignatureGitHub(t *testing.T) {
	secret := "s3cr3t"
	body := []byte(`{"ref":"refs/heads/main"}`)

	if err := VerifyWebhookSignature(secret, body, ghSig256(secret, body), ""); err != nil {
		t.Fatalf("sha256 válida rechazada: %v", err)
	}
	if err := VerifyWebhookSignature(secret, body, ghSig1(secret, body), ""); err != nil {
		t.Fatalf("sha1 válida rechazada: %v", err)
	}
	// Firma con secreto incorrecto.
	if err := VerifyWebhookSignature(secret, body, ghSig256("otro", body), ""); !errors.Is(err, domain.ErrSignatureInvalid) {
		t.Fatalf("firma inválida aceptada: %v", err)
	}
	// Cuerpo alterado tras firmar.
	if err := VerifyWebhookSignature(secret, append(body, '!'), ghSig256(secret, body), ""); !errors.Is(err, domain.ErrSignatureInvalid) {
		t.Fatalf("cuerpo manipulado aceptado: %v", err)
	}
	// Sin firma ni token.
	if err := VerifyWebhookSignature(secret, body, "", ""); !errors.Is(err, domain.ErrSignatureInvalid) {
		t.Fatalf("sin firma aceptado: %v", err)
	}
}

func TestVerifyWebhookSignatureGitLab(t *testing.T) {
	secret := "gitlab-token"
	body := []byte(`{}`)
	if err := VerifyWebhookSignature(secret, body, "", secret); err != nil {
		t.Fatalf("token GitLab válido rechazado: %v", err)
	}
	if err := VerifyWebhookSignature(secret, body, "", "otro-token"); !errors.Is(err, domain.ErrSignatureInvalid) {
		t.Fatalf("token GitLab incorrecto aceptado: %v", err)
	}
}

// Sin secreto se RECHAZA. Aceptar «porque es dev» convertía el endpoint público
// de webhooks en un disparador de builds y despliegues para cualquiera que
// adivinara la URL de un repo, y el secreto global estaba vacío en producción.
// Se comprueban también las dos variantes que un atacante intentaría: firma
// vacía y firma bien formada calculada con cualquier clave.
func TestVerifyWebhookSignatureSinSecretoRechaza(t *testing.T) {
	body := []byte("x")
	casos := map[string][2]string{
		"sin firma":       {"", ""},
		"firma inventada": {ghSig256("cualquiera", body), ""},
		"token de GitLab": {"", "cualquiera"},
	}
	for nombre, c := range casos {
		if err := VerifyWebhookSignature("", body, c[0], c[1]); !errors.Is(err, domain.ErrSignatureInvalid) {
			t.Fatalf("%s: sin secreto configurado NO se puede aceptar (err=%v)", nombre, err)
		}
	}
}

func TestParseWebhookGitHubPush(t *testing.T) {
	body := []byte(`{
		"ref":"refs/heads/main","after":"abc123","deleted":false,
		"repository":{"clone_url":"https://github.com/acme/app.git","html_url":"https://github.com/acme/app"},
		"pusher":{"name":"alice"},
		"head_commit":{"message":"fix: carrito con cantidad negativa"}}`)
	ev, err := ParseWebhook(domain.GitHub, "push", body)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if ev.RepoURL != "https://github.com/acme/app.git" || ev.CommitSHA != "abc123" ||
		ev.Ref != "refs/heads/main" || ev.Pusher != "alice" || ev.IsPullRequest() {
		t.Fatalf("evento push inesperado: %+v", ev)
	}
	if ev.CommitMessage != "fix: carrito con cantidad negativa" {
		t.Errorf("mensaje del commit = %q; el payload lo trae y se estaba tirando", ev.CommitMessage)
	}
}

func TestParseWebhookGitHubPR(t *testing.T) {
	body := []byte(`{
		"action":"opened","number":42,
		"pull_request":{"title":"Buscador con facetas","head":{"sha":"deadbeef","ref":"feature/x"}},
		"repository":{"clone_url":"https://github.com/acme/app.git"}}`)
	ev, err := ParseWebhook(domain.GitHub, "pull_request", body)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if !ev.IsPullRequest() || ev.PRNumber != 42 || ev.CommitSHA != "deadbeef" ||
		ev.Action != "opened" || ev.Ref != "feature/x" {
		t.Fatalf("evento PR inesperado: %+v", ev)
	}
	// Una preview representa la propuesta entera: la describe el título del PR.
	if ev.CommitMessage != "Buscador con facetas" {
		t.Errorf("título del PR = %q, want %q", ev.CommitMessage, "Buscador con facetas")
	}
}

func TestParseWebhookGitLabPushAndMR(t *testing.T) {
	// commits[] a propósito DESORDENADO: el que se despliega es checkout_sha, y
	// coger el último de la lista a ciegas daría el mensaje de otro commit.
	push := []byte(`{"ref":"refs/heads/main","checkout_sha":"c0ffee","user_name":"bob",
		"commits":[{"id":"c0ffee","message":"perf: cache del catálogo"},{"id":"otro","message":"wip"}],
		"project":{"git_http_url":"https://gitlab.com/acme/app.git"}}`)
	ev, err := ParseWebhook(domain.GitLab, "Push Hook", push)
	if err != nil {
		t.Fatalf("push GitLab: %v", err)
	}
	if ev.CommitSHA != "c0ffee" || ev.RepoURL != "https://gitlab.com/acme/app.git" || ev.IsPullRequest() {
		t.Fatalf("push GitLab inesperado: %+v", ev)
	}
	if ev.CommitMessage != "perf: cache del catálogo" {
		t.Errorf("mensaje GitLab = %q; se emparejó con el commit equivocado", ev.CommitMessage)
	}

	mr := []byte(`{"object_attributes":{"action":"open","iid":7,"source_branch":"feat","title":"Nuevo flujo de pago",
		"last_commit":{"id":"ababab"}},"project":{"git_http_url":"https://gitlab.com/acme/app.git"}}`)
	ev, err = ParseWebhook(domain.GitLab, "Merge Request Hook", mr)
	if err != nil {
		t.Fatalf("MR GitLab: %v", err)
	}
	if ev.PRNumber != 7 || ev.CommitSHA != "ababab" || ev.Ref != "feat" || ev.Action != "open" {
		t.Fatalf("MR GitLab inesperado: %+v", ev)
	}
	if ev.CommitMessage != "Nuevo flujo de pago" {
		t.Errorf("título del MR = %q, want %q", ev.CommitMessage, "Nuevo flujo de pago")
	}
}

func TestParseWebhookUnsupported(t *testing.T) {
	if _, err := ParseWebhook(domain.GitProvider("bitbucket"), "push", []byte(`{}`)); err == nil {
		t.Fatal("proveedor no soportado aceptado")
	}
	if _, err := ParseWebhook(domain.GitHub, "ping", []byte(`{}`)); err == nil {
		t.Fatal("evento GitHub ignorado no devolvió error")
	}
}

func TestBranchNameAndEventKind(t *testing.T) {
	if got := branchName("refs/heads/main"); got != "main" {
		t.Fatalf("branchName: %q", got)
	}
	if got := branchName("develop"); got != "develop" {
		t.Fatalf("branchName sin prefijo: %q", got)
	}
	if k := eventKind(domain.GitEvent{PRNumber: 3}); k != "pull_request" {
		t.Fatalf("eventKind PR: %q", k)
	}
	if k := eventKind(domain.GitEvent{}); k != "push" {
		t.Fatalf("eventKind push: %q", k)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty: %q", got)
	}
	if got := firstNonEmpty("", "  "); got != "" {
		t.Fatalf("firstNonEmpty vacío: %q", got)
	}
}
