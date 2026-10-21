// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ResolvedRepo es el proyecto y tenant resueltos a partir de la URL del repo de
// un webhook git (el visitante del webhook no porta tenant).
type ResolvedRepo struct {
	Tenant  string
	Project domain.Project
	// WebhookSecretEnc es el secreto HMAC del webhook cifrado del proyecto (nil si
	// el proyecto es anterior a los secretos por proyecto). El servicio lo descifra
	// para verificar la firma con el secreto DEL PROYECTO, no uno global.
	WebhookSecretEnc []byte
}

// ResolveRepo resuelve el proyecto cuyo repo_url coincide con el dado SIN tenant
// en el contexto (el emisor del webhook no lo porta). Usa una transacción de
// mantenimiento (pg.InMaintenanceTx), que habilita la política PERMISSIVE
// projects_maintenance (0003) para leer cross-tenant. Es el mismo mecanismo del
// reconciliador y es robusto con FORCE RLS aunque el dueño de la tabla no tenga
// BYPASSRLS (una función SECURITY DEFINER del dueño SÍ quedaría sujeta a RLS y no
// vería filas). Devuelve ErrNotFound si no hay coincidencia. El tenant sale de la
// propia fila (scan con t="").
//
// Devuelve TODOS los proyectos con esa URL, el más antiguo primero: la URL no es
// única entre organizaciones, y quedarse con una fila cualquiera (LIMIT 1 sin
// orden) dejaba que otra organización, registrando la misma URL, hiciera fallar
// la firma de los pushes de la dueña (CLOUD-05). Quien llama se queda con el que
// valide la firma.
func (s *Store) ResolveRepos(ctx context.Context, repoURL string) ([]ResolvedRepo, error) {
	var out []ResolvedRepo
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+projectCols+`, webhook_secret_enc FROM projects
			 WHERE lower(repo_url)=lower($1) AND deleted_at IS NULL
			 ORDER BY created_at, id LIMIT 20`,
			repoURL)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var enc []byte
			p, e := scanProjectWithSecret(rows, "", &enc)
			if e != nil {
				return e
			}
			out = append(out, ResolvedRepo{Tenant: p.Tenant, Project: p, WebhookSecretEnc: enc})
		}
		return rows.Err()
	})
	if err == nil && len(out) == 0 {
		return nil, domain.ErrNotFound
	}
	return out, err
}

// RecordWebhook persiste un webhook entrante para auditoría/dedup. Cross-tenant.
// Devuelve duplicate=true si delivery_id ya existía (idempotencia).
func (s *Store) RecordWebhook(ctx context.Context, tenant string, projectID uuid.UUID, provider, deliveryID, eventType string, payload []byte) (duplicate bool, err error) {
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	err = s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO git_webhooks (tenant, project_id, provider, delivery_id, event_type, payload)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (provider, delivery_id) WHERE delivery_id <> '' DO NOTHING`,
			tenant, projectID, provider, deliveryID, eventType, payload)
		if err != nil {
			return fmtErr("registrar webhook", err)
		}
		if tag.RowsAffected() == 0 && deliveryID != "" {
			duplicate = true
		}
		return nil
	})
	return duplicate, err
}
