// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"bytes"
	"context"
	"image/jpeg"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

/*
El ASISTENTE (`Asistente` de `Paneles.tsx`): preguntas sobre ESTE documento, que nunca
lo modifican. El contexto es la capa de texto con el número de página delante —lo que
deja citarla—; en un escaneo no hay capa, y primero hay que LEERLO con visión, una
consulta de pago por página, avisándolo antes.
*/

// Lo que se le pide al modelo, literal de la web: son instrucciones, no textos de la
// interfaz, y van en el idioma en que se probaron.
const (
	instruccionOCR = "Transcribe literalmente el texto de esta página escaneada, respetando el orden de lectura y los saltos de párrafo. " +
		"No resumas, no corrijas y no completes lo que no se lea: marca lo ilegible con […]. " +
		"Si la página no tiene texto, responde con una cadena vacía. Devuelve solo la transcripción."
	instruccionResumen  = "Resume este documento en un párrafo, en su mismo idioma. Di lo que dice, no de qué trata. Conserva los datos concretos (partes, fechas, importes, plazos). Devuelve solo el resumen."
	instruccionClaves   = "Lista los puntos clave de este documento en como mucho ocho puntos, en su mismo idioma: partes, obligaciones, importes, fechas y plazos. Empieza cada punto con «- » y cita la página entre paréntesis. Devuelve solo la lista."
	instruccionPregunta = "Responde a la pregunta usando SOLO este documento, en el idioma de la pregunta. Cita la página entre paréntesis. Si el documento no lo dice, responde exactamente que el documento no lo dice: no lo deduzcas ni lo completes con lo que sueles saber."
)

const (
	// maxContextoPDF es el tope del texto que se manda: un PDF largo no cabe, y
	// cortarlo aquí es honesto.
	maxContextoPDF = 60000
	// maxPaginasOCR es el tope de páginas que se leen de una vez (`MAX_PAGINAS`).
	maxPaginasOCR = 12
	// plazoIA es el del modo calidad: el servicio le da 120 s.
	plazoIA = 130 * time.Second
)

// paginasALeer es `paginasALeer` de `ocr.ts`: desde la página que se mira hacia delante
// y, si no se llega al tope, hacia atrás. Empezar siempre por la primera obligaría a
// pagar doce páginas de portada para preguntar por la cuarenta.
func paginasALeer(total, actual, tope int) []int {
	if total <= 0 {
		return nil
	}
	desde := min(max(1, actual), total)
	var r []int
	for p := desde; p <= total && len(r) < tope; p++ {
		r = append(r, p)
	}
	for p := desde - 1; p >= 1 && len(r) < tope; p-- {
		r = append([]int{p}, r...)
	}
	return r
}

// paginaLeida es lo transcrito de una página; vacío es que no tenía nada legible.
type paginaLeida struct {
	pagina int
	texto  string
}

// juntar es `juntar`: lo leído con la marca de página delante —que es lo que deja
// citar—, sin las páginas vacías, que invitarían al modelo a rellenar el hueco.
func juntar(ps []paginaLeida) string {
	ps = slices.Clone(ps)
	slices.SortFunc(ps, func(a, b paginaLeida) int { return a.pagina - b.pagina })
	var partes []string
	for _, p := range ps {
		if t := strings.TrimSpace(p.texto); t != "" {
			partes = append(partes, "[página "+strconv.Itoa(p.pagina)+"]\n"+t)
		}
	}
	return strings.Join(partes, "\n\n")
}

// esLegible es `esLegible`: cuatro caracteres sueltos no son un documento.
func esLegible(texto string) bool {
	return utf8.RuneCountInString(strings.Join(strings.Fields(texto), "")) >= 40
}

// contextoDe es el texto de las páginas con su número delante, hasta el tope.
func contextoDe(paginas [][]Fragmento) string {
	var s strings.Builder
	for i, frags := range paginas {
		trozos := make([]string, len(frags))
		for j, f := range frags {
			trozos[j] = f.Texto
		}
		texto := strings.TrimSpace(strings.Join(trozos, " "))
		if texto == "" {
			continue
		}
		s.WriteString("\n\n[página " + strconv.Itoa(i+1) + "]\n" + texto)
		if s.Len() > maxContextoPDF {
			break
		}
	}
	return recortar(s.String(), maxContextoPDF)
}

// recortar deja el texto en `n` bytes como mucho sin partir una letra.
func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// panelAsistente es el estado del panel: sobrevive a cambiar de pestaña.
type panelAsistente struct {
	l        *PanelLateral
	pregunta *ui.Entrada
	// leido es lo sacado del escaneo; vacío es que aún no se ha leído.
	leido     string
	leyendo   int
	respuesta []ia.Bloque
	error     string
	ocupado   bool
	vista     *fyne.Container
}

func (l *PanelLateral) asistente() fyne.CanvasObject {
	if l.asistenteP == nil {
		a := &panelAsistente{l: l, vista: container.New(ui.Columna{})}
		a.pregunta = ui.NuevaEntrada(l.t("pdf.ai.askPlaceholder", nil))
		a.pregunta.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
				a.preguntarLoEscrito()
				return true
			}
			return false
		}
		l.asistenteP = a
	}
	l.asistenteP.rehacer()
	return l.asistenteP.vista
}

func (a *panelAsistente) t(clave string) string { return a.l.t(clave, nil) }

func (a *panelAsistente) rehacer() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := anchoUtil()
	partes := []fyne.CanvasObject{ui.NuevoParrafo(a.t("pdf.ai.hint"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho)}
	if a.l.sinCapaDeTexto() {
		partes = append(partes, a.cajaEscaneo(ancho))
	}
	resumir := ui.NuevoBoton(a.t("pdf.ai.summarize"), func() { a.preguntar(instruccionResumen, "resumir") }).Con(ui.Secundario, ui.TallaSM)
	claves := ui.NuevoBoton(a.t("pdf.ai.key"), func() { a.preguntar(instruccionClaves, "claves") }).Con(ui.Secundario, ui.TallaSM)
	resumir.Inerte, claves.Inerte = a.ocupado, a.ocupado
	preguntar := ui.NuevoBoton(a.t("pdf.ai.ask"), a.preguntarLoEscrito).Con(ui.Primario, ui.TallaSM)
	preguntar.Inerte = a.ocupado || strings.TrimSpace(a.pregunta.Texto()) == ""
	a.pregunta.AlCambiar(func(s string) {
		if inerte := a.ocupado || strings.TrimSpace(s) == ""; preguntar.Inerte != inerte {
			preguntar.Inerte = inerte
			preguntar.Refresh()
		}
	})
	a.pregunta.Ancho = ancho - tema.Espacio(1) - preguntar.MinSize().Width
	a.pregunta.Inerte(a.ocupado)
	// `flex gap-1`: el campo y el botón arriba, cada uno a su alto.
	partes = append(partes,
		container.New(ui.Fila{Hueco: tema.Espacio(1)}, resumir, claves),
		container.New(ui.Fila{Hueco: tema.Espacio(1)}, a.pregunta, container.New(alPrincipio{}, preguntar)))
	if a.ocupado {
		// `flex items-center gap-2 py-3`.
		fila := container.New(ui.Fila{Hueco: tema.Espacio(2)}, ui.NuevaRueda(16, "text-muted", v),
			container.New(ui.Margen{Arr: (16 - tema.TextoXS.Linea) / 2}, ui.NuevoRotulo(a.t("pdf.ai.working"), tema.TextoXS, ui.Normal, "text-muted")))
		partes = append(partes, container.New(ui.Margen{Arr: tema.Espacio(3), Abj: tema.Espacio(3)}, fila))
	}
	if a.error != "" {
		partes = append(partes, ui.NuevoParrafo(a.error, tema.TextoXS, ui.Normal, "danger-text", v).A(ancho))
	}
	if a.respuesta != nil {
		partes = append(partes, a.vistaRespuesta(ancho))
	}
	a.vista.Objects = []fyne.CanvasObject{container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...)}
	a.vista.Refresh()
}

// cajaEscaneo es el `border bg-bg-subtle rounded-md p-2` del escaneo: lo que cuesta
// leerlo, dicho ANTES, y el botón; o, leído, que ya se puede preguntar.
func (a *panelAsistente) cajaEscaneo(ancho float32) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := tema.Espacio(2)
	dentro := ancho - 2*tema.Borde - 2*p
	lineas := []fyne.CanvasObject{
		ui.NuevoParrafo(a.l.t("pdf.ai.readHint", i18n.Vars{"n": maxPaginasOCR}), tema.TextoXS, ui.Normal, "text-muted", v).A(dentro),
	}
	if a.leido == "" {
		rotulo := a.t("pdf.ai.read")
		if a.leyendo > 0 {
			rotulo = a.l.t("pdf.ai.reading", i18n.Vars{"n": a.leyendo})
		}
		leer := ui.NuevoBoton(rotulo, a.leerEscaneo).Con(ui.Primario, ui.TallaSM)
		leer.Ancho, leer.Inerte = dentro, a.ocupado
		lineas = append(lineas, leer)
	} else {
		lineas = append(lineas, ui.NuevoParrafo(a.t("pdf.ai.readDone"), tema.TextoXS, ui.Normal, "success-text", v).A(dentro))
	}
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
	cuerpo := container.New(ui.Margen{Izq: tema.Borde + p, Arr: tema.Borde + p, Der: tema.Borde + p, Abj: tema.Borde + p},
		container.New(ui.Columna{Hueco: tema.Espacio(2)}, lineas...))
	return container.New(conCaja{caja}, append(caja.Objetos(), cuerpo)...)
}

// vistaRespuesta es `flex flex-col gap-1.5 text-xs`: un párrafo por bloque —los títulos
// en seminegrita, los puntos con su viñeta— y «Copiar» debajo.
func (a *panelAsistente) vistaRespuesta(ancho float32) fyne.CanvasObject {
	filas := make([]fyne.CanvasObject, 0, len(a.respuesta)+1)
	for _, b := range a.respuesta {
		switch b.Tipo {
		case ia.Punto:
			vineta := ui.NuevoRotulo("•", tema.TextoXS, ui.Normal, "text")
			hueco := tema.Espacio(1.5)
			resto := ancho - vineta.MinSize().Width - hueco
			filas = append(filas, container.New(ui.Fila{Hueco: hueco}, vineta, ui.ParrafoRico(b.Trozos, tema.Regular, tema.TextoXS, resto)))
		case ia.Parrafo:
			filas = append(filas, ui.ParrafoRico(b.Trozos, tema.Regular, tema.TextoXS, ancho))
		case ia.H1, ia.H2, ia.H3:
			filas = append(filas, ui.ParrafoRico(b.Trozos, tema.SemiNegra, tema.TextoXS, ancho))
		}
	}
	copiar := ui.NuevoBoton(a.t("pdf.ai.copy"), func() {
		fyne.CurrentApp().Clipboard().SetContent(ia.TextoPlano(a.respuesta))
	}).Con(ui.Fantasma, ui.TallaSM)
	filas = append(filas, container.New(alPrincipio{}, copiar))
	return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, filas...)
}

func (a *panelAsistente) preguntarLoEscrito() {
	if q := strings.TrimSpace(a.pregunta.Texto()); q != "" {
		a.preguntar(instruccionPregunta+"\n\nPregunta: "+q, "preguntar")
	}
}

// preguntar manda la instrucción con el documento como contexto: lo leído del escaneo,
// o la capa de texto con sus páginas.
func (a *panelAsistente) preguntar(instruccion, accion string) {
	if a.ocupado {
		return
	}
	a.ocupado, a.error, a.respuesta = true, "", nil
	a.rehacer()
	if a.l.sinCapaDeTexto() {
		if a.leido == "" {
			a.terminar("pdf.ai.readFirst", nil)
			return
		}
		a.enviar(recortar(a.leido, maxContextoPDF), instruccion, accion)
		return
	}
	p := a.l.pant
	if p == nil {
		a.terminar("pdf.ai.failed", nil)
		return
	}
	p.cargarTexto(func(paginas [][]Fragmento) {
		if paginas == nil {
			a.terminar("pdf.ai.failed", nil)
			return
		}
		contexto := contextoDe(paginas)
		if strings.TrimSpace(contexto) == "" {
			a.terminar("pdf.ai.noText", nil)
			return
		}
		a.enviar(contexto, instruccion, accion)
	})
}

// terminar cierra una consulta con su error (clave del catálogo) o su respuesta.
func (a *panelAsistente) terminar(clave string, respuesta []ia.Bloque) {
	a.ocupado, a.leyendo = false, 0
	if clave != "" {
		a.error = a.t(clave)
	}
	a.respuesta = respuesta
	a.rehacer()
}

// enviar es la consulta en sí, en el modo calidad, que es el de la web.
func (a *panelAsistente) enviar(contexto, instruccion, accion string) {
	p := a.l.pant
	if p == nil || p.IA == nil {
		a.terminar("pdf.ai.failed", nil)
		return
	}
	cliente := p.IA
	go func() {
		ctx, parar := context.WithTimeout(p.ctx, plazoIA)
		defer parar()
		texto, err := cliente.Asistir(ctx, api.PeticionIA{
			Accion: accion, Instruccion: instruccion, Contexto: contexto, Tipo: "pdf", Modo: "calidad",
		})
		texto = strings.TrimSpace(texto)
		fyne.Do(func() {
			defer p.avisarRed()
			if err != nil || texto == "" {
				a.terminar("pdf.ai.failed", nil)
				return
			}
			a.terminar("", ia.BloquesDe(texto))
		})
	}()
}

// leerEscaneo transcribe las páginas del escaneo con visión, UNA CONSULTA POR PÁGINA y
// en serie —doce a la vez son un 429 que las pierde todas—. Como en la web, solo las que
// ya se han dibujado; cada una se pide al motor a escala 2 y va en JPEG, que para una
// fotografía pesa varias veces menos que el PNG sin leerse peor.
func (a *panelAsistente) leerEscaneo() {
	p := a.l.pant
	if a.ocupado || p == nil || p.doc == nil || p.IA == nil {
		return
	}
	a.ocupado, a.error, a.respuesta = true, "", nil
	var paginas []int
	for _, n := range paginasALeer(a.l.c.Total, a.l.PaginaActual, maxPaginasOCR) {
		if p.imagenes[n] != nil {
			paginas = append(paginas, n)
		}
	}
	a.rehacer()
	doc, cliente := p.doc, p.IA
	go func() {
		leidas := make([]paginaLeida, 0, len(paginas))
		for i, n := range paginas {
			pct := int(math.Round(float64(i+1) / float64(len(paginas)) * 100))
			fyne.Do(func() { a.leyendo = pct; a.rehacer() })
			leidas = append(leidas, paginaLeida{pagina: n, texto: transcribir(p.ctx, doc, cliente, n)})
		}
		junto := juntar(leidas)
		fyne.Do(func() {
			defer p.avisarRed()
			if !esLegible(junto) {
				a.terminar("pdf.ai.unreadable", nil)
				return
			}
			a.leido = junto
			a.terminar("", nil)
		})
	}()
}

// transcribir es la consulta de visión de UNA página. Lo que falle deja la página
// vacía, que es lo que hace la web con una respuesta que no llega.
func transcribir(ctx context.Context, doc *Documento, cliente *api.IA, n int) string {
	ctx, parar := context.WithTimeout(ctx, plazoIA)
	defer parar()
	imgs, err := doc.Imagenes(ctx, n, n, 2)
	if err != nil || imgs[n] == nil {
		return ""
	}
	var buf bytes.Buffer
	if jpeg.Encode(&buf, imgs[n], &jpeg.Options{Quality: 92}) != nil {
		return ""
	}
	texto, _ := cliente.Asistir(ctx, api.PeticionIA{
		Accion: "transcribir", Instruccion: instruccionOCR, Tipo: "pdf", Modo: "calidad",
		Imagen: buf.Bytes(), ImagenTipo: "image/jpeg",
	})
	return texto
}

// ---- piezas ----

// conCaja coloca los objetos de una [ui.Caja] a todo el tamaño y el contenido encima.
type conCaja struct{ caja *ui.Caja }

func (c conCaja) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[len(objs)-1].MinSize() }

func (c conCaja) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c.caja.Redimensionar(t)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[len(objs)-1].Move(fyne.NewPos(0, 0))
	objs[len(objs)-1].Resize(t)
}

// fijo declara un tamaño y deja a los hijos donde se colocaron al construir.
type fijo struct{ tam fyne.Size }

func (f fijo) MinSize([]fyne.CanvasObject) fyne.Size { return f.tam }
func (fijo) Layout([]fyne.CanvasObject, fyne.Size)   {}
