// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// AuditSink es el destino SIEM de una organización. Los dos secretos (la cabecera de
// autorización del SIEM y el secreto de firma) viajan SELLADOS con la KEK del servicio:
// son credenciales de un tercero y no salen en claro de aquí.
type AuditSink struct {
	OrgID string
	URL   string
	// AuthHeader y SigningSecret vienen descifrados al leer con GetAuditSink y se
	// sellan al guardar. Nunca se serializan hacia el navegador.
	AuthHeader    string
	SigningSecret string
	Enabled       bool
	// LastSentID es la posición en el bigint monótono de audit_log: hasta aquí el SIEM
	// confirmó recepción. Vive en la base y no en memoria para que reiniciar el
	// servicio no reenvíe lo ya entregado ni se salte lo pendiente.
	LastSentID int64
	LastError  string
	LastSentAt *time.Time
}

// GetAuditSink devuelve el sink de la organización con sus secretos ya descifrados.
// Sin sink configurado devuelve ErrNotFound.
func (s *Store) GetAuditSink(ctx context.Context, orgID string) (AuditSink, error) {
	var a AuditSink
	var authSealed, signSealed string
	err := s.DB().QueryRow(ctx,
		`SELECT org_id, url, auth_header, signing_secret, enabled, last_sent_id, last_error, last_sent_at
		   FROM audit_sinks WHERE org_id = $1::uuid`, orgID).
		Scan(&a.OrgID, &a.URL, &authSealed, &signSealed, &a.Enabled, &a.LastSentID, &a.LastError, &a.LastSentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuditSink{}, domain.ErrNotFound
	}
	if err != nil {
		return AuditSink{}, errf("GetAuditSink", err)
	}
	if a.AuthHeader, err = s.unseal(authSealed); err != nil {
		return AuditSink{}, errf("GetAuditSink.auth", err)
	}
	if a.SigningSecret, err = s.unseal(signSealed); err != nil {
		return AuditSink{}, errf("GetAuditSink.sign", err)
	}
	return a, nil
}

// SaveAuditSink crea o actualiza el sink. NO toca `last_sent_id`: cambiar la URL o el
// token no debe reenviar el histórico entero al SIEM nuevo (ni saltárselo) — para eso
// está ResetAuditSinkCursor, que es una decisión explícita del administrador.
func (s *Store) SaveAuditSink(ctx context.Context, a AuditSink) error {
	auth, err := s.seal(a.AuthHeader)
	if err != nil {
		return errf("SaveAuditSink.auth", err)
	}
	sign, err := s.seal(a.SigningSecret)
	if err != nil {
		return errf("SaveAuditSink.sign", err)
	}
	_, err = s.DB().Exec(ctx,
		`INSERT INTO audit_sinks (org_id, url, auth_header, signing_secret, enabled)
		 VALUES ($1::uuid, $2, $3, $4, $5)
		 ON CONFLICT (org_id) DO UPDATE
		   SET url = EXCLUDED.url, auth_header = EXCLUDED.auth_header,
		       signing_secret = EXCLUDED.signing_secret, enabled = EXCLUDED.enabled,
		       updated_at = now()`,
		a.OrgID, a.URL, auth, sign, a.Enabled)
	return errf("SaveAuditSink", err)
}

// DeleteAuditSink retira el destino. Devuelve false si no había ninguno.
func (s *Store) DeleteAuditSink(ctx context.Context, orgID string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM audit_sinks WHERE org_id = $1::uuid`, orgID)
	if err != nil {
		return false, errf("DeleteAuditSink", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ListEnabledAuditSinks devuelve los sinks activos de TODAS las organizaciones: es lo
// que recorre el despachador de fondo.
func (s *Store) ListEnabledAuditSinks(ctx context.Context) ([]AuditSink, error) {
	rows, err := s.DB().Query(ctx,
		`SELECT org_id, url, auth_header, signing_secret, last_sent_id
		   FROM audit_sinks WHERE enabled ORDER BY org_id`)
	if err != nil {
		return nil, errf("ListEnabledAuditSinks", err)
	}
	defer rows.Close()
	var out []AuditSink
	for rows.Next() {
		var a AuditSink
		var authSealed, signSealed string
		if err := rows.Scan(&a.OrgID, &a.URL, &authSealed, &signSealed, &a.LastSentID); err != nil {
			return nil, errf("ListEnabledAuditSinks.scan", err)
		}
		if a.AuthHeader, err = s.unseal(authSealed); err != nil {
			return nil, errf("ListEnabledAuditSinks.auth", err)
		}
		if a.SigningSecret, err = s.unseal(signSealed); err != nil {
			return nil, errf("ListEnabledAuditSinks.sign", err)
		}
		a.Enabled = true
		out = append(out, a)
	}
	return out, errf("ListEnabledAuditSinks.rows", rows.Err())
}

// AuditSince lee los eventos de la organización POSTERIORES a afterID, en orden
// ascendente. Ascendente y por id monótono a propósito: el SIEM tiene que recibir los
// eventos en el orden en que ocurrieron, y el cursor avanza sin ambigüedad aunque dos
// eventos compartan `occurred_at`.
func (s *Store) AuditSince(ctx context.Context, orgID string, afterID int64, limit int) ([]AuditRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.DB().Query(ctx,
		auditSelect+` WHERE l.org_id = $1::uuid AND l.id > $2 ORDER BY l.id ASC LIMIT $3`,
		orgID, afterID, limit)
	if err != nil {
		return nil, errf("AuditSince", err)
	}
	defer rows.Close()
	out := make([]AuditRow, 0, limit)
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.OccurredAt, &a.ActorID, &a.ActorEmail,
			&a.Action, &a.Target, &a.IP, &a.Detail); err != nil {
			return nil, errf("AuditSince.scan", err)
		}
		out = append(out, a)
	}
	return out, errf("AuditSince.rows", rows.Err())
}

// MarkAuditSent avanza la posición confirmada y limpia el error. Solo se llama tras un
// 2xx del SIEM: si la entrega falla, el cursor NO se mueve y el lote se reintenta.
func (s *Store) MarkAuditSent(ctx context.Context, orgID string, lastID int64) error {
	_, err := s.DB().Exec(ctx,
		`UPDATE audit_sinks SET last_sent_id = $2, last_error = '', last_sent_at = now(),
		        updated_at = now()
		  WHERE org_id = $1::uuid`, orgID, lastID)
	return errf("MarkAuditSent", err)
}

// MarkAuditSinkError deja constancia del fallo SIN mover el cursor, para que el admin
// vea por qué su SIEM no recibe nada en vez de mirar una pantalla en silencio.
func (s *Store) MarkAuditSinkError(ctx context.Context, orgID, msg string) error {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := s.DB().Exec(ctx,
		`UPDATE audit_sinks SET last_error = $2, updated_at = now() WHERE org_id = $1::uuid`,
		orgID, msg)
	return errf("MarkAuditSinkError", err)
}

// seal y unseal envuelven el cifrado con la KEK del servicio, tolerando el vacío: un
// sink sin cabecera de autorización (SIEM que autentica por la URL) es legítimo.
func (s *Store) seal(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	return s.keyCipher.Encrypt(v)
}

func (s *Store) unseal(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	return s.keyCipher.Decrypt(v)
}
