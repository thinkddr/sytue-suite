// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"golang.org/x/text/unicode/norm"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: escribir una nota —el `editando` de NoteCard.tsx— y la herramienta de
// enlaces (LinkTool.tsx).
//
// Editar es pulsar la nota, como una ficha de papel: no hay «modo edición» ni lápiz. Se
// guarda SOLO (400 ms después de la última tecla y como mucho cada 2 s) y AL SALIR, se
// salga como se salga: Escape, Ctrl+Intro o llevando el foco a otra nota. Los guardados
// de una sesión son un solo paso de deshacer del cuaderno.

// sesionEdicion es la nota abierta en el editor.
type sesionEdicion struct {
	nota   string
	editor *EditorNota
	// fusion agrupa los guardados de esta sesión en un paso del historial.
	fusion string
	// html es lo último escrito y guardado lo último mandado.
	html, guardado string
	sucioDesde     time.Time
	reloj          *time.Timer
	// compartida es una nota que me han compartido con edición: se guarda con
	// `guardarCompartida` y vive en el lienzo de «Compartido conmigo» (compartidas.go).
	compartida bool
}

// abrirEditor abre una nota para escribir, cerrando (y guardando) la que hubiera.
func (p *Pantalla) abrirEditor(n Nota) {
	if s := p.edicion; s != nil && s.nota == n.ID {
		p.enfocar(s.editor.Zona())
		return
	}
	p.cerrarEditor()
	if !n.Editable(!p.hayRed(), p.bloqueado) {
		return
	}
	ed := NuevoEditorNota(p.tr, n.Doc, p.tr.T("note.placeholder", nil))
	s := &sesionEdicion{
		nota: n.ID, editor: ed, fusion: n.ID + ":" + strconv.FormatInt(time.Now().UnixNano(), 36),
	}
	s.html = ed.HTML()
	s.guardado = s.html
	ed.AlCambiar = func(html string) { p.alEscribir(s, html) }
	ed.AlTerminar = p.cerrarEditor
	ed.AlEnlazar = func(d fyne.CanvasObject) { p.herramientaEnlace(ed, d) }
	ed.AlCrecer = p.recolocarLienzo
	ed.AlIrA = p.abrirIrA
	ed.portal = p.portal
	if p.Asistir != nil {
		ed.ConAsistente(p.Asistir, string(p.loc))
	}
	p.edicion = s
	p.lienzo.Seleccionada, p.lienzo.Editando, p.lienzo.Editor = n.ID, n.ID, ed
	p.Repintar()
	p.enfocar(ed.Zona())
}

// alEscribir programa el guardado: la pausa, o lo que quede del techo de 2 s contando
// desde el primer cambio sin guardar.
func (p *Pantalla) alEscribir(s *sesionEdicion, html string) {
	s.html = html
	if s.reloj != nil {
		s.reloj.Stop()
	}
	if html == s.guardado {
		s.sucioDesde = time.Time{}
		return
	}
	ahora := time.Now()
	if s.sucioDesde.IsZero() {
		s.sucioDesde = ahora
	}
	s.reloj = time.AfterFunc(EsperaDeAutoguardado(s.sucioDesde, ahora), func() {
		enHilo(func() { p.guardarEdicion(s) })
	})
}

// guardarEdicion manda lo escrito si ha cambiado desde lo último que se mandó, con los
// enlaces a otras notas que haya en el texto.
func (p *Pantalla) guardarEdicion(s *sesionEdicion) {
	if s.html == s.guardado {
		return
	}
	html := s.html
	s.guardado, s.sucioDesde = html, time.Time{}
	pa := Parche{Contenido: &html}
	if s.compartida {
		if n, ok := p.compartida(s.nota); ok {
			pa.Enlaces = enlacesSincronizados(n.Note, s.editor.Documento(), p.AppURL)
		}
		nota := s.nota
		lanzar(func() error { return p.guardarCompartida(nota, pa) })
		return
	}
	if n, ok := p.nota(s.nota); ok && !p.activo.Cifrado {
		pa.Enlaces = enlacesSincronizados(n.Crudo, s.editor.Documento(), p.AppURL)
	}
	nota, fusion := s.nota, s.fusion
	lanzar(func() error { return p.editarCon(nota, pa, true, fusion) })
}

// cerrarEditor guarda lo pendiente y vuelve a la lectura.
func (p *Pantalla) cerrarEditor() {
	s := p.edicion
	if s == nil {
		return
	}
	p.portal.poner(s.editor) // el menú y la burbuja se van con el editor
	if s.reloj != nil {
		s.reloj.Stop()
	}
	p.edicion = nil
	p.lienzo.Editando, p.lienzo.Editor, p.lienzo.Ancha = "", nil, false
	if l := p.comp.lienzo; l != nil {
		l.Editando, l.Editor, l.Ancha = "", nil, false
	}
	p.guardarEdicion(s)
	p.Repintar()
}

// recolocarLienzo vuelve a colocar el lienzo cuando la nota que se escribe cambia de alto:
// sin esto la tarjeta crece por encima de la siguiente.
func (p *Pantalla) recolocarLienzo() {
	if p.desplazador != nil && p.desplazador.Content != nil {
		p.desplazador.Content.Refresh()
		p.desplazador.Refresh()
	}
}

// enlacesSincronizados son los `links[]` de la nota tras escribirla: los que apuntan a
// otras cosas de la suite se quedan, y los de nota salen del TEXTO —un enlace borrado del
// texto desaparece también del retroenlace—. Es `sincronizarEnlaces` de la web.
func enlacesSincronizados(n api.Note, d *Documento, appURL func(string) string) []api.NoteLink {
	out := []api.NoteLink{}
	for _, l := range n.Links {
		if l.TargetKind != "note" {
			out = append(out, l)
		}
	}
	vistos := map[string]bool{}
	for _, b := range d.Bloques {
		var href, rotulo string
		cerrar := func() {
			if _, nota, _, ok := destinoInterno(href, appURL); ok && !vistos[nota] {
				vistos[nota] = true
				out = append(out, api.NoteLink{TargetKind: "note", TargetID: nota, Label: strings.TrimSpace(rotulo), URL: href})
			}
			href, rotulo = "", ""
		}
		for _, t := range b.Tramos {
			if t.Enlace != href {
				cerrar()
				href = t.Enlace
			}
			rotulo += t.Texto
		}
		cerrar()
	}
	return out
}

// urlDeNota es el enlace a una nota, y a un título de dentro con `slug` (`urlDeNota` de la
// web). Los parámetros van en el orden de la web y no en el alfabético de `url.Values`:
// el enlace se GUARDA en el contenido, y dos clientes no deben escribirlo distinto.
func (p *Pantalla) urlDeNota(cuaderno, nota, slug string) string {
	if p.AppURL == nil {
		return ""
	}
	u := strings.TrimSuffix(p.AppURL("/"), "/") + "/?notebook=" + url.QueryEscape(cuaderno) + "&note=" + url.QueryEscape(nota)
	if slug != "" {
		u += "&h=" + url.QueryEscape(slug)
	}
	return u
}

// Medidas de la herramienta de enlaces (LinkTool.tsx).
const (
	AnchoHerramientaEnlace = 26 * tema.Rem // `w-[min(92vw,26rem)]`
	PXOpcionEnlace         = 2 * 0.25 * tema.Rem
	PYOpcionEnlace         = 1.5 * 0.25 * tema.Rem
)

// opcionEnlace es una de las que ofrece la herramienta (`Opcion` de LinkTool.tsx): lo que
// se enlaza, el nombre que se inserta si no había selección, la nota y el título que se
// leen, el cuaderno si es de otro, y de qué grupo es.
type opcionEnlace struct {
	url, rotulo  string
	nota, titulo string
	cuaderno     string
	tipo         tipoEnlace
}

type tipoEnlace int

const (
	enlaceURL tipoEnlace = iota
	enlaceLocal
	enlaceRemoto
	enlaceDrive
)

// Lo que LinkTool.tsx pide y enseña.
const (
	maxOpcionesEnlace = 8
	// esperaEnlace es el retardo de la búsqueda en otros cuadernos: escribir «informe»
	// son siete pulsaciones y siete viajes para resultados que nadie llegó a leer.
	esperaEnlace      = 250 * time.Millisecond
	minimoBusquedaEnl = 2
	limiteBusquedaEnl = 20
)

// titulo es un `<hN>` de un contenido: su texto y su slug único (`titulosDe`).
type titulo struct {
	nivel       int
	texto, slug string
}

var (
	// RE2 no tiene retrorreferencias: se casa cualquier cierre y se comprueba que sea
	// el del mismo nivel.
	reTitulo    = regexp.MustCompile(`(?is)<h([1-6])(?:\s[^>]*)?>(.*?)</h([1-6])>`)
	reEtiqueta  = regexp.MustCompile(`<[^>]*>`)
	reNoSlug    = regexp.MustCompile(`[^a-z0-9]+`)
	entidadesTx = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&nbsp;", " ")
)

// titulosDe son los títulos de un HTML saneado, en orden y con slug único.
func titulosDe(html string) []titulo {
	var out []titulo
	vistos := map[string]int{}
	for _, m := range reTitulo.FindAllStringSubmatch(html, -1) {
		if m[1] != m[3] {
			continue
		}
		texto := strings.TrimSpace(entidadesTx.Replace(reEtiqueta.ReplaceAllString(m[2], "")))
		if texto == "" {
			continue
		}
		base := cmp.Or(slugDe(texto), "titulo")
		n := vistos[base]
		vistos[base] = n + 1
		slug := base
		if n > 0 {
			slug = base + "-" + strconv.Itoa(n+1)
		}
		out = append(out, titulo{nivel: int(m[1][0] - '0'), texto: texto, slug: slug})
	}
	return out
}

// slugDe es `slugDe` de la web, letra a letra: NFD, fuera las marcas del bloque
// U+0300–U+036F, minúsculas y guiones, hasta 80. No es un buscador: es el ancla `h=` de
// un título y tiene que salir IGUAL que en la web o los enlaces no llevan a su sitio.
func slugDe(texto string) string {
	s := strings.Map(func(r rune) rune {
		if r >= 0x300 && r <= 0x36f {
			return -1
		}
		return r
	}, norm.NFD.String(texto))
	s = strings.Trim(reNoSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	return s[:min(len(s), 80)]
}

// opcionesEnlace son las de este cuaderno para lo escrito, al instante y sin red: la URL
// si lo es, y las notas y los títulos de dentro que lo contienen (hasta ocho).
func (p *Pantalla) opcionesEnlace(q string) []opcionEnlace {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	if esWeb(q) || strings.HasPrefix(q, "mailto:") {
		return []opcionEnlace{{url: q, rotulo: q, nota: q, tipo: enlaceURL}}
	}
	nq := buscar.Plegar(q)
	var out []opcionEnlace
	for _, n := range p.notas {
		t := strings.TrimSpace(n.Titulo)
		nombre := cmp.Or(t, "…")
		if t != "" && buscar.Contiene(t, nq) {
			out = append(out, opcionEnlace{url: p.urlDeNota(p.activo.ID, n.ID, ""), rotulo: t, nota: nombre, tipo: enlaceLocal})
		}
		for _, h := range titulosDe(n.Contenido) {
			if buscar.Contiene(h.texto, nq) {
				out = append(out, opcionEnlace{
					url: p.urlDeNota(p.activo.ID, n.ID, h.slug), rotulo: h.texto,
					nota: nombre, titulo: h.texto, tipo: enlaceLocal,
				})
			}
		}
		if len(out) >= maxOpcionesEnlace {
			break
		}
	}
	return out[:min(len(out), maxOpcionesEnlace)]
}

// opcionesRemotas son las de los demás cuadernos, de la búsqueda del servicio: la nota y
// sus títulos que casen.
func (p *Pantalla) opcionesRemotas(q string, hits []api.NoteHit) []opcionEnlace {
	nq := buscar.Plegar(q)
	var out []opcionEnlace
	for _, h := range hits {
		if h.NotebookID == p.activo.ID {
			continue // lo local ya está arriba
		}
		nombre := cmp.Or(h.Title, h.Notebook)
		out = append(out, opcionEnlace{
			url: p.urlDeNota(h.NotebookID, h.ID, ""), rotulo: nombre, nota: nombre, cuaderno: h.Notebook, tipo: enlaceRemoto,
		})
		for _, t := range h.Headings {
			if buscar.Contiene(t, nq) {
				out = append(out, opcionEnlace{
					url: p.urlDeNota(h.NotebookID, h.ID, slugDe(t)), rotulo: t,
					nota: nombre, titulo: t, cuaderno: h.Notebook, tipo: enlaceRemoto,
				})
			}
		}
		if len(out) >= maxOpcionesEnlace {
			break
		}
	}
	return out[:min(len(out), maxOpcionesEnlace)]
}

// filasEnlace pinta la lista: cada grupo rotulado por su primer elemento (`sytue-meta px-2
// pt-1`), cada opción `rounded-md px-2 py-1.5 text-base` con su glifo de 14, la activa en
// `bg-primary/10`; «ninguna coincide» si no hay nada, y «buscando…» mientras la red.
func (p *Pantalla) filasEnlace(opciones []opcionEnlace, activa int, buscando bool, elegir func(opcionEnlace), v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	filas := make([]fyne.CanvasObject, 0, len(opciones)+3)
	meta := func(s string) fyne.CanvasObject {
		return container.New(ui.Margen{Izq: PXOpcionEnlace, Der: PXOpcionEnlace, Arr: 1 * 0.25 * tema.Rem},
			lineaTok(s, tema.TextoXS, ui.Normal, "text-subtle", v))
	}
	// Cada grupo se rotula por su primer elemento: cuando el tipo cambia respecto al de
	// la fila anterior. -1 es «ninguna todavía».
	previo := tipoEnlace(-1)
	for i, o := range opciones {
		if i == 0 && o.tipo == enlaceLocal {
			filas = append(filas, meta(t("editor.linkThisNotebook", nil)))
		}
		if o.tipo == enlaceRemoto && previo != enlaceRemoto {
			filas = append(filas, meta(t("editor.linkOtherNotebooks", nil)))
		}
		if o.tipo == enlaceDrive && previo != enlaceDrive {
			filas = append(filas, meta(MarcaDrive))
		}
		previo = o.tipo
		glifo := NotaD
		if o.tipo == enlaceDrive {
			glifo = FicheroD
		}
		var texto fyne.CanvasObject
		switch {
		case o.tipo == enlaceURL:
			glifo = EnlaceD
			texto = lineaTok(t("editor.linkToUrl", i18n.Vars{"url": o.url}), tema.TextoBase, ui.Normal, "text", v)
		case o.titulo != "":
			glifo = TituloD
			d := &dosTonos{
				a: o.nota, b: " › " + o.titulo,
				ta: ui.Texto(o.nota, tema.TextoBase, ui.Normal, tema.Color("text", v)),
				tb: ui.Texto(" › "+o.titulo, tema.TextoBase, ui.Normal, tema.Color("text-muted", v)),
			}
			texto = container.New(d, d.ta, d.tb)
		default:
			texto = lineaTok(o.nota, tema.TextoBase, ui.Normal, "text", v)
		}
		piezas := []fyne.CanvasObject{Glifo(glifo, 14, "text-subtle", v), texto}
		if o.cuaderno != "" {
			piezas = append(piezas, lineaTok(o.cuaderno, tema.TextoXS, ui.Normal, "text-subtle", v))
		}
		op := NuevaOpcion(cmp.Or(o.titulo, o.nota), container.New(&filaElastica{hueco: HuecoFilaDialogo, elastico: 1}, piezas...),
			func() { elegir(o) })
		op.PX, op.PY, op.Superficie = PXOpcionEnlace, PYOpcionEnlace, "surface-raised"
		if i == activa {
			op.Activa, op.FondoActivo = true, velo(tema.Color("primary", v), 0.1)
		}
		filas = append(filas, op)
	}
	fila := func(o ...fyne.CanvasObject) fyne.CanvasObject {
		return container.New(ui.Margen{Izq: PXOpcionEnlace, Der: PXOpcionEnlace, Arr: 2 * 0.25 * tema.Rem, Abj: 2 * 0.25 * tema.Rem},
			container.New(ui.Fila{Hueco: HuecoFilaDialogo}, o...))
	}
	if len(opciones) == 0 && !buscando {
		filas = append(filas, fila(lineaTok(t("editor.linkNoResults", nil), tema.TextoBase, ui.Normal, "text-muted", v)))
	}
	if buscando {
		filas = append(filas, fila(ui.NuevaRueda(14, "text-subtle", v),
			lineaTok(t("editor.linkSearching", nil), tema.TextoXS, ui.Normal, "text-subtle", v)))
	}
	return container.New(ui.Columna{}, filas...)
}

// dosTonos es `<span class="truncate">nota<span class="text-text-muted"> › título</span>`:
// dos textos seguidos recortados juntos, con la elipsis al final del conjunto.
type dosTonos struct {
	a, b   string
	ta, tb *canvas.Text
}

func (d *dosTonos) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(ui.Medir(d.a+d.b, tema.TextoBase, ui.Normal).Width, tema.TextoBase.Linea)
}

func (d *dosTonos) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	a, b := d.ta, d.tb
	y := (t.Height - tema.TextoBase.Linea) / 2
	wa := ui.Truncar(a, d.a, tema.TextoBase, ui.Normal, t.Width)
	ui.ColocarTexto(a, tema.TextoBase, 0, y)
	ui.Truncar(b, d.b, tema.TextoBase, ui.Normal, t.Width-wa)
	if a.Text != d.a {
		b.Text = "" // la nota ya no cabe entera: el título no se ve
	}
	ui.ColocarTexto(b, tema.TextoBase, wa, y)
}

// recolocarEmergente vuelve a medir el panel cuando su lista cambia de alto: la capa lo
// mide al abrirse, y sin esto las opciones que llegan al escribir quedaban cortadas.
func recolocarEmergente(em *ui.Emergente) {
	if em != nil {
		em.Recolocar()
	}
}

// herramientaEnlace es LinkTool.tsx: UNA caja para todo. Se pega una URL y se enlaza; se
// escribe el nombre de una nota —o de un título de dentro— y salen primero las de este
// cuaderno, al instante, después las de los demás, de la búsqueda, y al final los
// ficheros de Sytue Drive que casen (suite.go). Flechas para elegir e Intro para enlazar.
func (p *Pantalla) herramientaEnlace(ed *EditorNota, disparador fyne.CanvasObject) {
	v := p.variante()
	var em *ui.Emergente
	campo := ui.NuevaEntrada(p.tr.T("editor.linkPlaceholder", nil))
	campo.Ancho = min(AnchoHerramientaEnlace, p.anchoVentana()*0.92) - 2*PXOpcionEnlace
	lista := container.NewStack()
	cuerpo := container.New(ui.Columna{Hueco: 2 * 0.25 * tema.Rem}, campo, lista)
	var (
		q                string
		locales, remotas []opcionEnlace
		drive            []opcionEnlace
		activa, vuelta   int
		buscando         bool
		reloj            *time.Timer
	)
	opciones := func() []opcionEnlace { return append(append(slices.Clip(locales), remotas...), drive...) }
	cerrar := func() {
		if reloj != nil {
			reloj.Stop()
		}
		vuelta++ // lo que esté en vuelo ya no se pinta
		if em != nil {
			em.Cerrar()
		}
		ed.reenfocar()
	}
	elegir := func(o opcionEnlace) {
		cerrar()
		ed.aplicarEnlace(o.url, o.rotulo)
	}
	pintar := func() {
		lista.Objects = nil
		if strings.TrimSpace(q) != "" {
			lista.Objects = []fyne.CanvasObject{p.filasEnlace(opciones(), activa, buscando, elegir, v)}
		}
		lista.Refresh()
		recolocarEmergente(em)
	}
	campo.AlCambiar(func(s string) {
		q, activa = s, 0
		consulta := strings.TrimSpace(s)
		locales, remotas, drive, buscando = p.opcionesEnlace(consulta), nil, nil, false
		if reloj != nil {
			reloj.Stop()
		}
		vuelta++
		if len([]rune(consulta)) >= minimoBusquedaEnl && (len(locales) == 0 || locales[0].tipo != enlaceURL) &&
			(p.svc != nil || p.Suite != nil) && p.hayRed() {
			buscando = true
			mia := vuelta
			reloj = time.AfterFunc(esperaEnlace, func() {
				r, d := p.buscarFuera(consulta)
				fyne.Do(func() {
					if mia != vuelta {
						return
					}
					buscando, remotas, drive = false, r, d
					pintar()
				})
			})
		}
		pintar()
	})
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyDown:
			activa = max(min(activa+1, len(opciones())-1), 0)
			pintar()
			return true
		case fyne.KeyUp:
			activa = max(activa-1, 0)
			pintar()
			return true
		case fyne.KeyReturn, fyne.KeyEnter:
			if o := opciones(); activa < len(o) {
				elegir(o[activa])
			}
			return true
		case fyne.KeyEscape:
			cerrar()
			return true
		default:
		}
		return false
	}
	pintar()
	// El `<Popover>` de RichText.tsx con el LinkTool dentro: `p-4` del panel y el `p-2` de
	// su `grid`.
	p2 := tema.Espacio(2)
	em = ui.MostrarPopover(disparador, container.New(ui.Margen{Izq: p2, Arr: p2, Der: p2, Abj: p2}, cuerpo))
	if c := fyne.CurrentApp().Driver().CanvasForObject(disparador); c != nil {
		c.Focus(campo)
	}
}
