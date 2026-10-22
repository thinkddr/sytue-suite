// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// ConvertirVenta crea el siguiente documento en una transacción y conserva su
// relación de origen. Bloquear el origen impide duplicar pedidos/facturas al
// reintentar. Un presupuesto necesita aceptación; un pedido, confirmación.
func (s *Store) ConvertirVenta(ctx context.Context, cat *catalogo.Catalogo, modelo, id, por string) (Registro, error) {
	destino, campo, estado := "pedido_venta", "presupuesto", "aceptado"
	if modelo == "pedido_venta" {
		destino, campo, estado = "factura_venta", "pedido", "confirmado"
	} else if modelo != "presupuesto" {
		return Registro{}, domain.ErrDatos
	}
	var out Registro
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		var origen Registro
		if err := escanearRegistro(tx.QueryRow(ctx, `SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at FROM records WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL FOR UPDATE`, id, modelo), &origen); err != nil {
			return err
		}
		err := escanearRegistro(tx.QueryRow(ctx, `SELECT id::text, model, ref, etiqueta, estado, data, created_at, updated_at FROM records WHERE tenant = current_setting('app.tenant', true) AND model = $1 AND data->>$2 = $3 AND deleted_at IS NULL ORDER BY created_at LIMIT 1`, destino, campo, id), &out)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrNoEncontrado) {
			return err
		}
		modulos := []string{"ventas"}
		if destino == "factura_venta" {
			modulos = append(modulos, "facturacion")
		}
		if err := exigirModulosVenta(ctx, tx, modulos...); err != nil {
			return err
		}
		if origen.Estado != estado && (modelo != "pedido_venta" || origen.Estado != "entregado") {
			return fmt.Errorf("el documento tiene que estar %s antes de convertirlo: %w", estado, domain.ErrDatos)
		}
		data := map[string]any{campo: id, "fecha": time.Now().Format("2006-01-02")}
		for _, k := range []string{"cliente", "base", "iva", "total", "notas"} {
			if v, ok := origen.Data[k]; ok {
				data[k] = v
			}
		}
		m, _ := cat.Modelo(destino)
		crudo, err := json.Marshal(data)
		if err != nil {
			return err
		}
		validado, etiqueta, err := domain.Validar(m, crudo)
		if err != nil {
			return fmt.Errorf("%w: %w", domain.ErrDatos, err)
		}
		out, err = insertarNuevo(ctx, tx, Nuevo{Modelo: m, Data: validado, Etiqueta: etiqueta}, nil, por)
		if err != nil {
			return err
		}
		grupos, err := copiarLineasVenta(ctx, tx, cat, modelo, origen.ID, out.ID, por)
		if err != nil {
			return err
		}
		return comprobarImportesVenta(origen, grupos)
	})
	return out, traducirError(err)
}

// Conserva el detalle y los tipos, sin generar un IVA medio al convertir.
func copiarLineasVenta(ctx context.Context, tx pgx.Tx, cat *catalogo.Catalogo, origen, id, destinoID, por string) ([]domain.GrupoIVA, error) {
	modeloOrigen, campoOrigen, modeloDestino, campoDestino := "linea_presupuesto", "presupuesto", "linea_venta", "pedido"
	if origen == "pedido_venta" {
		modeloOrigen, campoOrigen, modeloDestino, campoDestino = "linea_venta", "pedido", "linea_factura", "factura"
	}
	filas, err := tx.Query(ctx, `SELECT data FROM records WHERE tenant=current_setting('app.tenant',true) AND model=$1 AND data->>$2=$3 AND deleted_at IS NULL ORDER BY created_at,id`, modeloOrigen, campoOrigen, id)
	if err != nil {
		return nil, err
	}
	// Cerrar el cursor antes de escribir en la misma conexión.
	var entradas []map[string]any
	for filas.Next() {
		var crudo []byte
		if err := filas.Scan(&crudo); err != nil {
			filas.Close()
			return nil, err
		}
		var data map[string]any
		if err := json.Unmarshal(crudo, &data); err != nil {
			filas.Close()
			return nil, err
		}
		entradas = append(entradas, data)
	}
	err = filas.Err()
	filas.Close()
	if err != nil {
		return nil, err
	}
	m, _ := cat.Modelo(modeloDestino)
	fiscales := make([]domain.LineaFactura, 0, len(entradas))
	for _, l := range entradas {
		nombre := cadenaDe(l["nombre"])
		precio, tienePrecio := l["precio_con_iva"]
		tipo := cadenaDe(l["tipo_iva"])
		if modeloDestino == "linea_factura" && (!tienePrecio || tipo == "") {
			return nil, fmt.Errorf("completa precio con IVA y tipo de IVA de la línea %s antes de facturar: %w", nombre, domain.ErrDatos)
		}
		fiscal, err := domain.LineaDe(map[string]any{"descripcion": nombre, "cantidad": l["cantidad"], "precio_con_iva": precio, "tipo_iva": tipo})
		if err != nil {
			return nil, err
		}
		fiscales = append(fiscales, fiscal)
		data := map[string]any{campoDestino: destinoID, "cantidad": l["cantidad"]}
		if modeloDestino == "linea_factura" {
			data["descripcion"] = nombre
		} else {
			data["nombre"] = nombre
			data["producto"] = l["producto"]
			data["precio"] = precio
		}
		if tienePrecio {
			data["precio_con_iva"] = precio
		}
		if tipo != "" {
			data["tipo_iva"] = tipo
		}
		crudo, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		validado, etiqueta, err := domain.Validar(m, crudo)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", domain.ErrDatos, err)
		}
		if _, err := insertarNuevo(ctx, tx, Nuevo{Modelo: m, Data: validado, Etiqueta: etiqueta}, nil, por); err != nil {
			return nil, err
		}
	}
	return domain.Desglose(fiscales), nil
}

func comprobarImportesVenta(origen Registro, grupos []domain.GrupoIVA) error {
	base, err := domain.Centimos(origen.Data, "base")
	if err != nil {
		return err
	}
	iva, err := domain.Centimos(origen.Data, "iva")
	if err != nil {
		return err
	}
	var bases, cuotas int64
	for _, g := range grupos {
		bases += g.Base
		cuotas += g.Cuota
	}
	if len(grupos) > 0 && (base != bases || iva != cuotas) {
		return fmt.Errorf("revisa la base y el IVA del documento: no coinciden con sus líneas: %w", domain.ErrDatos)
	}
	if origen.Data["total"] != nil {
		total, err := domain.Centimos(origen.Data, "total")
		if err != nil {
			return err
		}
		if total != base+iva {
			return fmt.Errorf("revisa el total del documento: no coincide con su base e IVA: %w", domain.ErrDatos)
		}
	}
	return nil
}
