// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package tienda

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// La bandeja marca leídas y borra respuestas contra Postgres de verdad: el recuento de
// sin leer sale del FILTER del resumen, marcar no reescribe la hora de una ya leída y
// otra organización no toca nada.
func TestBandejaLeidasYBorrado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	const org = "org"
	var sitio uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'web', 'Web', '{}', 'sub') RETURNING id`, org).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	const form = "abcdefghij"
	ids := make([]uuid.UUID, 3)
	for i := range ids {
		if err := super.QueryRow(ctx, `INSERT INTO form_submissions (tenant, site_id, form_id, data, language, consent_text, consent_at)
			VALUES ($1, $2, $3, '{"nombre":"Ana"}', 'es', 'Acepto', now()) RETURNING id`, org, sitio, form).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	st := store.New(super)
	svc := Nuevo(st, nil, "https://web.example.test", "sites.example.test", nil, nil)

	sinLeer := func() int {
		t.Helper()
		_, resumen, err := st.Formularios(ctx, org, sitio)
		if err != nil {
			t.Fatal(err)
		}
		return resumen[form].SinLeer
	}
	if n := sinLeer(); n != 3 {
		t.Fatalf("sin leer al empezar = %d, quiero 3", n)
	}
	if n, err := svc.MarcarEnviosLeidos(ctx, org, sitio, form, ids[:1], true); err != nil || n != 1 {
		t.Fatalf("marcar una: n=%d err=%v", n, err)
	}
	// Volver a marcarla no cuenta como cambio (la hora de lectura se conserva).
	if n, _ := svc.MarcarEnviosLeidos(ctx, org, sitio, form, ids[:1], true); n != 0 {
		t.Fatalf("remarcar una ya leída cambió %d filas", n)
	}
	if n, err := svc.MarcarEnviosLeidos(ctx, org, sitio, form, nil, true); err != nil || n != 2 || sinLeer() != 0 {
		t.Fatalf("marcar todas: n=%d err=%v sin leer=%d", n, err, sinLeer())
	}
	if _, err := svc.MarcarEnviosLeidos(ctx, org, sitio, form, nil, false); !errors.Is(err, domain.ErrEntradaInvalida) {
		t.Fatalf("devolver todas a sin leer de golpe: %v; quiero ErrEntradaInvalida", err)
	}

	// Otra organización no marca ni borra.
	if _, err := svc.MarcarEnviosLeidos(ctx, "otra", sitio, form, ids, false); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("otra organización marcando: %v; quiero ErrNoExiste", err)
	}
	if err := svc.BorrarEnvio(ctx, "otra", sitio, form, ids[0]); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("otra organización borrando: %v; quiero ErrNoExiste", err)
	}

	if err := svc.BorrarEnvio(ctx, org, sitio, form, ids[0]); err != nil {
		t.Fatalf("borrar: %v", err)
	}
	if err := svc.BorrarEnvio(ctx, org, sitio, form, ids[0]); !errors.Is(err, domain.ErrFormularioNoExiste) {
		t.Fatalf("borrar dos veces: %v; quiero ErrFormularioNoExiste", err)
	}
	var quedan int
	if err := super.QueryRow(ctx, `SELECT count(*) FROM form_submissions WHERE site_id = $1`, sitio).Scan(&quedan); err != nil || quedan != 2 {
		t.Fatalf("quedan %d respuestas (err=%v), quiero 2", quedan, err)
	}
}

// Servir un pedido solo desde «pagado», y deshacerlo solo desde «servido».
func TestPedidoServidoYDeshacer(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	const org = "org"
	var sitio uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'web', 'Web', '{}', 'sub') RETURNING id`, org).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	nuevoPedido := func(numero, estado string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := super.QueryRow(ctx, `
			INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status, currency,
			                    subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown,
			                    buyer_email, buyer_name, shipping_method, language, terms_version,
			                    terms_accepted_at, reserved_until)
			VALUES ($1, $2, $3, false, gen_random_uuid(), '\x00', $4, 'EUR', 1000, 0, 1000, 0, '{}',
			        'c@example.test', 'C', '{}', 'es', 1, now(), now())
			RETURNING id`, org, sitio, numero, estado).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	st := store.New(super)
	pagado := nuevoPedido("P-000001", domain.OrderStatusPaid)
	pendiente := nuevoPedido("P-000002", domain.OrderStatusPendingPayment)

	o, err := st.MarcarPedidoServido(ctx, org, sitio, pagado, true)
	if err != nil || o.Status != domain.OrderStatusFulfilled || o.FulfilledAt == nil {
		t.Fatalf("servir un pagado: %+v err=%v", o, err)
	}
	if _, err := st.MarcarPedidoServido(ctx, org, sitio, pagado, true); !errors.Is(err, domain.ErrEstadoPedido) {
		t.Fatalf("servir dos veces: %v; quiero ErrEstadoPedido", err)
	}
	if _, err := st.MarcarPedidoServido(ctx, org, sitio, pendiente, true); !errors.Is(err, domain.ErrEstadoPedido) {
		t.Fatalf("servir uno sin pagar: %v; quiero ErrEstadoPedido", err)
	}
	if _, err := st.MarcarPedidoServido(ctx, "otra", sitio, pagado, false); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("otra organización: %v; quiero ErrNoExiste", err)
	}
	o, err = st.MarcarPedidoServido(ctx, org, sitio, pagado, false)
	if err != nil || o.Status != domain.OrderStatusPaid || o.FulfilledAt != nil {
		t.Fatalf("deshacer: %+v err=%v", o, err)
	}
	if _, err := st.MarcarPedidoServido(ctx, org, sitio, uuid.New(), true); !errors.Is(err, domain.ErrPedidoNoExiste) {
		t.Fatalf("pedido inexistente: %v; quiero ErrPedidoNoExiste", err)
	}
}
