// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/github"
)

// ponytail: DEFERIDO — GitLab (mismo patrón). Un project/group access token de
// GitLab clona por HTTPS (oauth2:<token>) y comenta la MR vía
// POST /projects/:id/merge_requests/:iid/notes; se conecta por (host, token) en
// vez de (app_id, installation). Al añadirlo: un cliente análogo en internal/
// gitlab + una rama por GitProvider aquí y en commentPreview. Sin repos GitLab
// privados conectados todavía, YAGNI hasta que haya demanda real.

// ConnectGitHubApp asociaría una instalación de la GitHub App al proyecto (para
// clonar su repo privado sin deploy-key manual y comentar la preview en el PR).
//
// FAIL-CLOSED por seguridad (review Wave 8): la App es UNA sola; su JWT puede
// acuñar un installation token para CUALQUIER instalación, y el installation_id
// es un entero enumerable NO secreto. Ligar un installation_id CRUDO del cliente
// es un confused-deputy cross-tenant: un tenant pondría repo_url + installation_id
// de OTRA org (ambos accesibles) y clonaría su código privado / comentaría sus PRs
// bajo la identidad de la App. requireRole solo verifica el rol en el PROPIO
// tenant, no la propiedad de la instalación; RLS no ayuda (el binding malicioso
// ocurre en el proyecto del atacante). Probar la propiedad exige la identidad del
// usuario que conecta: el flujo OAuth user-to-server de la GitHub App (callback de
// setup con installation_id + code → user token → GET /user/installations, exigir
// que el installation_id esté en la lista del usuario) — necesita GITHUB_APP_CLIENT_ID/
// SECRET + una App real para construirlo/verificarlo end-to-end.
//
// RESUELTO (Gap 3): la verificación de propiedad vive en ConnectGitHubAppWithOwnership,
// que el callback OAuth de instalación invoca. ConnectGitHubApp YA NO ES el punto de
// escritura: sigue FAIL-CLOSED (nunca persiste sin prueba de propiedad) y solo valida
// las precondiciones baratas (installation_id > 0 y repo github.com) para dar un error
// temprano y claro al cliente que envía el installation_id crudo. Ligar de verdad exige
// pasar por el callback con el `code` OAuth del usuario.
func (s *Service) ConnectGitHubApp(ctx context.Context, projectID uuid.UUID, installationID int64) error {
	if installationID <= 0 {
		return fmt.Errorf("%w: installation_id debe ser > 0", domain.ErrInvalidInput)
	}
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if _, _, ok := github.ParseRepo(proj.RepoURL); !ok {
		return fmt.Errorf("%w: el repositorio del proyecto no es de GitHub (github.com)", domain.ErrInvalidInput)
	}
	// FAIL-CLOSED: sin verificación de propiedad de la instalación (OAuth
	// user-to-server) NO se puede ligar de forma segura → se rechaza sin persistir.
	// El camino legítimo es el callback OAuth → ConnectGitHubAppWithOwnership.
	return fmt.Errorf("%w: conectar la GitHub App exige el flujo OAuth de propiedad de la instalación (callback de instalación con code)", domain.ErrPermission)
}

// ConnectGitHubAppWithOwnership liga una instalación de la GitHub App a un proyecto
// SOLO tras probar que la instalación PERTENECE al usuario que conecta. Es el único
// punto de escritura del binding, y lo invoca el callback OAuth de instalación.
//
// SEGURIDAD (cierre del confused-deputy cross-tenant): un installation_id es un
// entero enumerable NO secreto, así que requireRole (rol en el PROPIO tenant) y RLS
// no bastan — un tenant podría enviar repo_url + installation_id de OTRA org y clonar
// su código privado bajo la identidad de la App. La ÚNICA prueba válida es la
// identidad del usuario: se canjea su `code` OAuth por un user access token
// (ExchangeUserCode) y se exige que installationID ∈ GET /user/installations DE ESE
// usuario (UserInstallations). Si no pertenece → ErrPermission SIN persistir. Cualquier
// error de OAuth/red → FAIL-CLOSED (no persiste). Sin CLIENT_ID/SECRET o sin App
// configurada (s.gh nil) → ErrInvalidState (OAuth no disponible), sin persistir.
func (s *Service) ConnectGitHubAppWithOwnership(ctx context.Context, projectID uuid.UUID, installationID int64, code string) error {
	if installationID <= 0 {
		return fmt.Errorf("%w: installation_id debe ser > 0", domain.ErrInvalidInput)
	}
	proj, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if _, _, ok := github.ParseRepo(proj.RepoURL); !ok {
		return fmt.Errorf("%w: el repositorio del proyecto no es de GitHub (github.com)", domain.ErrInvalidInput)
	}
	// OAuth no configurado ⇒ no hay forma de probar propiedad ⇒ fail-closed.
	if s.gh == nil || s.cfg.GitHubAppClientID == "" || s.cfg.GitHubAppClientSecret == "" {
		return fmt.Errorf("%w: OAuth de la GitHub App no configurado (GITHUB_APP_CLIENT_ID/SECRET)", domain.ErrInvalidState)
	}

	// 1) code → user access token (identidad del usuario que instala/conecta).
	userToken, err := s.gh.ExchangeUserCode(ctx, s.cfg.GitHubAppClientID, s.cfg.GitHubAppClientSecret, code)
	if err != nil {
		// Fail-closed: cualquier fallo de canje ⇒ no se liga nada.
		return fmt.Errorf("%w: no se pudo verificar la propiedad de la instalación (OAuth)", domain.ErrPermission)
	}

	// 2) instalaciones a las que ESE usuario tiene acceso.
	installs, err := s.gh.UserInstallations(ctx, userToken)
	if err != nil {
		return fmt.Errorf("%w: no se pudo verificar la propiedad de la instalación (instalaciones del usuario)", domain.ErrPermission)
	}

	// 3) PROPIEDAD: la instalación DEBE estar entre las del usuario. Si no, rechaza
	// sin persistir (cierra el ligado de un installation_id ajeno).
	owned := false
	for _, id := range installs {
		if id == installationID {
			owned = true
			break
		}
	}
	if !owned {
		return fmt.Errorf("%w: la instalación %d no pertenece al usuario que conecta", domain.ErrPermission, installationID)
	}

	// Solo aquí, con propiedad probada, se persiste el binding.
	if err := s.store.SetProjectInstallation(ctx, projectID, installationID); err != nil {
		return err
	}
	s.audit(ctx, "github.connect", projectID.String(), map[string]any{"installation_id": installationID})
	return nil
}

// DisconnectGitHubApp desconecta la GitHub App del proyecto (vuelve al clone por
// deploy-key SSH o público). Idempotente.
func (s *Service) DisconnectGitHubApp(ctx context.Context, projectID uuid.UUID) error {
	if err := s.store.SetProjectInstallation(ctx, projectID, 0); err != nil {
		return err
	}
	s.audit(ctx, "github.disconnect", projectID.String(), nil)
	return nil
}

// githubInstallTTL: lo que vale el `state` de una instalación de la GitHub App.
const githubInstallTTL = 10 * time.Minute

type githubInstallIntent struct {
	Tenant    string    `json:"tenant"`
	ActorSub  string    `json:"actor_sub"`
	ProjectID uuid.UUID `json:"project_id"`
}

func githubInstallKey(state string) string { return "cloud:ghinstall:" + state }

// StartGitHubInstall emite el `state` para instalar la GitHub App en un proyecto.
//
// Antes el `state` era el id del proyecto: un enlace con el id de la víctima y el
// `code` del atacante ligaba la instalación del atacante a ese proyecto
// (CLOUD-18). Ahora es aleatorio, de un solo uso y atado a quien lo pidió, como
// en el alta por la forja (scaffold.go).
func (s *Service) StartGitHubInstall(ctx context.Context, projectID uuid.UUID, actorSub string) (string, error) {
	if !s.cache.Enabled() {
		return "", fmt.Errorf("%w: conectar GitHub no está disponible ahora", domain.ErrInvalidState)
	}
	if _, err := s.store.GetProject(ctx, projectID); err != nil {
		return "", err
	}
	raw, err := json.Marshal(githubInstallIntent{Tenant: tenancy.MustFrom(ctx).Slug, ActorSub: actorSub, ProjectID: projectID})
	if err != nil {
		return "", err
	}
	state, err := randomState()
	if err != nil {
		return "", err
	}
	if err := s.cache.SetTTL(ctx, githubInstallKey(state), string(raw), githubInstallTTL); err != nil {
		return "", fmt.Errorf("guardar el estado de la instalación: %w", err)
	}
	return state, nil
}

// ConsumeGitHubInstall canjea un `state` emitido por StartGitHubInstall por el
// proyecto al que va la instalación. Se consume aunque no valga.
func (s *Service) ConsumeGitHubInstall(ctx context.Context, state, actorSub string) (uuid.UUID, error) {
	if state == "" || !s.cache.Enabled() {
		return uuid.Nil, fmt.Errorf("%w: estado de instalación inválido", domain.ErrInvalidInput)
	}
	raw, found, err := s.cache.Get(ctx, githubInstallKey(state))
	if err != nil {
		return uuid.Nil, err
	}
	if !found {
		return uuid.Nil, fmt.Errorf("%w: la autorización caducó o ya se usó", domain.ErrInvalidInput)
	}
	_ = s.cache.Del(ctx, githubInstallKey(state))
	var in githubInstallIntent
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return uuid.Nil, err
	}
	if in.ActorSub != actorSub || in.Tenant != tenancy.MustFrom(ctx).Slug {
		return uuid.Nil, fmt.Errorf("%w: esta autorización la empezó otra persona", domain.ErrPermission)
	}
	return in.ProjectID, nil
}
