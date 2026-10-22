// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store es la persistencia de Sytue Web sobre PostgreSQL (pgx). Una
// petición, una transacción (CONVENTIONS §13.3), y cada función decide antes de
// tocar nada desde dónde lee:
//
//   - enTenant: la organización del sitio (`app.tenant`), lectura y escritura. El
//     tenant va ADEMÁS en el WHERE de cada consulta: la RLS es la segunda barrera, no
//     la única (no se evalúa para un rol que la elude), y de paso el planificador usa
//     los índices que empiezan por `tenant`. Lo que cuelga de un sitio entra por enSitio.
//   - enPublico: `app.pages_publico='on'`, solo lectura de `sites` y
//     `retired_slugs` (y `domains`, cuando exista), para resolver Host → sitio y la
//     disponibilidad de un slug sin sesión.
//   - enMantenimiento: `app.maintenance='on'`, cruzando organizaciones: el mapa, los
//     barridos, la CLI y SOLO dos funciones que llama la capa HTTP, TenantDePago y
//     TenantDeSitio (los webhooks llegan sin sesión y sin Host de la web). Lo vigila
//     TestMantenimientoSoloDondeToca.
//
// Consultas siempre parametrizadas y estáticas (CONVENTIONS §10). Los ficheros de
// cada área (tienda, dominios, IA…) los crea su dueño en este paquete.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// Store agrupa el acceso a datos de Sytue Web.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// DB expone el pool para lo que habla con Postgres sin pasar por el dominio: los
// limitadores compartidos entre réplicas (pg.RateLimiter) y el outbox.
func (s *Store) DB() *pg.Pool { return s.pool }

// enTenant y enPublico los usan los ficheros de cada área, que llegan en las PR
// siguientes (PAGES-ENCARGO §18.2); hoy solo los prueba el test de integración.
//
//nolint:unused // ver arriba: el esqueleto los deja escritos para que nadie los copie.
func (s *Store) enTenant(ctx context.Context, tenant string, fn func(tx pgx.Tx) error) error {
	if s.pool == nil || tenant == "" {
		return domain.ErrNoExiste
	}
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		return fn(tx)
	})
}

// enSitio es enTenant para lo que cuelga de UN sitio (tienda, pasarelas, pedidos, dominios,
// trabajos de IA): antes de nada comprueba, en la MISMA transacción, que ese sitio es de
// esa organización. No basta con que la consulta lleve `site_id`: el identificador llega de
// la ruta, y con la RLS sin evaluar —un rol de conexión que la elude— cualquier
// organización operaba la tienda de un sitio ajeno con solo conocer su UUID (WEB-14). Un
// sitio en la papelera sigue siendo de su organización: sus pagos pendientes se cierran.
func (s *Store) enSitio(ctx context.Context, tenant string, sitio uuid.UUID, fn func(tx pgx.Tx) error) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var suyo bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sites WHERE id = $1 AND tenant = $2)`, sitio, tenant).Scan(&suyo); err != nil {
			return err
		}
		if !suyo {
			return domain.ErrNoExiste
		}
		return fn(tx)
	})
}

// enPublico es la ÚNICA función que fija `app.pages_publico`. Sus políticas son
// FOR SELECT: una escritura aquí no ve filas y la rechaza el WITH CHECK.
//
//nolint:unused // ver enTenant.
func (s *Store) enPublico(ctx context.Context, fn func(tx pgx.Tx) error) error {
	if s.pool == nil {
		return domain.ErrNoExiste
	}
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.pages_publico', 'on', true)"); err != nil {
			return fmt.Errorf("pages: set_config app.pages_publico: %w", err)
		}
		return fn(tx)
	})
}

func (s *Store) enMantenimiento(ctx context.Context, fn func(tx pgx.Tx) error) error {
	if s.pool == nil {
		return domain.ErrNoExiste
	}
	return s.pool.InMaintenanceTx(ctx, fn)
}

// TenantDeSitio es la organización de un sitio, para los webhooks que llegan por
// `site_id` (Adyen y Stripe). Todo lo demás del webhook va después en enTenant.
func (s *Store) TenantDeSitio(ctx context.Context, sitio uuid.UUID) (string, error) {
	var tenant string
	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT tenant FROM sites WHERE id = $1`, sitio).Scan(&tenant)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNoExiste
	}
	return tenant, err
}

// TenantDePago es la organización y el sitio de un pago, para los webhooks que
// llegan por `payment_id` (Mollie y Redsys). Las tablas `payments` y `orders` las
// trae la migración de la tienda.
func (s *Store) TenantDePago(ctx context.Context, pago uuid.UUID) (tenant string, sitio uuid.UUID, err error) {
	err = s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT p.tenant, o.site_id
			  FROM payments p JOIN orders o ON o.id = p.order_id AND o.tenant = p.tenant
			 WHERE p.id = $1`, pago).Scan(&tenant, &sitio)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", uuid.Nil, domain.ErrNoExiste
	}
	return tenant, sitio, err
}

// Suspender deja un sitio en `suspended` por abuso (`pages suspender`): la web deja
// de servirse en el siguiente refresco del mapa y no admite escrituras. Vale aunque
// esté borrado: restaurarlo no puede levantar la suspensión.
func (s *Store) Suspender(ctx context.Context, slug, motivo string) error {
	return s.cambiarEstado(ctx, slug, "suspended", motivo)
}

// Reactivar levanta la suspensión (`pages reactivar`). No republica nada.
func (s *Store) Reactivar(ctx context.Context, slug string) error {
	return s.cambiarEstado(ctx, slug, "active", "")
}

func (s *Store) cambiarEstado(ctx context.Context, slug, estado, motivo string) error {
	return s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE sites SET status = $2, suspended_reason = $3, updated_at = now()
			 WHERE slug = $1`, slug, estado, motivo)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}
