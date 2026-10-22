// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/commerce"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

const subscriptionCols = `org_id, provider_customer_id, provider_subscription_id,
	status, seats, unit_price_cents, currency, paid_until,
	created_at, updated_at, plan`

func scanSubscription(row pgx.Row) (domain.OrgSubscription, error) {
	var sub domain.OrgSubscription
	var paidUntil *time.Time
	err := row.Scan(&sub.OrgID, &sub.ProviderCustomerID, &sub.ProviderSubscriptionID,
		&sub.Status, &sub.Seats, &sub.UnitPriceCents, &sub.Currency, &paidUntil,
		&sub.CreatedAt, &sub.UpdatedAt, &sub.Plan)
	if paidUntil != nil {
		sub.PaidUntil = *paidUntil
	}
	return sub, err
}

func (s *Store) EnsurePendingSubscription(ctx context.Context, q pg.Querier, orgID string, seats int) error {
	return s.EnsureSubscriptionPlan(ctx, q, orgID, seats, commerce.PlanPro)
}

// EnsureSubscriptionPlan crea la fila de suscripción con su contrato si aún no
// existe. El precio unitario se fija del plan, no del llamante: es el contrato
// el que dice cuánto cuesta.
func (s *Store) EnsureSubscriptionPlan(ctx context.Context, q pg.Querier, orgID string, seats int, plan commerce.Plan) error {
	if q == nil {
		q = s.DB()
	}
	if !commerce.ValidPlan(plan) {
		return errf("EnsureSubscriptionPlan", errors.New("plan desconocido: "+string(plan)))
	}
	_, err := q.Exec(ctx, `INSERT INTO org_subscriptions (org_id, seats, plan, unit_price_cents)
		VALUES ($1,$2,$3,$4) ON CONFLICT (org_id) DO NOTHING`, orgID, seats, plan, plan.PriceCents())
	return errf("EnsureSubscriptionPlan", err)
}

func (s *Store) GetOrgSubscription(ctx context.Context, orgID string) (domain.OrgSubscription, error) {
	// Con la clase de la organización: es lo que decide que una cuenta personal
	// nunca se quede fuera y que, sin pagar, vuelva al plan gratuito.
	sub, err := scanSubscription(s.DB().QueryRow(ctx,
		`SELECT `+subscriptionCols+` FROM org_subscriptions WHERE org_id=$1`, orgID))
	if err != nil {
		return sub, errf("GetOrgSubscription", err)
	}
	err = s.DB().QueryRow(ctx, `SELECT kind='personal', education FROM organizations WHERE id=$1`, orgID).Scan(&sub.Personal, &sub.Education)
	return sub, errf("GetOrgSubscription.kind", err)
}

func (s *Store) ActiveMemberCount(ctx context.Context, orgID string) (int, error) {
	var n int
	err := s.DB().QueryRow(ctx, `SELECT count(*) FROM memberships m
		JOIN users u ON u.id=m.user_id WHERE m.org_id=$1 AND u.status='active'`, orgID).Scan(&n)
	return n, errf("ActiveMemberCount", err)
}

// GrantedSeats suma las plazas CONCEDIDAS vigentes de una organización (mig
// 0031). «Vigente» es el mismo criterio que usa el panel de plataforma: ni
// revocada ni caducada, y sin caducidad significa vigente —es el caso de un
// empleado—. Sin concesiones devuelve 0, no un error: no tener ninguna es lo
// normal, y confundirlo con un fallo dejaría sin acceso a media suite.
func (s *Store) GrantedSeats(ctx context.Context, orgID string, now time.Time) (int, error) {
	var n int
	err := s.DB().QueryRow(ctx, `SELECT coalesce(sum(seats), 0)::int FROM seat_grants
		WHERE org_id = $1 AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > $2)`, orgID, now).Scan(&n)
	return n, errf("GrantedSeats", err)
}

func (s *Store) OrgSubscriptionEntitled(ctx context.Context, orgID string, now time.Time) (bool, error) {
	sub, err := s.GetOrgSubscription(ctx, orgID)
	if err != nil {
		return false, err
	}
	members, err := s.ActiveMemberCount(ctx, orgID)
	if err != nil {
		return false, err
	}
	granted, err := s.GrantedSeats(ctx, orgID, now)
	if err != nil {
		return false, err
	}
	return sub.Entitled(now, members, granted), nil
}

// RegisterSubscriptionCheckout enlaza IDs creados por nuestra llamada autenticada
// a Mollie. El payment queda pendiente hasta que el webhook se vuelva a verificar.
func (s *Store) RegisterSubscriptionCheckout(ctx context.Context, orgID, customerID, paymentID string, seats, amountCents int, plan commerce.Plan) error {
	if !commerce.ValidPlan(plan) {
		return errf("RegisterSubscriptionCheckout", errors.New("plan desconocido: "+string(plan)))
	}
	return s.InTx(ctx, func(q pg.Querier) error {
		// El plan y su precio se fijan al iniciar el cobro, no al cobrarlo: es lo
		// que permite al webhook comprobar que el importe que Mollie dice haber
		// cobrado es EXACTAMENTE el del contrato que se le pidió. Solo si no hay
		// una suscripción activa: un checkout nuevo no pisa un contrato vigente.
		if _, err := q.Exec(ctx, `INSERT INTO org_subscriptions (org_id, provider_customer_id, seats, plan, unit_price_cents)
			VALUES ($1,$2,$3,$4,$5) ON CONFLICT (org_id) DO UPDATE SET
			provider_customer_id=excluded.provider_customer_id, seats=excluded.seats,
			plan=excluded.plan, unit_price_cents=excluded.unit_price_cents
			WHERE org_subscriptions.status <> 'active'`, orgID, customerID, seats, plan, amountCents/seats); err != nil {
			return errf("RegisterSubscriptionCheckout subscription", err)
		}
		_, err := q.Exec(ctx, `INSERT INTO subscription_payments
			(provider_payment_id, org_id, status, amount_cents, currency)
			VALUES ($1,$2,'pending',$3,'EUR') ON CONFLICT (provider_payment_id) DO NOTHING`,
			paymentID, orgID, amountCents)
		return errf("RegisterSubscriptionCheckout payment", err)
	})
}

func (s *Store) SubscriptionForPayment(ctx context.Context, paymentID string) (domain.OrgSubscription, int, string, error) {
	var amount int
	var paymentStatus string
	row := s.DB().QueryRow(ctx, `SELECT `+subscriptionCols+`, p.amount_cents, p.status
		FROM subscription_payments p JOIN org_subscriptions s ON s.org_id=p.org_id
		WHERE p.provider_payment_id=$1`, paymentID)
	sub, err := scanSubscriptionWithPayment(row, &amount, &paymentStatus)
	return sub, amount, paymentStatus, errf("SubscriptionForPayment", err)
}

func scanSubscriptionWithPayment(row pgx.Row, amount *int, status *string) (domain.OrgSubscription, error) {
	var sub domain.OrgSubscription
	var paidUntil *time.Time
	err := row.Scan(&sub.OrgID, &sub.ProviderCustomerID, &sub.ProviderSubscriptionID,
		&sub.Status, &sub.Seats, &sub.UnitPriceCents, &sub.Currency, &paidUntil,
		&sub.CreatedAt, &sub.UpdatedAt, &sub.Plan, amount, status)
	if paidUntil != nil {
		sub.PaidUntil = *paidUntil
	}
	return sub, err
}

func (s *Store) SubscriptionForProviderID(ctx context.Context, providerID string) (domain.OrgSubscription, error) {
	sub, err := scanSubscription(s.DB().QueryRow(ctx, `SELECT `+subscriptionCols+
		` FROM org_subscriptions WHERE provider_subscription_id=$1`, providerID))
	return sub, errf("SubscriptionForProviderID", err)
}

// RecordVerifiedPayment aplica un estado obtenido por GET autenticado a Mollie.
// El INSERT/UPDATE es idempotente: un webhook repetido no añade meses de acceso.
func (s *Store) RecordVerifiedPayment(ctx context.Context, paymentID, orgID, status string,
	amountCents int, subscriptionID string, paidAt time.Time,
) error {
	return s.InTx(ctx, func(q pg.Querier) error {
		var previous string
		err := q.QueryRow(ctx, `SELECT status FROM subscription_payments
			WHERE provider_payment_id=$1 AND org_id=$2 FOR UPDATE`, paymentID, orgID).Scan(&previous)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return errf("RecordVerifiedPayment lock", err)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			if _, err = q.Exec(ctx, `INSERT INTO subscription_payments
				(provider_payment_id,org_id,status,amount_cents,currency)
				VALUES ($1,$2,'pending',$3,'EUR')`, paymentID, orgID, amountCents); err != nil {
				return errf("RecordVerifiedPayment insert", err)
			}
			previous = "pending"
		}
		if _, err = q.Exec(ctx, `UPDATE subscription_payments SET status=$3,
			paid_at=CASE WHEN $3='paid' THEN $4 ELSE paid_at END WHERE provider_payment_id=$1 AND org_id=$2`,
			paymentID, orgID, status, paidAt); err != nil {
			return errf("RecordVerifiedPayment update", err)
		}
		if status == "paid" && previous != "paid" {
			_, err = q.Exec(ctx, `UPDATE org_subscriptions SET status='active',
				provider_subscription_id=CASE WHEN $3<>'' THEN $3 ELSE provider_subscription_id END,
				paid_until=greatest(coalesce(paid_until, $2), $2) + interval '1 month'
				WHERE org_id=$1`, orgID, paidAt, subscriptionID)
			return errf("RecordVerifiedPayment activate", err)
		}
		if status == "failed" || status == "expired" || status == "canceled" ||
			status == "refunded" || status == "charged_back" {
			// Un checkout inicial antiguo que falle no puede derribar otra
			// suscripción ya activada. Sí suspenden: un pago que antes constaba
			// pagado (reembolso/chargeback) o un cobro de la suscripción vigente.
			if previous != "paid" && subscriptionID == "" {
				return nil
			}
			_, err = q.Exec(ctx, `UPDATE org_subscriptions SET status='past_due'
				WHERE org_id=$1 AND ($2='' OR provider_subscription_id=$2)`, orgID, subscriptionID)
			return errf("RecordVerifiedPayment past due", err)
		}
		return nil
	})
}
