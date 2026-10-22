// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// tiendaDePrueba es la tienda de la víctima: sitio, ajustes, producto, pasarela, pedido,
// pago y formulario, sembrados por fuera de lo que se prueba.
type tiendaDePrueba struct {
	sitio, producto, pasarela, pedido, pago uuid.UUID
}

func sembrarTienda(t *testing.T, super *pg.Pool, tenant, slug string) tiendaDePrueba {
	t.Helper()
	ctx := context.Background()
	var x tiendaDePrueba
	paso := func(sql string, args ...any) {
		t.Helper()
		if _, err := super.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("sembrar: %v\n%s", err, sql)
		}
	}
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by)
		VALUES ($1, $2, 'Tienda', '{}', 'sub') RETURNING id`, tenant, slug).Scan(&x.sitio); err != nil {
		t.Fatal(err)
	}
	paso(`INSERT INTO store_settings (site_id, tenant, enabled, currency, countries, shipping, terms, terms_version, notify, reply_to)
		VALUES ($1, $2, true, 'EUR', '{ES}', '[]', '{}', 1, '[{"email":"dueno@victima.example"}]', 'tienda@victima.example')`, x.sitio, tenant)
	if err := super.QueryRow(ctx, `INSERT INTO products (tenant, site_id, slug, name, description, vat_rate, kind, max_per_order, active)
		VALUES ($1, $2, '{"es":"pan"}', '{"es":"Pan"}', '{}', 10, 'general', 10, true) RETURNING id`, tenant, x.sitio).Scan(&x.producto); err != nil {
		t.Fatal(err)
	}
	paso(`INSERT INTO variants (tenant, site_id, product_id, sku, options, price_cents, stock, reserved)
		VALUES ($1, $2, $3, 'PAN-1', '{}', 250, 10, 0)`, tenant, x.sitio, x.producto)
	if err := super.QueryRow(ctx, `INSERT INTO gateways (tenant, site_id, provider, mode, credentials_enc, checked_at, created_by)
		VALUES ($1, $2, 'mollie', 'test', 'encv1:de-la-victima', now(), 'sub') RETURNING id`, tenant, x.sitio).Scan(&x.pasarela); err != nil {
		t.Fatal(err)
	}
	if err := super.QueryRow(ctx, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
			subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
			shipping_method, language, terms_version, terms_accepted_at, reserved_until)
		VALUES ($1, $2, 'P-000001', true, gen_random_uuid(), decode('00','hex'), 'pending_payment',
			250, 0, 250, 0, '{}', 'comprador@example.test', 'Comprador', '{}', 'es', 1, now(), now() + interval '35 minutes')
		RETURNING id`, tenant, x.sitio).Scan(&x.pedido); err != nil {
		t.Fatal(err)
	}
	paso(`INSERT INTO order_lines (order_id, tenant, position, sku, name, quantity, unit_price_cents, vat_rate, total_cents)
		VALUES ($1, $2, 1, 'PAN-1', 'Pan', 1, 250, 10, 250)`, x.pedido, tenant)
	if err := super.QueryRow(ctx, `INSERT INTO payments (tenant, order_id, gateway_id, attempt, provider, provider_ref, status, amount_cents, idempotency_key)
		VALUES ($1, $2, $3, 1, 'mollie', $4, 'open', 250, 'k') RETURNING id`, tenant, x.pedido, x.pasarela, "tr_"+slug).Scan(&x.pago); err != nil {
		t.Fatal(err)
	}
	paso(`INSERT INTO form_settings (site_id, tenant, form_id, enabled, notify) VALUES ($1, $2, 'contacto00', true, '[]')`, x.sitio, tenant)
	return x
}

// huella es lo que NO puede cambiar en la tienda de la víctima por nada que haga otra
// organización.
func huellaDeTienda(t *testing.T, super *pg.Pool, x tiendaDePrueba) string {
	t.Helper()
	var h string
	if err := super.QueryRow(context.Background(), `
		SELECT concat_ws('|',
			(SELECT enabled::text || reply_to || notify::text || next_order_number FROM store_settings WHERE site_id = $1),
			(SELECT count(*) FROM products WHERE site_id = $1),
			(SELECT string_agg(sku || ':' || price_cents || ':' || coalesce(stock, -1) || ':' || reserved, ',' ORDER BY sku) FROM variants WHERE site_id = $1),
			(SELECT string_agg(provider || ':' || credentials_enc || ':' || active, ',' ORDER BY provider) FROM gateways WHERE site_id = $1),
			(SELECT string_agg(status, ',' ORDER BY number) FROM orders WHERE site_id = $1),
			(SELECT string_agg(status || ':' || provider_ref || ':' || attempt, ',' ORDER BY attempt) FROM payments WHERE order_id = $2),
			(SELECT count(*) FROM refunds WHERE payment_id = $3),
			(SELECT count(*) FROM withdrawals WHERE order_id = $2),
			(SELECT count(*) FROM form_submissions WHERE site_id = $1),
			(SELECT count(*) FROM domains WHERE site_id = $1),
			(SELECT count(*) FROM ai_jobs WHERE site_id = $1))`,
		x.sitio, x.pedido, x.pago).Scan(&h); err != nil {
		t.Fatal(err)
	}
	return h
}

// WEB-14. La tienda de un sitio es de su organización aunque la base no evalúe ninguna
// política: con el UUID de un sitio, un pedido o un pago ajenos, otra organización no lee
// nada ni cambia nada. Antes el único filtro de organización de `tienda.go` era la RLS.
func TestLaTiendaDeUnSitioNoEsDeOtraOrganizacion(t *testing.T) {
	dosRoles(t, func(t *testing.T, s *Store, super *pg.Pool) {
		ctx := context.Background()
		v := sembrarTienda(t, super, "victima", "tienda-victima")
		a := sembrarTienda(t, super, "atacante", "tienda-atacante")
		antes := huellaDeTienda(t, super, v)

		const yo = "atacante"
		precio := 1
		ahora := time.Now()
		lineas := []domain.OrderLine{{SKU: "PAN-1", Name: "Pan", Quantity: 1, UnitPriceCents: 1, TotalCents: 1}}
		intentos := map[string]func() error{
			"ObtenerAjustesTienda": func() error { _, err := s.ObtenerAjustesTienda(ctx, yo, v.sitio); return err },
			"GuardarAjustesTienda": func() error {
				return s.GuardarAjustesTienda(ctx, yo, domain.StoreSettings{SiteID: v.sitio, ReplyTo: "yo@atacante.example", Countries: []string{"ES"}})
			},
			"ActivarTienda":    func() error { return s.ActivarTienda(ctx, yo, v.sitio) },
			"DesactivarTienda": func() error { return s.DesactivarTienda(ctx, yo, v.sitio) },
			"ListarProductos":  func() error { _, err := s.ListarProductos(ctx, yo, v.sitio); return err },
			"ObtenerProducto":  func() error { _, err := s.ObtenerProducto(ctx, yo, v.sitio, v.producto); return err },
			"CrearProducto": func() error {
				return s.CrearProducto(ctx, yo, &domain.Product{SiteID: v.sitio, Name: map[string]string{"es": "Colado"}, VATRate: 21,
					Variants: []domain.Variant{{SKU: "COLADO", PriceCents: 1}}})
			},
			"ActualizarProducto": func() error {
				return s.ActualizarProducto(ctx, yo, &domain.Product{ID: v.producto, SiteID: v.sitio, VATRate: 21,
					Variants: []domain.Variant{{SKU: "PAN-1", PriceCents: precio}}})
			},
			"EliminarProducto": func() error { return s.EliminarProducto(ctx, yo, v.sitio, v.producto) },
			"ListarPasarelas":  func() error { _, err := s.ListarPasarelas(ctx, yo, v.sitio); return err },
			"ObtenerPasarela":  func() error { _, err := s.ObtenerPasarela(ctx, yo, v.sitio, "mollie"); return err },
			"ObtenerPasarelaConCredenciales": func() error {
				_, err := s.ObtenerPasarelaConCredenciales(ctx, yo, v.sitio, "mollie")
				return err
			},
			"GuardarPasarela": func() error {
				return s.GuardarPasarela(ctx, yo, &domain.Gateway{SiteID: v.sitio, Provider: "mollie", Mode: "test",
					CredentialsEnc: "encv1:del-atacante", Active: true, CheckedAt: ahora, CreatedBy: "sub-a"})
			},
			"DesactivarPasarela":      func() error { return s.DesactivarPasarela(ctx, yo, v.sitio, "mollie") },
			"ContarPedidosPendientes": func() error { _, err := s.ContarPedidosPendientes(ctx, yo, v.sitio); return err },
			"ListarPedidos":           func() error { _, err := s.ListarPedidos(ctx, yo, v.sitio, 10); return err },
			"ObtenerPedidoPorNumero":  func() error { _, err := s.ObtenerPedidoPorNumero(ctx, yo, v.sitio, "P-000001"); return err },
			"CrearPedidoYReservar": func() error {
				_, _, err := s.CrearPedidoYReservar(ctx, yo, v.sitio,
					&domain.Order{ID: uuid.New(), CheckoutToken: uuid.New(), AccessTokenHash: []byte{0}, Status: domain.OrderStatusPendingPayment,
						Currency: "EUR", BuyerEmail: "x@example.test", Language: "es", TermsVersion: 1, TermsAcceptedAt: ahora, ReservedUntil: ahora.Add(time.Hour), ERPStatus: "none"},
					lineas, nil)
				return err
			},
			// Por id de pedido o de pago ajenos, desde la organización del atacante.
			"ObtenerPedidoPorID(sitio ajeno)":  func() error { _, err := s.ObtenerPedidoPorID(ctx, yo, v.sitio, v.pedido); return err },
			"ObtenerPedidoPorID(sitio propio)": func() error { _, err := s.ObtenerPedidoPorID(ctx, yo, a.sitio, v.pedido); return err },
			"ObtenerPago":                      func() error { _, err := s.ObtenerPago(ctx, yo, v.pago); return err },
			"ObtenerUltimoPagoPorOrden":        func() error { _, err := s.ObtenerUltimoPagoPorOrden(ctx, yo, v.pedido); return err },
			"CrearSiguienteIntentoPago(pedido ajeno)": func() error {
				return s.CrearSiguienteIntentoPago(ctx, yo, &domain.Payment{ID: uuid.New(), OrderID: v.pedido, GatewayID: a.pasarela, Attempt: 2,
					Provider: "mollie", Status: domain.PaymentStatusCreated, AmountCents: 1, IdempotencyKey: "x"})
			},
			"CrearSiguienteIntentoPago(pasarela ajena)": func() error {
				return s.CrearSiguienteIntentoPago(ctx, yo, &domain.Payment{ID: uuid.New(), OrderID: a.pedido, GatewayID: v.pasarela, Attempt: 2,
					Provider: "mollie", Status: domain.PaymentStatusCreated, AmountCents: 1, IdempotencyKey: "y"})
			},
			"MarcarPedidoPagado": func() error { _, _, _, err := s.MarcarPedidoPagado(ctx, yo, v.pedido, v.pago, "tr_x", 1); return err },
			"CrearReembolso": func() error {
				return s.CrearReembolso(ctx, yo, &domain.Refund{ID: uuid.New(), PaymentID: v.pago, AmountCents: 1, IdempotencyKey: "r", Status: "pending", CreatedBy: "sub-a"})
			},
			"ObtenerDefinicionFormulario": func() error { _, err := s.ObtenerDefinicionFormulario(ctx, yo, v.sitio, "contacto00"); return err },
			"GuardarFormulario": func() error {
				return s.GuardarFormulario(ctx, yo, &domain.FormSubmission{ID: uuid.New(), SiteID: v.sitio, FormID: "contacto00",
					Data: map[string]any{"x": 1}, Language: "es", ConsentAt: ahora})
			},
			// Dominios y trabajos de IA cuelgan igual de un sitio.
			"ListarDominiosPorSitio": func() error { _, err := s.ListarDominiosPorSitio(ctx, yo, v.sitio); return err },
			"ContarGruposDominios":   func() error { _, err := s.ContarGruposDominios(ctx, yo, v.sitio); return err },
			"CrearDominio": func() error {
				return s.CrearDominio(ctx, &domain.Domain{Tenant: yo, SiteID: v.sitio, Host: "www.colado.example",
					Token: "0123456789abcdef0123456789abcdef", CreatedBy: "sub-a"})
			},
			"CrearTrabajoAI": func() error {
				_, err := s.CrearTrabajoAI(ctx, &domain.AIJob{Tenant: yo, SiteID: v.sitio, Kind: domain.AIJobKindDesign, Input: []byte(`{}`), Provider: "scaleway", CreatedBy: "sub-a"})
				return err
			},
		}
		for nombre, intento := range intentos {
			if err := intento(); err == nil {
				t.Errorf("%s sobre lo de otra organización no dio error", nombre)
			}
		}
		// Los que no devuelven error cuando no hay nada que tocar: que no toquen nada.
		if err := s.ActualizarEstadoPago(ctx, yo, v.pago, "tr_del_atacante", domain.PaymentStatusPaid); err != nil {
			t.Errorf("ActualizarEstadoPago: %v", err)
		}
		if err := s.LiberarReservaPedidoVencido(ctx, yo, v.pedido); err != nil {
			t.Errorf("LiberarReservaPedidoVencido: %v", err)
		}
		if err := s.RegistrarDesistimiento(ctx, yo, &domain.Withdrawal{ID: uuid.New(), OrderID: v.pedido, Email: "x@example.test", Source: "form"}); err != nil {
			t.Errorf("RegistrarDesistimiento: %v", err)
		}
		if despues := huellaDeTienda(t, super, v); despues != antes {
			t.Fatalf("otra organización cambió la tienda de la víctima:\n antes   %s\n después %s", antes, despues)
		}

		// Control: lo mismo, desde su organización, funciona. Sin esto el test pasaría
		// también con un store que lo negara todo.
		if st, err := s.ObtenerAjustesTienda(ctx, "victima", v.sitio); err != nil || st.ReplyTo != "tienda@victima.example" {
			t.Fatalf("la víctima no lee sus ajustes: %v", err)
		}
		if ps, err := s.ListarProductos(ctx, "victima", v.sitio); err != nil || len(ps) != 1 || len(ps[0].Variants) != 1 {
			t.Fatalf("la víctima no lee su catálogo: %v %+v", err, ps)
		}
		if gw, err := s.ObtenerPasarelaConCredenciales(ctx, "victima", v.sitio, "mollie"); err != nil || gw.CredentialsEnc != "encv1:de-la-victima" {
			t.Fatalf("la víctima no lee su pasarela: %v", err)
		}
		if ps, err := s.ListarPedidos(ctx, "victima", v.sitio, 10); err != nil || len(ps) != 1 || len(ps[0].Lines) != 1 {
			t.Fatalf("la víctima no lee sus pedidos: %v", err)
		}
		if err := s.GuardarPasarela(ctx, "victima", &domain.Gateway{SiteID: v.sitio, Provider: "mollie", Mode: "test",
			CredentialsEnc: "encv1:nueva", Active: true, CheckedAt: ahora, CreatedBy: "sub"}); err != nil {
			t.Fatalf("la víctima no cambia su pasarela: %v", err)
		}
		if _, _, dup, err := s.MarcarPedidoPagado(ctx, "victima", v.pedido, v.pago, "", 250); err != nil || dup {
			t.Fatalf("la víctima no cobra su pedido: dup=%v err=%v", dup, err)
		}
		if err := s.CrearSiguienteIntentoPago(ctx, "atacante", &domain.Payment{ID: uuid.New(), OrderID: a.pedido, GatewayID: a.pasarela, Attempt: 2,
			Provider: "mollie", Status: domain.PaymentStatusCreated, AmountCents: 250, IdempotencyKey: "z"}); err != nil {
			t.Fatalf("un segundo intento legítimo no se crea: %v", err)
		}
		if err := s.RegistrarDesistimiento(ctx, "atacante", &domain.Withdrawal{ID: uuid.New(), OrderID: a.pedido, Email: "x@example.test", Source: "form"}); err != nil {
			t.Fatalf("un desistimiento legítimo no se registra: %v", err)
		}
		var n int
		if err := super.QueryRow(ctx, `SELECT count(*) FROM withdrawals WHERE order_id = $1`, a.pedido).Scan(&n); err != nil || n != 1 {
			t.Fatalf("desistimientos del pedido propio = %d (%v)", n, err)
		}
	})
}
