// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package indice es lo reciente y la búsqueda ENTRE las apps de Sytue Office, la edición
// local de la suite (apps/fyne/PLAN-EDICION-LOCAL.md §6.15, HU-05): lo que el Hub
// enseña en su portada y lo que encuentra su buscador, sin red y sin servidor.
//
// No hay un índice aparte que mantener al día: se leen los almacenes de cada app tal
// como están en la carpeta de datos, en SOLO LECTURA y sin cerrojos —las apps pueden
// estar abiertas a la vez y escribiendo—:
//
//   - los documentos de Docus, Calcus y Diapos: un JSON por documento en
//     `<datos>/Documentos/<producto>/` (`local/biblioteca`), con su título y el texto
//     de su contenido (contenido.go);
//   - las notas, los eventos, las fotos y álbumes y las bases: el diario
//     `registros.jsonl` de `<datos>/Datos/<producto>/`, con [almacen.Leer];
//   - los contactos: la libreta SQLite de Contacts (`<datos>/Datos/contacts/libreta.sqlite`,
//     `local/libreta`), abierta en solo lectura; mientras Contacts no la haya estrenado
//     (y pasado a ella su almacén de antes), su diario `registros.jsonl`;
//   - los ficheros de Drive: los de `<datos>/Drive` y los abiertos recientemente que
//     apunta su `.sytue/indice.json` (`local/archivos`);
//   - el correo: el índice SQLite de Mail, `<datos>/Correo/indice.sqlite`, abierto en
//     solo lectura (correo.go).
//
// Además da lo de hoy (hoy.go, HU-04) y los recordatorios que el Hub residente avisa en
// Linux cuando sus apps están cerradas (avisos.go, HU-06).
//
// Nada de aquí escribe en disco. Una fuente que falla no tumba a las demás: se apunta
// en [Instantanea.Fallos] y la búsqueda sigue con lo que haya.
package indice

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// Clases de elemento.
const (
	Documento = "documento"
	Nota      = "nota"
	Contacto  = "contacto"
	Evento    = "evento"
	Fichero   = "fichero"
	Correo    = "correo"
	// Hilo es la conversación de un correo: es lo que abre Mail (`AlAbrirElemento`).
	Hilo     = "hilo"
	Foto     = "foto"
	Album    = "album"
	Base     = "base"
	Tabla    = "tabla"
	Registro = "registro"
)

// Elemento es una cosa de alguna app: un documento, una nota, un contacto, un evento o
// un fichero de Drive.
type Elemento struct {
	// Producto es la clave del producto que lo abre (`docs`, `notes`, `pdf`…).
	Producto string
	Clase    string
	ID       string
	Titulo   string
	// Detalle es la segunda línea: la organización de un contacto, el lugar de un
	// evento, la carpeta de un fichero.
	Detalle string
	// Texto es donde también se busca y no se enseña entero (el cuerpo de una nota).
	Texto string
	// Fecha es la última modificación; en un evento, cuándo empieza.
	Fecha time.Time
	// Ruta es el fichero del equipo, si el elemento lo es.
	Ruta string
}

// Enlace es el `sytueoffice-<producto>:` con el que la app dueña abre el elemento:
// `/abrir?local=file://…` para un fichero del equipo (lo que ya entiende la cáscara
// `local`) y `/abrir?<clase>=<id>` para lo que vive en su almacén. Una app que aún no
// sepa abrir por identificador se abre en su portada.
func (e Elemento) Enlace() string {
	q := url.Values{}
	if e.Ruta != "" {
		ruta := filepath.ToSlash(e.Ruta)
		if !strings.HasPrefix(ruta, "/") {
			ruta = "/" + ruta // C:/… en Windows: file:///C:/…
		}
		// Como `storage.NewFileURI` de Fyne, que es lo que lee la app que lo recibe: la
		// ruta tal cual, sin escapar (el escape va en la consulta del enlace).
		q.Set("local", "file://"+ruta)
	} else if e.ID != "" {
		q.Set("id", e.ID)
		q.Set("clase", e.Clase)
	}
	s := espacio.Esquema(e.Producto) + ":/abrir"
	if len(q) > 0 {
		s += "?" + q.Encode()
	}
	return s
}

// Instantanea es lo que había en los almacenes al leerlos.
type Instantanea struct {
	Elementos []Elemento
	// Fallos son las fuentes que no se han podido leer, por nombre (`docs`, `notes`…).
	Fallos map[string]error
	// correo es el índice de Mail: no se carga entero, se le pregunta al buscar.
	correo string
}

// MaxFicherosDrive acota lo que se recorre de la carpeta de Drive: el Hub busca, no
// indexa un disco entero.
const MaxFicherosDrive = 5000

// Leer recorre los almacenes de la carpeta de datos `datos` (`espacio.Rutas.Datos`).
func Leer(datos string) Instantanea {
	s := Instantanea{Fallos: map[string]error{}}
	anotar := func(fuente string, es []Elemento, err error) {
		s.Elementos = append(s.Elementos, es...)
		if err != nil {
			s.Fallos[fuente] = err
		}
	}
	for _, p := range []string{"docs", "sheets", "slides"} {
		es, err := documentos(filepath.Join(datos, "Documentos", p), p)
		anotar(p, es, err)
	}
	for _, p := range []struct {
		producto string
		leer     func([]almacen.Fila) []Elemento
	}{{"notes", notas}, {"contacts", contactos}, {"calendar", eventos}, {"bases", bases}} {
		dir := filepath.Join(datos, "Datos", p.producto)
		if p.producto == "contacts" {
			if es, hay, err := contactosDeLaLibreta(dir); hay {
				anotar(p.producto, es, err)
				continue
			}
		}
		filas, err := almacen.Leer(dir)
		anotar(p.producto, p.leer(filas), err)
	}
	// Photos guarda su fototeca en SQLite (`local/fototeca`); la de antes, en el almacén.
	fotos, err := fototeca(filepath.Join(datos, "Datos", "photos"))
	anotar("photos", fotos, err)
	es, err := drive(filepath.Join(datos, "Drive"))
	anotar("drive", es, err)
	if s.correo = rutaCorreo(datos); s.correo != "" {
		if _, _, err := sinLeer(s.correo, 0); err != nil {
			s.Fallos["mail"] = err
			s.correo = ""
		}
	}
	return s
}

// Recientes son los documentos, notas y ficheros tocados más recientemente, hasta n.
func (s Instantanea) Recientes(n int) []Elemento {
	var out []Elemento
	for _, e := range s.Elementos {
		if e.Clase == Documento || e.Clase == Nota || e.Clase == Fichero {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b Elemento) int { return b.Fecha.Compare(a.Fecha) })
	return out[:min(n, len(out))]
}

// Buscar devuelve hasta n elementos en los que aparecen TODOS los términos de la
// consulta, sin mayúsculas ni acentos (`nucleo/buscar`). Primero los que lo llevan en el
// título; dentro de cada grupo, lo más reciente primero.
func (s Instantanea) Buscar(consulta string, n int) []Elemento {
	terminos := buscar.Terminos(consulta)
	if len(terminos) == 0 {
		return nil
	}
	type hallado struct {
		e        Elemento
		enTitulo bool
	}
	var out []hallado
	// El correo se busca en su índice (puede ser enorme): los n más recientes que casan.
	if s.correo != "" {
		correos, _ := buscarCorreo(s.correo, terminos, n)
		for _, e := range correos {
			titulo := buscar.Plegar(e.Titulo)
			enTitulo := true
			for _, t := range terminos {
				enTitulo = enTitulo && strings.Contains(titulo, buscar.Plegar(t))
			}
			out = append(out, hallado{e, enTitulo})
		}
	}
	for _, e := range s.Elementos {
		titulo := buscar.Plegar(e.Titulo)
		todo := titulo + "\n" + buscar.Plegar(e.Detalle) + "\n" + buscar.Plegar(e.Texto)
		enTitulo, esta := true, true
		for _, t := range terminos {
			t = buscar.Plegar(t)
			if !strings.Contains(todo, t) {
				esta = false
				break
			}
			enTitulo = enTitulo && strings.Contains(titulo, t)
		}
		if esta {
			out = append(out, hallado{e, enTitulo})
		}
	}
	slices.SortStableFunc(out, func(a, b hallado) int {
		if a.enTitulo != b.enTitulo {
			if a.enTitulo {
				return -1
			}
			return 1
		}
		return b.e.Fecha.Compare(a.e.Fecha)
	})
	res := make([]Elemento, 0, min(n, len(out)))
	for _, h := range out[:min(n, len(out))] {
		res = append(res, h.e)
	}
	return res
}

// ----- Documentos de la ofimática -----

// documentos lee la carpeta de un producto de la biblioteca sin abrirla
// (`biblioteca.Abrir` la crearía si faltase).
func documentos(dir, producto string) ([]Elemento, error) {
	entradas, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Elemento
	for _, en := range entradas {
		if en.IsDir() || !strings.HasSuffix(en.Name(), ".json") || strings.HasPrefix(en.Name(), ".") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, en.Name())) //nolint:gosec // G304: la biblioteca de la edición
		if err != nil {
			continue // borrado o renombrado mientras se leía
		}
		var r struct {
			Documento api.Document `json:"documento"`
			Estado    estadoDoc    `json:"estado"`
		}
		if json.Unmarshal(raw, &r) != nil || r.Documento.ID == "" || r.Documento.DeletedAt != nil {
			continue
		}
		d := r.Documento
		out = append(out, Elemento{
			Producto: producto, Clase: Documento, ID: d.ID, Titulo: d.Title,
			Texto: textoDeDocumento(d.Kind, r.Estado), Fecha: cmp.Or(d.UpdatedAt, d.CreatedAt),
		})
	}
	return out, nil
}

// ----- Notas, contactos y eventos -----

func notas(filas []almacen.Fila) []Elemento {
	var out []Elemento
	for _, f := range filas {
		if f.Entidad != "note" {
			continue
		}
		var n struct {
			ID         string     `json:"id"`
			Title      string     `json:"title"`
			Content    string     `json:"content"`
			UpdatedAt  time.Time  `json:"updated_at"`
			DeletedAt  *time.Time `json:"deleted_at"`
			ArchivedAt *time.Time `json:"archived_at"`
		}
		if json.Unmarshal(f.JSON, &n) != nil || n.DeletedAt != nil {
			continue
		}
		texto := htmltext.Text(n.Content)
		out = append(out, Elemento{
			Producto: "notes", Clase: Nota, ID: cmp.Or(n.ID, f.ID), Titulo: n.Title,
			Detalle: htmltext.Summary(n.Content, 120), Texto: texto, Fecha: cmp.Or(n.UpdatedAt, f.Fecha),
		})
	}
	return out
}

// agregado es el estado de un contacto o un calendario: varias filas de dominio, cada
// una con su JSON en base64 (`contacts.Aggregate`, `calendar` igual).
type agregado struct {
	Rows []struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
		JSON []byte `json:"json"`
	} `json:"rows"`
}

func contactos(filas []almacen.Fila) []Elemento {
	var out []Elemento
	for _, f := range filas {
		var a agregado
		if f.Entidad != "contact_state" || json.Unmarshal(f.JSON, &a) != nil {
			continue
		}
		var e Elemento
		var extra []string
		vivo := false
		for _, r := range a.Rows {
			var c struct {
				ID            string     `json:"id"`
				FormattedName string     `json:"formatted_name"`
				Organization  string     `json:"organization"`
				Title         string     `json:"title"`
				Nickname      string     `json:"nickname"`
				Note          string     `json:"note"`
				Email         string     `json:"email"`
				Number        string     `json:"number"`
				Value         string     `json:"value"`
				UpdatedAt     time.Time  `json:"updated_at"`
				DeletedAt     *time.Time `json:"deleted_at"`
			}
			if r.Kind == "card" || json.Unmarshal(r.JSON, &c) != nil {
				continue
			}
			if r.Kind == "contact" {
				if c.DeletedAt != nil {
					break
				}
				vivo = true
				e = Elemento{
					Producto: "contacts", Clase: Contacto, ID: cmp.Or(c.ID, r.ID), Titulo: c.FormattedName,
					Detalle: strings.Join(noVacios(c.Organization, c.Title), " · "), Fecha: cmp.Or(c.UpdatedAt, f.Fecha),
				}
				extra = append(extra, c.Nickname, c.Note)
				continue
			}
			extra = append(extra, c.Email, c.Number, c.Value)
		}
		if !vivo {
			continue
		}
		e.Texto = strings.Join(noVacios(extra...), "\n")
		if e.Detalle == "" {
			e.Detalle = strings.Join(noVacios(extra...), " · ")
		}
		out = append(out, e)
	}
	return out
}

// contactosDeLaLibreta son los contactos que se ven de la libreta SQLite de Contacts en
// dir, si la hay (hay es falso si Contacts sigue en el almacén de registros). Sin ningún
// cerrojo y sin escribir nada: la base se abre en solo lectura.
func contactosDeLaLibreta(dir string) (es []Elemento, hay bool, err error) {
	ctx, fin := context.WithTimeout(context.Background(), PlazoCorreo)
	defer fin()
	// El sujeto no hace falta para leer: la base solo lo usa al escribir.
	lib, err := libreta.AbrirLectura(ctx, dir, "lector")
	if errors.Is(err, libreta.ErrSinMigrar) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = lib.Cerrar() }()
	cs, err := lib.Contactos(ctx)
	for _, c := range cs {
		extra := []string{c.Nickname, c.Note}
		for _, e := range c.Emails {
			extra = append(extra, e.Email)
		}
		for _, t := range c.Phones {
			extra = append(extra, t.Phone)
		}
		e := Elemento{
			Producto: "contacts", Clase: Contacto, ID: c.ID, Titulo: c.FormattedName,
			Detalle: strings.Join(noVacios(c.Organization, c.Title), " · "), Fecha: c.UpdatedAt,
			Texto: strings.Join(noVacios(extra...), "\n"),
		}
		if e.Detalle == "" {
			e.Detalle = strings.Join(noVacios(extra...), " · ")
		}
		es = append(es, e)
	}
	return es, true, err
}

func eventos(filas []almacen.Fila) []Elemento {
	var out []Elemento
	for _, f := range filas {
		var a agregado
		if f.Entidad != "object_state" || json.Unmarshal(f.JSON, &a) != nil {
			continue
		}
		for _, r := range a.Rows {
			if r.Kind != "event" {
				continue
			}
			var ev struct {
				ID           string     `json:"id"`
				Summary      string     `json:"summary"`
				Location     string     `json:"location"`
				Description  string     `json:"description"`
				Start        time.Time  `json:"dtstart"`
				DeletedAt    *time.Time `json:"deleted_at"`
				RecurrenceID *string    `json:"recurrence_id"`
			}
			// Las excepciones de una serie repiten el título: basta con la serie.
			if json.Unmarshal(r.JSON, &ev) != nil || ev.DeletedAt != nil || ev.RecurrenceID != nil {
				continue
			}
			out = append(out, Elemento{
				Producto: "calendar", Clase: Evento, ID: cmp.Or(ev.ID, r.ID), Titulo: ev.Summary,
				Detalle: ev.Location, Texto: ev.Description, Fecha: ev.Start,
			})
		}
	}
	return out
}

func noVacios(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ----- Ficheros de Drive -----

// drive son los ficheros de la raíz de Drive y los abiertos hace poco desde fuera de
// ella (`local/archivos`: su índice, en claro). Sin carpeta, nada.
func drive(raiz string) ([]Elemento, error) {
	vistos := map[string]bool{}
	var out []Elemento
	agregar := func(ruta string, fecha time.Time) {
		if vistos[ruta] {
			return
		}
		p, ok := ProductoDeFichero(ruta)
		if !ok {
			p = "drive"
		}
		vistos[ruta] = true
		out = append(out, Elemento{Producto: p, Clase: Fichero, Titulo: filepath.Base(ruta), Detalle: carpetaVista(raiz, ruta), Fecha: fecha, Ruta: ruta})
	}
	var ind struct {
		Recientes []struct {
			Ruta    string    `json:"ruta"`
			Abierto time.Time `json:"abierto"`
		} `json:"recientes"`
	}
	if raw, err := os.ReadFile(filepath.Join(raiz, ".sytue", "indice.json")); err == nil { //nolint:gosec // G304: el índice de Drive local
		_ = json.Unmarshal(raw, &ind)
	}
	for _, r := range ind.Recientes {
		if info, err := os.Stat(r.Ruta); err == nil && info.Mode().IsRegular() && filepath.IsAbs(r.Ruta) {
			agregar(r.Ruta, r.Abierto)
		}
	}
	n := 0
	err := filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			if ruta == raiz {
				return err
			}
			return nil // una carpeta que no se deja leer no para el recorrido
		}
		if strings.HasPrefix(d.Name(), ".") && ruta != raiz {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		n++
		if n > MaxFicherosDrive {
			return filepath.SkipAll
		}
		if info, err := d.Info(); err == nil && !vistos[ruta] {
			agregar(ruta, info.ModTime())
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return out, err
}

// carpetaVista es la carpeta de un fichero tal como se enseña: dentro de Drive, desde su
// raíz («Drive/Obras»); fuera, entera.
func carpetaVista(raiz, ruta string) string {
	carpeta := filepath.Dir(ruta)
	if rel, err := filepath.Rel(raiz, carpeta); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join(filepath.Base(raiz), rel)
	}
	return carpeta
}

// extensiones es qué app de la edición abre cada extensión: las que declara cada
// `cmd-local/<app>/FyneApp.toml` (`[Development] extensiones`).
var extensiones = map[string]string{
	"docx": "docs", "dotx": "docs", "odt": "docs", "ott": "docs", "pages": "docs", "txt": "docs", "md": "docs", "html": "docs",
	"xlsx": "sheets", "xltx": "sheets", "ods": "sheets", "ots": "sheets", "numbers": "sheets", "csv": "sheets", "tsv": "sheets",
	"pptx": "slides", "potx": "slides", "odp": "slides", "otp": "slides", "key": "slides",
	"docus": "docs", "calcus": "sheets", "diapos": "slides", // los nativos de Sytue Office
	"pdf": "pdf",
	"jpg": "photos", "jpeg": "photos", "png": "photos", "gif": "photos",
	"webm": "cast", "mkv": "cast", "mp4": "cast", "m4v": "cast", "mov": "cast", "avi": "cast",
	"eml": "mail", "ics": "calendar", "vcf": "contacts", "onex": "notes", "enex": "notes",
}

// ProductoDeFichero es la app de la edición que abre ese fichero, por su extensión.
func ProductoDeFichero(nombre string) (string, bool) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(nombre), "."))
	p, ok := extensiones[ext]
	return p, ok
}
