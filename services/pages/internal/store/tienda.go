// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// ObtenerAjustesTienda lee los ajustes de la tienda de un sitio.
func (s *Store) ObtenerAjustesTienda(ctx context.Context, tenant string, siteID uuid.UUID) (*domain.StoreSettings, error) {
	var st domain.StoreSettings
	var shippingRaw, termsRaw, notifyRaw []byte

	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT site_id, tenant, enabled, currency, countries, shipping, terms,
			       terms_version, notify, reply_to, erp_enabled, next_order_number, updated_at
			  FROM store_settings
			 WHERE site_id = $1 AND tenant = $2`, siteID, tenant).Scan(
			&st.SiteID, &st.Tenant, &st.Enabled, &st.Currency, &st.Countries,
			&shippingRaw, &termsRaw, &st.TermsVersion, &notifyRaw,
			&st.ReplyTo, &st.ERPEnabled, &st.NextOrderNumber, &st.UpdatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			st = domain.StoreSettings{
				SiteID:          siteID,
				Tenant:          tenant,
				Enabled:         false,
				Currency:        "EUR",
				Countries:       []string{"ES"},
				Shipping:        []domain.ShippingMethod{},
				Terms:           domain.StoreTerms{DeliveryTime: map[string]string{}},
				TermsVersion:    1,
				Notify:          []domain.StoreNotifyPerson{},
				ReplyTo:         "",
				ERPEnabled:      false,
				NextOrderNumber: 1,
				UpdatedAt:       time.Now().UTC(),
			}
			return nil
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	if len(shippingRaw) > 0 {
		_ = json.Unmarshal(shippingRaw, &st.Shipping)
	}
	if len(termsRaw) > 0 {
		_ = json.Unmarshal(termsRaw, &st.Terms)
	}
	if len(notifyRaw) > 0 {
		_ = json.Unmarshal(notifyRaw, &st.Notify)
	}

	return &st, nil
}

// GuardarAjustesTienda guarda o actualiza la configuración de la tienda.
func (s *Store) GuardarAjustesTienda(ctx context.Context, tenant string, st domain.StoreSettings) error {
	shippingJSON, err := json.Marshal(st.Shipping)
	if err != nil {
		return fmt.Errorf("tienda: serializar envíos: %w", err)
	}
	termsJSON, err := json.Marshal(st.Terms)
	if err != nil {
		return fmt.Errorf("tienda: serializar términos: %w", err)
	}
	notifyJSON, err := json.Marshal(st.Notify)
	if err != nil {
		return fmt.Errorf("tienda: serializar avisos: %w", err)
	}

	return s.enSitio(ctx, tenant, st.SiteID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO store_settings (
				site_id, tenant, enabled, currency, countries, shipping, terms,
				terms_version, notify, reply_to, erp_enabled, next_order_number, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now())
			ON CONFLICT (site_id) DO UPDATE SET
				currency = EXCLUDED.currency,
				countries = EXCLUDED.countries,
				shipping = EXCLUDED.shipping,
				terms = EXCLUDED.terms,
				terms_version = EXCLUDED.terms_version,
				notify = EXCLUDED.notify,
				reply_to = EXCLUDED.reply_to,
				erp_enabled = EXCLUDED.erp_enabled,
				updated_at = now()
			WHERE store_settings.tenant = EXCLUDED.tenant`,
			st.SiteID, tenant, st.Enabled, "EUR", st.Countries, shippingJSON, termsJSON,
			st.TermsVersion, notifyJSON, st.ReplyTo, st.ERPEnabled, st.NextOrderNumber,
		)
		return err
	})
}

// ActivarTienda comprueba los 5 requisitos obligatorios (§10.3) antes de habilitar la tienda:
// 1. Datos legales completos en el sitio (tax_id, name, address).
// 2. Al menos una pasarela de pago activa.
// 3. Países de envío configurados.
// 4. Al menos un método de envío.
// 5. Correo configurado (reply_to o notify) para acuse de recibo.
func (s *Store) ActivarTienda(ctx context.Context, tenant string, siteID uuid.UUID) error {
	return s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		// 1. Datos legales del sitio
		var legalRaw []byte
		err := tx.QueryRow(ctx, `SELECT legal FROM sites WHERE id = $1 AND tenant = $2`, siteID, tenant).Scan(&legalRaw)
		if err != nil {
			return err
		}
		var legal struct {
			TaxID   string `json:"tax_id"`
			Name    string `json:"name"`
			Address string `json:"address"`
		}
		if len(legalRaw) > 0 {
			_ = json.Unmarshal(legalRaw, &legal)
		}
		if strings.TrimSpace(legal.TaxID) == "" || strings.TrimSpace(legal.Name) == "" || strings.TrimSpace(legal.Address) == "" {
			return domain.ErrTiendaNoLista
		}

		// 2. Al menos una pasarela activa
		var pasarelasActivas int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM gateways WHERE site_id = $1 AND tenant = $2 AND active = true`, siteID, tenant).Scan(&pasarelasActivas)
		if err != nil {
			return err
		}
		if pasarelasActivas == 0 {
			return domain.ErrTiendaNoLista
		}

		// 3, 4, 5. Ajustes de tienda
		var countries []string
		var shippingRaw, notifyRaw []byte
		var replyTo string
		err = tx.QueryRow(ctx, `
			SELECT countries, shipping, notify, reply_to
			  FROM store_settings
			 WHERE site_id = $1 AND tenant = $2`, siteID, tenant).Scan(&countries, &shippingRaw, &notifyRaw, &replyTo)
		if err != nil {
			return domain.ErrTiendaNoLista
		}

		var shipping []domain.ShippingMethod
		if len(shippingRaw) > 0 {
			_ = json.Unmarshal(shippingRaw, &shipping)
		}
		var notify []domain.StoreNotifyPerson
		if len(notifyRaw) > 0 {
			_ = json.Unmarshal(notifyRaw, &notify)
		}

		if len(countries) == 0 || len(shipping) == 0 || (strings.TrimSpace(replyTo) == "" && len(notify) == 0) {
			return domain.ErrTiendaNoLista
		}

		_, err = tx.Exec(ctx, `UPDATE store_settings SET enabled = true, updated_at = now() WHERE site_id = $1 AND tenant = $2`, siteID, tenant)
		return err
	})
}

// DesactivarTienda pone enabled = false en los ajustes de tienda.
func (s *Store) DesactivarTienda(ctx context.Context, tenant string, siteID uuid.UUID) error {
	return s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE store_settings SET enabled = false, updated_at = now() WHERE site_id = $1 AND tenant = $2`, siteID, tenant)
		return err
	})
}

// CrearProducto inserta un producto, sus variantes y registra el precio inicial en price_history.
func (s *Store) CrearProducto(ctx context.Context, tenant string, p *domain.Product) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if !domain.TiposIVAPermitidos[p.VATRate] {
		return fmt.Errorf("%w: tipo de IVA %.2f no permitido", domain.ErrProductoInvalido, p.VATRate)
	}
	if len(p.Options) > 3 {
		return fmt.Errorf("%w: máximo 3 ejes de opciones", domain.ErrProductoInvalido)
	}
	if p.MaxPerOrder < 1 || p.MaxPerOrder > 99 {
		p.MaxPerOrder = 10
	}
	if p.Kind != "general" && p.Kind != "food" {
		p.Kind = "general"
	}

	slugJSON, _ := json.Marshal(p.Slug)
	nameJSON, _ := json.Marshal(p.Name)
	descJSON, _ := json.Marshal(p.Description)
	optsJSON, _ := json.Marshal(p.Options)
	gpsrJSON, _ := json.Marshal(p.GPSR)
	foodJSON, _ := json.Marshal(p.Food)

	return s.enSitio(ctx, tenant, p.SiteID, func(tx pgx.Tx) error {
		ahora := time.Now().UTC()
		_, err := tx.Exec(ctx, `
			INSERT INTO products (
				id, tenant, site_id, slug, name, description, category, images,
				vat_rate, options, kind, gpsr, food, withdrawal_exempt, max_per_order,
				active, position, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $18)`,
			p.ID, tenant, p.SiteID, slugJSON, nameJSON, descJSON, p.Category, p.Images,
			p.VATRate, optsJSON, p.Kind, gpsrJSON, foodJSON, p.WithdrawalExempt, p.MaxPerOrder,
			p.Active, p.Position, ahora,
		)
		if err != nil {
			return fmt.Errorf("tienda: insertar producto: %w", err)
		}

		for i := range p.Variants {
			v := &p.Variants[i]
			if v.ID == uuid.Nil {
				v.ID = uuid.New()
			}
			v.SiteID = p.SiteID
			v.ProductID = p.ID
			v.Tenant = tenant
			vOptionsJSON, _ := json.Marshal(v.Options)

			_, err = tx.Exec(ctx, `
				INSERT INTO variants (id, tenant, site_id, product_id, sku, options, price_cents, compare_at_cents, stock, reserved)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				v.ID, tenant, p.SiteID, p.ID, v.SKU, vOptionsJSON, v.PriceCents, v.CompareAtCents, v.Stock, 0,
			)
			if err != nil {
				return fmt.Errorf("tienda: insertar variante %s: %w", v.SKU, err)
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO price_history (variant_id, tenant, price_cents, since)
				VALUES ($1, $2, $3, $4)`,
				v.ID, tenant, v.PriceCents, ahora,
			)
			if err != nil {
				return fmt.Errorf("tienda: insertar historial precio: %w", err)
			}
		}

		p.CreatedAt = ahora
		p.UpdatedAt = ahora
		return nil
	})
}

// ObtenerProducto devuelve un producto con sus variantes.
func (s *Store) ObtenerProducto(ctx context.Context, tenant string, siteID, productID uuid.UUID) (*domain.Product, error) {
	var p domain.Product
	var slugRaw, nameRaw, descRaw, optsRaw, gpsrRaw, foodRaw []byte

	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, tenant, site_id, slug, name, description, category, images,
			       vat_rate, options, kind, gpsr, food, withdrawal_exempt, max_per_order,
			       active, position, created_at, updated_at
			  FROM products
			 WHERE site_id = $1 AND id = $2 AND tenant = $3`, siteID, productID, tenant).Scan(
			&p.ID, &p.Tenant, &p.SiteID, &slugRaw, &nameRaw, &descRaw, &p.Category, &p.Images,
			&p.VATRate, &optsRaw, &p.Kind, &gpsrRaw, &foodRaw, &p.WithdrawalExempt, &p.MaxPerOrder,
			&p.Active, &p.Position, &p.CreatedAt, &p.UpdatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		if err != nil {
			return err
		}

		rows, err := tx.Query(ctx, `
			SELECT id, tenant, site_id, product_id, sku, options, price_cents, compare_at_cents, stock, reserved
			  FROM variants
			 WHERE product_id = $1 AND tenant = $2
			 ORDER BY sku ASC`, productID, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v domain.Variant
			var vOptsRaw []byte
			if err := rows.Scan(
				&v.ID, &v.Tenant, &v.SiteID, &v.ProductID, &v.SKU, &vOptsRaw,
				&v.PriceCents, &v.CompareAtCents, &v.Stock, &v.Reserved,
			); err != nil {
				return err
			}
			if len(vOptsRaw) > 0 {
				_ = json.Unmarshal(vOptsRaw, &v.Options)
			}
			p.Variants = append(p.Variants, v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	if len(slugRaw) > 0 {
		_ = json.Unmarshal(slugRaw, &p.Slug)
	}
	if len(nameRaw) > 0 {
		_ = json.Unmarshal(nameRaw, &p.Name)
	}
	if len(descRaw) > 0 {
		_ = json.Unmarshal(descRaw, &p.Description)
	}
	if len(optsRaw) > 0 {
		_ = json.Unmarshal(optsRaw, &p.Options)
	}
	if len(gpsrRaw) > 0 {
		_ = json.Unmarshal(gpsrRaw, &p.GPSR)
	}
	if len(foodRaw) > 0 {
		_ = json.Unmarshal(foodRaw, &p.Food)
	}

	return &p, nil
}

// ListarProductos devuelve todos los productos de un sitio con sus variantes.
func (s *Store) ListarProductos(ctx context.Context, tenant string, siteID uuid.UUID) ([]domain.Product, error) {
	var productos []domain.Product

	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, site_id, slug, name, description, category, images,
			       vat_rate, options, kind, gpsr, food, withdrawal_exempt, max_per_order,
			       active, position, created_at, updated_at
			  FROM products
			 WHERE site_id = $1 AND tenant = $2
			 ORDER BY position ASC, created_at DESC`, siteID, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			p, errScan := escanearFilaProducto(rows)
			if errScan != nil {
				return errScan
			}
			productos = append(productos, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		variantesPorProd, err := leerVariantesDeSitio(ctx, tx, tenant, siteID)
		if err != nil {
			return err
		}

		for i := range productos {
			productos[i].Variants = variantesPorProd[productos[i].ID]
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return productos, nil
}

func escanearFilaProducto(rows pgx.Rows) (domain.Product, error) {
	var p domain.Product
	var slugRaw, nameRaw, descRaw, optsRaw, gpsrRaw, foodRaw []byte
	if err := rows.Scan(
		&p.ID, &p.Tenant, &p.SiteID, &slugRaw, &nameRaw, &descRaw, &p.Category, &p.Images,
		&p.VATRate, &optsRaw, &p.Kind, &gpsrRaw, &foodRaw, &p.WithdrawalExempt, &p.MaxPerOrder,
		&p.Active, &p.Position, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return p, err
	}
	if len(slugRaw) > 0 {
		_ = json.Unmarshal(slugRaw, &p.Slug)
	}
	if len(nameRaw) > 0 {
		_ = json.Unmarshal(nameRaw, &p.Name)
	}
	if len(descRaw) > 0 {
		_ = json.Unmarshal(descRaw, &p.Description)
	}
	if len(optsRaw) > 0 {
		_ = json.Unmarshal(optsRaw, &p.Options)
	}
	if len(gpsrRaw) > 0 {
		_ = json.Unmarshal(gpsrRaw, &p.GPSR)
	}
	if len(foodRaw) > 0 {
		_ = json.Unmarshal(foodRaw, &p.Food)
	}
	return p, nil
}

func leerVariantesDeSitio(ctx context.Context, tx pgx.Tx, tenant string, siteID uuid.UUID) (map[uuid.UUID][]domain.Variant, error) {
	varRows, err := tx.Query(ctx, `
		SELECT id, tenant, site_id, product_id, sku, options, price_cents, compare_at_cents, stock, reserved
		  FROM variants
		 WHERE site_id = $1 AND tenant = $2
		 ORDER BY sku ASC`, siteID, tenant)
	if err != nil {
		return nil, err
	}
	defer varRows.Close()

	variantesPorProd := make(map[uuid.UUID][]domain.Variant)
	for varRows.Next() {
		var v domain.Variant
		var vOptsRaw []byte
		if err := varRows.Scan(
			&v.ID, &v.Tenant, &v.SiteID, &v.ProductID, &v.SKU, &vOptsRaw,
			&v.PriceCents, &v.CompareAtCents, &v.Stock, &v.Reserved,
		); err != nil {
			return nil, err
		}
		if len(vOptsRaw) > 0 {
			_ = json.Unmarshal(vOptsRaw, &v.Options)
		}
		variantesPorProd[v.ProductID] = append(variantesPorProd[v.ProductID], v)
	}
	return variantesPorProd, varRows.Err()
}

// ActualizarProducto modifica los campos de un producto y sincroniza sus variantes e historial de precios.
func (s *Store) ActualizarProducto(ctx context.Context, tenant string, p *domain.Product) error {
	if !domain.TiposIVAPermitidos[p.VATRate] {
		return fmt.Errorf("%w: tipo de IVA %.2f no permitido", domain.ErrProductoInvalido, p.VATRate)
	}
	if len(p.Options) > 3 {
		return fmt.Errorf("%w: máximo 3 ejes de opciones", domain.ErrProductoInvalido)
	}
	if p.MaxPerOrder < 1 || p.MaxPerOrder > 99 {
		p.MaxPerOrder = 10
	}
	if p.Kind != "general" && p.Kind != "food" {
		p.Kind = "general"
	}

	slugJSON, _ := json.Marshal(p.Slug)
	nameJSON, _ := json.Marshal(p.Name)
	descJSON, _ := json.Marshal(p.Description)
	optsJSON, _ := json.Marshal(p.Options)
	gpsrJSON, _ := json.Marshal(p.GPSR)
	foodJSON, _ := json.Marshal(p.Food)

	return s.enSitio(ctx, tenant, p.SiteID, func(tx pgx.Tx) error {
		ahora := time.Now().UTC()
		tag, err := tx.Exec(ctx, `
			UPDATE products
			   SET slug = $1, name = $2, description = $3, category = $4, images = $5,
			       vat_rate = $6, options = $7, kind = $8, gpsr = $9, food = $10,
			       withdrawal_exempt = $11, max_per_order = $12, active = $13, position = $14,
			       updated_at = now()
			 WHERE site_id = $15 AND id = $16 AND tenant = $17`,
			slugJSON, nameJSON, descJSON, p.Category, p.Images,
			p.VATRate, optsJSON, p.Kind, gpsrJSON, foodJSON,
			p.WithdrawalExempt, p.MaxPerOrder, p.Active, p.Position,
			p.SiteID, p.ID, tenant,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}

		for i := range p.Variants {
			v := &p.Variants[i]
			if v.ID == uuid.Nil {
				v.ID = uuid.New()
			}
			v.SiteID = p.SiteID
			v.ProductID = p.ID
			v.Tenant = tenant
			vOptionsJSON, _ := json.Marshal(v.Options)

			var precioAnterior int
			errPrecio := tx.QueryRow(ctx, `SELECT price_cents FROM variants WHERE id = $1 AND tenant = $2`, v.ID, tenant).Scan(&precioAnterior)

			_, err = tx.Exec(ctx, `
				INSERT INTO variants (id, tenant, site_id, product_id, sku, options, price_cents, compare_at_cents, stock, reserved)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (site_id, sku) DO UPDATE SET
					options = EXCLUDED.options,
					price_cents = EXCLUDED.price_cents,
					compare_at_cents = EXCLUDED.compare_at_cents,
					stock = EXCLUDED.stock
				WHERE variants.tenant = EXCLUDED.tenant`,
				v.ID, tenant, p.SiteID, p.ID, v.SKU, vOptionsJSON, v.PriceCents, v.CompareAtCents, v.Stock, v.Reserved,
			)
			if err != nil {
				return err
			}

			if errors.Is(errPrecio, pgx.ErrNoRows) || precioAnterior != v.PriceCents {
				_, _ = tx.Exec(ctx, `
					INSERT INTO price_history (variant_id, tenant, price_cents, since)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (variant_id, since) DO NOTHING`,
					v.ID, tenant, v.PriceCents, ahora,
				)
			}
		}

		p.UpdatedAt = ahora
		return nil
	})
}

// EliminarProducto borra un producto y sus variantes asociadas.
func (s *Store) EliminarProducto(ctx context.Context, tenant string, siteID, productID uuid.UUID) error {
	return s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM products WHERE site_id = $1 AND id = $2 AND tenant = $3`, siteID, productID, tenant)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// ListarPasarelas devuelve las pasarelas configuradas para un sitio (sin exponer credenciales).
func (s *Store) ListarPasarelas(ctx context.Context, tenant string, siteID uuid.UUID) ([]domain.Gateway, error) {
	var list []domain.Gateway
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant, site_id, provider, mode, public_config, next_ref, active,
			       notice_accepted_at, notice_accepted_by, checked_at, created_by,
			       created_at, updated_at
			  FROM gateways
			 WHERE site_id = $1 AND tenant = $2
			 ORDER BY provider ASC`, siteID, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var g domain.Gateway
			var pubRaw []byte
			if err := rows.Scan(
				&g.ID, &g.Tenant, &g.SiteID, &g.Provider, &g.Mode, &pubRaw, &g.NextRef, &g.Active,
				&g.NoticeAcceptedAt, &g.NoticeAcceptedBy, &g.CheckedAt, &g.CreatedBy,
				&g.CreatedAt, &g.UpdatedAt,
			); err != nil {
				return err
			}
			if len(pubRaw) > 0 {
				_ = json.Unmarshal(pubRaw, &g.PublicConfig)
			}
			list = append(list, g)
		}
		return rows.Err()
	})
	return list, err
}

// ObtenerPasarela lee una pasarela específica por proveedor.
func (s *Store) ObtenerPasarela(ctx context.Context, tenant string, siteID uuid.UUID, provider string) (*domain.Gateway, error) {
	var g domain.Gateway
	var pubRaw []byte

	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, tenant, site_id, provider, mode, public_config, next_ref, active,
			       notice_accepted_at, notice_accepted_by, checked_at, created_by,
			       created_at, updated_at
			  FROM gateways
			 WHERE site_id = $1 AND provider = $2 AND tenant = $3`, siteID, provider, tenant).Scan(
			&g.ID, &g.Tenant, &g.SiteID, &g.Provider, &g.Mode, &pubRaw, &g.NextRef, &g.Active,
			&g.NoticeAcceptedAt, &g.NoticeAcceptedBy, &g.CheckedAt, &g.CreatedBy,
			&g.CreatedAt, &g.UpdatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(pubRaw) > 0 {
		_ = json.Unmarshal(pubRaw, &g.PublicConfig)
	}
	return &g, nil
}

// ObtenerPasarelaConCredenciales lee una pasarela incluyendo sus credenciales selladas para procesar pagos.
func (s *Store) ObtenerPasarelaConCredenciales(ctx context.Context, tenant string, siteID uuid.UUID, provider string) (*domain.Gateway, error) {
	var g domain.Gateway
	var pubRaw []byte

	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, tenant, site_id, provider, mode, credentials_enc, public_config, next_ref, active,
			       notice_accepted_at, notice_accepted_by, checked_at, created_by,
			       created_at, updated_at
			  FROM gateways
			 WHERE site_id = $1 AND provider = $2 AND tenant = $3`, siteID, provider, tenant).Scan(
			&g.ID, &g.Tenant, &g.SiteID, &g.Provider, &g.Mode, &g.CredentialsEnc, &pubRaw, &g.NextRef, &g.Active,
			&g.NoticeAcceptedAt, &g.NoticeAcceptedBy, &g.CheckedAt, &g.CreatedBy,
			&g.CreatedAt, &g.UpdatedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNoExiste
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(pubRaw) > 0 {
		_ = json.Unmarshal(pubRaw, &g.PublicConfig)
	}
	return &g, nil
}

// GuardarPasarela guarda una pasarela con credenciales selladas.
// Si se guarda como activa, desactiva automáticamente cualquier otra pasarela activa con modo diferente (§10.3).
func (s *Store) GuardarPasarela(ctx context.Context, tenant string, g *domain.Gateway) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	if g.Mode != "test" && g.Mode != "live" {
		g.Mode = "test"
	}
	if g.Provider == "stripe" && g.NoticeAcceptedAt == nil {
		return fmt.Errorf("%w: stripe exige aceptar el aviso legal de transferencia internacional", domain.ErrCredencialesPasarela)
	}
	if !strings.HasPrefix(g.CredentialsEnc, "encv1:") {
		return fmt.Errorf("%w: las credenciales deben estar selladas", domain.ErrCredencialesPasarela)
	}

	pubJSON, _ := json.Marshal(g.PublicConfig)
	ahora := time.Now().UTC()

	return s.enSitio(ctx, tenant, g.SiteID, func(tx pgx.Tx) error {
		if g.Active {
			// Todas las pasarelas activas de un sitio deben estar en el MISMO modo (§10.3)
			_, err := tx.Exec(ctx, `
				UPDATE gateways
				   SET active = false, updated_at = now()
				 WHERE site_id = $1 AND mode <> $2 AND active = true AND tenant = $3`,
				g.SiteID, g.Mode, tenant,
			)
			if err != nil {
				return fmt.Errorf("tienda: alinear modos de pasarelas: %w", err)
			}
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO gateways (
				id, tenant, site_id, provider, mode, credentials_enc, public_config,
				next_ref, active, notice_accepted_at, notice_accepted_by, checked_at,
				created_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
			ON CONFLICT (site_id, provider) DO UPDATE SET
				mode = EXCLUDED.mode,
				credentials_enc = EXCLUDED.credentials_enc,
				public_config = EXCLUDED.public_config,
				active = EXCLUDED.active,
				notice_accepted_at = EXCLUDED.notice_accepted_at,
				notice_accepted_by = EXCLUDED.notice_accepted_by,
				checked_at = EXCLUDED.checked_at,
				updated_at = now()
			WHERE gateways.tenant = EXCLUDED.tenant`,
			g.ID, tenant, g.SiteID, g.Provider, g.Mode, g.CredentialsEnc, pubJSON,
			g.NextRef, g.Active, g.NoticeAcceptedAt, g.NoticeAcceptedBy, g.CheckedAt,
			g.CreatedBy, ahora,
		)
		return err
	})
}

// DesactivarPasarela marca active = false en la pasarela.
func (s *Store) DesactivarPasarela(ctx context.Context, tenant string, siteID uuid.UUID, provider string) error {
	return s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE gateways
			   SET active = false, updated_at = now()
			 WHERE site_id = $1 AND provider = $2 AND tenant = $3`,
			siteID, provider, tenant,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// ContarPedidosPendientes cuenta cuántos pedidos pending_payment vivos tiene un sitio (§10.4).
func (s *Store) ContarPedidosPendientes(ctx context.Context, tenant string, siteID uuid.UUID) (int, error) {
	var count int
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*)
			  FROM orders
			 WHERE site_id = $1 AND tenant = $2 AND status = 'pending_payment' AND reserved_until > now()`,
			siteID, tenant,
		).Scan(&count)
	})
	return count, err
}

// Topes de pedidos pendientes de pago vivos de una tienda (§10.4, WEB-06).
const (
	// maxPendientesTienda es el de la tienda entera: con él lleno, nadie compra.
	maxPendientesTienda = 30
	// maxPendientesComprador es el de un mismo correo: sin él, un solo visitante
	// llenaba los 30 huecos de la tienda con un ALTCHA por pedido.
	maxPendientesComprador = 3
)

// CrearPedidoYReservar realiza en UNA transacción (§10.4):
// 1. Idempotencia por checkout_token y topes de pedidos pendientes.
// 2. Reserva de variantes en variants (reserved = reserved + n).
// 3. Asignación de número secuencial P-000123.
// 4. Inserción de pedido, líneas y pago inicial.
//
// Los topes son tres pedidos sin pagar por comprador (ErrDemasiadosPendientes) y
// treinta por tienda (ErrTiendaOcupada).
func (s *Store) CrearPedidoYReservar(
	ctx context.Context,
	tenant string,
	siteID uuid.UUID,
	order *domain.Order,
	lines []domain.OrderLine,
	payment *domain.Payment,
) (*domain.Order, *domain.Payment, error) {
	var existingOrder *domain.Order
	var existingPayment *domain.Payment

	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		existingID, found, err := comprobarPendientesEIdempotenciaTx(ctx, tx, tenant, siteID, order.CheckoutToken, order.BuyerEmail)
		if err != nil {
			return err
		}
		if found {
			existingOrder, err = s.obtenerPedidoTx(ctx, tx, tenant, existingID)
			if err != nil {
				return err
			}
			existingPayment, _ = s.obtenerUltimoPagoTx(ctx, tx, tenant, existingID)
			return nil
		}

		if err := reservarLineasTx(ctx, tx, tenant, siteID, lines); err != nil {
			return err
		}

		order.Number = asignarSiguienteNumeroPedidoTx(ctx, tx, tenant, siteID)

		return insertarPedidoYLineasTx(ctx, tx, tenant, siteID, order, lines, payment)
	})
	if err != nil {
		return nil, nil, err
	}
	if existingOrder != nil {
		return existingOrder, existingPayment, nil
	}
	return order, payment, nil
}

// comprobarPendientesEIdempotenciaTx devuelve el pedido de ese checkout_token si ya
// existe (el doble clic), y si no, aplica los topes de pedidos pendientes. La
// idempotencia va primero: repetir un checkout ya hecho no es pedir otro hueco.
func comprobarPendientesEIdempotenciaTx(ctx context.Context, tx pgx.Tx, tenant string, siteID, token uuid.UUID, correo string) (uuid.UUID, bool, error) {
	var existingID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id
		  FROM orders
		 WHERE site_id = $1 AND checkout_token = $2 AND tenant = $3`,
		siteID, token, tenant,
	).Scan(&existingID)
	if err == nil {
		return existingID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, err
	}

	var pendientes, delComprador int
	err = tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE lower(btrim(buyer_email)) = lower($3))
		  FROM orders
		 WHERE site_id = $1 AND tenant = $2 AND status = 'pending_payment' AND reserved_until > now()`,
		siteID, tenant, strings.TrimSpace(correo),
	).Scan(&pendientes, &delComprador)
	if err != nil {
		return uuid.Nil, false, err
	}
	if delComprador >= maxPendientesComprador {
		return uuid.Nil, false, domain.ErrDemasiadosPendientes
	}
	if pendientes >= maxPendientesTienda {
		return uuid.Nil, false, domain.ErrTiendaOcupada
	}
	return uuid.Nil, false, nil
}

func reservarLineasTx(ctx context.Context, tx pgx.Tx, tenant string, siteID uuid.UUID, lines []domain.OrderLine) error {
	for _, l := range lines {
		tag, err := tx.Exec(ctx, `
			UPDATE variants
			   SET reserved = reserved + $1
			 WHERE site_id = $2 AND sku = $3 AND tenant = $4
			   AND (stock IS NULL OR stock - reserved >= $1)`,
			l.Quantity, siteID, l.SKU, tenant,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrExistenciasAgotadas
		}
	}
	return nil
}

func asignarSiguienteNumeroPedidoTx(ctx context.Context, tx pgx.Tx, tenant string, siteID uuid.UUID) string {
	var nextNum int
	err := tx.QueryRow(ctx, `
		UPDATE store_settings
		   SET next_order_number = next_order_number + 1, updated_at = now()
		 WHERE site_id = $1 AND tenant = $2
		 RETURNING next_order_number - 1`,
		siteID, tenant,
	).Scan(&nextNum)
	if err != nil {
		nextNum = 1
	}
	return fmt.Sprintf("P-%06d", nextNum)
}

func insertarPedidoYLineasTx(
	ctx context.Context,
	tx pgx.Tx,
	tenant string,
	siteID uuid.UUID,
	order *domain.Order,
	lines []domain.OrderLine,
	payment *domain.Payment,
) error {
	vatJSON, _ := json.Marshal(order.VATBreakdown)
	shipMethodJSON, _ := json.Marshal(order.ShippingMethod)
	shipAddrJSON, _ := json.Marshal(order.ShippingAddress)
	invoiceJSON, _ := json.Marshal(order.Invoice)

	_, err := tx.Exec(ctx, `
		INSERT INTO orders (
			id, tenant, site_id, number, test, checkout_token, access_token_hash,
			status, oversold, currency, subtotal_cents, shipping_cents, total_cents,
			vat_cents, vat_breakdown, buyer_email, buyer_name, buyer_phone,
			shipping_method, shipping_address, invoice, language, terms_version,
			terms_accepted_at, reserved_until, erp_status, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23,
			$24, $25, $26, now(), now()
		)`,
		order.ID, tenant, siteID, order.Number, order.Test, order.CheckoutToken, order.AccessTokenHash,
		order.Status, order.Oversold, order.Currency, order.SubtotalCents, order.ShippingCents, order.TotalCents,
		order.VATCents, vatJSON, order.BuyerEmail, order.BuyerName, order.BuyerPhone,
		shipMethodJSON, shipAddrJSON, invoiceJSON, order.Language, order.TermsVersion,
		order.TermsAcceptedAt, order.ReservedUntil, order.ERPStatus,
	)
	if err != nil {
		return fmt.Errorf("store: insertar pedido: %w", err)
	}

	for i, l := range lines {
		optJSON, _ := json.Marshal(l.Options)
		_, err = tx.Exec(ctx, `
			INSERT INTO order_lines (
				order_id, tenant, position, variant_id, sku, name,
				options, quantity, unit_price_cents, vat_rate, total_cents, withdrawal_exempt
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			order.ID, tenant, int16(i+1), l.VariantID, l.SKU, l.Name,
			optJSON, l.Quantity, l.UnitPriceCents, l.VATRate, l.TotalCents, l.WithdrawalExempt,
		)
		if err != nil {
			return fmt.Errorf("store: insertar linea %d: %w", i+1, err)
		}
	}

	if payment != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO payments (
				id, tenant, order_id, gateway_id, attempt, provider, method,
				provider_ref, status, amount_cents, refunded_cents, charged_back_cents,
				idempotency_key, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now(), now())`,
			payment.ID, tenant, order.ID, payment.GatewayID, payment.Attempt, payment.Provider, payment.Method,
			payment.ProviderRef, payment.Status, payment.AmountCents, payment.RefundedCents, payment.ChargedBackCents,
			payment.IdempotencyKey,
		)
		if err != nil {
			return fmt.Errorf("store: insertar pago: %w", err)
		}
	}
	return nil
}

func (s *Store) obtenerPedidoTx(ctx context.Context, tx pgx.Tx, tenant string, orderID uuid.UUID) (*domain.Order, error) {
	var o domain.Order
	var vatRaw, shipMethodRaw, shipAddrRaw, invoiceRaw []byte

	err := tx.QueryRow(ctx, `
		SELECT id, tenant, site_id, number, test, checkout_token, access_token_hash,
		       status, oversold, currency, subtotal_cents, shipping_cents, total_cents,
		       vat_cents, vat_breakdown, buyer_email, buyer_name, buyer_phone,
		       shipping_method, shipping_address, invoice, language, terms_version,
		       terms_accepted_at, reserved_until, paid_at, fulfilled_at,
		       buyer_email_sent_at, erp_status, erp_order_ref, erp_invoice_ref,
		       anonymized_at, created_at, updated_at
		  FROM orders
		 WHERE id = $1 AND tenant = $2`, orderID, tenant,
	).Scan(
		&o.ID, &o.Tenant, &o.SiteID, &o.Number, &o.Test, &o.CheckoutToken, &o.AccessTokenHash,
		&o.Status, &o.Oversold, &o.Currency, &o.SubtotalCents, &o.ShippingCents, &o.TotalCents,
		&o.VATCents, &vatRaw, &o.BuyerEmail, &o.BuyerName, &o.BuyerPhone,
		&shipMethodRaw, &shipAddrRaw, &invoiceRaw, &o.Language, &o.TermsVersion,
		&o.TermsAcceptedAt, &o.ReservedUntil, &o.PaidAt, &o.FulfilledAt,
		&o.BuyerEmailSentAt, &o.ERPStatus, &o.ERPOrderRef, &o.ERPInvoiceRef,
		&o.AnonymizedAt, &o.CreatedAt, &o.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrPedidoNoExiste
	}
	if err != nil {
		return nil, err
	}

	if len(vatRaw) > 0 {
		_ = json.Unmarshal(vatRaw, &o.VATBreakdown)
	}
	if len(shipMethodRaw) > 0 {
		_ = json.Unmarshal(shipMethodRaw, &o.ShippingMethod)
	}
	if len(shipAddrRaw) > 0 {
		_ = json.Unmarshal(shipAddrRaw, &o.ShippingAddress)
	}
	if len(invoiceRaw) > 0 {
		_ = json.Unmarshal(invoiceRaw, &o.Invoice)
	}

	// Cargar líneas
	rows, err := tx.Query(ctx, `
		SELECT order_id, tenant, position, variant_id, sku, name,
		       options, quantity, unit_price_cents, vat_rate, total_cents, withdrawal_exempt
		  FROM order_lines
		 WHERE order_id = $1 AND tenant = $2
		 ORDER BY position ASC`, orderID, tenant,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var l domain.OrderLine
		var optRaw []byte
		if err := rows.Scan(
			&l.OrderID, &l.Tenant, &l.Position, &l.VariantID, &l.SKU, &l.Name,
			&optRaw, &l.Quantity, &l.UnitPriceCents, &l.VATRate, &l.TotalCents, &l.WithdrawalExempt,
		); err != nil {
			return nil, err
		}
		if len(optRaw) > 0 {
			_ = json.Unmarshal(optRaw, &l.Options)
		}
		o.Lines = append(o.Lines, l)
	}

	return &o, nil
}

// ListarPedidos devuelve los pedidos de un sitio para el backoffice. La
// exportación CSV usa esta misma lectura, de modo que RLS y la vista no divergen.
func (s *Store) ListarPedidos(ctx context.Context, tenant string, siteID uuid.UUID, limit int) ([]domain.Order, error) {
	if limit < 1 || limit > 1000 {
		limit = 1000
	}
	var out []domain.Order
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM orders WHERE site_id = $1 AND tenant = $3 ORDER BY created_at DESC LIMIT $2`, siteID, limit, tenant)
		if err != nil {
			return err
		}
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range ids {
			order, err := s.obtenerPedidoTx(ctx, tx, tenant, id)
			if err != nil {
				return err
			}
			out = append(out, *order)
		}
		return nil
	})
	return out, err
}

func (s *Store) obtenerUltimoPagoTx(ctx context.Context, tx pgx.Tx, tenant string, orderID uuid.UUID) (*domain.Payment, error) {
	var p domain.Payment
	err := tx.QueryRow(ctx, `
		SELECT id, tenant, order_id, gateway_id, attempt, provider, method,
		       provider_ref, status, amount_cents, refunded_cents, charged_back_cents,
		       idempotency_key, created_at, updated_at
		  FROM payments
		 WHERE order_id = $1 AND tenant = $2
		 ORDER BY attempt DESC
		 LIMIT 1`, orderID, tenant,
	).Scan(
		&p.ID, &p.Tenant, &p.OrderID, &p.GatewayID, &p.Attempt, &p.Provider, &p.Method,
		&p.ProviderRef, &p.Status, &p.AmountCents, &p.RefundedCents, &p.ChargedBackCents,
		&p.IdempotencyKey, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNoExiste
	}
	return &p, err
}

// ObtenerPedidoPorID recupera un pedido y sus líneas (§10.4).
func (s *Store) ObtenerPedidoPorID(ctx context.Context, tenant string, siteID, orderID uuid.UUID) (*domain.Order, error) {
	var o *domain.Order
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		res, err := s.obtenerPedidoTx(ctx, tx, tenant, orderID)
		if err != nil {
			return err
		}
		if res.SiteID != siteID {
			return domain.ErrPedidoNoExiste
		}
		o = res
		return nil
	})
	return o, err
}

// ObtenerPedidoPorNumero recupera un pedido por su número (P-000123).
func (s *Store) ObtenerPedidoPorNumero(ctx context.Context, tenant string, siteID uuid.UUID, number string) (*domain.Order, error) {
	var orderID uuid.UUID
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id
			  FROM orders
			 WHERE site_id = $1 AND number = $2 AND tenant = $3`,
			siteID, number, tenant,
		).Scan(&orderID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrPedidoNoExiste
	}
	if err != nil {
		return nil, err
	}
	return s.ObtenerPedidoPorID(ctx, tenant, siteID, orderID)
}

// ObtenerPago recupera un intento de pago por su ID.
func (s *Store) ObtenerPago(ctx context.Context, tenant string, paymentID uuid.UUID) (*domain.Payment, error) {
	var p domain.Payment
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id, tenant, order_id, gateway_id, attempt, provider, method,
			       provider_ref, status, amount_cents, refunded_cents, charged_back_cents,
			       idempotency_key, created_at, updated_at
			  FROM payments
			 WHERE id = $1 AND tenant = $2`, paymentID, tenant,
		).Scan(
			&p.ID, &p.Tenant, &p.OrderID, &p.GatewayID, &p.Attempt, &p.Provider, &p.Method,
			&p.ProviderRef, &p.Status, &p.AmountCents, &p.RefundedCents, &p.ChargedBackCents,
			&p.IdempotencyKey, &p.CreatedAt, &p.UpdatedAt,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNoExiste
	}
	return &p, err
}

// ObtenerUltimoPagoPorOrden recupera el intento de pago más reciente de un pedido.
func (s *Store) ObtenerUltimoPagoPorOrden(ctx context.Context, tenant string, orderID uuid.UUID) (*domain.Payment, error) {
	var p *domain.Payment
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		res, err := s.obtenerUltimoPagoTx(ctx, tx, tenant, orderID)
		if err != nil {
			return err
		}
		p = res
		return nil
	})
	return p, err
}

// CrearSiguienteIntentoPago crea el siguiente intento (attempt <= 3) para un pedido.
func (s *Store) CrearSiguienteIntentoPago(ctx context.Context, tenant string, payment *domain.Payment) error {
	if payment.Attempt > 3 {
		return domain.ErrDemasiadosIntentos
	}
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		// El intento solo se crea si el pedido y la pasarela son de esta organización.
		tag, err := tx.Exec(ctx, `
			INSERT INTO payments (
				id, tenant, order_id, gateway_id, attempt, provider, method,
				provider_ref, status, amount_cents, refunded_cents, charged_back_cents,
				idempotency_key, created_at, updated_at
			)
			SELECT $1::uuid, $2::text, o.id, g.id, $5::smallint, $6::text, $7::text, $8::text, $9::text,
			       $10::integer, $11::integer, $12::integer, $13::text, now(), now()
			  FROM orders o
			  JOIN gateways g ON g.tenant = o.tenant AND g.site_id = o.site_id
			 WHERE o.id = $3::uuid AND o.tenant = $2::text AND g.id = $4::uuid`,
			payment.ID, tenant, payment.OrderID, payment.GatewayID, payment.Attempt, payment.Provider, payment.Method,
			payment.ProviderRef, payment.Status, payment.AmountCents, payment.RefundedCents, payment.ChargedBackCents,
			payment.IdempotencyKey,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNoExiste
		}
		return nil
	})
}

// ActualizarEstadoPago actualiza provider_ref y status en payments.
//
// Un pago cobrado no se mueve de aquí (domain.PagoCobrado): volverlo a `open` lo
// metía en el barrido, que lo encontraba pagado y lo declaraba duplicado, con su
// reembolso automático; y una notificación «fallido» tardía lo dejaba listo para lo
// mismo (WEB-03). Lo que pasa después de cobrar lo escriben MarcarPedidoPagado y
// los reembolsos.
func (s *Store) ActualizarEstadoPago(ctx context.Context, tenant string, paymentID uuid.UUID, providerRef, status string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE payments
			   SET provider_ref = CASE WHEN $2 <> '' THEN $2 ELSE provider_ref END,
			       status = $3,
			       updated_at = now()
			 WHERE id = $1 AND tenant = $4
			   AND status NOT IN ('paid', 'refunded', 'partially_refunded', 'charged_back')`,
			paymentID, providerRef, status, tenant,
		)
		return err
	})
}

// MarcarPedidoPagado ejecuta la transición a pagado (§10.5):
// - Transición monótona del pedido a paid.
// - reserved -> vendido en variants.
// - Si el pedido estaba vencido/cancelado: oversold = true y decremento de stock.
// - Actualiza el payment a paid.
func (s *Store) MarcarPedidoPagado(
	ctx context.Context,
	tenant string,
	orderID, paymentID uuid.UUID,
	providerRef string,
	amountCents int,
) (*domain.Order, bool, bool, error) {
	var o *domain.Order
	var oversold bool
	var duplicado bool

	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var currentStatus string
		var siteID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT status, site_id
			  FROM orders
			 WHERE id = $1 AND tenant = $2
			   FOR UPDATE`, orderID, tenant,
		).Scan(&currentStatus, &siteID)
		if err != nil {
			return err
		}

		// Un pedido que ya no espera pago (pagado, servido, reembolsado, en disputa) no
		// vuelve a «pagado». Un cobro más sobre él es DUPLICADO solo si OTRO pago del
		// pedido ya consta cobrado; si es este mismo, la notificación se repite y no se
		// hace nada otra vez (WEB-03: antes bastaba con que este pago no figurara
		// pagado, y reabrirlo lo convertía en «duplicado» con reembolso automático).
		if currentStatus != domain.OrderStatusPendingPayment && currentStatus != domain.OrderStatusExpired &&
			currentStatus != domain.OrderStatusCanceled {
			var payStatus string
			if err := tx.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1 AND order_id = $2 AND tenant = $3`,
				paymentID, orderID, tenant).Scan(&payStatus); err != nil {
				return err
			}
			if domain.PagoCobrado(payStatus) {
				return domain.ErrPagoYaTratado
			}
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM payments
				                WHERE order_id = $1 AND tenant = $2 AND id <> $3
				                  AND status IN ('paid', 'refunded', 'partially_refunded', 'charged_back'))`,
				orderID, tenant, paymentID).Scan(&duplicado); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				UPDATE payments
				   SET status = 'paid',
				       provider_ref = CASE WHEN $2 <> '' THEN $2 ELSE provider_ref END,
				       amount_cents = CASE WHEN $3 > 0 THEN $3 ELSE amount_cents END,
				       updated_at = now()
				 WHERE id = $1 AND tenant = $4`,
				paymentID, providerRef, amountCents, tenant,
			)
			if err != nil {
				return err
			}
			o, err = s.obtenerPedidoTx(ctx, tx, tenant, orderID)
			return err
		}

		oversold, err = ajustarStockTrasPagoTx(ctx, tx, tenant, siteID, orderID, currentStatus)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			UPDATE orders
			   SET status = 'paid',
			       oversold = $2,
			       paid_at = now(),
			       updated_at = now()
			 WHERE id = $1 AND tenant = $3`,
			orderID, oversold, tenant,
		)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			UPDATE payments
			   SET status = 'paid',
			       provider_ref = CASE WHEN $2 <> '' THEN $2 ELSE provider_ref END,
			       amount_cents = CASE WHEN $3 > 0 THEN $3 ELSE amount_cents END,
			       updated_at = now()
			 WHERE id = $1 AND tenant = $4`,
			paymentID, providerRef, amountCents, tenant,
		)
		if err != nil {
			return err
		}

		o, err = s.obtenerPedidoTx(ctx, tx, tenant, orderID)
		return err
	})
	if err != nil {
		return nil, false, false, err
	}
	return o, oversold, duplicado, nil
}

func ajustarStockTrasPagoTx(ctx context.Context, tx pgx.Tx, tenant string, siteID, orderID uuid.UUID, status string) (bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT sku, quantity
		  FROM order_lines
		 WHERE order_id = $1 AND tenant = $2`, orderID, tenant,
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	type skuQty struct {
		sku string
		qty int
	}
	var skus []skuQty
	for rows.Next() {
		var sq skuQty
		if err := rows.Scan(&sq.sku, &sq.qty); err != nil {
			return false, err
		}
		skus = append(skus, sq)
	}

	var oversold bool
	if status == "pending_payment" {
		for _, sq := range skus {
			_, err = tx.Exec(ctx, `
				UPDATE variants
				   SET reserved = GREATEST(reserved - $1, 0),
				       stock = CASE WHEN stock IS NOT NULL THEN GREATEST(stock - $1, 0) ELSE stock END
				 WHERE site_id = $2 AND sku = $3 AND tenant = $4`,
				sq.qty, siteID, sq.sku, tenant,
			)
			if err != nil {
				return false, err
			}
		}
		return false, nil
	}

	for _, sq := range skus {
		var stock *int
		_ = tx.QueryRow(ctx, `SELECT stock FROM variants WHERE site_id = $1 AND sku = $2 AND tenant = $3`, siteID, sq.sku, tenant).Scan(&stock)
		if stock != nil && *stock < sq.qty {
			oversold = true
		}
		_, err = tx.Exec(ctx, `
			UPDATE variants
			   SET stock = CASE WHEN stock IS NOT NULL THEN GREATEST(stock - $1, 0) ELSE stock END
			 WHERE site_id = $2 AND sku = $3 AND tenant = $4`,
			sq.qty, siteID, sq.sku, tenant,
		)
		if err != nil {
			return false, err
		}
	}
	return oversold, nil
}

// RegistrarEventoPago anota un evento recibido de webhook de forma idempotente.
// Devuelve true si fue insertado, false si ya existía.
func (s *Store) RegistrarEventoPago(
	ctx context.Context,
	tenant string,
	gatewayID uuid.UUID,
	provider, eventID, status string,
	paymentID *uuid.UUID,
) (bool, error) {
	var insertado bool
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO payment_events (tenant, gateway_id, provider, event_id, payment_id, status, received_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (gateway_id, event_id) DO NOTHING`,
			tenant, gatewayID, provider, eventID, paymentID, status,
		)
		if err != nil {
			return err
		}
		insertado = tag.RowsAffected() > 0
		return nil
	})
	return insertado, err
}

// LiberarReservaPedidoVencido pasa pending_payment a expired y libera reserved en variants (§10.5).
func (s *Store) LiberarReservaPedidoVencido(ctx context.Context, tenant string, orderID uuid.UUID) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var siteID uuid.UUID
		tag, err := tx.Exec(ctx, `
			UPDATE orders
			   SET status = 'expired', updated_at = now()
			 WHERE id = $1 AND tenant = $2 AND status = 'pending_payment'`,
			orderID, tenant,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}

		_ = tx.QueryRow(ctx, `SELECT site_id FROM orders WHERE id = $1 AND tenant = $2`, orderID, tenant).Scan(&siteID)

		// Liberar variantes
		rows, err := tx.Query(ctx, `SELECT sku, quantity FROM order_lines WHERE order_id = $1 AND tenant = $2`, orderID, tenant)
		if err != nil {
			return err
		}
		type item struct {
			sku string
			qty int
		}
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.sku, &it.qty); err == nil {
				items = append(items, it)
			}
		}
		rows.Close()

		for _, it := range items {
			_, _ = tx.Exec(ctx, `
				UPDATE variants
				   SET reserved = GREATEST(reserved - $1, 0)
				 WHERE site_id = $2 AND sku = $3 AND tenant = $4`,
				it.qty, siteID, it.sku, tenant,
			)
		}
		return nil
	})
}

// PedidoVencido representa un pedido cuya reserva venció.
type PedidoVencido struct {
	Tenant  string
	SiteID  uuid.UUID
	OrderID uuid.UUID
}

// ObtenerPedidosVencidos busca pedidos pending_payment con reserved_until < now().
func (s *Store) ObtenerPedidosVencidos(ctx context.Context) ([]PedidoVencido, error) {
	var list []PedidoVencido
	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT tenant, site_id, id
			  FROM orders
			 WHERE status = 'pending_payment' AND reserved_until < now()
			 LIMIT 50`,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var pv PedidoVencido
			if err := rows.Scan(&pv.Tenant, &pv.SiteID, &pv.OrderID); err != nil {
				return err
			}
			list = append(list, pv)
		}
		return nil
	})
	return list, err
}

// PagoPendienteBarrido representa un pago en curso que consultar a la pasarela.
type PagoPendienteBarrido struct {
	Tenant      string
	SiteID      uuid.UUID
	OrderID     uuid.UUID
	PaymentID   uuid.UUID
	GatewayID   uuid.UUID
	Provider    string
	ProviderRef string
	AmountCents int
	CreatedAt   time.Time
}

// ObtenerPagosPendientesBarrido devuelve pagos created, open o pending de entre 10 min y 48 h (§10.5).
func (s *Store) ObtenerPagosPendientesBarrido(ctx context.Context) ([]PagoPendienteBarrido, error) {
	var list []PagoPendienteBarrido
	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.tenant, o.site_id, p.order_id, p.id, p.gateway_id, p.provider, p.provider_ref, p.amount_cents, p.created_at
			  FROM payments p
			  JOIN orders o ON o.id = p.order_id AND o.tenant = p.tenant
			 WHERE p.status IN ('created', 'open', 'pending')
			   AND p.created_at BETWEEN now() - interval '48 hours' AND now() - interval '10 minutes'
			 LIMIT 50`,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var pb PagoPendienteBarrido
			if err := rows.Scan(
				&pb.Tenant, &pb.SiteID, &pb.OrderID, &pb.PaymentID, &pb.GatewayID,
				&pb.Provider, &pb.ProviderRef, &pb.AmountCents, &pb.CreatedAt,
			); err != nil {
				return err
			}
			list = append(list, pb)
		}
		return nil
	})
	return list, err
}

// ResolverPagoPorID busca un pago y su pasarela por ID (usado por webhooks como Mollie).
func (s *Store) ResolverPagoPorID(ctx context.Context, paymentID uuid.UUID) (*domain.Payment, *domain.Gateway, string, error) {
	var p domain.Payment
	var g domain.Gateway
	var tenant string
	var pubRaw []byte

	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT p.id, p.tenant, p.order_id, p.gateway_id, p.attempt, p.provider, p.method,
			       p.provider_ref, p.status, p.amount_cents, p.refunded_cents, p.charged_back_cents,
			       p.idempotency_key, p.created_at, p.updated_at,
			       g.id, g.site_id, g.provider, g.mode, g.credentials_enc, g.public_config, g.active
			  FROM payments p
			  JOIN gateways g ON g.id = p.gateway_id AND g.tenant = p.tenant
			 WHERE p.id = $1`, paymentID,
		).Scan(
			&p.ID, &p.Tenant, &p.OrderID, &p.GatewayID, &p.Attempt, &p.Provider, &p.Method,
			&p.ProviderRef, &p.Status, &p.AmountCents, &p.RefundedCents, &p.ChargedBackCents,
			&p.IdempotencyKey, &p.CreatedAt, &p.UpdatedAt,
			&g.ID, &g.SiteID, &g.Provider, &g.Mode, &g.CredentialsEnc, &pubRaw, &g.Active,
		)
		if err != nil {
			return err
		}
		g.Tenant = p.Tenant
		tenant = p.Tenant
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, "", domain.ErrNoExiste
	}
	if err != nil {
		return nil, nil, "", err
	}
	if len(pubRaw) > 0 {
		_ = json.Unmarshal(pubRaw, &g.PublicConfig)
	}
	return &p, &g, tenant, nil
}

// ResolverPagoPorReferencia busca un pago por la referencia del proveedor (provider_ref).
func (s *Store) ResolverPagoPorReferencia(ctx context.Context, provider, providerRef string) (*domain.Payment, *domain.Gateway, string, error) {
	var p domain.Payment
	var g domain.Gateway
	var tenant string
	var pubRaw []byte

	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT p.id, p.tenant, p.order_id, p.gateway_id, p.attempt, p.provider, p.method,
			       p.provider_ref, p.status, p.amount_cents, p.refunded_cents, p.charged_back_cents,
			       p.idempotency_key, p.created_at, p.updated_at,
			       g.id, g.site_id, g.provider, g.mode, g.credentials_enc, g.public_config, g.active
			  FROM payments p
			  JOIN gateways g ON g.id = p.gateway_id AND g.tenant = p.tenant
			 WHERE p.provider = $1 AND p.provider_ref = $2`, provider, providerRef,
		).Scan(
			&p.ID, &p.Tenant, &p.OrderID, &p.GatewayID, &p.Attempt, &p.Provider, &p.Method,
			&p.ProviderRef, &p.Status, &p.AmountCents, &p.RefundedCents, &p.ChargedBackCents,
			&p.IdempotencyKey, &p.CreatedAt, &p.UpdatedAt,
			&g.ID, &g.SiteID, &g.Provider, &g.Mode, &g.CredentialsEnc, &pubRaw, &g.Active,
		)
		if err != nil {
			return err
		}
		g.Tenant = p.Tenant
		tenant = p.Tenant
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, "", domain.ErrNoExiste
	}
	if err != nil {
		return nil, nil, "", err
	}
	if len(pubRaw) > 0 {
		_ = json.Unmarshal(pubRaw, &g.PublicConfig)
	}
	return &p, &g, tenant, nil
}

// CrearReembolso registra un reembolso y actualiza el pago en UNA transacción (§10.5).
func (s *Store) CrearReembolso(ctx context.Context, tenant string, r *domain.Refund) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		// Validar que el pago existe y tiene saldo para reembolsar con SELECT FOR UPDATE
		var amountCents, refundedCents int
		var status string
		err := tx.QueryRow(ctx, `
			SELECT amount_cents, refunded_cents, status
			  FROM payments
			 WHERE id = $1 AND tenant = $2
			   FOR UPDATE`, r.PaymentID, tenant,
		).Scan(&amountCents, &refundedCents, &status)
		if err != nil {
			return err
		}
		if status != "paid" && status != "partially_refunded" {
			return domain.ErrReembolsoInvalido
		}

		// Sumar reembolsos pendientes
		var pendingCents int
		_ = tx.QueryRow(ctx, `
			SELECT coalesce(sum(amount_cents), 0)
			  FROM refunds
			 WHERE payment_id = $1 AND tenant = $2 AND status = 'pending'`,
			r.PaymentID, tenant,
		).Scan(&pendingCents)

		if r.AmountCents > (amountCents - refundedCents - pendingCents) {
			return domain.ErrReembolsoInvalido
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO refunds (id, tenant, payment_id, amount_cents, idempotency_key, status, created_by, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
			r.ID, tenant, r.PaymentID, r.AmountCents, r.IdempotencyKey, r.Status, r.CreatedBy,
		)
		return err
	})
}

// ReservarReembolsoDuplicado deja el reembolso de un cobro duplicado listo para pedirlo
// a la pasarela, y dice si toca pedirlo: uno que falló vuelve a «pending» (solo una
// llamada lo consigue) y, con `nuevo`, si no lo había, se crea. Uno «pending» o «done»
// no se toca: nunca dos devoluciones del mismo cobro.
func (s *Store) ReservarReembolsoDuplicado(ctx context.Context, tenant string, paymentID uuid.UUID, amountCents int, clave string, nuevo bool) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			UPDATE refunds SET status = 'pending'
			 WHERE idempotency_key = $1 AND tenant = $2 AND payment_id = $3 AND status = 'failed'
			 RETURNING id`, clave, tenant, paymentID).Scan(&id)
		if !errors.Is(err, pgx.ErrNoRows) || !nuevo {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO refunds (tenant, payment_id, amount_cents, idempotency_key, status, created_by)
			SELECT $1, p.id, $3, $4, 'pending', 'system'
			  FROM payments p
			 WHERE p.id = $2 AND p.tenant = $1 AND p.status = 'paid'
			ON CONFLICT (idempotency_key) DO NOTHING
			RETURNING id`, tenant, paymentID, amountCents, clave).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	return id, err == nil, err
}

// ActualizarReembolso actualiza el estado del reembolso y acumula en payments.
func (s *Store) ActualizarReembolso(ctx context.Context, tenant string, refundID uuid.UUID, status string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		var paymentID uuid.UUID
		var amountCents int
		err := tx.QueryRow(ctx, `
			UPDATE refunds
			   SET status = $2, done_at = CASE WHEN $2 = 'done' THEN now() ELSE done_at END
			 WHERE id = $1 AND tenant = $3
			 RETURNING payment_id, amount_cents`,
			refundID, status, tenant,
		).Scan(&paymentID, &amountCents)
		if err != nil {
			return err
		}

		if status == "done" {
			var totalRefunded, totalAmount int
			var orderID uuid.UUID
			err = tx.QueryRow(ctx, `
				UPDATE payments
				   SET refunded_cents = refunded_cents + $2,
				       status = CASE WHEN refunded_cents + $2 >= amount_cents THEN 'refunded' ELSE 'partially_refunded' END,
				       updated_at = now()
				 WHERE id = $1 AND tenant = $3
				 RETURNING refunded_cents, amount_cents, order_id`,
				paymentID, amountCents, tenant,
			).Scan(&totalRefunded, &totalAmount, &orderID)
			if err != nil {
				return err
			}

			// Actualizar orden, salvo que otro pago suyo siga cobrado: devolver un cobro
			// duplicado no reembolsa el pedido, que sigue pagado con el otro.
			orderStatus := "partially_refunded"
			if totalRefunded >= totalAmount {
				orderStatus = "refunded"
			}
			_, _ = tx.Exec(ctx, `
				UPDATE orders
				   SET status = $2, updated_at = now()
				 WHERE id = $1 AND tenant = $3
				   AND NOT EXISTS (SELECT 1 FROM payments
				                    WHERE order_id = $1 AND tenant = $3 AND id <> $4
				                      AND status IN ('paid', 'partially_refunded'))`,
				orderID, orderStatus, tenant, paymentID,
			)
		}
		return nil
	})
}

// RegistrarDesistimiento anota una solicitud de desistimiento legal (§10.7).
func (s *Store) RegistrarDesistimiento(ctx context.Context, tenant string, w *domain.Withdrawal) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		// Solo sobre un pedido de esta organización: con un id ajeno no se inserta nada.
		_, err := tx.Exec(ctx, `
			INSERT INTO withdrawals (id, tenant, order_id, email, name, source, reason, created_at)
			SELECT $1::uuid, $2::text, o.id, $4::text, $5::text, $6::text, $7::text, now()
			  FROM orders o
			 WHERE o.id = $3::uuid AND o.tenant = $2::text`,
			w.ID, tenant, w.OrderID, w.Email, w.Name, w.Source, w.Reason,
		)
		return err
	})
}

// IRPublicado es el IR de la versión EN VIVO de un sitio, o ErrNoExiste si no está
// publicado: lo que acepta un formulario es lo publicado (§10.6, WEB-07).
func (s *Store) IRPublicado(ctx context.Context, tenant string, siteID uuid.UUID) ([]byte, error) {
	var raw []byte
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		var err error
		raw, err = irPublicado(ctx, tx, tenant, siteID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNoExiste
	}
	return raw, err
}

func irPublicado(ctx context.Context, tx pgx.Tx, tenant string, siteID uuid.UUID) ([]byte, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT v.ir
		  FROM sites s
		  JOIN publications p ON p.site_id = s.id AND p.tenant = s.tenant AND p.kind = 'live' AND p.number = s.published_number
		  JOIN site_versions v ON v.id = p.version_id AND v.tenant = s.tenant
		 WHERE s.id = $1 AND s.tenant = $2 AND s.deleted_at IS NULL`, siteID, tenant).Scan(&raw)
	return raw, err
}

// AvisosDeFormularioHoy cuenta los avisos de formulario de un sitio en las últimas 24 h:
// pasado el tope, el envío se guarda pero no avisa (§10.6, WEB-07).
func (s *Store) AvisosDeFormularioHoy(ctx context.Context, tenant string, siteID uuid.UUID) (int, error) {
	var n int
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM events_outbox
			 WHERE tenant = $1 AND type = 'pages.form.submitted' AND payload->>'site_id' = $2
			   AND created_at > now() - interval '1 day'`, tenant, siteID.String()).Scan(&n)
	})
	return n, err
}

// GuardarFormulario guarda el envío de un formulario validado (§10.6).
func (s *Store) GuardarFormulario(ctx context.Context, tenant string, sub *domain.FormSubmission) error {
	dataJSON, err := json.Marshal(sub.Data)
	if err != nil {
		return err
	}
	return s.enSitio(ctx, tenant, sub.SiteID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO form_submissions (
				id, tenant, site_id, form_id, data, language, consent_text, consent_at, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
			sub.ID, tenant, sub.SiteID, sub.FormID, dataJSON, sub.Language, sub.ConsentText, sub.ConsentAt,
		)
		return err
	})
}

// ObtenerDefinicionFormulario lee los ajustes de un formulario para un sitio. Un
// formulario sin fila es uno publicado que nadie ha configurado: encendido y sin
// nadie a quien avisar. Nada crea la fila al publicar (ni al volver a una
// publicación anterior), así que exigirla dejaba todos los formularios en 404.
// Que el formulario exista lo dice la versión publicada, no esta tabla.
func (s *Store) ObtenerDefinicionFormulario(ctx context.Context, tenant string, siteID uuid.UUID, formID string) (*domain.FormSettings, error) {
	var fs domain.FormSettings
	var notifyRaw []byte
	err := s.enSitio(ctx, tenant, siteID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT site_id, tenant, form_id, enabled, notify
			  FROM form_settings
			 WHERE site_id = $1 AND form_id = $2 AND tenant = $3`,
			siteID, formID, tenant,
		).Scan(&fs.SiteID, &fs.Tenant, &fs.FormID, &fs.Enabled, &notifyRaw)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.FormSettings{SiteID: siteID, Tenant: tenant, FormID: formID, Enabled: true}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(notifyRaw) > 0 {
		_ = json.Unmarshal(notifyRaw, &fs.Notify)
	}
	return &fs, nil
}

// Retención del outbox de Sytue Web. Un aviso despachado se guarda una semana, para
// poder mirar qué salió; uno que nadie despachó, 30 días, y después ya no sirve (un
// «pedido cobrado» de hace un mes no se manda). El tope diario de avisos de formulario
// cuenta las últimas 24 h del outbox: las dos retenciones lo cubren.
const (
	retencionOutboxProcesado = 7 * 24 * time.Hour
	retencionOutbox          = 30 * 24 * time.Hour
)

// PurgarOutbox aplica la retención del outbox, de todas las organizaciones a la vez.
func (s *Store) PurgarOutbox(ctx context.Context, ahora time.Time) (int64, error) {
	var n int64
	err := s.enMantenimiento(ctx, func(tx pgx.Tx) error {
		var err error
		n, err = events.Purge(ctx, tx, ahora.Add(-retencionOutboxProcesado), ahora.Add(-retencionOutbox))
		return err
	})
	return n, err
}

// EscribirEventoOutbox publica un evento en events_outbox de forma transaccional (§10.5).
func (s *Store) EscribirEventoOutbox(ctx context.Context, tenant string, ev events.Event) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		return events.Publish(ctx, tx, ev)
	})
}

// ObtenerSitioPublico obtiene los datos del sitio por tenant y siteID para peticiones públicas (§9.5).
func (s *Store) ObtenerSitioPublico(ctx context.Context, tenant string, siteID uuid.UUID) (*domain.SitioCompleto, error) {
	if s.pool == nil {
		return nil, domain.ErrNoExiste
	}
	var sc domain.SitioCompleto
	var rawLegal []byte
	err := s.enPublico(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant, slug, name, template, status, suspended_reason, draft, draft_rev,
			       published_number, canonical_host, legal, analytics_enabled, analytics_site_id,
			       export_origins, export_app_schemes, created_at, updated_at, deleted_at
			  FROM sites
			 WHERE id = $1 AND tenant = $2 AND deleted_at IS NULL`,
			siteID, tenant,
		)
		return row.Scan(
			&sc.ID, &sc.Tenant, &sc.Slug, &sc.Name, &sc.Template, &sc.Status, &sc.SuspendedReason,
			&sc.Draft, &sc.DraftRev, &sc.PublishedNumber, &sc.CanonicalHost, &rawLegal,
			&sc.AnalyticsEnabled, &sc.AnalyticsSiteID, &sc.ExportOrigins, &sc.ExportAppSchemes,
			&sc.CreatedAt, &sc.UpdatedAt, &sc.DeletedAt,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNoExiste
	}
	if err != nil {
		return nil, err
	}
	if len(rawLegal) > 0 {
		_ = json.Unmarshal(rawLegal, &sc.Legal)
	}
	return &sc, nil
}

// MarcarCorreoComprador apunta que el correo de confirmación del pedido ya salió: una
// segunda entrega del mismo aviso (el despachador es at-least-once) no lo repite.
func (s *Store) MarcarCorreoComprador(ctx context.Context, tenant string, orderID uuid.UUID) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE orders SET buyer_email_sent_at = now() WHERE id = $1 AND tenant = $2 AND buyer_email_sent_at IS NULL`, orderID, tenant)
		return err
	})
}

// ObtenerDesistimiento lee una solicitud de desistimiento, con el número de su pedido y
// el idioma en que se compró, que es en el que va el acuse.
func (s *Store) ObtenerDesistimiento(ctx context.Context, tenant string, id uuid.UUID) (*domain.Withdrawal, string, error) {
	w := domain.Withdrawal{ID: id, Tenant: tenant}
	var idioma string
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT w.order_id, o.site_id, o.number, w.email, w.ack_sent_at, o.language
			  FROM withdrawals w
			  JOIN orders o ON o.id = w.order_id AND o.tenant = w.tenant
			 WHERE w.id = $1 AND w.tenant = $2`, id, tenant,
		).Scan(&w.OrderID, &w.SiteID, &w.OrderNumber, &w.Email, &w.AckSentAt, &idioma)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", domain.ErrPedidoNoExiste
	}
	return &w, idioma, err
}

// MarcarAcuseDesistimiento apunta que el acuse de recibo del desistimiento ya salió.
func (s *Store) MarcarAcuseDesistimiento(ctx context.Context, tenant string, id uuid.UUID) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE withdrawals SET ack_sent_at = now() WHERE id = $1 AND tenant = $2 AND ack_sent_at IS NULL`, id, tenant)
		return err
	})
}

// IdiomaDeEnvio es el idioma en que se rellenó un formulario: el del aviso a la tienda.
// Vacío si no se encuentra (los textos caen al inglés).
func (s *Store) IdiomaDeEnvio(ctx context.Context, tenant string, id uuid.UUID) string {
	var idioma string
	_ = s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT language FROM form_submissions WHERE id = $1 AND tenant = $2`, id, tenant).Scan(&idioma)
	})
	return idioma
}

// ActualizarERP actualiza el estado y las referencias del pedido en SyERP.
func (s *Store) ActualizarERP(ctx context.Context, tenant string, orderID uuid.UUID, status, orderRef, invoiceRef string) error {
	return s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE orders
			   SET erp_status = $1,
			       erp_order_ref = CASE WHEN $2 <> '' THEN $2 ELSE erp_order_ref END,
			       erp_invoice_ref = CASE WHEN $3 <> '' THEN $3 ELSE erp_invoice_ref END,
			       updated_at = now()
			 WHERE id = $4 AND tenant = $5`,
			status, orderRef, invoiceRef, orderID, tenant,
		)
		return err
	})
}
