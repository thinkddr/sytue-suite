// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// El panel «Asistente» del carril (`panels/AiPanel.tsx` con `editor/ai.ts`) para un
// documento de texto: acciones sobre la selección o, sin ella, sobre el documento entero;
// reescribir, traducir y una petición libre. La respuesta se enseña y se inserta, se pone
// en lugar de la selección, se copia o se descarta. La inferencia es de Sytue AI
// (`api.IA.Asistir`), con el token de la persona.

// accionIA es una acción del panel con su rótulo.
type accionIA struct{ ID, Clave string }

// Las de QUICK y REWRITE, en su orden, y los idiomas de LANGS.
var (
	accionesRapidasIA = []accionIA{
		{"resumir", "ai.action.summarise"},
		{"mejorar", "ai.action.improve"},
		{"corregir", "ai.action.proofread"},
		{"explicar", "ai.action.explain"},
		{"continuar", "ai.action.continue"},
	}
	reescrituraIA = []accionIA{{"reescribir-claro", "ai.rewrite.clearer"}, {"reescribir-formal", "ai.rewrite.formal"}, {"reescribir-corto", "ai.rewrite.shorter"}}
	idiomasIA     = []string{"ai.lang.en", "ai.lang.fr", "ai.lang.de", "ai.lang.it", "ai.lang.pt", "ai.lang.ca", "ai.lang.gl", "ai.lang.eu", "ai.lang.es"}
)

// directivasIA son las INSTRUCTIONS de ai.ts, en español, tal cual.
var directivasIA = map[string]string{
	"resumir":           "Resume el texto de forma concisa, conservando las ideas principales.",
	"reescribir-claro":  "Reescribe el texto para que sea más claro y fácil de leer, sin cambiar su significado.",
	"reescribir-formal": "Reescribe el texto en un tono más formal y profesional.",
	"reescribir-corto":  "Reescribe el texto de forma más breve, eliminando lo superfluo.",
	"mejorar":           "Mejora la redacción del texto (gramática, estilo y fluidez), conservando su significado.",
	"corregir":          "Corrige la ortografía, la gramática y la puntuación del texto. Devuelve solo el texto corregido.",
	"explicar":          "Explica de forma sencilla el significado del texto.",
	"continuar":         "Continúa el texto de forma coherente con su estilo y contenido.",
}

// MaxContextoIA es MAX_CONTEXT_CHARS: lo que se manda, como mucho, en letras.
const MaxContextoIA = 12000

// PlazoIA es lo que se espera al asistente: el servicio da 60 s al modelo rápido.
const PlazoIA = 70 * time.Second

// PeticionIA es `buildAiRequest`: la directiva de la acción (o la petición libre en
// «chat», o el idioma en «traducir») y el contexto recortado.
func PeticionIA(accion, peticion, contexto string) api.PeticionIA {
	return PeticionIAde("document", accion, peticion, contexto)
}

// PeticionIAde es PeticionIA para un tipo de documento (`kind`: document, spreadsheet o
// presentation).
func PeticionIAde(tipo, accion, peticion, contexto string) api.PeticionIA {
	peticion = strings.TrimSpace(peticion)
	r := []rune(strings.TrimSpace(contexto))
	instruccion := directivasIA[accion]
	switch accion {
	case "chat":
		instruccion = peticion
	case "traducir":
		destino := peticion
		if destino == "" {
			destino = "inglés"
		}
		instruccion = "Traduce el texto a " + destino + ". Devuelve solo la traducción."
	}
	return api.PeticionIA{Accion: accion, Tipo: tipo, Instruccion: instruccion, Contexto: string(r[:min(len(r), MaxContextoIA)])}
}

// claveErrorIA es `aiErrorKey`: el mensaje del catálogo para cada fallo.
func claveErrorIA(err error) string {
	var e *api.Error
	switch {
	case api.IANoConfigurada(err):
		return "ai.error.notConfigured"
	case errors.Is(err, context.DeadlineExceeded):
		return "ai.error.timeout"
	case !errors.As(err, &e):
		return "ai.error.unreachable"
	case e.Status == http.StatusUnauthorized:
		return "ai.error.unauthenticated"
	case e.Status == http.StatusBadRequest:
		return "ai.error.badRequest"
	case e.Status == http.StatusGatewayTimeout:
		return "ai.error.timeout"
	}
	return "ai.error.generic"
}

// DestinoIA es sobre qué trabaja el panel «Asistente»: de dónde sale el contexto y qué
// hacen «Insertar» y «Reemplazar». El panel es UNO para los editores, como `AiPanel.tsx`;
// cada lienzo pone su destino. El del folio de Docus es el propio Folio.
type DestinoIA interface {
	// TipoIA es el `kind` de la petición: document, spreadsheet o presentation.
	TipoIA() string
	// ContextoIA es el texto elegido y true, o todo el texto y false.
	ContextoIA() (string, bool)
	// AplicarIA escribe la respuesta detrás de lo elegido o en su lugar, y devuelve la
	// clave del aviso de éxito; "" si no se pudo.
	AplicarIA(respuesta string, reemplazar bool) string
}

// PanelIA es el cuerpo del panel «Asistente» sobre `d`: acciones, reescritura,
// traducción y petición libre, con la respuesta y sus cuatro acciones.
type PanelIA struct {
	Ctx     *editor.Contexto
	Asistir func(context.Context, api.PeticionIA) (string, error)
	Idioma  string
	// Lanzar corre la petición fuera del hilo de la interfaz; las pruebas lo hacen en el
	// sitio.
	Lanzar func(func())
}

type panelAsistente struct {
	cfg PanelIA
	ctx *editor.Contexto
	d   DestinoIA
	// idioma es la clave del de destino; el nombre traducido es lo que viaja.
	idioma    string
	chat      *ui.AreaTexto
	respuesta *fyne.Container
	ocupado   bool
	resultado string
	fallo     string
	// conSeleccion: la última petición partió de una selección, y se puede reemplazar.
	conSeleccion bool
	cancelar     context.CancelFunc
	objeto       fyne.CanvasObject
}

func (a *App) panelAsistente(ab *abierto) fyne.CanvasObject { return a.nuevoAsistente(ab).objeto }

func (a *App) nuevoAsistente(ab *abierto) *panelAsistente {
	return PanelIA{Ctx: ab.ctx, Asistir: a.Asistir, Idioma: a.Idioma, Lanzar: a.lanzar}.nuevo(ab.folio)
}

// Objeto monta el panel sobre `d`.
func (c PanelIA) Objeto(d DestinoIA) fyne.CanvasObject { return c.nuevo(d).objeto }

func (c PanelIA) nuevo(d DestinoIA) *panelAsistente {
	p := &panelAsistente{cfg: c, ctx: c.Ctx, d: d, idioma: idiomasIA[0]}
	t, v := p.ctx.Traducir, tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	seccion := func(clave string, contenido ...fyne.CanvasObject) fyne.CanvasObject {
		rotulo := ui.NuevoVersal(t(clave), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
		return container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, append([]fyne.CanvasObject{rotulo.Objeto()}, contenido...)...)
	}
	chips := func(acciones []accionIA) fyne.CanvasObject {
		out := make([]fyne.CanvasObject, 0, len(acciones))
		for _, ac := range acciones {
			b := editor.NuevoBotonPequeno(t(ac.Clave), func() { p.pedir(ac.ID, "") })
			b.Inerte = p.ctx.SinRed
			out = append(out, b)
		}
		return container.New(&envuelta{hueco: tema.Espacio(1.5), ancho: anchoContenidoIA}, out...)
	}
	ambito := "ai.scope.document"
	if _, sel := d.ContextoIA(); sel {
		ambito = "ai.scope.selection"
	}
	glifo := iconos.TrazoEd(iconos.EdIA, iconos.TallaXS, "text-muted", v)
	opciones := make([]editor.Opcion, 0, len(idiomasIA))
	for _, k := range idiomasIA {
		opciones = append(opciones, editor.Opcion{Valor: k, Rotulo: t(k)})
	}
	idioma := editor.NuevoSelectorCinta(t("ai.lang.label"), anchoSelectorIdioma, opciones, p.idioma, func(k string) { p.idioma = k })
	traducir := editor.NuevoBotonPequeno(t("ai.translate"), func() { p.pedir("traducir", t(p.idioma)) })
	p.chat = ui.NuevaAreaTexto(t("ai.chat.placeholder"))
	p.chat.Filas = 3
	enviar := ui.NuevoBoton(t("ai.chat.send"), func() { p.pedir("chat", p.chat.Texto()) }).Con(ui.Primario, ui.TallaSM)
	p.respuesta = container.New(ui.Columna{Hueco: tema.Espacio(2)})
	p.pintarRespuesta()
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(2)},
		container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, glifo.Objeto(), ui.NuevoRotulo(t(ambito), tema.TextoXS, ui.Normal, "text-muted")),
		seccion("ai.section.actions", chips(accionesRapidasIA)),
		seccion("ai.section.rewrite", chips(reescrituraIA)),
		seccion("ai.section.translate", container.New(ui.Fila{Hueco: tema.Espacio(1.5)}, idioma, traducir)),
		seccion("ai.section.chat", p.chat, container.New(alFinal{}, enviar)),
		separador(v), // `border-t pt-3`
		p.respuesta,
	)
	m := editor.MargenPanel // `p-3`
	p.objeto = container.NewVScroll(container.New(layout.NewCustomPaddedLayout(m, m, m, m), cuerpo))
	return p
}

// contexto es el texto de la selección o, sin ella, el del documento; y si había selección.
func (p *panelAsistente) contexto(accion string) (string, bool) {
	texto, sel := p.d.ContextoIA()
	if sel {
		return texto, true
	}
	if accion == "chat" {
		return "", false
	}
	return texto, false
}

// pedir es `run`: una petición en vuelo como mucho; la anterior se cancela.
func (p *panelAsistente) pedir(accion, peticion string) {
	t := p.ctx.Traducir
	if p.cfg.Asistir == nil || p.ctx.SinRed || (accion == "chat" && strings.TrimSpace(peticion) == "") {
		return
	}
	contexto, conSel := p.contexto(accion)
	if accion != "chat" && strings.TrimSpace(contexto) == "" {
		p.resultado, p.fallo = "", t("ai.needsText")
		p.pintarRespuesta()
		return
	}
	if p.cancelar != nil {
		p.cancelar()
	}
	c, cancelar := context.WithTimeout(context.Background(), PlazoIA)
	p.cancelar = cancelar
	q := PeticionIAde(p.d.TipoIA(), accion, peticion, contexto)
	q.Idioma = p.cfg.Idioma
	p.ocupado, p.resultado, p.fallo, p.conSeleccion = true, "", "", conSel
	p.pintarRespuesta()
	asistir := p.cfg.Asistir
	p.cfg.Lanzar(func() {
		res, err := asistir(c, q)
		fyne.Do(func() {
			if c.Err() == context.Canceled {
				return // cancelada a propósito, o relevada por otra: sin mensaje
			}
			cancelar()
			p.ocupado, p.cancelar = false, nil
			switch res = strings.TrimSpace(res); {
			case err != nil:
				p.fallo = t(claveErrorIA(err))
			case res == "":
				p.fallo = t("ai.error.badResponse")
			default:
				p.resultado = res
			}
			p.pintarRespuesta()
		})
	})
}

// pintarRespuesta es la zona de abajo: generando (con «Cancelar»), el fallo, la
// respuesta con sus cuatro acciones, o la pista de que aún no hay nada.
func (p *panelAsistente) pintarRespuesta() {
	t, v := p.ctx.Traducir, tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var objs []fyne.CanvasObject
	switch {
	case p.ocupado:
		cancelar := editor.NuevoBotonPequeno(t("action.cancel"), func() {
			if p.cancelar != nil {
				p.cancelar()
			}
			p.ocupado, p.cancelar = false, nil
			p.pintarRespuesta()
		})
		objs = []fyne.CanvasObject{container.New(ui.Fila{Hueco: tema.Espacio(2)},
			ui.NuevaRueda(16, "text-muted", v), ui.NuevoRotulo(t("ai.generating"), tema.TextoSM, ui.Normal, "text-muted"), cancelar)}
	case p.fallo != "":
		objs = []fyne.CanvasObject{ui.NuevoParrafo(p.fallo, tema.TextoSM, ui.Normal, "danger-text", v)}
	case p.resultado != "":
		insertar := editor.NuevoBotonPequeno(t("ai.result.insert"), func() { p.aplicar(false) })
		insertar.Glifo = iconos.TrazoEd(iconos.EdMas, 14, "text", v)
		reemplazar := editor.NuevoBotonPequeno(t("ai.result.replace"), func() { p.aplicar(true) })
		reemplazar.Inerte = !p.conSeleccion
		copiar := editor.NuevoBotonPequeno(t("ai.result.copy"), func() {
			fyne.CurrentApp().Clipboard().SetContent(p.resultado)
			p.avisar(ui.AvisoExito, "ai.copied")
		})
		copiar.Glifo = iconos.TrazoEd(iconos.EdCopiar, 14, "text", v)
		descartar := editor.NuevoBotonPequeno(t("ai.result.discard"), func() { p.resultado = ""; p.pintarRespuesta() })
		// `whitespace-pre-wrap`: los saltos del modelo se respetan.
		lineas := make([]fyne.CanvasObject, 0, strings.Count(p.resultado, "\n")+1)
		for _, l := range strings.Split(p.resultado, "\n") {
			lineas = append(lineas, ui.NuevoParrafo(l, tema.TextoSM, ui.Normal, "text", v))
		}
		texto := container.New(ui.Columna{}, lineas...)
		fondo := canvas.NewRectangle(tema.Color("bg-subtle", v)) // `rounded-md border bg-bg-subtle p-2`
		fondo.StrokeColor, fondo.StrokeWidth, fondo.CornerRadius = tema.Color("border", v), tema.Borde, tema.RadioMD
		caja := container.NewStack(fondo,
			container.New(layout.NewCustomPaddedLayout(tema.Espacio(2), tema.Espacio(2), tema.Espacio(2), tema.Espacio(2)), texto))
		objs = []fyne.CanvasObject{
			caja,
			container.New(&envuelta{hueco: tema.Espacio(1.5), ancho: anchoContenidoIA}, insertar, reemplazar, copiar, descartar),
		}
	default:
		objs = []fyne.CanvasObject{ui.NuevoParrafo(t("ai.empty"), tema.TextoXS, ui.Normal, "text-muted", v)}
	}
	p.respuesta.Objects = objs
	p.respuesta.Refresh()
}

func (p *panelAsistente) avisar(tono ui.TonoAviso, clave string) {
	if p.ctx.Notificar != nil {
		p.ctx.Notificar(tono, p.ctx.Traducir(clave))
	}
}

// aplicar es `apply`: con formato (varios bloques, títulos, listas, énfasis) y sin
// reemplazar, se escriben los bloques con el formato del documento; si no, el texto va
// tal cual, en lugar de la selección o detrás de ella.
func (p *panelAsistente) aplicar(reemplazar bool) {
	if p.resultado == "" {
		return
	}
	if clave := p.d.AplicarIA(p.resultado, reemplazar); clave != "" {
		p.avisar(ui.AvisoExito, clave)
		p.resultado = ""
		p.pintarRespuesta()
	}
}

// TipoIA: un documento de texto.
func (f *Folio) TipoIA() string { return "document" }

// ContextoIA es la selección o, sin ella, el documento entero.
func (f *Folio) ContextoIA() (string, bool) {
	if sel := strings.TrimSpace(f.textoSeleccionado()); f.haySeleccion() && sel != "" {
		return sel, true
	}
	return f.textoDocumento(), false
}

// AplicarIA es `apply`: con formato (varios bloques, títulos, listas, énfasis) y sin
// reemplazar, se escriben los bloques con el formato del documento; si no, el texto va
// tal cual, en lugar de la selección o detrás de ella.
func (f *Folio) AplicarIA(respuesta string, reemplazar bool) string {
	if !f.editable() {
		return ""
	}
	bloques := ia.BloquesDe(respuesta)
	conFormato := len(bloques) > 1
	for _, b := range bloques {
		conFormato = conFormato || b.Tipo != ia.Parrafo
		for _, tr := range b.Trozos {
			conFormato = conFormato || tr.Negrita || tr.Cursiva
		}
	}
	if !reemplazar && f.haySeleccion() {
		_, fin := f.orden()
		f.mover(fin, false) // detrás de la selección, sin tocarla
	}
	if conFormato && !reemplazar {
		f.antesDeCambiar(false)
		if l, ok := f.doc.InsertarHTML(f.doc.Bloques(), f.cur.b, f.cur.p, rico.HTMLDeBloques(bloques)); ok {
			f.irA(l, 0)
		}
		f.reestructurado()
	} else {
		f.escribir(respuesta)
	}
	if reemplazar {
		return "ai.replaced"
	}
	return "ai.inserted"
}

// anchoContenidoIA es lo que queda del panel dentro de su `p-3`.
var anchoContenidoIA = editor.AnchoPanel - 2*editor.MargenPanel

// separador es una raya de borde a lo ancho.
func separador(v tema.Variante) fyne.CanvasObject {
	r := canvas.NewRectangle(tema.Color("border", v))
	r.SetMinSize(fyne.NewSize(0, tema.Borde))
	return r
}

// anchoSelectorIdioma es el del `<select>` del idioma de destino, a su medida en la web.
const anchoSelectorIdioma float32 = 120

// alFinal es `flex justify-end`: el hijo a su medida, pegado a la derecha.
type alFinal struct{}

func (alFinal) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (alFinal) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(t.Width-m.Width, 0))
	objs[0].Resize(m)
}
