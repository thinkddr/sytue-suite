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

	"git.sytue.eu/sytue/one/services/syerp/internal/domain"
)

// El banco entra al ERP como TAREAS: cada movimiento es una `linea_extracto`
// pendiente hasta que tiene su factura o su explicación, y justificarlo escribe
// su asiento. La línea es el origen del asiento, así que el índice único por
// documento impide asentar dos veces el mismo movimiento.

var (
	// ErrYaJustificado es un movimiento que ya tiene factura o explicación.
	ErrYaJustificado = errors.New("el movimiento ya está justificado")
	// ErrNoJustificado es deshacer un movimiento que sigue pendiente.
	ErrNoJustificado = errors.New("el movimiento no está justificado")
	// ErrFacturaNoAbierta es casar con una factura ya pagada o anulada.
	ErrFacturaNoAbierta = errors.New("la factura no está pendiente de cobro ni de pago")
)

// cuentaBancoPorDefecto es la de tesorería cuando la línea no dice de qué cuenta
// es (una línea creada a mano): la misma que usa el cobro de Stripe.
const cuentaBancoPorDefecto = "572000"

// MovimientoBanco es un movimiento de un extracto, listo para guardar.
type MovimientoBanco struct {
	// Externo es su huella: lo que hace que importarlo dos veces no lo duplique.
	Externo     string
	Fecha       time.Time
	Descripcion string
	Centimos    int64
}

// ExtractoBanco es una cuenta de un extracto importado con sus movimientos.
type ExtractoBanco struct {
	// CuentaExterno identifica la cuenta en el banco; CuentaNombre es cómo se ve.
	CuentaExterno, CuentaNombre string
	Externo                     string
	Hasta                       time.Time
	SaldoInicial, SaldoFinal    int64
	Movimientos                 []MovimientoBanco
}

// ImportarExtracto guarda un extracto y los movimientos que aún no estaban, en
// una transacción, y devuelve los índices (en e.Movimientos) de los nuevos con
// su id. Un fichero que se solapa con el anterior no duplica nada, y lo que ya
// estaba no se toca aunque se haya borrado: un movimiento borrado a propósito no
// vuelve con la importación siguiente.
func (s *Store) ImportarExtracto(ctx context.Context, e ExtractoBanco, ownerSub string) (map[int]string, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	nuevos := map[int]string{}
	err = s.escritura(ctx, func(tx pgx.Tx) error {
		// Dos importaciones a la vez del mismo fichero se esperan: la segunda
		// encuentra las huellas de la primera.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('syerp-banco:' || $1, 0))`, tenant); err != nil {
			return err
		}
		cuentaID, err := cuentaDelBanco(ctx, tx, tenant, e, ownerSub)
		if err != nil {
			return err
		}
		extractoID, err := guardarExtracto(ctx, tx, tenant, cuentaID, e, ownerSub)
		if err != nil {
			return err
		}
		vistos := map[string]bool{}
		filas, err := tx.Query(ctx, `
			SELECT data->>'externo_id' FROM records
			 WHERE tenant = $1 AND model = 'linea_extracto' AND data->>'externo_id' LIKE $2`,
			tenant, e.CuentaExterno+":%")
		if err != nil {
			return err
		}
		for filas.Next() {
			var x string
			if err := filas.Scan(&x); err != nil {
				filas.Close()
				return err
			}
			vistos[x] = true
		}
		filas.Close()
		if err := filas.Err(); err != nil {
			return err
		}
		for i, m := range e.Movimientos {
			if vistos[m.Externo] {
				continue
			}
			crudo, err := json.Marshal(map[string]any{
				"extracto": extractoID, "fecha": m.Fecha.Format(time.DateOnly),
				"descripcion": m.Descripcion, "importe": m.Centimos, "externo_id": m.Externo,
			})
			if err != nil {
				return err
			}
			var id string
			if err := tx.QueryRow(ctx, `
				INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
				VALUES ($1, 'linea_extracto', '', $2, 'pendiente', $3, $4)
				RETURNING id::text`, tenant, m.Descripcion, crudo, ownerSub).Scan(&id); err != nil {
				return err
			}
			vistos[m.Externo] = true
			nuevos[i] = id
		}
		return nil
	})
	return nuevos, err
}

// cuentaDelBanco devuelve la cuenta contable de una cuenta del banco, y la crea
// la primera vez: 572001, 572002… una por cuenta, que es como un contable separa
// dos bancos dentro de tesorería.
func cuentaDelBanco(ctx context.Context, tx pgx.Tx, tenant string, e ExtractoBanco, ownerSub string) (string, error) {
	filtro, err := json.Marshal(map[string]string{"externo_id": e.CuentaExterno})
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM records
		 WHERE tenant = $1 AND model = 'cuenta_contable' AND data @> $2::jsonb AND deleted_at IS NULL
		 LIMIT 1`, tenant, filtro).Scan(&id)
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	var n int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM records
		 WHERE tenant = $1 AND model = 'cuenta_contable' AND data->>'externo_id' LIKE 'n43:%'`, tenant).Scan(&n); err != nil {
		return "", err
	}
	crudo, err := json.Marshal(map[string]any{
		"codigo": fmt.Sprintf("572%03d", n+1), "nombre": e.CuentaNombre, "tipo": "banco", "externo_id": e.CuentaExterno,
	})
	if err != nil {
		return "", err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
		VALUES ($1, 'cuenta_contable', '', $2, '', $3, $4)
		RETURNING id::text`, tenant, e.CuentaNombre, crudo, ownerSub).Scan(&id)
	return id, err
}

func guardarExtracto(ctx context.Context, tx pgx.Tx, tenant, cuentaID string, e ExtractoBanco, ownerSub string) (string, error) {
	data := map[string]any{
		"cuenta": cuentaID, "fecha": e.Hasta.Format(time.DateOnly),
		"saldo_inicial": e.SaldoInicial, "saldo_final": e.SaldoFinal, "externo_id": e.Externo,
	}
	crudo, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	filtro, err := json.Marshal(map[string]string{"externo_id": e.Externo})
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM records
		 WHERE tenant = $1 AND model = 'extracto_bancario' AND data @> $2::jsonb AND deleted_at IS NULL
		 LIMIT 1`, tenant, filtro).Scan(&id)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE records SET data = $2, updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, id, crudo)
		return id, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	ref, err := reservarRef(ctx, tx, "doc:extracto_bancario", "EXT", periodoDe(data))
	if err != nil {
		return "", err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub)
		VALUES ($1, 'extracto_bancario', $2, '', 'confirmado', $3, $4)
		RETURNING id::text`, tenant, ref, crudo, ownerSub).Scan(&id)
	return id, err
}

// FacturaAbierta es una factura sin cobrar o sin pagar, con lo que hace falta
// para reconocerla en el banco.
type FacturaAbierta struct {
	ID          string `json:"id"`
	Modelo      string `json:"modelo"`
	Ref         string `json:"ref"`
	Estado      string `json:"estado"`
	Tercero     string `json:"tercero"`
	NIF         string `json:"-"`
	Fecha       string `json:"fecha"`
	Vencimiento string `json:"-"`
	Total       int64  `json:"total"`
}

// FacturasAbiertas son las facturas emitidas y recibidas que aún no se han
// cobrado ni pagado, las más recientes primero.
func (s *Store) FacturasAbiertas(ctx context.Context) ([]FacturaAbierta, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	var out []FacturaAbierta
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// ponytail: las 2.000 más recientes; una factura más vieja sin cobrar se
		// casa a mano desde su ficha.
		filas, err := tx.Query(ctx, `
			SELECT f.id::text, f.model, f.ref, f.estado,
			       COALESCE(c.etiqueta, ''), COALESCE(c.data->>'nif', ''),
			       COALESCE(f.data->>'fecha', ''), COALESCE(f.data->>'vencimiento', ''),
			       COALESCE(NULLIF(`+centimosSQL("f.data->'total'")+`, 0),
			                `+centimosSQL("f.data->'base'")+` + `+centimosSQL("f.data->'iva'")+`)
			  FROM records f
			  LEFT JOIN records c
			    ON c.tenant = f.tenant AND c.deleted_at IS NULL
			   AND c.id::text = COALESCE(f.data->>'cliente', f.data->>'proveedor')
			 WHERE f.tenant = $1 AND f.model IN ('factura_venta', 'factura_compra')
			   AND f.estado IN ('borrador', 'confirmado') AND f.deleted_at IS NULL
			 ORDER BY f.data->>'fecha' DESC
			 LIMIT 2000`, tenant)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var f FacturaAbierta
			if err := filas.Scan(&f.ID, &f.Modelo, &f.Ref, &f.Estado, &f.Tercero, &f.NIF,
				&f.Fecha, &f.Vencimiento, &f.Total); err != nil {
				return err
			}
			out = append(out, f)
		}
		return filas.Err()
	})
	return out, err
}

// centimosSQL lee un importe de `data` sin que una celda rara (texto, decimal)
// tumbe la consulta entera: lo que no es un número cuenta como cero.
func centimosSQL(expr string) string {
	return "(CASE WHEN jsonb_typeof(" + expr + ") = 'number' THEN (" + expr + ")::numeric::bigint ELSE 0 END)"
}

// LineaBanco es un movimiento del banco tal como lo ve la lista de tareas.
type LineaBanco struct {
	ID          string `json:"id"`
	Fecha       string `json:"fecha"`
	Descripcion string `json:"descripcion"`
	Centimos    int64  `json:"importe"`
	// Factura y Motivo explican un movimiento ya justificado.
	FacturaID     string `json:"factura_id,omitempty"`
	FacturaModelo string `json:"factura_modelo,omitempty"`
	FacturaRef    string `json:"factura_ref,omitempty"`
	Motivo        string `json:"motivo,omitempty"`
	// La pista de Sytue AI para un cargo: quién lo cobró y dónde pedir su factura.
	Empresa       string `json:"empresa,omitempty"`
	Web           string `json:"web,omitempty"`
	URLFactura    string `json:"url_factura,omitempty"`
	EmailFacturas string `json:"email_facturas,omitempty"`
}

// LineasBanco devuelve los movimientos en un estado: los pendientes, del más
// antiguo al más reciente (es el orden en que bloquean obligaciones), o los
// justificados, lo último primero. El total cuenta todos, no solo los servidos.
func (s *Store) LineasBanco(ctx context.Context, estado string, limite int) ([]LineaBanco, int, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, 0, err
	}
	orden := "l.data->>'fecha', l.created_at"
	if estado != "pendiente" {
		orden = "l.updated_at DESC"
	}
	var out []LineaBanco
	total := 0
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT l.id::text, COALESCE(l.data->>'fecha', ''), COALESCE(l.data->>'descripcion', ''),
			       `+centimosSQL("l.data->'importe'")+`,
			       COALESCE(f.id::text, ''), COALESCE(f.model, ''), COALESCE(f.ref, ''),
			       COALESCE(l.data->>'motivo', ''), COALESCE(l.data->>'empresa', ''), COALESCE(l.data->>'web', ''),
			       COALESCE(l.data->>'url_factura', ''), COALESCE(l.data->>'email_facturas', ''), count(*) OVER ()
			  FROM records l
			  LEFT JOIN records f
			    ON f.tenant = l.tenant
			   AND f.id::text = COALESCE(l.data->>'factura_venta', l.data->>'factura_compra')
			 WHERE l.tenant = $1 AND l.model = 'linea_extracto' AND l.estado = $2 AND l.deleted_at IS NULL
			 ORDER BY `+orden+`
			 LIMIT $3`, tenant, estado, limite)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var l LineaBanco
			if err := filas.Scan(&l.ID, &l.Fecha, &l.Descripcion, &l.Centimos,
				&l.FacturaID, &l.FacturaModelo, &l.FacturaRef, &l.Motivo,
				&l.Empresa, &l.Web, &l.URLFactura, &l.EmailFacturas, &total); err != nil {
				return err
			}
			out = append(out, l)
		}
		return filas.Err()
	})
	return out, total, err
}

// CargosSinPista son cargos pendientes a los que Sytue AI aún no ha buscado de
// dónde sacar la factura (no tienen `empresa`, ni vacía), los más antiguos antes.
func (s *Store) CargosSinPista(ctx context.Context, limite int) ([]LineaBanco, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	var out []LineaBanco
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `
			SELECT id::text, COALESCE(data->>'fecha', ''), COALESCE(data->>'descripcion', ''), `+centimosSQL("data->'importe'")+`
			  FROM records
			 WHERE tenant = $1 AND model = 'linea_extracto' AND estado = 'pendiente' AND deleted_at IS NULL
			   AND NOT (data ? 'empresa') AND `+centimosSQL("data->'importe'")+` < 0
			 ORDER BY data->>'fecha', created_at
			 LIMIT $2`, tenant, limite)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var l LineaBanco
			if err := filas.Scan(&l.ID, &l.Fecha, &l.Descripcion, &l.Centimos); err != nil {
				return err
			}
			out = append(out, l)
		}
		return filas.Err()
	})
	return out, err
}

// GuardarPista apunta en un movimiento dónde conseguir su factura. Se guarda
// aunque venga vacía: así no se vuelve a preguntar por él.
func (s *Store) GuardarPista(ctx context.Context, lineaID, empresa, web, urlFactura, email string) error {
	crudo, err := json.Marshal(map[string]string{
		"empresa": empresa, "web": web, "url_factura": urlFactura, "email_facturas": email,
	})
	if err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE records SET data = data || $2::jsonb, updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = 'linea_extracto'`, lineaID, crudo)
		return err
	})
}

// Liquidacion es lo que justifica un movimiento: la factura que cobra o paga, o
// el motivo por el que no lleva factura con la cuenta a la que va.
type Liquidacion struct {
	LineaID string
	// FacturaID y ModeloFactura (factura_venta o factura_compra), si lleva factura.
	FacturaID, ModeloFactura string
	// Motivo, si no lleva factura.
	Motivo        string
	Contrapartida string
	Concepto      string
}

// Liquidar asienta un movimiento del banco y lo da por justificado, todo en una
// transacción: el asiento del banco contra la contrapartida, la factura pagada y
// la línea conciliada con lo que la explica.
func (s *Store) Liquidar(ctx context.Context, l Liquidacion, ownerSub string) (Asiento, error) {
	var out Asiento
	err := s.escritura(ctx, func(tx pgx.Tx) error {
		var estado, fechaTxt, cuentaBanco string
		var importe int64
		err := tx.QueryRow(ctx, `
			SELECT l.estado, COALESCE(l.data->>'fecha', ''), `+centimosSQL("l.data->'importe'")+`,
			       COALESCE(c.data->>'codigo', '')
			  FROM records l
			  LEFT JOIN records e ON e.tenant = l.tenant AND e.id::text = l.data->>'extracto'
			  LEFT JOIN records c ON c.tenant = l.tenant AND c.id::text = e.data->>'cuenta'
			 WHERE l.id = $1::uuid AND l.model = 'linea_extracto' AND l.deleted_at IS NULL
			   FOR UPDATE OF l`, l.LineaID).Scan(&estado, &fechaTxt, &importe, &cuentaBanco)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNoEncontrado
		case err != nil:
			return err
		case estado != "pendiente":
			return ErrYaJustificado
		}
		fecha, err := time.Parse(time.DateOnly, fechaTxt)
		if err != nil || importe == 0 {
			return fmt.Errorf("movimiento sin fecha o sin importe: %w", domain.ErrDatos)
		}
		if cuentaBanco == "" {
			cuentaBanco = cuentaBancoPorDefecto
		}

		explica := map[string]any{"motivo": l.Motivo}
		tercero := ""
		if l.FacturaID != "" {
			campo := "cliente"
			if l.ModeloFactura == "factura_compra" {
				campo = "proveedor"
			}
			var estadoFactura string
			err := tx.QueryRow(ctx, `
				SELECT estado, COALESCE(data->>$3, '') FROM records
				 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = $2 AND deleted_at IS NULL
				 FOR UPDATE`, l.FacturaID, l.ModeloFactura, campo).Scan(&estadoFactura, &tercero)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				return ErrNoEncontrado
			case err != nil:
				return err
			case estadoFactura != "confirmado":
				return ErrFacturaNoAbierta
			}
			explica = map[string]any{l.ModeloFactura: l.FacturaID}
		}

		// Entra dinero: el banco al debe. Sale: al haber. La contrapartida, al revés.
		centimos := max(importe, -importe)
		banco := Apunte{Cuenta: cuentaBanco, Concepto: l.Concepto}
		otra := Apunte{Cuenta: l.Contrapartida, Concepto: l.Concepto, Tercero: tercero}
		if importe > 0 {
			banco.Debe, otra.Haber = centimos, centimos
		} else {
			banco.Haber, otra.Debe = centimos, centimos
		}
		if out, err = escribirAsiento(ctx, tx, "banco", fecha, l.Concepto, l.LineaID, ownerSub, []Apunte{banco, otra}); err != nil {
			return err
		}
		if l.FacturaID != "" {
			if _, err := tx.Exec(ctx, `UPDATE records SET estado = 'pagado', updated_at = now() WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, l.FacturaID); err != nil {
				return err
			}
		}
		crudo, err := json.Marshal(explica)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE records SET estado = 'conciliado', data = data || $2::jsonb, updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, l.LineaID, crudo)
		return err
	})
	return out, err
}

// Reabrir deshace la justificación de un movimiento: anula su asiento, devuelve
// la factura a pendiente y la línea vuelve a ser una tarea. Nada se borra: el
// asiento anulado queda en el libro, como toda corrección.
func (s *Store) Reabrir(ctx context.Context, lineaID string) error {
	return s.escritura(ctx, func(tx pgx.Tx) error {
		var estado, venta, compra string
		err := tx.QueryRow(ctx, `
			SELECT estado, COALESCE(data->>'factura_venta', ''), COALESCE(data->>'factura_compra', '')
			  FROM records
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND model = 'linea_extracto' AND deleted_at IS NULL
			 FOR UPDATE`, lineaID).Scan(&estado, &venta, &compra)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrNoEncontrado
		case err != nil:
			return err
		case estado != "conciliado":
			return ErrNoJustificado
		}
		// Reabrir anula el asiento del banco: en un período cerrado, no.
		filas, err := tx.Query(ctx, `
			SELECT fecha FROM moves
			 WHERE tenant = current_setting('app.tenant', true) AND origen_id = $1::uuid AND estado = 'asentado'`, lineaID)
		if err != nil {
			return err
		}
		fechas, err := pgx.CollectRows(filas, pgx.RowTo[time.Time])
		if err != nil {
			return err
		}
		for _, f := range fechas {
			if err := exigirPeriodoAbierto(ctx, tx, f); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE moves SET estado = 'anulado', concepto = concepto || ' · reabierto'
			 WHERE tenant = current_setting('app.tenant', true) AND origen_id = $1::uuid AND estado = 'asentado'`, lineaID); err != nil {
			return err
		}
		for _, f := range []string{venta, compra} {
			if f == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				UPDATE records SET estado = 'confirmado', updated_at = now()
				 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid AND estado = 'pagado'`, f); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `
			UPDATE records
			   SET estado = 'pendiente', data = data - 'factura_venta' - 'factura_compra' - 'motivo', updated_at = now()
			 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, lineaID)
		return err
	})
}

// Pendientes cuenta, por comprobación y por mes (AAAA-MM), lo que falta para que
// la contabilidad de un periodo esté completa: movimientos del banco sin
// justificar («banco») y facturas o nóminas que no están en el libro
// («facturas», «nominas»). Es el «te falta» de cada obligación.
func (s *Store) Pendientes(ctx context.Context, desde, hasta time.Time) (map[string]map[string]int, error) {
	tenant, err := tenantDe(ctx)
	if err != nil {
		return nil, err
	}
	var out map[string]map[string]int
	err = s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = pendientesEnTx(ctx, tx, tenant, desde, hasta)
		return err
	})
	return out, err
}

// Contador y listado del asistente comparten esta regla: confirmado/pagado no
// significa asentado. El alias r es el de records en ambas consultas.
const facturaPendienteSQL = `r.estado = 'borrador'
	OR (r.estado IN ('confirmado', 'pagado') AND NOT EXISTS (
		SELECT 1 FROM moves m
		WHERE m.tenant = r.tenant AND m.origen_id = r.id AND m.estado <> 'anulado'))`

func pendientesEnTx(ctx context.Context, tx pgx.Tx, tenant string, desde, hasta time.Time) (map[string]map[string]int, error) {
	out := map[string]map[string]int{}
	// TEXTO ISO: una celda mal escrita no tumba la vista por conversión.
	filas, err := tx.Query(ctx, `
		SELECT CASE r.model WHEN 'linea_extracto' THEN 'banco' WHEN 'nomina' THEN 'nominas' ELSE 'facturas' END,
		       left(r.data->>'fecha', 7), count(*)
		  FROM records r
		 WHERE r.tenant = $1
		   AND r.model IN ('linea_extracto', 'factura_venta', 'factura_compra', 'nomina')
		   AND r.deleted_at IS NULL
		   AND r.data->>'fecha' BETWEEN $2 AND $3
		   AND CASE WHEN r.model = 'linea_extracto' THEN r.estado = 'pendiente'
		            ELSE (`+facturaPendienteSQL+`) END
		 GROUP BY 1, 2`, tenant, desde.Format(time.DateOnly), hasta.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	for filas.Next() {
		var clave, mes string
		var n int
		if err := filas.Scan(&clave, &mes, &n); err != nil {
			return nil, err
		}
		if out[clave] == nil {
			out[clave] = map[string]int{}
		}
		out[clave][mes] = n
	}
	return out, filas.Err()
}

// escribirAsiento abre un asiento, escribe sus apuntes y lo cierra dentro de la
// transacción dada. El orden no es una preferencia: el trigger
// `move_lines_inmutables` no deja añadir apuntes a un asiento ya asentado.
func escribirAsiento(ctx context.Context, tx pgx.Tx, diario string, fecha time.Time,
	concepto, origenID, ownerSub string, apuntes []Apunte,
) (Asiento, error) {
	// Todo lo que entra al libro pasa por el período, no solo lo que llega por
	// `/asentar`: el banco, el TPV y los cobros escribían en meses ya cerrados
	// (NEG-12).
	if err := exigirPeriodoAbierto(ctx, tx, fecha); err != nil {
		return Asiento{}, err
	}
	ref, err := reservarRef(ctx, tx, "asiento:"+diario, prefijoDiario(diario), fmt.Sprintf("%04d", fecha.Year()))
	if err != nil {
		return Asiento{}, err
	}
	var moveID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO moves (tenant, diario, ref, fecha, concepto, moneda, origen_id, estado, owner_sub)
		VALUES (current_setting('app.tenant', true), $1, $2, $3, $4, 'EUR', $5::uuid, 'borrador', $6)
		RETURNING id::text`, diario, ref, fecha, concepto, nuloSiVacio(origenID), ownerSub).Scan(&moveID); err != nil {
		return Asiento{}, err
	}
	for _, ap := range apuntes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO move_lines (move_id, tenant, cuenta, concepto, debe, haber, tercero_id)
			VALUES ($1::uuid, current_setting('app.tenant', true), $2, $3, $4, $5, $6::uuid)`,
			moveID, ap.Cuenta, ap.Concepto, ap.Debe, ap.Haber, nuloSiVacio(ap.Tercero)); err != nil {
			return Asiento{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE moves SET estado = 'asentado', posted_at = now()
		 WHERE tenant = current_setting('app.tenant', true) AND id = $1::uuid`, moveID); err != nil {
		return Asiento{}, err
	}
	return Asiento{
		ID: moveID, Diario: diario, Ref: ref, Fecha: fecha, Concepto: concepto,
		Moneda: "EUR", Estado: "asentado", OrigenID: origenID, Apuntes: apuntes,
	}, nil
}
