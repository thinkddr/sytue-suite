// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Índice de un PDF: qué páginas tiene, cómo son y qué dice cada una.
//
// Existe para resolver hoy el problema que el visor propio resolverá del todo:
// elegir páginas sin escribir «1,3,5-8» de memoria. Rasterizar un PDF a imagen
// sin dependencias externas es el trabajo más caro de todo Sytue PDF, pero para
// RECONOCER una página no hace falta verla — basta su número, su tamaño y las
// primeras palabras. Con eso el usuario sabe cuál es la portada, cuál el anexo y
// cuál la que quiere quitar.
//
// Cuando exista el render fiel, esta respuesta se le añade la miniatura y la
// interfaz no cambia de forma.

// paginaIndice describe una página para poder reconocerla en una lista.
type paginaIndice struct {
	// Numero es 1-based, como lo ve el usuario en su visor.
	Numero int `json:"numero"`
	// Ancho y Alto en puntos, ya con la rotación aplicada.
	Ancho float64 `json:"ancho"`
	Alto  float64 `json:"alto"`
	// Rotacion en grados, tal cual la declara el documento.
	Rotacion int `json:"rotacion"`
	// Caja es el MediaBox ordenado [x0 y0 x1 y1]. Lo necesita el editor para
	// pasar de las coordenadas del ratón a las del PDF: con un origen que no sea
	// (0,0) —que existe— un resaltado caería desplazado por el margen.
	Caja [4]float64 `json:"caja"`
	// Extracto son las primeras palabras de la página, para reconocerla. Vacío
	// en una página que solo tiene imágenes, que también es información útil.
	// Ausente (nil) en la que no se llegó a leer: `/pdf/content` corta por tiempo
	// (presupuestoExtractos), y una página sin leer NO es una página sin texto.
	Extracto *string `json:"extracto,omitempty"`
}

// pdfInspect atiende POST /api/v1/pdf/inspect: recibe un PDF y devuelve su
// índice. Conversión pura como el resto del grupo — nada se guarda.
func (h *Handler) pdfInspect(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	const framingMargin = 1 << 20
	r.Body = http.MaxBytesReader(w, r.Body, domain.MaxPDFBytes+framingMargin)
	//nolint:gosec // G120: el cuerpo ya está acotado por http.MaxBytesReader
	if err := r.ParseMultipartForm(domain.MaxPDFBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(w, r, errPayloadTooLarge)
			return
		}
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			`Send the PDF as multipart/form-data in the "file" field.`,
		))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	docs, err := leerPDFsDelFormulario(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(docs) != 1 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Send a single PDF."))
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"paginas": indiceDe(docs[0], maxPaginasIndexadas, 0)})
}

// maxPaginasIndexadas acota los EXTRACTOS. Un PDF de diez mil páginas existe —un
// expediente escaneado— y no tiene sentido mandar diez mil extractos al
// navegador. `/pdf/inspect` (el orden de páginas de Drive) se queda además en
// las primeras, y la interfaz ofrece escribir el rango a mano.
const maxPaginasIndexadas = 500

// presupuestoExtractos es lo que `/pdf/content` se permite gastar en extractos.
//
// Sacar el texto de una página es leerla entera, y el contenido los sacaba de
// TODAS antes de contestar: medido con el motor real (arnés del frontal,
// 2026-10-16), el 100 % del tiempo de abrir un documento —un BOE de 43 páginas,
// 45 s con la máquina cargada; un formulario de 9, 9,4 s— para escribir 120
// caracteres bajo cada miniatura. Y además las cortaba en 500: en un expediente
// de 1192 páginas, de la 501 en adelante no existían en el visor.
//
// Ahora la lista lleva todas las páginas —su tamaño cuesta nada— y el extracto
// de las que dé tiempo a leer. La miniatura sigue enseñando la página dibujada.
const presupuestoExtractos = 300 * time.Millisecond

// indiceDe construye el índice de las `hasta` primeras páginas, con el extracto de
// las que quepan en `presupuesto` (0: de todas las que lo admitan).
func indiceDe(doc *domain.PDFDoc, hasta int, presupuesto time.Duration) []paginaIndice {
	n := min(doc.NumPaginas(), hasta)
	out := make([]paginaIndice, 0, n)
	inicio := time.Now()
	for i := 0; i < n; i++ {
		p, err := doc.Pagina(i)
		if err != nil {
			continue
		}
		pi := paginaIndice{
			Numero:   i + 1,
			Ancho:    p.Ancho,
			Alto:     p.Alto,
			Rotacion: p.Rotacion,
			Caja:     p.Caja,
		}
		// La primera SIEMPRE: es la que dice si el documento es un escaneo sin texto.
		if i < maxPaginasIndexadas && (presupuesto == 0 || i == 0 || time.Since(inicio) < presupuesto) {
			texto, err := doc.TextoPlano(i)
			if err != nil {
				// Una página ilegible no invalida el índice: se devuelve con el
				// extracto vacío, que sigue permitiendo elegirla por número y tamaño.
				texto = ""
			}
			e := extractoDe(texto, 120)
			pi.Extracto = &e
		}
		out = append(out, pi)
	}
	return out
}

// extractoDe recorta el texto a un resumen de una línea: colapsa los espacios y
// corta por palabra, no a mitad de una, que se lee peor de lo que ocupa.
// El parámetro no se llama `max`: eso tapa el builtin del mismo nombre dentro de
// la función, y quien lea `max(a, b)` aquí dentro se lleva un error de compilación
// difícil de explicar.
func extractoDe(texto string, maxRunas int) string {
	// strings.Fields ya parte por unicode.IsSpace y colapsa los espacios seguidos:
	// es exactamente lo que hacía el FieldsFunc con su lambda.
	limpio := strings.Join(strings.Fields(texto), " ")
	if limpio == "" {
		return ""
	}
	runas := []rune(limpio)
	if len(runas) <= maxRunas {
		return limpio
	}
	corte := string(runas[:maxRunas])
	if i := strings.LastIndexByte(corte, ' '); i > maxRunas/2 {
		corte = corte[:i]
	}
	return corte + "…"
}
