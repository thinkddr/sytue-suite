// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Canal y tipos de evento de ciclo de vida de deployment. El canal es canónico
// aquí (el store es el emisor, dentro de la MISMA tx que el cambio de estado —
// outbox transaccional) y service.EventChannel lo re-exporta para el Dispatcher
// y el Subscriber. Los tipos los consume el worker de notificaciones
// (service.DeployNotifier).
const (
	EventChannel events.Channel = "cloud"
	// eventType (2.º arg de events.NewEvent / evType de publishDeployEvent): string, NO
	// events.Channel. Channel es el canal de pub/sub ("cloud"); estos son tipos de evento.
	EventDeployReady = "cloud.deploy.ready"
	EventDeployError = "cloud.deploy.error"
)

// publishDeployEvent escribe en el outbox (misma tx) el aviso de una transición
// terminal de deployment. El payload lleva solo el id; el worker resuelve el
// resto (proyecto, commit, URL, destino) bajo el tenant del evento.
func publishDeployEvent(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID, evType string) error {
	ev, err := events.NewEvent(EventChannel, evType, tenant, map[string]any{"deployment_id": id})
	if err != nil {
		return err
	}
	return events.Publish(ctx, tx, ev)
}

// DeployNotifyTarget son los datos que el worker necesita para enviar el aviso
// de un deployment: la URL destino resuelta (proyecto > organización) y los
// campos del cuerpo del webhook.
type DeployNotifyTarget struct {
	WebhookURL  string // "" => sin destino configurado (no se notifica)
	ProjectID   uuid.UUID
	ProjectSlug string
	CommitSHA   string
	URL         string // URL pública de la app/preview
	Target      string
	// CreatedBy es el subject de quien lanzó el deployment: el destinatario de
	// la notificación del Hub (aviso en la topBar y apps nativas).
	CreatedBy string
}

// LoadDeployNotify resuelve el destino del aviso y los datos del deployment
// (cross-tenant: el worker pasa el tenant del evento y se fija app.tenant bajo
// RLS). La URL de webhook cae del proyecto a la organización (COALESCE), ” si
// ninguno la tiene. Devuelve ok=false si el deployment ya no existe (purgado).
func (s *Store) LoadDeployNotify(ctx context.Context, tenant string, deploymentID uuid.UUID) (DeployNotifyTarget, bool, error) {
	var out DeployNotifyTarget
	found := false
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			SELECT COALESCE(NULLIF(p.notify_webhook_url, ''), o.notify_webhook_url, ''),
			       d.project_id, p.slug, d.commit_sha, d.url, d.target, d.created_by
			FROM deployments d
			JOIN projects p ON p.id = d.project_id AND p.tenant = d.tenant
			LEFT JOIN organizations o ON o.tenant = d.tenant
			WHERE d.tenant=$1 AND d.id=$2`, tenant, deploymentID).
			Scan(&out.WebhookURL, &out.ProjectID, &out.ProjectSlug, &out.CommitSHA, &out.URL, &out.Target, &out.CreatedBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmtErr("resolver destino de aviso de deploy", err)
		}
		found = true
		return nil
	})
	return out, found, err
}

// ClaimDeployNotification reclama el envío de una transición (deployment_id,
// status) de forma idempotente: INSERT ON CONFLICT DO NOTHING. Devuelve
// claimed=true solo si esta llamada insertó la fila (no se había enviado). El
// worker envía el webhook únicamente si reclama, y libera (Release) si el envío
// falla para que un reintento del Dispatcher lo re-reclame. Cross-tenant.
func (s *Store) ClaimDeployNotification(ctx context.Context, tenant string, deploymentID uuid.UUID, status, url string) (bool, error) {
	claimed := false
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO deploy_notifications (tenant, deployment_id, status, url)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (deployment_id, status) DO NOTHING`,
			tenant, deploymentID, status, url)
		if err != nil {
			return fmtErr("reclamar aviso de deploy", err)
		}
		claimed = tag.RowsAffected() == 1
		return nil
	})
	return claimed, err
}

// ReleaseDeployNotification libera la reclamación de una transición cuando el
// envío falla, de modo que un reintento vuelva a reclamarla y reenviarla.
func (s *Store) ReleaseDeployNotification(ctx context.Context, tenant string, deploymentID uuid.UUID, status string) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM deploy_notifications WHERE tenant=$1 AND deployment_id=$2 AND status=$3`,
			tenant, deploymentID, status); err != nil {
			return fmtErr("liberar aviso de deploy", err)
		}
		return nil
	})
}
