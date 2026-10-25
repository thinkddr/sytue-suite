// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

// Contrapartida de pdf/Portada.tsx y pdf/Abrir.tsx de `@sytue/office`; la pantalla de
// la web que porta la marca `cmd/certipdf/main.go`, que es quien la monta.

import (
	"cmp"
	"image/color"
	"math"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
La PORTADA de CertiPDF (`PortadaPdf`): lo que se ve sin documento abierto. Lo mismo que
las de Docus, Calcus y Diapos —crear en blanco, los recientes con filtro— y, además, la
zona de abrir un PDF del ordenador o escanear, que ya era la puerta de CertiPDF porque un
PDF casi siempre viene de fuera. Los recientes salen de la carpeta «PDF» de Drive: un PDF
no es un documento de Docs.

Medido en Chromium a 1280 y a 360 (`TestLaPortadaSeColocaComoEnLaWeb`): la columna de
`max-w-[1280px] px-6 pt-8 pb-10`, la cabecera, «Crear nuevo» con la tarjeta y la zona de
abrir en `1fr 2fr` desde `lg:` —una debajo de otra por debajo—, y «Recientes» con la
insignia, el filtro de `w-64` y la rejilla `auto-fill minmax(180px, 1fr) gap-4`.
*/

// MaxRecientes es el `MAXIMO` de `pdf-recientes.ts`: los 48 más recientes.
const MaxRecientes = 48

// Las medidas de la portada que no comparte con la de documentos.
var (
	// abajoPortada es el `pb-10`.
	abajoPortada = tema.Espacio(10)
	// rellenoAbrir es el `p-6` de la zona de abrir (embebida) y huecoAbrir su `gap-3`.
	rellenoAbrir = tema.Espacio(6)
	huecoAbrir   = tema.Espacio(3)
	// bordeAbrir es border-2 border-dashed y radioAbrir el token rounded-xl.
	bordeAbrir float32 = 2
	radioAbrir         = tema.RadioXL
	// cuerpoCargando es el `py-10` de la rueda de 22.
	cuerpoCargando = tema.Espacio(10)
)

// GlifoPDF es `PdfGlyph` de `DocList.tsx`: la hoja y el estampador dentro, escalado a
// 0,63. Las figuras se copian TAL CUAL: `TestElGlifoPDFEsElDeLaWeb`.
//
// El grosor del sello NO se escribe aquí y en la web sí (`grosorDeGlifo(size) / 0.63`):
// el navegador encoge el trazo con el grupo y hay que compensarlo; `oksvg` no lo
// encoge cuando el grosor es HEREDADO (ver `grosorEnGrupo` en iconos/pintar.go), así
// que el sello sale ya con el mismo grosor que la hoja, que es lo que la web busca.
const GlifoPDF = `<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8zM14 3v5h5" />` +
	`<g transform="translate(4.4 7.6) scale(0.63)"><path d="M4.5 20.5h15M6.5 17.5h11v-1.8a2 2 0 0 0-1.5-1.9l-2.2-.6a1.3 1.3 0 0 1-.9-1.5l.5-2.9A3.4 3.4 0 0 0 12 3.5a3.4 3.4 0 0 0-3.4 4.3l.5 2.9a1.3 1.3 0 0 1-.9 1.5l-2.2.6a2 2 0 0 0-1.5 1.9Z" /></g>`

// Portada es la pantalla sin documento.
type Portada struct {
	t   TraductorVars
	loc i18n.Locale

	// Lista son los PDF de «PDF», el más reciente primero. Nil mientras cargan.
	Lista []api.File
	// Fallo es que no se pudieron cargar.
	Fallo bool
	// Creando es «Creando…» en la tarjeta; ErrorCrear, la frase de debajo.
	Creando, ErrorCrear bool
	// Abriendo es «Abriendo…» en el botón de elegir; ErrorAbrir, la frase del fallo.
	Abriendo   bool
	ErrorAbrir string
	// Encima es que hay un fichero arrastrado sobre la ventana: el `encima` de `AbrirPdf`.
	Encima bool
	// Peticiones son las de firma (`PeticionesDeFirma`): las pendientes que me hacen y
	// las que he hecho. Nil mientras cargan o si no se pudieron traer: sin ellas no hay
	// sección, como en la web.
	Peticiones *api.PDFPeticionesFirma
	// AlPeticion abre la de una fila: la recibida, para firmarla; la pedida, su firmado.
	AlPeticion func(p api.PDFPeticionFirma, recibida bool)
	// AlDesbloquear quita el bloqueo a una persona (ADR 0059): los ajustes de quien
	// recibe, con la lista de quién ha bloqueado.
	AlDesbloquear func(sub string)

	// AlCrear crea un PDF en blanco; AlElegir abre el selector de ficheros; AlEscanear,
	// el de fotos; AlAbrir abre un reciente.
	AlCrear, AlElegir, AlEscanear func()
	AlAbrir                       func(api.File)
	Refresco                      ui.RefrescoLista

	filtro   *ui.Entrada
	consulta string
	vista    *fyne.Container
	marco    *marcoPortada
	scroll   *container.Scroll
	fondo    *canvas.Rectangle
}

// NuevaPortada monta la portada, cargando.
func NuevaPortada(t TraductorVars, loc i18n.Locale) *Portada {
	p := &Portada{t: t, loc: loc}
	p.filtro = ui.NuevaEntrada(t("list.filter.placeholder", nil))
	p.filtro.Busqueda = true // `type="search"`
	p.filtro.AlCambiar(func(s string) { p.consulta = s; p.Recomponer() })
	p.marco = &marcoPortada{p: p}
	p.vista = container.New(p.marco)
	p.scroll = container.NewVScroll(p.vista)
	p.fondo = canvas.NewRectangle(color.Transparent)
	// El fondo, las tarjetas y los rótulos se colorean al componer: un cambio de tema en
	// caliente (desde esta app o desde otra de las trece) los recompone. Sin esto el
	// fondo se quedaba claro con los textos del tema oscuro encima (visto en una ventana
	// real el 2026-09-27). `AddListener` llama en el hilo de la interfaz —lo invoca
	// `SetTheme`, que ya va por `fyne.Do`— y se apunta UNA vez: Objeto se llama en
	// cada repintado de la pantalla.
	fyne.CurrentApp().Settings().AddListener(func(a fyne.Settings) {
		p.fondo.FillColor = tema.Color("bg", tema.De(a.ThemeVariant()))
		p.fondo.Refresh()
		p.Recomponer()
	})
	return p
}

// Objeto es lo que se mete en la pantalla: la columna, que se desplaza entera.
func (p *Portada) Objeto() fyne.CanvasObject {
	p.recomponer()
	p.fondo.FillColor = tema.Color("bg", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	p.Refresco.Arriba = func() bool { return p.scroll.Offset.Y <= 0.5 }
	return container.NewStack(p.fondo, ui.RefrescarAlTirar(container.NewThemeOverride(p.scroll, temaVisor{}), &p.Refresco))
}

// PonerEncima es el `dragover`/`dragleave` de la zona de abrir: con un fichero encima,
// `border-accent bg-selected`. La web lo mira sobre la zona y aquí sobre la ventana
// entera, porque soltarlo en cualquier sitio de la ventana también lo abre.
func (p *Portada) PonerEncima(encima bool) {
	if p.Encima != encima {
		p.Encima = encima
		p.Recomponer()
	}
}

// Recomponer vuelve a pintar con el estado que haya.
func (p *Portada) Recomponer() {
	p.recomponer()
	p.vista.Refresh()
}

// Filtrados son los recientes cuyo nombre contiene lo escrito, sin distinguir mayúsculas.
func Filtrados(lista []api.File, q string) []api.File {
	q = buscar.Plegar(strings.TrimSpace(q))
	if q == "" {
		return lista
	}
	var out []api.File
	for _, f := range lista {
		if strings.Contains(buscar.Plegar(f.Name), q) {
			out = append(out, f)
		}
	}
	return out
}

func (p *Portada) recomponer() {
	t := func(c string) string { return p.t(c, nil) }
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := p.marco
	m.titulo = ui.NuevoRotulo(t("pdf.list.heading"), ui.EscalonTitulo(2), ui.PesoTitulo(2), "text")
	m.lema = ui.NuevoParrafo(t("pdf.list.tagline"), tema.TextoBase, ui.Normal, "text-muted", v)
	m.nuevo = ui.NuevoVersal(t("list.new.heading"), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	m.reciente = ui.NuevoVersal(t("list.recent.heading"), tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
	m.acceso = nuevoAccesoPDF(t("kind.pdf"), t("list.new.blank"), t("list.new.creating"), p.Creando, p.AlCrear)
	m.abrir = p.cajaAbrir()
	m.errorCrear = nil
	if p.ErrorCrear {
		m.errorCrear = ui.NuevoRotulo(t("create.error.description"), tema.TextoSM, ui.Normal, "danger-text")
	}
	m.insignia = nil
	if len(p.Lista) > 0 {
		m.insignia = ui.NuevaInsignia(i18n.FormatNumber(p.loc, float64(len(p.Lista))), ui.InsigniaNeutra)
	}
	m.peticiones = p.seccionPeticiones()
	m.cuerpo, m.tarjetas = p.cuerpoRecientes(), nil
	if len(p.Lista) > 0 {
		for _, f := range Filtrados(p.Lista, p.consulta) {
			file := f
			m.tarjetas = append(m.tarjetas, nuevaTarjetaPDF(f, t("kind.pdf"), p.loc, func() {
				if p.AlAbrir != nil {
					p.AlAbrir(file)
				}
			}))
		}
	}
	objs := append(append([]fyne.CanvasObject{m.titulo, m.lema}, m.nuevo.Objetos()...), m.reciente.Objetos()...)
	objs = append(objs, m.acceso, m.abrir, p.filtro)
	for _, o := range []fyne.CanvasObject{m.errorCrear, m.insignia, m.cuerpo, m.peticiones} {
		if o != nil && !esNil(o) {
			objs = append(objs, o)
		}
	}
	for _, c := range m.tarjetas {
		objs = append(objs, c)
	}
	p.vista.Objects = objs
}

// esNil dice si una interfaz lleva un puntero nulo dentro (un `*ui.Rotulo` nil).
func esNil(o fyne.CanvasObject) bool {
	switch x := o.(type) {
	case *ui.Rotulo:
		return x == nil
	case *ui.Insignia:
		return x == nil
	case *fyne.Container:
		return x == nil
	}
	return false
}

// seccionPeticiones es `PeticionesDeFirma` de `Portada.tsx`: «Te piden firmar» con las
// pendientes y «Firmas que has pedido» con las diez más recientes; cada petición es una fila
// `rounded-lg border bg-surface px-3 py-2 text-sm` con el nombre y, a la derecha, quién
// y la fecha o el estado. Nil si no hay ninguna.
//
// ponytail: una columna; la web pasa a dos desde `lg:`. Sin medir contra Chromium.
func (p *Portada) seccionPeticiones() *fyne.Container {
	if p.Peticiones == nil {
		return nil
	}
	t := func(c string) string { return p.t(c, nil) }
	var recibidas []api.PDFPeticionFirma
	for _, x := range p.Peticiones.Recibidas {
		if x.Estado == api.PDFFirmaPendiente {
			recibidas = append(recibidas, x)
		}
	}
	pedidas := p.Peticiones.Pedidas[:min(len(p.Peticiones.Pedidas), 10)]
	if len(recibidas) == 0 && len(pedidas) == 0 && len(p.Peticiones.Bloqueados) == 0 {
		return nil
	}
	var partes []fyne.CanvasObject
	bloque := func(titulo string, xs []api.PDFPeticionFirma, recibida bool) {
		if len(xs) == 0 {
			return
		}
		partes = append(partes, ui.NuevoRotulo(ui.Versalitas(titulo), tema.TextoXS, ui.SemiNegrita, "text-subtle"))
		for _, x := range xs {
			quien, extra := x.NombrePide, t("pdf.signatureRequest.status."+x.Estado)
			if recibida {
				extra = i18n.FormatDate(p.loc, x.Creada.In(time.Local))
				if x.Externo && x.Ruta == "" {
					extra += " · " + t("pdf.home.requests.external")
				}
			} else {
				quien = x.CorreoDestino
			}
			partes = append(partes, p.filaPeticion(x, recibida, strings.TrimSuffix(x.NombrePDF(), ".pdf"), quien+" · "+extra))
		}
	}
	bloque(t("pdf.home.requests.received"), recibidas, true)
	bloque(t("pdf.home.requests.sent"), pedidas, false)
	if bs := p.Peticiones.Bloqueados; len(bs) > 0 {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		partes = append(partes,
			ui.NuevoRotulo(ui.Versalitas(t("pdf.home.blocked.title")), tema.TextoXS, ui.SemiNegrita, "text-subtle"),
			ui.NuevoParrafo(t("pdf.home.blocked.help"), tema.TextoXS, ui.Normal, "text-muted", v))
		for _, b := range bs {
			sub := b.Sub
			quitar := accionDeFranja(t("pdf.home.blocked.unblock"), "text", false, func() {
				if p.AlDesbloquear != nil {
					p.AlDesbloquear(sub)
				}
			})
			nombre := ui.NuevoRotulo(cmp.Or(b.Nombre, b.Sub), tema.TextoSM, ui.Normal, "text")
			partes = append(partes, container.New(ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3)},
				container.NewBorder(nil, nil, nombre, quitar)))
		}
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, partes...)
}

// filaPeticion es una fila de la sección: toda ella se pulsa.
func (p *Portada) filaPeticion(x api.PDFPeticionFirma, recibida bool, nombre, meta string) fyne.CanvasObject {
	fondo := NuevaAccion("", func() {
		if p.AlPeticion != nil {
			p.AlPeticion(x, recibida)
		}
	}).ConBorde()
	fondo.Nombre, fondo.Radio, fondo.Fondo, fondo.FondoEncima = nombre, tema.RadioLG, "surface", "bg-subtle"
	izq := ui.NuevoRotulo(nombre, tema.TextoSM, ui.Medio, "text")
	der := ui.NuevoRotulo(meta, tema.TextoXS, ui.Normal, "text-muted")
	return container.NewStack(fondo, container.New(ui.Margen{Izq: tema.Espacio(3), Der: tema.Espacio(3), Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
		container.NewBorder(nil, nil, izq, der)))
}

// cuerpoRecientes es lo de debajo de «Recientes» cuando NO son tarjetas: la rueda, el
// fallo, el vacío o «sin resultados». Nil si hay tarjetas.
func (p *Portada) cuerpoRecientes() fyne.CanvasObject {
	t := func(c string) string { return p.t(c, nil) }
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	switch {
	case p.Fallo:
		return ui.NuevoRotulo(t("pdf.list.loadFailed"), tema.TextoSM, ui.Normal, "danger-text")
	case p.Lista == nil:
		return container.New(centradoEnTodo{relleno: 0}, ui.NuevaRueda(22, "text-muted", v))
	case len(p.Lista) == 0:
		return ui.NuevoVacio(t("pdf.list.heading"), t("pdf.list.empty"))
	case len(Filtrados(p.Lista, p.consulta)) == 0:
		return ui.NuevoVacio(p.t("list.empty.search.title", i18n.Vars{"query": p.consulta}), t("list.empty.search.description"))
	}
	return nil
}

// cajaAbrir es `AbrirPdf` embebida: `rounded-xl border-2 border-dashed p-6`, en columna
// centrada con `gap-3`: el título, la frase, «Elegir un fichero», «Escanear con la
// cámara», el fallo si lo hay y dónde se guarda.
//
// Con un fichero encima ([Portada.PonerEncima]) el borde pasa a `border-accent` y el fondo
// a `bg-selected`.
//
// ponytail: el cambio es de golpe y la web lo funde en los 150 ms de `transition-colors`.
// En Wayland y en el móvil no hay aviso de arrastre (`soltar.AlArrastrar`) y no cambia.
func (p *Portada) cajaAbrir() fyne.CanvasObject {
	t := func(c string) string { return p.t(c, nil) }
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	rotulo := "pdf.open.pick"
	if p.Abriendo {
		rotulo = "pdf.open.opening"
	}
	elegir := botonPrimario(t(rotulo), tema.TextoSM, tema.Espacio(4), tema.Espacio(2), p.AlElegir)
	elegir.Inerte = p.Abriendo || p.AlElegir == nil
	escanear := NuevaAccion(t("pdf.open.scan"), p.AlEscanear).ConBorde()
	escanear.Escalon, escanear.PX, escanear.PY = tema.TextoSM, tema.Espacio(4), tema.Espacio(2)
	escanear.Inerte = p.Abriendo || p.AlEscanear == nil
	ayuda := ui.NuevoParrafo(t("pdf.open.drop"), tema.TextoSM, ui.Normal, "text-muted", v)
	ayuda.Alineacion = fyne.TextAlignCenter
	p.marco.ayudaAbrir = ayuda
	partes := []fyne.CanvasObject{
		ui.NuevoRotulo(t("pdf.open.title"), tema.TextoLG, ui.SemiNegrita, "text"),
		ayuda,
		elegir, escanear,
	}
	if p.ErrorAbrir != "" {
		partes = append(partes, ui.NuevoRotulo(p.ErrorAbrir, tema.TextoXS, ui.Normal, "danger-text"))
	}
	partes = append(partes, ui.NuevoRotulo(t("pdf.open.savedIn"), tema.Texto2XS, ui.Normal, "text-subtle"))
	fondo, borde := canvas.NewRectangle(tema.Color("surface", v)), "border"
	if p.Encima {
		fondo.FillColor, borde = tema.Color("selected", v), "accent"
	}
	fondo.CornerRadius = radioAbrir
	// `border-2 border-dashed rounded-xl`.
	return container.NewStack(fondo, ui.NuevoBordeDiscontinuo(bordeAbrir, radioAbrir, borde),
		container.New(ui.Margen{Izq: bordeAbrir + rellenoAbrir, Arr: bordeAbrir + rellenoAbrir, Der: bordeAbrir + rellenoAbrir, Abj: bordeAbrir + rellenoAbrir},
			container.New(ui.Columna{Hueco: huecoAbrir, Encaja: true}, partes...)))
}

// ---- La colocación ----

// marcoPortada coloca la portada entera. Es un layout y no una pila de contenedores
// porque casi todo depende del ancho —el lema se reparte, «Crear nuevo» pasa de una
// columna a dos en `lg:`, la rejilla cuenta columnas— y el alto de la columna sale de
// colocarlo todo.
type marcoPortada struct {
	p *Portada

	titulo          *ui.Rotulo
	lema            *ui.Parrafo
	nuevo, reciente *ui.Versal
	acceso          *accesoPDF
	abrir           fyne.CanvasObject
	ayudaAbrir      *ui.Parrafo
	errorCrear      *ui.Rotulo
	insignia        *ui.Insignia
	cuerpo          fyne.CanvasObject
	peticiones      *fyne.Container
	tarjetas        []*tarjetaPDF
	ventana, alto   float32
}

// rejillaPDF es `grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-4`.
var rejillaPDF = ui.Rejilla{Min: editor.MinTesela, Hueco: tema.Espacio(4)}

// colocar pone cada pieza y devuelve el alto de la columna. Con `mover` a falso solo
// cuenta.
func (m *marcoPortada) colocar(t fyne.Size, mover bool) float32 {
	caja := min(t.Width, tema.AnchoContenido)
	x := (t.Width-caja)/2 + editor.MargenPortada
	ancho := max(caja-2*editor.MargenPortada, 0)
	mv := func(o fyne.CanvasObject, pos fyne.Position, tam fyne.Size) {
		if mover && o != nil {
			o.Move(pos)
			o.Resize(tam)
		}
	}

	y := editor.ArribaPortada
	mv(m.titulo, fyne.NewPos(x, y), fyne.NewSize(ancho, ui.EscalonTitulo(2).Linea))
	y += ui.EscalonTitulo(2).Linea + editor.BajoLema
	lema := min(ancho, editor.AnchoLema)
	alto := ui.AltoParrafo(m.lema, lema)
	mv(m.lema, fyne.NewPos(x, y), fyne.NewSize(lema, alto))
	y += alto + editor.BajoCabecera

	// «Crear nuevo»: la tarjeta y la zona de abrir, en `1fr 2fr` desde `lg:` (la
	// ventana, con rem de 16) y una debajo de otra por debajo.
	if mover {
		m.nuevo.Colocar(x, y)
	}
	y += tema.TextoXS.Linea + editor.BajoEncabezado
	anchoAbrir := ancho
	if m.ventana >= ui.CorteLG {
		anchoAbrir = 2 * (ancho - editor.HuecoAccesos) / 3
	}
	// El p text-sm envuelve al ancho interior antes de sumar el alto de la columna.
	m.ayudaAbrir.Ancho = max(anchoAbrir-2*(bordeAbrir+rellenoAbrir), 1)
	aAbrir := m.abrir.MinSize()
	if m.ventana >= ui.CorteLG {
		col := (ancho - editor.HuecoAccesos) / 3
		mv(m.acceso, fyne.NewPos(x, y), fyne.NewSize(col, editor.MedidaAccesoNuevo()))
		mv(m.abrir, fyne.NewPos(x+col+editor.HuecoAccesos, y), fyne.NewSize(2*col, aAbrir.Height))
		y += max(editor.MedidaAccesoNuevo(), aAbrir.Height)
	} else {
		mv(m.acceso, fyne.NewPos(x, y), fyne.NewSize(ancho, editor.MedidaAccesoNuevo()))
		y += editor.MedidaAccesoNuevo() + editor.HuecoAccesos
		mv(m.abrir, fyne.NewPos(x, y), fyne.NewSize(ancho, aAbrir.Height))
		y += aAbrir.Height
	}
	if m.errorCrear != nil {
		y += tema.Espacio(2) // mt-2
		mv(m.errorCrear, fyne.NewPos(x, y), m.errorCrear.MinSize())
		y += tema.TextoSM.Linea
	}
	y += editor.BajoNuevo

	// Las peticiones de firma, con su `mb-10`, entre «Crear nuevo» y «Recientes».
	if m.peticiones != nil {
		alto := m.peticiones.MinSize().Height
		mv(m.peticiones, fyne.NewPos(x, y), fyne.NewSize(ancho, alto))
		y += alto + tema.Espacio(10)
	}

	// «Recientes»: el encabezado, la insignia y el filtro en `ml-auto w-64 max-w-[60%]`.
	fila := ui.AltoCampo(ui.CampoMD)
	if mover {
		m.reciente.Colocar(x, y+(fila-tema.TextoXS.Linea)/2)
	}
	if m.insignia != nil {
		mi := ui.MedidaInsignia(m.insignia.Texto)
		mv(m.insignia, fyne.NewPos(x+m.reciente.Ancho()+editor.HuecoEncabezado, y+(fila-mi.Height)/2), mi)
	}
	busc := min(editor.AnchoBuscador, 0.6*ancho)
	mv(m.p.filtro, fyne.NewPos(x+ancho-busc, y), fyne.NewSize(busc, fila))
	y += fila + editor.BajoEncabezado

	if m.cuerpo != nil {
		alto := m.cuerpo.MinSize().Height
		switch c := m.cuerpo.(type) {
		case *ui.Vacio:
			alto = c.MedidaVacio(ancho)
		case *fyne.Container:
			alto = 22 + 2*cuerpoCargando // la rueda de 22 con su `py-10`
		}
		mv(m.cuerpo, fyne.NewPos(x, y), fyne.NewSize(ancho, alto))
		y += alto
	} else if n := len(m.tarjetas); n > 0 {
		cols := rejillaPDF.Columnas(ancho)
		col := rejillaPDF.AnchoColumna(ancho)
		alto := editor.MedidaTesela()
		for i, c := range m.tarjetas {
			mv(c, fyne.NewPos(x+rejillaPDF.X(ancho, i%cols), y+float32(i/cols)*(alto+rejillaPDF.Hueco)), fyne.NewSize(col, alto))
		}
		filas := (n + cols - 1) / cols
		y += float32(filas)*alto + float32(filas-1)*rejillaPDF.Hueco
	}
	return y + abajoPortada
}

func (m *marcoPortada) MinSize([]fyne.CanvasObject) fyne.Size {
	if m.alto == 0 {
		m.alto = m.colocar(fyne.NewSize(max(m.ventana, 1280), 800), false)
	}
	return fyne.NewSize(editor.MinTesela+2*editor.MargenPortada, m.alto)
}

func (m *marcoPortada) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	if m.ventana != t.Width {
		m.ventana = t.Width
	}
	alto := m.colocar(t, true)
	if math.Abs(float64(alto-m.alto)) > 0.01 {
		m.alto = alto
		// El alto de la columna depende del ancho: el desplazador tiene que saberlo.
		fyne.Do(m.p.scroll.Refresh)
	}
}

// ---- La tarjeta de «Crear nuevo» ----

// accesoPDF es el botón «PDF · En blanco»: `flex items-center gap-3 self-start
// rounded-lg border p-4` con la tesela de color de `h-11 w-11 rounded-lg` y el glifo de
// 22 en blanco. Las medidas son las del acceso de la portada de documentos
// ([editor.RellenoAcceso] y compañía): en la web son las mismas clases.
type accesoPDF struct {
	ui.Pulsable
	titulo, blanco, creado string
	creando                bool
}

func nuevoAccesoPDF(titulo, blanco, creado string, creando bool, alPulsar func()) *accesoPDF {
	a := &accesoPDF{titulo: titulo, blanco: blanco, creado: creado, creando: creando}
	a.Nombre, a.AlPulsar, a.Inerte = titulo, alPulsar, creando
	a.ExtendBaseWidget(a)
	return a
}

func (a *accesoPDF) MinSize() fyne.Size {
	a.ExtendBaseWidget(a)
	return fyne.NewSize(2*tema.Borde+2*editor.RellenoAcceso+editor.LadoTeselaAcceso, editor.MedidaAccesoNuevo())
}

func (a *accesoPDF) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	blanco := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	r := &accesoPDFRenderer{
		a:      a,
		aro:    ui.NuevoAro(tema.RadioLG, ui.ConHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		tesela: canvas.NewRectangle(tema.Color("product-pdf", v)),
		glifo:  iconos.TrazoDeCuerpo(GlifoPDF, editor.GlifoAcceso, blanco),
		rueda:  ui.NuevaRuedaCon(editor.RuedaAcceso, blanco),
		titulo: ui.Texto(a.titulo, tema.TextoRaiz, ui.SemiNegrita, tema.Color("text", v)),
		sub:    ui.Texto(a.blanco, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
	}
	r.tesela.CornerRadius = tema.RadioLG
	r.Refresh()
	return r
}

type accesoPDFRenderer struct {
	a           *accesoPDF
	aro         *ui.Aro
	caja        *ui.Caja
	tesela      *canvas.Rectangle
	glifo       *iconos.Dibujo
	rueda       *ui.Rueda
	titulo, sub *canvas.Text
}

func (r *accesoPDFRenderer) MinSize() fyne.Size { return r.a.MinSize() }
func (r *accesoPDFRenderer) Destroy()           {}
func (r *accesoPDFRenderer) Objects() []fyne.CanvasObject {
	objs := append(append([]fyne.CanvasObject{}, r.caja.Objetos()...), r.tesela)
	if r.a.creando {
		objs = append(objs, r.rueda)
	} else {
		objs = append(objs, r.glifo.Objeto())
	}
	return r.aro.Antes(append(objs, r.titulo, r.sub)...)
}

func (r *accesoPDFRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	lado := editor.LadoTeselaAcceso
	x := tema.Borde + editor.RellenoAcceso
	y := (t.Height - lado) / 2
	r.tesela.Move(fyne.NewPos(x, y))
	r.tesela.Resize(fyne.NewSize(lado, lado))
	g := editor.GlifoAcceso
	r.glifo.Colocar(x+(lado-g)/2, y+(lado-g)/2, g)
	rd := editor.RuedaAcceso
	r.rueda.Move(fyne.NewPos(x+(lado-rd)/2, y+(lado-rd)/2))
	r.rueda.Resize(fyne.NewSize(rd, rd))
	tx := x + lado + editor.HuecoAcceso
	texto := tema.TextoRaiz.Linea + tema.TextoSM.Linea
	ty := (t.Height - texto) / 2
	cabe := max(t.Width-tx-tema.Borde-editor.RellenoAcceso, 0)
	ui.Truncar(r.titulo, r.a.titulo, tema.TextoRaiz, ui.SemiNegrita, cabe)
	ui.ColocarTexto(r.titulo, tema.TextoRaiz, tx, ty)
	ui.ColocarTexto(r.sub, tema.TextoSM, tx, ty+tema.TextoRaiz.Linea)
}

func (r *accesoPDFRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde := tema.Color("surface", v), tema.Color("border", v)
	if r.a.Encima() && !r.a.creando {
		fondo, borde = tema.Color("bg-subtle", v), tema.Color("border-strong", v)
	}
	// `disabled:opacity-60` mientras crea.
	o := float32(1)
	if r.a.creando {
		o = 0.6
	}
	sup := tema.Color("surface", v)
	r.caja.Pintar(opaco(fondo, o, sup), opaco(borde, o, sup))
	r.tesela.FillColor = opaco(tema.Color("product-pdf", v), o, sup)
	r.tesela.Refresh()
	r.sub.Text = r.a.blanco
	if r.a.creando {
		r.sub.Text = r.a.creado
	}
	r.titulo.Color, r.sub.Color = opaco(tema.Color("text", v), o, sup), opaco(tema.Color("text-muted", v), o, sup)
	r.titulo.Refresh()
	r.sub.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.a.Enfocado())
	r.Layout(r.a.Size())
}

// ---- La tarjeta de un reciente ----

// tarjetaPDF es un reciente: `rounded-lg border bg-surface hover:border-border-strong`,
// la banda `h-24 bg-bg-subtle` —SIN el `border-b` de la tesela de Docs (Portada.tsx no
// lo lleva): sus 84 son banda entera— con el glifo de 32 en el color del producto y,
// debajo, `p-3 gap-0.5` con el nombre sin «.pdf» en `font-semibold` y «PDF · fecha» en
// `text-sm text-text-muted`. Las medidas son las de la tesela de documentos
// ([editor.AltoBandaTesela], [editor.RellenoTesela]…); a diferencia de aquella, esta no
// lleva acciones y su nombre va en semibold.
type tarjetaPDF struct {
	ui.Pulsable
	nombre, meta string
}

func nuevaTarjetaPDF(f api.File, tipo string, loc i18n.Locale, alPulsar func()) *tarjetaPDF {
	nombre := f.Name
	if strings.HasSuffix(strings.ToLower(nombre), ".pdf") {
		nombre = nombre[:len(nombre)-4]
	}
	meta := tipo
	if !f.UpdatedAt.IsZero() {
		meta += " · " + i18n.FormatDate(loc, f.UpdatedAt.In(time.Local))
	}
	c := &tarjetaPDF{nombre: nombre, meta: meta}
	c.Nombre, c.AlPulsar = f.Name, alPulsar
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityRole es el `<a href>` de la web.
func (c *tarjetaPDF) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleLink }

func (c *tarjetaPDF) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(editor.MinTesela, editor.MedidaTesela())
}

func (c *tarjetaPDF) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tarjetaPDFRenderer{
		c:      c,
		aro:    ui.NuevoAro(tema.RadioLG, ui.SinHueco, v).Abierto(),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		banda:  canvas.NewRectangle(tema.Color("bg-subtle", v)),
		glifo:  iconos.TrazoDeCuerpo(GlifoPDF, editor.GlifoBandaTesela, tema.Color("product-pdf", v)),
		nombre: ui.Texto(c.nombre, tema.TextoRaiz, ui.SemiNegrita, tema.Color("text", v)),
		meta:   ui.Texto(c.meta, tema.TextoSM, ui.Normal, tema.Color("text-muted", v)),
	}
	r.banda.TopLeftCornerRadius = tema.RadioLG - tema.Borde
	r.banda.TopRightCornerRadius = tema.RadioLG - tema.Borde
	r.Refresh()
	return r
}

type tarjetaPDFRenderer struct {
	c            *tarjetaPDF
	aro          *ui.Aro
	caja         *ui.Caja
	banda        *canvas.Rectangle
	glifo        *iconos.Dibujo
	nombre, meta *canvas.Text
}

func (r *tarjetaPDFRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *tarjetaPDFRenderer) Destroy()           {}
func (r *tarjetaPDFRenderer) Objects() []fyne.CanvasObject {
	return r.aro.Antes(append(r.caja.Objetos(), r.banda, r.glifo.Objeto(), r.nombre, r.meta)...)
}

func (r *tarjetaPDFRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	b := tema.Borde
	banda := editor.AltoBandaTesela
	r.banda.Move(fyne.NewPos(b, b))
	r.banda.Resize(fyne.NewSize(t.Width-2*b, banda))
	g := editor.GlifoBandaTesela
	r.glifo.Colocar((t.Width-g)/2, b+(banda-g)/2, g)
	x := b + editor.RellenoTesela
	y := b + banda + editor.RellenoTesela
	cabe := max(t.Width-2*x, 0)
	ui.Truncar(r.nombre, r.c.nombre, tema.TextoRaiz, ui.SemiNegrita, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoRaiz, x, y)
	ui.Truncar(r.meta, r.c.meta, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.meta, tema.TextoSM, x, y+tema.TextoRaiz.Linea+editor.HuecoTesela)
}

func (r *tarjetaPDFRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde := tema.Color("surface", v), tema.Color("border", v)
	if r.c.Encima() {
		borde = tema.Color("border-strong", v)
	}
	r.caja.Pintar(fondo, borde)
	r.banda.FillColor = tema.Color("bg-subtle", v)
	r.banda.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.c.Enfocado())
	r.Layout(r.c.Size())
}
