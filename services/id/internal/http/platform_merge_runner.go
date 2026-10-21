// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/merge"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Ejecución de una fusión. Se lanza en segundo plano porque mueve las filas de
// diez bases y puede tardar minutos: dejar la petición HTTP abierta ataría el
// resultado a que no se caiga la conexión del navegador. El estado vive en
// `org_merges`, así que la consola pregunta por él y un fallo se reanuda.
//
// NO se hereda el contexto de la petición: cancelarlo al responder 202 abortaría
// la fusión a mitad. Se usa un contexto propio con un tope generoso.
const mergeTimeout = 30 * time.Minute

// runMerge arranca la fusión en segundo plano. Idempotente frente a llamadas
// solapadas: StartOrgMerge solo transiciona desde pending/failed, así que una
// segunda petición sobre una fusión ya en curso no lanza un segundo proceso.
func (s *Server) runMerge(mergeID, sourceSlug, targetSlug, sourceOrgID, targetOrgID string) {
	ctx := context.Background()
	if err := s.store.StartOrgMerge(ctx, mergeID); err != nil {
		s.log.Warn("merge: no se pudo marcar en curso (¿ya lo estaba?)",
			"merge_id", mergeID, "error", err)
		return
	}
	go func() { //nolint:contextcheck // contexto propio deliberado: heredar el de la petición abortaría la fusión al responder 202.
		ctx, cancel := context.WithTimeout(context.Background(), mergeTimeout)
		defer cancel()
		if err := s.executeMerge(ctx, mergeID, sourceSlug, targetSlug, sourceOrgID, targetOrgID); err != nil {
			s.log.Error("merge: fallo", "merge_id", mergeID, "error", err)
			if err := s.store.FinishOrgMerge(context.Background(), mergeID, err.Error()); err != nil {
				s.log.Error("merge: no se pudo registrar el fallo", "merge_id", mergeID, "error", err)
			}
			return
		}
		if err := s.store.FinishOrgMerge(context.Background(), mergeID, ""); err != nil {
			s.log.Error("merge: no se pudo cerrar", "merge_id", mergeID, "error", err)
		}
		s.log.Info("merge: completada", "merge_id", mergeID,
			"origen", sourceSlug, "destino", targetSlug)
	}()
}

// auditMerge deja constancia de UN paso de la fusión en el registro de
// plataforma. Va sin actor a propósito: no lo decidió una persona en esta
// petición, lo decidió la regla que ese servicio tiene escrita, y quién pidió la
// fusión ya consta en `platform.merge.requested` con el mismo identificador
// (`target` = id de la fusión, que es lo que ata todas estas filas entre sí).
//
// No se puede reutilizar auditPlatform: la fusión corre en segundo plano, sin
// `*http.Request` del que sacar la IP.
//
// Un fallo al auditar NO aborta la fusión: dejar los datos a medias por no poder
// escribir una fila de registro sería peor que la fusión sin registro, y el
// progreso real se puede reconstruir desde `org_merges`. Queda en el log a nivel
// error, que es lo que dispara la alerta.
func (s *Server) auditMerge(ctx context.Context, mergeID, orgID, action, service string, detail map[string]any) {
	d := map[string]any{
		"at":      time.Now().UTC().Format(time.RFC3339),
		"service": service,
	}
	for k, v := range detail {
		d[k] = v
	}
	err := s.store.Audit(ctx, nil, store.AuditEntry{
		OrgID:  orgID,
		Action: action,
		Target: mergeID,
		Detail: d,
	})
	if err != nil {
		s.log.Error("merge: no se pudo auditar el paso",
			"merge_id", mergeID, "accion", action, "servicio", service, "error", err)
	}
}

// executeMerge mueve primero los DATOS de los diez servicios y solo al final la
// IDENTIDAD. El orden importa: mientras el origen siga existiendo como
// organización activa, un fallo a mitad deja a sus usuarios entrando en una
// organización con parte de sus datos ya movidos, pero SIGUEN pudiendo entrar y
// la fusión se reanuda. Al revés —identidad primero— un fallo dejaría a esos
// usuarios en el destino sin sus datos y sin forma de volver.
func (s *Server) executeMerge(ctx context.Context, mergeID, sourceSlug, targetSlug, sourceOrgID, targetOrgID string) error {
	// Preflight antes de tocar la primera fila: comprueba que las diez bases son
	// alcanzables y que ninguna tabla se quedaría fuera por falta de política.
	if err := merge.Preflight(ctx, s.cfg.DatabaseDSN); err != nil {
		return err
	}
	for _, svc := range merge.Services {
		if err := s.store.MarkMergeService(ctx, mergeID, svc, "running"); err != nil {
			return err
		}
		dsn, err := merge.DSNForDatabase(s.cfg.DatabaseDSN, svc)
		if err != nil {
			return err
		}
		pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 2})
		if err != nil {
			return err
		}
		res, err := merge.MoveService(ctx, pool, sourceSlug, targetSlug, s.documentCallRevoker())
		pool.Close()
		if err != nil {
			_ = s.store.MarkMergeService(ctx, mergeID, svc, "failed")
			return err
		}
		if err := s.store.MarkMergeService(ctx, mergeID, svc, "completed"); err != nil {
			return err
		}
		// Lo que el gancho de la base decidió descartar se registra con nombre y
		// motivo. Es material de un cliente: que la decisión sea correcta no la
		// hace invisible.
		if res.Prepared != "" {
			s.log.Warn("merge: material descartado antes de mover", "merge_id", mergeID,
				"servicio", svc, "detalle", res.Prepared)
			s.auditMerge(ctx, mergeID, sourceOrgID, "platform.merge.discarded", svc,
				map[string]any{"reason": res.Prepared})
		}
		// QUÉ se movió y CUÁNDO, tabla a tabla, en el registro de plataforma y no
		// solo en el log de la aplicación. El log rota y no es consultable desde la
		// consola; a quien fusiona le van a preguntar meses después cuántas filas de
		// qué se reetiquetaron, y `org_merges` solo guarda un estado por servicio.
		//
		// Se escribe también cuando no se movió nada: un cero es la prueba de que el
		// servicio se visitó, y es justo lo que hay que poder distinguir de «no se
		// intentó» cuando se investiga una fusión reanudada.
		s.auditMerge(ctx, mergeID, sourceOrgID, "platform.merge.moved", svc,
			map[string]any{"rows": res.Rows, "total": res.Total})
		s.log.Info("merge: servicio movido", "merge_id", mergeID, "servicio", svc,
			"filas", res.Total)
	}
	// VERIFICACIÓN ANTES DE LA IDENTIDAD, que es el punto de no retorno.
	//
	// Aquí los datos ya se movieron pero el origen SIGUE siendo una organización
	// operativa: sus usuarios entran, y la fusión se puede reanudar o dejar
	// parada sin que nadie se quede fuera. En cuanto se mueve la identidad, esos
	// usuarios pasan al destino y el origen queda 'merged'; descubrir entonces
	// que quedaron filas detrás significa tener datos de un cliente en una
	// organización a la que ya no pertenece nadie.
	//
	// Que los diez MoveService no dieran error no demuestra que no quede nada:
	// declaran movido lo que vieron al abrir su transacción, y el origen estuvo
	// escribiendo todo el rato. Por eso se cuenta, y por eso se cuenta aquí.
	restos, err := merge.Verify(ctx, s.cfg.DatabaseDSN, sourceSlug)
	if err != nil {
		return err
	}
	if len(restos) > 0 {
		detalle := make([]map[string]any, 0, len(restos))
		for _, r := range restos {
			detalle = append(detalle, map[string]any{
				"service": r.Service, "table": r.Table, "rows": r.Rows,
			})
		}
		s.auditMerge(ctx, mergeID, sourceOrgID, "platform.merge.leftover", "verify",
			map[string]any{"leftovers": detalle})
		// Se para SIN tocar la identidad. Reanudar vuelve a mover los servicios
		// —el criterio es `tenant = origen`, así que barre lo que llegó tarde— y
		// vuelve a verificar. Si insiste, hay algo escribiendo en el origen y eso
		// lo tiene que mirar una persona, no un reintento.
		return merge.LeftoversError(restos)
	}
	s.auditMerge(ctx, mergeID, sourceOrgID, "platform.merge.verified", "verify",
		map[string]any{"leftovers": 0})

	if err := s.store.MarkMergeService(ctx, mergeID, "identity", "running"); err != nil {
		return err
	}
	moved, err := s.store.MergeIdentity(ctx, sourceOrgID, targetOrgID)
	if err != nil {
		_ = s.store.MarkMergeService(ctx, mergeID, "identity", "failed")
		return err
	}
	s.log.Info("merge: identidad movida", "merge_id", mergeID, "miembros", moved)
	s.auditMerge(ctx, mergeID, sourceOrgID, "platform.merge.moved", "identity",
		map[string]any{"members": moved})
	return s.store.MarkMergeService(ctx, mergeID, "identity", "completed")
}
