// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/github"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

func TestConnectGitHubAppValidatesInstallationAndRepo(t *testing.T) {
	ctx := tenantCtx(t)

	// installation id <= 0 → inválido (no persiste nada).
	s := newTestService(&fakeStore{project: domain.Project{RepoURL: "https://github.com/o/r"}}, nil)
	if err := s.ConnectGitHubApp(ctx, uuid.New(), 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("installation<=0 debe ser ErrInvalidInput: %v", err)
	}

	// repo no-GitHub → inválido (la App solo cubre github.com).
	s = newTestService(&fakeStore{project: domain.Project{RepoURL: "https://gitlab.com/o/r"}}, nil)
	if err := s.ConnectGitHubApp(ctx, uuid.New(), 123); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("repo no-GitHub debe ser ErrInvalidInput: %v", err)
	}

	// Con instalación y repo GitHub válidos, el connect está FAIL-CLOSED (sin
	// verificación de propiedad OAuth): rechaza con ErrPermission y NO persiste —
	// cierra el confused-deputy cross-tenant de ligar un installation_id ajeno.
	fs := &fakeStore{project: domain.Project{RepoURL: "https://github.com/o/r"}}
	s = newTestService(fs, nil)
	if err := s.ConnectGitHubApp(ctx, uuid.New(), 999); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("connect debe estar fail-closed (ErrPermission): %v", err)
	}
	if fs.setInstallID != 0 {
		t.Fatalf("connect fail-closed NO debe persistir la instalación: %d", fs.setInstallID)
	}

	// Desconectar sigue disponible (seguro): llama a SetProjectInstallation(0).
	if err := s.DisconnectGitHubApp(ctx, uuid.New()); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if fs.setInstallID != 0 {
		t.Fatalf("disconnect debe dejar la instalación en 0: %d", fs.setInstallID)
	}
}

// ghOAuthServer simula los dos endpoints del flujo de propiedad: el canje de code
// (github.com/login/oauth/access_token) y las instalaciones del usuario
// (api.github.com/user/installations). userInstalls son los IDs que el user token
// "posee". failExchange/failInstalls fuerzan un fallo de red (500) en cada paso.
func ghOAuthServer(t *testing.T, userInstalls []int64, failExchange, failInstalls bool) *github.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			if failExchange {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"ghu_user"}`))
		case "/user/installations":
			if failInstalls {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			body := `{"total_count":` + strconv.Itoa(len(userInstalls)) + `,"installations":[`
			for i, id := range userInstalls {
				if i > 0 {
					body += ","
				}
				body += `{"id":` + strconv.FormatInt(id, 10) + `}`
			}
			body += `]}`
			_, _ = w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generar clave: %v", err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	c, err := github.NewClient(1234, pemStr, github.WithBaseURLs(srv.URL, srv.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestConnectGitHubAppWithOwnership(t *testing.T) {
	ctx := tenantCtx(t)
	ghRepo := domain.Project{RepoURL: "https://github.com/o/r"}

	withOAuthCfg := func(s *Service) *Service {
		s.cfg.GitHubAppClientID = "Iv1.cid"
		s.cfg.GitHubAppClientSecret = "secret"
		return s
	}

	// PROPIEDAD OK: la instalación 999 está entre las del usuario → liga + persiste.
	fs := &fakeStore{project: ghRepo}
	s := withOAuthCfg(newTestService(fs, nil))
	s.SetGitHub(ghOAuthServer(t, []int64{111, 999}, false, false))
	if err := s.ConnectGitHubAppWithOwnership(ctx, uuid.New(), 999, "code"); err != nil {
		t.Fatalf("propiedad válida debe ligar: %v", err)
	}
	if fs.setInstallID != 999 {
		t.Fatalf("debe persistir la instalación 999: %d", fs.setInstallID)
	}

	// NO PERTENECE: la instalación 999 NO está entre las del usuario → ErrPermission
	// SIN persistir (cierre del confused-deputy cross-tenant).
	fs = &fakeStore{project: ghRepo}
	s = withOAuthCfg(newTestService(fs, nil))
	s.SetGitHub(ghOAuthServer(t, []int64{111, 222}, false, false))
	if err := s.ConnectGitHubAppWithOwnership(ctx, uuid.New(), 999, "code"); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("instalación ajena debe ser ErrPermission: %v", err)
	}
	if fs.setInstallID != 0 {
		t.Fatalf("instalación ajena NO debe persistir: %d", fs.setInstallID)
	}

	// SIN CLIENT_ID/SECRET → ErrInvalidState sin persistir (OAuth no configurado).
	fs = &fakeStore{project: ghRepo}
	s = newTestService(fs, nil) // sin withOAuthCfg
	s.SetGitHub(ghOAuthServer(t, []int64{999}, false, false))
	if err := s.ConnectGitHubAppWithOwnership(ctx, uuid.New(), 999, "code"); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("sin CLIENT_ID/SECRET debe ser ErrInvalidState: %v", err)
	}
	if fs.setInstallID != 0 {
		t.Fatalf("sin OAuth NO debe persistir: %d", fs.setInstallID)
	}

	// gh nil (App no configurada) → ErrInvalidState sin persistir.
	fs = &fakeStore{project: ghRepo}
	s = withOAuthCfg(newTestService(fs, nil)) // cfg OK pero gh nil
	if err := s.ConnectGitHubAppWithOwnership(ctx, uuid.New(), 999, "code"); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("gh nil debe ser ErrInvalidState: %v", err)
	}
	if fs.setInstallID != 0 {
		t.Fatalf("gh nil NO debe persistir: %d", fs.setInstallID)
	}

	// ERROR DE RED en el canje del code → fail-closed (ErrPermission) sin persistir.
	fs = &fakeStore{project: ghRepo}
	s = withOAuthCfg(newTestService(fs, nil))
	s.SetGitHub(ghOAuthServer(t, []int64{999}, true, false))
	if err := s.ConnectGitHubAppWithOwnership(ctx, uuid.New(), 999, "code"); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("fallo de canje debe ser fail-closed (ErrPermission): %v", err)
	}
	if fs.setInstallID != 0 {
		t.Fatalf("fallo de canje NO debe persistir: %d", fs.setInstallID)
	}

	// ERROR DE RED al listar instalaciones → fail-closed sin persistir.
	fs = &fakeStore{project: ghRepo}
	s = withOAuthCfg(newTestService(fs, nil))
	s.SetGitHub(ghOAuthServer(t, []int64{999}, false, true))
	if err := s.ConnectGitHubAppWithOwnership(ctx, uuid.New(), 999, "code"); !errors.Is(err, domain.ErrPermission) {
		t.Fatalf("fallo al listar instalaciones debe ser fail-closed: %v", err)
	}
	if fs.setInstallID != 0 {
		t.Fatalf("fallo al listar NO debe persistir: %d", fs.setInstallID)
	}

	// installation_id <= 0 → ErrInvalidInput; repo no-GitHub → ErrInvalidInput.
	s = withOAuthCfg(newTestService(&fakeStore{project: ghRepo}, nil))
	s.SetGitHub(ghOAuthServer(t, []int64{999}, false, false))
	if err := s.ConnectGitHubAppWithOwnership(ctx, uuid.New(), 0, "code"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("installation<=0 debe ser ErrInvalidInput: %v", err)
	}
	s = withOAuthCfg(newTestService(&fakeStore{project: domain.Project{RepoURL: "https://gitlab.com/o/r"}}, nil))
	s.SetGitHub(ghOAuthServer(t, []int64{999}, false, false))
	if err := s.ConnectGitHubAppWithOwnership(ctx, uuid.New(), 999, "code"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("repo no-GitHub debe ser ErrInvalidInput: %v", err)
	}
}

// TestCommentPreviewNoopWhenDisabled: sin GitHub App (gh nil) el comentario de
// preview es un no-op y no toca el store (no panic aunque st sea nil).
func TestCommentPreviewNoopWhenDisabled(t *testing.T) {
	n := &DeployNotifier{} // gh nil, st nil
	for _, ev := range []events.Event{
		{Type: store.EventDeployReady, Tenant: "acme"},
		{Type: store.EventDeployError, Tenant: "acme"},
	} {
		if err := n.commentPreview(context.Background(), ev); err != nil {
			t.Fatalf("commentPreview no-op debe devolver nil: %v", err)
		}
	}
}
