// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// PlanDeEntrada es con qué cifraría hoy la admisión SMTP el correo de una cuenta:
// el plan de entrada activo más alto de su época y la autoridad de Rooms que lo
// autorizó (ADR 0087, cuarta tanda). Con él, el MX decide si aplaza la entrega
// en vez de cifrar para un plan que ya nadie puede reclamar.
type PlanDeEntrada struct {
	// Scope es el ámbito privado del correo de la cuenta.
	Scope storage.Scope
	// Privada: la cuenta ya recibe por la entrada privada (época frozen o
	// private). Si no, el correo va por el camino de siempre.
	Privada bool
	// Generacion es la del plan activo más alto; 0 si la época aún no tiene
	// ninguno (la admisión falla entonces con un 4xx, como siempre).
	Generacion int64
	// AutoridadUsuario es la identidad de la autoridad que autorizó ese plan: una
	// cuenta de Matrix o la de la bóveda (vaultproof.UserID). Vacía en un plan de
	// antes de guardarla, que por fuerza es de Matrix.
	AutoridadUsuario string
}

// PlanDeEntradaVigente lee el plan con el que AdmitInbound cifraría el correo de
// `recipient`. Es solo una lectura, sin bloqueos: la admisión vuelve a decidir
// dentro de su transacción, y como los planes solo rotan hacia arriba (a uno de
// la bóveda), lo que vea aquí nunca es más nuevo que lo que verá ella.
func (s *Store) PlanDeEntradaVigente(ctx context.Context, recipient ResolvedAddress) (PlanDeEntrada, error) {
	tenant, err := tenancy.From(ctx)
	if err != nil || tenant.Slug != recipient.Tenant || recipient.AccountID == uuid.Nil {
		return PlanDeEntrada{}, domain.ErrNotFound
	}
	out := PlanDeEntrada{Scope: storage.Scope{Tenant: tenant.Slug, Subject: recipient.Subject, Product: "mail"}}
	err = s.privateTx(ctx, recipient.Subject, func(tx pgx.Tx) error {
		var kind string
		if err := tx.QueryRow(ctx, `SELECT kind FROM accounts WHERE tenant=$1 AND id=$2 AND subject=$3`, tenant.Slug, recipient.AccountID, recipient.Subject).Scan(&kind); err != nil {
			return err
		}
		if kind != "user" {
			return nil
		}
		var mode string
		var epoch *uuid.UUID
		err := tx.QueryRow(ctx, `SELECT mode,epoch FROM mail_private_storage_states WHERE tenant=$1 AND owner_sub=$2 AND account_id=$3`, tenant.Slug, recipient.Subject, recipient.AccountID).Scan(&mode, &epoch)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // sin fila: legacy, como en AdmitInbound
		}
		if err != nil {
			return err
		}
		if (mode != "frozen" && mode != "private") || epoch == nil || *epoch == uuid.Nil {
			return nil
		}
		out.Privada = true
		if err := pg.BindPrivateIngressScope(ctx, tx, pg.PrivateIngressConfig{Scope: out.Scope, Epoch: *epoch}); err != nil {
			return err
		}
		var user *string
		err = tx.QueryRow(ctx, `SELECT key_generation,authority_user FROM private_ingress_key_plans WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND revoked_at IS NULL ORDER BY key_generation DESC LIMIT 1`, tenant.Slug, recipient.Subject, out.Scope.Product, *epoch).Scan(&out.Generacion, &user)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if user != nil {
			out.AutoridadUsuario = *user
		}
		return nil
	})
	if err != nil {
		return PlanDeEntrada{}, err
	}
	return out, nil
}
