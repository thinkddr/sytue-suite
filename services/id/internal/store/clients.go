// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

const clientCols = `id, coalesce(secret_hash,''), name, type, redirect_uris,
	post_logout_redirect_uris, grant_types, response_types, scopes,
	require_pkce, token_endpoint_auth_method, coalesce(org_id::text,''),
	may_impersonate, may_impersonate_admins, created_at, updated_at`

func scanClient(row pgx.Row) (domain.Client, error) {
	var c domain.Client
	err := row.Scan(&c.ID, &c.SecretHash, &c.Name, &c.Type, &c.RedirectURIs,
		&c.PostLogoutRedirectURIs, &c.GrantTypes, &c.ResponseTypes, &c.Scopes,
		&c.RequirePKCE, &c.TokenEndpointAuthMethod, &c.OrgID,
		&c.MayImpersonate, &c.MayImpersonateAdmins, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// GetClient recupera un client OAuth2/OIDC por id.
func (s *Store) GetClient(ctx context.Context, id string) (domain.Client, error) {
	const sql = `SELECT ` + clientCols + ` FROM oauth_clients WHERE id = $1`
	c, err := scanClient(s.DB().QueryRow(ctx, sql, id))
	return c, errf("GetClient", err)
}

// SetClientSecretHash fija el secret_hash de un client (rotación de secreto).
func (s *Store) SetClientSecretHash(ctx context.Context, q pg.Querier, id, hash string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE oauth_clients SET secret_hash=nullif($2,'') WHERE id=$1`, id, hash)
	return errf("SetClientSecretHash", err)
}

// ListClients devuelve todos los clients (consola admin).
func (s *Store) ListClients(ctx context.Context) ([]domain.Client, error) {
	const sql = `SELECT ` + clientCols + ` FROM oauth_clients ORDER BY id`
	rows, err := s.DB().Query(ctx, sql)
	if err != nil {
		return nil, errf("ListClients", err)
	}
	defer rows.Close()
	var out []domain.Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, errf("ListClients.scan", err)
		}
		out = append(out, c)
	}
	return out, errf("ListClients.iter", rows.Err())
}

// --- Clients de UNA organización (Console). Deny-by-default: el org_id va en el
// WHERE, nunca se filtra en memoria; los clients globales (org_id NULL, mig 0002)
// quedan fuera por construcción. ---

// ListOrgClients devuelve los clients OIDC registrados por una org (admin o DCR).
func (s *Store) ListOrgClients(ctx context.Context, orgID string) ([]domain.Client, error) {
	const sql = `SELECT ` + clientCols + ` FROM oauth_clients
		WHERE org_id = $1 ORDER BY created_at DESC`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListOrgClients", err)
	}
	defer rows.Close()
	out := make([]domain.Client, 0, 8)
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, errf("ListOrgClients.scan", err)
		}
		out = append(out, c)
	}
	return out, errf("ListOrgClients.iter", rows.Err())
}

// GetOrgClient recupera un client ACOTADO a la org. El de otra org (o inexistente)
// devuelve ErrNotFound: el admin no puede distinguir ambos casos.
func (s *Store) GetOrgClient(ctx context.Context, orgID, id string) (domain.Client, error) {
	const sql = `SELECT ` + clientCols + ` FROM oauth_clients WHERE id = $1 AND org_id = $2`
	c, err := scanClient(s.DB().QueryRow(ctx, sql, id, orgID))
	return c, errf("GetOrgClient", err)
}

// DeleteOrgClient borra un client de la org (false si no existe o es de otra org).
// Sus códigos y refresh tokens caen en cascada (FK de la mig 0001).
func (s *Store) DeleteOrgClient(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM oauth_clients WHERE id=$1 AND org_id=$2`, id, orgID)
	if err != nil {
		return false, errf("DeleteOrgClient", err)
	}
	return tag.RowsAffected() > 0, nil
}
