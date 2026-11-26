// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// La CARTERA: lo que se debe y a quién, y lo que falta en el almacén. Cuatro lecturas
// que un ERP enseña en la portada y que aquí había que componer a mano filtrando
// listas: la antigüedad de lo pendiente de cobro y de pago, el estado de cuenta de un
// contacto, los productos por debajo de su stock mínimo y el libro mayor de una cuenta.
//
// Las tres primeras leen la maestra (`records`); el mayor, el libro (`move_lines`). Cada
// una es UNA consulta en UNA transacción, con `tenant` explícito en el WHERE por lo
// mismo que `Listar`: es la primera columna de sus índices.

// EstadosAbiertos son los de una factura emitida que aún no se ha cobrado o pagado: los
// mismos que «Pendiente de cobro» en `Resumen`, para que la portada cuente lo mismo.
var EstadosAbiertos = []string{"confirmado", "emitida", "pendiente"}

// fechaSQL es la fecha de una clave de `data`, o NULL si no tiene forma de AAAA-MM-DD: un
// valor importado raro no tumba la consulta entera al convertirlo.
func fechaSQL(clave string) string {
	return `(CASE WHEN data->>'` + clave + `' ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' THEN (data->>'` + clave + `')::date END)`
}

// Tramos de antigüedad: por días desde el vencimiento. «al_dia» es lo que aún no vence.
const (
	TramoAlDia = "al_dia"
	Tramo1a30  = "1_30"
	Tramo31a60 = "31_60"
	Tramo61a90 = "61_90"
	TramoMas90 = "mas_90"
)

// diasPorTramo son los días de cada tramo vencido.
const diasPorTramo = 30

// TramosAntiguedad son los tramos en el orden en que se enseñan.
var TramosAntiguedad = []string{TramoAlDia, Tramo1a30, Tramo31a60, Tramo61a90, TramoMas90}

// TramoDeDias es el tramo de un documento que vence hace `dias` (negativo o cero: aún
// no ha vencido). Es la regla de la consulta, escrita también aquí para probarla sin
// base de datos.
func TramoDeDias(dias int) string {
	switch {
	case dias <= 0:
		return TramoAlDia
	case dias <= diasPorTramo:
		return Tramo1a30
	case dias <= 2*diasPorTramo:
		return Tramo31a60
	case dias <= 3*diasPorTramo:
		return Tramo61a90
	}
	return TramoMas90
}

// TramoAntiguedad es un tramo con lo que suma y cuántos documentos lo forman.
type TramoAntiguedad struct {
	Tramo      string `json:"tramo"`
	Centimos   int64  `json:"centimos"`
	Documentos int    `json:"documentos"`
}

// Antiguedad es lo pendiente de cobro (facturas de cliente) o de pago (de proveedor)
// por tramos de días vencido. Vencido es todo lo que no está al día.
type Antiguedad struct {
	Modelo  string            `json:"modelo"`
	Hoy     string            `json:"hoy"`
	Tramos  []TramoAntiguedad `json:"tramos"`
	Total   int64             `json:"total"`
	Vencido int64             `json:"vencido"`
}

// Antiguedad agrupa las facturas abiertas de `modelo` por tramos de días desde su
// vencimiento (o desde su fecha, si no lo tienen) a `hoy`. Los cinco tramos salen
// siempre, también a cero: un tramo vacío es información («nada vencido de más de 90»).
func (s *Store) Antiguedad(ctx context.Context, modelo string, hoy time.Time) (Antiguedad, error) {
	out := Antiguedad{Modelo: modelo, Hoy: hoy.Format(time.DateOnly)}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return out, err
	}
	porTramo := map[string]TramoAntiguedad{}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT CASE WHEN dias <= 0 THEN $5
			            WHEN dias <= 30 THEN $6
			            WHEN dias <= 60 THEN $7
			            WHEN dias <= 90 THEN $8
			            ELSE $9 END AS tramo,
			       COALESCE(sum(total), 0), count(*)
			  FROM (SELECT $3::date - COALESCE(`+fechaSQL("vencimiento")+`, `+fechaSQL("fecha")+`, $3::date) AS dias,
			               COALESCE((data->>'total')::bigint, 0) AS total
			          FROM records
			         WHERE tenant = $1 AND model = $2 AND deleted_at IS NULL AND estado = ANY($4)) d
			 GROUP BY 1`,
			tenant, modelo, hoy.Format(time.DateOnly), EstadosAbiertos,
			TramoAlDia, Tramo1a30, Tramo31a60, Tramo61a90, TramoMas90)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var t TramoAntiguedad
			if err := filas.Scan(&t.Tramo, &t.Centimos, &t.Documentos); err != nil {
				return err
			}
			porTramo[t.Tramo] = t
		}
		return filas.Err()
	})
	for _, clave := range TramosAntiguedad {
		t := porTramo[clave]
		t.Tramo = clave
		out.Tramos = append(out.Tramos, t)
		out.Total += t.Centimos
		if clave != TramoAlDia {
			out.Vencido += t.Centimos
		}
	}
	return out, err
}

// ResumenCuenta son las cifras de un lado de la relación con un contacto: lo facturado
// (emitido, sin borradores ni anuladas), lo pagado, lo pendiente y, de eso, lo vencido.
type ResumenCuenta struct {
	Facturado int64 `json:"facturado"`
	Pagado    int64 `json:"pagado"`
	Pendiente int64 `json:"pendiente"`
	Vencido   int64 `json:"vencido"`
}

// DocumentoCuenta es una factura en el estado de cuenta.
type DocumentoCuenta struct {
	ID          string `json:"id"`
	Modelo      string `json:"modelo"`
	Ref         string `json:"ref"`
	Fecha       string `json:"fecha"`
	Vencimiento string `json:"vencimiento,omitempty"`
	Estado      string `json:"estado"`
	Total       int64  `json:"total"`
	// DiasVencida es cuántos días lleva vencida una factura abierta (0 si no lo está).
	DiasVencida int `json:"dias_vencida"`
}

// EstadoCuenta es la relación económica con un contacto: lo que se le ha facturado
// (`Ventas`), lo que ha facturado él (`Compras`) y sus documentos, los abiertos primero.
type EstadoCuenta struct {
	Ventas     ResumenCuenta     `json:"ventas"`
	Compras    ResumenCuenta     `json:"compras"`
	Documentos []DocumentoCuenta `json:"documentos"`
	// Recortado: hay más documentos de los que se enseñan (los totales cuentan todos).
	Recortado bool `json:"recortado"`
}

// MaxDocumentosCuenta es cuántas facturas se listan en el estado de cuenta.
const MaxDocumentosCuenta = 50

// EstadoDeCuenta reúne las facturas de cliente cuyo cliente es el contacto y las de
// proveedor cuyo proveedor es él. Los totales salen de TODAS; la lista, de las
// `MaxDocumentosCuenta` más relevantes: abiertas antes, y dentro, las más vencidas.
func (s *Store) EstadoDeCuenta(ctx context.Context, contacto string, hoy time.Time) (EstadoCuenta, error) {
	out := EstadoCuenta{Documentos: []DocumentoCuenta{}}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return out, err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT id::text AS rid, model, ref, estado,
			       COALESCE(`+fechaSQL("fecha")+`::text, ''), COALESCE(`+fechaSQL("vencimiento")+`::text, ''),
			       COALESCE((data->>'total')::bigint, 0),
			       GREATEST($3::date - COALESCE(`+fechaSQL("vencimiento")+`, `+fechaSQL("fecha")+`, $3::date), 0)
			  FROM records
			 WHERE tenant = $1 AND deleted_at IS NULL
			   AND estado NOT IN ('borrador', 'anulado')
			   AND ((model = 'factura_venta' AND data @> jsonb_build_object('cliente', $2::text))
			     OR (model = 'factura_compra' AND data @> jsonb_build_object('proveedor', $2::text)))
			 ORDER BY (estado = ANY($4)) DESC, 8 DESC, 5 DESC, id`,
			tenant, contacto, hoy.Format(time.DateOnly), EstadosAbiertos)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var d DocumentoCuenta
			if err := filas.Scan(&d.ID, &d.Modelo, &d.Ref, &d.Estado, &d.Fecha, &d.Vencimiento, &d.Total, &d.DiasVencida); err != nil {
				return err
			}
			lado := &out.Ventas
			if d.Modelo == "factura_compra" {
				lado = &out.Compras
			}
			abierta := false
			for _, e := range EstadosAbiertos {
				abierta = abierta || d.Estado == e
			}
			if !abierta {
				d.DiasVencida = 0
			}
			sumarACuenta(lado, d, abierta)
			if len(out.Documentos) < MaxDocumentosCuenta {
				out.Documentos = append(out.Documentos, d)
			} else {
				out.Recortado = true
			}
		}
		return filas.Err()
	})
	return out, err
}

// sumarACuenta pone un documento en su lado del estado de cuenta.
func sumarACuenta(r *ResumenCuenta, d DocumentoCuenta, abierta bool) {
	r.Facturado += d.Total
	switch {
	case abierta:
		r.Pendiente += d.Total
		if d.DiasVencida > 0 {
			r.Vencido += d.Total
		}
	case d.Estado == "pagado":
		r.Pagado += d.Total
	}
}

// ProductoBajoMinimo es un producto con menos existencias que su stock mínimo.
type ProductoBajoMinimo struct {
	ID      string `json:"id"`
	Ref     string `json:"ref,omitempty"`
	Nombre  string `json:"nombre"`
	Minimo  string `json:"minimo"`
	Stock   string `json:"stock"`
	Unidad  string `json:"unidad,omitempty"`
	Faltan  string `json:"faltan"`
	Almacen int    `json:"almacenes"`
}

// MaxBajoMinimo es cuántos productos bajo mínimo se devuelven: los más apurados.
const MaxBajoMinimo = 50

// decimalSQL es un decimal de `data` como numeric, o NULL si no tiene forma de número.
func decimalSQL(alias, clave string) string {
	return `(CASE WHEN ` + alias + `.data->>'` + clave + `' ~ '^-?[0-9]+(\.[0-9]+)?$' THEN (` + alias + `.data->>'` + clave + `')::numeric END)`
}

// BajoMinimo devuelve los productos activos con stock mínimo cuyas existencias, sumadas
// en todos los almacenes y lotes, no llegan a él; los más apurados primero (menos stock
// en proporción a su mínimo). Las cantidades viajan como cadena decimal, como en el
// resto del ERP: un float aquí redondearía «0,1 + 0,2».
func (s *Store) BajoMinimo(ctx context.Context) ([]ProductoBajoMinimo, error) {
	out := []ProductoBajoMinimo{}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return out, err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT p.id::text AS pid, p.ref, p.etiqueta, COALESCE(p.data->>'unidad', ''),
			       min_.minimo::text, COALESCE(sum(e.cantidad), 0)::text,
			       (min_.minimo - COALESCE(sum(e.cantidad), 0))::text, count(DISTINCT e.almacen)
			  FROM records p
			  CROSS JOIN LATERAL (SELECT `+decimalSQL("p", "stock_minimo")+` AS minimo) min_
			  LEFT JOIN LATERAL (
			       SELECT `+decimalSQL("x", "existencias")+` AS cantidad, x.data->>'almacen' AS almacen
			         FROM records x
			        WHERE x.tenant = p.tenant AND x.model = 'existencia' AND x.deleted_at IS NULL
			          AND x.data @> jsonb_build_object('producto', p.id::text)) e ON true
			 WHERE p.tenant = $1 AND p.model = 'producto' AND p.deleted_at IS NULL
			   AND p.estado <> 'pausada' AND min_.minimo > 0
			 GROUP BY p.id, p.ref, p.etiqueta, p.data, min_.minimo
			HAVING COALESCE(sum(e.cantidad), 0) < min_.minimo
			 ORDER BY COALESCE(sum(e.cantidad), 0) / min_.minimo, p.etiqueta, p.id
			 LIMIT $2`, tenant, MaxBajoMinimo)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var p ProductoBajoMinimo
			if err := filas.Scan(&p.ID, &p.Ref, &p.Nombre, &p.Unidad, &p.Minimo, &p.Stock, &p.Faltan, &p.Almacen); err != nil {
				return err
			}
			p.Minimo, p.Stock, p.Faltan = sinCerosDeMas(p.Minimo), sinCerosDeMas(p.Stock), sinCerosDeMas(p.Faltan)
			out = append(out, p)
		}
		return filas.Err()
	})
	return out, err
}

// sinCerosDeMas quita los ceros que `numeric` arrastra a la derecha («12.500» → «12.5»,
// «3.000» → «3»): la cantidad se escribe como la escribió quien la dio de alta.
func sinCerosDeMas(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			for len(s) > i+1 && s[len(s)-1] == '0' {
				s = s[:len(s)-1]
			}
			if len(s) == i+1 {
				s = s[:i]
			}
			return s
		}
	}
	return s
}

// ApunteMayor es una línea del libro mayor de una cuenta, con el saldo que deja.
type ApunteMayor struct {
	AsientoID string `json:"asiento_id"`
	Ref       string `json:"ref"`
	Fecha     string `json:"fecha"`
	Concepto  string `json:"concepto"`
	Debe      int64  `json:"debe"`
	Haber     int64  `json:"haber"`
	Saldo     int64  `json:"saldo"`
}

// Mayor es el libro mayor de una cuenta en un período: el saldo con el que entra, cada
// apunte asentado con el saldo que deja, y el saldo final.
type Mayor struct {
	Cuenta       string        `json:"cuenta"`
	SaldoInicial int64         `json:"saldo_inicial"`
	Apuntes      []ApunteMayor `json:"apuntes"`
	Debe         int64         `json:"debe"`
	Haber        int64         `json:"haber"`
	SaldoFinal   int64         `json:"saldo_final"`
	Recortado    bool          `json:"recortado"`
}

// MaxApuntesMayor es cuántos apuntes devuelve un mayor; el saldo final cuenta todos.
const MaxApuntesMayor = 1000

// Mayor lee los apuntes ASENTADOS de una cuenta entre `desde` y `hasta` (cero = sin
// límite) con su saldo acumulado. El saldo de entrada es lo asentado antes de `desde`,
// y el acumulado se calcula en Postgres con una ventana, no línea a línea en Go: así el
// saldo final es exacto aunque la lista se recorte.
func (s *Store) Mayor(ctx context.Context, cuenta string, desde, hasta time.Time) (Mayor, error) {
	out := Mayor{Cuenta: cuenta, Apuntes: []ApunteMayor{}}
	tenant, err := tenantDe(ctx)
	if err != nil {
		return out, err
	}
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(sum(l.debe - l.haber), 0)
			  FROM move_lines l JOIN moves m ON m.id = l.move_id AND m.tenant = l.tenant
			 WHERE l.tenant = $1 AND m.estado = 'asentado' AND l.cuenta = $2
			   AND $3::date IS NOT NULL AND m.fecha < $3::date`,
			tenant, cuenta, nuloSiCero(desde)).Scan(&out.SaldoInicial); err != nil {
			return err
		}
		filas, err := tx.Query(ctx, `
			SELECT m.id::text AS mid, m.ref, m.fecha::text, l.concepto, l.debe, l.haber,
			       $4::bigint + sum(l.debe - l.haber) OVER (ORDER BY m.fecha, m.id, l.id)
			  FROM move_lines l JOIN moves m ON m.id = l.move_id AND m.tenant = l.tenant
			 WHERE l.tenant = $1 AND m.estado = 'asentado' AND l.cuenta = $2
			   AND ($3::date IS NULL OR m.fecha >= $3::date)
			   AND ($5::date IS NULL OR m.fecha <= $5::date)
			 ORDER BY m.fecha, m.id, l.id`,
			tenant, cuenta, nuloSiCero(desde), out.SaldoInicial, nuloSiCero(hasta))
		if err != nil {
			return err
		}
		defer filas.Close()
		out.SaldoFinal = out.SaldoInicial
		for filas.Next() {
			var a ApunteMayor
			if err := filas.Scan(&a.AsientoID, &a.Ref, &a.Fecha, &a.Concepto, &a.Debe, &a.Haber, &a.Saldo); err != nil {
				return err
			}
			out.Debe += a.Debe
			out.Haber += a.Haber
			out.SaldoFinal = a.Saldo
			if len(out.Apuntes) < MaxApuntesMayor {
				out.Apuntes = append(out.Apuntes, a)
			} else {
				out.Recortado = true
			}
		}
		return filas.Err()
	})
	return out, err
}
