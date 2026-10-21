// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Persistencia de la identidad SALIENTE (Sytue Integration Gateway · Ola 2A): SP SAML
// de terceros (saml_sps), clave de firma del IdP por-org (saml_idp_keys, SELLADA en
// reposo como signing_keys), clients OIDC por-org y tokens de acceso inicial del DCR
// (RFC 7591). Todo acotado por org (aislamiento tenant, DENY-BY-DEFAULT): un SP/clave
// de la org A jamás se resuelve para la org B.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// --- SP SAML de terceros (Sytue ID como IdP) ---

const samlSPCols = `id, org_id, name, entity_id, acs_url, cert_pem, name_id_format,
	attr_mapping, idp_initiated, relay_state_default, enabled,
	coalesce(created_by::text,''), created_at, updated_at`

func scanSAMLSP(row pgx.Row) (domain.SAMLServiceProvider, error) {
	var p domain.SAMLServiceProvider
	var mappingRaw []byte
	err := row.Scan(&p.ID, &p.OrgID, &p.Name, &p.EntityID, &p.ACSURL, &p.CertPEM,
		&p.NameIDFormat, &mappingRaw, &p.IDPInitiated, &p.RelayStateDefault, &p.Enabled,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return domain.SAMLServiceProvider{}, err
	}
	if len(mappingRaw) > 0 {
		_ = json.Unmarshal(mappingRaw, &p.AttrMapping)
	}
	return p, nil
}

// CreateSAMLServiceProvider registra un SP SAML de terceros en una organización.
// Único por (org, entity_id) ⇒ ErrConflict.
func (s *Store) CreateSAMLServiceProvider(ctx context.Context, q pg.Querier, p domain.SAMLServiceProvider) (domain.SAMLServiceProvider, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO saml_sps
		(org_id, name, entity_id, acs_url, cert_pem, name_id_format, attr_mapping,
		 idp_initiated, relay_state_default, enabled, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, nullif($11,'')::uuid)
		RETURNING ` + samlSPCols
	row := q.QueryRow(ctx, sql, p.OrgID, p.Name, p.EntityID, p.ACSURL, p.CertPEM,
		p.NameIDFormat, marshalMapping(p.AttrMapping), p.IDPInitiated,
		p.RelayStateDefault, p.Enabled, p.CreatedBy)
	got, err := scanSAMLSP(row)
	return got, errf("CreateSAMLServiceProvider", err)
}

// UpdateSAMLServiceProvider actualiza un SP acotado por (org, id): uno de otra org NO
// se toca (aislamiento tenant). ErrNotFound si no pertenece a la org.
func (s *Store) UpdateSAMLServiceProvider(ctx context.Context, q pg.Querier, p domain.SAMLServiceProvider) (domain.SAMLServiceProvider, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE saml_sps SET
		name=$3, entity_id=$4, acs_url=$5, cert_pem=$6, name_id_format=$7,
		attr_mapping=$8, idp_initiated=$9, relay_state_default=$10, enabled=$11,
		updated_at=now()
		WHERE id=$1 AND org_id=$2
		RETURNING ` + samlSPCols
	row := q.QueryRow(ctx, sql, p.ID, p.OrgID, p.Name, p.EntityID, p.ACSURL, p.CertPEM,
		p.NameIDFormat, marshalMapping(p.AttrMapping), p.IDPInitiated,
		p.RelayStateDefault, p.Enabled)
	got, err := scanSAMLSP(row)
	return got, errf("UpdateSAMLServiceProvider", err)
}

// GetSAMLServiceProvider recupera un SP acotado por (org, id). Cross-org ⇒ ErrNotFound.
func (s *Store) GetSAMLServiceProvider(ctx context.Context, orgID, id string) (domain.SAMLServiceProvider, error) {
	const sql = `SELECT ` + samlSPCols + ` FROM saml_sps WHERE id=$1 AND org_id=$2`
	p, err := scanSAMLSP(s.DB().QueryRow(ctx, sql, id, orgID))
	return p, errf("GetSAMLServiceProvider", err)
}

// GetSAMLServiceProviderByID recupera un SP por su UUID sin acotar por org (uso INTERNO
// del flujo IdP-initiated: el llamante DEBE verificar que OrgID == org de la sesión).
func (s *Store) GetSAMLServiceProviderByID(ctx context.Context, id string) (domain.SAMLServiceProvider, error) {
	const sql = `SELECT ` + samlSPCols + ` FROM saml_sps WHERE id=$1`
	p, err := scanSAMLSP(s.DB().QueryRow(ctx, sql, id))
	return p, errf("GetSAMLServiceProviderByID", err)
}

// GetSAMLServiceProviderByEntityID resuelve el SP de una org por su EntityID (Issuer del
// AuthnRequest en el flujo SP-initiated). ACOTADO a la org de la SESIÓN: un SP de otra
// org no casa aquí (aislamiento tenant, DENY-BY-DEFAULT). ErrNotFound si ninguno casa.
func (s *Store) GetSAMLServiceProviderByEntityID(ctx context.Context, orgID, entityID string) (domain.SAMLServiceProvider, error) {
	const sql = `SELECT ` + samlSPCols + ` FROM saml_sps WHERE org_id=$1 AND entity_id=$2`
	p, err := scanSAMLSP(s.DB().QueryRow(ctx, sql, orgID, entityID))
	return p, errf("GetSAMLServiceProviderByEntityID", err)
}

// ListSAMLServiceProviders lista los SP de una organización (keyset por created_at,id).
func (s *Store) ListSAMLServiceProviders(ctx context.Context, orgID string) ([]domain.SAMLServiceProvider, error) {
	const sql = `SELECT ` + samlSPCols + ` FROM saml_sps
		WHERE org_id=$1 ORDER BY created_at DESC, id DESC`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListSAMLServiceProviders", err)
	}
	defer rows.Close()
	var out []domain.SAMLServiceProvider
	for rows.Next() {
		p, err := scanSAMLSP(rows)
		if err != nil {
			return nil, errf("ListSAMLServiceProviders.scan", err)
		}
		out = append(out, p)
	}
	return out, errf("ListSAMLServiceProviders.iter", rows.Err())
}

// DeleteSAMLServiceProvider borra un SP acotado por (org, id). true si borró.
func (s *Store) DeleteSAMLServiceProvider(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM saml_sps WHERE id=$1 AND org_id=$2`, id, orgID)
	if err != nil {
		return false, errf("DeleteSAMLServiceProvider", err)
	}
	return tag.RowsAffected() > 0, nil
}

// --- Clave de firma del IdP por-org (Sytue ID como IdP) ---

// GetOrCreateIDPKey devuelve el material de firma del IdP de una org (clave privada
// descifrada + cert público), generándolo y SELLÁNDOLO la 1ª vez (encv1:… como
// signing_keys). gen produce (privPEM, certPEM) y solo se invoca si no hay fila. La
// inserción es idempotente ante carrera (ON CONFLICT DO NOTHING + re-lectura).
func (s *Store) GetOrCreateIDPKey(ctx context.Context, orgID string, gen func() (string, string, error)) (privPEM, certPEM string, err error) {
	priv, cert, err := s.getIDPKey(ctx, orgID)
	if err == nil {
		return priv, cert, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", "", err
	}
	np, nc, gerr := gen()
	if gerr != nil {
		return "", "", errf("GetOrCreateIdPKey.gen", gerr)
	}
	sealed, serr := s.keyCipher.Encrypt(np)
	if serr != nil {
		return "", "", errf("GetOrCreateIdPKey.seal", serr)
	}
	_, ierr := s.DB().Exec(ctx,
		`INSERT INTO saml_idp_keys (org_id, private_pem, cert_pem)
		 VALUES ($1,$2,$3) ON CONFLICT (org_id) DO NOTHING`, orgID, sealed, nc)
	if ierr != nil {
		return "", "", errf("GetOrCreateIdPKey.insert", ierr)
	}
	// Re-leer: si otra petición ganó la carrera, devuelve SU par (no el nuestro).
	return s.getIDPKey(ctx, orgID)
}

// getIDPKey lee y DESCIFRA la clave del IdP de una org. ErrNotFound si no existe.
// GetIDPKey devuelve la clave de firma del IdP SAML de una organización SIN
// crearla: (ErrNotFound) si todavía no existe. La metadata pública del IdP la usa
// para no acuñar una clave por una petición anónima (ID-02); la generación vive en
// GetOrCreateIDPKey, que solo recorren los caminos autenticados.
func (s *Store) GetIDPKey(ctx context.Context, orgID string) (privPEM, certPEM string, err error) {
	return s.getIDPKey(ctx, orgID)
}

func (s *Store) getIDPKey(ctx context.Context, orgID string) (privPEM, certPEM string, err error) {
	var sealed, cert string
	if err := s.DB().QueryRow(ctx,
		`SELECT private_pem, cert_pem FROM saml_idp_keys WHERE org_id=$1`, orgID).
		Scan(&sealed, &cert); err != nil {
		return "", "", errf("getIdPKey", err)
	}
	priv, derr := s.keyCipher.Decrypt(sealed)
	if derr != nil {
		return "", "", errf("getIdPKey.decrypt", derr)
	}
	return priv, cert, nil
}

// --- Clients OIDC por-org + registro dinámico (RFC 7591) ---

// CreateOAuthClient inserta un client OIDC acotado a una org (registro dinámico o
// admin). El secret_hash (Argon2id, ya calculado) se guarda tal cual; ” ⇒ NULL.
func (s *Store) CreateOAuthClient(ctx context.Context, q pg.Querier, c domain.Client, orgID, managedBy string) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO oauth_clients
		(id, secret_hash, name, type, redirect_uris, post_logout_redirect_uris,
		 grant_types, response_types, scopes, require_pkce, token_endpoint_auth_method,
		 org_id, managed_by)
		VALUES ($1, nullif($2,''), $3, $4, $5, $6, $7, $8, $9, $10, $11,
		        nullif($12,'')::uuid, $13)`
	// Los text[] son NOT NULL DEFAULT '{}': un slice nil (p. ej. sin
	// post_logout_redirect_uris) se guarda como '{}', no como NULL.
	_, err := q.Exec(ctx, sql, c.ID, c.SecretHash, c.Name, string(c.Type),
		nonNilDomains(c.RedirectURIs), nonNilDomains(c.PostLogoutRedirectURIs),
		nonNilDomains(c.GrantTypes), nonNilDomains(c.ResponseTypes), nonNilDomains(c.Scopes),
		c.RequirePKCE, c.TokenEndpointAuthMethod, orgID, managedBy)
	return errf("CreateOAuthClient", err)
}

// InsertDCRToken persiste el HASH de un token de acceso inicial del DCR, acotado a una
// org y con caducidad (RFC 7591 §1.2).
func (s *Store) InsertDCRToken(ctx context.Context, q pg.Querier, tokenHash, orgID, createdBy string, exp time.Time) error {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO dcr_initial_tokens (token_hash, org_id, created_by, expires_at)
		VALUES ($1,$2, nullif($3,'')::uuid, $4)`
	_, err := q.Exec(ctx, sql, tokenHash, orgID, createdBy, exp)
	return errf("InsertDCRToken", err)
}

// OrgForDCRToken devuelve la org de un token de acceso inicial vigente (no caducado).
// ErrNotFound si el token no existe o expiró. NO lo consume (reutilizable en su ventana).
func (s *Store) OrgForDCRToken(ctx context.Context, tokenHash string, now time.Time) (string, error) {
	var org string
	err := s.DB().QueryRow(ctx,
		`SELECT org_id::text FROM dcr_initial_tokens WHERE token_hash=$1 AND expires_at > $2`,
		tokenHash, now).Scan(&org)
	return org, errf("OrgForDCRToken", err)
}
