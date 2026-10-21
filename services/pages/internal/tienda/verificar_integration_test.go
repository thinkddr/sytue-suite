// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package tienda

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// WEB-03. Una notificación de Stripe bien firmada pero de otro importe no cobra el pedido:
// pagos.Verificar va antes de tocar nada. Con el importe del pago, sí.
func TestUnaNotificacionDeOtroImporteNoCobraElPedido(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	const secreto = "whsec" + "_pruebas"
	creds, _ := json.Marshal(map[string]string{"secret_key": "sk" + "_test_x", "webhook_secret": secreto})
	enc, err := kc.Encrypt(string(creds))
	if err != nil {
		t.Fatal(err)
	}
	var sitio, pasarela, pedido, pago uuid.UUID
	fila := func(dest *uuid.UUID, sql string, args ...any) {
		t.Helper()
		if err := super.QueryRow(ctx, sql, args...).Scan(dest); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	fila(&sitio, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ('org', 'tienda', 'Tienda', '{}', 'sub') RETURNING id`)
	fila(&pasarela, `INSERT INTO gateways (tenant, site_id, provider, mode, credentials_enc, checked_at, created_by, notice_accepted_at)
		VALUES ('org', $1, 'stripe', 'test', $2, now(), 'sub', now()) RETURNING id`, sitio, enc)
	fila(&pedido, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
			subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
			shipping_method, language, terms_version, terms_accepted_at, reserved_until)
		VALUES ('org', $1, 'P-000001', true, gen_random_uuid(), decode('00','hex'), 'pending_payment',
			250, 0, 250, 23, '{}', 'c@example.test', 'C', '{}', 'es', 1, now(), now() + interval '35 minutes') RETURNING id`, sitio)
	fila(&pago, `INSERT INTO payments (tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key)
		VALUES ('org', $1, $2, 1, 'stripe', 'cs_test_sesion01', 'open', 250, 'k') RETURNING id`, pedido, pasarela)

	svc := Nuevo(store.New(super), kc, "https://web.example.test", "sites.example.test", nil, nil)
	notificar := func(importe int) error {
		cuerpo := fmt.Sprintf(`{"id":"evt_%d","type":"checkout.session.completed","livemode":false,"data":{"object":{"id":"cs_test_sesion01","payment_status":"paid","amount_total":%d,"currency":"eur","metadata":{"sytue_payment_id":%q}}}}`,
			importe, importe, pago.String())
		marca := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(secreto))
		mac.Write([]byte(marca + "." + cuerpo))
		return svc.ProcesarWebhookStripe(ctx, []byte(cuerpo), "t="+marca+",v1="+hex.EncodeToString(mac.Sum(nil)))
	}
	estado := func() string {
		var e string
		if err := super.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, pedido).Scan(&e); err != nil {
			t.Fatal(err)
		}
		return e
	}

	if err := notificar(1); err == nil || estado() != "pending_payment" {
		t.Fatalf("notificación de 1 céntimo para un pago de 250: err=%v, pedido %s; quiero error y sin cobrar", err, estado())
	}
	if err := notificar(250); err != nil || estado() != "paid" {
		t.Fatalf("notificación del pago esperado: err=%v, pedido %s; quiero cobrado", err, estado())
	}
}
