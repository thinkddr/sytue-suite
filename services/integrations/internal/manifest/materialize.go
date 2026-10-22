// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package manifest

import (
	"context"
	"fmt"
)

// Claves internas del gateway en la Config PÚBLICA de una instancia. El prefijo '_' es
// imposible en una clave de campo de admin (fieldKeyRe exige inicial [a-z]), así que
// nunca colisionan. No son secretos: son ids de recursos en Sytue ID + la URL de
// lanzamiento (aptos para el tile). El client_secret OIDC NO se guarda aquí (se muestra
// una vez y lo custodia el SaaS).
const (
	KeySSOSPID      = "_sso_sp_id"      // id del SP SAML creado en Sytue ID
	KeyOIDCClientID = "_oidc_client_id" // client_id del client OIDC registrado por DCR
	KeyLaunchURL    = "_launch_url"     // href resuelto del tile («abrir» la app)
)

// SAMLSPInput/SAMLSP y OIDCClientInput/OIDCClient son el contrato con Sytue ID (W2A)
// que materialize necesita, desacoplado del transporte HTTP (directory.Client lo
// implementa; los tests lo imitan con un id-mock httptest).
type SAMLSPInput struct {
	Name         string
	EntityID     string
	ACSURL       string
	NameIDFormat string
}

// SAMLSP es el SP creado: su id en Sytue ID, la URL IdP-initiated (1-click del portal)
// y la URL de metadata del IdP (que el admin registra en el SP de terceros).
type SAMLSP struct {
	ID          string
	InitiateURL string
	MetadataURL string
}

// OIDCClientInput son los metadatos con los que la app se registra por DCR (RFC 7591).
type OIDCClientInput struct {
	Name         string
	RedirectURIs []string
}

// OIDCClient es el client registrado: client_id + client_secret (este último se emite
// UNA vez).
type OIDCClient struct {
	ClientID     string
	ClientSecret string
}

// IDApps es la superficie admin de Sytue ID que materialize consume para dar de alta la
// identidad SALIENTE de una app: registrar/borrar un SP SAML y registrar un client OIDC.
// La implementa directory.Client (con el token client_credentials de `sytue-gateway`).
type IDApps interface {
	CreateSAMLSP(ctx context.Context, org string, in SAMLSPInput) (SAMLSP, error)
	DeleteSAMLSP(ctx context.Context, org, spID string) error
	CreateOIDCClient(ctx context.Context, org string, in OIDCClientInput) (OIDCClient, error)
}

// Outcome es el resultado de materializar: las claves internas a fusionar en la config
// de la instancia (ids + launch), el secreto OIDC a mostrar una vez, y si hay que
// agendar el primer sync SCIM.
type Outcome struct {
	Config           map[string]string
	OIDCClientSecret string
	LaunchURL        string
	EnqueueSCIM      bool
	// SAMLMetadataURL es transitorio (no se persiste): la URL de metadata del IdP a
	// mostrar UNA vez para que el admin la registre en el SP de terceros. "" salvo en la
	// primera activación de una identidad SAML.
	SAMLMetadataURL string
}

// noopRollback es el rollback cuando no se creó nada en id (identidad none/OIDC, o
// reactivación idempotente): no hay recurso que deshacer.
func noopRollback(context.Context) error { return nil }

// Materialize crea en Sytue ID los recursos de identidad del manifiesto para esta
// instancia y resuelve la URL de lanzamiento del tile. Es IDEMPOTENTE: si la config ya
// trae el id del SP/client (una reactivación), no recrea nada. Devuelve el Outcome y un
// ROLLBACK best-effort que deshace lo recién creado en id —para revertir si la
// persistencia posterior falla y no dejar un SP huérfano—. `cfg` es la config pública
// de la instancia (valores de admin + claves internas de una activación previa).
//
//nolint:gocognit,gocyclo // valida y materializa el manifiesto campo a campo; cada rama es una regla del contrato de conector.
func Materialize(ctx context.Context, ids IDApps, org string, m *Manifest, cfg map[string]string) (Outcome, func(context.Context) error, error) {
	if cfg == nil {
		cfg = map[string]string{}
	}
	for _, k := range m.RequiredFieldKeys() {
		if v, ok := cfg[k]; !ok || v == "" {
			return Outcome{}, noopRollback, fmt.Errorf("%w: falta el campo obligatorio %q", ErrInvalid, k)
		}
	}

	out := Outcome{Config: map[string]string{}, EnqueueSCIM: m.provisioningKind() == ProvisioningSCIM}
	rollback := noopRollback

	// Lanzamiento por URL directa (OIDC/none): se resuelve ya; SAML-initiate lo fija el
	// SP creado (o el guardado en una reactivación).
	launch := ""
	if m.Launch.Kind == LaunchURL {
		u, err := render(m.Launch.URL, cfg)
		if err != nil {
			return Outcome{}, noopRollback, err
		}
		launch = u
	}

	switch m.Identity.Kind {
	case IdentitySAML:
		if spID := cfg[KeySSOSPID]; spID != "" { // reactivación: no recrear el SP.
			if launch == "" {
				launch = cfg[KeyLaunchURL]
			}
			break
		}
		entityID, err := render(m.Identity.EntityID, cfg)
		if err != nil {
			return Outcome{}, noopRollback, err
		}
		acs, err := render(m.Identity.ACSURL, cfg)
		if err != nil {
			return Outcome{}, noopRollback, err
		}
		sp, err := ids.CreateSAMLSP(ctx, org, SAMLSPInput{
			Name:         m.Name,
			EntityID:     entityID,
			ACSURL:       acs,
			NameIDFormat: m.Identity.NameIDFormat,
		})
		if err != nil {
			return Outcome{}, noopRollback, fmt.Errorf("materialize: crear SP SAML: %w", err)
		}
		out.Config[KeySSOSPID] = sp.ID
		out.SAMLMetadataURL = sp.MetadataURL
		if launch == "" { // launch initiate: la URL 1-click la emite id.
			launch = sp.InitiateURL
		}
		spID := sp.ID
		rollback = func(rbctx context.Context) error { return ids.DeleteSAMLSP(rbctx, org, spID) }

	case IdentityOIDC:
		if cfg[KeyOIDCClientID] != "" { // reactivación: el client ya existe (sin re-emitir secreto).
			if launch == "" {
				launch = cfg[KeyLaunchURL]
			}
			break
		}
		uris := make([]string, 0, len(m.Identity.RedirectURIs))
		for _, tpl := range m.Identity.RedirectURIs {
			u, err := render(tpl, cfg)
			if err != nil {
				return Outcome{}, noopRollback, err
			}
			uris = append(uris, u)
		}
		client, err := ids.CreateOIDCClient(ctx, org, OIDCClientInput{Name: m.Name, RedirectURIs: uris})
		if err != nil {
			return Outcome{}, noopRollback, fmt.Errorf("materialize: registrar client OIDC: %w", err)
		}
		out.Config[KeyOIDCClientID] = client.ClientID
		out.OIDCClientSecret = client.ClientSecret
		// ponytail: W2A no expone borrado de clients OIDC (RFC 7592). El client queda
		// registrado (org-scoped, PKCE, scopes self-service): no es un agujero, solo un
		// huérfano si la persistencia falla. Añadir DeleteOIDCClient cuando id lo exponga.

	case IdentityNone:
		// Sin recurso de identidad: solo lanzamiento por URL (ya resuelto arriba).
	}

	out.Config[KeyLaunchURL] = launch
	out.LaunchURL = launch
	return out, rollback, nil
}

// Deactivate revierte la materialización de una instancia: borra en Sytue ID el SP SAML
// que se creó. El client OIDC no se puede borrar (W2A no expone el endpoint): se olvida
// su id y se deja registrado. Idempotente: sin recurso guardado, no hace nada.
func Deactivate(ctx context.Context, ids IDApps, org string, cfg map[string]string) error {
	if spID := cfg[KeySSOSPID]; spID != "" {
		if err := ids.DeleteSAMLSP(ctx, org, spID); err != nil {
			return fmt.Errorf("deactivate: borrar SP SAML: %w", err)
		}
	}
	return nil
}

// ClearedConfig devuelve la config de la instancia SIN las claves internas del gateway
// (tras desactivar): olvida el SP/client y la URL de lanzamiento, conservando los
// valores de admin para una posible reactivación.
func ClearedConfig(cfg map[string]string) map[string]string {
	out := make(map[string]string, len(cfg))
	for k, v := range cfg {
		switch k {
		case KeySSOSPID, KeyOIDCClientID, KeyLaunchURL:
			continue
		}
		out[k] = v
	}
	return out
}
