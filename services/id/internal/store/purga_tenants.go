// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// --- Purga del contenido de una organización (mig 0129, ADR 0054) ------------
//
// La cola la llena la purga de una cuenta personal (PurgarCuentaPropia) y la
// vacía el barrido de Sytue ID (http.BarrerPurgasDeTenant), servicio a servicio.
// Aquí solo vive el estado: qué falta y cuándo se reintenta.

// Motivos de una purga de organización.
const (
	PurgaCuentaBorrada = "account_deleted"
	PurgaSupresionRGPD = "dsr_erasure"
)

// ProgresoServicio es cómo va la purga en UN servicio. Se guarda en `progress`.
type ProgresoServicio struct {
	// BaseVacia: la base ya no guarda filas del tenant.
	BaseVacia bool `json:"db_done,omitempty"`
	// Hecho: base vacía y objetos borrados (o el servicio no tiene).
	Hecho bool `json:"done,omitempty"`
	// SinBase: la base no existe en este despliegue (servicio apagado).
	SinBase bool `json:"no_database,omitempty"`
	Filas   int  `json:"rows,omitempty"`
	// Retenido: lo que impide terminar (retención legal, un guardián de
	// integridad), en prosa.
	Retenido string `json:"retained,omitempty"`
	// Error del último intento.
	Error string `json:"error,omitempty"`
	// EsperaObjetos: la base está vacía y el servicio aún no ha borrado sus objetos.
	EsperaObjetos bool       `json:"waiting_objects,omitempty"`
	Actualizado   *time.Time `json:"updated_at,omitempty"`
}

// PurgaDeTenant es una fila de la cola.
type PurgaDeTenant struct {
	OrgID       string
	Slug        string
	Motivo      string
	Solicitada  time.Time
	Completada  *time.Time
	Intentos    int
	UltimoError string
	Progreso    map[string]ProgresoServicio
}

// EncolarPurgaDeTenant deja la organización en la cola. Idempotente: una que ya
// estaba no cambia (ni su motivo ni su progreso).
func (s *Store) EncolarPurgaDeTenant(ctx context.Context, q pg.Querier, orgID, motivo string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `
		INSERT INTO tenant_purges (org_id, slug, reason)
		SELECT id, slug, $2 FROM organizations WHERE id = $1
		ON CONFLICT (org_id) DO NOTHING`, orgID, motivo)
	return errf("EncolarPurgaDeTenant", err)
}

// ReclamarPurgasDeTenant toma hasta `limite` purgas pendientes cuyo turno ha
// llegado y adelanta su próximo intento `arriendo`: otra réplica no las verá
// hasta entonces. Si esta se cae a mitad, la fila vuelve sola.
//
// En orden aleatorio, como las bajas propias: una purga que falla siempre no
// debe tapar a las demás.
func (s *Store) ReclamarPurgasDeTenant(ctx context.Context, limite int, arriendo time.Duration) ([]PurgaDeTenant, error) {
	rows, err := s.DB().Query(ctx, `
		UPDATE tenant_purges SET attempts = attempts + 1, next_attempt_at = now() + $2::interval
		WHERE org_id IN (
		    SELECT org_id FROM tenant_purges
		    WHERE completed_at IS NULL AND next_attempt_at <= now()
		    ORDER BY random() LIMIT $1 FOR UPDATE SKIP LOCKED)
		RETURNING org_id::text, slug, reason, requested_at, completed_at, attempts, last_error, progress`,
		limite, intervaloPG(arriendo))
	if err != nil {
		return nil, errf("ReclamarPurgasDeTenant", err)
	}
	defer rows.Close()
	var out []PurgaDeTenant
	for rows.Next() {
		p, err := escanearPurga(rows)
		if err != nil {
			return nil, errf("ReclamarPurgasDeTenant.scan", err)
		}
		out = append(out, p)
	}
	return out, errf("ReclamarPurgasDeTenant.rows", rows.Err())
}

// GuardarPurgaDeTenant escribe el progreso de un intento. Con `completa`, la
// cierra; si no, la deja para dentro de `espera`.
func (s *Store) GuardarPurgaDeTenant(ctx context.Context, orgID string, progreso map[string]ProgresoServicio, completa bool, espera time.Duration, ultimoError string) error {
	raw, err := json.Marshal(progreso)
	if err != nil {
		return errf("GuardarPurgaDeTenant", err)
	}
	_, err = s.DB().Exec(ctx, `
		UPDATE tenant_purges SET progress = $2, last_error = $3,
		       completed_at = CASE WHEN $4 THEN now() ELSE NULL END,
		       next_attempt_at = now() + $5::interval
		WHERE org_id = $1 AND completed_at IS NULL`,
		orgID, raw, ultimoError, completa, intervaloPG(espera))
	return errf("GuardarPurgaDeTenant", err)
}

// PurgaDeTenantDe devuelve la purga de una organización, o nil si no hay.
func (s *Store) PurgaDeTenantDe(ctx context.Context, orgID string) (*PurgaDeTenant, error) {
	rows, err := s.DB().Query(ctx, `SELECT org_id::text, slug, reason, requested_at, completed_at, attempts, last_error, progress
		FROM tenant_purges WHERE org_id = $1`, orgID)
	if err != nil {
		return nil, errf("PurgaDeTenantDe", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errf("PurgaDeTenantDe.rows", rows.Err())
	}
	p, err := escanearPurga(rows)
	if err != nil {
		return nil, errf("PurgaDeTenantDe.scan", err)
	}
	return &p, nil
}

type escaneable interface{ Scan(dest ...any) error }

func escanearPurga(r escaneable) (PurgaDeTenant, error) {
	var p PurgaDeTenant
	var raw []byte
	if err := r.Scan(&p.OrgID, &p.Slug, &p.Motivo, &p.Solicitada, &p.Completada, &p.Intentos, &p.UltimoError, &raw); err != nil {
		return p, err
	}
	p.Progreso = map[string]ProgresoServicio{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p.Progreso); err != nil {
			return p, err
		}
	}
	return p, nil
}

// intervaloPG escribe una duración como intervalo de PostgreSQL, en segundos.
func intervaloPG(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d seconds", int64(d/time.Second))
}
