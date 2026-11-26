// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	domain "git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
)

// Líneas de factura y factura simplificada.
//
// Una factura de venta puede llevar líneas (`linea_factura`) con su tipo de IVA y
// ser completa (F1, la de siempre) o simplificada (F2, el tique de una venta al
// por menor). La cabecera sigue mandando en el libro: el asiento sale de su base
// y su IVA. Por eso, al emitirse, la cabecera tiene que ser la suma de las
// líneas, y una simplificada tiene que poder serlo. Eso vale con o sin
// VERI*FACTU: una factura que no cuadra está mal aunque no se envíe a nadie.

// LineasDeFactura son las líneas vivas de una factura, en el orden en que se
// crearon.
func (s *Store) LineasDeFactura(ctx context.Context, facturaID string) ([]domain.LineaFactura, error) {
	var out []domain.LineaFactura
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = lineasDeFactura(ctx, tx, facturaID)
		return err
	})
	return out, err
}

func lineasDeFactura(ctx context.Context, tx pgx.Tx, facturaID string) ([]domain.LineaFactura, error) {
	filas, err := tx.Query(ctx, `
		SELECT data FROM records
		 WHERE tenant = current_setting('app.tenant', true) AND model = 'linea_factura'
		   AND data @> jsonb_build_object('factura', $1::text) AND deleted_at IS NULL
		 ORDER BY created_at, id`, facturaID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []domain.LineaFactura
	for filas.Next() {
		var crudo []byte
		if err := filas.Scan(&crudo); err != nil {
			return nil, err
		}
		var data map[string]any
		if err := json.Unmarshal(crudo, &data); err != nil {
			return nil, err
		}
		l, err := domain.LineaDe(data)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, filas.Err()
}

// cuadreDeFactura comprueba lo que tiene que cumplir una factura al emitirse: que
// su base y su IVA sean los de sus líneas, si las tiene, y que una simplificada
// pueda serlo. Devuelve el desglose de las líneas (nada si no tiene). La regla es
// `dominio.CuadreFactura`, la misma que aplica el ERP de Sytue Office.
func cuadreDeFactura(ctx context.Context, tx pgx.Tx, f Registro) ([]domain.GrupoIVA, error) {
	lineas, err := lineasDeFactura(ctx, tx, f.ID)
	if err != nil {
		return nil, err
	}
	return domain.CuadreFactura(f.Data, lineas)
}

// desgloseDeGrupos pasa el desglose de las líneas al de la AEAT: cada tipo es una
// operación sujeta (S1) y las líneas al 0 % van como diga el régimen de la
// factura (exenta, no sujeta o con inversión del sujeto pasivo).
func desgloseDeGrupos(grupos []domain.GrupoIVA, regimen string) ([]verifactu.Detalle, error) {
	out := make([]verifactu.Detalle, 0, len(grupos))
	for _, g := range grupos {
		if g.Tipo > 0 {
			out = append(out, verifactu.Detalle{Calificacion: "S1", Tipo: g.Tipo * 100, Base: g.Base, Cuota: g.Cuota})
			continue
		}
		d, ok := regimenesIVA[regimen]
		if !ok {
			return nil, fmt.Errorf("%w: una línea al 0 %% tiene que decir por qué no lleva IVA: elige el régimen de IVA de la factura", verifactu.ErrDatos)
		}
		d.Base = g.Base
		out = append(out, d)
	}
	return out, nil
}
