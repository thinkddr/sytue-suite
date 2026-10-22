// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Búsqueda y recolección de los logs de runtime (WS4 / Ola O1).
//
// El tail en vivo (SSE) solo sirve a quien está mirando la pantalla en ese
// momento: lo que escribió la app anoche no está en ningún sitio donde buscarlo.
// Esto lo arregla por los dos extremos: un recolector de fondo que va guardando
// las líneas (RunLogCollector) y una consulta por texto + rango de tiempo con
// paginación keyset (SearchLogs). La retención la aplica el cron de purga.
package service

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// Cadencia del recolector de logs.
const (
	// logCollectTick es cada cuánto despierta el worker a buscar trabajo.
	logCollectTick = 30 * time.Second
	// logCollectDue es el intervalo EFECTIVO por deployment: un pod no se lee más
	// de una vez por minuto. Sube el retraso máximo de la búsqueda a ~1 min y
	// divide por dos las llamadas al API de Kubernetes respecto al tick.
	logCollectDue = time.Minute
	// logCollectBatch es cuántos deployments reclama una pasada.
	logCollectBatch = 50
	// logCollectMaxLines acota lo que se lee de UN pod en UNA pasada (cota de
	// memoria y de coste de escritura). El resto se lee en las siguientes: el
	// corte avanza con lo guardado, así que no deja huecos, solo retraso.
	logCollectMaxLines = 2000
	// logCollectBackfill es lo que se mira hacia atrás la PRIMERA vez que se
	// recolecta un deployment. Corto a propósito: al desplegar interesa lo que
	// está pasando, no re-ingerir el historial entero que el pod tenga guardado.
	logCollectBackfill = 5 * time.Minute
)

// RunLogCollector arranca el recolector de logs de runtime (sin estado,
// cross-réplica): cada tick RECLAMA deployments vivos con FOR UPDATE SKIP LOCKED
// (reparte entre réplicas y los arrienda), lee del runtime lo escrito desde el
// último corte y lo persiste. Bloquea hasta que se cancele ctx. Lo arranca
// cmd/cloud-api. Sin streamer cableado (dev sin clúster) no hace nada.
func (s *Service) RunLogCollector(ctx context.Context) error {
	if s.logStreamer == nil {
		return nil
	}
	ticker := time.NewTicker(logCollectTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.collectLogsOnce(ctx, time.Now()); err != nil {
				log.From(ctx).Warn("tick del recolector de logs falló", "error", err)
			}
		}
	}
}

// collectLogsOnce hace UNA pasada: reclamar, leer y guardar. El error de un
// deployment concreto no aborta la pasada —un pod reiniciándose o ya borrado es
// lo normal, no una avería— y solo se propaga el fallo de la reclamación, que sí
// significa que la base de datos no está.
//
// ponytail: recorre el lote en serie; el techo es logCollectBatch × latencia de
// Kubernetes por tick y por réplica (SKIP LOCKED reparte entre réplicas). Si un
// tick no vacía la cola, paralelizar el lote como hace el prober de uptime.
func (s *Service) collectLogsOnce(ctx context.Context, now time.Time) error {
	targets, err := s.pstore.ClaimLogCollection(ctx, now, logCollectDue, logCollectBatch)
	if err != nil {
		return err
	}
	for _, t := range targets {
		since := t.Since
		if since.IsZero() {
			since = now.Add(-logCollectBackfill)
		}
		lines, err := s.logStreamer.CollectDeploymentLogs(ctx, t.Tenant, t.ID, since, logCollectMaxLines)
		if err != nil {
			log.From(ctx).Debug("no se pudieron leer logs del deployment",
				"deployment_id", t.ID, "error", err)
			continue
		}
		if _, err := s.pstore.AppendRuntimeLogs(ctx, t.Tenant, t.ProjectID, t.ID, lines); err != nil {
			log.From(ctx).Warn("guardar logs de runtime falló",
				"deployment_id", t.ID, "error", err)
		}
	}
	return nil
}

// LogPage es una página de logs con el token de continuación ya codificado
// (opaco) para la siguiente petición ("" si no hay más).
type LogPage struct {
	Lines      []domain.LogLine
	NextCursor string
}

// SearchLogs busca en los logs de runtime de un proyecto por texto y rango de
// tiempo, con paginación keyset (el cursor es el token opaco de la página
// anterior; vacío = primera página). El aislamiento por organización lo da el
// store con el tenant del contexto, nunca un parámetro de la petición.
func (s *Service) SearchLogs(ctx context.Context, projectID uuid.UUID, f store.RuntimeLogFilter, cursor string, limit int) (LogPage, error) {
	after, err := decodePageCursor(cursor)
	if err != nil {
		return LogPage{}, err
	}
	page, err := s.pstore.SearchRuntimeLogs(ctx, projectID, f, after, limit)
	if err != nil {
		return LogPage{}, err
	}
	return LogPage{Lines: page.Lines, NextCursor: encodePageCursor(page.Next)}, nil
}

// ExportLogs descarga el registro filtrado como texto plano.
//
// Buscar en pantalla ya se podía; llevárselo, no. Y es lo que se necesita
// justo cuando hace falta de verdad: adjuntar lo que pasó a un ticket, pasárselo
// a quien no tiene cuenta en el panel, o guardarlo antes de que la retención se
// lo lleve. El filtro es el MISMO de la búsqueda —lo que se descarga es lo que
// se está viendo, no «todo»—, sin cursor: la página es de la pantalla, no del
// fichero.
func (s *Service) ExportLogs(ctx context.Context, projectID uuid.UUID, f store.RuntimeLogFilter, w io.Writer) error {
	return s.pstore.ExportRuntimeLogs(ctx, projectID, f, w)
}
