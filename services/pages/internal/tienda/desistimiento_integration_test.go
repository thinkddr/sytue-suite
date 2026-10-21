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

// §10.7. Un desistimiento por el formulario solo cuenta con el número Y el correo del
// comprador, y de un pedido cobrado hace menos de 12 meses y 14 días; por el enlace del
// pedido, lo prueba su token. Antes bastaba el número, que es correlativo: cualquiera
// registraba desistimientos de pedidos ajenos, y el acuse salía al comprador.
func TestUnDesistimientoEsDelCompradorYDeUnPedidoCobrado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	const org = "org"
	var sitio uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'tienda', 'Tienda', '{}', 'sub') RETURNING id`, org).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	pedido := func(numero, estado, pagado string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		if err := super.QueryRow(ctx, `INSERT INTO orders (tenant, site_id, number, test, checkout_token, access_token_hash, status,
				subtotal_cents, shipping_cents, total_cents, vat_cents, vat_breakdown, buyer_email, buyer_name,
				shipping_method, language, terms_version, terms_accepted_at, reserved_until, paid_at)
			VALUES ($1, $2, $3, true, gen_random_uuid(), decode('00','hex'), $4,
				250, 0, 250, 23, '{}', 'compradora@example.test', 'Compradora', '{}', 'es', 1, now(), now(), now() - $5::interval)
			RETURNING id`, org, sitio, numero, estado, pagado).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	cobrado := pedido("P-000001", "paid", "1 day")
	pedido("P-000002", "pending_payment", "0 days")
	pedido("P-000003", "fulfilled", "2 years")
	svc := Nuevo(store.New(super), nil, "https://web.example.test", "sites.example.test", nil, nil)
	guardados := func() int {
		t.Helper()
		var n int
		if err := super.QueryRow(ctx, `SELECT count(*) FROM withdrawals`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	for _, c := range []struct{ numero, correo string }{
		{"P-000001", "otra@example.test"},       // el número de otro
		{"P-000002", "compradora@example.test"}, // sin cobrar
		{"P-000003", "compradora@example.test"}, // fuera de plazo
	} {
		if err := svc.RegistrarDesistimientoPublico(ctx, org, sitio, c.numero, c.correo, "X", "", nil); !errors.Is(err, domain.ErrPedidoNoExiste) || guardados() != 0 {
			t.Fatalf("desistir de %s con %s: err=%v y %d guardados; quiero que no cuente", c.numero, c.correo, err, guardados())
		}
	}
	if err := svc.RegistrarDesistimientoPublico(ctx, org, sitio, "P-000001", " Compradora@Example.test ", "C", "", nil); err != nil || guardados() != 1 {
		t.Fatalf("el comprador desiste por el formulario: err=%v y %d guardados", err, guardados())
	}
	if err := svc.RegistrarDesistimientoPublico(ctx, org, sitio, "", "", "C", "", &cobrado); err != nil || guardados() != 2 {
		t.Fatalf("el comprador desiste por el enlace del pedido: err=%v y %d guardados", err, guardados())
	}
	var fuentes, correos int
	if err := super.QueryRow(ctx, `SELECT count(DISTINCT source), count(*) FILTER (WHERE email = 'compradora@example.test') FROM withdrawals`).Scan(&fuentes, &correos); err != nil {
		t.Fatal(err)
	}
	if fuentes != 2 || correos != 2 {
		t.Fatalf("fuentes %d y correos del pedido %d; quiero form y token, y el correo del pedido en los dos", fuentes, correos)
	}
}
