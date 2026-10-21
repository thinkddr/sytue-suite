// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package tienda

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// Los avisos del outbox salen: la confirmación al comprador (una vez aunque el aviso se
// entregue dos), el aviso a la tienda por correo y por la campana, el acuse del
// desistimiento y el aviso de formulario a quien diga form_settings. Lo que lleva más
// de dos días sin salir no sale, salvo el desistimiento. Antes nada de esto salía:
// ningún despachador consumía el outbox.
func TestLosAvisosDeLaTiendaSalen(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	var sitio, pedido, viejo, desistimiento, envio uuid.UUID
	fila := func(dest *uuid.UUID, sql string, args ...any) {
		t.Helper()
		if err := super.QueryRow(ctx, sql, args...).Scan(dest); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	fila(&sitio, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ('org', 'tienda', 'Panadería Aurora', '{}', 'sub') RETURNING id`)
	if _, err := super.Exec(ctx, `INSERT INTO store_settings (site_id, tenant, notify, reply_to)
		VALUES ($1, 'org', '[{"sub":"u-tienda","email":"tienda@example.test"}]', 'hola@aurora.example')`, sitio); err != nil {
		t.Fatal(err)
	}
	nuevoPedido := func(dest *uuid.UUID, numero, correo string) {
		fila(dest, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
				subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
				shipping_method, language, terms_version, terms_accepted_at, reserved_until, paid_at)
			VALUES ('org', $1, $2, true, gen_random_uuid(), decode('00','hex'), 'paid',
				2910, 0, 2910, 23, '{}', $3, 'C', '{}', 'es', 1, now(), now(), now()) RETURNING id`, sitio, numero, correo)
	}
	nuevoPedido(&pedido, "P-000001", "ana@example.test")
	nuevoPedido(&viejo, "P-000002", "luis@example.test")
	fila(&desistimiento, `INSERT INTO withdrawals (tenant, order_id, email, source) VALUES ('org', $1, 'ana@example.test', 'form') RETURNING id`, pedido)
	if _, err := super.Exec(ctx, `INSERT INTO form_settings (site_id, tenant, form_id, notify)
		VALUES ($1, 'org', 'contacto01', '[{"sub":"u-form","email":"form@example.test"}]')`, sitio); err != nil {
		t.Fatal(err)
	}
	fila(&envio, `INSERT INTO form_submissions (tenant, site_id, form_id, data, language, consent_text, consent_at)
		VALUES ('org', $1, 'contacto01', '{}', 'en', 'x', now()) RETURNING id`, sitio)

	var mu sync.Mutex
	var campana []string
	hubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		mu.Lock()
		campana = append(campana, n.Subject+":"+n.Type+":"+n.Params["number"]+":"+n.Params["site"])
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(hubSrv.Close)

	svc := Nuevo(store.New(super), nil, "https://web.example.test", "sites.example.test", hubnotify.New(hubSrv.URL, "secreto"), nil)
	var correos []string
	svc.enviarCorreo = func(_ context.Context, op correo.Opciones, para, asunto, _ string) error {
		if op.NombreRemitente != "Panadería Aurora (vía Sytue Web)" || op.ResponderA != "hola@aurora.example" {
			t.Errorf("el correo a %s sale como %+v; quiero el nombre de la tienda y su respuesta", para, op)
		}
		correos = append(correos, para+":"+asunto)
		return nil
	}
	entregar := func(tipo string, hace time.Duration, carga map[string]any) {
		t.Helper()
		crudo, _ := json.Marshal(carga)
		ev := events.Event{ID: uuid.NewString(), Channel: "pages", Type: tipo, Tenant: "org", Payload: crudo, CreatedAt: time.Now().Add(-hace)}
		if err := svc.entregarAviso(ctx, ev); err != nil {
			t.Fatalf("%s: %v", tipo, err)
		}
	}
	delPedido := map[string]any{"order_id": pedido, "site_id": sitio}
	entregar("order.paid.email_buyer", 0, delPedido)
	entregar("order.paid.email_buyer", 0, delPedido) // el Dispatcher es at-least-once
	entregar("order.paid.email_store", 0, delPedido)
	entregar("order.paid.hub", 0, delPedido)
	entregar("order.paid.email_buyer", 72*time.Hour, map[string]any{"order_id": viejo, "site_id": sitio})
	entregar("pages.order.withdrawal", 72*time.Hour, map[string]any{"withdrawal_id": desistimiento, "site_id": sitio})
	entregar("pages.order.withdrawal", 0, map[string]any{"withdrawal_id": desistimiento, "site_id": sitio})
	entregar("pages.form.submitted", 0, map[string]any{"site_id": sitio, "form_id": "contacto01", "submission_id": envio})

	quiero := []string{
		"ana@example.test:Confirmación del pedido P-000001",
		"ana@example.test:Confirmación de desistimiento recibida: P-000001",
		"form@example.test:Form submission received: contacto01",
		"tienda@example.test:Nuevo pedido recibido: P-000001",
	}
	slices.Sort(correos)
	slices.Sort(quiero)
	if !slices.Equal(correos, quiero) {
		t.Errorf("correos:\n%s\nquiero:\n%s", strings.Join(correos, "\n"), strings.Join(quiero, "\n"))
	}
	mu.Lock()
	slices.Sort(campana)
	quieroCampana := []string{
		"u-form:pages.form.received::Panadería Aurora",
		"u-tienda:pages.order.paid:P-000001:Panadería Aurora",
		"u-tienda:pages.withdrawal.requested:P-000001:Panadería Aurora",
		"u-tienda:pages.withdrawal.requested:P-000001:Panadería Aurora",
	}
	if !slices.Equal(campana, quieroCampana) {
		t.Errorf("campana:\n%s\nquiero:\n%s", strings.Join(campana, "\n"), strings.Join(quieroCampana, "\n"))
	}
	mu.Unlock()
}

func TestAvisosTienda_OrderPaidERP(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)

	var sitio, pasarela uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by)
		VALUES ('org', 'aurora', 'Panadería Aurora', '{}', 'sub') RETURNING id`).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	if err := super.QueryRow(ctx, `INSERT INTO gateways (tenant, site_id, provider, mode, credentials_enc, checked_at, created_by, notice_accepted_at)
		VALUES ('org', $1, 'stripe', 'live', 'encv1:x', now(), 'sub', now()) RETURNING id`, sitio).Scan(&pasarela); err != nil {
		t.Fatal(err)
	}
	if _, err := super.Exec(ctx, `INSERT INTO store_settings (site_id, tenant, notify, reply_to)
		VALUES ($1, 'org', '[]', 'hola@aurora.example')`, sitio); err != nil {
		t.Fatal(err)
	}

	crearPedidoReal := func(numero, facturaJSON string, total int) (uuid.UUID, uuid.UUID) {
		var pID, payID uuid.UUID
		if err := super.QueryRow(ctx, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
				subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
				shipping_method, invoice, language, terms_version, terms_accepted_at, reserved_until, paid_at)
			VALUES ('org', $1, $2, false, gen_random_uuid(), decode('00','hex'), 'paid',
				$3, 0, $3, 505, '{"21.00":505}', 'ana@example.test', 'Ana Compradora',
				'{}', $4::jsonb, 'es', 1, now(), now(), now()) RETURNING id`, sitio, numero, total, facturaJSON).Scan(&pID); err != nil {
			t.Fatalf("crear pedido: %v", err)
		}
		if _, err := super.Exec(ctx, `INSERT INTO order_lines (order_id, tenant, position, sku, name, quantity, unit_price_cents, vat_rate, total_cents, withdrawal_exempt)
			VALUES ($1, 'org', 1, 'PAN-01', 'Hogaza de Masa Madre', 2, $2, 21, $3, '')`, pID, total/2, total); err != nil {
			t.Fatalf("crear linea: %v", err)
		}
		if err := super.QueryRow(ctx, `INSERT INTO payments (tenant, order_id, gateway_id, attempt, provider, method, provider_ref, status, amount_cents, idempotency_key)
			VALUES ('org', $1, $2, 1, 'stripe', 'card', 'ch_' || $4, 'paid', $3, 'idemp-' || $4) RETURNING id`, pID, pasarela, total, numero).Scan(&payID); err != nil {
			t.Fatalf("crear cobro: %v", err)
		}
		return pID, payID
	}

	pedidoOK, _ := crearPedidoReal("P-000101", `{"tax_id":"12345678Z","name":"Ana Empresa SL","address":{"pais":"ES","cp":"28001"}}`, 2910)
	pedidoDisabled, _ := crearPedidoReal("P-000102", `{}`, 1500)

	var pTest uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
			subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
			shipping_method, language, terms_version, terms_accepted_at, reserved_until, paid_at)
		VALUES ('org', $1, 'P-000103', true, gen_random_uuid(), decode('00','hex'), 'paid',
			2910, 0, 2910, 505, '{}', 'test@example.test', 'Test', '{}', 'es', 1, now(), now(), now()) RETURNING id`, sitio).Scan(&pTest); err != nil {
		t.Fatal(err)
	}

	var llamadasERP int
	syerpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadasERP++
		if r.Method != http.MethodPost || r.URL.Path != "/internal/pedidos-tienda" {
			t.Errorf("ruta inesperada: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer secreto-syerp" {
			t.Errorf("Authorization header erróneo: %q", auth)
		}
		if tenant := r.Header.Get("X-Sytue-Tenant"); tenant != "org" {
			t.Errorf("X-Sytue-Tenant header erróneo: %q", tenant)
		}

		var pt pedidoTienda
		if err := json.NewDecoder(r.Body).Decode(&pt); err != nil {
			t.Errorf("error decodificando pedidoTienda: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if pt.Numero == "P-000101" {
			if pt.Factura != "completa" || pt.Comprador.NIF != "12345678Z" {
				t.Errorf("pedidoOK debería ser factura completa con NIF, obtenido %+v", pt)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"estado":      "creado",
				"pedido_ref":  "PV-2026-0001",
				"factura_ref": "FV-2026-0001",
				"verifactu":   "pendiente",
			})
			return
		}

		if pt.Numero == "P-000102" {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"code":    "modules_disabled",
				"message": "Ventas and facturacion modules are disabled",
			})
			return
		}

		t.Errorf("petición a SyERP inesperada para pedido %s", pt.Numero)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(syerpSrv.Close)

	svc := Nuevo(store.New(super), nil, "https://web.example.test", "sites.example.test", nil, nil).
		ConSyERP(syerpSrv.URL, "secreto-syerp")

	// 1. Pedido live exitoso a SyERP
	cargaOK, _ := json.Marshal(map[string]any{"order_id": pedidoOK, "site_id": sitio})
	evOK := events.Event{ID: uuid.NewString(), Channel: "pages", Type: "order.paid.erp", Tenant: "org", Payload: cargaOK}
	if err := svc.entregarAviso(ctx, evOK); err != nil {
		t.Fatalf("entregarAviso pedidoOK: %v", err)
	}

	var status, orderRef, invRef string
	if err := super.QueryRow(ctx, `SELECT erp_status, erp_order_ref, erp_invoice_ref FROM orders WHERE id = $1`, pedidoOK).
		Scan(&status, &orderRef, &invRef); err != nil {
		t.Fatal(err)
	}
	if status != "done" || orderRef != "PV-2026-0001" || invRef != "FV-2026-0001" {
		t.Errorf("pedidoOK: esperado done / PV-2026-0001 / FV-2026-0001, obtenido %q / %q / %q", status, orderRef, invRef)
	}

	// 2. Pedido live con módulos desactivados en SyERP (409)
	cargaDis, _ := json.Marshal(map[string]any{"order_id": pedidoDisabled, "site_id": sitio})
	evDis := events.Event{ID: uuid.NewString(), Channel: "pages", Type: "order.paid.erp", Tenant: "org", Payload: cargaDis}
	if err := svc.entregarAviso(ctx, evDis); err != nil {
		t.Fatalf("entregarAviso pedidoDisabled: %v", err)
	}
	var statusDis string
	if err := super.QueryRow(ctx, `SELECT erp_status FROM orders WHERE id = $1`, pedidoDisabled).Scan(&statusDis); err != nil {
		t.Fatal(err)
	}
	if statusDis != "skipped" {
		t.Errorf("pedidoDisabled: esperado status skipped, obtenido %q", statusDis)
	}

	// 3. Pedido test mode no debe llamar a SyERP
	llamadasAntes := llamadasERP
	cargaTest, _ := json.Marshal(map[string]any{"order_id": pTest, "site_id": sitio})
	evTest := events.Event{ID: uuid.NewString(), Channel: "pages", Type: "order.paid.erp", Tenant: "org", Payload: cargaTest}
	if err := svc.entregarAviso(ctx, evTest); err != nil {
		t.Fatalf("entregarAviso pTest: %v", err)
	}
	if llamadasERP != llamadasAntes {
		t.Errorf("pedido test mode no debería llamar a SyERP, llamadas=%d antes=%d", llamadasERP, llamadasAntes)
	}
}
