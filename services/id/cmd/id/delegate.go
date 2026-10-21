// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	iddb "git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	idhttp "git.sytue.eu/sytue/one/services/id/internal/http"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Aprovisionamiento del client DELEGABLE de una organización — OPERACIÓN DE
// PLATAFORMA, fuera de banda.
//
// Es el único camino por el que `may_impersonate` se activa (mig 0025): ninguna
// API lo escribe, ni el registro dinámico, ni el CRUD admin de la Console. Vive
// AQUÍ, en el binario, y no en el store, para que la propiedad sea verificable de
// un vistazo: `grep -r may_impersonate services/id/internal` solo encuentra
// LECTURAS. Un handler HTTP no puede llamar a esto ni por accidente.
//
// Por qué no un endpoint: si un admin de organización pudiera marcarse un client
// como delegable desde la Console, el robo de UNA sesión de admin se convertiría
// en impersonación silenciosa de toda la organización — justo la escalada que el
// canje existe para impedir. Ejecutar esto exige acceso al despliegue y a la base
// de datos (el control de cambios del operador), que es un nivel de credencial
// distinto del de cualquier usuario de la suite.
//
// El client resultante SIEMPRE está acotado a una organización (el CHECK de la
// mig 0025 lo exige, y aquí se pasa su org_id): no existe la variante global.
const (
	// delegatedClientPrefix es el prefijo del id del client delegable de cada org.
	// Un id derivado del slug hace el aprovisionamiento IDEMPOTENTE (re-ejecutar
	// solo rota el secreto) y deja evidente en el audit_log quién actuó.
	delegatedClientPrefix = "sytue-migrate-"
	// delegatedSecretBytes es la entropía del secreto generado (256 bits).
	delegatedSecretBytes = 32
)

// delegatedClientSpec describe el client delegable de una organización. Los scopes
// son los MÍNIMOS para escribir en la cuenta de una persona (identidad + perfil):
// el techo real del token canjeado lo pone domain.ImpersonationScope, que además
// quita offline_access y los scopes de máquina. Sin `admin` y sin
// may_impersonate_admins: migrar ficheros y contactos no necesita actuar como
// administrador de la organización.
func delegatedClientSpec(orgSlug string) domain.Client {
	return domain.Client{
		ID:                      delegatedClientPrefix + orgSlug,
		Name:                    "Migración de contenido — " + orgSlug,
		Type:                    domain.ClientConfidential,
		GrantTypes:              []string{"client_credentials", domain.GrantTokenExchange},
		ResponseTypes:           []string{},
		Scopes:                  []string{"openid", "profile", "email"},
		RedirectURIs:            []string{},
		PostLogoutRedirectURIs:  []string{},
		TokenEndpointAuthMethod: "client_secret_post",
	}
}

// runDelegatedClient es el punto de entrada del subcomando `delegated-client`:
//
//	id delegated-client <org-slug>
//
// Crea (o rota el secreto de) el client delegable de esa organización y ESCRIBE
// EL SECRETO POR SALIDA ESTÁNDAR una sola vez: solo se guarda su hash Argon2id.
// El operador lo instala después en el Sytue Integration Gateway, que lo sella
// con su KEK. Devuelve el código de salida del proceso.
func runDelegatedClient(args []string) int {
	slug := ""
	if len(args) > 0 {
		slug = strings.TrimSpace(args[0])
	}
	if slug == "" {
		fmt.Fprintln(os.Stderr, "uso: id delegated-client <org-slug>")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := idhttp.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "delegated-client: configuración inválida:", err)
		return 1
	}
	clientID, secret, err := provisionDelegatedClient(ctx, cfg.DatabaseDSN, slug)
	if err != nil {
		fmt.Fprintln(os.Stderr, "delegated-client:", err)
		return 1
	}

	fmt.Printf("\n"+
		"────────────────────────────────────────────────────────────\n"+
		" Client DELEGABLE aprovisionado para la organización %q\n"+
		"   client_id     : %s\n"+
		"   client_secret : %s\n"+
		"\n"+
		" El secreto NO se vuelve a mostrar (solo se guarda su hash).\n"+
		" Instálalo en el Sytue Integration Gateway, que lo sella con su KEK. /operator\n"+
		" no se publica en el Ingress: se llega desde dentro del clúster.\n"+
		"\n"+
		"   kubectl -n sytue port-forward svc/sytue-one-integrations 8093:80 &\n"+
		"   curl -X PUT http://127.0.0.1:8093/operator/delegated-client \\\n"+
		"     -H \"Authorization: Bearer $SYTUE_INTEGRATIONS_OPERATOR_TOKEN\" \\\n"+
		"     -H 'Content-Type: application/json' \\\n"+
		"     -d '{\"org\":\"%s\",\"client_id\":\"%s\",\"client_secret\":\"…\"}'\n"+
		"────────────────────────────────────────────────────────────\n\n",
		slug, clientID, secret, slug, clientID)
	return 0
}

// provisionDelegatedClient aplica las migraciones (idempotentes), resuelve la
// organización y hace upsert del client delegable con un secreto NUEVO. Devuelve
// el id del client y el secreto en claro (única vez que existe fuera de memoria).
func provisionDelegatedClient(ctx context.Context, dsn, orgSlug string) (clientID, secret string, err error) {
	if err := pg.MigrateUp(iddb.Migrations, iddb.MigrationsDir, dsn); err != nil {
		return "", "", fmt.Errorf("migraciones: %w", err)
	}
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 2})
	if err != nil {
		return "", "", fmt.Errorf("conexión: %w", err)
	}
	defer pool.Close()

	st := store.New(pool)
	org, err := st.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", "", fmt.Errorf("la organización %q no existe", orgSlug)
		}
		return "", "", fmt.Errorf("buscar organización %q: %w", orgSlug, err)
	}

	secret, err = domain.GenerateToken(delegatedSecretBytes)
	if err != nil {
		return "", "", fmt.Errorf("generar secreto: %w", err)
	}
	hash, err := domain.HashPassword(secret)
	if err != nil {
		return "", "", fmt.Errorf("hashear secreto: %w", err)
	}

	c := delegatedClientSpec(orgSlug)
	// El UPSERT reafirma org_id y may_impersonate en cada pasada: un client que
	// alguien hubiera desmarcado vuelve a quedar como esta operación declara, y
	// re-ejecutar solo ROTA el secreto. may_impersonate_admins se deja en false
	// EXPLÍCITAMENTE: actuar como administrador se concede aparte, nunca aquí.
	const sql = `
		INSERT INTO oauth_clients
		    (id, secret_hash, name, type, redirect_uris, post_logout_redirect_uris,
		     grant_types, response_types, scopes, require_pkce, token_endpoint_auth_method,
		     org_id, managed_by, may_impersonate, may_impersonate_admins)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, false, $10, $11::uuid, 'admin', true, false)
		ON CONFLICT (id) DO UPDATE
		   SET secret_hash = EXCLUDED.secret_hash,
		       grant_types = EXCLUDED.grant_types,
		       scopes = EXCLUDED.scopes,
		       token_endpoint_auth_method = EXCLUDED.token_endpoint_auth_method,
		       org_id = EXCLUDED.org_id,
		       may_impersonate = true,
		       may_impersonate_admins = false,
		       updated_at = now()`
	if _, err := st.DB().Exec(ctx, sql, c.ID, hash, c.Name, string(c.Type),
		c.RedirectURIs, c.PostLogoutRedirectURIs, c.GrantTypes, c.ResponseTypes, c.Scopes,
		c.TokenEndpointAuthMethod, org.ID); err != nil {
		return "", "", fmt.Errorf("aprovisionar client delegable: %w", err)
	}
	return c.ID, secret, nil
}
