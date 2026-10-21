// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SytueGit es el destino de los repositorios: la forja de la organización (Sytue
// Git, API de Gitea). No copia nada por aquí — le pide a la forja que IMPORTE el
// repositorio del origen, y es ella quien clona la historia completa y arrastra
// issues, pull requests, wiki, releases, etiquetas e hitos.
//
// Por qué así y no con un `git clone --mirror` propio: reimplementar la
// importación significaría reescribir también la traducción de issues y PRs entre
// dos forjas. Eso ya existe, probado, dentro del destino.
//
// Autenticación: un token de ADMINISTRADOR de la forja (configuración del
// servicio) más la cabecera `Sudo`, con la que cada repositorio se crea EN NOMBRE
// de la persona destino. El repositorio acaba siendo suyo, no de una cuenta de
// servicio compartida — igual que el resto de la migración escribe en la cuenta de
// cada cual y no en la de quien lanza el trabajo.
type SytueGit struct {
	base  string
	token string
	hc    *http.Client
}

// NewSytueGit construye el cliente de la forja. hc es opcional.
func NewSytueGit(baseURL, adminToken string, hc *http.Client) *SytueGit {
	if hc == nil {
		// La petición solo ENCOLA la importación en la forja; la copia la hace ella.
		hc = defaultSuiteClient()
	}
	return &SytueGit{base: strings.TrimRight(baseURL, "/"), token: adminToken, hc: hc}
}

// MigrateRepo importa un repositorio del origen a la forja, a nombre del usuario
// destino, y devuelve la ruta creada ("owner/nombre").
//
// Idempotente: si la ruta ya existe no se vuelve a importar (ni se machaca lo que
// haya). Un repositorio que la forja no puede traer —vacío, nombre ocupado— se
// devuelve como ErrUnimportable para que el informe lo registre como OMITIDO con
// su motivo, no como un error que se reintenta eternamente.
func (s *SytueGit) MigrateRepo(ctx context.Context, targetUser string, r Repo) (string, error) {
	login, err := s.login(ctx, targetUser)
	if err != nil {
		return "", err
	}
	owner := login
	if r.OwnerIsOrg && r.Owner != "" {
		owner = safeSlug(r.Owner)
		if err := s.ensureOrg(ctx, login, owner); err != nil {
			return "", err
		}
	}
	name := safeSlug(r.Name)
	path := owner + "/" + name

	exists, err := s.repoExists(ctx, login, owner, name)
	if err != nil {
		return "", err
	}
	if exists {
		return path, fmt.Errorf("%w: en Sytue Git ya hay un repositorio en %s, así que no se toca", ErrUnimportable, path)
	}

	body := map[string]any{
		"clone_addr":    r.CloneURL,
		"repo_owner":    owner,
		"repo_name":     name,
		"service":       "github",
		"auth_token":    r.AuthToken, // la forja clona con la credencial del origen.
		"description":   trimTo(r.Description, 255),
		"private":       r.Private,
		"mirror":        false, // una migración, no un espejo: el origen deja de mandar.
		"lfs":           true,
		"wiki":          true,
		"labels":        true,
		"issues":        true,
		"milestones":    true,
		"pull_requests": true,
		"releases":      true,
	}
	if err := s.call(ctx, http.MethodPost, "/api/v1/repos/migrate", login, body, nil); err != nil {
		return "", err
	}
	return path, nil
}

// login traduce a nombre de usuario en la forja la persona destino (su correo de
// Sytue). No se adivina de la parte local del correo: se pregunta a la forja.
//
// Que no exista NO es un fallo definitivo — la cuenta aparece la primera vez que
// esa persona entra en Sytue Git—, así que se devuelve como error normal con un
// mensaje accionable: quien lea el informe sabe qué hacer y reanudar basta.
func (s *SytueGit) login(ctx context.Context, targetUser string) (string, error) {
	email := strings.TrimSpace(targetUser)
	if email == "" {
		return "", fmt.Errorf("migrate: el trabajo no dice a quién pertenecen los repositorios en Sytue")
	}
	var out struct {
		Data []struct {
			Login string `json:"login"`
			Email string `json:"email"`
		} `json:"data"`
	}
	q := "?q=" + url.QueryEscape(email) + "&limit=10"
	if err := s.call(ctx, http.MethodGet, "/api/v1/users/search"+q, "", nil, &out); err != nil {
		return "", err
	}
	for _, u := range out.Data {
		if strings.EqualFold(u.Email, email) || strings.EqualFold(u.Login, email) {
			return u.Login, nil
		}
	}
	return "", fmt.Errorf(
		"migrate: %s todavía no tiene cuenta en Sytue Git; basta con que entre una vez con su cuenta de Sytue y luego reanudes este trabajo", email,
	)
}

// ensureOrg crea en la forja la organización equivalente a la del origen si no
// existe, con la persona destino como propietaria (Sudo). Así los repositorios de
// una organización de GitHub no acaban colgando de una cuenta personal.
func (s *SytueGit) ensureOrg(ctx context.Context, login, org string) error {
	err := s.call(ctx, http.MethodGet, "/api/v1/orgs/"+url.PathEscape(org), login, nil, nil)
	if err == nil {
		return nil
	}
	if !isNotFound(err) {
		return err
	}
	body := map[string]any{"username": org, "visibility": "private"}
	if err := s.call(ctx, http.MethodPost, "/api/v1/orgs", login, body, nil); err != nil {
		// Carrera con otro trabajo de la misma tanda: si ya existe, sirve igual.
		if isConflict(err) {
			return nil
		}
		return err
	}
	return nil
}

// repoExists dice si la ruta ya está ocupada en la forja.
func (s *SytueGit) repoExists(ctx context.Context, login, owner, name string) (bool, error) {
	err := s.call(ctx, http.MethodGet, "/api/v1/repos/"+url.PathEscape(owner)+"/"+url.PathEscape(name), login, nil, nil)
	switch {
	case err == nil:
		return true, nil
	case isNotFound(err):
		return false, nil
	default:
		return false, err
	}
}

// --- HTTP --------------------------------------------------------------------

// call emite una petición a la forja con el token de administrador y, si login no
// está vacío, actuando EN NOMBRE de esa persona (Sudo).
//
// Los 4xx que un reintento no arregla se traducen a ErrUnimportable con el mensaje
// de la forja dentro: es lo que acaba viendo quien lea el informe del trabajo.
func (s *SytueGit) call(ctx context.Context, method, path, login string, body, dst any) error {
	var payload io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+s.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if login != "" {
		req.Header.Set("Sudo", login)
	}

	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: %s %s: %w", method, redactPath(s.base+path), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		ge := &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue Git"}
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("migrate: Sytue Git rechazó la operación (HTTP %d): revisa el token de administrador de la forja configurado en la pasarela", resp.StatusCode)
		case http.StatusConflict, http.StatusUnprocessableEntity:
			// La forja dice que ESTE repositorio no se puede traer (vacío, nombre
			// ocupado). Reintentarlo daría siempre lo mismo.
			return fmt.Errorf("%w: %s", ErrUnimportable, ge.msg)
		default:
			return ge
		}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst); err != nil {
		return fmt.Errorf("migrate: respuesta de Sytue Git ilegible: %w", err)
	}
	return nil
}

// --- Helpers -----------------------------------------------------------------

// safeSlug deja un nombre válido para una ruta de la forja: letras, dígitos, '-',
// '_' y '.'. El nombre viene del origen, así que se sanea aquí.
func safeSlug(s string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(s))
	out = strings.Trim(out, "-.")
	if len(out) > 100 {
		out = out[:100]
	}
	if out == "" {
		return "repo"
	}
	return out
}

// trimTo acota un texto libre del origen al máximo que acepta el destino.
func trimTo(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

var _ RepoDestination = (*SytueGit)(nil)
