// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
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

// limiteSimplificada es el tope de una factura simplificada: 3.000 €, IVA incluido
// (RD 1619/2012, art. 4.1, para las ventas al por menor y el resto de su lista).
//
// ponytail: el tope general de 400 € depende de la actividad de la organización,
// que el ERP no sabe; se vigila el de 3.000, que es el de las tiendas.
const limiteSimplificada = 300000

// regimenesSinSimplificada son los regímenes en los que el RD 1619/2012 (art. 4.3)
// no deja expedir factura simplificada: la entrega intracomunitaria, lo que no se
// localiza en España (las ventas a distancia a otro Estado, entre otras) y la
// inversión del sujeto pasivo, que necesita identificar al cliente.
var regimenesSinSimplificada = map[string]bool{
	"iva_entrega_ue": true,
	"iva_no_sujeta":  true,
	"iva_inversion":  true,
}

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
// pueda serlo. Devuelve el desglose de las líneas (nada si no tiene).
func cuadreDeFactura(ctx context.Context, tx pgx.Tx, f Registro) ([]domain.GrupoIVA, error) {
	base, err := domain.Centimos(f.Data, "base")
	if err != nil {
		return nil, err
	}
	iva, err := domain.Centimos(f.Data, "iva")
	if err != nil {
		return nil, err
	}
	lineas, err := lineasDeFactura(ctx, tx, f.ID)
	if err != nil {
		return nil, err
	}
	grupos := domain.Desglose(lineas)
	if len(grupos) > 0 {
		var bases, cuotas int64
		for _, g := range grupos {
			bases += g.Base
			cuotas += g.Cuota
		}
		if bases != base || cuotas != iva {
			return nil, fmt.Errorf("la base y el IVA de la factura (%s € y %s €) tienen que ser los de sus líneas (%s € y %s €): %w",
				verifactu.Importe(base), verifactu.Importe(iva), verifactu.Importe(bases), verifactu.Importe(cuotas), domain.ErrDatos)
		}
	}
	if cadenaDe(f.Data["tipo_factura"]) == "F2" {
		switch {
		case regimenesSinSimplificada[cadenaDe(f.Data["regimen_iva"])]:
			return nil, fmt.Errorf("en este régimen de IVA no se puede emitir una factura simplificada (RD 1619/2012, art. 4.3): emite una completa: %w",
				domain.ErrDatos)
		case base+iva > limiteSimplificada:
			return nil, fmt.Errorf("una factura simplificada no puede pasar de 3.000 € con el IVA incluido (RD 1619/2012, art. 4.1): emite una completa: %w",
				domain.ErrDatos)
		}
	}
	return grupos, nil
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
