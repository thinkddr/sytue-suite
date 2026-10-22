// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package tienda

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pagos"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// pasarelaQueReembolsa apunta cada reembolso pedido y falla los primeros `fallos`.
type pasarelaQueReembolsa struct {
	pagos.Pasarela
	pedidos []string
	fallos  int
}

func (p *pasarelaQueReembolsa) Reembolsar(_ context.Context, ref string, centimos int64, clave string) error {
	p.pedidos = append(p.pedidos, ref+"|"+clave)
	if p.fallos > 0 {
		p.fallos--
		return errors.New("pasarela caída")
	}
	return nil
}

// Decisión del dueño (2026-10-16): el segundo cobro de un pedido ya pagado se reembolsa
// solo, por su pasarela, y la tienda recibe un aviso. UNA devolución por cobro: ni la
// notificación repetida ni el reintento de una que falló abren otra. Y se devuelve el
// cobro duplicado, no el último pago del pedido, y el pedido sigue pagado.
func TestUnCobroDuplicadoSeReembolsaUnaSolaVez(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	var sitio, pasarela, pedido, bueno, duplicado, ultimo uuid.UUID
	fila := func(dest *uuid.UUID, sql string, args ...any) {
		t.Helper()
		if err := super.QueryRow(ctx, sql, args...).Scan(dest); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	fila(&sitio, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ('org', 'tienda', 'Tienda', '{}', 'sub') RETURNING id`)
	fila(&pasarela, `INSERT INTO gateways (tenant, site_id, provider, mode, credentials_enc, checked_at, created_by)
		VALUES ('org', $1, 'mollie', 'test', 'encv1:x', now(), 'sub') RETURNING id`, sitio)
	fila(&pedido, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
			subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
			shipping_method, language, terms_version, terms_accepted_at, reserved_until, paid_at)
		VALUES ('org', $1, 'P-000001', true, gen_random_uuid(), decode('00','hex'), 'paid',
			250, 0, 250, 23, '{}', 'c@example.test', 'C', '{}', 'es', 1, now(), now(), now()) RETURNING id`, sitio)
	pago := func(dest *uuid.UUID, intento int, ref, estado string) {
		fila(dest, `INSERT INTO payments (tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key)
			VALUES ('org', $1, $2, $3, 'mollie', $4, $5, 250, $4) RETURNING id`, pedido, pasarela, intento, ref, estado)
	}
	pago(&duplicado, 1, "tr_dup", "open") // un intento anterior que se cobró tarde
	pago(&bueno, 2, "tr_bueno", "paid")
	pago(&ultimo, 3, "tr_ultimo", "expired") // el último pago del pedido no es ninguno de los dos

	falsa := &pasarelaQueReembolsa{fallos: 1}
	svc := Nuevo(store.New(super), nil, "https://web.example.test", "sites.example.test", nil, nil)
	svc.pasarelaFija = falsa
	st := store.New(super)
	notificar := func() {
		t.Helper()
		p, gw, _, err := st.ResolverPagoPorID(ctx, duplicado)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.tratarPagoCompletado(ctx, "org", p, gw, 250); err != nil {
			t.Fatal(err)
		}
	}
	estado := func(sql string, id uuid.UUID) string {
		t.Helper()
		var e string
		if err := super.QueryRow(ctx, sql, id).Scan(&e); err != nil {
			t.Fatal(err)
		}
		return e
	}
	clave := "tr_dup|dup-" + duplicado.String()

	notificar() // el duplicado: se pide el reembolso y la pasarela falla
	if len(falsa.pedidos) != 1 || falsa.pedidos[0] != clave {
		t.Fatalf("reembolsos pedidos %v; quiero uno del cobro duplicado (%s)", falsa.pedidos, clave)
	}
	notificar() // la notificación se repite: se reintenta el que falló, con la misma clave
	notificar() // y otra vez: ya está hecho, no se pide nada
	if len(falsa.pedidos) != 2 || falsa.pedidos[1] != clave {
		t.Fatalf("reembolsos pedidos %v; quiero el que falló reintentado una vez y ninguno más", falsa.pedidos)
	}
	var n int
	if err := super.QueryRow(ctx, `SELECT count(*) FROM refunds WHERE payment_id = $1 AND status = 'done'`, duplicado).Scan(&n); err != nil || n != 1 {
		t.Fatalf("reembolsos hechos del duplicado: %d (%v); quiero 1", n, err)
	}
	if e := estado(`SELECT status FROM payments WHERE id = $1`, duplicado); e != "refunded" {
		t.Errorf("el cobro duplicado está %q; quiero refunded", e)
	}
	if e := estado(`SELECT status FROM payments WHERE id = $1`, bueno); e != "paid" {
		t.Errorf("el cobro bueno está %q; quiero paid", e)
	}
	if e := estado(`SELECT status FROM orders WHERE id = $1`, pedido); e != "paid" {
		t.Errorf("el pedido está %q; quiero paid: devolver el duplicado no lo reembolsa", e)
	}
	if err := super.QueryRow(ctx, `SELECT count(*) FROM events_outbox WHERE type = 'pages.payment.duplicated'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("avisos de duplicado a la tienda: %d (%v); quiero 1", n, err)
	}
}
