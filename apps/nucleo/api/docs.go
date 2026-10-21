// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/coedicion"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// Docs es el cliente de Sytue Docs (services/docs, `/api/v1`). Docus, Calcus y Diapos
// comparten servicio y documentos: lo que los separa es `kind`, y lo que guarda cada uno
// es UNA cadena en un registro LWW propio (DocContentKey). Este fichero trae lo común
// —listado por tipo, alta, renombrar, borrar, destacar, leer y guardar ese registro—;
// lo que solo entiende un producto va en su fichero.
type Docs struct{ c *Client }

// NewDocs crea el cliente sobre la raíz del servicio (ServiceURL(apex, "docs")).
func NewDocs(servicio string, s Session, h *http.Client) *Docs {
	return &Docs{c: New(servicio+"/api/v1", s, h)}
}

// DocKind es el tipo de documento (domain.Kind).
type DocKind string

// Los tres tipos del servicio. Un PDF no es un documento de este servicio: vive en Drive.
const (
	DocText   DocKind = "document"
	DocSheet  DocKind = "spreadsheet"
	DocSlides DocKind = "presentation"
)

// DocContentKey es la clave del registro LWW donde la web guarda el contenido de cada
// tipo (`CONTENT_KEY` de packages/office/src/editor/types.ts): HTML en un documento de texto,
// `JSON.stringify(modelo)` en hojas y presentaciones.
func DocContentKey(k DocKind) string {
	switch k {
	case DocSheet:
		return "doc:sheet"
	case DocSlides:
		return "doc:slides"
	default:
		return "doc:html"
	}
}

// Document son los metadatos de un documento (domain.Document). El contenido no va aquí.
type Document struct {
	ID        string    `json:"id"`
	Kind      DocKind   `json:"kind"`
	Title     string    `json:"title"`
	OwnerSub  string    `json:"owner_sub"`
	Seq       int64     `json:"seq"`
	Starred   bool      `json:"starred"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	SourceRef string    `json:"source_ref,omitempty"`
	// DeletedAt solo viene en la papelera (Papelera): cuándo se tiró.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// DocsFilter es lo que admite el listado. Kind vacío son todos los tipos.
type DocsFilter struct {
	Kind    DocKind
	Query   string // filtra por título
	Starred bool   // solo los destacados por quien pregunta
	Cursor  string // el `next_cursor` de la página anterior
	Limit   int    // el servicio acota a 1..200; 0 deja su defecto (50)
}

// List devuelve una página, lo modificado más recientemente primero, y el cursor de la
// siguiente ("" si no hay más).
func (d *Docs) List(ctx context.Context, f DocsFilter) ([]Document, string, error) {
	q := url.Values{}
	if f.Kind != "" {
		q.Set("kind", string(f.Kind))
	}
	if f.Query != "" {
		q.Set("q", f.Query)
	}
	if f.Starred {
		q.Set("starred", "1")
	}
	if f.Cursor != "" {
		q.Set("cursor", f.Cursor)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	out, err := JSON[struct {
		Documents  []Document `json:"documents"`
		NextCursor string     `json:"next_cursor"`
	}](ctx, d.c, Request{Path: "/documents", Query: q})
	return out.Documents, out.NextCursor, err
}

type docResp struct {
	Document Document `json:"document"`
	Role     string   `json:"role"`
}

// Create da de alta un documento vacío. El servicio le crea además su fichero en Drive.
func (d *Docs) Create(ctx context.Context, kind DocKind, titulo string) (Document, error) {
	doc, _, err := d.CreateDesde(ctx, kind, titulo, "")
	return doc, err
}

// CreateDesde es Create anclado a un fichero (`drive:<uuid>`), como `/open-from-drive` de
// la web: el servicio lo hace idempotente, así que dos aperturas a la vez dan UN documento.
// creado=false es que ya existía, y sembrarlo pisaría lo que su dueño lleve escrito. Con
// `fuente` vacía es Create, y creado siempre true.
func (d *Docs) CreateDesde(ctx context.Context, kind DocKind, titulo, fuente string) (doc Document, creado bool, err error) {
	cuerpo := map[string]string{"kind": string(kind), "title": titulo}
	if fuente != "" {
		cuerpo["source_ref"] = fuente
	}
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return Document{}, false, err
	}
	resp, err := d.c.Do(ctx, Request{
		Method: http.MethodPost, Path: "/documents", Body: datos,
		Header: http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}},
	})
	if err != nil {
		return Document{}, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out docResp
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSON)).Decode(&out); err != nil {
		return Document{}, false, err
	}
	return out.Document, resp.StatusCode == http.StatusCreated, nil
}

// BySource es el documento nacido de ese fichero (`drive:<uuid>`), o ok=false si nunca se
// ha abierto. Ahorra descargar y convertir el fichero para descubrirlo.
func (d *Docs) BySource(ctx context.Context, fuente string) (doc Document, ok bool, err error) {
	out, err := JSON[struct {
		Documents []Document `json:"documents"`
	}](ctx, d.c, Request{Path: "/documents", Query: url.Values{"source_ref": {fuente}, "limit": {"1"}}})
	if err != nil || len(out.Documents) == 0 {
		return Document{}, false, err
	}
	return out.Documents[0], true, nil
}

// Get devuelve el documento y el papel de quien pregunta: viewer, commenter, editor u owner.
func (d *Docs) Get(ctx context.Context, id string) (Document, string, error) {
	out, err := JSON[docResp](ctx, d.c, Request{Path: "/documents/" + url.PathEscape(id)})
	return out.Document, out.Role, err
}

// Importado es un fichero ofimático convertido al modelo de la suite: el
// `ImportResult` de services/docs. Content es el valor de DocContentKey(Kind): HTML
// para un texto, el JSON del modelo para una hoja o una presentación.
type Importado struct {
	Kind    DocKind `json:"kind"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	// Comments solo vienen de ImportarConComentarios: los del fichero, para guardarlos al
	// crear el documento (ImportComments). El HTML lleva el ancla de cada uno con texto.
	Comments []ImportadoComentario `json:"comments,omitempty"`
	// Template: el fichero era una plantilla (.dotx, .xltx…). Truncated: un libro que no
	// cabía entero y llega con las primeras filas. Ninguno de los dos se ancla al fichero.
	Template  bool `json:"template,omitempty"`
	Truncated bool `json:"truncated,omitempty"`
}

// ImportadoComentario es un comentario de un fichero importado (domain.ImportedComment).
// Text ya lleva la autoría original delante del cuerpo: los guarda quien importa.
type ImportadoComentario struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
	Text     string `json:"text"`
	Quote    string `json:"quote,omitempty"`
	Anchored bool   `json:"anchored"`
	Resolved bool   `json:"resolved,omitempty"`
}

// MaxImportar es lo más grande que convierte el servicio (`domain.MaxImportBytes`).
const MaxImportar = 25 << 20

// Importar convierte un fichero (.docx, .xlsx, .pptx, .odt…) sin crear ningún
// documento: el servicio lee, convierte y contesta, y no guarda nada. El formato lo
// decide la FIRMA del contenido, no la extensión; el nombre da el título.
//
// El multipart es el mismo que el del motor de PDF —campo «file», en memoria para
// poder repetirlo tras un 401—, así que se reutiliza su compositor.
func (d *Docs) Importar(ctx context.Context, nombre string, datos []byte) (Importado, error) {
	return d.importar(ctx, nombre, datos, false)
}

// ImportarConComentarios es Importar para quien va a CREAR el documento: devuelve además los
// comentarios del fichero y el HTML lleva su ancla. Quien lo llama tiene que guardarlos con
// [Docs.ImportComments] y sembrar [Importado.ContenidoConMarks]: un mark sin comentario queda
// resaltado para siempre.
func (d *Docs) ImportarConComentarios(ctx context.Context, nombre string, datos []byte) (Importado, error) {
	return d.importar(ctx, nombre, datos, true)
}

func (d *Docs) importar(ctx context.Context, nombre string, datos []byte, comentarios bool) (Importado, error) {
	r, err := formularioPDF("/import", nil, PDFFichero{Nombre: nombre, Datos: datos})
	if err != nil {
		return Importado{}, err
	}
	if comentarios {
		r.Query = url.Values{"comments": {"1"}}
	}
	return JSON[Importado](ctx, d.c, r)
}

// ImportComments guarda en el documento recién creado los comentarios de un fichero
// importado, de parte de quien importa, y devuelve los ids de los anclados que quedaron
// guardados.
func (d *Docs) ImportComments(ctx context.Context, id string, comentarios []ImportadoComentario) ([]string, error) {
	out, err := PostJSON[struct {
		Anchored []string `json:"anchored"`
	}](ctx, d.c, "/documents/"+url.PathEscape(id)+"/comments/import", map[string]any{"comments": comentarios})
	return out.Anchored, err
}

// reMark es un `<mark data-comment-id>` del folio; los ids los genera el servicio en hex.
var reMark = regexp.MustCompile(`<mark data-comment-id="([^"]*)">([\s\S]*?)</mark>`)

// ContenidoConMarks es el contenido para sembrar tras guardar los comentarios: solo con los
// marks de los `guardados` (los ids que dijo [Docs.ImportComments]) y sin los demás, que
// quedarían resaltados sin comentario. Sin guardados —o sin comentarios— no queda ninguno.
// Un contenido que no es un texto (hoja, presentación) va tal cual.
func (i Importado) ContenidoConMarks(guardados []string) string {
	if i.Kind != DocText {
		return i.Content
	}
	return reMark.ReplaceAllStringFunc(i.Content, func(m string) string {
		p := reMark.FindStringSubmatch(m)
		if slices.Contains(guardados, p[1]) {
			return m
		}
		return p[2]
	})
}

// CanWrite dice si ese papel deja escribir (domain.Role.CanWrite).
func CanWrite(role string) bool { return role == "editor" || role == "owner" }

func (d *Docs) sinCuerpo(ctx context.Context, metodo, ruta string, cuerpo any) error {
	r := Request{Method: metodo, Path: ruta}
	if cuerpo != nil {
		datos, err := json.Marshal(cuerpo)
		if err != nil {
			return err
		}
		r.Body, r.Header = datos, http.Header{"Content-Type": {"application/json"}}
	}
	resp, err := d.c.Do(ctx, r)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Rename cambia el título.
func (d *Docs) Rename(ctx context.Context, id, titulo string) error {
	return d.sinCuerpo(ctx, http.MethodPatch, "/documents/"+url.PathEscape(id), map[string]string{"title": titulo})
}

// Delete manda el documento a la papelera (y su fichero de Drive a la de Drive).
func (d *Docs) Delete(ctx context.Context, id string) error {
	return d.sinCuerpo(ctx, http.MethodDelete, "/documents/"+url.PathEscape(id), nil)
}

// Star lo marca o desmarca como destacado para quien pregunta.
func (d *Docs) Star(ctx context.Context, id string, destacado bool) error {
	return d.sinCuerpo(ctx, http.MethodPut, "/documents/"+url.PathEscape(id)+"/star", map[string]bool{"starred": destacado})
}

// DocDot es la identidad de una operación CRDT (domain.Dot): actor y contador.
type DocDot struct {
	Actor   string `json:"a"`
	Counter uint64 `json:"c"`
}

// DocContent es el registro de contenido tal y como está en el servidor. Dot y Lamport
// dicen QUIÉN lo escribió por última vez: si cambian entre abrir y guardar, otro ha
// guardado entretanto.
type DocContent struct {
	Value   string
	Dot     DocDot
	Lamport uint64
	// MaxLamport es el reloj más alto que ha visto el documento: un `set` nuevo tiene que
	// superarlo para ganar (domain.State.Apply).
	MaxLamport uint64
	// Elementos es la secuencia por carácter del estado, vivos y tumbas, sin interpretar:
	// lo que lee `coedicion.ElementosDeEstado`. Seq es la última op que cubre el estado,
	// desde donde se piden las siguientes.
	Elementos []json.RawMessage
	Seq       int64
}

// Content lee el registro `clave` del estado consolidado (GET …/state). Un documento
// recién creado no lo tiene: devuelve el cero, con el reloj del documento.
func (d *Docs) Content(ctx context.Context, id, clave string) (DocContent, error) {
	out, err := JSON[struct {
		Seq   int64 `json:"seq"`
		State struct {
			Lamport   uint64            `json:"lamport"`
			Elements  []json.RawMessage `json:"elements"`
			Registers map[string]struct {
				Value   json.RawMessage `json:"value"`
				Dot     DocDot          `json:"dot"`
				Lamport uint64          `json:"lamport"`
			} `json:"registers"`
		} `json:"state"`
	}](ctx, d.c, Request{Path: "/documents/" + url.PathEscape(id) + "/state"})
	if err != nil {
		return DocContent{}, err
	}
	c := DocContent{MaxLamport: out.State.Lamport, Elementos: out.State.Elements, Seq: out.Seq}
	if r, ok := out.State.Registers[clave]; ok {
		c.Dot, c.Lamport = r.Dot, r.Lamport
		_ = json.Unmarshal(r.Value, &c.Value) // un valor que no es cadena (null tras restaurar) es «vacío», como en la web
	}
	return c, nil
}

type docOp struct {
	Kind    string `json:"kind"`
	Dot     DocDot `json:"dot"`
	Key     string `json:"key"`
	Value   any    `json:"value"`
	Lamport uint64 `json:"lamport"`
}

// SaveContent escribe el registro `clave` por el camino de la web: POST …/ops con un `set`.
//
// autoritativo añade al lote el registro `<clave>:lww`. Es lo que manda la web cuando quien
// guarda NO lleva la secuencia por carácter del documento (editor/state/coedicion.ts): sin
// esa marca, un par que sí la lleva seguiría fusionando sobre un texto que ya no existe.
// Solo tiene sentido en documentos de texto; hojas y presentaciones van siempre sin ella.
func (d *Docs) SaveContent(ctx context.Context, id, clave, valor string, dot DocDot, lamport uint64, autoritativo bool) (int64, error) {
	return d.saveContentBase(ctx, id, clave, valor, dot, lamport, autoritativo, 0)
}

// saveContentBase es SaveContent con `base_seq` (0 = sin él): el servicio rechaza con 409,
// sin escribir nada, si otro escribió la clave después de esa seq. Es la comprobación que la
// web hace con su base y que aquí cierra la ventana entre mirar el registro y escribirlo.
func (d *Docs) saveContentBase(ctx context.Context, id, clave, valor string, dot DocDot, lamport uint64, autoritativo bool, baseSeq int64) (int64, error) {
	ops := []docOp{{Kind: "set", Dot: dot, Key: clave, Value: valor, Lamport: lamport}}
	if autoritativo {
		ops = append([]docOp{{Kind: "set", Dot: dot, Key: clave + ":lww", Value: lamport, Lamport: lamport}}, ops...)
	}
	cuerpo := map[string]any{"ops": ops}
	if baseSeq > 0 {
		cuerpo["base_seq"] = baseSeq
	}
	out, err := PostJSON[struct {
		Seq int64 `json:"seq"`
	}](ctx, d.c, "/documents/"+url.PathEscape(id)+"/ops", cuerpo)
	return out.Seq, err
}

// ErrLoteGrande: el lote con las ops por carácter no cabe en lo que acepta el servicio.
// Quien guarda renuncia a la secuencia y manda el registro solo, autoritativo.
var ErrLoteGrande = errors.New("docs: el cambio no cabe en un lote de ops")

// SaveCoedicion es el guardado de `buildOpsBody`: las ops por carácter que llevan la
// secuencia hasta `valor` y, SIEMPRE, el `set` del registro, que es lo que lee el
// servicio; con `autoritativo`, también la marca `<clave>:lww`. Da la seq del lote.
func (d *Docs) SaveCoedicion(ctx context.Context, id, clave, valor string, dot DocDot, lamport uint64, autoritativo bool, chars []coedicion.Op) (int64, error) {
	ops := make([]any, 0, len(chars)+2)
	for _, c := range chars {
		ops = append(ops, c)
	}
	if autoritativo {
		ops = append(ops, docOp{Kind: "set", Dot: dot, Key: coedicion.ClaveLWW(clave), Value: lamport, Lamport: lamport})
	}
	ops = append(ops, docOp{Kind: "set", Dot: dot, Key: clave, Value: valor, Lamport: lamport})
	cuerpo, err := json.Marshal(map[string]any{"ops": ops})
	if err != nil {
		return 0, err
	}
	if len(cuerpo) > coedicion.MaxCuerpo {
		return 0, ErrLoteGrande
	}
	out, err := JSON[struct {
		Seq int64 `json:"seq"`
	}](ctx, d.c, Request{
		Method: http.MethodPost, Path: "/documents/" + url.PathEscape(id) + "/ops",
		Header: http.Header{"Content-Type": {"application/json"}}, Body: cuerpo,
	})
	return out.Seq, err
}

// DocOpSecuenciada es una op del registro del documento (domain.SequencedOp): su seq,
// quién la mandó y la op sin interpretar.
type DocOpSecuenciada struct {
	Seq   int64           `json:"seq"`
	Actor string          `json:"actor"`
	Op    json.RawMessage `json:"op"`
}

// OpsDesde son las ops posteriores a `desde` (GET …/ops?since=): el sondeo con el que la
// web se entera de lo que escriben los demás cuando no tiene el WebSocket.
func (d *Docs) OpsDesde(ctx context.Context, id string, desde int64) ([]DocOpSecuenciada, error) {
	out, err := JSON[struct {
		Ops []DocOpSecuenciada `json:"ops"`
	}](ctx, d.c, Request{Path: "/documents/" + url.PathEscape(id) + "/ops", Query: url.Values{"since": {strconv.FormatInt(desde, 10)}}})
	return out.Ops, err
}

// DocMiembro es una conexión presente en un documento: el actor de su pestaña y su persona.
type DocMiembro struct {
	Conn string `json:"conn"`
	Sub  string `json:"sub"`
}

// Presencia renueva la de esta conexión (PUT …/presence, TTL de 30 s en el servicio) y
// da las presentes, ella incluida.
func (d *Docs) Presencia(ctx context.Context, id, conn string) ([]DocMiembro, error) {
	cuerpo, err := json.Marshal(map[string]string{"conn": conn})
	if err != nil {
		return nil, err
	}
	out, err := JSON[struct {
		Members []DocMiembro `json:"members"`
	}](ctx, d.c, Request{
		Method: http.MethodPut, Path: "/documents/" + url.PathEscape(id) + "/presence",
		Header: http.Header{"Content-Type": {"application/json"}}, Body: cuerpo,
	})
	return out.Members, err
}

// ErrDocConflicto: entre leer el registro y escribirlo, otro lo ha cambiado. Mandarlo
// habría borrado su trabajo sin que nadie lo viera, así que no se manda.
var ErrDocConflicto = errors.New("docs: otro ha guardado este documento entretanto")

// SaveChecked es SaveContent con la comprobación de delante: lee el registro, se planta
// si ya no es el que se leyó (ErrDocConflicto) y, si sigue igual, escribe con un reloj
// que gana al más alto que el documento haya visto. Devuelve el registro resultante,
// listo para ser el `base` del guardado siguiente.
//
// Vive aquí y no en una vista porque lo necesita todo lo que guarda contenido de Sytue
// Docs —Docus, Calcus, Diapos—, y una copia por producto es una copia que se desalinea.
//
// La escritura va con `base_seq` (la seq que se leyó): si otro escribió entre la lectura de
// aquí y el POST, el servicio contesta 409 y es también ErrDocConflicto, sin ventana.
func (d *Docs) SaveChecked(ctx context.Context, id, clave, valor string, base DocContent, dot DocDot, autoritativo bool) (DocContent, error) {
	// Hojas y presentaciones: solo lo que cambió, como parche (docs_parche.go). El servicio lo
	// aplica sobre lo último que tiene, así que lo que otra persona haya escrito en OTRAS
	// celdas se conserva y no hay conflicto. Si no se deja decir así, abajo va el registro entero.
	// ponytail: la base que se devuelve es lo que se mandó, no el registro fusionado: lo ajeno
	// no aparece hasta reabrir; el parche siguiente sigue sin pisarlo.
	if clave == "doc:sheet" || clave == "doc:slides" {
		if opID, err := nuevoOpID(); err == nil {
			p, err := mandarParche(ctx, d.c, id, clave, base.Value, valor, opID, base.Seq)
			switch {
			case err == nil && p.SinEfecto:
				return DocContent{Value: valor, Dot: base.Dot, Lamport: base.Lamport, MaxLamport: base.MaxLamport, Seq: p.Seq}, nil
			case err == nil:
				return DocContent{Value: valor, Dot: p.Dot, Lamport: p.Lamport, MaxLamport: p.Lamport, Seq: p.Seq}, nil
			case !sinParches(err):
				return base, err
			}
		}
	}
	ahora, err := d.Content(ctx, id, clave)
	if err != nil {
		return base, err
	}
	if ahora.Dot != base.Dot || ahora.Lamport != base.Lamport {
		return base, ErrDocConflicto
	}
	lamport := max(ahora.MaxLamport, base.MaxLamport) + 1
	seq, err := d.saveContentBase(ctx, id, clave, valor, dot, lamport, autoritativo, base.Seq)
	var http409 *Error
	if errors.As(err, &http409) && http409.Status == http.StatusConflict {
		return base, ErrDocConflicto
	}
	if err != nil {
		return base, err
	}
	return DocContent{Value: valor, Dot: dot, Lamport: lamport, MaxLamport: lamport, Seq: seq}, nil
}

// DocVersion es una entrada del historial (domain.Version).
type DocVersion struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	AuthorSub string    `json:"author_sub"`
	Seq       int64     `json:"seq"`
	Auto      bool      `json:"auto"`
	CreatedAt time.Time `json:"created_at"`
}

// Versions lista el historial, la más reciente primero.
func (d *Docs) Versions(ctx context.Context, id string) ([]DocVersion, error) {
	out, err := JSON[struct {
		Versions []DocVersion `json:"versions"`
	}](ctx, d.c, Request{Path: "/documents/" + url.PathEscape(id) + "/versions"})
	return out.Versions, err
}

// RestoreVersion devuelve el documento a esa versión. No destruye nada: el servicio añade
// las operaciones que llevan hasta ella, así que restaurar también se puede deshacer.
func (d *Docs) RestoreVersion(ctx context.Context, id, version string) error {
	return d.sinCuerpo(ctx, http.MethodPost, "/documents/"+url.PathEscape(id)+"/versions/"+url.PathEscape(version)+"/restore", struct{}{})
}

// CreateVersion guarda una versión con nombre del estado de ahora (`label` puede ir
// vacío: el servicio la guarda sin nombre, como la web cuando no se escribe ninguno).
func (d *Docs) CreateVersion(ctx context.Context, id, label string) (DocVersion, error) {
	peticion := map[string]string{}
	if label = strings.TrimSpace(label); label != "" {
		peticion["label"] = label
	}
	out, err := PostJSON[struct {
		Version DocVersion `json:"version"`
	}](ctx, d.c, "/documents/"+url.PathEscape(id)+"/versions", peticion)
	return out.Version, err
}

// ExportFormats son los formatos que el menú «Archivo» ofrece, en su orden
// (`EXPORT_FORMATS` de packages/office/src/editor/types.ts).
var ExportFormats = office.Formats()

// maxExport acota lo que se lee de una exportación: un documento no pesa esto.
const maxExport = 256 << 20

// Export pide el documento en ese formato y devuelve sus bytes, por el mismo camino de dos
// pasos que el BFF de la web: el servicio lo materializa (`POST …/exports`) y luego se
// descarga en línea (`?inline=true`), porque la URL prefirmada apunta al almacenamiento
// interno. El servicio serializa desde SU estado: quien exporte, que guarde antes.
func (d *Docs) Export(ctx context.Context, id, formato string) ([]byte, error) {
	return d.export(ctx, id, formato, false)
}

// ExportTemplate pide la misma exportación como plantilla de Office u OpenDocument.
// El protocolo es el de la web: format y template:true, sin otro endpoint.
func (d *Docs) ExportTemplate(ctx context.Context, id, formato string) ([]byte, error) {
	return d.export(ctx, id, formato, true)
}

// ExportPDFSimple pide el PDF de siempre (catorce fuentes estándar, sin estructura) en vez
// del accesible (PDF/UA-1), que es el que da Export: «PDF accesible» apagado en el menú.
func (d *Docs) ExportPDFSimple(ctx context.Context, id string) ([]byte, error) {
	no := false
	return d.exportCon(ctx, id, "pdf", false, &no)
}

func (d *Docs) export(ctx context.Context, id, formato string, plantilla bool) ([]byte, error) {
	return d.exportCon(ctx, id, formato, plantilla, nil)
}

func (d *Docs) exportCon(ctx context.Context, id, formato string, plantilla bool, accesible *bool) ([]byte, error) {
	blob, err := PostJSON[struct {
		Export struct {
			ID string `json:"id"`
		} `json:"export"`
	}](ctx, d.c, "/documents/"+url.PathEscape(id)+"/exports", struct {
		Format     string `json:"format"`
		Template   bool   `json:"template,omitempty"`
		Accessible *bool  `json:"accessible,omitempty"`
	}{Format: formato, Template: plantilla, Accessible: accesible})
	if err != nil {
		return nil, err
	}
	resp, err := d.c.Do(ctx, Request{
		Path:  "/documents/" + url.PathEscape(id) + "/exports/" + url.PathEscape(blob.Export.ID),
		Query: url.Values{"inline": {"true"}},
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, maxExport))
}

// ExportExt es la extensión del fichero de cada formato para cada tipo (`exportExt`).
func ExportExt(k DocKind, formato string) string {
	return exportExt(k, formato, false)
}

// ExportTemplateExt es la extensión de una plantilla para cada tipo de documento.
func ExportTemplateExt(k DocKind, formato string) string {
	return exportExt(k, formato, true)
}

func exportExt(k DocKind, formato string, plantilla bool) string {
	ext := map[string][3]string{ // documento, hoja, presentación
		"pdf": {"pdf", "pdf", "pdf"}, "html": {"html", "html", "html"}, "markdown": {"md", "md", "md"},
		"odf": {"odt", "ods", "odp"}, "ooxml": {"docx", "xlsx", "pptx"},
	}[formato]
	if plantilla {
		switch formato {
		case "ooxml":
			ext = [3]string{"dotx", "xltx", "potx"}
		case "odf":
			ext = [3]string{"ott", "ots", "otp"}
		}
	}
	switch {
	case ext[0] == "":
		return "bin"
	case k == DocSheet:
		return ext[1]
	case k == DocSlides:
		return ext[2]
	}
	return ext[0]
}

// ExportFilename es el nombre del fichero descargado (`exportFilename`): el título sin
// los caracteres que un sistema de ficheros no admite, a lo sumo 120, y «documento» si
// no hay título.
func ExportFilename(titulo string, k DocKind, formato string) string {
	return exportFilename(titulo, k, formato, false)
}

// ExportTemplateFilename conserva el saneado y límite del nombre de descarga,
// y elige la extensión de plantilla de Office u OpenDocument.
func ExportTemplateFilename(titulo string, k DocKind, formato string) string {
	return exportFilename(titulo, k, formato, true)
}

func exportFilename(titulo string, k DocKind, formato string, plantilla bool) string {
	base := strings.TrimSpace(titulo)
	if base == "" {
		base = "documento"
	}
	base = reNombreProhibido.ReplaceAllString(base, "_")
	if r := []rune(base); len(r) > 120 {
		base = string(r[:120])
	}
	return base + "." + exportExt(k, formato, plantilla)
}

var reNombreProhibido = regexp.MustCompile(`[\\/:*?"<>|]+`)

// DocPermission es quién tiene acceso (domain.Permission). PrincipalKind es «user»,
// «group», «tenant» (toda la organización) o «public».
type DocPermission struct {
	PrincipalKind string `json:"principal_kind"`
	PrincipalID   string `json:"principal_id"`
	Role          string `json:"role"`
}

// DocShare es un enlace público (domain.ShareLink). Token solo viene al CREARLO: en la
// lista llega vacío, porque el servicio lo guarda con hash.
type DocShare struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	Token     string     `json:"token,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Sharing es lo que enseña «Gestionar acceso»: los permisos y los enlaces, como el GET
// del BFF de la web (`/api/sharing`). Solo el propietario lo ve: a los demás el
// servicio les dice 403, y se devuelve tal cual —un diálogo vacío parecería «no hay
// nadie con acceso»—. Los enlaces son informativos: si fallan, lista vacía.
func (d *Docs) Sharing(ctx context.Context, id string) ([]DocPermission, []DocShare, error) {
	base := "/documents/" + url.PathEscape(id)
	perms, err := JSON[struct {
		Permissions []DocPermission `json:"permissions"`
	}](ctx, d.c, Request{Path: base + "/permissions"})
	if err != nil {
		return nil, nil, err
	}
	links, _ := JSON[struct {
		Shares []DocShare `json:"shares"`
	}](ctx, d.c, Request{Path: base + "/shares"})
	return perms.Permissions, links.Shares, nil
}

// Grant da (o cambia) el acceso de alguien: `kind` «user» con su dirección, o «tenant»
// con el id vacío para toda la organización.
func (d *Docs) Grant(ctx context.Context, id, kind, principal, role string) error {
	return d.sinCuerpo(ctx, http.MethodPut, "/documents/"+url.PathEscape(id)+"/permissions",
		map[string]string{"principal_kind": kind, "principal_id": principal, "role": role})
}

// Revoke quita el acceso de ese principal.
func (d *Docs) Revoke(ctx context.Context, id, kind, principal string) error {
	resp, err := d.c.Do(ctx, Request{
		Method: http.MethodDelete, Path: "/documents/" + url.PathEscape(id) + "/permissions",
		Query: url.Values{"principal_kind": {kind}, "principal_id": {principal}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// CreateShare crea un enlace público con ese papel y lo devuelve CON su token, que es la
// única vez que se ve.
func (d *Docs) CreateShare(ctx context.Context, id, role string) (DocShare, error) {
	out, err := PostJSON[struct {
		Share DocShare `json:"share"`
	}](ctx, d.c, "/documents/"+url.PathEscape(id)+"/shares", map[string]string{"role": role})
	return out.Share, err
}

// RevokeShare anula un enlace público.
func (d *Docs) RevokeShare(ctx context.Context, id, enlace string) error {
	return d.sinCuerpo(ctx, http.MethodDelete, "/documents/"+url.PathEscape(id)+"/shares/"+url.PathEscape(enlace), nil)
}

// DocComment es un comentario (domain.Comment). ParentID no vacío: es una respuesta.
type DocComment struct {
	ID        string `json:"id"`
	ParentID  string `json:"parent_id"`
	AuthorSub string `json:"author_sub"`
	// AuthorName es el nombre del autor en el directorio; lo resuelve el servicio.
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	Resolved   bool      `json:"resolved"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Anchor es a qué trozo del documento se refiere: el texto citado a secas, o un
	// objeto con `text` y `mark` (el id del `<mark data-comment-id>` del folio). Lo
	// escribe la web; aquí se lee con AnchorText.
	Anchor json.RawMessage `json:"anchor,omitempty"`
}

// Autor es quién lo escribió, como lo enseña `autorDe` de la web: su nombre, o el
// principio del `sub` si el directorio no lo resolvió (alguien de otra organización).
func (c DocComment) Autor() string {
	if c.AuthorName != "" {
		return c.AuthorName
	}
	if r := []rune(c.AuthorSub); len(r) > 8 {
		return string(r[:8])
	}
	return c.AuthorSub
}

// AnchorText es el texto citado del ancla (`anchorText` de CommentsPanel.tsx), o "".
func (c DocComment) AnchorText() string {
	var s string
	if json.Unmarshal(c.Anchor, &s) == nil {
		return strings.TrimSpace(s)
	}
	var o struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(c.Anchor, &o) == nil {
		return strings.TrimSpace(o.Text)
	}
	return ""
}

// CanComment dice si ese papel deja comentar. Es el `RoleCommenter` del servicio: quien
// puede escribir, también comenta.
func CanComment(role string) bool { return role == "commenter" || CanWrite(role) }

// Comments lista los comentarios. Con conResueltos también los cerrados (`?resolved=true`).
func (d *Docs) Comments(ctx context.Context, id string, conResueltos bool) ([]DocComment, error) {
	q := url.Values{}
	if conResueltos {
		q.Set("resolved", "true")
	}
	out, err := JSON[struct {
		Comments []DocComment `json:"comments"`
	}](ctx, d.c, Request{Path: "/documents/" + url.PathEscape(id) + "/comments", Query: q})
	return out.Comments, err
}

// CreateComment abre un hilo, o responde a uno si `padre` no está vacío. El servicio
// rechaza un cuerpo en blanco, así que aquí tampoco se manda.
func (d *Docs) CreateComment(ctx context.Context, id, padre, cuerpo string) (DocComment, error) {
	cuerpo = strings.TrimSpace(cuerpo)
	if cuerpo == "" {
		return DocComment{}, ErrComentarioVacio
	}
	peticion := map[string]any{"body": cuerpo}
	if padre != "" {
		peticion["parent_id"] = padre
	}
	out, err := PostJSON[struct {
		Comment DocComment `json:"comment"`
	}](ctx, d.c, "/documents/"+url.PathEscape(id)+"/comments", peticion)
	return out.Comment, err
}

// CreateAnchoredComment abre un hilo anclado a un trozo del documento: el `anchor`
// `{ mark, text }` de la web, con el id del `<mark data-comment-id>` que lo envuelve en el
// folio y el texto citado.
func (d *Docs) CreateAnchoredComment(ctx context.Context, id, cuerpo, marca, texto string) (DocComment, error) {
	cuerpo = strings.TrimSpace(cuerpo)
	if cuerpo == "" {
		return DocComment{}, ErrComentarioVacio
	}
	peticion := map[string]any{"body": cuerpo, "anchor": map[string]string{"mark": marca, "text": texto}}
	out, err := PostJSON[struct {
		Comment DocComment `json:"comment"`
	}](ctx, d.c, "/documents/"+url.PathEscape(id)+"/comments", peticion)
	return out.Comment, err
}

// AnchorMark es el id del `<mark data-comment-id>` del ancla (`anchorMark` de
// CommentsPanel.tsx), o "".
func (c DocComment) AnchorMark() string {
	var o struct {
		Mark string `json:"mark"`
	}
	if json.Unmarshal(c.Anchor, &o) == nil {
		return o.Mark
	}
	return ""
}

// ErrComentarioVacio: un comentario en blanco no se manda (el servicio lo rechazaría).
var ErrComentarioVacio = errors.New("docs: el comentario está vacío")

// ResolveComment marca el hilo como resuelto, o lo reabre.
func (d *Docs) ResolveComment(ctx context.Context, id, comentario string, resuelto bool) error {
	return d.sinCuerpo(ctx, http.MethodPut,
		"/documents/"+url.PathEscape(id)+"/comments/"+url.PathEscape(comentario)+"/resolve",
		map[string]bool{"resolved": resuelto})
}

// DeleteComment borra un comentario o una respuesta.
func (d *Docs) DeleteComment(ctx context.Context, id, comentario string) error {
	return d.sinCuerpo(ctx, http.MethodDelete,
		"/documents/"+url.PathEscape(id)+"/comments/"+url.PathEscape(comentario), nil)
}

// nuevoOpID es el identificador de un parche: el servicio no lo aplica dos veces.
func nuevoOpID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
