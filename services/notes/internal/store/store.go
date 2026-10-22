// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia de Sytue Notepads sobre PostgreSQL
// (pgx). Todo acceso pasa por transacciones con `app.tenant` fijado
// (pg.InTenantTx) para que RLS aísle por organización. Consultas SIEMPRE
// parametrizadas (sin SQL dinámico), conforme a CONVENTIONS §10.
//
// RLS separa ORGANIZACIONES; separar PERSONAS dentro de una organización es
// trabajo de las cláusulas de visibilidad de este paquete (`accesoLectura`), que
// se aplican en cada consulta y no en el que llama.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Store agrupa el acceso a datos de Sytue Notepads.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// DB expone el pool para lo que necesita hablar con Postgres SIN pasar por una
// consulta del dominio: hoy, el limitador compartido entre réplicas del enlace
// público (pg.RateLimiter). Mismo nombre y mismo motivo que en Sytue ID.
//
// No abre una puerta a saltarse el store: quien lo use sigue teniendo que
// escoger la transacción con RLS que le corresponda. Lo que evita es tener que
// pasar el pool por separado a NewRouter, que ya recibe el Store.
func (s *Store) DB() *pg.Pool { return s.pool }

// isNoRows es un atajo para pgx.ErrNoRows.
func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// uniqueViolation indica si el error es una violación de unicidad (23505).
func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// checkViolation indica si el error es una violación de CHECK (23514). Las
// invariantes duras del esquema —cifrado sin indexar, o en claro o cifrado— viven
// ahí, así que un 23514 es entrada inválida y no un fallo del servidor.
func checkViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}

// REGLAS DE VISIBILIDAD (se repiten literalmente en cada consulta, con sus $n)
//
//	lectura de cuaderno:  nb.owner_sub = $sub OR nb.visibility IN ('organization','public')
//	                      OR EXISTS (SELECT 1 FROM notebook_lectores l WHERE l.notebook_id = nb.id AND l.sujeto = $sub)
//	                      OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $sub)
//	                      (compartido con esa persona, para leer o para editar: 0010)
//	escritura de cuaderno: nb.owner_sub = $sub OR nb.visibility = 'organization'
//	                       OR EXISTS (SELECT 1 FROM notebook_editores e WHERE e.notebook_id = nb.id AND e.sujeto = $sub)
//	                       (solo para sus NOTAS: el cuaderno en sí —título, visibilidad, subcuadernos— sigue
//	                       siendo del dueño; una persona con edición concedida no puede abrirlo a toda la
//	                       organización)
//	visibilidad de nota:  n.visibility IS NULL OR n.visibility <> 'private' OR n.owner_sub = $sub
//	nota compartida:      EXISTS (SELECT 1 FROM note_personas c WHERE c.note_id = n.id AND c.sujeto = $sub)
//	                      solo la lee «Compartido conmigo» (SharedNotes) y, con `puede_editar`,
//	                      la escribe UpdateNote; ninguna otra consulta la mira
//
// Van escritas a mano en cada SQL y no compuestas con Sprintf: el SQL de este
// paquete es estático de principio a fin, que es lo que permite leerlo y auditarlo
// sin ejecutarlo. Un cuaderno PÚBLICO se lee pero no se escribe: publicar es dar
// de leer.
