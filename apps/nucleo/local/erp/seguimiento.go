// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// TAREAS Y DOCUMENTOS colgados de un registro (entrega 3): las `actividades` y los
// `adjuntos` del servidor (store/compartido.go, http/actividades.go). En local hay una
// persona: el responsable es siempre ella y no hay avisos a otros (E-09). Los
// documentos son FICHEROS en `<datos>/Datos/erp/adjuntos/`, con su nombre de id y la
// extensión de su tipo, legibles desde fuera de la app (R-CLARO); la base guarda el
// nombre, el tipo y la huella.

const esquemaSeguimiento = `
CREATE TABLE IF NOT EXISTS actividad (
  id       TEXT PRIMARY KEY,
  modelo   TEXT NOT NULL DEFAULT '',
  registro TEXT NOT NULL DEFAULT '',
  tipo     TEXT NOT NULL,
  resumen  TEXT NOT NULL,
  notas    TEXT NOT NULL DEFAULT '',
  vence    TEXT NOT NULL,             -- AAAA-MM-DD
  estado   TEXT NOT NULL DEFAULT 'pendiente' CHECK (estado IN ('pendiente', 'hecha', 'cancelada')),
  creada   TEXT NOT NULL,
  hecha    TEXT
);
CREATE INDEX IF NOT EXISTS actividad_vence ON actividad(estado, vence);
CREATE TABLE IF NOT EXISTS adjunto (
  id       TEXT PRIMARY KEY,
  modelo   TEXT NOT NULL,
  registro TEXT NOT NULL,
  nombre   TEXT NOT NULL,
  tipo     TEXT NOT NULL,
  tamano   INTEGER NOT NULL,
  sha256   TEXT NOT NULL,
  fichero  TEXT NOT NULL,             -- relativo a la carpeta de adjuntos
  creado   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS adjunto_registro ON adjunto(modelo, registro, creado);
`

// TiposDeActividad son los que admite el servidor.
var TiposDeActividad = []string{"subir_documento", "pedir_documento", "llamada", "correo", "reunion", "tarea"}

// MaxAdjunto es el tope de un documento, el del servidor.
const MaxAdjunto = 10 << 20

// tiposAdjunto son los que se aceptan, por su tipo DETECTADO en el contenido.
var tiposAdjunto = map[string]string{
	"application/pdf": ".pdf", "image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp",
	"application/xml": ".xml",
}

var (
	errAdjuntoTipo   = errors.New("erp: tipo de documento no admitido")
	errAdjuntoGrande = errors.New("erp: documento demasiado grande")
)

// Actividad es una tarea con vencimiento (la forma de `store.Actividad`).
type Actividad struct {
	ID                string     `json:"id"`
	Modelo            string     `json:"modelo"`
	Registro          string     `json:"registro"`
	RegistroEtiqueta  string     `json:"registro_etiqueta,omitempty"`
	Tipo              string     `json:"tipo"`
	Resumen           string     `json:"resumen"`
	Notas             string     `json:"notas,omitempty"`
	Vence             string     `json:"vence"`
	ResponsableSub    string     `json:"responsable_sub,omitempty"`
	ResponsableNombre string     `json:"responsable_nombre,omitempty"`
	Estado            string     `json:"estado"`
	CreadaEn          time.Time  `json:"creada_en"`
	HechaEn           *time.Time `json:"hecha_en,omitempty"`
}

// Adjunto es un documento colgado de un registro, sin su contenido.
type Adjunto struct {
	ID       string    `json:"id"`
	Modelo   string    `json:"modelo"`
	Registro string    `json:"registro"`
	Nombre   string    `json:"nombre"`
	Tipo     string    `json:"tipo"`
	Tamano   int       `json:"tamano"`
	SHA256   string    `json:"sha256"`
	CreadoEn time.Time `json:"creado_en"`
	fichero  string
}

const columnasActividad = `id, modelo, registro, tipo, resumen, notas, vence, estado, creada, hecha`

func escanearActividad(f escaner) (Actividad, error) {
	var a Actividad
	var creada string
	var hecha sql.NullString
	err := f.Scan(&a.ID, &a.Modelo, &a.Registro, &a.Tipo, &a.Resumen, &a.Notas, &a.Vence, &a.Estado, &creada, &hecha)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNoEncontrado
	}
	a.CreadaEn, _ = time.Parse(marca, creada)
	if hecha.Valid {
		t, _ := time.Parse(marca, hecha.String)
		a.HechaEn = &t
	}
	a.ResponsableSub = Persona
	return a, err
}

// CrearActividad apunta una tarea ya validada.
func (a *Almacen) CrearActividad(ctx context.Context, act Actividad) (Actividad, error) {
	id := uuid.NewString()
	if _, err := a.db.ExecContext(ctx, `INSERT INTO actividad (id, modelo, registro, tipo, resumen, notas, vence, creada) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, act.Modelo, act.Registro, act.Tipo, act.Resumen, act.Notas, act.Vence, a.marcaAhora()); err != nil {
		return Actividad{}, err
	}
	return a.Actividad(ctx, id)
}

// Actividad lee una tarea.
func (a *Almacen) Actividad(ctx context.Context, id string) (Actividad, error) {
	return escanearActividad(a.db.QueryRowContext(ctx, `SELECT `+columnasActividad+` FROM actividad WHERE id = ?`, id))
}

// FiltroActividades acota la lista.
type FiltroActividades struct {
	Estado, Modelo, Registro, Hasta string
}

// Actividades lista las tareas, las que vencen antes primero (200 como mucho).
func (a *Almacen) Actividades(ctx context.Context, f FiltroActividades) ([]Actividad, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT `+columnasActividad+` FROM actividad
		WHERE (? = '' OR estado = ?) AND (? = '' OR modelo = ?) AND (? = '' OR registro = ?) AND (? = '' OR vence <= ?)
		ORDER BY vence, creada LIMIT 200`, f.Estado, f.Estado, f.Modelo, f.Modelo, f.Registro, f.Registro, f.Hasta, f.Hasta)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []Actividad{}
	for filas.Next() {
		act, err := escanearActividad(filas)
		if err != nil {
			return nil, err
		}
		out = append(out, act)
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}
	_ = filas.Close()
	// Con las filas ya cerradas: la base tiene una sola conexión.
	for i := range out {
		if out[i].Registro != "" {
			var etiqueta, ref string
			if a.db.QueryRowContext(ctx, `SELECT etiqueta, ref FROM registro WHERE id = ?`, out[i].Registro).Scan(&etiqueta, &ref) == nil {
				out[i].RegistroEtiqueta = cmpOr(etiqueta, ref)
			}
		}
	}
	return out, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// CerrarActividad la da por hecha o cancelada. Solo desde pendiente.
func (a *Almacen) CerrarActividad(ctx context.Context, id, estado string) (Actividad, error) {
	var hecha any
	if estado == "hecha" {
		hecha = a.marcaAhora()
	}
	res, err := a.db.ExecContext(ctx, `UPDATE actividad SET estado = ?, hecha = ? WHERE id = ? AND estado = 'pendiente'`, estado, hecha, id)
	if err != nil {
		return Actividad{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Actividad{}, ErrNoEncontrado
	}
	return a.Actividad(ctx, id)
}

// carpetaAdjuntos es donde viven los documentos.
func (a *Almacen) carpetaAdjuntos() string { return filepath.Join(a.dir, "adjuntos") }

// GuardarAdjunto escribe el documento en su fichero y lo apunta, y da por hechas las
// tareas «subir documento» del registro: subir la factura ES la tarea.
func (a *Almacen) GuardarAdjunto(ctx context.Context, modelo, registro, nombre, tipo string, contenido []byte) (Adjunto, int, error) {
	suma := sha256.Sum256(contenido)
	ad := Adjunto{
		ID: uuid.NewString(), Modelo: modelo, Registro: registro, Nombre: nombre, Tipo: tipo,
		Tamano: len(contenido), SHA256: hex.EncodeToString(suma[:]),
	}
	ad.fichero = ad.ID + tiposAdjunto[tipo]
	if err := os.MkdirAll(a.carpetaAdjuntos(), 0o700); err != nil {
		return Adjunto{}, 0, err
	}
	ruta := filepath.Join(a.carpetaAdjuntos(), ad.fichero)
	// Primero a un temporal y luego en su sitio: un corte no deja medio PDF con su nombre.
	tmp := ruta + ".tmp"
	if err := os.WriteFile(tmp, contenido, 0o600); err != nil {
		return Adjunto{}, 0, err
	}
	if err := os.Rename(tmp, ruta); err != nil {
		_ = os.Remove(tmp)
		return Adjunto{}, 0, err
	}
	cerradas := 0
	ahora := a.marcaAhora()
	err := a.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO adjunto (id, modelo, registro, nombre, tipo, tamano, sha256, fichero, creado) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ad.ID, modelo, registro, nombre, tipo, ad.Tamano, ad.SHA256, ad.fichero, ahora); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE actividad SET estado = 'hecha', hecha = ? WHERE modelo = ? AND registro = ? AND tipo = 'subir_documento' AND estado = 'pendiente'`,
			ahora, modelo, registro)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		cerradas = int(n)
		return nil
	})
	if err != nil {
		_ = os.Remove(ruta)
		return Adjunto{}, 0, err
	}
	ad.CreadoEn, _ = time.Parse(marca, ahora)
	return ad, cerradas, nil
}

const columnasAdjunto = `id, modelo, registro, nombre, tipo, tamano, sha256, fichero, creado`

func escanearAdjunto(f escaner) (Adjunto, error) {
	var ad Adjunto
	var creado string
	err := f.Scan(&ad.ID, &ad.Modelo, &ad.Registro, &ad.Nombre, &ad.Tipo, &ad.Tamano, &ad.SHA256, &ad.fichero, &creado)
	if errors.Is(err, sql.ErrNoRows) {
		return ad, ErrNoEncontrado
	}
	ad.CreadoEn, _ = time.Parse(marca, creado)
	return ad, err
}

// Adjuntos lista los documentos de un registro, el último primero.
func (a *Almacen) Adjuntos(ctx context.Context, modelo, registro string) ([]Adjunto, error) {
	filas, err := a.db.QueryContext(ctx, `SELECT `+columnasAdjunto+` FROM adjunto WHERE modelo = ? AND registro = ? ORDER BY creado DESC`, modelo, registro)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()
	out := []Adjunto{}
	for filas.Next() {
		ad, err := escanearAdjunto(filas)
		if err != nil {
			return nil, err
		}
		out = append(out, ad)
	}
	return out, filas.Err()
}

// AdjuntoConContenido lee un documento entero.
func (a *Almacen) AdjuntoConContenido(ctx context.Context, id string) (Adjunto, []byte, error) {
	ad, err := escanearAdjunto(a.db.QueryRowContext(ctx, `SELECT `+columnasAdjunto+` FROM adjunto WHERE id = ?`, id))
	if err != nil {
		return ad, nil, err
	}
	contenido, err := os.ReadFile(filepath.Join(a.carpetaAdjuntos(), filepath.Base(ad.fichero)))
	return ad, contenido, err
}

// BorrarAdjunto lo borra de verdad, fila y fichero: es un fichero subido por error.
func (a *Almacen) BorrarAdjunto(ctx context.Context, id string) error {
	ad, err := escanearAdjunto(a.db.QueryRowContext(ctx, `SELECT `+columnasAdjunto+` FROM adjunto WHERE id = ?`, id))
	if err != nil {
		return err
	}
	if _, err := a.db.ExecContext(ctx, `DELETE FROM adjunto WHERE id = ?`, id); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(a.carpetaAdjuntos(), filepath.Base(ad.fichero))); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// --- Rutas ------------------------------------------------------------------------

func (s *Servidor) rutasSeguimiento(h func(string, http.HandlerFunc)) {
	v := "/api/v1"
	h("GET "+v+"/actividades", s.actividades)
	h("POST "+v+"/actividades", s.crearActividad)
	h("POST "+v+"/actividades/{id}/hecha", s.cerrarActividad("hecha"))
	h("POST "+v+"/actividades/{id}/cancelar", s.cerrarActividad("cancelada"))
	h("GET "+v+"/equipo", s.equipo)
	h("GET "+v+"/adjuntos", s.adjuntos)
	h("POST "+v+"/adjuntos", s.subirAdjunto)
	h("GET "+v+"/adjuntos/{id}", s.descargarAdjunto)
	h("DELETE "+v+"/adjuntos/{id}", s.borrarAdjunto)
}

func (s *Servidor) actividades(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := FiltroActividades{Estado: q.Get("estado"), Modelo: q.Get("modelo"), Registro: q.Get("registro"), Hasta: q.Get("hasta")}
	if f.Hasta != "" {
		if _, err := time.Parse(time.DateOnly, f.Hasta); err != nil {
			fallo(w, http.StatusBadRequest, "bad_request", "hasta must be YYYY-MM-DD.", nil)
			return
		}
	}
	lista, err := s.alm.Actividades(r.Context(), f)
	if err != nil {
		falloDe(w, err)
		return
	}
	for i := range lista {
		lista[i].ResponsableNombre = s.Nombre
	}
	responder(w, http.StatusOK, map[string]any{"actividades": lista})
}

func (s *Servidor) crearActividad(w http.ResponseWriter, r *http.Request) {
	var a Actividad
	if err := leer(r, &a); err != nil {
		falloDe(w, err)
		return
	}
	a.Resumen = validation.TruncateRunes(strings.TrimSpace(a.Resumen), 300)
	a.Notas = validation.TruncateRunes(strings.TrimSpace(a.Notas), 4000)
	v := validation.New().Required("resumen", a.Resumen)
	if !slices.Contains(TiposDeActividad, a.Tipo) {
		v.Add("tipo", "tipo desconocido")
	}
	if _, err := time.Parse(time.DateOnly, a.Vence); err != nil {
		v.Add("vence", "fecha AAAA-MM-DD")
	}
	if a.Modelo != "" {
		if _, ok := s.cat.Modelo(a.Modelo); !ok && a.Modelo != "obligacion" {
			v.Add("modelo", "modelo desconocido")
		}
	}
	if err := v.Err(); err != nil {
		falloValidacion(w, err)
		return
	}
	creada, err := s.alm.CrearActividad(r.Context(), a)
	if err != nil {
		falloDe(w, err)
		return
	}
	creada.ResponsableNombre = s.Nombre
	responder(w, http.StatusCreated, creada)
}

func (s *Servidor) cerrarActividad(estado string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := s.alm.CerrarActividad(r.Context(), r.PathValue("id"), estado)
		if err != nil {
			falloDe(w, err)
			return
		}
		a.ResponsableNombre = s.Nombre
		responder(w, http.StatusOK, a)
	}
}

// equipo es la persona sola: en local no se ponen tareas a otros (E-09).
func (s *Servidor) equipo(w http.ResponseWriter, _ *http.Request) {
	responder(w, http.StatusOK, map[string]any{
		"personas": []map[string]string{{"sub": Persona, "nombre": s.Nombre, "correo": ""}},
		"yo":       Persona,
	})
}

// modeloAdjunto acepta los modelos del catálogo y el proceso de una obligación.
func (s *Servidor) modeloAdjunto(modelo string) bool {
	_, ok := s.cat.Modelo(modelo)
	return ok || modelo == "obligacion"
}

func (s *Servidor) adjuntos(w http.ResponseWriter, r *http.Request) {
	modelo, registro := r.URL.Query().Get("modelo"), r.URL.Query().Get("registro")
	lista, err := s.alm.Adjuntos(r.Context(), modelo, registro)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"adjuntos": lista})
}

// tipoDetectado es el tipo del contenido, sin parámetros.
func tipoDetectado(b []byte) string {
	t, _, _ := mime.ParseMediaType(http.DetectContentType(b))
	if t == "text/xml" {
		return "application/xml"
	}
	return t
}

func (s *Servidor) subirAdjunto(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	modelo, registro := q.Get("modelo"), strings.TrimSpace(q.Get("registro"))
	if registro == "" || len(registro) > 200 {
		fallo(w, http.StatusBadRequest, "bad_request", "registro is required.", nil)
		return
	}
	if !s.modeloAdjunto(modelo) {
		fallo(w, http.StatusNotFound, "modelo_desconocido", "That model does not exist in the catalogue.", nil)
		return
	}
	if _, err := uuid.Parse(registro); err == nil {
		if _, err := s.alm.Obtener(r.Context(), modelo, registro); err != nil {
			falloDe(w, err)
			return
		}
	}
	contenido, err := io.ReadAll(io.LimitReader(r.Body, MaxAdjunto+1))
	if err != nil || len(contenido) > MaxAdjunto {
		falloDe(w, errAdjuntoGrande)
		return
	}
	tipo := tipoDetectado(contenido)
	ext, ok := tiposAdjunto[tipo]
	if len(contenido) == 0 || !ok {
		falloDe(w, errAdjuntoTipo)
		return
	}
	nombre := validation.TruncateRunes(strings.TrimSpace(filepath.Base(validation.SanitizeText(q.Get("nombre")))), 200)
	if nombre == "" || nombre == "." || nombre == "/" {
		nombre = "documento"
	}
	if !strings.EqualFold(filepath.Ext(nombre), ext) {
		nombre += ext
	}
	a, cerradas, err := s.alm.GuardarAdjunto(r.Context(), modelo, registro, nombre, tipo, contenido)
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusCreated, map[string]any{"adjunto": a, "tareas_hechas": cerradas})
}

func (s *Servidor) descargarAdjunto(w http.ResponseWriter, r *http.Request) {
	a, contenido, err := s.alm.AdjuntoConContenido(r.Context(), r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	disposicion := "attachment"
	if r.URL.Query().Get("ver") == "1" && (a.Tipo == "application/pdf" || strings.HasPrefix(a.Tipo, "image/")) {
		disposicion = "inline"
	}
	w.Header().Set("Content-Type", a.Tipo)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposicion, map[string]string{"filename": a.Nombre}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, a.Nombre, a.CreadoEn, bytes.NewReader(contenido))
}

func (s *Servidor) borrarAdjunto(w http.ResponseWriter, r *http.Request) {
	if err := s.alm.BorrarAdjunto(r.Context(), r.PathValue("id")); err != nil {
		falloDe(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
