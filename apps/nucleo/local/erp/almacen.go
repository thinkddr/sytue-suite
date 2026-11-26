// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3" // el controlador de SQLite

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/dominio"
)

var (
	// ErrNoEncontrado: el registro no existe (o no está donde se busca: vivo o en la papelera).
	ErrNoEncontrado = errors.New("erp: no encontrado")
	// ErrDatos: lo enviado no vale (una relación rota, un estado de partida equivocado).
	ErrDatos = errors.New("erp: datos inválidos")
)

// Fichero es el nombre de la base dentro de la carpeta del producto.
const Fichero = "erp.sqlite"

// marca es el formato de las fechas guardadas: ANCHO FIJO, para que ordenar el texto
// sea ordenar el tiempo.
const marca = "2006-01-02T15:04:05.000000000Z"

const esquema = `
CREATE TABLE IF NOT EXISTS meta (clave TEXT PRIMARY KEY, valor TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS registro (
  id       TEXT PRIMARY KEY,
  modelo   TEXT NOT NULL,
  ref      TEXT NOT NULL DEFAULT '',
  etiqueta TEXT NOT NULL DEFAULT '',
  estado   TEXT NOT NULL DEFAULT '',
  data     TEXT NOT NULL,              -- los campos del registro, JSON legible
  creado   TEXT NOT NULL,
  editado  TEXT NOT NULL,
  borrado  TEXT                        -- en la papelera desde entonces; NULL, vivo
);
CREATE INDEX IF NOT EXISTS registro_modelo ON registro(modelo, borrado, creado);
CREATE UNIQUE INDEX IF NOT EXISTS registro_ref ON registro(modelo, ref) WHERE ref <> '';
CREATE TABLE IF NOT EXISTS secuencia (clave TEXT NOT NULL, periodo TEXT NOT NULL, ultimo INTEGER NOT NULL,
  PRIMARY KEY (clave, periodo));
CREATE TABLE IF NOT EXISTS modulo (clave TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS organizacion (id INTEGER PRIMARY KEY CHECK (id = 1), data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS historial (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  modelo   TEXT NOT NULL,
  registro TEXT NOT NULL,
  tipo     TEXT NOT NULL,              -- creado, cambio, borrado, restaurado, nota
  cambios  TEXT NOT NULL DEFAULT '',
  texto    TEXT NOT NULL DEFAULT '',
  creado   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS historial_registro ON historial(registro, id);
INSERT OR IGNORE INTO meta (clave, valor) VALUES ('version', '1');
`

// Almacen es el ERP de una persona en un fichero SQLite. Un solo escritor.
type Almacen struct {
	db  *sql.DB
	cat *catalogo.Catalogo
	// dir es la carpeta del producto: la base y los ficheros de los adjuntos.
	dir string
	// ahora es el reloj; los tests lo fijan.
	ahora func() time.Time
}

// Abrir abre (o crea) la base en `dir`.
func Abrir(dir string, cat *catalogo.Catalogo) (*Almacen, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.Join(dir, Fichero) + "?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate&_synchronous=FULL"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	// Al abrir no hay aún nada que cancelar: el esquema se crea entero o no se abre.
	if _, err := db.ExecContext(context.Background(), esquema+esquemaLibro+esquemaSeguimiento+esquemaImportacion+esquemaInformes+esquemaObligaciones); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("erp: preparar la base: %w", err)
	}
	return &Almacen{db: db, cat: cat, dir: dir, ahora: time.Now}, nil
}

// Cerrar cierra la base.
func (a *Almacen) Cerrar() error { return a.db.Close() }

// Ruta es el fichero de la base.
func (a *Almacen) Ruta() string { return filepath.Join(a.dir, Fichero) }

// CopiarEnCaliente deja en destino una copia coherente de la base abierta (`VACUUM
// INTO`), sin cerrarla ni parar la app: la copia de seguridad (`copia.BaseAbierta`).
func (a *Almacen) CopiarEnCaliente(ctx context.Context, destino string) error {
	_, err := a.db.ExecContext(ctx, "VACUUM INTO ?", destino)
	return err
}

func (a *Almacen) marcaAhora() string { return a.ahora().UTC().Format(marca) }

func (a *Almacen) tx(ctx context.Context, f func(*sql.Tx) error) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// --- Módulos y organización ------------------------------------------------------

// ModulosActivos son los módulos encendidos, en orden. Nunca nil.
func (a *Almacen) ModulosActivos(ctx context.Context) ([]string, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT clave FROM modulo ORDER BY clave`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []string{}
	for filas.Next() {
		var c string
		if err := filas.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, filas.Err()
}

// ActivarModulo enciende o apaga un módulo.
func (a *Almacen) ActivarModulo(ctx context.Context, clave string, activo bool) error {
	q := `INSERT OR IGNORE INTO modulo (clave) VALUES (?)`
	if !activo {
		q = `DELETE FROM modulo WHERE clave = ?`
	}
	_, err := a.db.ExecContext(ctx, q, clave)
	return err
}

// Organizacion es el perfil; ErrNoEncontrado hasta que se guarda el primero.
func (a *Almacen) Organizacion(ctx context.Context) (api.ERPOrganizacion, error) {
	var crudo string
	var o api.ERPOrganizacion
	err := a.db.QueryRowContext(ctx, `SELECT data FROM organizacion WHERE id = 1`).Scan(&crudo)
	if errors.Is(err, sql.ErrNoRows) {
		return o, ErrNoEncontrado
	}
	if err != nil {
		return o, err
	}
	// En dos pasos: en `return o, json.Unmarshal(…, &o)` el orden en que se leen los dos
	// valores no está garantizado, y podría salir el perfil vacío.
	err = json.Unmarshal([]byte(crudo), &o)
	return o, err
}

// GuardarOrganizacion crea o sustituye el perfil.
func (a *Almacen) GuardarOrganizacion(ctx context.Context, o api.ERPOrganizacion) error {
	crudo, err := json.Marshal(o)
	if err != nil {
		return err
	}
	_, err = a.db.ExecContext(ctx, `INSERT INTO organizacion (id, data) VALUES (1, ?)
		ON CONFLICT (id) DO UPDATE SET data = excluded.data`, string(crudo))
	return err
}

// --- Registros -------------------------------------------------------------------

const columnas = `id, modelo, ref, etiqueta, estado, data, creado, editado`

type escaner interface{ Scan(...any) error }

func escanear(f escaner) (api.ERPRegistro, error) {
	var r api.ERPRegistro
	var data, creado, editado string
	if err := f.Scan(&r.ID, &r.Modelo, &r.Ref, &r.Etiqueta, &r.Estado, &data, &creado, &editado); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r, ErrNoEncontrado
		}
		return r, err
	}
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&r.Data); err != nil {
		return r, err
	}
	r.CreadoEn, _ = time.Parse(marca, creado)
	r.EditadoEn, _ = time.Parse(marca, editado)
	return r, nil
}

// obtener es un registro vivo (fuera de la papelera).
func obtener(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, modelo, id string,
) (api.ERPRegistro, error) {
	return escanear(q.QueryRowContext(ctx, `SELECT `+columnas+` FROM registro WHERE id = ? AND modelo = ? AND borrado IS NULL`, id, modelo))
}

// Obtener devuelve un registro vivo.
func (a *Almacen) Obtener(ctx context.Context, modelo, id string) (api.ERPRegistro, error) {
	return obtener(ctx, a.db, modelo, id)
}

// comprobarRelaciones exige que cada relación apunte a un registro vivo del modelo que
// declara el campo.
func comprobarRelaciones(ctx context.Context, tx *sql.Tx, m *catalogo.Modelo, data map[string]any) error {
	for _, f := range m.Campos {
		id, ok := data[f.Clave].(string)
		if f.Tipo != catalogo.TipoRelacion || !ok || id == "" {
			continue
		}
		var modelo string
		err := tx.QueryRowContext(ctx, `SELECT modelo FROM registro WHERE id = ? AND borrado IS NULL`, id).Scan(&modelo)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s: el registro referenciado no existe", ErrDatos, f.Clave)
		}
		if err != nil {
			return err
		}
		if modelo != f.Modelo {
			return fmt.Errorf("%w: %s: se esperaba un registro de %q y es de %q", ErrDatos, f.Clave, f.Modelo, modelo)
		}
	}
	return nil
}

// periodoDe es el año con el que se numera: el de la fecha del documento.
func (a *Almacen) periodoDe(data map[string]any) string {
	for _, clave := range []string{"fecha", "fecha_inicio"} {
		if s, ok := data[clave].(string); ok && len(s) >= 10 {
			if _, err := time.Parse(time.DateOnly, s[:10]); err == nil {
				return s[:4]
			}
		}
	}
	return fmt.Sprintf("%04d", a.ahora().UTC().Year())
}

func reservarRef(ctx context.Context, tx *sql.Tx, clave, prefijo, periodo string) (string, error) {
	var n int64
	err := tx.QueryRowContext(ctx, `INSERT INTO secuencia (clave, periodo, ultimo) VALUES (?, ?, 1)
		ON CONFLICT (clave, periodo) DO UPDATE SET ultimo = ultimo + 1 RETURNING ultimo`, clave, periodo).Scan(&n)
	if err != nil {
		return "", fmt.Errorf("reservar número de %s: %w", prefijo, err)
	}
	return fmt.Sprintf("%s-%s-%05d", prefijo, periodo, n), nil
}

func anotar(ctx context.Context, tx *sql.Tx, modelo, id, tipo string, cambios map[string]api.ERPCambio, texto, cuando string) error {
	c := ""
	if len(cambios) > 0 {
		b, err := json.Marshal(cambios)
		if err != nil {
			return err
		}
		c = string(b)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO historial (modelo, registro, tipo, cambios, texto, creado) VALUES (?, ?, ?, ?, ?, ?)`,
		modelo, id, tipo, c, texto, cuando)
	return err
}

func (a *Almacen) insertar(ctx context.Context, tx *sql.Tx, m *catalogo.Modelo, data map[string]any, etiqueta string) (api.ERPRegistro, error) {
	return a.insertarEn(ctx, tx, m, data, etiqueta, "")
}

// insertarEn inserta con un estado de partida que no es el primero del flujo (el banco
// crea los extractos ya confirmados); vacío, el primero.
func (a *Almacen) insertarEn(ctx context.Context, tx *sql.Tx, m *catalogo.Modelo, data map[string]any, etiqueta, estadoInicial string) (api.ERPRegistro, error) {
	if err := comprobarRelaciones(ctx, tx, m, data); err != nil {
		return api.ERPRegistro{}, err
	}
	ref := ""
	if m.Secuencia != "" {
		var err error
		if ref, err = reservarRef(ctx, tx, "doc:"+m.Clave, m.Secuencia, a.periodoDe(data)); err != nil {
			return api.ERPRegistro{}, err
		}
	}
	crudo, err := json.Marshal(data)
	if err != nil {
		return api.ERPRegistro{}, err
	}
	estado := estadoInicial
	if estado == "" && len(m.Estados) > 0 {
		estado = m.Estados[0]
	}
	id, ahora := uuid.NewString(), a.marcaAhora()
	if _, err := tx.ExecContext(ctx, `INSERT INTO registro (`+columnas+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, m.Clave, ref, etiqueta, estado, string(crudo), ahora, ahora); err != nil {
		return api.ERPRegistro{}, err
	}
	if err := anotar(ctx, tx, m.Clave, id, "creado", nil, "", ahora); err != nil {
		return api.ERPRegistro{}, err
	}
	return obtener(ctx, tx, m.Clave, id)
}

// Crear inserta un registro ya validado: numera, pone el estado inicial y lo anota.
func (a *Almacen) Crear(ctx context.Context, m *catalogo.Modelo, data map[string]any, etiqueta string) (api.ERPRegistro, error) {
	var r api.ERPRegistro
	err := a.tx(ctx, func(tx *sql.Tx) error {
		var err error
		r, err = a.insertar(ctx, tx, m, data, etiqueta)
		return err
	})
	return r, err
}

// mismo compara dos valores tal y como se guardan (JSON).
func mismo(x, y any) bool {
	bx, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Equal(bx, by)
}

// Actualizar REEMPLAZA los datos; no toca ni la referencia ni el estado.
func (a *Almacen) Actualizar(ctx context.Context, m *catalogo.Modelo, id string, data map[string]any, etiqueta string) (api.ERPRegistro, error) {
	var r api.ERPRegistro
	err := a.tx(ctx, func(tx *sql.Tx) error {
		previo, err := obtener(ctx, tx, m.Clave, id)
		if err != nil {
			return err
		}
		if err := comprobarRelaciones(ctx, tx, m, data); err != nil {
			return err
		}
		cambios := map[string]api.ERPCambio{}
		for _, f := range m.Campos {
			antes, despues := previo.Data[f.Clave], data[f.Clave]
			if !mismo(antes, despues) {
				cambios[f.Clave] = api.ERPCambio{Antes: antes, Despues: despues}
			}
		}
		crudo, err := json.Marshal(data)
		if err != nil {
			return err
		}
		ahora := a.marcaAhora()
		if _, err := tx.ExecContext(ctx, `UPDATE registro SET data = ?, etiqueta = CASE WHEN ? <> '' THEN ? ELSE etiqueta END, editado = ?
			WHERE id = ? AND modelo = ? AND borrado IS NULL`, string(crudo), etiqueta, etiqueta, ahora, id, m.Clave); err != nil {
			return err
		}
		if len(cambios) > 0 {
			if err := anotar(ctx, tx, m.Clave, id, "cambio", cambios, "", ahora); err != nil {
				return err
			}
		}
		r, err = obtener(ctx, tx, m.Clave, id)
		return err
	})
	return r, err
}

func (a *Almacen) cambiarEstado(ctx context.Context, tx *sql.Tx, modelo, id, estado string) error {
	previo, err := obtener(ctx, tx, modelo, id)
	if err != nil {
		return err
	}
	if previo.Estado == estado {
		return nil
	}
	if err := a.cuadrarAlEmitir(ctx, tx, previo, estado); err != nil {
		return err
	}
	ahora := a.marcaAhora()
	if _, err := tx.ExecContext(ctx, `UPDATE registro SET estado = ?, editado = ? WHERE id = ?`, estado, ahora, id); err != nil {
		return err
	}
	return anotar(ctx, tx, modelo, id, "cambio", map[string]api.ERPCambio{"estado": {Antes: previo.Estado, Despues: estado}}, "", ahora)
}

// CambiarEstado mueve el documento dentro de su flujo (quien llama ha comprobado que
// el estado es del modelo). Una factura de venta que se emite tiene que cuadrar con sus
// líneas; sin VERI*FACTU (D-29). Cambiar el estado no asienta: eso es `Asentar`.
func (a *Almacen) CambiarEstado(ctx context.Context, modelo, id, estado string) (api.ERPRegistro, error) {
	var r api.ERPRegistro
	err := a.tx(ctx, func(tx *sql.Tx) error {
		if err := a.cambiarEstado(ctx, tx, modelo, id, estado); err != nil {
			return err
		}
		var err error
		r, err = obtener(ctx, tx, modelo, id)
		return err
	})
	return r, err
}

func (a *Almacen) papelera(ctx context.Context, tx *sql.Tx, modelo, id string, borrar bool) error {
	ahora := a.marcaAhora()
	q, tipo := `UPDATE registro SET borrado = ? WHERE id = ? AND modelo = ? AND borrado IS NULL`, "borrado"
	args := []any{ahora, id, modelo}
	if !borrar {
		q, tipo = `UPDATE registro SET borrado = NULL, editado = ? WHERE id = ? AND modelo = ? AND borrado IS NOT NULL`, "restaurado"
	}
	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoEncontrado
	}
	return anotar(ctx, tx, modelo, id, tipo, nil, "", ahora)
}

// Borrar manda el registro a la papelera.
func (a *Almacen) Borrar(ctx context.Context, modelo, id string) error {
	return a.tx(ctx, func(tx *sql.Tx) error { return a.papelera(ctx, tx, modelo, id, true) })
}

// Restaurar lo saca de la papelera.
func (a *Almacen) Restaurar(ctx context.Context, modelo, id string) (api.ERPRegistro, error) {
	var r api.ERPRegistro
	err := a.tx(ctx, func(tx *sql.Tx) error {
		if err := a.papelera(ctx, tx, modelo, id, false); err != nil {
			return err
		}
		var err error
		r, err = obtener(ctx, tx, modelo, id)
		return err
	})
	return r, err
}

// Lote aplica «estado», «borrar» o «restaurar» a varios registros y dice cuántos tocó.
func (a *Almacen) Lote(ctx context.Context, modelo, accion string, ids []string, estado string) (int, error) {
	n := 0
	err := a.tx(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			var err error
			switch accion {
			case "estado":
				err = a.cambiarEstado(ctx, tx, modelo, id, estado)
			case "borrar":
				err = a.papelera(ctx, tx, modelo, id, true)
			case "restaurar":
				err = a.papelera(ctx, tx, modelo, id, false)
			}
			if errors.Is(err, ErrNoEncontrado) {
				continue
			}
			if err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// Duplicar clona el registro como borrador, con referencia nueva.
func (a *Almacen) Duplicar(ctx context.Context, m *catalogo.Modelo, id string) (api.ERPRegistro, error) {
	var r api.ERPRegistro
	err := a.tx(ctx, func(tx *sql.Tx) error {
		orig, err := obtener(ctx, tx, m.Clave, id)
		if err != nil {
			return err
		}
		copia := make(map[string]any, len(orig.Data))
		for k, v := range orig.Data {
			copia[k] = v
		}
		for _, k := range []string{"ref", "externo_id", "tpv_operacion", "tpv_huella", "sesion", "medio_pago"} {
			delete(copia, k)
		}
		etiqueta := orig.Etiqueta
		if etiqueta != "" && !strings.HasSuffix(etiqueta, " (copia)") {
			etiqueta += " (copia)"
		}
		r, err = a.insertar(ctx, tx, m, copia, etiqueta)
		return err
	})
	return r, err
}

// Convertir lleva el presupuesto aceptado a pedido y el pedido confirmado o entregado
// a factura, con sus líneas, como `store.ConvertirVenta`. Si ya se convirtió, devuelve
// lo que salió entonces.
func (a *Almacen) Convertir(ctx context.Context, modelo, id string) (api.ERPRegistro, error) {
	destino, campo, estado := "pedido_venta", "presupuesto", "aceptado"
	lineaOrigen, campoOrigen, lineaDestino, campoDestino := "linea_presupuesto", "presupuesto", "linea_venta", "pedido"
	switch modelo {
	case "presupuesto":
	case "pedido_venta":
		destino, campo, estado = "factura_venta", "pedido", "confirmado"
		lineaOrigen, campoOrigen, lineaDestino, campoDestino = "linea_venta", "pedido", "linea_factura", "factura"
	default:
		return api.ERPRegistro{}, fmt.Errorf("%w: solo se convierten presupuestos y pedidos", ErrDatos)
	}
	m, ok := a.cat.Modelo(destino)
	ml, okl := a.cat.Modelo(lineaDestino)
	if !ok || !okl {
		return api.ERPRegistro{}, ErrNoEncontrado
	}
	var out api.ERPRegistro
	err := a.tx(ctx, func(tx *sql.Tx) error {
		origen, err := obtener(ctx, tx, modelo, id)
		if err != nil {
			return err
		}
		hechos, err := a.listarTx(ctx, tx, destino, false, "")
		if err != nil {
			return err
		}
		for _, h := range hechos {
			if h.Data[campo] == id {
				out = h
				return nil
			}
		}
		if origen.Estado != estado && (modelo != "pedido_venta" || origen.Estado != "entregado") {
			return fmt.Errorf("%w: el documento tiene que estar %s antes de convertirlo", ErrDatos, estado)
		}
		data := map[string]any{campo: id, "fecha": a.ahora().Format(time.DateOnly)}
		for _, k := range []string{"cliente", "base", "iva", "total", "notas"} {
			if v, ok := origen.Data[k]; ok {
				data[k] = v
			}
		}
		if out, err = a.insertarValidado(ctx, tx, m, data); err != nil {
			return err
		}
		grupos, err := a.copiarLineasVenta(ctx, tx, lineaOrigen, campoOrigen, id, ml, campoDestino, out.ID)
		if err != nil {
			return err
		}
		return dominio.ComprobarImportesVenta(origen.Data, grupos)
	})
	return out, err
}

// insertarValidado pasa los datos por dominio.Validar antes de insertarlos, como hace
// el servidor con lo que crea por su cuenta (una conversión, una importación).
func (a *Almacen) insertarValidado(ctx context.Context, tx *sql.Tx, m *catalogo.Modelo, data map[string]any) (api.ERPRegistro, error) {
	crudo, err := json.Marshal(data)
	if err != nil {
		return api.ERPRegistro{}, err
	}
	validado, etiqueta, err := dominio.Validar(m, crudo)
	if err != nil {
		return api.ERPRegistro{}, fmt.Errorf("%w: %w", dominio.ErrDatos, err)
	}
	return a.insertar(ctx, tx, m, validado, etiqueta)
}

// copiarLineasVenta es `copiarLineasVenta` del servidor: conserva el detalle y los
// tipos de IVA, sin inventar un IVA medio, y devuelve el desglose de lo copiado.
func (a *Almacen) copiarLineasVenta(ctx context.Context, tx *sql.Tx, modeloOrigen, campoOrigen, id string,
	ml *catalogo.Modelo, campoDestino, destinoID string,
) ([]dominio.GrupoIVA, error) {
	lineas, err := a.listarTx(ctx, tx, modeloOrigen, false, "")
	if err != nil {
		return nil, err
	}
	// listarTx da lo último primero; las líneas van en el orden en que se crearon.
	slices.Reverse(lineas)
	var fiscales []dominio.LineaFactura
	for _, reg := range lineas {
		l := reg.Data
		if l[campoOrigen] != id {
			continue
		}
		nombre, _ := l["nombre"].(string)
		precio, tienePrecio := l["precio_con_iva"]
		tipo, _ := l["tipo_iva"].(string)
		if ml.Clave == "linea_factura" && (!tienePrecio || tipo == "") {
			return nil, fmt.Errorf("completa precio con IVA y tipo de IVA de la línea %s antes de facturar: %w", nombre, dominio.ErrDatos)
		}
		fiscal, err := dominio.LineaDe(map[string]any{"descripcion": nombre, "cantidad": l["cantidad"], "precio_con_iva": precio, "tipo_iva": tipo})
		if err != nil {
			return nil, err
		}
		fiscales = append(fiscales, fiscal)
		data := map[string]any{campoDestino: destinoID, "cantidad": l["cantidad"]}
		if ml.Clave == "linea_factura" {
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
		if _, err := a.insertarValidado(ctx, tx, ml, data); err != nil {
			return nil, err
		}
	}
	return dominio.Desglose(fiscales), nil
}

// listarTx devuelve los registros de un modelo, lo último primero.
func (a *Almacen) listarTx(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, modelo string, papelera bool, estado string,
) ([]api.ERPRegistro, error) {
	cond := "borrado IS NULL"
	if papelera {
		cond = "borrado IS NOT NULL"
	}
	filas, err := q.QueryContext(ctx, `SELECT `+columnas+` FROM registro WHERE modelo = ? AND `+cond+
		` AND (? = '' OR estado = ?) ORDER BY creado DESC, id DESC`, modelo, estado, estado)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	var out []api.ERPRegistro
	for filas.Next() {
		r, err := escanear(filas)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, filas.Err()
}

// plegar es para buscar: minúsculas y sin tildes.
func plegar(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'á', 'à', 'ä', 'â':
			return 'a'
		case 'é', 'è', 'ë', 'ê':
			return 'e'
		case 'í', 'ì', 'ï', 'î':
			return 'i'
		case 'ó', 'ò', 'ö', 'ô':
			return 'o'
		case 'ú', 'ù', 'ü', 'û':
			return 'u'
		}
		return r
	}, strings.ToLower(s))
}

func coincide(r api.ERPRegistro, q string) bool {
	q = plegar(strings.TrimSpace(q))
	return q == "" || strings.Contains(plegar(r.Etiqueta), q) || strings.Contains(plegar(r.Ref), q)
}

// Listar devuelve una página con los filtros de la pantalla. La paginación es por
// desplazamiento: el cursor es opaco por fuera, como en el servidor.
//
// Filtra y ordena en memoria sobre las filas del modelo: es una base de una persona.
// Si una lista de decenas de miles de filas lo pide, se mide antes de indexar (R-MEDIR).
func (a *Almacen) Listar(ctx context.Context, m *catalogo.Modelo, f api.ERPFiltro) (api.ERPPagina, error) {
	todas, err := a.listarTx(ctx, a.db, m.Clave, f.Papelera, f.Estado)
	if err != nil {
		return api.ERPPagina{}, err
	}
	filas := todas[:0]
	for _, r := range todas {
		if coincide(r, f.Q) {
			filas = append(filas, r)
		}
	}
	if f.Orden != "" && f.Orden != "creado" {
		tipo := ""
		for _, c := range m.Campos {
			if c.Clave == f.Orden {
				tipo = c.Tipo
			}
		}
		valor := func(r api.ERPRegistro) any {
			switch f.Orden {
			case "etiqueta":
				return r.Etiqueta
			case "ref":
				return r.Ref
			case "estado":
				return r.Estado
			}
			return r.Data[f.Orden]
		}
		slices.SortStableFunc(filas, func(x, y api.ERPRegistro) int {
			c := comparar(valor(x), valor(y), tipo)
			if !f.Asc {
				c = -c
			}
			return c
		})
	} else if f.Asc {
		slices.Reverse(filas)
	}
	limite := f.Limite
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	desde, _ := strconv.Atoi(f.Cursor)
	desde = min(max(desde, 0), len(filas))
	hasta := min(desde+limite, len(filas))
	pag := api.ERPPagina{Registros: slices.Clone(filas[desde:hasta])}
	if pag.Registros == nil {
		pag.Registros = []api.ERPRegistro{}
	}
	if hasta < len(filas) {
		pag.Siguiente = strconv.Itoa(hasta)
	}
	pag.Etiquetas, err = a.etiquetas(ctx, m, pag.Registros)
	return pag, err
}

func comparar(x, y any, tipo string) int {
	if tipo == catalogo.TipoDinero || tipo == catalogo.TipoEntero || tipo == catalogo.TipoDecimal {
		return cmp.Compare(numero(x), numero(y))
	}
	sx, _ := x.(string)
	sy, _ := y.(string)
	if tipo == catalogo.TipoBooleano {
		sx, sy = fmt.Sprint(x), fmt.Sprint(y)
	}
	return cmp.Compare(plegar(sx), plegar(sy))
}

func numero(v any) float64 {
	switch n := v.(type) {
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	case int64:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

func entero(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}

// etiquetas resuelve el nombre de las relaciones de la página, de una vez.
func (a *Almacen) etiquetas(ctx context.Context, m *catalogo.Modelo, regs []api.ERPRegistro) (map[string]string, error) {
	out := map[string]string{}
	for _, r := range regs {
		for _, f := range m.Campos {
			id, ok := r.Data[f.Clave].(string)
			if f.Tipo != catalogo.TipoRelacion || !ok || id == "" {
				continue
			}
			if _, hecho := out[id]; hecho {
				continue
			}
			var etiqueta, ref string
			err := a.db.QueryRowContext(ctx, `SELECT etiqueta, ref FROM registro WHERE id = ? AND borrado IS NULL`, id).Scan(&etiqueta, &ref)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out[id] = cmp.Or(etiqueta, ref)
		}
	}
	return out, nil
}

// Buscar busca por nombre o referencia en todos los modelos.
func (a *Almacen) Buscar(ctx context.Context, q string, limite int) ([]api.ERPRegistro, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT `+columnas+` FROM registro WHERE borrado IS NULL ORDER BY editado DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []api.ERPRegistro{}
	for filas.Next() && len(out) < limite {
		r, err := escanear(filas)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(q) != "" && coincide(r, q) {
			out = append(out, r)
		}
	}
	return out, filas.Err()
}

// Estadisticas cuenta por estado y suma el primer importe de la lista.
func (a *Almacen) Estadisticas(ctx context.Context, m *catalogo.Modelo) (api.ERPEstadisticas, error) {
	st := api.ERPEstadisticas{PorEstado: map[string]int{}}
	campo := ""
	for _, col := range m.Lista {
		for _, c := range m.Campos {
			if campo == "" && c.Clave == col && c.Tipo == catalogo.TipoDinero {
				campo = col
			}
		}
	}
	regs, err := a.listarTx(ctx, a.db, m.Clave, false, "")
	if err != nil {
		return st, err
	}
	for _, r := range regs {
		st.Total++
		st.PorEstado[r.Estado]++
		if campo != "" {
			st.SumaDinero += entero(r.Data[campo])
		}
	}
	return st, nil
}

// Resumen son los indicadores de la portada: la facturación de clientes y lo asentado
// en el libro.
func (a *Almacen) Resumen(ctx context.Context) (api.ERPResumen, error) {
	var res api.ERPResumen
	var haber int64
	var err error
	if res.Contabilidad.TotalAsientos, res.Contabilidad.TotalDebe, haber, err = a.resumenContable(ctx); err != nil {
		return res, err
	}
	res.Contabilidad.Cuadra = res.Contabilidad.TotalDebe == haber
	regs, err := a.listarTx(ctx, a.db, "factura_venta", false, "")
	if err != nil {
		return res, err
	}
	f := &res.Facturacion
	for _, r := range regs {
		if r.Estado == "anulado" {
			continue
		}
		total := entero(r.Data["total"])
		f.TotalCentimos += total
		f.TotalRegistros++
		if r.Estado == "confirmado" || r.Estado == "emitida" || r.Estado == "pendiente" {
			f.PendienteCobroCentimos += total
			f.FacturasPendientes++
			if v, _ := r.Data["vencimiento"].(string); v != "" && (f.ProximoVencimiento == "" || v < f.ProximoVencimiento) {
				f.ProximoVencimiento = v
			}
		}
	}
	return res, nil
}

// Historial es lo que ha pasado con el registro, lo último primero.
func (a *Almacen) Historial(ctx context.Context, id, nombre string) ([]api.ERPEntradaHistorial, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT id, tipo, cambios, texto, creado FROM historial WHERE registro = ? ORDER BY id DESC LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []api.ERPEntradaHistorial{}
	for filas.Next() {
		var e api.ERPEntradaHistorial
		var n int64
		var cambios, creado string
		if err := filas.Scan(&n, &e.Tipo, &cambios, &e.Texto, &creado); err != nil {
			return nil, err
		}
		e.ID, e.Sub, e.Nombre = strconv.FormatInt(n, 10), Persona, nombre
		e.CreadoEn, _ = time.Parse(marca, creado)
		if cambios != "" {
			dec := json.NewDecoder(strings.NewReader(cambios))
			dec.UseNumber()
			if err := dec.Decode(&e.Cambios); err != nil {
				return nil, err
			}
		}
		out = append(out, e)
	}
	return out, filas.Err()
}

// Anotar deja una nota en un registro vivo.
func (a *Almacen) Anotar(ctx context.Context, modelo, id, texto string) error {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return fmt.Errorf("%w: la nota está vacía", ErrDatos)
	}
	return a.tx(ctx, func(tx *sql.Tx) error {
		if _, err := obtener(ctx, tx, modelo, id); err != nil {
			return err
		}
		return anotar(ctx, tx, modelo, id, "nota", nil, texto, a.marcaAhora())
	})
}

// BuscarPorEtiqueta es el id del registro vivo de `modelo` que se llama así (por su
// nombre o su referencia, sin distinguir mayúsculas ni tildes); el más antiguo si hay
// varios. Es lo que resuelve una relación escrita por su nombre al importar.
func (a *Almacen) BuscarPorEtiqueta(ctx context.Context, modelo, texto string) (string, error) {
	regs, err := a.listarTx(ctx, a.db, modelo, false, "")
	if err != nil {
		return "", err
	}
	q := plegar(strings.TrimSpace(texto))
	for i := len(regs) - 1; i >= 0; i-- {
		if plegar(regs[i].Etiqueta) == q || plegar(regs[i].Ref) == q {
			return regs[i].ID, nil
		}
	}
	return "", ErrNoEncontrado
}
