// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // el motor de SQLite del índice

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// El índice es UNO para todas las cuentas (`<Datos>/Correo/indice.sqlite`): las
// bandejas unificadas y la búsqueda cruzan cuentas, y con una base por cuenta cada
// consulta tendría que juntar varias. Cada fila lleva su cuenta.
const esquema = `
CREATE TABLE IF NOT EXISTS carpetas (
	cuenta TEXT NOT NULL,
	nombre TEXT NOT NULL,
	rol TEXT NOT NULL DEFAULT '',
	uidvalidity INTEGER NOT NULL DEFAULT 0,
	ultimo INTEGER NOT NULL DEFAULT 0,
	-- local: carpeta que solo existe aquí (los borradores de una cuenta sin «Drafts»).
	local INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (cuenta, nombre)
);
CREATE TABLE IF NOT EXISTS mensajes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cuenta TEXT NOT NULL,
	carpeta TEXT NOT NULL,
	-- uid 0: todavía no está en el servidor (borrador, enviado, movido sin UIDPLUS).
	uid INTEGER NOT NULL DEFAULT 0,
	message_id TEXT NOT NULL DEFAULT '',
	en_respuesta TEXT NOT NULL DEFAULT '',
	hilo TEXT NOT NULL DEFAULT '',
	asunto TEXT NOT NULL DEFAULT '',
	de TEXT NOT NULL DEFAULT '',
	de_nombre TEXT NOT NULL DEFAULT '',
	para TEXT NOT NULL DEFAULT '[]',
	cc TEXT NOT NULL DEFAULT '[]',
	responder TEXT NOT NULL DEFAULT '[]',
	fecha INTEGER NOT NULL DEFAULT 0,
	recibido INTEGER NOT NULL DEFAULT 0,
	tamano INTEGER NOT NULL DEFAULT 0,
	marcas TEXT NOT NULL DEFAULT '[]',
	vista TEXT NOT NULL DEFAULT '',
	adjuntos INTEGER NOT NULL DEFAULT 0,
	eml INTEGER NOT NULL DEFAULT 0,
	buscable TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS mensajes_carpeta ON mensajes (cuenta, carpeta, fecha DESC, id DESC);
CREATE INDEX IF NOT EXISTS mensajes_uid ON mensajes (cuenta, carpeta, uid);
CREATE INDEX IF NOT EXISTS mensajes_hilo ON mensajes (hilo);
CREATE INDEX IF NOT EXISTS mensajes_mid ON mensajes (message_id);
CREATE TABLE IF NOT EXISTS cambios (
	seq INTEGER PRIMARY KEY AUTOINCREMENT,
	mensaje INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS pendientes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cuenta TEXT NOT NULL,
	tipo TEXT NOT NULL,
	mensaje INTEGER NOT NULL DEFAULT 0,
	carpeta TEXT NOT NULL DEFAULT '',
	uid INTEGER NOT NULL DEFAULT 0,
	destino TEXT NOT NULL DEFAULT '',
	marca TEXT NOT NULL DEFAULT '',
	poner INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS salida (
	id TEXT PRIMARY KEY,
	cuenta TEXT NOT NULL,
	de TEXT NOT NULL,
	destinatarios TEXT NOT NULL,
	asunto TEXT NOT NULL DEFAULT '',
	sale INTEGER NOT NULL,
	creado INTEGER NOT NULL,
	estado TEXT NOT NULL,
	intentos INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS etiquetas (
	id TEXT PRIMARY KEY,
	nombre TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS mensaje_etiquetas (
	mensaje INTEGER NOT NULL,
	etiqueta TEXT NOT NULL,
	PRIMARY KEY (mensaje, etiqueta)
);
CREATE TABLE IF NOT EXISTS ajustes (
	clave TEXT PRIMARY KEY,
	valor TEXT NOT NULL
);
`

// Estados de la bandeja de salida.
const (
	salidaPendiente  = "pendiente"  // en la ventana de deshacer, o esperando red
	salidaProgramada = "programado" // envío con hora
	salidaError      = "error"      // el servidor lo rechazó: no se reintenta solo
)

type indice struct {
	db *sql.DB
}

// FicheroIndice es el nombre del índice SQLite dentro de la carpeta del correo.
const FicheroIndice = "indice.sqlite"

// RutaIndice es el fichero del índice SQLite del correo.
func (b *Buzon) RutaIndice() string { return filepath.Join(b.raiz, FicheroIndice) }

// CopiarIndice deja en destino una copia coherente del índice abierto (`VACUUM INTO`),
// sin cerrarlo: la copia de seguridad (`copia.BaseAbierta`). Los `.eml` ya son ficheros.
func (b *Buzon) CopiarIndice(ctx context.Context, destino string) error {
	_, err := b.x.db.ExecContext(ctx, "VACUUM INTO ?", destino)
	return err
}

func abrirIndice(raiz string) (*indice, error) {
	ruta := filepath.Join(raiz, FicheroIndice)
	db, err := sql.Open("sqlite3", "file:"+ruta+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, err
	}
	// Una conexión: SQLite serializa las escrituras de todos modos, y así no hay
	// «database is locked» entre la pantalla y el sincronizador.
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, esquema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("correo: no se pudo preparar el índice: %w", err)
	}
	if err := migrar(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("correo: no se pudo poner al día el índice: %w", err)
	}
	return &indice{db: db}, nil
}

// migraciones son las columnas y tablas que llegaron después de la primera versión del
// índice: `CREATE TABLE IF NOT EXISTS` no añade columnas a una tabla que ya existe.
var migraciones = []string{
	// modseq y existen: el HIGHESTMODSEQ (CONDSTORE) y el EXISTS de la última vuelta.
	"ALTER TABLE carpetas ADD COLUMN modseq INTEGER NOT NULL DEFAULT 0",
	"ALTER TABLE carpetas ADD COLUMN existen INTEGER NOT NULL DEFAULT 0",
	// estrenada: la carpeta ya tuvo su primera vuelta (con mensajes o VACÍA). Sin ella,
	// una entrada vacía en la primera vuelta seguía con `ultimo` a 0 y el primer correo
	// que llegaba después se tomaba por «lo que ya estaba»: ni avisaba ni se miraba como
	// respuesta iMIP.
	"ALTER TABLE carpetas ADD COLUMN estrenada INTEGER NOT NULL DEFAULT 0",
	// pospuesto: hasta cuándo está fuera de Recibidos (unix; 0, no lo está). vuelto:
	// cuándo volvió solo al vencer (ordena la lista, como `unsnoozed_at`).
	"ALTER TABLE mensajes ADD COLUMN pospuesto INTEGER NOT NULL DEFAULT 0",
	"ALTER TABLE mensajes ADD COLUMN vuelto INTEGER NOT NULL DEFAULT 0",
	// Conversaciones silenciadas y remitentes cuyas imágenes remotas se enseñan siempre.
	"CREATE TABLE IF NOT EXISTS silenciados (hilo TEXT PRIMARY KEY)",
	"CREATE TABLE IF NOT EXISTS confiados (direccion TEXT PRIMARY KEY)",
	// Los mensajes con la respuesta de un invitado (iMIP REPLY) ya procesados
	// (respuestas.go): dejados para Calendar o descartados.
	"CREATE TABLE IF NOT EXISTS respuestas_imip (mensaje INTEGER PRIMARY KEY, message_id TEXT NOT NULL DEFAULT '', estado TEXT NOT NULL, cuando INTEGER NOT NULL DEFAULT 0)",
	"CREATE INDEX IF NOT EXISTS respuestas_imip_mid ON respuestas_imip (message_id)",
}

func migrar(ctx context.Context, db *sql.DB) error {
	for _, q := range migraciones {
		if _, err := db.ExecContext(ctx, q); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	return nil
}

func (x *indice) cerrar() error { return x.db.Close() }

// sobre es una fila de mensajes.
type sobre struct {
	id                     int64
	cuenta, carpeta        string
	uid                    uint32
	messageID, enRespuesta string
	hilo                   string
	asunto, de, deNombre   string
	para, cc, responder    []string
	fecha, recibido        time.Time
	tamano                 int64
	marcas                 []string
	vista                  string
	adjuntos, eml          bool
	buscable               string
	rol                    string // de la carpeta (consulta con JOIN)
	etiquetas              []string
	pospuesto, vuelto      time.Time
}

func jsonLista(xs []string) string {
	if xs == nil {
		xs = []string{}
	}
	b, _ := json.Marshal(xs)
	return string(b)
}

func deLista(s string) []string {
	var xs []string
	_ = json.Unmarshal([]byte(s), &xs)
	return xs
}

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// u32 lee del índice un número que se guardó desde un uint32 de IMAP (un UID, el
// UIDVALIDITY, el EXISTS): siempre cabe.
func u32(n int64) uint32 {
	return uint32(n) //nolint:gosec // G115: en esas columnas solo se escriben valores de 32 bits de IMAP.
}

func deUnix(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

const columnas = `m.id, m.cuenta, m.carpeta, m.uid, m.message_id, m.en_respuesta, m.hilo, m.asunto, m.de, m.de_nombre,
	m.para, m.cc, m.responder, m.fecha, m.recibido, m.tamano, m.marcas, m.vista, m.adjuntos, m.eml, COALESCE(f.rol, ''),
	m.pospuesto, m.vuelto`

// ordenFecha es la fecha por la que se ordena una lista: la de llegada o, si volvió de
// «Pospuestos» después, la de vuelta (lo devuelto sube arriba, como en Gmail).
const ordenFecha = "MAX(m.fecha, m.vuelto)"

const desde = ` FROM mensajes m LEFT JOIN carpetas f ON f.cuenta = m.cuenta AND f.nombre = m.carpeta `

type escaner interface{ Scan(...any) error }

func leerSobre(r escaner) (sobre, error) {
	var s sobre
	var para, cc, resp, marcas string
	var fecha, recibido, pospuesto, vuelto int64
	var uid int64
	err := r.Scan(&s.id, &s.cuenta, &s.carpeta, &uid, &s.messageID, &s.enRespuesta, &s.hilo, &s.asunto, &s.de, &s.deNombre,
		&para, &cc, &resp, &fecha, &recibido, &s.tamano, &marcas, &s.vista, &s.adjuntos, &s.eml, &s.rol, &pospuesto, &vuelto)
	s.uid = u32(uid)
	s.pospuesto, s.vuelto = deUnix(pospuesto), deUnix(vuelto)
	s.para, s.cc, s.responder, s.marcas = deLista(para), deLista(cc), deLista(resp), deLista(marcas)
	s.fecha, s.recibido = deUnix(fecha), deUnix(recibido)
	return s, err
}

func (x *indice) sobres(ctx context.Context, consulta string, args ...any) ([]sobre, error) {
	//nolint:gosec // G202: consulta es siempre un literal de este paquete; los valores van en args.
	filas, err := x.db.QueryContext(ctx, "SELECT "+columnas+desde+consulta, args...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []sobre
	for filas.Next() {
		s, err := leerSobre(filas)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}
	return out, x.ponerEtiquetas(ctx, out)
}

// ponerEtiquetas añade las etiquetas locales de cada sobre.
func (x *indice) ponerEtiquetas(ctx context.Context, ss []sobre) error {
	if len(ss) == 0 {
		return nil
	}
	ids := make([]string, len(ss))
	por := make(map[int64]int, len(ss))
	for i, s := range ss {
		ids[i] = strconv.FormatInt(s.id, 10)
		por[s.id] = i
	}
	//nolint:gosec // G202: ids son enteros formateados con strconv, no texto de nadie.
	filas, err := x.db.QueryContext(ctx, "SELECT mensaje, etiqueta FROM mensaje_etiquetas WHERE mensaje IN ("+strings.Join(ids, ",")+")")
	if err != nil {
		return err
	}
	defer filas.Close()
	for filas.Next() {
		var m int64
		var e string
		if err := filas.Scan(&m, &e); err != nil {
			return err
		}
		ss[por[m]].etiquetas = append(ss[por[m]].etiquetas, e)
	}
	return filas.Err()
}

func (x *indice) sobre(ctx context.Context, id int64) (sobre, error) {
	ss, err := x.sobres(ctx, "WHERE m.id = ?", id)
	if err != nil {
		return sobre{}, err
	}
	if len(ss) == 0 {
		return sobre{}, ErrNoEncontrado
	}
	return ss[0], nil
}

// ErrNoEncontrado: el mensaje o la carpeta ya no está.
var ErrNoEncontrado = errors.New("correo: no encontrado")

// apuntar deja constancia de que un mensaje cambió, para el delta de la pantalla.
func apuntar(ctx context.Context, tx execer, id int64) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO cambios (mensaje) VALUES (?)", id)
	return err
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// insertar guarda un sobre nuevo y devuelve su id.
func insertar(ctx context.Context, tx execer, s *sobre) error {
	r, err := tx.ExecContext(ctx, `INSERT INTO mensajes (cuenta, carpeta, uid, message_id, en_respuesta, hilo, asunto, de, de_nombre,
		para, cc, responder, fecha, recibido, tamano, marcas, vista, adjuntos, eml, buscable)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.cuenta, s.carpeta, s.uid, s.messageID, s.enRespuesta, s.hilo, s.asunto, s.de, s.deNombre,
		jsonLista(s.para), jsonLista(s.cc), jsonLista(s.responder), unix(s.fecha), unix(s.recibido), s.tamano,
		jsonLista(s.marcas), s.vista, s.adjuntos, s.eml, s.buscable)
	if err != nil {
		return err
	}
	if s.id, err = r.LastInsertId(); err != nil {
		return err
	}
	if s.hilo == "" {
		// Sin Message-ID ni referencias, el mensaje es su propio hilo.
		s.hilo = "m" + strconv.FormatInt(s.id, 10)
		if _, err := tx.ExecContext(ctx, "UPDATE mensajes SET hilo = ? WHERE id = ?", s.hilo, s.id); err != nil {
			return err
		}
	}
	return apuntar(ctx, tx, s.id)
}

// textoBuscable es lo que la búsqueda mira de un mensaje, plegado (sin acentos ni
// mayúsculas, `buscar.Plegar`).
func textoBuscable(s *sobre, cuerpo string) string {
	const tope = 64 << 10
	if len(cuerpo) > tope {
		cuerpo = cuerpo[:tope]
	}
	return buscar.Plegar(strings.Join(slices.Concat([]string{s.asunto, s.de, s.deNombre, cuerpo}, s.para, s.cc), "\n"))
}

// ---- Ajustes sueltos (firma, reglas) ----

func (x *indice) ajuste(ctx context.Context, clave string) (string, error) {
	var v string
	err := x.db.QueryRowContext(ctx, "SELECT valor FROM ajustes WHERE clave = ?", clave).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (x *indice) ponerAjuste(ctx context.Context, clave, valor string) error {
	_, err := x.db.ExecContext(ctx, "INSERT INTO ajustes (clave, valor) VALUES (?, ?) ON CONFLICT(clave) DO UPDATE SET valor = excluded.valor", clave, valor)
	return err
}

// ---- Carpetas ----

type carpeta struct {
	cuenta, nombre, rol string
	uidvalidity, ultimo uint32
	local               bool
	// modseq y existen son el HIGHESTMODSEQ y el EXISTS de la última vuelta: con
	// CONDSTORE, si no han cambiado y no llegó nada, las marcas no se vuelven a pedir.
	modseq  uint64
	existen uint32
	// estrenada: ya tuvo su primera vuelta, aunque estuviera vacía (sincronizar.go).
	estrenada bool
}

func (x *indice) carpetas(ctx context.Context, cuenta string) ([]carpeta, error) {
	q := "SELECT cuenta, nombre, rol, uidvalidity, ultimo, local, modseq, existen, estrenada FROM carpetas"
	var args []any
	if cuenta != "" {
		q += " WHERE cuenta = ?"
		args = append(args, cuenta)
	}
	filas, err := x.db.QueryContext(ctx, q+" ORDER BY cuenta, nombre", args...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []carpeta
	for filas.Next() {
		var c carpeta
		var v, u, ms, ex int64
		if err := filas.Scan(&c.cuenta, &c.nombre, &c.rol, &v, &u, &c.local, &ms, &ex, &c.estrenada); err != nil {
			return nil, err
		}
		//nolint:gosec // G115: el MODSEQ se guardó desde un uint64 que SQLite solo admite sin el bit alto.
		c.uidvalidity, c.ultimo, c.modseq, c.existen = u32(v), u32(u), uint64(ms), u32(ex)
		out = append(out, c)
	}
	return out, filas.Err()
}

// carpetaDeRol es la carpeta de una cuenta con ese rol. Si la cuenta no la tiene, se
// crea una LOCAL (solo en este equipo): un borrador o un enviado tienen que vivir en
// algún sitio aunque el servidor no tenga «Drafts».
func (x *indice) carpetaDeRol(ctx context.Context, cuenta, rol string) (carpeta, error) {
	cs, err := x.carpetas(ctx, cuenta)
	if err != nil {
		return carpeta{}, err
	}
	for _, c := range cs {
		if c.rol == rol {
			return c, nil
		}
	}
	c := carpeta{cuenta: cuenta, nombre: "\\Local\\" + rol, rol: rol, local: true}
	_, err = x.db.ExecContext(ctx, "INSERT OR IGNORE INTO carpetas (cuenta, nombre, rol, local) VALUES (?, ?, ?, 1)", c.cuenta, c.nombre, c.rol)
	return c, err
}

// idCarpeta es el id que ve la pantalla. Las carpetas con rol se juntan entre cuentas
// («Recibidos» enseña la entrada de todas): `rol:inbox`. Las de la persona llevan su
// cuenta: `c:<cuenta>:<nombre>`.
func idCarpeta(cuenta, nombre, rol string) string {
	if rol != "" {
		return "rol:" + rol
	}
	return "c:" + cuenta + ":" + nombre
}

// leerIDCarpeta deshace idCarpeta.
func leerIDCarpeta(id string) (rol, cuenta, nombre string, ok bool) {
	if r, ok := strings.CutPrefix(id, "rol:"); ok && r != "" {
		return r, "", "", true
	}
	if resto, ok := strings.CutPrefix(id, "c:"); ok {
		if c, n, ok := strings.Cut(resto, ":"); ok && c != "" && n != "" {
			return "", c, n, true
		}
	}
	return "", "", "", false
}

// filtroCarpeta es la condición SQL de una carpeta de la pantalla.
func filtroCarpeta(id string) (string, []any, error) {
	rol, cuenta, nombre, ok := leerIDCarpeta(id)
	switch {
	case !ok:
		return "", nil, ErrNoEncontrado
	case rol == "snoozed":
		return "m.pospuesto > 0", nil, nil
	case rol == "inbox":
		// Lo pospuesto no está en Recibidos hasta que vuelve.
		return "f.rol = ? AND m.pospuesto = 0", []any{rol}, nil
	case rol != "":
		return "f.rol = ?", []any{rol}, nil
	}
	return "m.cuenta = ? AND m.carpeta = ?", []any{cuenta, nombre}, nil
}

// ---- Cursores ----

// cursor de página: la fecha y el id del último sobre entregado (paginación por clave:
// el correo que llega arriba no descoloca las páginas siguientes).
func cursorDe(s sobre) string {
	return strconv.FormatInt(max(unix(s.fecha), unix(s.vuelto)), 10) + "." + strconv.FormatInt(s.id, 10)
}

func leerCursor(c string) (int64, int64, bool) {
	f, i, ok := strings.Cut(c, ".")
	if !ok {
		return 0, 0, false
	}
	a, err1 := strconv.ParseInt(f, 10, 64)
	b, err2 := strconv.ParseInt(i, 10, 64)
	return a, b, err1 == nil && err2 == nil
}

// ---- Conversión al modelo de la pantalla ----

func (s sobre) mensaje(hilos map[string]int) api.MailMessage {
	m := api.MailMessage{
		ID:            strconv.FormatInt(s.id, 10),
		MailboxID:     idCarpeta(s.cuenta, s.carpeta, s.rol),
		ThreadID:      s.hilo,
		ThreadCount:   hilos[s.hilo],
		MessageID:     s.messageID,
		InReplyTo:     s.enRespuesta,
		Subject:       s.asunto,
		From:          s.de,
		FromName:      s.deNombre,
		To:            s.para,
		Cc:            s.cc,
		ReplyTo:       s.responder,
		Preview:       s.vista,
		Size:          s.tamano,
		Keywords:      slices.Concat(s.marcas, s.etiquetas),
		HasAttachment: s.adjuntos,
		ReceivedAt:    s.recibido,
		Date:          s.fecha,
	}
	if m.ReceivedAt.IsZero() {
		m.ReceivedAt = m.Date
	}
	if m.Date.IsZero() {
		m.Date = m.ReceivedAt
	}
	if !s.vuelto.IsZero() {
		v := s.vuelto
		m.UnsnoozedAt = &v
	}
	return m
}

// cuentaHilos cuenta los mensajes de cada hilo (fuera de Papelera y Spam).
func (x *indice) cuentaHilos(ctx context.Context, ss []sobre) (map[string]int, error) {
	out := map[string]int{}
	if len(ss) == 0 {
		return out, nil
	}
	hilos := make([]any, 0, len(ss))
	marcas := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ya := out[s.hilo]; !ya {
			out[s.hilo] = 0
			hilos = append(hilos, s.hilo)
			marcas = append(marcas, "?")
		}
	}
	//nolint:gosec // G202: lo que se une son marcadores «?»; los hilos van en args.
	filas, err := x.db.QueryContext(ctx, "SELECT m.hilo, COUNT(*)"+desde+"WHERE m.hilo IN ("+strings.Join(marcas, ",")+") AND COALESCE(f.rol, '') NOT IN ('trash', 'junk') GROUP BY m.hilo", hilos...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	for filas.Next() {
		var h string
		var n int
		if err := filas.Scan(&h, &n); err != nil {
			return nil, err
		}
		out[h] = n
	}
	return out, filas.Err()
}

func (x *indice) mensajes(ctx context.Context, ss []sobre) ([]api.MailMessage, error) {
	hilos, err := x.cuentaHilos(ctx, ss)
	if err != nil {
		return nil, err
	}
	out := make([]api.MailMessage, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.mensaje(hilos))
	}
	return out, nil
}

// conMarca pone o quita una marca de una lista.
func conMarca(ms []string, marca string, poner bool) []string {
	ms = slices.DeleteFunc(slices.Clone(ms), func(m string) bool { return strings.EqualFold(m, marca) })
	if poner {
		ms = append(ms, marca)
	}
	return ms
}

func tieneMarca(ms []string, marca string) bool {
	return slices.ContainsFunc(ms, func(m string) bool { return strings.EqualFold(m, marca) })
}
