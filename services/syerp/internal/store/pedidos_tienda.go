// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// ErrModulosDesactivados indica que ventas o facturación están apagados en la org.
var ErrModulosDesactivados = errors.New("modules_disabled")

// PedidoTiendaInput modela la entrada validada de Pages para SyERP.
type PedidoTiendaInput struct {
	Tenant    string
	Origen    string
	Modo      string
	PedidoID  string
	Numero    string
	Fecha     string
	Divisa    string
	Factura   string
	Comprador struct {
		Nombre    string
		Correo    string
		NIF       string
		Direccion struct {
			Pais string
			CP   string
		}
	}
	Lineas []struct {
		SKU                    string
		Nombre                 string
		Cantidad               int64
		PrecioUnitarioCentimos int64
		TipoIVA                int
	}
	Envio *struct {
		Nombre         string
		PrecioCentimos int64
		TipoIVA        int
	}
	TotalCentimos int64
	Cobro         struct {
		Pasarela        string
		Referencia      string
		Fecha           string
		ImporteCentimos int64
	}
}

// ResultadoPedidoTienda es la respuesta devuelta a Pages.
type ResultadoPedidoTienda struct {
	Estado     string `json:"estado"`
	PedidoRef  string `json:"pedido_ref"`
	FacturaRef string `json:"factura_ref"`
	Verifactu  string `json:"verifactu"`
}

// ProcesarPedidoTienda ejecuta en UNA transacción la idempotencia, contacto,
// pedido_venta, factura_venta, líneas de factura, asiento contable y VeriFactu.
func (s *Store) ProcesarPedidoTienda(ctx context.Context, p PedidoTiendaInput, sub string) (ResultadoPedidoTienda, error) {
	activos, err := s.ModulosActivos(ctx)
	if err != nil {
		return ResultadoPedidoTienda{}, err
	}
	if !slices.Contains(activos, "ventas") || !slices.Contains(activos, "facturacion") {
		return ResultadoPedidoTienda{}, ErrModulosDesactivados
	}

	var res ResultadoPedidoTienda
	err = s.escritura(ctx, func(tx pgx.Tx) error {
		// 1. Idempotencia: ¿ya existe el pedido de Pages?
		existente, ok, err := comprobarIdempotenciaPedido(ctx, tx, p.PedidoID)
		if err != nil {
			return err
		}
		if ok {
			res = existente
			return nil
		}

		// 2. Contacto: F2 genérico por sitio o F1 por NIF.
		clienteID, err := s.asegurarClienteTienda(ctx, tx, p, sub)
		if err != nil {
			return err
		}

		// 3. Crear pedido_venta (externo_id = pages:<pedido_id>, confirmado) + linea_venta.
		pedidoID, pvRef, err := crearPedidoVentaTienda(ctx, tx, p, clienteID, sub)
		if err != nil {
			return err
		}

		// 4. Crear factura_venta (F1/F2, SIN externo_id, pagado) + linea_factura.
		facturaID, fvRef, baseTotal, ivaTotal, err := crearFacturaVentaTienda(ctx, tx, p, clienteID, pedidoID, sub)
		if err != nil {
			return err
		}

		// 5. Emitir VeriFactu (en la misma transacción).
		// La factura nace en borrador y pasa a pagada DESPUÉS de emitirse: nacida
		// pagada, `registroQueToca` la daba por emitida y quedaba fuera de la
		// cadena (NEG-14).
		if err := s.emitirVerifactu(ctx, tx, "factura_venta", []string{facturaID}, "pagado", sub); err != nil {
			return fmt.Errorf("emitir verifactu: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE records SET estado = 'pagado', updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, facturaID); err != nil {
			return err
		}

		// 6. Asiento contable de factura y cobro (banco 572000, cliente 430000).
		if err := crearAsientosTienda(ctx, tx, p, clienteID, facturaID, fvRef, baseTotal, ivaTotal, sub); err != nil {
			return err
		}

		// 7. Eventos entrantes para idempotencia.
		if _, err := tx.Exec(ctx, `
			INSERT INTO eventos_entrantes (tenant, proveedor, evento_id, tipo)
			VALUES (current_setting('app.tenant', true), 'pages', $1, 'order.paid')
			ON CONFLICT DO NOTHING`, p.PedidoID); err != nil {
			return fmt.Errorf("marcar evento_entrante: %w", err)
		}

		_, vfActivo, _ := orgVerifactu(ctx, tx)
		vfEstado := "no_activo"
		if vfActivo {
			vfEstado = "pendiente"
		}

		res = ResultadoPedidoTienda{
			Estado:     "creado",
			PedidoRef:  pvRef,
			FacturaRef: fvRef,
			Verifactu:  vfEstado,
		}
		return nil
	})
	return res, err
}

func comprobarIdempotenciaPedido(ctx context.Context, tx pgx.Tx, pedidoIDStr string) (ResultadoPedidoTienda, bool, error) {
	externoID := "pages:" + pedidoIDStr
	var pedidoID, pedidoRef string
	err := tx.QueryRow(ctx, `
		SELECT id::text, ref FROM records
		 WHERE tenant = current_setting('app.tenant', true)
		   AND model = 'pedido_venta'
		   AND data @> jsonb_build_object('externo_id', $1::text)
		   AND deleted_at IS NULL LIMIT 1`, externoID).Scan(&pedidoID, &pedidoRef)
	if err == nil {
		var facturaRef string
		_ = tx.QueryRow(ctx, `
			SELECT ref FROM records
			 WHERE tenant = current_setting('app.tenant', true)
			   AND model = 'factura_venta'
			   AND data @> jsonb_build_object('pedido', $1::text)
			   AND deleted_at IS NULL LIMIT 1`, pedidoID).Scan(&facturaRef)
		_, vfActivo, _ := orgVerifactu(ctx, tx)
		vfEstado := "no_activo"
		if vfActivo {
			vfEstado = "pendiente"
		}
		return ResultadoPedidoTienda{
			Estado:     "ya_existia",
			PedidoRef:  pedidoRef,
			FacturaRef: facturaRef,
			Verifactu:  vfEstado,
		}, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ResultadoPedidoTienda{}, false, err
	}
	return ResultadoPedidoTienda{}, false, nil
}

func (s *Store) asegurarClienteTienda(ctx context.Context, tx pgx.Tx, p PedidoTiendaInput, sub string) (string, error) {
	if p.Factura == "simplificada" {
		etiqueta := "Ventas en línea — " + p.Origen
		var clienteID string
		err := tx.QueryRow(ctx, `
			SELECT id::text FROM records
			 WHERE tenant = current_setting('app.tenant', true)
			   AND model = 'contacto'
			   AND lower(etiqueta) = lower($1)
			   AND deleted_at IS NULL LIMIT 1`, etiqueta).Scan(&clienteID)
		if errors.Is(err, pgx.ErrNoRows) {
			crudo, _ := json.Marshal(map[string]any{
				"nombre": etiqueta, "tipo": "cliente", "cliente_generico": true,
			})
			if err := tx.QueryRow(ctx, `
				INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
				VALUES (current_setting('app.tenant', true), 'contacto', '', $1, 'activo', $2, $3)
				RETURNING id::text`, etiqueta, crudo, sub).Scan(&clienteID); err != nil {
				return "", fmt.Errorf("crear contacto generico: %w", err)
			}
		} else if err != nil {
			return "", err
		}
		return clienteID, nil
	}

	// F1 completa: buscar por NIF o crear.
	id, err := s.BuscarPorNIF(ctx, "contacto", p.Comprador.NIF)
	if err == nil {
		return id, nil
	}
	// La completa solo se admite con un NIF español (validarPedidoTienda): el
	// cliente es de España a efectos fiscales aunque el pedido vaya a otro país.
	// Con el país de la dirección, VERI*FACTU lo mandaba como «PT» + NIF.
	cp := p.Comprador.Direccion.CP
	if p.Comprador.Direccion.Pais != "ES" {
		cp = ""
	}
	crudo, _ := json.Marshal(map[string]any{
		"nombre": p.Comprador.Nombre, "nif": p.Comprador.NIF,
		"email": p.Comprador.Correo, "pais": "ES",
		"cp": cp, "tipo": "cliente",
	})
	var clienteID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
		VALUES (current_setting('app.tenant', true), 'contacto', '', $1, 'activo', $2, $3)
		RETURNING id::text`, p.Comprador.Nombre, crudo, sub).Scan(&clienteID); err != nil {
		return "", fmt.Errorf("crear contacto F1: %w", err)
	}
	return clienteID, nil
}

func crearPedidoVentaTienda(ctx context.Context, tx pgx.Tx, p PedidoTiendaInput, clienteID, sub string) (string, string, error) {
	anho := time.Now().Format("2006")
	pvRef, err := reservarRef(ctx, tx, "doc:pedido_venta", "PV", anho)
	if err != nil {
		return "", "", fmt.Errorf("reservar ref pedido: %w", err)
	}
	externoID := "pages:" + p.PedidoID
	dataPedido, _ := json.Marshal(map[string]any{
		"cliente": clienteID, "fecha": p.Fecha, "total": p.TotalCentimos,
		"divisa": p.Divisa, "externo_id": externoID, "numero": p.Numero,
		"origen": p.Origen,
	})
	var pedidoID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
		VALUES (current_setting('app.tenant', true), 'pedido_venta', $1, $2, 'confirmado', $3, $4)
		RETURNING id::text`, pvRef, p.Numero, dataPedido, sub).Scan(&pedidoID); err != nil {
		return "", "", fmt.Errorf("insertar pedido_venta: %w", err)
	}

	for _, l := range p.Lineas {
		totalLinea := l.Cantidad * l.PrecioUnitarioCentimos
		dataLV, _ := json.Marshal(map[string]any{
			"pedido": pedidoID, "nombre": l.Nombre, "sku": l.SKU,
			"cantidad": fmt.Sprintf("%d", l.Cantidad), "precio": l.PrecioUnitarioCentimos,
			"total": totalLinea,
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
			VALUES (current_setting('app.tenant', true), 'linea_venta', '', $1, 'confirmado', $2, $3)`,
			l.Nombre, dataLV, sub); err != nil {
			return "", "", fmt.Errorf("insertar linea_venta: %w", err)
		}
	}
	if p.Envio != nil {
		dataEnvio, _ := json.Marshal(map[string]any{
			"pedido": pedidoID, "nombre": p.Envio.Nombre,
			"cantidad": "1", "precio": p.Envio.PrecioCentimos,
			"total": p.Envio.PrecioCentimos,
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
			VALUES (current_setting('app.tenant', true), 'linea_venta', '', $1, 'confirmado', $2, $3)`,
			p.Envio.Nombre, dataEnvio, sub); err != nil {
			return "", "", fmt.Errorf("insertar linea_venta envio: %w", err)
		}
	}
	return pedidoID, pvRef, nil
}

func crearFacturaVentaTienda(ctx context.Context, tx pgx.Tx, p PedidoTiendaInput, clienteID, pedidoID, sub string) (string, string, int64, int64, error) {
	anho := time.Now().Format("2006")
	fvRef, err := reservarRef(ctx, tx, "doc:factura_venta", "FV", anho)
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("reservar ref factura: %w", err)
	}

	var lfDominio []domain.LineaFactura
	for _, l := range p.Lineas {
		lfDominio = append(lfDominio, domain.LineaFactura{
			Descripcion: l.Nombre,
			Cantidad:    fmt.Sprintf("%d", l.Cantidad),
			Precio:      l.PrecioUnitarioCentimos,
			Tipo:        int64(l.TipoIVA),
			Importe:     l.Cantidad * l.PrecioUnitarioCentimos,
		})
	}
	if p.Envio != nil {
		lfDominio = append(lfDominio, domain.LineaFactura{
			Descripcion: p.Envio.Nombre,
			Cantidad:    "1",
			Precio:      p.Envio.PrecioCentimos,
			Tipo:        int64(p.Envio.TipoIVA),
			Importe:     p.Envio.PrecioCentimos,
		})
	}
	grupos := domain.Desglose(lfDominio)
	var baseTotal, ivaTotal int64
	for _, g := range grupos {
		baseTotal += g.Base
		ivaTotal += g.Cuota
	}

	tipoFactura := "F1"
	if p.Factura == "simplificada" {
		tipoFactura = "F2"
	}

	dataFactura, _ := json.Marshal(map[string]any{
		"cliente": clienteID, "pedido": pedidoID, "fecha": p.Fecha,
		"base": baseTotal, "iva": ivaTotal, "total": p.TotalCentimos,
		"tipo_factura": tipoFactura, "regimen_iva": "iva_general",
	})
	var facturaID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
		VALUES (current_setting('app.tenant', true), 'factura_venta', $1, $2, 'borrador', $3, $4)
		RETURNING id::text`, fvRef, fvRef, dataFactura, sub).Scan(&facturaID); err != nil {
		return "", "", 0, 0, fmt.Errorf("insertar factura_venta: %w", err)
	}

	for _, l := range lfDominio {
		dataLF, _ := json.Marshal(map[string]any{
			"factura": facturaID, "descripcion": l.Descripcion,
			"cantidad": l.Cantidad, "precio_con_iva": l.Precio,
			"tipo_iva": fmt.Sprintf("%d", l.Tipo),
		})
		if _, err := tx.Exec(ctx, `
			INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
			VALUES (current_setting('app.tenant', true), 'linea_factura', '', $1, 'pagado', $2, $3)`,
			l.Descripcion, dataLF, sub); err != nil {
			return "", "", 0, 0, fmt.Errorf("insertar linea_factura: %w", err)
		}
	}
	return facturaID, fvRef, baseTotal, ivaTotal, nil
}

func crearAsientosTienda(ctx context.Context, tx pgx.Tx, p PedidoTiendaInput, clienteID, facturaID, fvRef string, baseTotal, ivaTotal int64, sub string) error {
	fechaDoc, _ := time.Parse("2006-01-02", p.Fecha)
	if fechaDoc.IsZero() {
		fechaDoc = time.Now()
	}
	fechaCobro, _ := time.Parse(time.RFC3339, p.Cobro.Fecha)
	if fechaCobro.IsZero() {
		fechaCobro = fechaDoc
	}

	// Asiento de ventas.
	if _, err := escribirAsiento(ctx, tx, "ventas", fechaDoc, "Factura "+fvRef, facturaID, sub, []Apunte{
		{Cuenta: "430000", Concepto: "Factura " + fvRef, Debe: p.TotalCentimos, Tercero: clienteID},
		{Cuenta: "700000", Concepto: "Ventas tienda", Haber: baseTotal},
		{Cuenta: "477000", Concepto: "IVA repercutido", Haber: ivaTotal},
	}); err != nil {
		return fmt.Errorf("asiento factura: %w", err)
	}

	// Asiento de cobro.
	if _, err := escribirAsiento(ctx, tx, "bancos", fechaCobro, "Cobro pedido "+p.Numero, "", sub, []Apunte{
		{Cuenta: "572000", Concepto: "Cobro " + p.Cobro.Pasarela, Debe: p.TotalCentimos},
		{Cuenta: "430000", Concepto: "Cobro pedido " + p.Numero, Haber: p.TotalCentimos, Tercero: clienteID},
	}); err != nil {
		return fmt.Errorf("asiento cobro: %w", err)
	}
	return nil
}

// FacturaPorPedidoTienda localiza la factura y líneas asociadas a un pedido_id de Pages.
func (s *Store) FacturaPorPedidoTienda(ctx context.Context, pedidoID string) (Registro, []domain.LineaFactura, bool, error) {
	externoID := "pages:" + pedidoID
	var f Registro
	var facturaID string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var pvID string
		err := tx.QueryRow(ctx, `
			SELECT id::text FROM records
			 WHERE tenant = current_setting('app.tenant', true)
			   AND model = 'pedido_venta'
			   AND data @> jsonb_build_object('externo_id', $1::text)
			   AND deleted_at IS NULL LIMIT 1`, externoID).Scan(&pvID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoEncontrado
			}
			return err
		}
		err = escanearRegistro(tx.QueryRow(ctx, `
			SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at
			  FROM records
			 WHERE tenant = current_setting('app.tenant', true)
			   AND model = 'factura_venta'
			   AND data @> jsonb_build_object('pedido', $1::text)
			   AND deleted_at IS NULL LIMIT 1`, pvID), &f)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoEncontrado
			}
			return err
		}
		facturaID = f.ID
		return nil
	})
	if err != nil {
		return f, nil, false, err
	}
	lineas, err := s.LineasDeFactura(ctx, facturaID)
	if err != nil {
		return f, nil, false, err
	}
	simplificada := cadenaDe(f.Data["tipo_factura"]) == "F2"
	return f, lineas, simplificada, nil
}
