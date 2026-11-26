// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
)

// El LIBRO DIARIO (entrega 2, E-05). Es el de `services/syerp/internal/store/libro.go`
// sobre SQLite: el asiento sale de `dominio.AsientoDe`, la referencia de
// `dominio.PrefijoDiario` con la misma secuencia por diario y año, y lo que en Postgres
// garantizan los disparadores (el cuadre, la inmutabilidad, «un documento, un
// asiento», el período cerrado) lo garantiza aquí la transacción que escribe y el
// esquema: un apunte a un solo lado, un asiento vivo por documento.

var (
	// ErrYaAsentado: el documento ya tiene un asiento que no está anulado.
	ErrYaAsentado = errors.New("erp: el documento ya está en el libro")
	// ErrPeriodoCerrado: la fecha cae en un período ya cerrado.
	ErrPeriodoCerrado = errors.New("erp: el período contable está cerrado")
	// ErrCierreHaciaAtras: cerrar con una fecha que no avanza el corte.
	ErrCierreHaciaAtras = errors.New("erp: el libro ya está cerrado hasta una fecha posterior")
)

const esquemaLibro = `
CREATE TABLE IF NOT EXISTS asiento (
  id       TEXT PRIMARY KEY,
  diario   TEXT NOT NULL,
  ref      TEXT NOT NULL,
  fecha    TEXT NOT NULL,              -- AAAA-MM-DD
  concepto TEXT NOT NULL DEFAULT '',
  moneda   TEXT NOT NULL DEFAULT 'EUR',
  estado   TEXT NOT NULL CHECK (estado IN ('asentado', 'anulado')),
  origen   TEXT NOT NULL DEFAULT '',   -- el documento o el movimiento del banco
  creado   TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS asiento_ref ON asiento(ref);
CREATE UNIQUE INDEX IF NOT EXISTS asiento_origen_vivo ON asiento(origen) WHERE origen <> '' AND estado = 'asentado';
CREATE INDEX IF NOT EXISTS asiento_fecha ON asiento(fecha);
CREATE TABLE IF NOT EXISTS apunte (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  asiento  TEXT NOT NULL REFERENCES asiento(id),
  cuenta   TEXT NOT NULL,
  concepto TEXT NOT NULL DEFAULT '',
  debe     INTEGER NOT NULL DEFAULT 0 CHECK (debe >= 0),
  haber    INTEGER NOT NULL DEFAULT 0 CHECK (haber >= 0),
  tercero  TEXT NOT NULL DEFAULT '',
  CHECK ((debe = 0) <> (haber = 0))    -- a UN lado, y nunca a cero
);
CREATE INDEX IF NOT EXISTS apunte_asiento ON apunte(asiento, id);
CREATE TABLE IF NOT EXISTS cierre (
  id     INTEGER PRIMARY KEY AUTOINCREMENT,
  hasta  TEXT,                         -- última fecha CERRADA; NULL, abierto entero
  accion TEXT NOT NULL CHECK (accion IN ('cierre', 'reapertura')),
  motivo TEXT NOT NULL DEFAULT '',
  en     TEXT NOT NULL
);
`

// Asiento es un apunte doble tal como sale a la API (la forma de `store.Asiento`).
type Asiento struct {
	ID       string    `json:"id"`
	Diario   string    `json:"diario"`
	Ref      string    `json:"ref"`
	Fecha    time.Time `json:"fecha"`
	Concepto string    `json:"concepto"`
	Moneda   string    `json:"moneda"`
	Estado   string    `json:"estado"`
	OrigenID string    `json:"origen_id,omitempty"`
	Apuntes  []Apunte  `json:"apuntes,omitempty"`
}

// Apunte es una línea del asiento. Importes en céntimos.
type Apunte struct {
	Cuenta   string `json:"cuenta"`
	Concepto string `json:"concepto,omitempty"`
	Debe     int64  `json:"debe"`
	Haber    int64  `json:"haber"`
	Tercero  string `json:"tercero,omitempty"`
}

// SaldoCuenta es una fila del sumas y saldos.
type SaldoCuenta struct {
	Cuenta string `json:"cuenta"`
	Debe   int64  `json:"debe"`
	Haber  int64  `json:"haber"`
	Saldo  int64  `json:"saldo"`
}

// Cierre es un movimiento del libro de cierres.
type Cierre struct {
	Hasta  *time.Time `json:"hasta,omitempty"`
	Accion string     `json:"accion"`
	Motivo string     `json:"motivo,omitempty"`
	Por    string     `json:"por,omitempty"`
	En     time.Time  `json:"en"`
}

type consultor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// cierreVigente es la última fecha cerrada, o nil si el libro está abierto entero.
func cierreVigente(ctx context.Context, q consultor) (*time.Time, error) {
	var hasta sql.NullString
	err := q.QueryRowContext(ctx, `SELECT hasta FROM cierre ORDER BY id DESC LIMIT 1`).Scan(&hasta)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !hasta.Valid) {
		return nil, nil //nolint:nilnil // nil es «el libro está abierto entero» (ver arriba), no un fallo
	}
	if err != nil {
		return nil, err
	}
	d, err := time.Parse(time.DateOnly, hasta.String)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// exigirPeriodoAbierto falla si `fecha` cae en el período cerrado. Va DENTRO de la
// transacción que escribe, como en el servidor.
func exigirPeriodoAbierto(ctx context.Context, tx *sql.Tx, fecha time.Time) error {
	hasta, err := cierreVigente(ctx, tx)
	if err != nil || hasta == nil {
		return err
	}
	if !fecha.Truncate(24 * time.Hour).After(hasta.Truncate(24 * time.Hour)) {
		return ErrPeriodoCerrado
	}
	return nil
}

// escribirAsiento comprueba el período y el cuadre, numera y escribe el asiento con sus
// apuntes. Es el `escribirAsiento` del servidor: todo lo que entra al libro (un
// documento, el banco) pasa por aquí.
func (a *Almacen) escribirAsiento(ctx context.Context, tx *sql.Tx, diario string, fecha time.Time,
	concepto, moneda, origen string, apuntes []Apunte,
) (Asiento, error) {
	if err := exigirPeriodoAbierto(ctx, tx, fecha); err != nil {
		return Asiento{}, err
	}
	var debe, haber int64
	for _, ap := range apuntes {
		debe += ap.Debe
		haber += ap.Haber
	}
	if debe != haber || len(apuntes) < 2 {
		return Asiento{}, fmt.Errorf("asiento descuadrado: debe=%d haber=%d: %w", debe, haber, dominio.ErrDatos)
	}
	ref, err := reservarRef(ctx, tx, "asiento:"+diario, dominio.PrefijoDiario(diario), fmt.Sprintf("%04d", fecha.Year()))
	if err != nil {
		return Asiento{}, err
	}
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO asiento (id, diario, ref, fecha, concepto, moneda, estado, origen, creado)
		VALUES (?, ?, ?, ?, ?, ?, 'asentado', ?, ?)`, id, diario, ref, fecha.Format(time.DateOnly), concepto, moneda, origen, a.marcaAhora()); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") && origen != "" {
			return Asiento{}, ErrYaAsentado
		}
		return Asiento{}, err
	}
	for _, ap := range apuntes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO apunte (asiento, cuenta, concepto, debe, haber, tercero) VALUES (?, ?, ?, ?, ?, ?)`,
			id, ap.Cuenta, ap.Concepto, ap.Debe, ap.Haber, ap.Tercero); err != nil {
			return Asiento{}, err
		}
	}
	return Asiento{
		ID: id, Diario: diario, Ref: ref, Fecha: fecha.UTC(), Concepto: concepto, Moneda: moneda,
		Estado: "asentado", OrigenID: origen, Apuntes: apuntes,
	}, nil
}

func yaAsentado(ctx context.Context, tx *sql.Tx, origen string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM asiento WHERE origen = ? AND estado = 'asentado'`, origen).Scan(&n)
	return n > 0, err
}

// Asentar lleva un documento al libro: su asiento y, si su flujo lo contempla, el
// documento confirmado. Todo en una transacción.
func (a *Almacen) Asentar(ctx context.Context, m *catalogo.Modelo, id string) (Asiento, error) {
	if m.Contable == nil {
		return Asiento{}, fmt.Errorf("el modelo %q no lleva libro: %w", m.Clave, dominio.ErrDatos)
	}
	var out Asiento
	err := a.tx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = a.asentarEn(ctx, tx, m, id)
		return err
	})
	return out, err
}

func (a *Almacen) asentarEn(ctx context.Context, tx *sql.Tx, m *catalogo.Modelo, id string) (Asiento, error) {
	r, err := obtener(ctx, tx, m.Clave, id)
	if err != nil {
		return Asiento{}, err
	}
	if ya, err := yaAsentado(ctx, tx, id); err != nil {
		return Asiento{}, err
	} else if ya {
		return Asiento{}, ErrYaAsentado
	}
	etiqueta := r.Etiqueta
	if etiqueta == "" {
		etiqueta = r.Ref
	}
	d, err := dominio.AsientoDe(m, etiqueta, r.Data)
	if err != nil {
		return Asiento{}, err
	}
	apuntes := make([]Apunte, 0, len(d.Apuntes))
	for _, ap := range d.Apuntes {
		apuntes = append(apuntes, Apunte{Cuenta: ap.Cuenta, Concepto: ap.Concepto, Debe: ap.Debe, Haber: ap.Haber, Tercero: ap.Tercero})
	}
	out, err := a.escribirAsiento(ctx, tx, d.Diario, d.Fecha, d.Concepto, d.Moneda, id, apuntes)
	if err != nil {
		return Asiento{}, err
	}
	// El documento pasa a confirmado si su flujo lo contempla; una factura que se
	// emite así tiene que cuadrar antes (sin VERI*FACTU, D-29).
	if dominio.EstadoValido(m, "confirmado") {
		if err := a.cambiarEstado(ctx, tx, m.Clave, id, "confirmado"); err != nil {
			return Asiento{}, err
		}
	}
	return out, nil
}

// cuadrarAlEmitir es `cuadrarAlEmitir` del servidor sin VERI*FACTU: la factura de venta
// que se emite ahora tiene que cuadrar con sus líneas (dominio.CuadreFactura).
func (a *Almacen) cuadrarAlEmitir(ctx context.Context, tx *sql.Tx, previo api.ERPRegistro, estado string) error {
	if previo.Modelo != "factura_venta" || !dominio.Emitible(previo.Estado, previo.Data, estado) {
		return nil
	}
	lineas, err := a.lineasDeFactura(ctx, tx, previo.ID)
	if err != nil {
		return err
	}
	if _, err := dominio.CuadreFactura(previo.Data, lineas); err != nil {
		return fmt.Errorf("%s: %w", previo.Ref, err)
	}
	return nil
}

// lineasDeFactura son las líneas vivas de una factura, en el orden en que se crearon.
func (a *Almacen) lineasDeFactura(ctx context.Context, q consultor, facturaID string) ([]dominio.LineaFactura, error) {
	regs, err := a.listarTx(ctx, q, "linea_factura", false, "")
	if err != nil {
		return nil, err
	}
	var out []dominio.LineaFactura
	for i := len(regs) - 1; i >= 0; i-- {
		if regs[i].Data["factura"] != facturaID {
			continue
		}
		l, err := dominio.LineaDe(regs[i].Data)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func escanearAsientos(filas *sql.Rows) ([]Asiento, error) {
	defer func() { _ = filas.Close() }()
	var out []Asiento
	for filas.Next() {
		var as Asiento
		var fecha string
		if err := filas.Scan(&as.ID, &as.Diario, &as.Ref, &fecha, &as.Concepto, &as.Moneda, &as.Estado, &as.OrigenID); err != nil {
			return nil, err
		}
		as.Fecha, _ = time.Parse(time.DateOnly, fecha)
		out = append(out, as)
	}
	return out, filas.Err()
}

func (a *Almacen) conApuntes(ctx context.Context, as []Asiento) error {
	for i := range as {
		aps, err := a.apuntesDe(ctx, as[i].ID)
		if err != nil {
			return err
		}
		as[i].Apuntes = append(as[i].Apuntes, aps...)
	}
	return nil
}

// apuntesDe son los apuntes de un asiento, en su orden.
func (a *Almacen) apuntesDe(ctx context.Context, asiento string) ([]Apunte, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT cuenta, concepto, debe, haber, tercero FROM apunte WHERE asiento = ? ORDER BY id`, asiento)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	var out []Apunte
	for filas.Next() {
		var ap Apunte
		if err := filas.Scan(&ap.Cuenta, &ap.Concepto, &ap.Debe, &ap.Haber, &ap.Tercero); err != nil {
			return nil, err
		}
		out = append(out, ap)
	}
	return out, filas.Err()
}

const columnasAsiento = `id, diario, ref, fecha, concepto, moneda, estado, origen`

// Libro devuelve los asientos entre dos fechas (cero = sin acotar), lo último primero,
// con sus apuntes.
func (a *Almacen) Libro(ctx context.Context, desde, hasta time.Time, limite int) ([]Asiento, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	filas, err := a.db.QueryContext(ctx, `SELECT `+columnasAsiento+` FROM asiento
		WHERE (? = '' OR fecha >= ?) AND (? = '' OR fecha <= ?)
		ORDER BY fecha DESC, creado DESC, id DESC LIMIT ?`,
		fechaONada(desde), fechaONada(desde), fechaONada(hasta), fechaONada(hasta), limite)
	if err != nil {
		return nil, err
	}
	out, err := escanearAsientos(filas)
	if err != nil {
		return nil, err
	}
	return out, a.conApuntes(ctx, out)
}

func fechaONada(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateOnly)
}

// AsientosDe son los asientos de un documento de `modelo`, del primero al último.
func (a *Almacen) AsientosDe(ctx context.Context, modelo, id string) ([]Asiento, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT `+columnasAsiento+` FROM asiento
		WHERE origen = ? AND EXISTS (SELECT 1 FROM registro r WHERE r.id = ? AND r.modelo = ?)
		ORDER BY fecha, creado, id`, id, id, modelo)
	if err != nil {
		return nil, err
	}
	return escanearAsientos(filas)
}

// Anular marca un asiento asentado como anulado, con su motivo en el concepto. No
// borra ni reescribe apuntes; en un período cerrado, no se toca.
func (a *Almacen) Anular(ctx context.Context, id, motivo string) error {
	return a.tx(ctx, func(tx *sql.Tx) error {
		var fecha string
		err := tx.QueryRowContext(ctx, `SELECT fecha FROM asiento WHERE id = ?`, id).Scan(&fecha)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoEncontrado
		}
		if err != nil {
			return err
		}
		f, _ := time.Parse(time.DateOnly, fecha)
		if err := exigirPeriodoAbierto(ctx, tx, f); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE asiento SET estado = 'anulado', concepto = concepto || ' · ' || ? WHERE id = ? AND estado = 'asentado'`, motivo, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNoEncontrado
		}
		return nil
	})
}

// Balance es el sumas y saldos de lo ASENTADO entre `desde` y `hasta` (cero = sin acotar).
func (a *Almacen) Balance(ctx context.Context, desde, hasta time.Time) ([]SaldoCuenta, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT l.cuenta, sum(l.debe), sum(l.haber), sum(l.debe) - sum(l.haber)
		FROM apunte l JOIN asiento m ON m.id = l.asiento
		WHERE m.estado = 'asentado' AND (? = '' OR m.fecha >= ?) AND (? = '' OR m.fecha <= ?)
		GROUP BY l.cuenta ORDER BY l.cuenta`, fechaONada(desde), fechaONada(desde), fechaONada(hasta), fechaONada(hasta))
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []SaldoCuenta{}
	for filas.Next() {
		var c SaldoCuenta
		if err := filas.Scan(&c.Cuenta, &c.Debe, &c.Haber, &c.Saldo); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, filas.Err()
}

// CierreVigente dice hasta cuándo está cerrado el libro.
func (a *Almacen) CierreVigente(ctx context.Context) (*time.Time, error) {
	return cierreVigente(ctx, a.db)
}

// HistorialDeCierres son los cierres y reaperturas, el último primero.
func (a *Almacen) HistorialDeCierres(ctx context.Context) ([]Cierre, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT hasta, accion, motivo, en FROM cierre ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []Cierre{}
	for filas.Next() {
		var c Cierre
		var hasta sql.NullString
		var en string
		if err := filas.Scan(&hasta, &c.Accion, &c.Motivo, &en); err != nil {
			return nil, err
		}
		if hasta.Valid {
			d, _ := time.Parse(time.DateOnly, hasta.String)
			c.Hasta = &d
		}
		c.Por = Persona
		c.En, _ = time.Parse(marca, en)
		out = append(out, c)
	}
	return out, filas.Err()
}

// CerrarPeriodo mueve la fecha de corte hacia adelante; hacia atrás es ReabrirPeriodo.
func (a *Almacen) CerrarPeriodo(ctx context.Context, hasta time.Time, motivo string) error {
	return a.tx(ctx, func(tx *sql.Tx) error {
		actual, err := cierreVigente(ctx, tx)
		if err != nil {
			return err
		}
		if actual != nil && !hasta.After(*actual) {
			return ErrCierreHaciaAtras
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO cierre (hasta, accion, motivo, en) VALUES (?, 'cierre', ?, ?)`,
			hasta.Format(time.DateOnly), motivo, a.marcaAhora())
		return err
	})
}

// ReabrirPeriodo retrocede la fecha de corte, o abre el libro entero (hasta = nil).
func (a *Almacen) ReabrirPeriodo(ctx context.Context, hasta *time.Time, motivo string) error {
	var h any
	if hasta != nil {
		h = hasta.Format(time.DateOnly)
	}
	_, err := a.db.ExecContext(ctx, `INSERT INTO cierre (hasta, accion, motivo, en) VALUES (?, 'reapertura', ?, ?)`, h, motivo, a.marcaAhora())
	return err
}

// resumenContable son las cifras del libro para la portada.
func (a *Almacen) resumenContable(ctx context.Context) (asientos, debe, haber int64, err error) {
	err = a.db.QueryRowContext(ctx, `SELECT count(DISTINCT m.id), COALESCE(sum(l.debe), 0), COALESCE(sum(l.haber), 0)
		FROM asiento m LEFT JOIN apunte l ON l.asiento = m.id WHERE m.estado = 'asentado'`).Scan(&asientos, &debe, &haber)
	return asientos, debe, haber, err
}
