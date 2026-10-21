// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// PDF es el cliente de Sytue certiDocs, y habla con DOS servicios porque certiDocs no
// tiene uno propio:
//
//   - el FICHERO vive en Sytue Drive, en la carpeta «PDF» de la raíz —la misma que usa la
//     web (pages/api/pdf-abrir.ts), así que los dos frentes ven los mismos documentos—;
//   - el MOTOR vive en Sytue Docs (`/api/v1/pdf/*`) y es una conversión pura: entra el
//     fichero por multipart, sale el resultado, y allí no se guarda nada.
//
// En la web ese puente lo hace el BFF de Astro (pages/api/pdf*.ts), que descarga de Drive
// y reenvía a Docs. Aquí no hay BFF: lo hace este cliente, con el token de la persona.
type PDF struct {
	drive *Drive
	motor *Client
}

// NewPDF crea el cliente sobre las raíces de Docs y de Drive.
func NewPDF(docs, drive string, s Session, h *http.Client) *PDF {
	return &PDF{drive: NewDrive(drive, s, h), motor: New(docs+"/api/v1", s, h)}
}

const (
	// PDFCarpeta es la carpeta de la raíz de Drive donde certiDocs guarda lo que abre.
	PDFCarpeta = "PDF"
	// PDFMaxBytes es el tope del motor (`domain.MaxPDFBytes`), que vale por fichero Y
	// para el cuerpo entero de una petición. Se comprueba ANTES de subir nada: mandar
	// más de cien megas para recibir un 413 es tráfico tirado.
	PDFMaxBytes = 100 << 20
)

var (
	// ErrNoEsPDF: el fichero no empieza por «%PDF-». El nombre lo pone quien sube y no
	// dice nada; guardarlo igual llenaría la carpeta de cosas que no se pueden abrir.
	ErrNoEsPDF = errors.New("api: el fichero no es un PDF")
	// ErrPDFGrande: pasa de PDFMaxBytes.
	ErrPDFGrande = errors.New("api: el PDF pasa del tope del motor")
)

// PDFPideClave dice si el motor ha rechazado el documento porque pide una contraseña que
// no se le ha dado (o no es esa). Es lo que abre el campo de contraseña en vez de un error.
func PDFPideClave(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code() == "pdf_password_required"
}

// PDFMensaje es la frase —en inglés: está escrita en el servicio— con la que el motor
// explica un rechazo («Page 9 does not exist: the document has 3.»), o "".
func PDFMensaje(err error) string {
	var e *Error
	if !errors.As(err, &e) {
		return ""
	}
	var cuerpo struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal([]byte(e.Body), &cuerpo)
	return cuerpo.Message
}

// --- Drive: dónde viven -----------------------------------------------------------

// carpetaPDF busca la carpeta «PDF» en la raíz de Drive; id "" si todavía no existe.
func (p *PDF) carpetaPDF(ctx context.Context) (raiz, id string, err error) {
	r, err := p.drive.Root(ctx)
	if err != nil {
		return "", "", err
	}
	// Las carpetas llegan enteras en la primera página; lo paginado son los ficheros.
	hijos, err := p.drive.Children(ctx, r.ID, "")
	if err != nil {
		return "", "", err
	}
	if i := slices.IndexFunc(hijos.Folders, func(c Folder) bool { return c.Name == PDFCarpeta }); i >= 0 {
		id = hijos.Folders[i].ID
	}
	return r.ID, id, nil
}

// Lista devuelve los PDF de la carpeta «PDF», el más reciente primero. Sin carpeta todavía
// no hay PDF: es el estado de quien no ha abierto ninguno, no un fallo.
func (p *PDF) Lista(ctx context.Context) ([]File, error) {
	_, carpeta, err := p.carpetaPDF(ctx)
	if err != nil || carpeta == "" {
		return nil, err
	}
	todos, err := p.ficheros(ctx, carpeta)
	if err != nil {
		return nil, err
	}
	pdfs := todos[:0:0]
	for _, f := range todos {
		if esPDF(f.Name) {
			pdfs = append(pdfs, f)
		}
	}
	slices.SortStableFunc(pdfs, func(a, b File) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return pdfs, nil
}

// ficheros recorre las páginas de una carpeta.
// ponytail: todas antes de contestar. La web corta en los 48 más recientes; con miles de
// PDF en la carpeta, pedir `/folders/{id}/files` ordenado y paginar al desplazar.
func (p *PDF) ficheros(ctx context.Context, carpeta string) ([]File, error) {
	var todos []File
	for cursor := ""; ; {
		pagina, err := p.drive.Children(ctx, carpeta, cursor)
		if err != nil {
			return nil, err
		}
		todos = append(todos, pagina.Files...)
		if cursor = pagina.NextCursor; cursor == "" {
			return todos, nil
		}
	}
}

func esPDF(nombre string) bool { return strings.HasSuffix(strings.ToLower(nombre), ".pdf") }

// Subir guarda un PDF nuevo en la carpeta «PDF» —la crea si falta— con un nombre que no
// pise a ninguno: en Drive, subir con un nombre que ya existe SUSTITUYE el fichero.
func (p *PDF) Subir(ctx context.Context, nombre string, cuerpo io.Reader, tamano int64) (File, error) {
	if tamano > PDFMaxBytes {
		return File{}, ErrPDFGrande
	}
	cabecera := make([]byte, 5)
	if n, _ := io.ReadFull(cuerpo, cabecera); string(cabecera[:n]) != "%PDF-" {
		return File{}, ErrNoEsPDF
	}
	raiz, carpeta, err := p.carpetaPDF(ctx)
	if err != nil {
		return File{}, err
	}
	if carpeta == "" { // la primera vez: crearla es parte de abrir
		creada, err := p.drive.CreateFolder(ctx, raiz, PDFCarpeta)
		if err != nil {
			return File{}, err
		}
		carpeta = creada.ID
	}
	return p.guardar(ctx, carpeta, NombrePDF(nombre), "", io.MultiReader(bytes.NewReader(cabecera), cuerpo), tamano)
}

// GuardarCopia deja el resultado de una operación JUNTO al original, con una palabra que
// dice qué se hizo: «contrato (girado).pdf». El original no se toca nunca.
func (p *PDF) GuardarCopia(ctx context.Context, original File, sufijo string, datos []byte) (File, error) {
	return p.guardar(ctx, original.FolderID, original.Name, sufijo, bytes.NewReader(datos), int64(len(datos)))
}

// guardar sube con el primer nombre libre de la carpeta: «base.pdf», «base (2).pdf»… o,
// con sufijo, «base (sufijo).pdf», «base (sufijo 2).pdf»…, como cualquier gestor de archivos.
func (p *PDF) guardar(ctx context.Context, carpeta, nombre, sufijo string, cuerpo io.Reader, tamano int64) (File, error) {
	hay, err := p.ficheros(ctx, carpeta)
	if err != nil {
		return File{}, err
	}
	ocupado := func(n string) bool {
		return slices.ContainsFunc(hay, func(f File) bool { return strings.EqualFold(f.Name, n) })
	}
	return p.drive.Upload(ctx, carpeta, NombreLibre(nombre, sufijo, ocupado, time.Now()), "application/pdf", cuerpo, tamano)
}

// NombreLibre es el primer nombre que `ocupado` no reconoce. Tras diez intentos cae al
// sello de tiempo, que no choca con nada.
func NombreLibre(nombre, sufijo string, ocupado func(string) bool, ahora time.Time) string {
	base := nombre
	if esPDF(nombre) {
		base = nombre[:len(nombre)-len(".pdf")]
	}
	for n := 1; n <= 10; n++ {
		cola := ""
		switch {
		case sufijo != "" && n == 1:
			cola = " (" + sufijo + ")"
		case sufijo != "":
			cola = " (" + sufijo + " " + strconv.Itoa(n) + ")"
		case n > 1:
			cola = " (" + strconv.Itoa(n) + ")"
		}
		if candidato := base + cola + ".pdf"; !ocupado(candidato) {
			return candidato
		}
	}
	return base + " (" + strings.TrimSpace(sufijo+" "+ahora.UTC().Format("2006-01-02-15-04-05")) + ").pdf"
}

// NombrePDF deja un nombre que Drive acepta y que la lista reconoce: la última parte de la
// ruta, sin caracteres de control ni los que Windows prohíbe, y acabado en «.pdf».
func NombrePDF(crudo string) string {
	n := strings.TrimSpace(crudo)
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	n = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`"*:<>?|`, r) {
			return '_'
		}
		return r
	}, n))
	if !esPDF(n) {
		n += ".pdf"
	}
	if r := []rune(n); len(r) > 120 {
		n = string(r[len(r)-120:])
	}
	if n == ".pdf" {
		return "documento.pdf"
	}
	return n
}

// Renombrar cambia el nombre de un PDF. La extensión se conserva: sin ella el fichero
// desaparecería de la lista, que es por donde se le reconoce.
func (p *PDF) Renombrar(ctx context.Context, id, nombre string) (File, error) {
	cuerpo, err := json.Marshal(map[string]string{"name": NombrePDF(nombre)})
	if err != nil {
		return File{}, err
	}
	return JSON[File](ctx, p.drive.c, Request{
		Method: http.MethodPatch, Path: "/files/" + url.PathEscape(id), Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Eliminar manda el PDF a la papelera de Drive: se recupera desde allí.
func (p *PDF) Eliminar(ctx context.Context, id string) error {
	resp, err := p.drive.c.Do(ctx, Request{Method: http.MethodDelete, Path: "/files/" + url.PathEscape(id)})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Descargar trae el PDF a memoria, que es donde el motor lo necesita: cada operación lo
// vuelve a mandar entero. Uno mayor que el tope del motor no se llega a bajar entero.
func (p *PDF) Descargar(ctx context.Context, id string) ([]byte, error) {
	cuerpo, err := p.drive.Download(ctx, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cuerpo.Close() }()
	datos, err := io.ReadAll(io.LimitReader(cuerpo, PDFMaxBytes+1))
	if err == nil && len(datos) > PDFMaxBytes {
		err = ErrPDFGrande
	}
	return datos, err
}

// --- Docs: el motor ---------------------------------------------------------------

// PDFFichero es un documento tal y como se le manda al motor.
type PDFFichero struct {
	Nombre string
	Datos  []byte
}

// PDFPagina es una página tal y como la describe el motor. Ancho y alto van en puntos
// (1/72 de pulgada) y SIN aplicar la rotación.
type PDFPagina struct {
	Numero   int     `json:"numero"`
	Ancho    float64 `json:"ancho"`
	Alto     float64 `json:"alto"`
	Rotacion int     `json:"rotacion"`
	Extracto string  `json:"extracto"`
}

// PDFMarcador es una entrada del índice del documento (/Outlines).
type PDFMarcador struct {
	Titulo string `json:"titulo"`
	Pagina int    `json:"pagina"` // 0-based; -1 si el destino no se pudo resolver
	Nivel  int    `json:"nivel"`
}

// PDFAdjunto es un fichero incrustado: una factura electrónica lleva su XML así.
type PDFAdjunto struct {
	Nombre      string `json:"nombre"`
	TamanoBytes int    `json:"tamano_bytes"`
}

// PDFAnotacion es un comentario o una marca que ya está en el documento.
type PDFAnotacion struct {
	Pagina int    `json:"pagina"`
	Tipo   string `json:"tipo"`
	Texto  string `json:"texto,omitempty"`
	Autor  string `json:"autor,omitempty"`
}

// PDFCampo es un campo de formulario.
type PDFCampo struct {
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"`
	Valor  string `json:"valor,omitempty"`
}

// PDFFirma es lo que una firma DECLARA. Que el certificado sea de fiar y siga vigente no
// lo comprueba el motor, y la vista lo dice.
type PDFFirma struct {
	Nombre    string `json:"nombre,omitempty"`
	Motivo    string `json:"motivo,omitempty"`
	Lugar     string `json:"lugar,omitempty"`
	Fecha     string `json:"fecha,omitempty"`
	CubreTodo bool   `json:"cubre_todo"`
}

// PDFHuella son los metadatos que el fichero lleva encima. El servicio los serializa sin
// etiquetas JSON, de ahí las mayúsculas.
type PDFHuella struct {
	Autor       string
	Creador     string
	Productor   string
	Titulo      string
	ConAdjuntos bool
	ConJS       bool
}

// PDFPermisos es lo que el documento PIDE que no se haga: una petición escrita en el
// fichero, no un candado. También sin etiquetas JSON en el servicio.
type PDFPermisos struct {
	Modificar bool
	Ensamblar bool
}

// PDFContenido es todo lo que se sabe del documento, de una sola llamada.
type PDFContenido struct {
	// Paginas se corta en las 500 primeras (`maxPaginasIndexadas`); Total no.
	Paginas     []PDFPagina    `json:"paginas"`
	Total       int            `json:"total"`
	Indice      []PDFMarcador  `json:"indice"`
	Adjuntos    []PDFAdjunto   `json:"adjuntos"`
	Anotaciones []PDFAnotacion `json:"anotaciones"`
	Campos      []PDFCampo     `json:"campos"`
	Firmas      []PDFFirma     `json:"firmas"`
	Cifrado     bool           `json:"cifrado"`
	Permisos    PDFPermisos    `json:"permisos"`
	Huella      PDFHuella      `json:"huella"`
}

// PDFFragmento es un trozo de texto con su sitio, en coordenadas del PDF (origen abajo a
// la izquierda).
type PDFFragmento struct {
	Texto  string  `json:"texto"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Tamano float64 `json:"tamano"`
}

// Contenido lee el documento: páginas, índice, adjuntos, comentarios, campos, firmas,
// permisos y metadatos. `clave` es la contraseña de apertura, o "".
func (p *PDF) Contenido(ctx context.Context, doc PDFFichero, clave string) (PDFContenido, error) {
	r, err := formularioPDF("/pdf/content", map[string]string{"clave": clave}, doc)
	if err != nil {
		return PDFContenido{}, err
	}
	return JSON[PDFContenido](ctx, p.motor, r)
}

// Texto devuelve el texto de cada página, que es lo que alimenta la lectura y la búsqueda.
// Una página sin texto —un escaneo— viene vacía.
func (p *PDF) Texto(ctx context.Context, doc PDFFichero, clave string) ([][]PDFFragmento, error) {
	r, err := formularioPDF("/pdf/text", map[string]string{"clave": clave}, doc)
	if err != nil {
		return nil, err
	}
	out, err := JSON[struct {
		Paginas [][]PDFFragmento `json:"paginas"`
	}](ctx, p.motor, r)
	return out.Paginas, err
}

// Render devuelve UNA página dibujada en SVG, con las fuentes del documento dentro: está
// pensada para que la pinte un navegador. `escala` 1 son 72 ppp; el `viewBox` va en puntos.
func (p *PDF) Render(ctx context.Context, doc PDFFichero, pagina int, escala float64, clave string) (string, error) {
	r, err := formularioPDF("/pdf/render", map[string]string{
		"pagina": strconv.Itoa(pagina), "escala": strconv.FormatFloat(escala, 'f', -1, 64), "clave": clave,
	}, doc)
	if err != nil {
		return "", err
	}
	resp, err := p.motor.Do(ctx, r)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	// El motor corta el SVG en 24 MiB (`maxSVGBytes`): una página maliciosa no llena la memoria.
	svg, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return string(svg), err
}

// Operaciones de página del motor. Reordenar es PDFExtraer con las páginas en otro orden.
const (
	PDFExtraer  = "extraer"
	PDFBorrar   = "borrar"
	PDFRotar    = "rotar"
	PDFCombinar = "combinar"
)

// Paginas aplica una operación de página y devuelve el PDF resultante; aquí no se guarda
// nada. `paginas` es la lista 1-based que ve la persona («1,3,5-8»), en el ORDEN dado;
// `grados` solo cuenta al rotar; PDFCombinar une `docs` tal y como llegan.
func (p *PDF) Paginas(ctx context.Context, operacion, paginas string, grados int, clave string, docs ...PDFFichero) ([]byte, error) {
	campos := map[string]string{"op": operacion, "paginas": paginas, "clave": clave}
	if operacion == PDFRotar {
		campos["grados"] = strconv.Itoa(grados)
	}
	r, err := formularioPDF("/pdf/pages", campos, docs...)
	if err != nil {
		return nil, err
	}
	resp, err := p.motor.Do(ctx, r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, 4*PDFMaxBytes))
}

// formularioPDF compone el multipart que espera el motor: los documentos en el campo
// «file», en orden, y los campos que no vengan vacíos. Va en memoria —como el propio
// documento— porque Request.Body tiene que poder repetirse tras un 401.
func formularioPDF(ruta string, campos map[string]string, docs ...PDFFichero) (Request, error) {
	return formularioPDFCon(ruta, campos, nil, docs...)
}

// formularioPDFCon es formularioPDF con ficheros sueltos además de los documentos: la
// imagen de una firma, el XFDF que se importa. Cuentan para el tope como los demás,
// porque el tope del motor es del CUERPO entero.
func formularioPDFCon(ruta string, campos map[string]string, partes []PDFParte, docs ...PDFFichero) (Request, error) {
	todas := make([]PDFParte, 0, len(docs)+len(partes))
	for _, d := range docs {
		todas = append(todas, PDFParte{Campo: "file", Nombre: d.Nombre, Datos: d.Datos})
	}
	todas = append(todas, partes...)
	total := 0
	for _, d := range todas {
		total += len(d.Datos)
	}
	if total > PDFMaxBytes {
		return Request{}, ErrPDFGrande
	}
	var cuerpo bytes.Buffer
	cuerpo.Grow(total + 1024)
	w := multipart.NewWriter(&cuerpo)
	for _, d := range todas {
		parte, err := w.CreateFormFile(d.Campo, d.Nombre)
		if err != nil {
			return Request{}, err
		}
		if _, err := parte.Write(d.Datos); err != nil {
			return Request{}, err
		}
	}
	for k, v := range campos {
		if v == "" {
			continue
		}
		if err := w.WriteField(k, v); err != nil {
			return Request{}, err
		}
	}
	if err := w.Close(); err != nil {
		return Request{}, fmt.Errorf("api: cerrar el formulario: %w", err)
	}
	return Request{
		Method: http.MethodPost, Path: ruta, Body: cuerpo.Bytes(),
		Header: http.Header{"Content-Type": {w.FormDataContentType()}},
	}, nil
}

// PDFEnBlanco es un PDF 1.4 de una página A4 vacía, escrito a mano: es `pdfEnBlanco` de
// `pdf/blanco.ts`, con el que la web crea «PDF en blanco» y lo sube por el mismo camino
// que un PDF del ordenador. Cuatro objetos y una tabla xref; lo único que no se puede
// escribir a ojo son los desplazamientos de la xref, y se calculan al montarlo.
//
// Sin `/Contents`: una página sin flujo de contenido es válida (PDF 32000-1 §7.7.3.3) y
// se dibuja en blanco.
func PDFEnBlanco() []byte {
	objetos := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		// A4 en puntos, la misma página que usa el editor (595.28 × 841.89).
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595.28 841.89] /Resources << >> >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	desplazamientos := make([]int, 0, len(objetos))
	for i, o := range objetos {
		desplazamientos = append(desplazamientos, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objetos)+1)
	for _, d := range desplazamientos {
		fmt.Fprintf(&b, "%010d 00000 n \n", d)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objetos)+1, xref)
	return []byte(b.String())
}
