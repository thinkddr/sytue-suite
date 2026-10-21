// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/ocr"
)

// El resto de Sytue PDF sobre HTTP: ver, anotar, rellenar, proteger, firmar y
// comparar. Comparte forma con `/pdf/pages` y `/pdf/inspect` — conversión PURA:
// entra un PDF por multipart, sale el resultado, y NADA se guarda ni se registra.
//
// Que sean puras no es un detalle de implementación, es la promesa: partir un
// contrato o firmar una nómina hoy obliga a subirlos a una web cualquiera, y ahí
// se quedan. Aquí el fichero se procesa en memoria y se devuelve.
//
// La verja es la sesión (RequireAuth + RequireTenant, que aplica el router). No
// hay documento que autorizar porque el fichero lo aporta quien llama.

// maxTamanoCampoJSON acota los campos de texto que llevan JSON (anotaciones,
// valores de formulario). Sin tope, un cuerpo de pocos megas puede describir un
// millón de objetos y el trabajo de construirlos no lo acota `MaxPDFBytes`.
const maxTamanoCampoJSON = 8 << 20

// prepararFormularioPDF hace el trabajo común: comprueba sesión, acota el cuerpo
// y parsea el multipart.
func (h *Handler) prepararFormularioPDF(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return false
	}
	const framingMargin = 1 << 20
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxPDFBytes+framingMargin)
	//nolint:gosec // G120: el cuerpo ya está acotado por http.MaxBytesReader
	if err := r.ParseMultipartForm(domain.MaxPDFBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(w, r, errPayloadTooLarge)
			return false
		}
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			`Send the PDF as multipart/form-data in the "file" field.`,
		))
		return false
	}
	return true
}

func limpiarFormulario(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}

// unDocumento exige exactamente un fichero, que es lo que quieren casi todas
// estas operaciones.
func unDocumento(r *http.Request) (*domain.PDFDoc, error) {
	docs, err := leerPDFsDelFormulario(r)
	if err != nil {
		return nil, err
	}
	if len(docs) != 1 {
		return nil, httpx.ErrBadRequest.WithMessage("Send a single PDF.")
	}
	return docs[0], nil
}

// responderPDF devuelve el fichero resultante. Con la entrada, avisa también de
// qué pasó con su marca PDF/A (avisarPDFA).
func responderPDF(w http.ResponseWriter, entrada *domain.PDFDoc, salida []byte, nombre string) {
	avisarPDFA(w, entrada, salida)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.Itoa(len(salida)))
	// Ni cachés intermedias ni el navegador deben guardar el resultado: puede ser
	// un contrato, y la petición ya lleva la sesión del usuario.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(salida)
}

// avisarPDFA dice a la interfaz si el documento era PDF/A y la salida ya no lo
// es («perdido»: la edición rompía la conformidad y se quitó la marca) o bajó
// de nivel «a» a «b» («rebajado»: censurar quita la estructura etiquetada).
// Decisión del dueño (2026-10-15): solo la marca, si cumple, y avisando.
func avisarPDFA(w http.ResponseWriter, entrada *domain.PDFDoc, salida []byte) {
	if entrada == nil {
		return
	}
	antes := entrada.DeclaraPDFA()
	if antes == "" {
		return
	}
	despues := ""
	if d, err := domain.LeerPDF(salida); err == nil {
		despues = d.DeclaraPDFA()
	}
	switch {
	case despues == "":
		w.Header().Set("X-Sytue-PDFA", "perdido")
	case strings.HasSuffix(antes, "a") && strings.HasSuffix(despues, "b"):
		w.Header().Set("X-Sytue-PDFA", "rebajado")
	}
}

// --- Ver ---------------------------------------------------------------------

// pdfRender atiende POST /api/v1/pdf/render: devuelve UNA página dibujada en SVG.
//
// Va página a página y no el documento entero a propósito: un expediente de
// trescientas páginas pesaría cientos de megas en una sola respuesta, y el
// visor solo necesita lo que se está mirando. La interfaz pide las de alrededor
// mientras el usuario lee.
func (h *Handler) pdfRender(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	escala := 1.0
	if s := strings.TrimSpace(r.FormValue("escala")); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v <= 0 || v > 16 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The scale must be between 0 and 16."))
			return
		}
		escala = v
	}
	opts := domain.OpcionesRender{Escala: escala, SinAnotaciones: r.FormValue("sin_anotaciones") == "1"}

	// `formato=png` entrega la MISMA página rasterizada. Es para el cliente que no
	// tiene detrás un motor de SVG con texto —el escritorio Fyne—, y no cambia
	// nada de lo de siempre: sin el campo, o con `svg`, sale el SVG con `<text>`,
	// que es lo que la web necesita para seleccionar, buscar y leer en voz alta.
	formato := strings.TrimSpace(r.FormValue("formato"))
	if formato != "" && formato != "svg" && formato != "png" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`The format must be "svg" or "png".`))
		return
	}
	if formato == "png" {
		h.pdfRenderPNG(w, r, doc, opts)
		return
	}

	// Con `paginas` se devuelve un LOTE en JSON. El visor pide la ventana que se
	// está mirando de una vez: sin esto, desplazarse por un documento de veinte
	// megas lo vuelve a subir y a parsear en cada página.
	if lista := strings.TrimSpace(r.FormValue("paginas")); lista != "" {
		indices, err := parsearPaginas(lista, doc.NumPaginas())
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if len(indices) > maxPaginasPorLote {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
				"Too many pages in a single batch (maximum %d).", maxPaginasPorLote,
			)))
			return
		}
		out := make(map[string]string, len(indices))
		for _, i := range indices {
			svg, err := doc.RenderizarPaginaSVG(i, opts)
			if err != nil {
				// Una página ilegible no tumba el lote: se devuelve sin ella y la
				// interfaz enseña el hueco en vez de una pantalla de error.
				continue
			}
			out[strconv.Itoa(i+1)] = svg
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"paginas": out, "total": doc.NumPaginas()})
		return
	}

	pagina, err := enteroOpcional(r.FormValue("pagina"), 1, doc.NumPaginas())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	svg, err := doc.RenderizarPaginaSVG(pagina-1, opts)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That page could not be drawn."))
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// El SVG lleva texto del documento: sin esto, un navegador podría
	// interpretarlo como HTML si algo lo sirviera con otro tipo.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, svg)
}

// maxPaginasPorLote acota cuántas páginas se rasterizan en un lote. Es MENOS
// que en SVG: una A4 a ×2 son 8 MiB de mapa de bits mientras se dibuja, y veinte
// a la vez en varias peticiones simultáneas se comen el servicio.
const maxPaginasPNGPorLote = 8

// pdfRenderPNG entrega la página rasterizada. Una imagen no se selecciona ni la
// lee un lector de pantalla —por eso NO es la salida por defecto— pero se ve en
// cualquier cliente, que es lo que el escritorio necesita.
func (h *Handler) pdfRenderPNG(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc, opts domain.OpcionesRender) {
	if lista := strings.TrimSpace(r.FormValue("paginas")); lista != "" {
		indices, err := parsearPaginas(lista, doc.NumPaginas())
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if len(indices) > maxPaginasPNGPorLote {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
				"Too many pages in a single batch (maximum %d).", maxPaginasPNGPorLote,
			)))
			return
		}
		out := make(map[string]string, len(indices))
		for _, i := range indices {
			datos, err := doc.RenderizarPaginaPNG(i, opts)
			if err != nil {
				// Una página que no cabe o no se deja dibujar no tumba el lote: la
				// interfaz enseña el hueco, como con el SVG.
				continue
			}
			out[strconv.Itoa(i+1)] = base64.StdEncoding.EncodeToString(datos)
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{
			"paginas": out, "total": doc.NumPaginas(), "formato": "png",
		})
		return
	}

	pagina, err := enteroOpcional(r.FormValue("pagina"), 1, doc.NumPaginas())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	datos, err := doc.RenderizarPaginaPNG(pagina-1, opts)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That page could not be drawn."))
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(datos)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(datos)
}

// maxPaginasPorLote acota cuántas páginas se dibujan en una sola petición. Una
// página compleja son cientos de kilobytes de SVG: veinte de golpe ya son varios
// megas, y más que eso no cabe en ninguna ventana de visor.
const maxPaginasPorLote = 20

// pdfContenido atiende POST /api/v1/pdf/content: todo lo que la interfaz
// necesita saber del documento en UNA llamada — páginas, texto, anotaciones,
// campos, firmas, permisos y metadatos.
//
// Es una sola petición y no seis porque el fichero hay que subirlo de todas
// formas, y subir un documento de veinte megas seis veces para pintar la barra
// lateral sería absurdo.
func (h *Handler) pdfContenido(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	permisos := doc.PermisosDelDocumento()
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"paginas":     indiceDe(doc, doc.NumPaginas(), presupuestoExtractos),
		"total":       doc.NumPaginas(),
		"indice":      doc.IndiceDelDocumento(),
		"enlaces":     doc.EnlacesDelDocumento(),
		"adjuntos":    doc.AdjuntosDelDocumento(),
		"anotaciones": doc.AnotacionesDelDocumento(),
		"campos":      doc.CamposDelFormulario(),
		"firmas":      doc.FirmasDelDocumento(),
		"cifrado":     doc.EstaCifrado(),
		"permisos":    permisos,
		"huella":      doc.Huella(),
	})
}

// pdfAdjunto atiende POST /api/v1/pdf/attachment: saca un fichero incrustado.
//
// Va en su propio endpoint y no en `/content` porque el cuerpo ES el fichero:
// un XML de factura o una hoja de cálculo no caben en el JSON de la barra
// lateral, y meterlos en base64 multiplicaría por cuatro lo que ya se subió.
func (h *Handler) pdfAdjunto(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	nombre := strings.TrimSpace(r.FormValue("nombre"))
	if nombre == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Say which attachment to extract."))
		return
	}
	datos, err := doc.ExtraerAdjunto(nombre)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound.WithMessage("This document has no such attachment."))
		return
	}
	responderAdjunto(w, datos, nombre)
}

// responderAdjunto sirve el fichero incrustado.
//
// SIEMPRE como `application/octet-stream` y SIEMPRE como descarga, aunque el
// documento declare otro tipo: el adjunto lo eligió quien hizo el PDF, y honrar
// un `text/html` suyo sería servir HTML ajeno desde el origen de la sesión, que
// es un XSS con todas las letras. El tipo declarado se enseña en la lista, que
// es donde informa sin decidir nada.
func responderAdjunto(w http.ResponseWriter, datos []byte, nombre string) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(datos)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// El nombre ya viene saneado del dominio, pero se cita igual: es la cabecera
	// donde una comilla suelta cambia lo que el navegador entiende.
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+strings.ReplaceAll(nombre, `"`, "")+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(datos)
}

// pdfTexto atiende POST /api/v1/pdf/text: el texto de cada página, que es lo que
// alimenta la búsqueda del visor.
func (h *Handler) pdfTexto(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	type fragmento struct {
		Texto  string  `json:"texto"`
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Tamano float64 `json:"tamano"`
	}
	paginas := make([][]fragmento, 0, doc.NumPaginas())
	for i := 0; i < doc.NumPaginas(); i++ {
		frags, err := doc.TextoDePagina(i)
		if err != nil {
			paginas = append(paginas, nil)
			continue
		}
		out := make([]fragmento, 0, len(frags))
		for _, f := range frags {
			if strings.TrimSpace(f.Texto) == "" {
				continue
			}
			out = append(out, fragmento{Texto: f.Texto, X: f.X, Y: f.Y, Tamano: f.Tamano})
		}
		paginas = append(paginas, out)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"paginas": paginas})
}

// --- Anotar ------------------------------------------------------------------

// quienFirma devuelve el nombre con el que se atribuye lo que el usuario
// escribe. El nombre visible si lo hay, y si no el correo: un identificador
// interno no le dice nada a quien lee el comentario tres meses después.
func quienFirma(r *http.Request) string {
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		return ""
	}
	if claims.Name != "" {
		return claims.Name
	}
	return claims.Email
}

// maxIndiceComentario acota el número de comentario que se acepta pedir. El
// motor comprueba que exista de verdad; esto solo evita que un número absurdo
// llegue a recorrer el documento entero.
const maxIndiceComentario = 100000

// pdfAnotar atiende POST /api/v1/pdf/annotate.
//
//	op          poner | borrar | aplanar
//	anotaciones JSON con la lista (op=poner)
//	indices     lista separada por comas (op=borrar)
func (h *Handler) pdfAnotar(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var salida []byte
	switch op := strings.TrimSpace(r.FormValue("op")); op {
	case "", "poner":
		var anots []domain.Anotacion
		if err := leerJSONDelFormulario(r, "anotaciones", &anots); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		salida, err = domain.AnadirAnotacionesPDF(doc, anots)
	case "borrar":
		indices, err2 := listaDeEnteros(r.FormValue("indices"))
		if err2 != nil {
			httpx.Fail(w, r, err2)
			return
		}
		salida, err = domain.BorrarAnotacionesPDF(doc, indices)
	case "aplanar":
		salida, err = domain.AplanarAnotacionesPDF(doc)
	case "responder":
		indice, err2 := enteroOpcional(r.FormValue("indice"), 1, maxIndiceComentario)
		if err2 != nil {
			httpx.Fail(w, r, err2)
			return
		}
		// El índice llega 1-based como todo lo que ve el usuario; el motor cuenta
		// desde 0. La conversión, en un solo sitio.
		//
		// El AUTOR sale de la sesión y no del formulario: si lo pusiera el cliente,
		// cualquiera podría firmar una réplica con el nombre de otro, y en un
		// documento de revisión eso es justo lo que no puede pasar.
		salida, err = domain.ResponderAAnotacionPDF(doc, indice-1, r.FormValue("texto"), quienFirma(r))
	case "marcar":
		indice, err2 := enteroOpcional(r.FormValue("indice"), 1, maxIndiceComentario)
		if err2 != nil {
			httpx.Fail(w, r, err2)
			return
		}
		salida, err = domain.MarcarComentarioPDF(doc, indice-1, r.FormValue("estado"), quienFirma(r))
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("%q is not a valid operation. Use poner, borrar, responder, marcar or aplanar.", op),
		))
		return
	}
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "anotado.pdf")
}

// --- Formularios -------------------------------------------------------------

// pdfFormulario atiende POST /api/v1/pdf/form.
//
//	op       leer | detectar | crear | rellenar | aplanar
//	         | exportar-datos | importar-datos   (XFDF, ISO 19444-1)
//	valores  JSON {"nombre del campo": "valor"} (op=rellenar)
//	campos   JSON [{tipo,nombre,pagina,rect,…}] (op=crear)
//
// pdfImportarDatos atiende el caso «importar-datos» de pdfFormulario: un XFDF
// que se vuelca sobre el impreso. Va aparte porque encadena tres operaciones
// —leer el fichero, interpretarlo y rellenar— y era el caso más largo del switch.
func (h *Handler) pdfImportarDatos(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	datos, err := leerFicheroDelFormulario(r, "datos")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	valores, err := domain.ImportarXFDF(datos)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	salida, err := domain.RellenarFormularioPDF(doc, valores)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	// Cuántos campos entraron: sin esto, importar un fichero de OTRO impreso
	// —que no casa con ningún campo— se anuncia igual que uno que sí encajó.
	w.Header().Set("X-Sytue-Campos-Importados", strconv.Itoa(len(valores)))
	responderPDF(w, doc, salida, "formulario.pdf")
}

func (h *Handler) pdfFormulario(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Una función por operación, igual que en `pdfEditar`: el despachador se lee
	// de un vistazo y tocar una no obliga a leer las otras siete.
	switch op := strings.TrimSpace(r.FormValue("op")); op {
	case "", "leer":
		httpx.JSON(w, r, http.StatusOK, map[string]any{"campos": doc.CamposDelFormulario()})
	case "exportar-datos":
		formExportarDatos(w, r, doc)
	case "importar-datos":
		h.pdfImportarDatos(w, r, doc)
	case "detectar":
		formDetectar(w, r, doc)
	case "crear":
		formCrear(w, r, doc)
	case "rellenar":
		formRellenar(w, r, doc)
	case "aplanar":
		formAplanar(w, r, doc)
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("%q is not a valid operation. Use leer, detectar, crear, rellenar, "+
				"aplanar, exportar-datos or importar-datos.", op),
		))
	}
}

func formExportarDatos(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	xfdf, err := doc.ExportarXFDF(nombreDelFicheroSubido(r))
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	w.Header().Set("Content-Type", "application/vnd.adobe.xfdf")
	w.Header().Set("Content-Length", strconv.Itoa(len(xfdf)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="datos.xfdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xfdf)
}

func formDetectar(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	pagina, err := enteroOpcional(r.FormValue("pagina"), 1, doc.NumPaginas())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	props, err := doc.DetectarCamposPDF(pagina - 1)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"propuestas": props})
}

func formCrear(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	var campos []domain.CampoNuevo
	if err := leerJSONDelFormulario(r, "campos", &campos); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	salida, err := domain.AnadirCamposPDF(doc, campos)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "formulario.pdf")
}

func formRellenar(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	var valores map[string]string
	if err := leerJSONDelFormulario(r, "valores", &valores); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	salida, err := domain.RellenarFormularioPDF(doc, valores)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "formulario.pdf")
}

func formAplanar(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	salida, err := domain.AplanarFormularioPDF(doc)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "formulario.pdf")
}

// --- Proteger ----------------------------------------------------------------

// pdfProteger atiende POST /api/v1/pdf/protect.
//
//	op          proteger | desproteger | sanear | redactar
//	usuario     contraseña de apertura (op=proteger)
//	dueno       contraseña de permisos (op=proteger)
//	permisos    lista separada por comas: imprimir,copiar,modificar,anotar,…
//	areas       JSON con las zonas a censurar (op=redactar)
func (h *Handler) pdfProteger(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var salida []byte
	switch op := strings.TrimSpace(r.FormValue("op")); op {
	case "proteger":
		usuario, dueno := r.FormValue("usuario"), r.FormValue("dueno")
		if usuario == "" && dueno == "" {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
				"Protecting needs at least one password.",
			))
			return
		}
		salida, err = domain.ProtegerPDF(doc, domain.OpcionesProteccion{
			ContrasenaUsuario: usuario,
			ContrasenaDueno:   dueno,
			Permitir:          permisosDeLista(r.FormValue("permisos")),
		})
	case "desproteger":
		salida, err = domain.DesprotegerPDF(doc)
	case "metadatos":
		var m domain.Metadatos
		if err := leerJSONDelFormulario(r, "metadatos", &m); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		salida, conXMP, err := domain.EditarMetadatosPDF(doc, m)
		if err != nil {
			httpx.Fail(w, r, errorDeDominio(err))
			return
		}
		// Si el documento lleva XMP, el cambio puede NO verse: un lector moderno
		// prefiere ese bloque, que aquí no se toca. Decirlo es la diferencia entre
		// una limitación conocida y un cambio que se pierde en silencio.
		if conXMP {
			w.Header().Set("X-Sytue-Metadatos-Xmp", "1")
		}
		responderPDF(w, doc, salida, "documento.pdf")
	case "sanear":
		salida, err = domain.SanearMetadatosPDF(doc)
	case "redactar":
		var areas []domain.AreaRedaccion
		if err := leerJSONDelFormulario(r, "areas", &areas); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		salida, err = domain.RedactarPDF(doc, areas)
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("%q is not a valid operation. Use proteger, desproteger, metadatos, "+
				"sanear or redactar.", op),
		))
		return
	}
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "documento.pdf")
}

// permisosDeLista traduce «imprimir,copiar» a los permisos. Lo que no se nombra
// se prohíbe: quien protege un documento espera que lo NO dicho quede fuera, no
// dentro.
func permisosDeLista(s string) domain.Permisos {
	var p domain.Permisos
	for _, x := range strings.Split(s, ",") {
		switch strings.TrimSpace(strings.ToLower(x)) {
		case "imprimir":
			p.Imprimir = true
		case "imprimir-alta":
			p.ImprimirEnAlta = true
		case "modificar":
			p.Modificar = true
		case "copiar":
			p.Copiar = true
		case "anotar":
			p.Anotar = true
		case "rellenar":
			p.RellenarCampos = true
		case "ensamblar":
			p.Ensamblar = true
		}
	}
	// La accesibilidad NO es negociable: prohibir la extracción de texto para un
	// lector de pantalla deja el documento fuera del alcance de quien lo necesita,
	// y la propia especificación marcó ese bit como obsoleto por eso mismo.
	p.Accesibilidad = true
	return p
}

// --- Firmar ------------------------------------------------------------------

// pdfFirmar atiende POST /api/v1/pdf/sign.
//
//	op          dibujada | certificado
//	firma       JSON con el trazo y el recuadro
//	imagen      PNG/JPEG de una rúbrica escaneada (op=dibujada, manda sobre el trazo)
//	certificado fichero PEM (op=certificado)
//	clave_priv  fichero PEM con la clave (op=certificado)
//	pkcs12      fichero .p12/.pfx con certificado y clave dentro (op=certificado)
//	clave_p12   contraseña del .p12/.pfx
func (h *Handler) pdfFirmar(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var salida []byte
	switch op := strings.TrimSpace(r.FormValue("op")); op {
	case "", "dibujada":
		var f domain.Firma
		if err := leerJSONDelFormulario(r, "firma", &f); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		// La firma ESCANEADA: una foto o un PNG de la rúbrica de siempre, que es
		// como firma quien no quiere dibujar con el dedo. El dominio ya sabía
		// estamparla —`Firma.ImagenPNG` manda sobre los trazos— pero no había por
		// dónde entregarla: el campo es `json:"-"`, así que viaja como fichero
		// del formulario y no dentro del JSON.
		if r.MultipartForm != nil && len(r.MultipartForm.File["imagen"]) > 0 {
			if f.ImagenPNG, err = leerFicheroDelFormulario(r, "imagen"); err != nil {
				httpx.Fail(w, r, err)
				return
			}
		}
		salida, err = domain.FirmarDibujadoPDF(doc, f)
	case "certificado":
		var opts domain.OpcionesFirma
		if err := leerJSONDelFormulario(r, "firma", &opts); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		// Un `.p12` trae las dos cosas dentro y es como lo entregan la FNMT y las
		// autoridades de certificación: si viene, no hacen falta los dos PEM.
		if r.MultipartForm != nil && len(r.MultipartForm.File["pkcs12"]) > 0 {
			if opts.PKCS12, err = leerFicheroDelFormulario(r, "pkcs12"); err != nil {
				httpx.Fail(w, r, err)
				return
			}
			opts.ClaveDelPKCS12 = r.FormValue("clave_p12")
		} else {
			if opts.CertificadoPEM, err = leerFicheroDelFormulario(r, "certificado"); err != nil {
				httpx.Fail(w, r, err)
				return
			}
			if opts.ClavePEM, err = leerFicheroDelFormulario(r, "clave_priv"); err != nil {
				httpx.Fail(w, r, err)
				return
			}
		}
		// El sello de tiempo, si el despliegue tiene autoridad. Es lo que hace que
		// la firma siga verificándose cuando el certificado caduque; sin autoridad
		// se firma igual y la interfaz avisa de que no lleva sello.
		opts.Sello = h.selloDeTiempo(r.Context())
		// Se firma ANEXANDO (§7.5.6), que es lo que conserva las firmas anteriores:
		// dos partes pueden firmar el mismo contrato. Solo si el documento no lo
		// admite —cifrado, o con la tabla de referencias reconstruida— se cae a
		// reescribirlo, que es correcto porque entonces no hay firma previa que
		// romper (un PDF firmado y cifrado no se puede leer para firmarlo).
		salida, err = domain.FirmarIncrementalPDF(doc, opts)
		anexada := err == nil
		if errors.Is(err, domain.ErrNoIncremental) {
			// Si ya había firmas, esto devuelve ErrInvalidaFirmas (409) y la
			// interfaz pregunta antes de repetirlo con invalidar_firmas=1.
			salida, err = domain.FirmarConCertificadoPDF(doc, opts)
		}
		// Quien firma tiene que saber si se anexó o se reescribió: con certificado
		// del sistema la app avisa en ese caso (anexado = false), y aquí igual.
		w.Header().Set("X-Sytue-Firma-Anexada", strconv.FormatBool(anexada))
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("%q is not a valid operation. Use dibujada or certificado.", op),
		))
		return
	}
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "firmado.pdf")
}

// --- Herramientas: comprimir, convertir y recortar ---------------------------

// pdfHerramientas atiende POST /api/v1/pdf/tools.
//
//	op        comprimir | recortar | imagenes-a-pdf | archivo
//	nivel     sin-perdida | equilibrada | fuerte   (op=comprimir)
//	margenes  «izq,abajo,der,arriba» en puntos      (op=recortar)
func (h *Handler) pdfHerramientas(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	op := strings.TrimSpace(r.FormValue("op"))
	// «imágenes a PDF» es la única que NO recibe un PDF de entrada: lo produce.
	if op == "imagenes-a-pdf" {
		h.imagenesAPDF(w, r)
		return
	}

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	switch op {
	case "comprimir":
		nivel := domain.NivelCompresion(strings.TrimSpace(r.FormValue("nivel")))
		switch nivel {
		case domain.CompresionSinPerdida, domain.CompresionEquilibrada, domain.CompresionFuerte:
		case "":
			nivel = domain.CompresionEquilibrada
		default:
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
				"The level must be sin-perdida, equilibrada or fuerte.",
			))
			return
		}
		salida, res, err := domain.ComprimirPDF(doc, nivel)
		if err != nil {
			httpx.Fail(w, r, errorDeDominio(err))
			return
		}
		// El resultado va con su resumen en cabeceras: el cuerpo es el PDF, y sin
		// ellas la interfaz no podría decir cuánto se ahorró.
		w.Header().Set("X-Sytue-Bytes-Antes", strconv.Itoa(res.Antes))
		w.Header().Set("X-Sytue-Bytes-Despues", strconv.Itoa(res.Despues))
		w.Header().Set("X-Sytue-Ahorro", strconv.Itoa(res.Ahorro()))
		responderPDF(w, doc, salida, "comprimido.pdf")

	case "recortar":
		margenes, err := cuatroReales(r.FormValue("margenes"))
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		salida, err := domain.RecortarPDF(doc, margenes)
		if err != nil {
			httpx.Fail(w, r, errorDeDominio(err))
			return
		}
		responderPDF(w, doc, salida, "recortado.pdf")

	case "archivo":
		h.prepararArchivo(w, r, doc)

	case "comprobar-archivo":
		httpx.JSON(w, r, http.StatusOK, doc.ComprobarArchivo())

	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
			"%q is not a valid operation.", op,
		)))
	}
}

func (h *Handler) prepararArchivo(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	nivel := domain.PDFA2B
	if strings.TrimSpace(r.FormValue("nivel")) == "2A" {
		nivel = domain.PDFA2A
	}
	salida, err := domain.PrepararArchivoPDF(doc, domain.OpcionesArchivo{
		Nivel:  nivel,
		Idioma: strings.TrimSpace(r.FormValue("idioma")),
		Titulo: strings.TrimSpace(r.FormValue("titulo")),
		Autor:  strings.TrimSpace(r.FormValue("autor")),
	})
	if err != nil {
		// Negarse a declarar PDF/A es ACCIONABLE: el mensaje dice qué fuentes
		// faltan por incrustar, que es lo que hay que arreglar.
		httpx.Fail(w, r, httpx.ErrBadRequest.
			WithMessage(err.Error()).
			WithCode("pdf_archive_not_possible"))
		return
	}
	responderPDF(w, doc, salida, "archivo.pdf")
}

// imagenesAPDF construye un PDF con una página por imagen enviada.
func (h *Handler) imagenesAPDF(w http.ResponseWriter, r *http.Request) {
	if r.MultipartForm == nil || len(r.MultipartForm.File["file"]) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`Send the images in the "file" field.`))
		return
	}
	var imagenes []domain.ImagenAPagina
	for _, ch := range r.MultipartForm.File["file"] {
		f, err := ch.Open()
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("One of the images could not be read."))
			return
		}
		datos, err := io.ReadAll(io.LimitReader(f, domain.MaxImportBytes+1))
		_ = f.Close()
		if err != nil || int64(len(datos)) > domain.MaxImportBytes {
			httpx.Fail(w, r, errPayloadTooLarge)
			return
		}
		imagenes = append(imagenes, domain.ImagenAPagina{Datos: datos, Nombre: ch.Filename})
	}
	salida, err := domain.ImagenesAPDF(imagenes)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, nil, salida, "documento.pdf")
}

// cuatroReales lee «10,20,10,20».
func cuatroReales(s string) ([4]float64, error) {
	var out [4]float64
	partes := strings.Split(s, ",")
	if len(partes) != 4 {
		return out, httpx.ErrBadRequest.WithMessage(
			`Give four margins in points: "left,bottom,right,top".`,
		)
	}
	for i, p := range partes {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || v < 0 || v > 10000 {
			return out, httpx.ErrBadRequest.WithMessage(fmt.Sprintf("%q is not a valid margin.", p))
		}
		out[i] = v
	}
	return out, nil
}

// --- Editar el contenido (ola P5) --------------------------------------------

// pdfEditar atiende POST /api/v1/pdf/edit.
//
//	op       analizar | texto | imagen | capas
//	pagina   0-based (op=analizar, imagen)
//	cambios  JSON con la lista de sustituciones (op=texto)
//	id       nombre del XObject (op=imagen)
//	visibles lista de identificadores de capa separados por comas (op=capas)
//
// pdfBuscarReemplazar atiende el caso «buscar-reemplazar» de pdfEditar. Sale a
// una función propia porque son dos operaciones en una —listar coincidencias y
// aplicarlas— y juntas hacían de `pdfEditar` un switch imposible de leer.
func (h *Handler) pdfBuscarReemplazar(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	var b domain.BusquedaReemplazo
	if err := leerJSONDelFormulario(r, "busqueda", &b); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Sin `aplicar` solo se BUSCA: es lo que permite enseñar la lista, decir
	// cuáles quedan apretadas y elegir, antes de tocar el documento.
	if r.FormValue("aplicar") != "1" {
		c, err := doc.BuscarParaReemplazar(b)
		if err != nil {
			httpx.Fail(w, r, errorDeDominio(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"coincidencias": c})
		return
	}
	var elegidas []domain.Coincidencia
	if r.FormValue("elegidas") != "" {
		if err := leerJSONDelFormulario(r, "elegidas", &elegidas); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	salida, n, err := domain.ReemplazarEnDocumentoPDF(doc, b, elegidas)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	w.Header().Set("X-Sytue-Reemplazos", strconv.Itoa(n))
	responderPDF(w, doc, salida, "documento.pdf")
}

// pdfSustituirImagen atiende el caso «imagen» de pdfEditar: cambiar una imagen
// del documento o, sin fichero, quitarla.
func (h *Handler) pdfSustituirImagen(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	pagina, err := enteroOpcional(r.FormValue("pagina"), 1, doc.NumPaginas())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The image id is missing."))
		return
	}
	// Sin fichero «imagen» se QUITA, que es una operación legítima y no un
	// error: la interfaz ofrece las dos con el mismo botón.
	var nueva []byte
	if r.MultipartForm != nil && len(r.MultipartForm.File["imagen"]) > 0 {
		nueva, err = leerFicheroDelFormulario(r, "imagen")
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	salida, err := domain.SustituirImagenPDF(doc, pagina-1, id, nueva)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "editado.pdf")
}

func (h *Handler) pdfEditar(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Cada operación va en su función: juntas, el despachador crecía hasta ser
	// ilegible y cambiar una obligaba a leer las cinco de alrededor.
	switch op := strings.TrimSpace(r.FormValue("op")); op {
	case "", "analizar":
		editarAnalizar(w, r, doc)
	case "texto":
		editarTexto(w, r, doc)
	case "buscar-reemplazar":
		h.pdfBuscarReemplazar(w, r, doc)
	case "imagen":
		h.pdfSustituirImagen(w, r, doc)
	case "capas":
		editarCapas(w, r, doc)
	case "indice":
		editarIndice(w, r, doc)
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
			"%q is not a valid operation. Use analizar, texto, buscar-reemplazar, imagen, capas or indice.", op,
		)))
	}
}

func editarAnalizar(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	pagina, err := enteroOpcional(r.FormValue("pagina"), 1, doc.NumPaginas())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	analisis, err := doc.AnalizarEdicion(pagina - 1)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, analisis)
}

func editarTexto(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	var cambios []domain.CambioDeTexto
	if err := leerJSONDelFormulario(r, "cambios", &cambios); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	salida, err := domain.EditarTextoPDF(doc, cambios)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "editado.pdf")
}

func editarCapas(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	visibles, err := listaDeEnteros(r.FormValue("visibles"))
	if err != nil && strings.TrimSpace(r.FormValue("visibles")) != "" {
		httpx.Fail(w, r, err)
		return
	}
	salida, err2 := domain.CambiarCapasPDF(doc, visibles)
	if err2 != nil {
		httpx.Fail(w, r, errorDeDominio(err2))
		return
	}
	responderPDF(w, doc, salida, "capas.pdf")
}

func editarIndice(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	// Una lista vacía es LEGÍTIMA: es cómo se quita el índice que trae un
	// documento mal generado. Por eso no se exige el campo, solo que si viene
	// sea JSON válido.
	var marcadores []domain.Marcador
	if crudo := strings.TrimSpace(r.FormValue("marcadores")); crudo != "" {
		if err := leerJSONDelFormulario(r, "marcadores", &marcadores); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	salida, err := domain.EscribirIndicePDF(doc, marcadores)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	responderPDF(w, doc, salida, "indice.pdf")
}

// --- Capa de texto de un escaneo (ola P8) ------------------------------------

// pdfOCR atiende POST /api/v1/pdf/ocr.
//
//	op      estado | capa
//	textos  JSON {"0": [{texto, rect}, …]} con lo reconocido (op=capa)
//
// El RECONOCIMIENTO no se hace aquí: este endpoint acepta texto ya reconocido
// venga de donde venga, que es lo que decidió el ADR-0034. Así la herramienta
// sirve desde el primer día —un escaneo procesado fuera se puede incorporar— sin
// quedar secuestrada por qué proveedor de IA se elija.
func (h *Handler) pdfOCR(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	doc, err := unDocumento(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	switch op := strings.TrimSpace(r.FormValue("op")); op {
	case "", "estado":
		// Qué páginas están escaneadas, para ofrecer la herramienta solo donde
		// sirve: proponerla sobre un documento que ya tiene texto duplicaría la capa.
		//
		// `disponible` es lo que decide si la interfaz enseña el botón, y se
		// responde PREGUNTÁNDOLE AL SERVICIO, no mirando si hay una URL puesta.
		//
		// La diferencia importa en el perfil piloto: allí `ocr.enabled` es false y
		// Sytue OCR no se despliega —2×512 MiB que ese hardware no tiene—, pero el
		// chart de docs pasa `SYTUE_DOCS_OCR_URL` siempre y sin condición, así que
		// el cliente existe igual. Con `h.ocr != nil` como criterio, Sytue PDF
		// ofrecía «reconocer texto» apuntando a un Service que no existe: el botón
		// estaba, y solo al pulsarlo aparecía un error de red. Preguntando, la
		// herramienta se apaga sola y el resto del estudio —anotar, firmar,
		// organizar, censurar— sigue funcionando igual.
		//
		// El sondeo es el mismo que ya se hacía para los idiomas; ahora su
		// resultado decide las dos cosas. Con tope propio: sin él, un OCR colgado
		// —que no es lo mismo que ausente— dejaría el panel esperando a que
		// venciera el plazo de la petición entera.
		//
		// ponytail: sin caché. Se consulta al abrir el panel, no en bucle; si algún
		// día se nota, se guarda el resultado unos segundos.
		disponible, idiomas := ocrVivo(r.Context(), h.ocr, r.Header.Get("Authorization"))
		respuesta := map[string]any{
			"paginas_sin_texto": doc.PaginasSinTexto(),
			"total":             doc.NumPaginas(),
			"disponible":        disponible,
		}
		if len(idiomas) > 0 {
			respuesta["idiomas"] = idiomas
		}
		httpx.JSON(w, r, http.StatusOK, respuesta)

	case "reconocer":
		h.reconocerYEscribir(w, r, doc)
	case "capa":
		var textos map[int][]domain.TextoReconocido
		if err := leerJSONDelFormulario(r, "textos", &textos); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		salida, err := domain.CapaDeTextoPDF(doc, textos)
		if err != nil {
			httpx.Fail(w, r, errorDeDominio(err))
			return
		}
		responderPDF(w, doc, salida, "con-texto.pdf")
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
			"%q is not a valid operation. Use estado, reconocer or capa.", op,
		)))
	}
}

// ocrVivo responde las dos preguntas de una vez: ¿hay reconocimiento detrás y
// con qué idiomas? Devuelve `false` sin error propio —no hay nada que enseñarle
// a nadie: la herramienta simplemente no se ofrece—.
func ocrVivo(ctx context.Context, cli *ocr.Cliente, autorizacion string) (bool, []string) {
	if cli == nil {
		return false, nil
	}
	ctx, cancelar := context.WithTimeout(ctx, sondeoOCR)
	defer cancelar()
	idiomas, err := cli.Idiomas(ctx, autorizacion)
	if err != nil {
		return false, nil
	}
	return true, idiomas
}

// sondeoOCR acota lo que se espera al preguntar al servicio si está vivo. Es una
// pregunta de UI —¿enseño el botón?—, así que más vale contestar «no» rápido que
// «sí» tarde: el panel de herramientas no se pinta hasta tener la respuesta.
const sondeoOCR = 3 * time.Second

// maxPaginasPorOCR acota cuántas páginas se reconocen en una sola petición. Cada
// una es un segundo largo de CPU en un servicio compartido, así que un
// expediente entero se pide por tandas y la interfaz enseña el avance.
const maxPaginasPorOCR = 25

// reconocerYEscribir hace el viaje completo: saca la imagen escaneada de cada
// página, la manda a Sytue OCR, traduce lo reconocido a coordenadas del PDF y
// escribe la capa de texto invisible.
//
// LA CONVERSIÓN DE COORDENADAS SE QUEDA AQUÍ y no en el servicio de OCR: aquel
// solo ve una imagen y no sabe dónde está colocada en la página —ni si está
// rotada, ni a qué escala—. Mandarle esa responsabilidad obligaría a enseñarle
// PDF, que es justo lo que se quiere evitar para poder cambiarle el motor.
func (h *Handler) reconocerYEscribir(w http.ResponseWriter, r *http.Request, doc *domain.PDFDoc) {
	// Sin cliente no hay a quién preguntar. Que el servicio ESTÉ VIVO no se sondea
	// aquí a propósito: la primera llamada a `Reconocer` ya lo descubre y devuelve
	// el mismo `ocr_not_configured`, así que un sondeo previo solo añadiría una
	// vuelta de red al camino bueno para contestar lo mismo.
	if h.ocr == nil {
		httpx.Fail(w, r, errorDeOCR(ocr.ErrNoConfigurado))
		return
	}

	// Sin lista, se reconocen las páginas que NO tienen texto, que es lo que
	// quiere quien pulsa el botón sobre un escaneo.
	var indices []int
	if lista := strings.TrimSpace(r.FormValue("paginas")); lista != "" {
		var err error
		if indices, err = parsearPaginas(lista, doc.NumPaginas()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	} else {
		indices = doc.PaginasSinTexto()
	}
	if len(indices) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.
			WithMessage("This document already has text on every page.").
			WithCode("pdf_already_has_text"))
		return
	}
	if len(indices) > maxPaginasPorOCR {
		indices = indices[:maxPaginasPorOCR]
	}

	idiomas := strings.TrimSpace(r.FormValue("idiomas"))
	autorizacion := r.Header.Get("Authorization")
	porPagina := map[int][]domain.TextoReconocido{}
	var reconocidas, sinImagen int

	for _, i := range indices {
		// El caso normal es UNA imagen que es la página entera. Cuando no la hay se
		// prueban todas las que tengan tamaño suficiente: un escaneo puede llegar
		// partido en bandas —lo normal en un TIFF de gran formato convertido a
		// PDF— o con márgenes que dejan la imagen por debajo del 60 % del papel, y
		// esas páginas se contaban como «sin imagen» y no se reconocían nunca.
		var trozos []domain.ImagenColocada
		if imagen, ok := doc.ImagenDeEscaneo(i); ok {
			trozos = []domain.ImagenColocada{imagen}
		} else {
			trozos = doc.ImagenesParaOCR(i)
		}
		if len(trozos) == 0 {
			// Una página vectorial sin texto existe (un plano, un gráfico): no es un
			// escaneo y mandarla al OCR no daría nada. Se cuenta y se sigue.
			sinImagen++
			continue
		}

		var textos []domain.TextoReconocido
		for _, trozo := range trozos {
			res, err := h.ocr.Reconocer(r.Context(), trozo.Datos, idiomas, autorizacion)
			if err != nil {
				httpx.Fail(w, r, errorDeOCR(err))
				return
			}
			// Cada trozo convierte con SU colocación: las coordenadas que devuelve el
			// reconocedor son de esa imagen, no de la página.
			for _, p := range res.Palabras {
				textos = append(textos, domain.TextoReconocido{
					Texto: p.Texto,
					Rect:  trozo.RectDePDF(p.X, p.Y, p.Ancho, p.Alto),
				})
			}
		}
		if len(textos) > 0 {
			porPagina[i] = textos
			reconocidas++
		}
	}

	if len(porPagina) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.
			WithMessage("No text could be recognised on those pages.").
			WithCode("pdf_ocr_empty"))
		return
	}
	salida, err := domain.CapaDeTextoPDF(doc, porPagina)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	// El resumen va en cabeceras porque el cuerpo es el PDF; sin él, la interfaz
	// no podría decir cuántas páginas se reconocieron ni cuántas se saltaron.
	w.Header().Set("X-Sytue-Ocr-Paginas", strconv.Itoa(reconocidas))
	w.Header().Set("X-Sytue-Ocr-Sin-Imagen", strconv.Itoa(sinImagen))
	responderPDF(w, doc, salida, "con-texto.pdf")
}

// errorDeOCR traduce los fallos del reconocedor. Se distinguen porque llevan a
// sitios distintos: un idioma que falta lo arregla quien opera instalando el
// modelo; una cola llena se arregla esperando; y «no configurado» significa que
// el despliegue está a medias.
func errorDeOCR(err error) error {
	switch {
	case errors.Is(err, ocr.ErrIdioma):
		return httpx.ErrBadRequest.
			WithMessage("That language is not installed on the recognition server.").
			WithCode("ocr_language_unavailable")
	case errors.Is(err, ocr.ErrOcupado):
		return httpx.NewError(http.StatusServiceUnavailable, "ocr_busy",
			"The recognition service is busy; try again in a moment.")
	case errors.Is(err, ocr.ErrNoConfigurado):
		return httpx.NewError(http.StatusServiceUnavailable, "ocr_not_configured",
			"Text recognition is not available on this server.")
	default:
		// El texto del fallo NO SALE al cliente: viene del otro servicio y puede
		// traer su URL interna o lo que respondiera un proxy de por medio. Se
		// envuelve, que es lo que hace que `httpx.Fail` lo deje en el registro con
		// el request_id, y al navegador va una frase fija.
		return httpx.NewError(http.StatusBadGateway, "ocr_failed",
			"The recognition service could not complete the request.").Wrap(err)
	}
}

// --- Comparar ----------------------------------------------------------------

// plazoCompararVisual acota el tiempo de una comparación visual. Se mira entre página y
// página (el intérprete no se interrumpe a medias), así que una página patológica puede
// pasarse; lo que no ocurre es que veinte páginas la sumen. Es MENOR que el
// `RequestTimeout` del router (30 s): si fuera mayor, el router cortaría con un 504 antes
// de que esta comparación devolviera lo hecho hasta ahí con `truncado`.
const plazoCompararVisual = 25 * time.Second

// pdfComparar atiende POST /api/v1/pdf/compare: dos ficheros en «file» y sale
// qué cambia en cada página.
//
// Con `modo=visual` se comparan los PÍXELES de cada página en vez del texto
// (ver `domain.CompararVisual`); el modo de texto sigue siendo el de siempre.
func (h *Handler) pdfComparar(w http.ResponseWriter, r *http.Request) {
	if !h.prepararFormularioPDF(w, r) {
		return
	}
	defer limpiarFormulario(r)

	modo := strings.TrimSpace(r.FormValue("modo"))
	if modo != "" && modo != "texto" && modo != "visual" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(`The mode must be "texto" or "visual".`))
		return
	}

	docs, err := leerPDFsDelFormulario(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(docs) != 2 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Comparing needs exactly two PDFs."))
		return
	}
	if modo == "visual" {
		h.pdfCompararVisual(w, r, docs[0], docs[1])
		return
	}
	difs, err := domain.CompararPDF(docs[0], docs[1])
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"diferencias": difs,
		"iguales":     len(difs) == 0,
	})
}

// opcionesVisualesDelFormulario lee y valida los ajustes. Un valor fuera de rango se
// RECHAZA y no se recorta en silencio: quien pide una tolerancia de 900 se ha
// equivocado, y un resultado «normal» le haría creer que se aplicó.
func opcionesVisualesDelFormulario(r *http.Request) (domain.OpcionesVisual, error) {
	var o domain.OpcionesVisual
	if s := strings.TrimSpace(r.FormValue("escala")); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || !(v >= 0.25 && v <= 3) { // «!(…)» y no «< ||  >»: NaN no es menor ni mayor que nada
			return o, httpx.ErrBadRequest.WithMessage("The scale must be between 0.25 and 3.")
		}
		o.Escala = v
	}
	if s := strings.TrimSpace(r.FormValue("tolerancia")); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > 255 {
			return o, httpx.ErrBadRequest.WithMessage("The tolerance must be between 1 and 255.")
		}
		o.Tolerancia = v
	}
	if s := strings.TrimSpace(r.FormValue("vecindad")); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 || v > 3 {
			return o, httpx.ErrBadRequest.WithMessage("The neighbourhood must be between 0 and 3.")
		}
		o.Vecindad, o.SinVecindad = v, v == 0
	}
	return o, nil
}

// pdfCompararVisual responde a `modo=visual`. Sin `pagina`, el resumen de todo el
// documento (regiones y porcentajes, sin imágenes); con `pagina`, esa página con su
// resaltado en PNG (base64 en el JSON).
func (h *Handler) pdfCompararVisual(w http.ResponseWriter, r *http.Request, a, b *domain.PDFDoc) {
	opts, err := opcionesVisualesDelFormulario(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if s := strings.TrimSpace(r.FormValue("pagina")); s != "" {
		n, err := enteroOpcional(s, 1, max(a.NumPaginas(), b.NumPaginas()))
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		dif, err := domain.CompararVisualPagina(a, b, n, opts)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(err.Error()))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{
			"modo": "visual", "paginas_a": a.NumPaginas(), "paginas_b": b.NumPaginas(),
			"pagina": dif,
		})
		return
	}
	ctx, parar := context.WithTimeout(r.Context(), plazoCompararVisual)
	defer parar()
	res, err := domain.CompararVisual(ctx, a, b, opts)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"modo": "visual", "paginas_a": res.PaginasA, "paginas_b": res.PaginasB,
		"comparadas": res.Comparadas, "truncado": res.Truncado,
		"paginas": res.Paginas, "iguales": res.Iguales,
	})
}

// --- Ayudas ------------------------------------------------------------------

// leerJSONDelFormulario decodifica un campo de texto que lleva JSON. Se rechaza
// lo que no encaje con el destino en vez de ignorarlo: un cliente que manda
// «color» donde se espera «colour» merece enterarse, no que su anotación salga
// negra sin motivo.
func leerJSONDelFormulario(r *http.Request, campo string, destino any) error {
	crudo := r.FormValue(campo)
	if strings.TrimSpace(crudo) == "" {
		return httpx.ErrBadRequest.WithMessage(fmt.Sprintf("The %q field is missing.", campo))
	}
	if len(crudo) > maxTamanoCampoJSON {
		return errPayloadTooLarge
	}
	dec := json.NewDecoder(strings.NewReader(crudo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		return httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("The %q field is not valid JSON: %v", campo, err),
		)
	}
	return nil
}

// nombreDelFicheroSubido devuelve el nombre del PDF que llegó, para que el XFDF
// diga a qué documento pertenecen los datos. Es informativo: si no se sabe, se
// va vacío en vez de inventarlo.
func nombreDelFicheroSubido(r *http.Request) string {
	if r.MultipartForm == nil {
		return ""
	}
	fs := r.MultipartForm.File["file"]
	if len(fs) == 0 {
		return ""
	}
	// El nombre lo pone el cliente: se queda el último tramo, sin rutas.
	n := fs[0].Filename
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	return n
}

// leerFicheroDelFormulario lee un fichero pequeño (un certificado, una clave).
func leerFicheroDelFormulario(r *http.Request, campo string) ([]byte, error) {
	const maxTamano = 256 << 10
	if r.MultipartForm == nil {
		return nil, httpx.ErrBadRequest.WithMessage(fmt.Sprintf("The %q file is missing.", campo))
	}
	cabeceras := r.MultipartForm.File[campo]
	if len(cabeceras) == 0 {
		return nil, httpx.ErrBadRequest.WithMessage(fmt.Sprintf("The %q file is missing.", campo))
	}
	f, err := cabeceras[0].Open()
	if err != nil {
		return nil, httpx.ErrBadRequest.WithMessage(fmt.Sprintf("The %q file could not be read.", campo))
	}
	defer func() { _ = f.Close() }()
	datos, err := io.ReadAll(io.LimitReader(f, maxTamano+1))
	if err != nil || len(datos) > maxTamano {
		return nil, httpx.ErrBadRequest.WithMessage(fmt.Sprintf("The %q file is too large.", campo))
	}
	return datos, nil
}

// listaDeEnteros lee «0,2,5» y devuelve los índices.
func listaDeEnteros(s string) ([]int, error) {
	var out []int
	for _, trozo := range strings.Split(s, ",") {
		trozo = strings.TrimSpace(trozo)
		if trozo == "" {
			continue
		}
		n, err := strconv.Atoi(trozo)
		if err != nil || n < 0 {
			return nil, httpx.ErrBadRequest.WithMessage(
				fmt.Sprintf("%q is not a valid index.", trozo),
			)
		}
		out = append(out, n)
		if len(out) > maxPaginasPorPeticion {
			return nil, httpx.ErrBadRequest.WithMessage("Too many items in a single request.")
		}
	}
	if len(out) == 0 {
		return nil, httpx.ErrBadRequest.WithMessage("No item was given.")
	}
	return out, nil
}

// errorDeDominio traduce los errores del motor a una respuesta de cliente. Casi
// todos lo son —un rango que no existe, un certificado que no cuadra— y
// devolverlos como 500 escondería la causa a quien puede arreglarla.
func errorDeDominio(err error) error {
	switch {
	case errors.Is(err, domain.ErrCertificadoNoUsable):
		return httpx.ErrBadRequest.
			WithMessage("The certificate or the private key cannot be used to sign.").
			WithCode("pdf_bad_certificate")
	case errors.Is(err, domain.ErrCertificarFirmado):
		return httpx.ErrBadRequest.
			WithMessage("Only the first signature can certify the document.").
			WithCode("pdf_certify_signed")
	case errors.Is(err, domain.ErrFirmaNoCabe):
		return httpx.ErrBadRequest.
			WithMessage("The certificate chain is too long for the reserved signature space.").
			WithCode("pdf_signature_too_large")
	case errors.Is(err, domain.ErrInvalidaFirmas):
		// 409 y no 400: la petición es correcta, pero guardarla destruiría las
		// firmas. La interfaz lo pregunta y la repite con invalidar_firmas=1.
		return httpx.ErrConflict.
			WithMessage("Saving this change would invalidate the signatures of the document: " + err.Error()).
			WithCode("pdf_invalidates_signatures")
	case errors.Is(err, domain.ErrLetrasDesconocidas):
		return httpx.ErrBadRequest.
			WithMessage("The font of this text does not say which letters it uses, so it cannot be rewritten.").
			WithCode("pdf_unknown_letters")
	case errors.Is(err, domain.ErrPDFClaveIncorrecta):
		return httpx.ErrBadRequest.
			WithMessage("The password is not correct.").
			WithCode("pdf_password_required")
	default:
		return httpx.ErrBadRequest.WithMessage(err.Error())
	}
}
