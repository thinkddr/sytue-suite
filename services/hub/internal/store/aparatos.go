// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// # Aparatos de iOS a los que despertar (mig. 0010)
//
// El registro único de tokens APNs de las quince apps nativas. Lo usan las rutas
// `/api/v1/me/push/apns` (registrar y olvidar) y el despertador
// (internal/despertar), que reserva aquí cada envío para que con varias réplicas
// salga como mucho uno cada `ventana` por aparato.

// TopeAparatosPorPersona es cuántos aparatos guarda una persona en una organización:
// quince apps en dos o tres aparatos. Al pasarlo se olvidan los que llevan más tiempo
// sin registrarse, así que un cliente que se registre en bucle no llena la tabla.
const TopeAparatosPorPersona = 45

// Aparato es una app de la suite en un aparato de iOS: dónde mandar el despertar.
type Aparato struct {
	Tenant string
	Bundle string
	Token  string
}

// RegistrarAparato guarda (o renueva) el token de la app `bundle` de la persona. El
// mismo token registrado por otra persona pasa a ser de esta: es el aparato, que se
// ha reinstalado o ha cambiado de cuenta.
func (s *Store) RegistrarAparato(ctx context.Context, subject, bundle, token string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO aparatos_apns (tenant, subject, bundle, token)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (tenant, bundle, token)
			 DO UPDATE SET subject = EXCLUDED.subject, visto = now()`,
			t.Slug, subject, bundle, token); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`DELETE FROM aparatos_apns WHERE tenant = $1 AND ctid IN (
			     SELECT ctid FROM aparatos_apns WHERE tenant = $1 AND subject = $2
			     ORDER BY visto DESC OFFSET $3)`,
			t.Slug, subject, TopeAparatosPorPersona)
		return err
	})
}

// OlvidarAparatos borra los aparatos de la persona: con `token`, los de ese token (de
// cualquiera de sus apps); vacío, TODOS los suyos en esta organización. Idempotente.
func (s *Store) OlvidarAparatos(ctx context.Context, subject, token string) (int64, error) {
	t := tenancy.MustFrom(ctx)
	var n int64
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`DELETE FROM aparatos_apns
			 WHERE tenant = $1 AND subject = $2 AND ($3 = '' OR token = $3)`,
			t.Slug, subject, token)
		n = tag.RowsAffected()
		return err
	})
	return n, err
}

// ReservarDespertar apunta un cambio para los aparatos de la persona con uno de esos
// bundles y RESERVA los que pueden despertarse ya: los que no han recibido nada en la
// última `ventana`. Esos se devuelven, y quien los recibe manda el despertar. Los
// demás quedan pendientes para BarrerPendientes.
//
// Son dos sentencias en la misma transacción, y la segunda ve lo que otra réplica
// acaba de confirmar: si dos réplicas reciben a la vez el mismo cambio, la primera
// reserva y la segunda, que espera al cerrojo de la fila, ya no la ve fuera de la
// ventana. Así sale un despertar, no dos.
func (s *Store) ReservarDespertar(ctx context.Context, subject string, bundles []string, ventana time.Duration) ([]Aparato, error) {
	t := tenancy.MustFrom(ctx)
	var out []Aparato
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE aparatos_apns SET pendiente = coalesce(pendiente, now())
			 WHERE tenant = $1 AND subject = $2 AND bundle = ANY($3)`,
			t.Slug, subject, bundles); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`UPDATE aparatos_apns SET ultimo_push = now(), pendiente = NULL
			 WHERE tenant = $1 AND subject = $2 AND bundle = ANY($3)
			   AND pendiente IS NOT NULL
			   AND (ultimo_push IS NULL OR ultimo_push < now() - make_interval(secs => $4))
			 RETURNING tenant, bundle, token`,
			t.Slug, subject, bundles, ventana.Seconds())
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Aparato])
		return err
	})
	return out, err
}

// BarrerPendientes reserva, en TODAS las organizaciones, hasta `limite` aparatos con un
// cambio pendiente cuya ventana ya ha pasado, y los devuelve para despertarlos. Con
// varias réplicas barriendo a la vez, SKIP LOCKED reparte las filas y ninguna sale dos
// veces.
func (s *Store) BarrerPendientes(ctx context.Context, ventana time.Duration, limite int) ([]Aparato, error) {
	var out []Aparato
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := enableMaintenance(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`UPDATE aparatos_apns a SET ultimo_push = now(), pendiente = NULL
			 FROM (SELECT tenant, bundle, token FROM aparatos_apns
			       WHERE pendiente IS NOT NULL
			         AND (ultimo_push IS NULL OR ultimo_push < now() - make_interval(secs => $1))
			       ORDER BY pendiente
			       LIMIT $2
			       FOR UPDATE SKIP LOCKED) p
			 WHERE a.tenant = p.tenant AND a.bundle = p.bundle AND a.token = p.token
			 RETURNING a.tenant, a.bundle, a.token`,
			ventana.Seconds(), limite)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Aparato])
		return err
	})
	return out, err
}

// BorrarAparato borra un aparato cuyo token Apple ya no acepta (apns.ErrTokenCaduco).
func (s *Store) BorrarAparato(ctx context.Context, a Aparato) error {
	return s.pool.InTenantTx(tenancy.Into(ctx, tenancy.Tenant{Slug: a.Tenant}), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`DELETE FROM aparatos_apns WHERE tenant = $1 AND bundle = $2 AND token = $3`,
			a.Tenant, a.Bundle, a.Token)
		return err
	})
}

// PurgarAparatos borra, en todas las organizaciones, los aparatos que no se han vuelto
// a registrar desde `antes`: la app se registra cada vez que arranca con el interruptor
// encendido, así que uno que lleva un mes sin hacerlo ya no está (desinstalada, o el
// aparato perdido). Devuelve cuántos.
func (s *Store) PurgarAparatos(ctx context.Context, antes time.Time) (int64, error) {
	var n int64
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := enableMaintenance(ctx, tx); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM aparatos_apns WHERE visto < $1`, antes)
		n = tag.RowsAffected()
		return err
	})
	return n, err
}
