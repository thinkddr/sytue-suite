// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La galería de plantillas de «Crear nuevo» (`GaleriaPlantillas.tsx` de la web): bajo los
// accesos «En blanco», el selector del tema con que se crea y dos filas que se desplazan
// de lado —las de la organización, con la versión a elegir si hay más de una, y las de
// Sytue—. La lista la da el servicio (`GET /templates`); elegir una crea el documento CON
// su contenido en el servidor, así que aquí no se siembra nada.
//
// ponytail: la miniatura es el glifo del tipo sobre su tono, no el esquema dibujado de la
// web (diez dibujos a mano). Techo: se ve igual una carta que un informe. Se releva
// portando `ESQUEMAS` a trazos.

// ClaveDeTema es el nombre traducido de cada tema del catálogo común
// (`NOMBRE_DE_TEMA` de packages/office/src/editor/temas-nombres.ts).
var ClaveDeTema = map[string]string{
	"papel":   "slides.theme.paper",
	"carbon":  "slides.theme.charcoal",
	"pizarra": "slides.theme.slate",
	"arena":   "slides.theme.sand",
	"menta":   "slides.theme.mint",
	"cielo":   "slides.theme.sky",
}

// OpcionTema es un tema con que se puede crear o aplicar: su id y su rótulo ya traducido.
type OpcionTema struct{ ID, Rotulo string }

// OpcionesDeTema son los temas que ofrece el servicio: la marca de la organización
// primero, si tiene, y luego el catálogo.
func OpcionesDeTema(t func(string, i18n.Vars) string, d api.DocThemes) []OpcionTema {
	if len(d.Themes) == 0 {
		return nil
	}
	var out []OpcionTema
	if d.Brand != nil {
		out = append(out, OpcionTema{api.IDTemaMarca, t("theme.brand", i18n.Vars{"name": d.Brand.Name, "version": max(d.Brand.Version, 1)})})
	}
	for _, x := range d.Themes {
		r := x.ID
		if k, ok := ClaveDeTema[x.ID]; ok {
			r = t(k, nil)
		}
		out = append(out, OpcionTema{x.ID, r})
	}
	return out
}

// CargarPlantillas pide la galería y los temas. Lo que falle se deja vacío: los accesos
// «En blanco» no dependen de ella.
func (p *Portada) CargarPlantillas() {
	if p.Svc == nil || !p.ConPlantillas || !p.hayRed() {
		return
	}
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		g, errG := p.Svc.Templates(ctx, string(p.Loc))
		th, errT := p.Svc.Themes(ctx)
		fyne.Do(func() {
			deTipo := func(l []api.DocTemplate) []api.DocTemplate {
				var out []api.DocTemplate
				for _, x := range l {
					if x.Kind == p.Tipo.Kind() {
						out = append(out, x)
					}
				}
				return out
			}
			if errG == nil {
				p.Plantillas, p.PlantillasOrg = deTipo(g.Templates), deTipo(g.OrgTemplates)
			}
			if errT == nil {
				p.Temas, p.Marca, p.PuedeMarca = OpcionesDeTema(p.t2(), th), th.Brand, th.CanManage
				p.MarcaVersiones = th.Versions
			}
			p.Repintar()
		})
	})
}

// CrearDesde da de alta un documento con el contenido de una plantilla —`version` solo
// llega con las de la organización y solo si no es la vigente— y lo abre.
func (p *Portada) CrearDesde(pl api.DocTemplate, version string) {
	if p.Svc == nil || p.Creando || !p.hayRed() {
		return
	}
	p.Creando, p.CreandoClave = true, pl.Key
	p.Repintar()
	p.lanzar(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		d, err := p.Svc.CreateFromTemplate(ctx, api.DocNuevo{
			Template: pl.Key, Locale: string(p.Loc), TemplateVersion: version, Theme: p.Tema,
		})
		fyne.Do(func() {
			p.Creando, p.CreandoClave = false, ""
			if err != nil || d.ID == "" {
				if p.Avisos != nil {
					p.Avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro,
						p.t("create.error.title"), p.t("create.error.description")))
				}
				p.Repintar()
				return
			}
			p.Repintar()
			if p.AlAbrir != nil {
				p.AlAbrir(d)
			}
		})
	})
}

// ---- piezas ----

// Medidas de la galería: `w-40` la tarjeta, `h-24` la miniatura y `p-2.5` el cuerpo.
var (
	AnchoCromoPlantilla = tema.Espacio(40)
	AltoMiniatura       = tema.Espacio(24)
	RellenoCromo        = tema.Espacio(2.5)
	HuecoCromos         = tema.Espacio(3)
	HuecoFilaGaleria    = tema.Espacio(5)
	BajoEncabezadoFila  = tema.Espacio(2)
)

// CromoPlantilla es una tarjeta de la galería: miniatura, nombre y descripción.
type CromoPlantilla struct {
	activable

	Plantilla api.DocTemplate
	Tipo      Tipo
	// Creando es `aria-busy`: la descripción dice «Creando…».
	Creando bool
	creado  string
}

// NuevoCromoPlantilla crea la tarjeta; `creado` es el texto de «Creando…», ya traducido.
func NuevoCromoPlantilla(pl api.DocTemplate, creado string, alPulsar func()) *CromoPlantilla {
	c := &CromoPlantilla{Plantilla: pl, Tipo: TipoDe(pl.Kind), creado: creado}
	c.Rotulo, c.AlPulsar = pl.Name, alPulsar
	c.ExtendBaseWidget(c)
	return c
}

// MedidaCromoPlantilla es lo que ocupa: la miniatura con su raya, y el cuerpo con el
// nombre y dos líneas de descripción, dentro del borde.
func MedidaCromoPlantilla() fyne.Size {
	return fyne.NewSize(AnchoCromoPlantilla,
		2*tema.Borde+AltoMiniatura+tema.Borde+2*RellenoCromo+tema.TextoSM.Linea+tema.Espacio(0.5)+2*tema.TextoXS.Linea)
}

// MinSize es MedidaCromoPlantilla.
func (c *CromoPlantilla) MinSize() fyne.Size { c.ExtendBaseWidget(c); return MedidaCromoPlantilla() }

func (c *CromoPlantilla) descripcion() string {
	if c.Creando {
		return c.creado
	}
	return c.Plantilla.Description
}

// CreateRenderer compone aro, caja, miniatura con el glifo del tipo, nombre y descripción.
func (c *CromoPlantilla) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cromoRenderer{
		c:      c,
		aro:    ui.NuevoAro(tema.RadioLG, ui.ConHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		glifo:  iconos.TrazoDeRuta(c.Tipo.Glifo(), 32, color.NRGBA{}),
		nombre: ui.Texto(c.Plantilla.Name, tema.TextoSM, ui.Medio, color.NRGBA{}),
	}
	for range 2 {
		r.desc = append(r.desc, ui.Texto("", tema.TextoXS, ui.Normal, color.NRGBA{}))
	}
	// Las esquinas de arriba, redondeadas como la caja (`overflow-hidden` en la web): con
	// las cuatro rectas la miniatura asomaba por fuera del borde (visto en ventana real).
	r.fondo.TopLeftCornerRadius, r.fondo.TopRightCornerRadius = tema.RadioLG-tema.Borde, tema.RadioLG-tema.Borde
	r.objetos = r.aro.Antes(r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.fondo, r.glifo.Objeto(), r.nombre, r.desc[0], r.desc[1])
	r.Refresh()
	return r
}

type cromoRenderer struct {
	c       *CromoPlantilla
	aro     *ui.Aro
	caja    *ui.Caja
	fondo   *canvas.Rectangle
	glifo   *iconos.Dibujo
	nombre  *canvas.Text
	desc    []*canvas.Text
	objetos []fyne.CanvasObject
}

func (r *cromoRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *cromoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *cromoRenderer) Destroy()                     {}

func (r *cromoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	// La miniatura, a todo el ancho dentro del borde.
	r.fondo.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.fondo.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), AltoMiniatura))
	r.glifo.Colocar((t.Width-32)/2, tema.Borde+(AltoMiniatura-32)/2, 32)
	x := tema.Borde + RellenoCromo
	cabe := max(t.Width-2*x, 0)
	y := 2*tema.Borde + AltoMiniatura + RellenoCromo
	ui.Truncar(r.nombre, r.c.Plantilla.Name, tema.TextoSM, ui.Medio, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoSM, x, y)
	y += tema.TextoSM.Linea + tema.Espacio(0.5)
	lineas := ui.Repartir(r.c.descripcion(), tema.TextoXS, ui.Normal, cabe)
	for i, l := range r.desc {
		txt := ""
		if i < len(lineas) {
			txt = lineas[i]
			if i == len(r.desc)-1 && len(lineas) > len(r.desc) {
				txt += "…" // el resto no cabe en dos líneas (`line-clamp-2`)
			}
		}
		ui.Truncar(l, txt, tema.TextoXS, ui.Normal, cabe)
		ui.ColocarTexto(l, tema.TextoXS, x, y)
		y += tema.TextoXS.Linea
	}
}

func (r *cromoRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	sup := tema.Color("surface", v)
	fondo, borde := sup, tema.Color("border", v)
	if c.Encima() && !c.Inerte {
		fondo, borde = tema.Color("bg-subtle", v), tema.Color("border-strong", v)
	}
	o := float32(1)
	if c.Inerte && !c.Creando {
		o = 0.6
	}
	r.caja.Pintar(atenuarSobre(fondo, sup, o), atenuarSobre(borde, sup, o))
	r.fondo.FillColor = atenuarSobre(tema.Color("bg-subtle", v), sup, o)
	r.nombre.Color = atenuarSobre(tema.Color("text", v), sup, o)
	for _, l := range r.desc {
		l.Color = atenuarSobre(tema.Color("text-muted", v), sup, o)
	}
	r.glifo.TenirCon(atenuarSobre(tema.Color(c.Tipo.TonoProducto(), v), sup, o))
	r.aro.Pintar(v)
	r.aro.Ver(c.Enfocado())
	r.fondo.Refresh()
	r.Layout(c.Size())
}

// ---- la galería entera ----

// encabezadoFila es el `sytue-overline` de cada fila de la galería, como widget.
type encabezadoFila struct {
	widget.BaseWidget
	texto string
}

func nuevoEncabezadoFila(texto string) *encabezadoFila {
	e := &encabezadoFila{texto: texto}
	e.ExtendBaseWidget(e)
	return e
}

func (e *encabezadoFila) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &encabezadoRenderer{e: e, v: ui.NuevoVersal(e.texto, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))}
	r.Layout(e.Size())
	return r
}

type encabezadoRenderer struct {
	e *encabezadoFila
	v *ui.Versal
}

func (r *encabezadoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.v.Ancho(), tema.TextoXS.Linea)
}
func (r *encabezadoRenderer) Objects() []fyne.CanvasObject { return r.v.Objetos() }
func (r *encabezadoRenderer) Destroy()                     {}
func (r *encabezadoRenderer) Layout(fyne.Size)             { r.v.Colocar(0, 0) }
func (r *encabezadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.v.Pintar(tema.Color("text-subtle", v))
}

// columnaCromo apila una tarjeta y, si la hay, su selector de versión, a un ancho fijo.
type columnaCromo struct{}

func (columnaCromo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var alto float32
	for i, o := range objs {
		if i > 0 {
			alto += tema.Espacio(1)
		}
		alto += o.MinSize().Height
	}
	return fyne.NewSize(AnchoCromoPlantilla, alto)
}

func (columnaCromo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var y float32
	for _, o := range objs {
		h := o.MinSize().Height
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, h))
		y += h + tema.Espacio(1)
	}
}

// selectorTema es «Tema del documento nuevo»: el selector con «Como en la plantilla» y los
// temas del servicio.
func (p *Portada) selectorTema(t func(string, i18n.Vars) string, bloqueado bool) fyne.CanvasObject {
	opciones := append(make([]string, 0, 1+len(p.Temas)), t("list.themes.none", nil))
	elegida := 0
	for i, o := range p.Temas {
		opciones = append(opciones, o.Rotulo)
		if o.ID == p.Tema {
			elegida = i + 1
		}
	}
	sel := ui.NuevoSelector(opciones, func(i int) {
		p.Tema = ""
		if i > 0 {
			p.Tema = p.Temas[i-1].ID
		}
	})
	sel.Talla, sel.Elegida, sel.Rotulo = ui.CampoSM, elegida, t("list.themes.label", nil)
	sel.Ancho = tema.Espacio(56) // sin ancho medía lo mínimo y recortaba el valor a «C…» (visto en ventana real)
	sel.Inerte(bloqueado)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	etiqueta := ui.Texto(t("list.themes.label", nil), tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	return container.NewHBox(etiqueta, sel)
}

// selectorVersion es el de la versión de una plantilla de la organización con más de una;
// la vigente es la primera.
func (p *Portada) selectorVersion(t func(string, i18n.Vars) string, pl api.DocTemplate, bloqueado bool) *ui.Selector {
	nombres := make([]string, len(pl.Versions))
	sel := 0
	for i, ver := range pl.Versions {
		clave := "list.templates.versionNo"
		if i == 0 {
			clave = "list.templates.versionCurrent"
		}
		nombres[i] = t(clave, i18n.Vars{"no": ver.No})
		if ver.ID == p.versionPlantilla[pl.Key] {
			sel = i
		}
	}
	s := ui.NuevoSelector(nombres, func(i int) {
		if p.versionPlantilla == nil {
			p.versionPlantilla = map[string]string{}
		}
		p.versionPlantilla[pl.Key] = pl.Versions[i].ID
	})
	s.Talla, s.Elegida = ui.CampoSM, sel
	s.Rotulo = t("list.templates.version", i18n.Vars{"name": pl.Name})
	s.Inerte(bloqueado)
	return s
}

// cromo es la tarjeta de una plantilla con, debajo, su selector de versión si lo tiene.
func (p *Portada) cromo(t func(string, i18n.Vars) string, pl api.DocTemplate, bloqueado bool) fyne.CanvasObject {
	c := NuevoCromoPlantilla(pl, t("list.new.creating", nil), func() {
		v := ""
		// `version` solo viaja si no es la vigente: sin tocar el selector, el servicio ya copia esa.
		if id := p.versionPlantilla[pl.Key]; id != "" && len(pl.Versions) > 0 && id != pl.Versions[0].ID {
			v = id
		}
		p.CrearDesde(pl, v)
	})
	c.Creando, c.Inerte = p.Creando && p.CreandoClave == pl.Key, bloqueado
	col := []fyne.CanvasObject{c}
	if len(pl.Versions) > 1 {
		col = append(col, p.selectorVersion(t, pl, bloqueado))
	}
	return container.New(columnaCromo{}, col...)
}

// filaGaleria es un encabezado y, debajo, la fila de tarjetas que se desplaza de lado.
func (p *Portada) filaGaleria(t func(string, i18n.Vars) string, titulo string, lista []api.DocTemplate, bloqueado bool) fyne.CanvasObject {
	cromos := make([]fyne.CanvasObject, 0, len(lista))
	for _, pl := range lista {
		cromos = append(cromos, p.cromo(t, pl, bloqueado))
	}
	desplazable := ui.NuevoDesplazaLado(container.New(ui.Fila{Hueco: HuecoCromos}, cromos...), 0, func() fyne.Scrollable {
		return p.vista.scroll
	})
	return container.New(ui.Columna{Hueco: BajoEncabezadoFila}, nuevoEncabezadoFila(titulo), desplazable)
}

// galeria construye la galería con el estado de la portada, o nil si no hay nada que
// ofrecer (sin plantillas la web no pinta ni el selector de tema).
func (p *Portada) galeria() fyne.CanvasObject {
	if len(p.Plantillas) == 0 && len(p.PlantillasOrg) == 0 {
		return nil
	}
	t := p.t2()
	bloqueado := p.Creando || !p.hayRed()
	var filas []fyne.CanvasObject
	if len(p.Temas) > 0 {
		filas = append(filas, p.selectorTema(t, bloqueado))
	}
	if len(p.PlantillasOrg) > 0 {
		filas = append(filas, p.filaGaleria(t, t("list.templates.org", nil), p.PlantillasOrg, bloqueado))
	}
	if len(p.Plantillas) > 0 {
		filas = append(filas, p.filaGaleria(t, t("list.templates.heading", nil), p.Plantillas, bloqueado))
	}
	return container.New(ui.Columna{Hueco: HuecoFilaGaleria}, filas...)
}

// MedidaGaleria es lo que ocupa una galería ya construida a ese ancho; 0 si no hay.
func MedidaGaleria(g fyne.CanvasObject) float32 {
	if g == nil {
		return 0
	}
	return g.MinSize().Height
}
