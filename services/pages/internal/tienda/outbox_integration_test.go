// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package tienda

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// WEB-10. El outbox no tiene RLS ni purga: ningún aviso guarda datos personales. Ni el
// pedido cobrado (comprador, factura, líneas), ni el formulario (sus campos y a quién
// avisar), ni el desistimiento (su correo). Solo identificadores e importes.
func TestElOutboxNoGuardaDatosPersonales(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	const org = "org"
	var sitio, pasarela, pedido, pago uuid.UUID
	fila := func(dest *uuid.UUID, sql string, args ...any) {
		t.Helper()
		if err := super.QueryRow(ctx, sql, args...).Scan(dest); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	fila(&sitio, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'tienda', 'Tienda', '{}', 'sub') RETURNING id`, org)
	fila(&pasarela, `INSERT INTO gateways (tenant, site_id, provider, mode, credentials_enc, checked_at, created_by)
		VALUES ($1, $2, 'mollie', 'test', 'encv1:x', now(), 'sub') RETURNING id`, org, sitio)
	fila(&pedido, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
			subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
			shipping_method, invoice, language, terms_version, terms_accepted_at, reserved_until)
		VALUES ($1, $2, 'P-000001', true, gen_random_uuid(), decode('00','hex'), 'pending_payment',
			250, 0, 250, 23, '{}', 'ana.comprador@example.test', 'Ana Compradora', '{}',
			'{"tax_id":"12345678Z","name":"Ana Compradora","address":"Calle Privada 1"}', 'es', 1, now(), now() + interval '35 minutes')
		RETURNING id`, org, sitio)
	if _, err := super.Exec(ctx, `INSERT INTO order_lines (order_id, tenant, position, sku, name, quantity, unit_price_cents, vat_rate, total_cents)
		VALUES ($1, $2, 1, 'PAN-1', 'Pan', 1, 250, 10, 250)`, pedido, org); err != nil {
		t.Fatal(err)
	}
	fila(&pago, `INSERT INTO payments (tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key)
		VALUES ($1, $2, $3, 1, 'mollie', 'tr_1', 'open', 250, 'k') RETURNING id`, org, pedido, pasarela)
	formID, campos := publicarConFormulario(t, super, org, sitio)
	if _, err := super.Exec(ctx, `INSERT INTO form_settings (site_id, tenant, form_id, enabled, notify)
		VALUES ($1, $2, $3, true, '[{"email":"aviso.persona@example.test"}]')`, sitio, org, formID); err != nil {
		t.Fatal(err)
	}

	st := store.New(super)
	svc := Nuevo(st, nil, "https://web.example.test", "sites.example.test", nil, nil)
	p, err := st.ObtenerPago(ctx, org, pago)
	if err != nil {
		t.Fatal(err)
	}
	gw, err := st.ObtenerPasarela(ctx, org, sitio, "mollie")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.tratarPagoCompletado(ctx, org, p, gw, 250); err != nil {
		t.Fatalf("pago cobrado: %v", err)
	}
	if err := svc.EnviarFormularioPublico(ctx, org, sitio, formID, valoresValidos(campos), true, "es"); err != nil {
		t.Fatalf("formulario: %v", err)
	}
	if err := svc.RegistrarDesistimientoPublico(ctx, org, sitio, "P-000001", "ana.comprador@example.test", "Ana", "No lo quiero", nil); err != nil {
		t.Fatalf("desistimiento: %v", err)
	}

	filas, err := super.Query(ctx, `SELECT type, payload::text FROM events_outbox ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer filas.Close()
	tipos := map[string]bool{}
	for filas.Next() {
		var tipo, carga string
		if err := filas.Scan(&tipo, &carga); err != nil {
			t.Fatal(err)
		}
		tipos[tipo] = true
		for _, personal := range []string{"example.test", "Ana", "Luis", "12345678Z", "Calle Privada", "PAN-1"} {
			if strings.Contains(carga, personal) {
				t.Errorf("el aviso %s guarda %q en el outbox: %s", tipo, personal, carga)
			}
		}
	}
	for _, tipo := range []string{"order.paid.email_buyer", "order.paid.erp", "pages.form.submitted", "pages.order.withdrawal"} {
		if !tipos[tipo] {
			t.Errorf("no se escribió el aviso %s", tipo)
		}
	}
}

// La ronda del barrido purga el outbox: lo despachado hace más de una semana y lo que
// nadie despachó en 30 días. Lo reciente se queda, también el aviso de formulario de
// ayer, que cuenta para el tope diario.
func TestElBarridoPurgaElOutbox(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	if _, err := super.Exec(ctx, `INSERT INTO events_outbox (id, channel, type, tenant, created_at, processed_at) VALUES
		(gen_random_uuid(), 'pages', 'viejo.despachado',   'org', now() - interval '9 days',  now() - interval '8 days'),
		(gen_random_uuid(), 'pages', 'viejo.sin.despachar', 'org', now() - interval '31 days', NULL),
		(gen_random_uuid(), 'pages', 'reciente.despachado', 'org', now() - interval '2 days',  now() - interval '1 day'),
		(gen_random_uuid(), 'pages', 'reciente.pendiente',  'org', now() - interval '20 days', NULL),
		(gen_random_uuid(), 'pages', 'pages.form.submitted', 'org', now() - interval '1 hour', NULL)`); err != nil {
		t.Fatal(err)
	}
	svc := Nuevo(store.New(super), nil, "https://web.example.test", "sites.example.test", nil, nil)
	if err := svc.EjecutarRondaBarrido(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := super.Query(ctx, `SELECT type FROM events_outbox ORDER BY type`)
	if err != nil {
		t.Fatal(err)
	}
	var quedan []string
	for rows.Next() {
		var tipo string
		if err := rows.Scan(&tipo); err != nil {
			t.Fatal(err)
		}
		quedan = append(quedan, tipo)
	}
	if got := strings.Join(quedan, ","); got != "pages.form.submitted,reciente.despachado,reciente.pendiente" {
		t.Fatalf("quedan %s; quiero solo los recientes", got)
	}
}
