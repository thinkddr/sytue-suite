// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Operaciones de página sobre un PDF (ola P4 de Sytue PDF): combinar, extraer,
// reordenar, rotar y borrar.
//
// Es una conversión PURA, igual que `/api/v1/import`: entra uno o varios PDF por
// multipart y sale el PDF resultante. No toca la base de datos ni hay documento
// que autorizar, porque el fichero lo aporta quien llama — la verja es la sesión
// (RequireAuth + RequireTenant, que aplica el router). Nada se guarda ni se
// registra: lo que entra se procesa en memoria y se devuelve.
//
// Que esto exista dentro de la suite es la mitad del valor: partir un PDF en dos
// obliga hoy a subirlo a una web cualquiera, y ahí acaban contratos y nóminas.

// maxPaginasPorPeticion acota cuántas páginas puede pedir una sola llamada. Sin
// tope, «extrae estas páginas» con una lista de un millón de índices haría
// trabajar al servidor sin límite aunque el fichero de entrada sea diminuto.
const maxPaginasPorPeticion = 10000

// maxFicherosPorPeticion acota cuántos PDF se combinan de una vez.
const maxFicherosPorPeticion = 64

// pdfPages atiende POST /api/v1/pdf/pages.
//
// multipart/form-data:
//   - `file`      uno o varios PDF (varios solo tienen sentido con op=combinar)
//   - `op`        combinar | extraer | borrar | rotar
//   - `paginas`   lista 1-based separada por comas: «1,3,5» o rangos «2-7»
//   - `grados`    para op=rotar: 90, 180, 270 o -90 (se suman a la rotación actual)
//
// Las páginas se numeran DESDE 1 en la API porque es lo que el usuario ve en su
// visor; dentro se trabaja 0-based. Mezclar los dos criterios es la forma más
// fácil de borrar la página equivocada, así que la conversión se hace en un solo
// sitio y se valida el rango antes de tocar nada.
func (h *Handler) pdfPages(w http.ResponseWriter, r *http.Request) {
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
			`Send the PDFs as multipart/form-data in the "file" field.`,
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

	op := strings.TrimSpace(r.FormValue("op"))
	salida, err := aplicarOperacionDePagina(op, docs, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if len(docs) == 1 {
		avisarPDFA(w, docs[0], salida)
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.Itoa(len(salida)))
	// Ni cachés intermedias ni el navegador deben guardar el resultado: puede
	// ser un contrato, y la petición ya lleva la sesión del usuario.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="documento.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(salida)
}

// leerPDFsDelFormulario abre todos los ficheros del campo «file».
func leerPDFsDelFormulario(r *http.Request) ([]*domain.PDFDoc, error) {
	if r.MultipartForm == nil {
		return nil, httpx.ErrBadRequest.WithMessage(`The PDF is missing ("file" field).`)
	}
	cabeceras := r.MultipartForm.File["file"]
	if len(cabeceras) == 0 {
		return nil, httpx.ErrBadRequest.WithMessage(`The PDF is missing ("file" field).`)
	}
	if len(cabeceras) > maxFicherosPorPeticion {
		return nil, httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("Too many files in a single request (maximum %d).", maxFicherosPorPeticion),
		)
	}

	// La contraseña llega en el mismo formulario y vale para todos los ficheros de
	// la petición: combinar dos PDF protegidos con claves distintas es un caso que
	// no existe fuera de un laboratorio.
	clave := r.FormValue("clave")
	// Quien ya ha confirmado que acepta invalidar las firmas lo dice aquí; sin
	// esto, un cambio que no se puede anexar a un PDF firmado se rechaza con 409
	// (errorDeDominio) en vez de reescribirlo en silencio.
	invalidar := r.FormValue("invalidar_firmas") == "1" || r.FormValue("invalidar_firmas") == "true"
	docs := make([]*domain.PDFDoc, 0, len(cabeceras))
	for _, ch := range cabeceras {
		d, err := abrirPDFDeParte(ch, clave)
		if err != nil {
			return nil, err
		}
		if invalidar {
			d.PermitirInvalidarFirmas()
		}
		docs = append(docs, d)
	}
	return docs, nil
}

func abrirPDFDeParte(ch *multipart.FileHeader, clave string) (*domain.PDFDoc, error) {
	f, err := ch.Open()
	if err != nil {
		return nil, httpx.ErrBadRequest.WithMessage("One of the files sent could not be read.")
	}
	defer func() { _ = f.Close() }()

	datos, err := io.ReadAll(io.LimitReader(f, domain.MaxPDFBytes+1))
	if err != nil {
		return nil, errPayloadTooLarge
	}
	if int64(len(datos)) > domain.MaxPDFBytes {
		return nil, errPayloadTooLarge
	}

	d, err := domain.LeerPDFConClave(datos, clave)
	if err != nil {
		return nil, traducirErrorPDF(err, ch.Filename)
	}
	return d, nil
}

// traducirErrorPDF convierte los errores del motor en respuestas que la interfaz
// pueda mostrar tal cual. Un PDF cifrado no es «error del servidor»: es algo que
// el usuario puede resolver, y merece decírselo.
//
// CADA CASO LLEVA SU CÓDIGO. La frase está en inglés y no la puede traducir nadie;
// el código sí (`ERRORES` de la web, `erroresPDF` de Fyne). Con los cuatro como
// `bad_request` a secas, «no es un PDF» y «está dañado» se veían iguales.
func traducirErrorPDF(err error, nombre string) error {
	switch {
	case errors.Is(err, domain.ErrPDFClaveIncorrecta):
		// Se distingue de los demás con un código propio para que la interfaz
		// pueda PEDIR la contraseña en vez de enseñar un error y quedarse ahí.
		return httpx.ErrBadRequest.
			WithMessage(fmt.Sprintf("“%s” needs a password to be opened.", nombre)).
			WithCode("pdf_password_required")
	case errors.Is(err, domain.ErrPDFCifrado):
		return httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
			"“%s” uses a protection scheme we cannot open (a third-party security handler).", nombre,
		)).WithCode("pdf_encryption_unsupported")
	case errors.Is(err, domain.ErrPDFNoEsPDF):
		return httpx.ErrBadRequest.WithMessage(fmt.Sprintf("“%s” is not a PDF.", nombre)).
			WithCode("pdf_not_pdf")
	case errors.Is(err, domain.ErrPDFNoSoportado):
		return httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
			"“%s” uses a PDF feature we do not support yet.", nombre,
		)).WithCode("pdf_unsupported")
	default:
		return httpx.ErrBadRequest.WithMessage(fmt.Sprintf("“%s” could not be read.", nombre)).
			WithCode("pdf_unreadable")
	}
}

// aplicarOperacionDePagina ejecuta la operación pedida sobre los documentos.
func aplicarOperacionDePagina(op string, docs []*domain.PDFDoc, r *http.Request) ([]byte, error) {
	paginas, grados := r.FormValue("paginas"), r.FormValue("grados")
	switch op {
	case "combinar":
		if len(docs) < 2 {
			return nil, httpx.ErrBadRequest.WithMessage("Combining needs at least two files.")
		}
		return domain.CombinarPDF(docs...)

	case "extraer", "borrar", "rotar":
		if len(docs) != 1 {
			return nil, httpx.ErrBadRequest.WithMessage(
				fmt.Sprintf("“%s” works on a single file.", op),
			)
		}
		doc := docs[0]
		indices, err := parsearPaginas(paginas, doc.NumPaginas())
		if err != nil {
			return nil, err
		}
		switch op {
		case "extraer":
			return domain.ExtraerPaginasPDF(doc, indices)
		case "borrar":
			datos, err := domain.BorrarPaginasPDF(doc, indices)
			if err != nil {
				// El motor se niega a dejar el documento sin páginas; es un
				// error del usuario, no del servidor.
				return nil, httpx.ErrBadRequest.WithMessage(
					"All the pages of the document cannot be deleted.",
				)
			}
			return datos, nil
		default:
			g, err := parsearGrados(grados)
			if err != nil {
				return nil, err
			}
			giros := make(map[int]int, len(indices))
			for _, i := range indices {
				giros[i] = g
			}
			return domain.RotarPaginasPDF(doc, giros)
		}

	case "marca-agua", "fondo", "numerar", "bates", "encabezado", "pie":
		if len(docs) != 1 {
			return nil, httpx.ErrBadRequest.WithMessage(
				fmt.Sprintf("“%s” works on a single file.", op),
			)
		}
		return aplicarCapa(op, docs[0], r)

	case "":
		return nil, httpx.ErrBadRequest.WithMessage(
			"The op is missing: combinar, extraer, borrar, rotar, marca-agua, " +
				"numerar, bates, encabezado or pie.",
		)
	default:
		return nil, httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("Unknown op: %q.", op),
		)
	}
}

// parsearPaginas convierte «1,3,5-8» en índices 0-based, validando el rango
// contra el documento real. Se conserva el ORDEN dado y se permiten repetidas:
// reordenar es pedir las páginas en otro orden, y duplicar una página es una
// operación legítima.
func parsearPaginas(spec string, total int) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, httpx.ErrBadRequest.WithMessage(
			`Give the pages, for example "1,3,5-8".`,
		)
	}
	var out []int
	for _, trozo := range strings.Split(spec, ",") {
		trozo = strings.TrimSpace(trozo)
		if trozo == "" {
			continue
		}
		desde, hasta, err := parsearRango(trozo)
		if err != nil {
			return nil, err
		}
		if desde < 1 || hasta > total {
			return nil, httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
				"Page %s does not exist: the document has %d.", trozo, total,
			))
		}
		paso := 1
		if hasta < desde {
			paso = -1 // «8-5» es válido y significa esas páginas al revés
		}
		for p := desde; ; p += paso {
			out = append(out, p-1) // a 0-based, en un solo sitio
			if len(out) > maxPaginasPorPeticion {
				return nil, httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
					"Too many pages in a single request (maximum %d).", maxPaginasPorPeticion,
				))
			}
			if p == hasta {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, httpx.ErrBadRequest.WithMessage(`Give the pages, for example "1,3,5-8".`)
	}
	return out, nil
}

// parsearRango entiende «5» y «5-8». Devuelve ambos extremos 1-based.
func parsearRango(trozo string) (int, int, error) {
	malo := httpx.ErrBadRequest.WithMessage(fmt.Sprintf(
		"%q could not be understood. Use numbers and ranges, for example \"1,3,5-8\".", trozo,
	))

	if i := strings.IndexByte(trozo, '-'); i > 0 { // > 0: un «-» inicial sería negativo
		a, err1 := strconv.Atoi(strings.TrimSpace(trozo[:i]))
		b, err2 := strconv.Atoi(strings.TrimSpace(trozo[i+1:]))
		if err1 != nil || err2 != nil {
			return 0, 0, malo
		}
		return a, b, nil
	}
	n, err := strconv.Atoi(trozo)
	if err != nil {
		return 0, 0, malo
	}
	return n, n, nil
}

// parsearGrados valida el giro. Solo múltiplos de 90: PDF no admite otra cosa en
// /Rotate, y aceptar 45 escribiría un fichero que los visores ignoran.
func parsearGrados(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, httpx.ErrBadRequest.WithMessage("The rotation is missing: 90, 180, 270 or -90.")
	}
	g, err := strconv.Atoi(s)
	if err != nil || g%90 != 0 {
		return 0, httpx.ErrBadRequest.WithMessage("The rotation must be a multiple of 90 degrees.")
	}
	return ((g % 360) + 360) % 360, nil
}

// aplicarCapa ejecuta las operaciones que SUPERPONEN texto: marca de agua,
// numeración —corriente o Bates— y encabezado o pie.
//
// El texto lo escribe el usuario, así que se acota su longitud: una cadena de un
// megabyte se dibujaría en cada página y el fichero resultante sería absurdo.
func aplicarCapa(op string, doc *domain.PDFDoc, r *http.Request) ([]byte, error) {
	const maxTexto = 500

	texto := strings.TrimSpace(r.FormValue("texto"))
	if op != "numerar" && op != "bates" && texto == "" {
		return nil, httpx.ErrBadRequest.WithMessage("The text to be written is missing.")
	}
	if len([]rune(texto)) > maxTexto {
		return nil, httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("The text is too long (maximum %d characters).", maxTexto),
		)
	}

	switch op {
	case "marca-agua":
		return domain.MarcaDeAguaPDF(doc, texto)

	case "fondo":
		return domain.FondoPDF(doc, texto)

	case "numerar":
		return domain.NumerarPaginasPDF(doc, texto, posicionDe(r.FormValue("posicion")))

	case "bates":
		desde, err := enteroOpcional(r.FormValue("desde"), 1, 1_000_000_000)
		if err != nil {
			return nil, err
		}
		digitos, err := enteroOpcional(r.FormValue("digitos"), 6, 12)
		if err != nil {
			return nil, err
		}
		return domain.NumeracionBatesPDF(doc, texto, desde, digitos)

	case "encabezado":
		return domain.EncabezadoPiePDF(doc, texto, domain.CapaSuperiorCen)

	default: // "pie"
		return domain.EncabezadoPiePDF(doc, texto, domain.CapaInferiorCen)
	}
}

// posicionDe traduce el nombre de posición que llega del cliente. Uno
// desconocido cae en el pie centrado en vez de dar error: es una preferencia
// estética, y fallar por ella tiraría el trabajo entero.
func posicionDe(s string) domain.PosicionCapa {
	switch strings.TrimSpace(s) {
	case "superior-izquierda":
		return domain.CapaSuperiorIzq
	case "superior-centro":
		return domain.CapaSuperiorCen
	case "superior-derecha":
		return domain.CapaSuperiorDer
	case "inferior-izquierda":
		return domain.CapaInferiorIzq
	case "inferior-derecha":
		return domain.CapaInferiorDer
	default:
		return domain.CapaInferiorCen
	}
}

// enteroOpcional lee un entero del formulario con valor por defecto y techo.
//
// El suelo es 1 y no un parámetro: los dos campos que pasan por aquí —la página
// desde la que empezar y los dígitos del número— cuentan desde uno, y un suelo
// configurable que siempre vale lo mismo es una opción que no existe.
//
// El techo no se llama `max`: taparía el builtin homónimo dentro de la función y
// cualquier uso posterior de `max(a, b)` aquí fallaría al compilar.
func enteroOpcional(s string, porDefecto, maximo int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return porDefecto, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > maximo {
		return 0, httpx.ErrBadRequest.WithMessage(
			fmt.Sprintf("%q is not valid: a number between 1 and %d is expected.", s, maximo),
		)
	}
	return n, nil
}
