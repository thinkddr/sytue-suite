// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"time"
)

// TRABAJOS MASIVOS VISIBLES Y CANCELABLES (mig 0093).
//
// El trabajo corre DENTRO de la petición, como antes; lo que cambia es que su
// progreso se escribe elemento a elemento, así que otra petición puede mirarlo
// y puede pedir que se pare. No hay cola ni worker: doscientas personas caben
// de sobra en el plazo de la petición, y una cola para eso sería infraestructura
// nueva que mantener a cambio de nada.

// LatidoMaximo es cuánto puede estar un trabajo sin dar señales antes de darlo
// por interrumpido. Dos minutos: bastante más de lo que tarda un lote entero, y
// lo bastante poco para no dejar una fila «ejecutando» eternamente cuando la
// petición muere con el pod.
const LatidoMaximo = 2 * time.Minute

// TrabajoMasivo es un lote tal como se mira desde fuera.
type TrabajoMasivo struct {
	ID          string
	Accion      string
	Total       int
	Hechos      int
	Estado      string
	Cancelando  bool
	Resultados  json.RawMessage
	CreadoPor   string
	CreadoEn    time.Time
	Latido      time.Time
	TerminadoEn *time.Time
}

// EstadoVisible resuelve el estado MOSTRADO: un trabajo «ejecutando» que lleva
// sin latir más de LatidoMaximo está interrumpido, no corriendo. Es la misma
// idea que la expiración perezosa del PIM: no se inventa que terminó ni se
// finge que sigue.
func (t TrabajoMasivo) EstadoVisible(ahora time.Time) string {
	if t.Estado == "ejecutando" && ahora.Sub(t.Latido) > LatidoMaximo {
		return "interrumpido"
	}
	return t.Estado
}

// CrearTrabajo abre el lote antes de tocar a nadie: si se abriera al final, un
// trabajo en curso no existiría para nadie y no habría qué cancelar.
func (s *Store) CrearTrabajo(ctx context.Context, orgID, accion, porID string, total int) (string, error) {
	var id string
	err := s.DB().QueryRow(ctx, `
		INSERT INTO bulk_jobs (org_id, accion, total, creado_por)
		VALUES ($1, $2, $3, $4) RETURNING id::text`, orgID, accion, total, nilSiVacio(porID)).Scan(&id)
	return id, errf("CrearTrabajo", err)
}

// AvanzarTrabajo apunta un elemento hecho y devuelve si hay que PARAR.
//
// El avance y la lectura de la cancelación van en la MISMA sentencia: hacerlo
// en dos sería una carrera contra quien cancela justo en medio, y el precio de
// perderla es un elemento de más tocado.
func (s *Store) AvanzarTrabajo(ctx context.Context, id string, resultados any) (bool, error) {
	crudo, err := json.Marshal(resultados)
	if err != nil {
		return false, errf("AvanzarTrabajo.marshal", err)
	}
	var cancelar bool
	err = s.DB().QueryRow(ctx, `
		UPDATE bulk_jobs
		   SET hechos = hechos + 1, resultados = $2::jsonb, latido = now()
		 WHERE id = $1::uuid AND estado = 'ejecutando'
		RETURNING cancelacion_pedida`, id, crudo).Scan(&cancelar)
	return cancelar, errf("AvanzarTrabajo", err)
}

// CerrarTrabajo lo da por terminado (o por cancelado) con su informe final.
func (s *Store) CerrarTrabajo(ctx context.Context, id, estado string, resultados any) error {
	crudo, err := json.Marshal(resultados)
	if err != nil {
		return errf("CerrarTrabajo.marshal", err)
	}
	_, err = s.DB().Exec(ctx, `
		UPDATE bulk_jobs
		   SET estado = $2, resultados = $3::jsonb, latido = now(), terminado_en = now()
		 WHERE id = $1::uuid`, id, estado, crudo)
	return errf("CerrarTrabajo", err)
}

// PedirCancelacionTrabajo marca el lote para que su bucle pare. Devuelve false
// si el trabajo no está en esta organización o ya había terminado: cancelar lo
// terminado no es un error, es que no hay nada que cancelar.
func (s *Store) PedirCancelacionTrabajo(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `
		UPDATE bulk_jobs SET cancelacion_pedida = true
		 WHERE id = $1::uuid AND org_id = $2 AND estado = 'ejecutando'`, id, orgID)
	if err != nil {
		return false, errf("PedirCancelacionTrabajo", err)
	}
	return tag.RowsAffected() > 0, nil
}

const trabajoCols = `id::text, accion, total, hechos, estado, cancelacion_pedida,
	resultados, coalesce(creado_por::text,''), creado_en, latido, terminado_en`

// Trabajo lee UNO, acotado por organización (un id de otra org no liga).
func (s *Store) Trabajo(ctx context.Context, orgID, id string) (TrabajoMasivo, error) {
	var t TrabajoMasivo
	err := s.DB().QueryRow(ctx, `SELECT `+trabajoCols+`
		FROM bulk_jobs WHERE id = $1::uuid AND org_id = $2`, id, orgID).
		Scan(&t.ID, &t.Accion, &t.Total, &t.Hechos, &t.Estado, &t.Cancelando,
			&t.Resultados, &t.CreadoPor, &t.CreadoEn, &t.Latido, &t.TerminadoEn)
	return t, errf("Trabajo", err)
}

// Trabajos lista los últimos de la organización. Tope de 50: es un historial
// para mirar lo de hoy, no un archivo.
func (s *Store) Trabajos(ctx context.Context, orgID string) ([]TrabajoMasivo, error) {
	rows, err := s.DB().Query(ctx, `SELECT `+trabajoCols+`
		FROM bulk_jobs WHERE org_id = $1 ORDER BY creado_en DESC LIMIT 50`, orgID)
	if err != nil {
		return nil, errf("Trabajos", err)
	}
	defer rows.Close()
	out := make([]TrabajoMasivo, 0, 16)
	for rows.Next() {
		var t TrabajoMasivo
		if err := rows.Scan(&t.ID, &t.Accion, &t.Total, &t.Hechos, &t.Estado, &t.Cancelando,
			&t.Resultados, &t.CreadoPor, &t.CreadoEn, &t.Latido, &t.TerminadoEn); err != nil {
			return nil, errf("Trabajos.scan", err)
		}
		out = append(out, t)
	}
	return out, errf("Trabajos.iter", rows.Err())
}
