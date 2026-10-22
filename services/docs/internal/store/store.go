// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store es la capa de persistencia de Sytue Docs sobre PostgreSQL
// (pgx). Todas las consultas son parametrizadas (sin SQL dinámico) y se ejecutan
// dentro de transacciones con el tenant fijado (RLS, platform/pkg/pg.SetTenant),
// de modo que el aislamiento multi-tenant lo refuerza la base de datos.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// appRole es el rol NO superusuario y SIN BYPASSRLS bajo el que se ejecutan
// todas las transacciones de tenant. Fijarlo con SET LOCAL ROLE garantiza que
// las políticas RLS se apliquen SIEMPRE, con independencia de con qué rol se
// conecte el pool (p. ej. un superusuario en dev/CI). Debe coincidir con el rol
// creado en la migración 0001.
const appRole = "docs_app"

// Store agrupa el acceso a datos del servicio.
type Store struct {
	rol                      *pg.AppRole
	pool                     *pg.Pool
	callAccess               CallAccessRevoker
	callIdentity             callaccess.IdentityReader
	documentCallProofEnabled bool
}

// Compile-time: *Store satisface el contrato de persistencia del dominio.
var _ domain.Store = (*Store)(nil)

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{rol: pg.NewAppRole(appRole), pool: pool} }

// Healthz verifica la conectividad con PostgreSQL (sondas /readyz).
//
// Ya NO comprueba aquí el rol de aplicación: eso lo decide ResolveAppRole una vez
// en el arranque, y si el aislamiento no se puede garantizar el servicio no llega
// a servir. Repetirlo en cada sonda solo añadía una transacción cada diez
// segundos para volver a responder lo mismo.
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// ResolveAppRole decide cómo se sostiene el aislamiento por organización y
// verifica que se sostiene de verdad. Se llama en el arranque, tras las
// migraciones y ANTES de servir tráfico; si devuelve error, el servicio no debe
// arrancar. Ver platform/pkg/pg.AppRole para el razonamiento completo.
func (s *Store) ResolveAppRole(ctx context.Context) (string, error) {
	return s.rol.Resolve(ctx, s.pool)
}

// tenantTx ejecuta fn dentro de una transacción con `app.tenant` fijado al
// tenant del contexto y el rol de aplicación (docs_app) activado con SET LOCAL
// ROLE, de modo que las políticas RLS aíslan las filas SIEMPRE — incluso si el
// pool se conecta con un superusuario (que de otro modo saltaría RLS).
func (s *Store) tenantTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// SET LOCAL ROLE: solo vive dentro de esta transacción. El identificador
		// del rol es una constante del paquete (no entra input externo).
		// El rol solo se asume si Resolve comprobó que se PUEDE. Cuando no, el
		// aislamiento lo sostiene la RLS forzada sobre un rol sin superusuario ni
		// BYPASSRLS, que Resolve verifica antes de permitirlo; ver pg.AppRole.
		if s.rol.Activo() {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+appRole); err != nil {
				return fmt.Errorf("store: fijar rol de aplicación: %w", err)
			}
		}
		if err := fn(tx); err != nil {
			return err
		}
		return s.flushCallRevocations(ctx, tx)
	})
}

// isNoRows normaliza el "sin filas" de pgx.
func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
