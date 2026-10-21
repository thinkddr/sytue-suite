// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// providerCols proyecta un identity_provider. client_secret se descifra en la
// aplicación (scanProvider), nunca sale de aquí en el listado admin.
const providerCols = `id, org_id, type, name, slug, issuer, client_id,
	client_secret, scopes, attribute_mapping, allowed_email_domains,
	authorization_endpoint, token_endpoint, userinfo_endpoint, jwks_uri,
	pkce, idp_entity_id, idp_sso_url, idp_certificate, sp_entity_id,
	saml_binding, name_id_format,
	enabled, coalesce(created_by::text,''), created_at, updated_at`

// scanProvider materializa un identity_provider, descifrando el client_secret en
// reposo (o dejándolo ” si el provider es público). decryptSecret=false lo omite
// (listados admin: el secreto NUNCA se devuelve).
func (s *Store) scanProvider(row pgx.Row, decryptSecret bool) (domain.IdentityProvider, error) {
	var p domain.IdentityProvider
	var mappingRaw []byte
	var storedSecret string
	err := row.Scan(&p.ID, &p.OrgID, &p.Type, &p.Name, &p.Slug, &p.Issuer, &p.ClientID,
		&storedSecret, &p.Scopes, &mappingRaw, &p.AllowedEmailDomains,
		&p.AuthorizationEndpoint, &p.TokenEndpoint, &p.UserinfoEndpoint, &p.JWKSURI,
		&p.PKCE, &p.IDPEntityID, &p.IDPSSOURL, &p.IDPCertificate, &p.SPEntityID,
		&p.SAMLBinding, &p.NameIDFormat,
		&p.Enabled, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	if len(mappingRaw) > 0 {
		_ = json.Unmarshal(mappingRaw, &p.Mapping)
	}
	if decryptSecret && storedSecret != "" {
		sec, derr := s.keyCipher.Decrypt(storedSecret)
		if derr != nil {
			return domain.IdentityProvider{}, errf("scanProvider.decrypt", derr)
		}
		p.ClientSecret = sec
	}
	return p, nil
}

// encryptSecret cifra el client_secret en reposo (KEK/AES-256-GCM). Un secreto
// vacío (client público, PKCE puro) se guarda tal cual ” sin envolver.
func (s *Store) encryptSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return s.keyCipher.Encrypt(plain)
}

// nonNilDomains evita insertar NULL en la columna text[] NOT NULL: un slice nil
// (allowed_email_domains omitido) se guarda como '{}' (sin restricción de dominio).
func nonNilDomains(d []string) []string {
	if d == nil {
		return []string{}
	}
	return d
}

func marshalMapping(m map[string]string) []byte {
	if len(m) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// CreateIdentityProvider inserta un provider de login federado de una organización.
// type ∈ {oidc, saml} (por defecto 'oidc'). Los campos OIDC (issuer/client_id/
// endpoints) y los SAML (idp_entity_id/idp_sso_url/…) coexisten en la misma tabla;
// el tipo no usado se guarda en ”. El client_secret (OIDC) se cifra en reposo; el
// idp_certificate (SAML) es PÚBLICO y se guarda en claro. Único por (org, slug) ⇒
// ErrConflict.
func (s *Store) CreateIdentityProvider(ctx context.Context, q pg.Querier, p domain.IdentityProvider) (domain.IdentityProvider, error) {
	if q == nil {
		q = s.DB()
	}
	enc, err := s.encryptSecret(p.ClientSecret)
	if err != nil {
		return domain.IdentityProvider{}, errf("CreateIdentityProvider.encrypt", err)
	}
	const sql = `INSERT INTO identity_providers
		(org_id, type, name, slug, issuer, client_id, client_secret, scopes,
		 attribute_mapping, allowed_email_domains, authorization_endpoint,
		 token_endpoint, userinfo_endpoint, jwks_uri, pkce,
		 idp_entity_id, idp_sso_url, idp_certificate, sp_entity_id,
		 saml_binding, name_id_format, enabled, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
		        $16,$17,$18,$19,$20,$21,$22, nullif($23,'')::uuid)
		RETURNING ` + providerCols
	row := q.QueryRow(ctx, sql, p.OrgID, defaultStr(p.Type, "oidc"), p.Name, p.Slug,
		p.Issuer, p.ClientID, enc, defaultStr(p.Scopes, "openid email profile"),
		marshalMapping(p.Mapping), nonNilDomains(p.AllowedEmailDomains),
		p.AuthorizationEndpoint, p.TokenEndpoint, p.UserinfoEndpoint, p.JWKSURI, p.PKCE,
		p.IDPEntityID, p.IDPSSOURL, p.IDPCertificate, p.SPEntityID,
		defaultStr(p.SAMLBinding, "redirect"), p.NameIDFormat, p.Enabled, p.CreatedBy)
	got, err := s.scanProvider(row, true)
	return got, errf("CreateIdentityProvider", err)
}

// UpdateIdentityProvider actualiza un provider acotado por (org, id): un provider
// de otra org NO se toca (aislamiento tenant). newSecret nil ⇒ conserva el secreto
// actual; no nil ⇒ lo re-cifra (” lo vacía a client público). Devuelve
// ErrNotFound si el provider no pertenece a la org.
func (s *Store) UpdateIdentityProvider(ctx context.Context, q pg.Querier, p domain.IdentityProvider, newSecret *string) (domain.IdentityProvider, error) {
	if q == nil {
		q = s.DB()
	}
	// client_secret: COALESCE($7, client_secret) ⇒ NULL conserva el valor previo.
	var secretArg any
	if newSecret != nil {
		enc, err := s.encryptSecret(*newSecret)
		if err != nil {
			return domain.IdentityProvider{}, errf("UpdateIdentityProvider.encrypt", err)
		}
		secretArg = enc
	}
	// type es inmutable (no se conmuta oidc↔saml): no se actualiza aquí.
	const sql = `UPDATE identity_providers SET
		name=$3, slug=$4, issuer=$5, client_id=$6,
		client_secret=COALESCE($7, client_secret), scopes=$8,
		attribute_mapping=$9, allowed_email_domains=$10,
		authorization_endpoint=$11, token_endpoint=$12, userinfo_endpoint=$13,
		jwks_uri=$14, pkce=$15,
		idp_entity_id=$16, idp_sso_url=$17, idp_certificate=$18, sp_entity_id=$19,
		saml_binding=$20, name_id_format=$21,
		enabled=$22, updated_at=now()
		WHERE id=$1 AND org_id=$2
		RETURNING ` + providerCols
	row := q.QueryRow(ctx, sql, p.ID, p.OrgID, p.Name, p.Slug, p.Issuer, p.ClientID,
		secretArg, defaultStr(p.Scopes, "openid email profile"), marshalMapping(p.Mapping),
		nonNilDomains(p.AllowedEmailDomains), p.AuthorizationEndpoint, p.TokenEndpoint,
		p.UserinfoEndpoint, p.JWKSURI, p.PKCE,
		p.IDPEntityID, p.IDPSSOURL, p.IDPCertificate, p.SPEntityID,
		defaultStr(p.SAMLBinding, "redirect"), p.NameIDFormat, p.Enabled)
	got, err := s.scanProvider(row, true)
	return got, errf("UpdateIdentityProvider", err)
}

// GetIdentityProvider recupera un provider acotado por (org, id) con el secreto
// descifrado. Cross-org ⇒ ErrNotFound (no revela su existencia).
func (s *Store) GetIdentityProvider(ctx context.Context, orgID, id string) (domain.IdentityProvider, error) {
	const sql = `SELECT ` + providerCols + ` FROM identity_providers WHERE id=$1 AND org_id=$2`
	p, err := s.scanProvider(s.DB().QueryRow(ctx, sql, id, orgID), true)
	return p, errf("GetIdentityProvider", err)
}

// GetIdentityProviderByID recupera un provider por su UUID sin acotar por org,
// con el secreto descifrado. USO INTERNO del flujo RP (el callback ya se liga al
// provider vía el federated_auth_state, que a su vez referencia el provider_id);
// nunca se expone directamente a un endpoint admin.
func (s *Store) GetIdentityProviderByID(ctx context.Context, id string) (domain.IdentityProvider, error) {
	const sql = `SELECT ` + providerCols + ` FROM identity_providers WHERE id=$1`
	p, err := s.scanProvider(s.DB().QueryRow(ctx, sql, id), true)
	return p, errf("GetIdentityProviderByID", err)
}

// ListIdentityProviders lista los providers de una organización SIN el secreto
// (decryptSecret=false): el listado admin nunca lo devuelve en claro.
func (s *Store) ListIdentityProviders(ctx context.Context, orgID string) ([]domain.IdentityProvider, error) {
	const sql = `SELECT ` + providerCols + ` FROM identity_providers
		WHERE org_id=$1 ORDER BY created_at DESC, id DESC`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListIdentityProviders", err)
	}
	defer rows.Close()
	var out []domain.IdentityProvider
	for rows.Next() {
		p, err := s.scanProvider(rows, false)
		if err != nil {
			return nil, errf("ListIdentityProviders.scan", err)
		}
		out = append(out, p)
	}
	return out, errf("ListIdentityProviders.iter", rows.Err())
}

// DeleteIdentityProvider borra un provider acotado por (org, id). Devuelve true
// si borró una fila (existía y pertenecía a la org). La cascada limpia sus
// federated_identities y auth_states.
func (s *Store) DeleteIdentityProvider(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM identity_providers WHERE id=$1 AND org_id=$2`, id, orgID)
	if err != nil {
		return false, errf("DeleteIdentityProvider", err)
	}
	return tag.RowsAffected() > 0, nil
}

// FindProviderByEmailDomain resuelve el provider ACTIVO cuyo allowed_email_domains
// contiene el dominio dado (enrutado de login por dominio). Case-insensitive; usa
// el índice GIN parcial. Si varias orgs reclaman el mismo dominio devuelve el más
// antiguo (determinista). Sin secreto. ErrNotFound si ninguno casa.
//
// Nota de seguridad: este enrutado es solo CONVENIENCIA. La autenticación real la
// hace el IdP de ESE provider (de su org); un provider de la org A jamás autentica
// contra un usuario de la org B — el enlace/JIT se acota a la org del provider y
// nunca liga a un usuario preexistente de otra org (ver handleFederatedCallback).
func (s *Store) FindProviderByEmailDomain(ctx context.Context, dom string) (domain.IdentityProvider, error) {
	const sql = `SELECT ` + providerCols + ` FROM identity_providers
		WHERE enabled AND allowed_email_domains @> ARRAY[lower($1)]::text[]
		ORDER BY created_at ASC, id ASC LIMIT 1`
	p, err := s.scanProvider(s.DB().QueryRow(ctx, sql, dom), false)
	return p, errf("FindProviderByEmailDomain", err)
}

// --- federated_identities: enlace (provider, sub externo) ⇄ usuario ---

// GetFederatedUserID devuelve el usuario ligado a (provider, sub externo), o
// ErrNotFound si aún no hay enlace.
func (s *Store) GetFederatedUserID(ctx context.Context, providerID, externalSubject string) (string, error) {
	var userID string
	err := s.DB().QueryRow(ctx,
		`SELECT user_id::text FROM federated_identities WHERE provider_id=$1 AND external_subject=$2`,
		providerID, externalSubject).Scan(&userID)
	return userID, errf("GetFederatedUserID", err)
}

// LinkFederatedIdentity liga (provider, sub externo) a un usuario. Idempotente:
// si ya existe con el MISMO usuario, no falla; un sub ya ligado a OTRO usuario ⇒
// ErrConflict (no se re-liga en silencio).
func (s *Store) LinkFederatedIdentity(ctx context.Context, q pg.Querier, providerID, externalSubject, userID string) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO federated_identities (provider_id, external_subject, user_id)
		VALUES ($1,$2,$3)
		ON CONFLICT (provider_id, external_subject)
		DO UPDATE SET external_subject = EXCLUDED.external_subject
		WHERE federated_identities.user_id = EXCLUDED.user_id`
	tag, err := q.Exec(ctx, sql, providerID, externalSubject, userID)
	if err != nil {
		return errf("LinkFederatedIdentity", err)
	}
	if tag.RowsAffected() == 0 {
		// El sub ya está ligado a un usuario distinto: no re-ligar (anti-takeover).
		return domain.ErrConflict
	}
	return nil
}

// --- federated_auth_states: estado efímero del flujo RP (single-use) ---

// FederatedAuthState es el estado en curso de un flujo Authorization Code contra
// el IdP upstream, recuperado al consumirlo en el callback.
type FederatedAuthState struct {
	ProviderID   string
	Nonce        string
	CodeVerifier string
	ReturnTo     string
	ExpiresAt    time.Time
}

// InsertFederatedAuthState persiste el estado de un flujo RP. Solo se guarda el
// HASH del state (el valor viaja en cookie + URL).
func (s *Store) InsertFederatedAuthState(ctx context.Context, q pg.Querier, stateHash, providerID, nonce, codeVerifier, returnTo string, exp time.Time) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO federated_auth_states
		(state_hash, provider_id, nonce, code_verifier, return_to, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`
	_, err := q.Exec(ctx, sql, stateHash, providerID, nonce, codeVerifier, returnTo, exp)
	return errf("InsertFederatedAuthState", err)
}

// ConsumeFederatedAuthState borra y devuelve el estado de un flujo RP de forma
// ATÓMICA (un solo uso, anti-replay), validando que no haya expirado. ErrNotFound
// si el state no existe, ya se usó o caducó.
func (s *Store) ConsumeFederatedAuthState(ctx context.Context, q pg.Querier, stateHash string, now time.Time) (FederatedAuthState, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `DELETE FROM federated_auth_states
		WHERE state_hash=$1 AND expires_at > $2
		RETURNING provider_id::text, nonce, code_verifier, return_to, expires_at`
	var st FederatedAuthState
	err := q.QueryRow(ctx, sql, stateHash, now).
		Scan(&st.ProviderID, &st.Nonce, &st.CodeVerifier, &st.ReturnTo, &st.ExpiresAt)
	return st, errf("ConsumeFederatedAuthState", err)
}
