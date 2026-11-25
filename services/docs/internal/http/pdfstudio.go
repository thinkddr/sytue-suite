// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
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
	if change := domain.ArchivalChange(entrada, salida); change != "" {
		w.Header().Set("X-Sytue-PDFA", change)
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
	fields := map[string]string{}
	for _, key := range []string{"escala", "sin_anotaciones", "formato", "paginas", "pagina"} {
		fields[key] = r.FormValue(key)
	}
	result, err := domain.RenderView(r.Context(), doc, fields)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(result.Data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Data)
}

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
	httpx.JSON(w, r, http.StatusOK, domain.Summary(doc, presupuestoExtractos))
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
	pages, err := domain.TextSpans(r.Context(), doc)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"paginas": pages})
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

	salida, err := domain.ApplyAnnotationOperation(doc, domain.AnnotationOperationInput{Fields: map[string]string{
		"op": r.FormValue("op"), "anotaciones": r.FormValue("anotaciones"), "indices": r.FormValue("indices"),
		"indice": r.FormValue("indice"), "texto": r.FormValue("texto"), "estado": r.FormValue("estado"),
	}, Author: quienFirma(r)})
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
	input := domain.FormOperationInput{Fields: map[string]string{}, Filename: nombreDelFicheroSubido(r)}
	for _, field := range []string{"op", "pagina", "campos", "valores"} {
		input.Fields[field] = r.FormValue(field)
	}
	if strings.TrimSpace(input.Fields["op"]) == "importar-datos" {
		input.Data, err = leerFicheroDelFormulario(r, "datos")
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		input.DataProvided = true
	}
	result, err := domain.ApplyFormOperation(doc, input)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	if imported := domain.FormImportedFieldsHeader(result); imported != "" {
		w.Header().Set("X-Sytue-Campos-Importados", imported)
	}
	if result.ContentType == "application/pdf" {
		responderPDF(w, doc, result.Data, "formulario.pdf")
		return
	}
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(result.Data)))
	w.Header().Set("Cache-Control", "no-store")
	if result.ContentType == "application/vnd.adobe.xfdf" {
		w.Header().Set("Content-Disposition", `attachment; filename="datos.xfdf"`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Data)
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

	result, err := domain.ApplyProtectionOperation(doc, map[string]string{
		"op": r.FormValue("op"), "usuario": r.FormValue("usuario"), "dueno": r.FormValue("dueno"),
		"permisos": r.FormValue("permisos"), "metadatos": r.FormValue("metadatos"), "areas": r.FormValue("areas"),
	})
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	if result.UnchangedXMP {
		w.Header().Set("X-Sytue-Metadatos-Xmp", "1")
	}
	responderPDF(w, doc, result.Data, "documento.pdf")
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

	result, err := domain.ApplyToolOperation(doc, map[string]string{
		"op": op, "nivel": r.FormValue("nivel"), "margenes": r.FormValue("margenes"),
		"idioma": r.FormValue("idioma"), "titulo": r.FormValue("titulo"), "autor": r.FormValue("autor"),
	})
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	for name, value := range domain.ToolOperationHeaders(result) {
		w.Header().Set(name, value)
	}
	if result.ContentType == "application/json" {
		w.Header().Set("Content-Type", result.ContentType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(result.Data)
		return
	}
	name := "archivo.pdf"
	if op == "comprimir" {
		name = "comprimido.pdf"
	}
	if op == "recortar" {
		name = "recortado.pdf"
	}
	responderPDF(w, doc, result.Data, name)
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

// --- Editar el contenido (ola P5) --------------------------------------------

// pdfEditar atiende POST /api/v1/pdf/edit.
//
//	op       analizar | texto | imagen | capas
//	pagina   1-based (op=analizar, imagen)
//	cambios  JSON con la lista de sustituciones (op=texto)
//	id       nombre del XObject (op=imagen)
//	visibles lista de identificadores de capa separados por comas (op=capas)
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
	input := domain.EditOperationInput{Fields: map[string]string{}}
	for _, name := range []string{"op", "pagina", "cambios", "busqueda", "aplicar", "elegidas", "id", "visibles", "marcadores"} {
		input.Fields[name] = r.FormValue(name)
	}
	if strings.TrimSpace(input.Fields["op"]) == "imagen" && r.MultipartForm != nil && len(r.MultipartForm.File["imagen"]) > 0 {
		input.Image, err = leerFicheroDelFormulario(r, "imagen")
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		defer clear(input.Image)
	}
	result, err := domain.ApplyEditOperation(doc, input)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	for name, value := range domain.EditOperationHeaders(result) {
		w.Header().Set(name, value)
	}
	if result.ContentType == "application/json" {
		w.Header().Set("Cache-Control", "no-store")
		httpx.JSON(w, r, http.StatusOK, json.RawMessage(result.Data))
		return
	}
	name := "documento.pdf"
	switch strings.TrimSpace(input.Fields["op"]) {
	case "texto", "imagen":
		name = "editado.pdf"
	case "capas":
		name = "capas.pdf"
	case "indice":
		name = "indice.pdf"
	}
	responderPDF(w, doc, result.Data, name)
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

// pdfComparar atiende POST /api/v1/pdf/compare: dos ficheros en «file» y sale
// qué cambia en cada página.
//
// Con `modo=visual` se comparan los PÍXELES de cada página en vez del texto
// (ver `domain.CompararVisual`); el modo de texto sigue siendo el de siempre. El
// cuerpo lo monta el motor (`domain.ApplyCompareOperation`), el mismo que compara
// en el dispositivo con el Drive en la bóveda (ADR 0086).
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
	fields := map[string]string{}
	for _, key := range []string{"modo", "pagina", "escala", "tolerancia", "vecindad"} {
		fields[key] = r.FormValue(key)
	}
	out, err := domain.ApplyCompareOperation(r.Context(), docs[0], docs[1], fields)
	if err != nil {
		httpx.Fail(w, r, errorDeDominio(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, json.RawMessage(out))
}

// --- Ayudas ------------------------------------------------------------------

// leerJSONDelFormulario decodifica un campo de texto que lleva JSON. Se rechaza
// lo que no encaje con el destino en vez de ignorarlo: un cliente que manda
// «color» donde se espera «colour» merece enterarse, no que su anotación salga
// negra sin motivo.
func leerJSONDelFormulario(r *http.Request, campo string, destino any) error {
	if err := domain.DecodeOperationJSON(r.FormValue(campo), campo, destino); err != nil {
		return errorDeDominio(err)
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
	return domain.UploadedDocumentName(fs[0].Filename)
}

// leerFicheroDelFormulario lee un fichero pequeño (un certificado, una clave).
func leerFicheroDelFormulario(r *http.Request, campo string) ([]byte, error) {
	const maxTamano = domain.MaxOperationFileBytes
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

// errorDeDominio traduce los errores del motor a una respuesta de cliente. Casi
// todos lo son —un rango que no existe, un certificado que no cuadra— y
// devolverlos como 500 escondería la causa a quien puede arreglarla.
func errorDeDominio(err error) error {
	failure := domain.DescribeOperationError(err)
	return httpx.NewError(failure.Status, failure.Code, failure.Message)
}
