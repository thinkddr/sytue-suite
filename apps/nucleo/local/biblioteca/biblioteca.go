// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package biblioteca es el almacén de documentos de Sytue Office, la edición local de la
// ofimática: Docus, Calcus y Diapos sin cuenta, sin cifrado y sin servidor.
//
// Implementa [api.DocsData] y [api.SlidesData], las mismas interfaces que cumple el
// repositorio privado de la edición conectada (`apps/nucleo/office`), así que las
// pantallas no cambian: lo que cambia es lo que se les enchufa (PLAN-EDICION-LOCAL D-04).
//
// # Por qué no el motor de registros
//
// El plan proponía (D-06) reutilizar `office.Repository` sobre un puerto de
// `records.Crypto` en claro. Ese motor exige un diario SELLADO (`Journal.Encrypted()`),
// referencias con forma de Matrix y un estado de migración desde el servicio; para una
// edición que por requisito no cifra ni tiene servicio, todo eso es accidente. Aquí la
// unidad es un documento: un fichero JSON legible con sus metadatos, su estado (el
// `model.State` de siempre), sus comentarios y, al lado, sus versiones. Lo que SÍ se
// reutiliza entero es el dominio: el CRDT (`model.State.Apply`), la importación y la
// exportación de `office/model`. Lo que hace el repositorio conectado con ellos son unas
// pocas líneas por método, y están copiadas en su comportamiento, no en su almacén.
//
// # En disco
//
//	<raíz>/<producto>/<id>.json              documento, estado y comentarios
//	<raíz>/<producto>/<id>.versiones/<v>.json una versión guardada
//
// Un documento puede estar además anclado a un fichero del disco (`.docus`, `.docx`…):
// ver fichero.go. Papelera, copias, plantillas, temas y bloques, en opcionales.go.
//
// Cada escritura va a un temporal y se renombra: un corte a mitad no deja un fichero a
// medias (R-NO-CORROMPE). Todo está en claro: la protección en reposo es la del disco.
package biblioteca

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
	"git.sytue.eu/sytue/one/apps/nucleo/office/ancla"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

var (
	_ api.DocsData   = (*Biblioteca)(nil)
	_ api.SlidesData = (*Biblioteca)(nil)
)

// ErrProducto: el producto no es uno de los tres de la ofimática.
var ErrProducto = errors.New("biblioteca: producto desconocido")

// ErrSinCompartir: compartir necesita un servidor; en local no existe.
var ErrSinCompartir = errors.New("biblioteca: compartir no está disponible sin conexión a un servicio")

// Persona es quien firma lo que se escribe: el perfil local (D-30).
type Persona struct {
	// ID es estable por instalación; va en `owner_sub` y en el autor de comentarios.
	ID string
	// Nombre es lo que se enseña como autor.
	Nombre string
}

// Biblioteca guarda los documentos de un producto ("docs", "sheets" o "slides").
type Biblioteca struct {
	dir     string
	tipo    api.DocKind
	persona Persona
	// Idioma es la lengua del PDF accesible exportado («es» si va vacía).
	Idioma string

	mu   sync.Mutex
	docs map[string]*registro
	// ops es el diario de esta sesión, para DocumentOpsSince. No se guarda: sin
	// coedición nadie más lo pide, y el estado ya las lleva aplicadas.
	ops map[string][]api.DocOpSecuenciada
	// ultimo es el último documento cuyo contenido se leyó o se guardó: el que está
	// abierto en el editor, para «Guardar en la nube».
	ultimo string
	// ficheros pone en fila lo que lee o escribe ficheros de la persona (fichero.go):
	// dos aperturas del mismo fichero a la vez no dan dos documentos.
	ficheros sync.Mutex
}

type registro struct {
	Formato     int              `json:"formato"`
	Documento   api.Document     `json:"documento"`
	Estado      json.RawMessage  `json:"estado,omitempty"`
	Comentarios []api.DocComment `json:"comentarios,omitempty"`
	// Fichero es el fichero del disco al que está anclado (fichero.go); nil si ninguno.
	Fichero   *Fichero             `json:"fichero,omitempty"`
	estado    *model.State         // cargado a demanda
	versiones map[string]versionEn // índice cargado a demanda
}

type versionEn struct {
	Version api.DocVersion  `json:"version"`
	Estado  json.RawMessage `json:"estado"`
}

const formatoRegistro = 1

// Producto devuelve el tipo de documento de un producto de la ofimática.
func Producto(producto string) (api.DocKind, bool) {
	switch producto {
	case "docs":
		return api.DocText, true
	case "sheets":
		return api.DocSheet, true
	case "slides":
		return api.DocSlides, true
	}
	return "", false
}

// Abrir abre (y crea si falta) la biblioteca de producto bajo raiz.
func Abrir(raiz, producto string, p Persona) (*Biblioteca, error) {
	tipo, ok := Producto(producto)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProducto, producto)
	}
	if p.ID == "" {
		p.ID = "local"
	}
	dir := filepath.Join(raiz, producto)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	b := &Biblioteca{dir: dir, tipo: tipo, persona: p, docs: map[string]*registro{}, ops: map[string][]api.DocOpSecuenciada{}}
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entradas {
		nombre := e.Name()
		if e.IsDir() || !strings.HasSuffix(nombre, ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, nombre))
		if err != nil {
			return nil, err
		}
		var r registro
		if err := json.Unmarshal(raw, &r); err != nil || r.Documento.ID == "" {
			// Un fichero que no se entiende no tumba la biblioteca: se deja donde está.
			continue
		}
		b.docs[r.Documento.ID] = &r
	}
	return b, nil
}

// Dir es la carpeta del producto, para enseñarla en Ajustes.
func (b *Biblioteca) Dir() string { return b.dir }

// Tipo es el tipo de documento que guarda.
func (b *Biblioteca) Tipo() api.DocKind { return b.tipo }

// PonerPersona cambia quién firma lo que se escriba a partir de ahora.
func (b *Biblioteca) PonerPersona(p Persona) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if p.ID == "" {
		p.ID = b.persona.ID
	}
	b.persona = p
}

func noEncontrado(id string) error {
	return &api.Error{Status: http.StatusNotFound, URL: "/documents/" + id}
}

func (b *Biblioteca) ruta(id string) string          { return filepath.Join(b.dir, id+".json") }
func (b *Biblioteca) rutaVersiones(id string) string { return filepath.Join(b.dir, id+".versiones") }

// vivo devuelve el registro de un documento que no está en la papelera. Con b.mu.
func (b *Biblioteca) vivo(id string) (*registro, error) {
	r, ok := b.docs[id]
	if !ok || r.Documento.DeletedAt != nil {
		return nil, noEncontrado(id)
	}
	return r, nil
}

func (r *registro) cargarEstado() (*model.State, error) {
	if r.estado != nil {
		return r.estado, nil
	}
	if len(r.Estado) == 0 {
		r.estado = model.NewState()
		return r.estado, nil
	}
	s, err := model.StateFromJSON(r.Estado)
	if err != nil {
		return nil, err
	}
	r.estado = s
	return s, nil
}

// guardar escribe el registro entero con temporal y renombrado. Con b.mu.
func (b *Biblioteca) guardar(r *registro) error {
	if r.estado != nil {
		raw, err := r.estado.JSON()
		if err != nil {
			return err
		}
		r.Estado = raw
	}
	r.Formato = formatoRegistro
	raw, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		return err
	}
	return EscribirAtomico(b.ruta(r.Documento.ID), raw)
}

// EscribirAtomico escribe datos en ruta sin dejar nunca un fichero a medias: temporal en
// la misma carpeta, sincronizado, y renombrado encima.
func EscribirAtomico(ruta string, datos []byte) error {
	dir := filepath.Dir(ruta)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(ruta)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op tras el renombrado
	if _, err := f.Write(datos); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, ruta)
}

// ListDocuments lista los documentos vivos que cumplen f.
func (b *Biblioteca) ListDocuments(_ context.Context, f api.DocsFilter) ([]api.Document, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	q := strings.TrimSpace(f.Query)
	var out []api.Document
	for _, r := range b.docs {
		d := r.Documento
		if d.DeletedAt != nil || (f.Kind != "" && d.Kind != f.Kind) || (f.Starred && !d.Starred) {
			continue
		}
		// «Compartidos conmigo» no tiene nada en local: todo es de quien usa el equipo.
		if f.Scope == api.DocsShared {
			continue
		}
		if q != "" && !buscar.Contiene(d.Title, q) {
			continue
		}
		out = append(out, d)
	}
	if f.ByTitle {
		slices.SortFunc(out, func(a, c api.Document) int {
			if x := cmp.Compare(strings.ToLower(a.Title), strings.ToLower(c.Title)); x != 0 {
				return x
			}
			return cmp.Compare(a.ID, c.ID)
		})
	} else {
		slices.SortFunc(out, func(a, c api.Document) int {
			if x := c.UpdatedAt.Compare(a.UpdatedAt); x != 0 {
				return x
			}
			return cmp.Compare(a.ID, c.ID)
		})
	}
	if f.Cursor != "" {
		if i := slices.IndexFunc(out, func(d api.Document) bool { return d.ID == f.Cursor }); i >= 0 {
			out = out[i+1:]
		}
	}
	limite := f.Limit
	if limite <= 0 {
		limite = 50
	} else if limite > 200 {
		limite = 200
	}
	next := ""
	if len(out) > limite {
		out = out[:limite]
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

// CreateDocumentFromSource crea un documento, o devuelve el anclado a fuente si ya existe.
func (b *Biblioteca) CreateDocumentFromSource(ctx context.Context, kind api.DocKind, titulo, fuente string) (api.Document, bool, error) {
	if kind == "" {
		kind = b.tipo
	}
	if kind != b.tipo {
		return api.Document{}, false, model.ErrInvalidKind
	}
	fuente = strings.TrimSpace(fuente)
	if fuente != "" {
		if d, ok, err := b.DocumentBySource(ctx, fuente); err != nil || ok {
			return d, false, err
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ahora := time.Now().UTC()
	r := &registro{Documento: api.Document{
		ID: uuid.NewString(), Kind: kind, Title: titulo, OwnerSub: b.persona.ID,
		CreatedAt: ahora, UpdatedAt: ahora, SourceRef: fuente,
	}}
	if err := b.guardar(r); err != nil {
		return api.Document{}, false, err
	}
	b.docs[r.Documento.ID] = r
	return r.Documento, true, nil
}

// DocumentBySource busca el documento vivo anclado a fuente.
func (b *Biblioteca) DocumentBySource(_ context.Context, fuente string) (api.Document, bool, error) {
	fuente = strings.TrimSpace(fuente)
	if fuente == "" {
		return api.Document{}, false, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, r := range b.docs {
		if r.Documento.DeletedAt == nil && r.Documento.SourceRef == fuente {
			return r.Documento, true, nil
		}
	}
	return api.Document{}, false, nil
}

// GetDocument devuelve un documento y el papel de quien pregunta: siempre «owner».
func (b *Biblioteca) GetDocument(_ context.Context, id string) (api.Document, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return api.Document{}, "", err
	}
	return r.Documento, "owner", nil
}

// ImportDocument convierte un fichero ofimático al modelo, sin red: es `office/model`.
func (b *Biblioteca) ImportDocument(_ context.Context, nombre string, datos []byte, comentarios bool) (api.Importado, error) {
	return Importar(nombre, datos, comentarios)
}

// Importar convierte un fichero (docx, odt, xlsx, ods, pptx, odp, csv, md, html…) al
// modelo de la suite. Es lo que hace el servicio en `POST /import`, en el dispositivo.
func Importar(nombre string, datos []byte, comentarios bool) (api.Importado, error) {
	if len(datos) > api.MaxImportar {
		return api.Importado{}, fmt.Errorf("biblioteca: %q supera los %d MiB que se importan", nombre, api.MaxImportar>>20)
	}
	var (
		res *model.ImportResult
		err error
	)
	if comentarios {
		res, err = model.ImportWithComments(nombre, datos)
	} else {
		res, err = model.Import(nombre, datos)
	}
	if err != nil {
		return api.Importado{}, err
	}
	out := api.Importado{Kind: api.DocKind(res.Kind), Title: res.Title, Content: res.Content, Template: res.Template, Truncated: res.Truncated}
	for _, c := range res.Comments {
		ic := api.ImportadoComentario{ID: c.ID, ParentID: c.ParentID, Text: c.Text, Quote: c.Quote, Anchored: c.Anchored, Resolved: c.Resolved}
		if c.Cell != nil {
			ic.Cell = &api.ImportadoCelda{Hoja: c.Cell.Hoja, Fila: c.Cell.Fila, Col: c.Cell.Col, Text: c.Cell.Text}
		}
		out.Comments = append(out.Comments, ic)
	}
	return out, nil
}

// Recortes de los comentarios importados, los del servicio (`guardarImportado`).
const (
	maxCuerpoImportado = 20000
	maxCitaImportada   = 1000
)

func recortar(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// ImportComments guarda los comentarios de un fichero importado en el documento id y
// devuelve los ids de los anclados que se guardaron.
func (b *Biblioteca) ImportComments(_ context.Context, id string, comentarios []api.ImportadoComentario) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return nil, err
	}
	ahora := time.Now().UTC()
	nuevos := map[string]string{}
	var anclados []string
	for _, c := range comentarios {
		if c.ParentID != "" {
			continue
		}
		cuerpo := strings.TrimSpace(recortar(c.Text, maxCuerpoImportado))
		if cuerpo == "" || (c.Anchored && !ancla.EsMarca(c.ID)) {
			continue
		}
		cid := uuid.NewString()
		if u, err := uuid.Parse(c.ID); err == nil && u != uuid.Nil {
			cid = u.String()
		}
		var datos map[string]string
		switch {
		case c.Anchored:
			datos = map[string]string{"mark": c.ID, "text": recortar(c.Quote, maxCitaImportada)}
		case c.Cell != nil:
			datos = ancla.DeCelda(c.Cell.Hoja, c.Cell.Fila, c.Cell.Col, c.Cell.Text)
		}
		var anchor json.RawMessage
		if datos != nil {
			if raw, err := json.Marshal(datos); err == nil && ancla.Validar(raw) == "" {
				anchor = raw
			}
		}
		r.Comentarios = append(r.Comentarios, b.comentario(cid, "", cuerpo, anchor, c.Resolved, ahora))
		nuevos[c.ID] = cid
		if c.Anchored {
			anclados = append(anclados, c.ID)
		}
	}
	for _, c := range comentarios {
		padre, ok := nuevos[c.ParentID]
		cuerpo := strings.TrimSpace(recortar(c.Text, maxCuerpoImportado))
		if c.ParentID == "" || !ok || cuerpo == "" {
			continue
		}
		r.Comentarios = append(r.Comentarios, b.comentario(uuid.NewString(), padre, cuerpo, nil, c.Resolved, ahora))
	}
	return anclados, b.guardar(r)
}

func (b *Biblioteca) comentario(id, padre, cuerpo string, anchor json.RawMessage, resuelto bool, t time.Time) api.DocComment {
	return api.DocComment{ID: id, ParentID: padre, AuthorSub: b.persona.ID, AuthorName: b.persona.Nombre, Body: cuerpo, Resolved: resuelto, CreatedAt: t, UpdatedAt: t, Anchor: anchor}
}

// cambiar aplica f al documento vivo id y lo guarda.
func (b *Biblioteca) cambiar(id string, f func(*registro)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return err
	}
	f(r)
	return b.guardar(r)
}

// RenameDocument cambia el título.
func (b *Biblioteca) RenameDocument(_ context.Context, id, titulo string) error {
	return b.cambiar(id, func(r *registro) { r.Documento.Title = titulo; r.Documento.UpdatedAt = time.Now().UTC() })
}

// DeleteDocument manda el documento a la papelera: el fichero se queda, marcado.
func (b *Biblioteca) DeleteDocument(_ context.Context, id string) error {
	return b.cambiar(id, func(r *registro) {
		ahora := time.Now().UTC()
		r.Documento.DeletedAt = &ahora
		r.Documento.UpdatedAt = ahora
	})
}

// StarDocument destaca o quita el destacado.
func (b *Biblioteca) StarDocument(_ context.Context, id string, destacado bool) error {
	return b.cambiar(id, func(r *registro) { r.Documento.Starred = destacado })
}

// DocumentContent materializa el registro clave del estado del documento id.
func (b *Biblioteca) DocumentContent(_ context.Context, id, clave string) (api.DocContent, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.contenido(id, clave)
}

func (b *Biblioteca) contenido(id, clave string) (api.DocContent, error) {
	r, err := b.vivo(id)
	if err != nil {
		return api.DocContent{}, err
	}
	b.ultimo = id
	st, err := r.cargarEstado()
	if err != nil {
		return api.DocContent{}, err
	}
	out := api.DocContent{MaxLamport: st.Lamport, Seq: r.Documento.Seq}
	for _, el := range st.Elements {
		raw, err := json.Marshal(el)
		if err != nil {
			return api.DocContent{}, err
		}
		out.Elementos = append(out.Elementos, raw)
	}
	if reg, ok := st.Registers[clave]; ok {
		out.Dot = api.DocDot{Actor: reg.Dot.Actor, Counter: reg.Dot.Counter}
		out.Lamport = reg.Lamport
		_ = json.Unmarshal(reg.Value, &out.Value)
	}
	return out, nil
}

// aplicar aplica ops al documento id, como `applyAndPersistOps` del repositorio
// conectado: mismo CRDT, misma numeración, mismo conflicto por baseSeq. Con b.mu.
func (b *Biblioteca) aplicar(id string, ops []model.Op, actor string, baseSeq int64, comprobar bool) (int64, error) {
	r, err := b.vivo(id)
	if err != nil {
		return 0, err
	}
	if comprobar && baseSeq > 0 && r.Documento.Seq > baseSeq {
		return 0, &api.Error{Status: http.StatusConflict, URL: "/documents/" + id + "/ops"}
	}
	st, err := r.cargarEstado()
	if err != nil {
		return 0, err
	}
	// Se trabaja sobre una copia: si una op falla, el documento queda como estaba.
	raw, err := st.JSON()
	if err != nil {
		return 0, err
	}
	copia, err := model.StateFromJSON(raw)
	if err != nil {
		return 0, err
	}
	if actor == "" {
		actor = b.persona.ID
	}
	seq := r.Documento.Seq
	var diario []api.DocOpSecuenciada
	for _, op := range ops {
		if op.Dot.Actor == "" {
			op.Dot.Actor = actor
		}
		if op.Lamport <= copia.Lamport {
			op.Lamport = copia.Lamport + 1
		}
		if op.Dot.Counter == 0 {
			op.Dot.Counter = op.Lamport
		}
		if _, err := copia.Apply(op); err != nil {
			return 0, err
		}
		seq++
		copia.Seq = seq
		rawOp, err := json.Marshal(op)
		if err != nil {
			return 0, err
		}
		diario = append(diario, api.DocOpSecuenciada{Seq: seq, Actor: op.Dot.Actor, Op: rawOp})
	}
	antes, antesSeq, antesFecha := r.estado, r.Documento.Seq, r.Documento.UpdatedAt
	r.estado = copia
	r.Documento.Seq = seq
	r.Documento.UpdatedAt = time.Now().UTC()
	if err := b.guardar(r); err != nil {
		r.estado, r.Documento.Seq, r.Documento.UpdatedAt = antes, antesSeq, antesFecha
		return 0, err
	}
	b.ops[id] = append(b.ops[id], diario...)
	return seq, nil
}

func opsDeRegistro(clave, valor string, dot api.DocDot, lamport uint64, autoritativo bool) ([]model.Op, error) {
	var ops []model.Op
	if autoritativo {
		raw, err := json.Marshal(lamport)
		if err != nil {
			return nil, err
		}
		ops = append(ops, model.Op{Kind: model.OpSet, Dot: model.Dot{Actor: dot.Actor, Counter: dot.Counter}, Key: coedicion.ClaveLWW(clave), Value: raw, Lamport: lamport})
	}
	raw, err := json.Marshal(valor)
	if err != nil {
		return nil, err
	}
	return append(ops, model.Op{Kind: model.OpSet, Dot: model.Dot{Actor: dot.Actor, Counter: dot.Counter}, Key: clave, Value: raw, Lamport: lamport}), nil
}

// SaveDocumentContent escribe el registro LWW clave del documento id.
func (b *Biblioteca) SaveDocumentContent(_ context.Context, id, clave, valor string, dot api.DocDot, lamport uint64, autoritativo bool, baseSeq int64) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if dot.Actor == "" {
		dot.Actor = b.persona.ID
	}
	ops, err := opsDeRegistro(clave, valor, dot, lamport, autoritativo)
	if err != nil {
		return 0, err
	}
	return b.aplicar(id, ops, dot.Actor, baseSeq, true)
}

// SaveDocumentCoedition escribe las operaciones por carácter y el registro que las acompaña.
func (b *Biblioteca) SaveDocumentCoedition(_ context.Context, id, clave, valor string, dot api.DocDot, lamport uint64, autoritativo bool, chars []coedicion.Op) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if dot.Actor == "" {
		dot.Actor = b.persona.ID
	}
	ops := make([]model.Op, 0, len(chars)+2)
	for _, c := range chars {
		raw, err := json.Marshal(c)
		if err != nil {
			return 0, err
		}
		var op model.Op
		if err := json.Unmarshal(raw, &op); err != nil {
			return 0, err
		}
		if op.Lamport == 0 {
			op.Lamport = max(lamport, 1)
		}
		ops = append(ops, op)
	}
	reg, err := opsDeRegistro(clave, valor, dot, lamport, autoritativo)
	if err != nil {
		return 0, err
	}
	return b.aplicar(id, append(ops, reg...), dot.Actor, 0, false)
}

// Ultimo devuelve el documento abierto más reciente (id y título); "" si ninguno.
func (b *Biblioteca) Ultimo() (string, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, err := b.vivo(b.ultimo); err == nil {
		return r.Documento.ID, r.Documento.Title
	}
	return "", ""
}

// DocumentOpsSince devuelve las operaciones de esta sesión posteriores a desde.
func (b *Biblioteca) DocumentOpsSince(_ context.Context, id string, desde int64) ([]api.DocOpSecuenciada, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := b.vivo(id); err != nil {
		return nil, err
	}
	var out []api.DocOpSecuenciada
	for _, op := range b.ops[id] {
		if op.Seq > desde {
			out = append(out, op)
		}
	}
	return out, nil
}

// DocumentPresence: en local solo está quien usa el equipo.
func (b *Biblioteca) DocumentPresence(_ context.Context, id, conn string) ([]api.DocMiembro, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := b.vivo(id); err != nil {
		return nil, err
	}
	return []api.DocMiembro{{Conn: conn, Sub: b.persona.ID}}, nil
}

// SaveDocumentChecked guarda solo si base sigue siendo la cabeza.
func (b *Biblioteca) SaveDocumentChecked(_ context.Context, id, clave, valor string, base api.DocContent, dot api.DocDot, autoritativo bool) (api.DocContent, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ahora, err := b.contenido(id, clave)
	if err != nil {
		return base, err
	}
	if ahora.Dot != base.Dot || ahora.Lamport != base.Lamport {
		return base, api.ErrDocConflicto
	}
	lamport := max(ahora.MaxLamport, base.MaxLamport) + 1
	if dot.Actor == "" {
		dot.Actor = b.persona.ID
	}
	if dot.Counter == 0 {
		dot.Counter = lamport
	}
	ops, err := opsDeRegistro(clave, valor, dot, lamport, autoritativo)
	if err != nil {
		return base, err
	}
	seq, err := b.aplicar(id, ops, dot.Actor, base.Seq, true)
	var httpErr *api.Error
	if errors.As(err, &httpErr) && httpErr.Status == http.StatusConflict {
		return base, api.ErrDocConflicto
	}
	if err != nil {
		return base, err
	}
	// La base del guardado siguiente es el registro tal como quedó: con `autoritativo`,
	// la marca `<clave>:lww` se aplica antes con ese reloj y aplicar sube el del registro
	// uno más. Devolver el reloj pedido hacía que el guardado siguiente de la misma
	// ventana (el autoguardado de Docus encadena con esta respuesta) se viera como de
	// otro y fallara con conflicto (lo destapó la prueba de carga, carga_test.go).
	out := api.DocContent{Value: valor, Dot: dot, Lamport: lamport, MaxLamport: lamport, Seq: seq}
	if r, ok := b.docs[id]; ok && r.estado != nil {
		if reg, ok := r.estado.Registers[clave]; ok {
			out.Dot = api.DocDot{Actor: reg.Dot.Actor, Counter: reg.Dot.Counter}
			out.Lamport = reg.Lamport
		}
		out.MaxLamport = max(out.MaxLamport, r.estado.Lamport)
	}
	return out, nil
}

// cargarVersiones lee el índice de versiones de r. Con b.mu.
func (b *Biblioteca) cargarVersiones(r *registro) (map[string]versionEn, error) {
	if r.versiones != nil {
		return r.versiones, nil
	}
	r.versiones = map[string]versionEn{}
	dir := b.rutaVersiones(r.Documento.ID)
	entradas, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return r.versiones, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var v versionEn
		if json.Unmarshal(raw, &v) == nil && v.Version.ID != "" {
			r.versiones[v.Version.ID] = v
		}
	}
	return r.versiones, nil
}

// DocumentVersions lista las versiones guardadas, la más reciente primero.
func (b *Biblioteca) DocumentVersions(_ context.Context, id string) ([]api.DocVersion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return nil, err
	}
	vs, err := b.cargarVersiones(r)
	if err != nil {
		return nil, err
	}
	out := make([]api.DocVersion, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Version)
	}
	slices.SortFunc(out, func(a, c api.DocVersion) int {
		if x := c.CreatedAt.Compare(a.CreatedAt); x != 0 {
			return x
		}
		return cmp.Compare(c.Seq, a.Seq)
	})
	return out, nil
}

// CreateDocumentVersion guarda el estado actual como versión con nombre.
func (b *Biblioteca) CreateDocumentVersion(_ context.Context, id, label string) (api.DocVersion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return api.DocVersion{}, err
	}
	st, err := r.cargarEstado()
	if err != nil {
		return api.DocVersion{}, err
	}
	raw, err := st.JSON()
	if err != nil {
		return api.DocVersion{}, err
	}
	vs, err := b.cargarVersiones(r)
	if err != nil {
		return api.DocVersion{}, err
	}
	v := versionEn{Version: api.DocVersion{ID: uuid.NewString(), Label: strings.TrimSpace(label), AuthorSub: b.persona.ID, Seq: r.Documento.Seq, CreatedAt: time.Now().UTC()}, Estado: raw}
	enc, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return api.DocVersion{}, err
	}
	if err := EscribirAtomico(filepath.Join(b.rutaVersiones(id), v.Version.ID+".json"), enc); err != nil {
		return api.DocVersion{}, err
	}
	vs[v.Version.ID] = v
	return v.Version, nil
}

// RestoreDocumentVersion vuelve el contenido del documento al de la versión.
func (b *Biblioteca) RestoreDocumentVersion(_ context.Context, id, version string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return err
	}
	vs, err := b.cargarVersiones(r)
	if err != nil {
		return err
	}
	v, ok := vs[version]
	if !ok {
		return noEncontrado(id)
	}
	snap, err := model.StateFromJSON(v.Estado)
	if err != nil {
		return err
	}
	st, err := r.cargarEstado()
	if err != nil {
		return err
	}
	clave := api.DocContentKey(r.Documento.Kind)
	var valor string
	if reg, ok := snap.Registers[clave]; ok {
		_ = json.Unmarshal(reg.Value, &valor)
	}
	lam := st.Lamport + 1
	ops, err := opsDeRegistro(clave, valor, api.DocDot{Actor: b.persona.ID, Counter: lam}, lam, r.Documento.Kind == api.DocText)
	if err != nil {
		return err
	}
	_, err = b.aplicar(id, ops, b.persona.ID, 0, false)
	return err
}

// ExportDocument exporta a formato con `office/model`, como el servicio.
func (b *Biblioteca) ExportDocument(_ context.Context, id, formato string, plantilla bool, accesible *bool) ([]byte, error) {
	b.mu.Lock()
	r, err := b.vivo(id)
	if err != nil {
		b.mu.Unlock()
		return nil, err
	}
	st, err := r.cargarEstado()
	if err != nil {
		b.mu.Unlock()
		return nil, err
	}
	raw, err := st.JSON()
	titulo, kind := r.Documento.Title, model.Kind(r.Documento.Kind)
	comentarios := make([]model.Comment, 0, len(r.Comentarios))
	for _, c := range r.Comentarios {
		var padre *string
		if c.ParentID != "" {
			p := c.ParentID
			padre = &p
		}
		comentarios = append(comentarios, model.Comment{ID: c.ID, DocumentID: id, ParentID: padre, AuthorSub: c.AuthorSub, AuthorName: c.AuthorName, Anchor: c.Anchor, Body: c.Body, Resolved: c.Resolved, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt})
	}
	idioma := cmp.Or(b.Idioma, "es")
	b.mu.Unlock()
	if err != nil {
		return nil, err
	}
	// La conversión, fuera del cerrojo y sobre una copia del estado.
	copia, err := model.StateFromJSON(raw)
	if err != nil {
		return nil, err
	}
	f := model.BlobFormat(formato)
	if f == model.FormatPDF && !plantilla && (accesible == nil || *accesible) {
		datos, _, err := model.ExportPDFAccesible(kind, titulo, idioma, copia)
		return datos, err
	}
	datos, _, _, err := model.ExportWithComments(kind, titulo, copia, f, comentarios)
	if err != nil {
		return nil, err
	}
	if plantilla {
		datos, _, _, err = model.ComoPlantilla(kind, f, datos)
	}
	return datos, err
}

// DocumentSharing: nada compartido en local.
func (b *Biblioteca) DocumentSharing(ctx context.Context, id string) ([]api.DocPermission, []api.DocShare, error) {
	_, _, err := b.GetDocument(ctx, id)
	return nil, nil, err
}

// GrantDocumentPermission: sin servidor no hay a quién dar permiso.
func (b *Biblioteca) GrantDocumentPermission(context.Context, string, string, string, string) error {
	return ErrSinCompartir
}

// RevokeDocumentPermission: no hay permisos que quitar.
func (b *Biblioteca) RevokeDocumentPermission(context.Context, string, string, string) error {
	return ErrSinCompartir
}

// CreateDocumentShare: un enlace público necesita un servidor que lo sirva.
func (b *Biblioteca) CreateDocumentShare(context.Context, string, string) (api.DocShare, error) {
	return api.DocShare{}, ErrSinCompartir
}

// RevokeDocumentShare: no hay enlaces.
func (b *Biblioteca) RevokeDocumentShare(context.Context, string, string) error {
	return ErrSinCompartir
}

// DocumentComments lista los comentarios del documento id.
func (b *Biblioteca) DocumentComments(_ context.Context, id string, conResueltos bool) ([]api.DocComment, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return nil, err
	}
	var out []api.DocComment
	for _, c := range r.Comentarios {
		if conResueltos || !c.Resolved {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, c api.DocComment) int {
		if x := a.CreatedAt.Compare(c.CreatedAt); x != 0 {
			return x
		}
		return cmp.Compare(a.ID, c.ID)
	})
	return out, nil
}

// CreateDocumentComment añade un comentario o una respuesta.
func (b *Biblioteca) CreateDocumentComment(_ context.Context, id, padre, cuerpo string) (api.DocComment, error) {
	return b.crearComentario(id, padre, cuerpo, nil)
}

// CreateAnchoredDocumentComment añade un comentario anclado a una marca del texto.
func (b *Biblioteca) CreateAnchoredDocumentComment(_ context.Context, id, cuerpo, marca, texto string) (api.DocComment, error) {
	raw, err := json.Marshal(map[string]string{"mark": marca, "text": texto})
	if err != nil {
		return api.DocComment{}, err
	}
	return b.crearComentario(id, "", cuerpo, raw)
}

// CreateCellDocumentComment añade un comentario anclado a una celda.
func (b *Biblioteca) CreateCellDocumentComment(_ context.Context, id, cuerpo string, a api.AnclaCelda) (api.DocComment, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return api.DocComment{}, err
	}
	return b.crearComentario(id, "", cuerpo, raw)
}

func (b *Biblioteca) crearComentario(id, padre, cuerpo string, anchor json.RawMessage) (api.DocComment, error) {
	cuerpo = strings.TrimSpace(cuerpo)
	if cuerpo == "" {
		return api.DocComment{}, api.ErrComentarioVacio
	}
	// La misma regla que el servicio: un ancla que él rechazaría tampoco se guarda aquí,
	// o el documento exportado no se podría volver a importar tal cual.
	if motivo := ancla.Validar(anchor); motivo != "" {
		return api.DocComment{}, fmt.Errorf("%w: %s", api.ErrAnclaNoValida, motivo)
	}
	var c api.DocComment
	err := b.cambiar(id, func(r *registro) {
		c = b.comentario(uuid.NewString(), strings.TrimSpace(padre), cuerpo, anchor, false, time.Now().UTC())
		r.Comentarios = append(r.Comentarios, c)
	})
	return c, err
}

// ResolveDocumentComment marca o desmarca un comentario como resuelto.
func (b *Biblioteca) ResolveDocumentComment(_ context.Context, id, comentario string, resuelto bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, err := b.vivo(id)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(r.Comentarios, func(c api.DocComment) bool { return c.ID == comentario })
	if i < 0 {
		return noEncontrado(id)
	}
	r.Comentarios[i].Resolved = resuelto
	r.Comentarios[i].UpdatedAt = time.Now().UTC()
	return b.guardar(r)
}

// DeleteDocumentComment borra el comentario y sus respuestas directas.
func (b *Biblioteca) DeleteDocumentComment(_ context.Context, id, comentario string) error {
	return b.cambiar(id, func(r *registro) {
		r.Comentarios = slices.DeleteFunc(r.Comentarios, func(c api.DocComment) bool {
			return c.ID == comentario || c.ParentID == comentario
		})
	})
}

// ----- Diapos -----

func aDiapos(d api.Document) api.SlidesDocument {
	return api.SlidesDocument{ID: d.ID, Kind: string(d.Kind), Title: d.Title, OwnerSub: d.OwnerSub, Seq: d.Seq, Starred: d.Starred, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, SourceRef: d.SourceRef}
}

// ListSlides lista las presentaciones que cumplen q.
func (b *Biblioteca) ListSlides(ctx context.Context, q, cursor string) ([]api.SlidesDocument, string, error) {
	docs, next, err := b.ListDocuments(ctx, api.DocsFilter{Kind: api.DocSlides, Query: q, Cursor: cursor, Limit: api.SlidesPagina})
	if err != nil {
		return nil, "", err
	}
	out := make([]api.SlidesDocument, len(docs))
	for i, d := range docs {
		out[i] = aDiapos(d)
	}
	return out, next, nil
}

// CreateSlidePresentation crea una presentación.
func (b *Biblioteca) CreateSlidePresentation(ctx context.Context, titulo string) (api.SlidesDocument, error) {
	d, _, err := b.CreateDocumentFromSource(ctx, api.DocSlides, titulo, "")
	return aDiapos(d), err
}

// GetSlidePresentation devuelve una presentación.
func (b *Biblioteca) GetSlidePresentation(ctx context.Context, id string) (api.SlidesDocument, string, error) {
	d, rol, err := b.GetDocument(ctx, id)
	return aDiapos(d), rol, err
}

// RenameSlidePresentation cambia el título.
func (b *Biblioteca) RenameSlidePresentation(ctx context.Context, id, titulo string) error {
	return b.RenameDocument(ctx, id, titulo)
}

// DeleteSlidePresentation manda la presentación a la papelera.
func (b *Biblioteca) DeleteSlidePresentation(ctx context.Context, id string) error {
	return b.DeleteDocument(ctx, id)
}

// SlideContent lee el modelo de la presentación.
func (b *Biblioteca) SlideContent(ctx context.Context, id string) (api.SlidesContent, error) {
	c, err := b.DocumentContent(ctx, id, api.SlidesContentKey)
	if err != nil {
		return api.SlidesContent{}, err
	}
	return api.SlidesContent{Modelo: c.Value, Dot: api.SlidesDot(c.Dot), Lamport: c.MaxLamport, Seq: c.Seq}, nil
}

// SaveSlideContent guarda el modelo si base sigue siendo la cabeza.
func (b *Biblioteca) SaveSlideContent(ctx context.Context, id string, base api.SlidesContent, actor string, contador uint64, modelo string) (api.SlidesContent, error) {
	if len(modelo) > 1<<20 {
		return api.SlidesContent{}, api.ErrSlidesTooBig
	}
	ahora, err := b.SlideContent(ctx, id)
	if err != nil {
		return api.SlidesContent{}, err
	}
	if ahora.Dot != base.Dot {
		return ahora, api.ErrSlidesConflict
	}
	lamport := max(ahora.Lamport, base.Lamport) + 1
	if actor == "" {
		actor = b.persona.ID
	}
	if contador == 0 {
		contador = lamport
	}
	dot := api.DocDot{Actor: actor, Counter: contador}
	seq, err := b.SaveDocumentContent(ctx, id, api.SlidesContentKey, modelo, dot, lamport, false, base.Seq)
	var httpErr *api.Error
	if errors.As(err, &httpErr) && httpErr.Status == http.StatusConflict {
		if ultimo, e := b.SlideContent(ctx, id); e == nil {
			return ultimo, api.ErrSlidesConflict
		}
		return api.SlidesContent{}, api.ErrSlidesConflict
	}
	if err != nil {
		return api.SlidesContent{}, err
	}
	return api.SlidesContent{Modelo: modelo, Dot: api.SlidesDot(dot), Lamport: lamport, Seq: seq}, nil
}
