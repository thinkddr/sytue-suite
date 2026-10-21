// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package tienda

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pagos/adyen"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// Una notificación de Adyen, firmada con la clave de la tienda y con la referencia con
// que se creó el cobro (el id del pago), encuentra su pedido y lo cobra. Con el número
// de pedido como referencia —como se creaba antes— no casaba ninguna.
func TestUnaNotificacionDeAdyenEncuentraSuPedido(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	kc, err := keycrypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	const clave = "44782DEF547AAA06C910C43932B1EB0C71FC68D9D0C057550C48EC2ACF6BA056"
	creds, _ := json.Marshal(map[string]string{"api_key": "x", "merchant_account": "TiendaECOM", "hmac_key": clave})
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
	fila(&pasarela, `INSERT INTO gateways (tenant, site_id, provider, mode, credentials_enc, checked_at, created_by)
		VALUES ('org', $1, 'adyen', 'test', $2, now(), 'sub') RETURNING id`, sitio, enc)
	fila(&pedido, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
			subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
			shipping_method, language, terms_version, terms_accepted_at, reserved_until)
		VALUES ('org', $1, 'P-000001', true, gen_random_uuid(), decode('00','hex'), 'pending_payment',
			250, 0, 250, 23, '{}', 'c@example.test', 'C', '{}', 'es', 1, now(), now() + interval '35 minutes') RETURNING id`, sitio)
	fila(&pago, `INSERT INTO payments (tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key)
		VALUES ('org', $1, $2, 1, 'adyen', 'CS1234567890', 'open', 250, 'k') RETURNING id`, pedido, pasarela)

	n := adyen.Notificacion{
		PSPReference: "9912345678901234", MerchantAccountCode: "TiendaECOM", MerchantReference: pago.String(),
		EventCode: "AUTHORISATION", Success: "true",
	}
	n.Amount.Value, n.Amount.Currency = 250, "EUR"
	k, _ := hex.DecodeString(clave)
	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(strings.Join([]string{n.PSPReference, "", n.MerchantAccountCode, n.MerchantReference, "250", "EUR", n.EventCode, n.Success}, ":")))
	n.AdditionalData = map[string]string{"hmacSignature": base64.StdEncoding.EncodeToString(mac.Sum(nil))}

	svc := Nuevo(store.New(super), kc, "https://web.example.test", "sites.example.test", nil, nil)
	if err := svc.ProcesarWebhookAdyen(ctx, n); err != nil {
		t.Fatalf("notificación de Adyen: %v", err)
	}
	var estado string
	if err := super.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, pedido).Scan(&estado); err != nil || estado != "paid" {
		t.Fatalf("pedido %q (%v); quiero paid", estado, err)
	}
}
