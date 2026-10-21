// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/voz"
)

/*
Lo que la pantalla le pide al motor: abrir, dibujar por lotes y aplicar cambios. Es la
mitad de `PdfApp.tsx` que no pinta.

Todo va en su propia rutina —habla con la red— y vuelve a la interfaz con `fyne.Do`. Lo
que llega tarde se tira: si entretanto se abrió otro documento o cambió la escala del
dibujo, pintar lo que llega sería pintar lo de antes.
*/

// plazo es cuánto se espera a una operación del motor. Un PDF de 25 MiB por una conexión
// mala sube en ese tiempo; más, y quien espera ya ha pensado que no funciona.
const plazo = 2 * time.Minute

// Abrir enseña un documento: `cargar(”)` de la web.
func (p *Pantalla) Abrir(doc *Documento) {
	if p.callar != nil {
		p.Callar()
	}
	// Otro documento empieza como una página recién cargada, que es lo que es en la web
	// abrir otro: ni la herramienta, ni la pestaña de la cinta, ni el panel con lo
	// buscado, lo que dijo el Asistente o un índice a medio editar pasan al siguiente.
	// Volver a leer el MISMO documento tras un cambio no pasa por aquí y lo conserva.
	if p.dialogo != nil {
		p.dialogo.cerrar()
	}
	p.barra.reiniciar()
	p.panel, p.PanelAbierto, p.DosPaginas, p.Zoom, p.tipoCampo = nil, false, false, 1, "texto"
	p.doc = doc
	p.Contenido, p.visor, p.hojas, p.textos = nil, nil, nil, nil
	p.PideClave, p.ErrorClave, p.Aviso, p.Error, p.Copia = false, "", "", "", ""
	p.PaginaActual, p.Ajuste = 1, AjusteAncho
	p.zonas, p.camposNuevos = nil, nil
	p.olvidarTramos()
	p.peticion, p.sinEntregar, p.bloqueando = nil, "", false
	p.cargar("")
	if pend := p.porEntregar; pend != nil {
		// Es el firmado que vuelve de la aplicación oficial: se entrega a quien lo pidió.
		p.porEntregar = nil
		p.entregarFirma(*pend, doc.Ruta())
		return
	}
	p.buscarPeticion(doc)
}

// FalloAlAbrir enseña por qué no se pudo ni traer el fichero.
func (p *Pantalla) FalloAlAbrir(err error) {
	p.Contenido = nil
	p.Estado, p.Error = p.estadoDeError(err), p.mensajeDeError(err, "pdf.error.open")
	p.Recomponer()
}

// Reintentar vuelve a pedir el documento con la contraseña que hubiera.
func (p *Pantalla) Reintentar() {
	if p.doc == nil {
		if p.AlReintentar != nil {
			p.AlReintentar()
		}
		return
	}
	p.cargar(p.claveUsada)
}

func (p *Pantalla) enviarClave(c string) { p.cargar(c) }

// cargar es `cargar` de `PdfApp.tsx`: pide el contenido; si el documento pide una
// contraseña, abre su formulario en vez de un aviso rojo.
func (p *Pantalla) cargar(clave string) {
	doc := p.doc
	if doc == nil {
		return
	}
	p.Error = ""
	if p.Contenido == nil {
		p.Estado = editor.Cargando
	}
	p.Recomponer()
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		doc.PonerClave(clave)
		c, err := doc.Contenido(ctx)
		fyne.Do(func() {
			defer p.avisarRed()
			if p.doc != doc {
				return
			}
			if err != nil {
				if api.PDFPideClave(err) {
					// La web no dice nada más: vuelve a enseñar el formulario, con el
					// error que hubiera (ninguno, porque `cargar` lo limpia).
					p.PideClave, p.ErrorClave = true, p.Error
				} else {
					p.Estado, p.Error = p.estadoDeError(err), p.mensajeDeError(err, "pdf.error.open")
				}
				p.Recomponer()
				return
			}
			p.PideClave, p.claveUsada = false, clave
			p.PonerContenido(c)
		})
	}()
}

// pedirLote es `pedirLote`: dibuja las páginas desde `desde` hasta `desde+3` que falten.
// Una página que el motor no supo dibujar no se reintenta sin fin: se marca y la hoja
// enseña el hueco.
func (p *Pantalla) pedirLote(desde int) {
	doc := p.doc
	if doc == nil || p.Contenido == nil {
		return
	}
	hasta := min(desde+paginasLote-1, p.Contenido.Total)
	var faltan []int
	for n := desde; n <= hasta; n++ {
		if p.imagenes[n] == nil && !p.fallidas[n] && !p.pidiendo[n] {
			faltan = append(faltan, n)
		}
	}
	if len(faltan) == 0 {
		return
	}
	if p.escala == 0 {
		p.escala = p.escalaDeDibujo()
	}
	escala := p.escala
	for _, n := range faltan {
		p.pidiendo[n] = true
	}
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		imgs, err := doc.Imagenes(ctx, faltan[0], faltan[len(faltan)-1], escala)
		fyne.Do(func() {
			defer p.avisarRed()
			if p.doc != doc || p.escala != escala {
				return
			}
			for _, n := range faltan {
				delete(p.pidiendo, n)
			}
			if err != nil {
				p.Error = p.mensajeDeError(err, "pdf.error.render")
				p.Recomponer()
				return
			}
			for _, n := range faltan {
				img, ok := imgs[n]
				if !ok {
					p.fallidas[n] = true
				}
				p.ponerImagen(n, img)
			}
		})
	}()
}

// cargarTexto es `cargarTexto`: el texto de todas las páginas, una vez por documento.
// `listo` recibe nil si no se pudo.
func (p *Pantalla) cargarTexto(listo func([][]Fragmento)) {
	if p.textos != nil {
		listo(p.textos)
		return
	}
	doc := p.doc
	if doc == nil {
		listo(nil)
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		t, err := doc.Texto(ctx)
		fyne.Do(func() {
			defer p.avisarRed()
			if p.doc != doc {
				return
			}
			if err != nil {
				listo(nil)
				return
			}
			p.textos = t
			listo(t)
		})
	}()
}

// preguntarSiInvalidaFirmas atiende el 409 de un PDF firmado que no se puede
// anexar: se pregunta, como en la web, y solo con un sí se repite permitiendo
// que las firmas se pierdan. Dice si se encargó.
func (p *Pantalla) preguntarSiInvalidaFirmas(err error, c api.PDFCambio, mensaje string, hecho func(bool)) bool {
	var e *api.Error
	if !errors.As(err, &e) || e.Code() != "pdf_invalidates_signatures" || c.Campos["invalidar_firmas"] != "" {
		return false
	}
	p.Recomponer()
	p.confirmar(p.t("pdf.signatures.invalidate.confirm"), p.t("pdf.signatures.invalidate.apply"), func() {
		otra := c
		otra.Campos = maps.Clone(c.Campos)
		if otra.Campos == nil {
			otra.Campos = map[string]string{}
		}
		otra.Campos["invalidar_firmas"] = "1"
		p.Aplicar(otra, mensaje, hecho)
	})
	return true
}

// Aplicar cambia el documento (`aplicar` de la web) y dice si llegó a aplicarse: quien
// pierda algo al hacerlo —las marcas de censura— necesita saberlo para no tirarlo tras
// un fallo.
func (p *Pantalla) Aplicar(c api.PDFCambio, mensaje string, hecho func(bool)) {
	doc := p.doc
	if doc == nil {
		if hecho != nil {
			hecho(false)
		}
		return
	}
	p.Trabajando, p.Error = true, ""
	p.Recomponer()
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		r, err := doc.Aplicar(ctx, c)
		fyne.Do(func() {
			defer p.avisarRed()
			if p.doc != doc {
				return
			}
			p.Trabajando = false
			if err != nil && p.preguntarSiInvalidaFirmas(err, c, mensaje, hecho) {
				return
			}
			if err != nil {
				p.Error = p.mensajeDeError(err, "pdf.error.apply")
				p.Recomponer()
				if hecho != nil {
					hecho(false)
				}
				return
			}
			p.Aviso = p.avisoDe(r, mensaje)
			// La copia se guarda con otro nombre: sin su ruta, el aviso diría que existe
			// y no habría forma de llegar a ella.
			p.Copia = ""
			if r.Copia {
				p.Copia = doc.RutaDe(r.Fichero)
				// Firmar con una petición pendiente de ESTE PDF la entrega sola, como en la web.
				if (c.Op == "firmar" || c.Op == "firmar-certificado") && p.peticion != nil {
					p.entregarFirma(*p.peticion, p.Copia)
				}
				p.Recomponer()
			} else {
				// El fichero cambió: se vuelve a leer todo. Reusar lo dibujado enseñaría
				// el documento ANTERIOR con el panel del nuevo.
				p.textos = nil
				p.cargar(p.claveUsada)
			}
			if hecho != nil {
				hecho(true)
			}
		})
	}()
}

// avisoDe es el texto de la franja verde tras aplicar. Reconocer cuenta lo que hizo, y
// hay que decirlo: de veinte páginas se pueden reconocer doce y saltar ocho.
func (p *Pantalla) avisoDe(r api.PDFAplicado, mensaje string) string {
	deCopia := mensaje
	if r.Copia {
		deCopia = p.tv("pdf.saved.copy", i18n.Vars{"name": r.Fichero.Name})
	}
	var partes []string
	if h := r.Cabecera.Get("X-Sytue-Ocr-Paginas"); h != "" {
		// Como número y no como cadena: el plural del catálogo sale de `count`.
		n, _ := strconv.Atoi(h)
		partes = append(partes, p.tv("pdf.saved.ocr", i18n.Vars{"count": n}))
		if s, _ := strconv.Atoi(r.Cabecera.Get("X-Sytue-Ocr-Sin-Imagen")); s > 0 {
			partes = append(partes, p.tv("pdf.saved.ocr.skipped", i18n.Vars{"count": s}))
		}
	}
	// Un PDF/A que deja de serlo, o que baja de nivel, se dice (como en la web).
	switch r.Cabecera.Get("X-Sytue-PDFA") {
	case "perdido":
		partes = append(partes, p.tv("pdf.saved.pdfa.lost", nil))
	case "rebajado":
		partes = append(partes, p.tv("pdf.saved.pdfa.downgraded", nil))
	}
	return strings.Join(append(partes, deCopia), " · ")
}

// erroresPDF son los códigos que se traducen a lo que le pasa a quien está delante
// (`ERRORES` de `errores.ts`). Sin el mapa, la pantalla enseñaría «download_failed».
var erroresPDF = map[string]string{
	"download_failed":          "pdf.error.download",
	"upload_failed":            "pdf.error.upload",
	"too_large":                "pdf.error.tooLarge",
	"docs_unreachable":         "pdf.error.offline",
	"unauthenticated":          "pdf.error.session",
	"ocr_not_configured":       "pdf.error.ocr.unavailable",
	"ocr_busy":                 "pdf.error.ocr.busy",
	"ocr_language_unavailable": "pdf.error.ocr.language",
	"ocr_failed":               "pdf.error.ocr.failed",
	// «Pedir firma» (ADR 0051 §7).
	"pdf_signature_recipient_not_found":  "pdf.error.signatureRequest.recipient",
	"pdf_signature_request_self":         "pdf.error.signatureRequest.self",
	"pdf_signature_request_exists":       "pdf.error.signatureRequest.exists",
	"pdf_signature_request_not_signed":   "pdf.error.signatureRequest.notSigned",
	"pdf_signature_request_not_found":    "pdf.error.signatureRequest.gone",
	"pdf_signature_requests_unavailable": "pdf.error.signatureRequest.unavailable",
	"pdf_signature_request_too_many":     "pdf.error.signatureRequest.tooMany",
	// A alguien de otra organización (ADR 0057).
	"pdf_signature_share_blocked":          "pdf.error.signatureRequest.shareBlocked",
	"pdf_signature_request_link_gone":      "pdf.error.signatureRequest.linkGone",
	"pdf_signature_request_recipient_busy": "pdf.error.signatureRequest.recipientBusy",
	"pdf_signature_request_changed":        "pdf.error.signatureRequest.changed",
	"pdf_signature_request_busy":           "pdf.error.signatureRequest.busy",
	"pdf_signature_org_closed":             "pdf.error.signatureRequest.orgClosed",
	"pdf_signature_blocks_full":            "pdf.error.signatureRequest.blocksFull",
	"pdf_unknown_letters":                  "pdf.error.unknownLetters",
	// Lo que el motor no sabe abrir: antes todo era `bad_request` con una frase en inglés.
	"pdf_not_pdf":                "pdf.error.notPdf",
	"pdf_unreadable":             "pdf.error.unreadable",
	"pdf_encryption_unsupported": "pdf.error.encryption",
	"pdf_unsupported":            "pdf.error.unsupported",
	"payload_too_large":          "pdf.error.tooLarge",
	// Reconocer, firmar y archivar: el motor los rechaza con su código propio.
	"pdf_already_has_text":     "pdf.error.ocr.alreadyText",
	"pdf_ocr_empty":            "pdf.error.ocr.empty",
	"pdf_bad_certificate":      "pdf.error.certificate",
	"pdf_certify_signed":       "pdf.error.certifySigned",
	"pdf_signature_too_large":  "pdf.error.signatureTooLarge",
	"pdf_archive_not_possible": "pdf.error.archive",
}

// mensajeDeError es `mensajeDeError`: lo que no llega a ser una respuesta es la RED, no
// el documento —decir «no se pudo abrir» de un fichero que está bien manda a buscar donde
// no es—; un código conocido se traduce; y lo demás dice lo que se intentaba. La frase del
// motor NO se enseña: está en inglés, y en una interfaz en otro idioma no la entiende
// quien la lee.
//
// Aquí no hay BFF que ponga `download_failed` o `unauthenticated`, así que los estados
// HTTP equivalentes se traducen igual: un 401 es la sesión y un 413, el tamaño.
func (p *Pantalla) mensajeDeError(err error, generico string) string {
	if errors.Is(err, api.ErrPDFGrande) {
		return p.t("pdf.error.tooLarge")
	}
	var e *api.Error
	if !errors.As(err, &e) {
		return p.t("pdf.error.offline")
	}
	if k, ok := erroresPDF[e.Code()]; ok {
		return p.t(k)
	}
	switch e.Status {
	case http.StatusUnauthorized:
		return p.t("pdf.error.session")
	case http.StatusRequestEntityTooLarge:
		return p.t("pdf.error.tooLarge")
	}
	return p.t(generico)
}

// estadoDeError es cuál de los estados sin documento toca: sin conexión, sin sesión o
// un error del documento.
func (p *Pantalla) estadoDeError(err error) editor.Estado {
	var e *api.Error
	switch {
	case errors.Is(err, api.ErrPDFGrande):
		return editor.Error
	case !errors.As(err, &e):
		return editor.SinConexion
	case e.Status == http.StatusUnauthorized:
		return editor.SinSesion
	case e.Status == http.StatusForbidden:
		return editor.SinPermiso
	}
	return editor.Error
}

// leer es «Leer en voz alta» (`leerEnVozAlta` de la web): desde la página que se mira
// hasta el final —empezar por la 1 obligaría a escuchar lo ya leído—, con la voz del
// sistema en el idioma de la interfaz (una voz inglesa leyendo español no se entiende).
// Pulsado otra vez, calla. Sin voz en el sistema lo DICE (`pdf.speak.unsupported`), y
// un escaneo sin OCR, que no hay texto (`pdf.speak.noText`): callarse se leería como
// que el botón está roto.
func (p *Pantalla) leer() {
	if p.callar != nil {
		p.Callar()
		return
	}
	p.barra.Leyendo = true
	p.Recomponer()
	desde := p.PaginaActual
	p.cargarTexto(func(paginas [][]Fragmento) {
		var trozos []string
		for _, frags := range paginas[min(max(desde-1, 0), len(paginas)):] {
			if t := TextoParaLeer(frags); t != "" {
				trozos = append(trozos, t)
			}
		}
		if len(trozos) == 0 {
			p.barra.Leyendo, p.Aviso, p.Error = false, p.t("pdf.speak.noText"), ""
			p.Recomponer()
			return
		}
		p.lectura++
		esta := p.lectura
		callar, err := hablar(strings.Join(trozos, ". "), p.Idioma, func() {
			fyne.Do(func() {
				// Solo si es ESTA lectura la que acaba: callar y volver a pulsar deja
				// otra en marcha que no se puede dar por terminada.
				if p.callar != nil && p.lectura == esta {
					p.callar, p.barra.Leyendo = nil, false
					p.Recomponer()
				}
			})
		})
		if err != nil {
			p.barra.Leyendo, p.Aviso, p.Error = false, p.t("pdf.speak.unsupported"), ""
			p.Recomponer()
			return
		}
		p.callar = callar
	})
}

// hablar es la voz del sistema; las pruebas la cambian por una que no suena.
var hablar = voz.Leer

// Callar para la lectura en voz alta, si la hay. Al cambiar de documento o cerrar la
// ventana TIENE que callarse: la voz del sistema seguiría leyendo sobre otra cosa.
func (p *Pantalla) Callar() {
	if p.callar != nil {
		p.callar()
	}
	p.callar, p.barra.Leyendo = nil, false
	p.Recomponer()
}

// Cargar deja la pantalla en «cargando» mientras llega un documento nuevo.
func (p *Pantalla) Cargar() {
	p.doc, p.Contenido, p.visor, p.hojas, p.textos = nil, nil, nil, nil, nil
	p.PideClave, p.Aviso, p.Error, p.Copia = false, "", "", ""
	p.Estado = editor.Cargando
	p.Recomponer()
}

// NoEsta dice si un fallo es que el fichero no existe (404): la página «no encontrada»,
// y no un error que reintentar.
func NoEsta(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

func (p *Pantalla) avisarRed() {
	if p.trasRed != nil {
		p.trasRed()
	}
}

// MostrarPortada deja la pantalla sin documento, con la portada, y trae los recientes de
// la carpeta «PDF» (`pdf-recientes.ts`: los 48 más recientes).
func (p *Pantalla) MostrarPortada(pdf *api.PDF, loc i18n.Locale) {
	if p.Portada == nil {
		p.Portada = NuevaPortada(p.tv, loc)
	}
	po := p.Portada
	po.AlCrear, po.AlElegir, po.AlEscanear = p.AlCrear, p.AlElegir, p.AlEscanear
	po.AlAbrir = func(f api.File) {
		if p.AlAbrirRuta != nil {
			p.AlAbrirRuta("/" + api.PDFCarpeta + "/" + f.Name)
		}
	}
	po.AlPeticion = func(x api.PDFPeticionFirma, recibida bool) { p.abrirPeticion(pdf, x, recibida) }
	po.AlDesbloquear = func(sub string) { p.desbloquear(pdf, sub) }
	p.doc, p.Contenido, p.visor, p.hojas = nil, nil, nil, nil
	p.Estado = editor.Vacio
	po.Lista, po.Fallo, po.Peticiones = nil, false, nil
	p.Recomponer()
	if pdf == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		ps, err := pdf.PeticionesFirma(ctx)
		if err != nil {
			return // sin la lista no hay sección, y ya
		}
		fyne.Do(func() { po.Peticiones = &ps; po.Recomponer() })
	}()
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		lista, err := pdf.Lista(ctx)
		fyne.Do(func() {
			defer p.avisarRed()
			if err != nil {
				po.Fallo = true
			} else {
				po.Lista = append([]api.File{}, lista[:min(len(lista), MaxRecientes)]...)
			}
			po.Recomponer()
		})
	}()
}

// desbloquear quita el bloqueo desde la portada y la vuelve a pintar sin esa persona.
func (p *Pantalla) desbloquear(pdf *api.PDF, sub string) {
	po := p.Portada
	if pdf == nil || po == nil {
		return
	}
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		err := pdf.DesbloquearFirma(ctx, sub)
		fyne.Do(func() {
			if err != nil {
				p.EstadoAbrir(false, p.mensajeDeError(err, "pdf.error.apply"))
				return
			}
			if po.Peticiones != nil {
				bs := po.Peticiones.Bloqueados[:0:0]
				for _, b := range po.Peticiones.Bloqueados {
					if b.Sub != sub {
						bs = append(bs, b)
					}
				}
				po.Peticiones.Bloqueados = bs
			}
			po.Recomponer()
		})
	}()
}

// abrirPeticion es pulsar una fila de las peticiones de la portada. Lo que está en mi
// unidad se abre por su ruta; lo de otra organización (ADR 0057) primero se trae —el PDF
// que me piden firmar, o la copia firmada que pedí— y después se abre.
func (p *Pantalla) abrirPeticion(pdf *api.PDF, x api.PDFPeticionFirma, recibida bool) {
	abrir := func(ruta string) {
		if ruta != "" && p.AlAbrirRuta != nil {
			p.AlAbrirRuta(ruta)
		}
	}
	var traer func(context.Context, string) (api.PDFPeticionFirma, error)
	switch {
	case recibida && x.Externo && x.Ruta == "":
		traer = pdf.AbrirFirma
	case !recibida && x.Externo && x.Estado == api.PDFFirmaFirmada && x.RutaFirmada == "":
		traer = pdf.TraerFirmado
	case recibida:
		abrir(x.Ruta)
		return
	default:
		abrir(cmp.Or(x.RutaFirmada, x.Ruta))
		return
	}
	po := p.Portada
	if pdf == nil || po == nil || po.Abriendo {
		return
	}
	// Un paso a la vez: dos «abrir» seguidos dejarían dos copias en la unidad.
	p.EstadoAbrir(true, "")
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazo)
		defer parar()
		y, err := traer(ctx, x.ID)
		fyne.Do(func() {
			if err != nil {
				p.EstadoAbrir(false, p.mensajeDeError(err, "pdf.error.apply"))
				return
			}
			p.EstadoAbrir(false, "")
			abrir(cmp.Or(y.RutaFirmada, y.Ruta))
		})
	}()
}

// ArrastrarEncima enciende la zona de abrir de la portada mientras haya un fichero
// arrastrado sobre la ventana (ver [Portada.PonerEncima]). Con un documento abierto no hay
// zona que encender.
func (p *Pantalla) ArrastrarEncima(encima bool) {
	if p.Portada != nil {
		p.Portada.PonerEncima(encima && p.Estado == editor.Vacio)
	}
}

// EstadoAbrir dice cómo va abrir un fichero del disco: en la portada, «Abriendo…» y la
// frase del fallo en su zona de abrir; con un documento abierto, la rueda de la cinta y
// la franja roja.
func (p *Pantalla) EstadoAbrir(abriendo bool, fallo string) {
	if p.Estado == editor.Vacio && p.Portada != nil {
		p.Portada.Abriendo, p.Portada.ErrorAbrir = abriendo, fallo
		p.Portada.Recomponer()
		return
	}
	p.estadoCinta(abriendo, fallo)
}

// EstadoCrear es EstadoAbrir para «En blanco»: «Creando…» en su tarjeta y
// `create.error.description` debajo.
func (p *Pantalla) EstadoCrear(creando, fallo bool) {
	if p.Estado == editor.Vacio && p.Portada != nil {
		p.Portada.Creando, p.Portada.ErrorCrear = creando, fallo
		p.Portada.Recomponer()
		return
	}
	frase := ""
	if fallo {
		frase = p.t("create.error.description")
	}
	p.estadoCinta(creando, frase)
}

func (p *Pantalla) estadoCinta(trabajando bool, fallo string) {
	p.Trabajando = trabajando
	if fallo != "" {
		p.Error = fallo
	}
	p.Recomponer()
}
