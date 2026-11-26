// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Es `PrimerosPasos.tsx` del portal, dentro de la portada (lanzador.go lleva el
// marcador de paridad de `index.astro`, que es la pantalla).

package hub

import (
	"context"
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/autorizacion"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Primeros pasos» en la portada: la tarjeta de `PrimerosPasos.tsx`, encima de las apps.
//
// Dice lo que a ESTA persona le queda por dejar listo (verificación en dos pasos, una
// forma de recuperar la cuenta, la foto y conectar sus dispositivos), cada paso lleva a
// donde se arregla y desaparece al cumplirse. Qué pasos quedan lo decide
// `api.PrimerosPasos`, el espejo de la función de la web, con los mismos vectores.
// «Instalar Sytue One» no sale nunca aquí: ya está instalada.
//
// Medidas de las clases de la web, de fuera adentro:
//
//	<section class="mb-4"><Card>                   borde de 1 y sombra `shadow-sm`
//	  <div class="px-4 pt-3 flex items-center">    título `text-sm font-semibold` y
//	                                               «Ocultar» `ghost sm` (`max-sm:min-h-11`)
//	  <ul class="grid gap-1 p-2 sm:grid-cols-2">
//	    <a class="flex min-h-11 items-start gap-3 rounded-md p-2">
//	      icono 18 `mt-0.5` · título `text-sm font-medium` · cuerpo `mt-0.5 text-sm
//	      leading-snug` · chevron 16 `mt-0.5`
const (
	// MargenPasos es el `mb-4` de la sección: lo que la separa de lo que viene debajo.
	MargenPasos float32 = 4 * 0.25 * tema.Rem
	// MargenXCabeceraPasos y MargenSupCabeceraPasos son el `px-4 pt-3` de la cabecera.
	MargenXCabeceraPasos   float32 = 4 * 0.25 * tema.Rem
	MargenSupCabeceraPasos float32 = 3 * 0.25 * tema.Rem
	// RellenoListaPasos es el `p-2` de la lista y HuecoListaPasos su `gap-1`, entre filas
	// y entre columnas.
	RellenoListaPasos float32 = 2 * 0.25 * tema.Rem
	HuecoListaPasos   float32 = 1 * 0.25 * tema.Rem
	// RellenoPaso es el `p-2` de cada paso y HuecoPaso su `gap-3`.
	RellenoPaso float32 = 2 * 0.25 * tema.Rem
	HuecoPaso   float32 = 3 * 0.25 * tema.Rem
	// LadoIconoPaso es el `<Icon size={18}>`, LadoChevronPaso el `size={16}` y
	// DesplazaPaso el `mt-0.5` de los dos y del cuerpo.
	LadoIconoPaso   float32 = 18
	LadoChevronPaso float32 = 16
	DesplazaPaso    float32 = 0.5 * 0.25 * tema.Rem
	// AltoMinimoPaso es el `min-h-11` de cada paso y del «Ocultar» en el teléfono.
	AltoMinimoPaso float32 = 11 * 0.25 * tema.Rem
)

// EscalonCuerpoPaso es el `text-sm leading-snug` del cuerpo de un paso.
var EscalonCuerpoPaso = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.375 * tema.TextoSM.Tam}

// iconoPaso y textosPaso son ICONO y TEXTOS de PrimerosPasos.tsx. Conectar los
// dispositivos es el mismo paso que cerraba el alta: mismo texto.
var (
	iconoPaso = map[api.ClavePaso]iconos.Icono{
		api.PasoDosPasos:     iconos.Escudo,
		api.PasoRecuperacion: iconos.Llave,
		api.PasoFoto:         iconos.Camara,
		api.PasoDispositivos: iconos.Dispositivo,
		api.PasoInstalar:     iconos.Descargar,
	}
	textosPaso = map[api.ClavePaso][2]string{
		api.PasoDosPasos:     {"firstSteps.mfa.title", "firstSteps.mfa.body"},
		api.PasoRecuperacion: {"firstSteps.recovery.title", "firstSteps.recovery.body"},
		api.PasoFoto:         {"firstSteps.photo.title", "firstSteps.photo.body"},
		api.PasoDispositivos: {"ob.next.devices.title", "ob.next.devices.body"},
		api.PasoInstalar:     {"firstSteps.install.title", "firstSteps.install.body"},
	}
)

// TarjetaPasos es la tarjeta. Se coloca con su alto a un ancho (`AltoPara`), que depende
// también de la VENTANA: desde `sm` los pasos van en dos columnas y en el teléfono el
// «Ocultar» tiene el alto de un dedo.
type TarjetaPasos struct {
	widget.BaseWidget

	T     *i18n.Translator
	Pasos []api.PasoPendiente
	// Ventana es el ancho de la ventana, el de los cortes de la web.
	Ventana float32
	// AlOcultar quita la tarjeta para siempre (para esta persona en esta organización).
	AlOcultar func()
	// AlAbrir lleva a donde se arregla el paso.
	AlAbrir func(api.PasoPendiente)

	tarjeta *ui.Tarjeta
	titulo  *canvas.Text
	ocultar *ui.Boton
	filas   []*ui.FilaPulsable
	pasos   []*contenidoPaso
}

var _ fyne.Widget = (*TarjetaPasos)(nil)

// NuevaTarjetaPasos crea la tarjeta con los pasos que quedan.
func NuevaTarjetaPasos(t *i18n.Translator, pasos []api.PasoPendiente, ventana float32) *TarjetaPasos {
	c := &TarjetaPasos{T: t, Pasos: pasos, Ventana: ventana}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el título de la sección (`aria-labelledby`).
func (c *TarjetaPasos) AccessibilityLabel() string { return c.T.T("firstSteps.title", nil) }

// AccessibilityRole: la sección agrupa.
func (c *TarjetaPasos) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

func (c *TarjetaPasos) dosColumnas() bool { return c.Ventana == 0 || c.Ventana >= ui.CorteSM }

// altoBoton es lo que mide «Ocultar»: `sm`, y en el teléfono `min-h-11`.
func (c *TarjetaPasos) altoBoton() float32 {
	alto := ui.MedidaBoton(c.T.T("firstSteps.hide", nil), ui.TallaSM, 0).Height
	if !c.dosColumnas() {
		alto = max(alto, AltoMinimoPaso)
	}
	return alto
}

// altoCabecera es la fila del título y el botón (`items-center`).
func (c *TarjetaPasos) altoCabecera() float32 { return max(tema.TextoSM.Linea, c.altoBoton()) }

// columnas reparte el interior de la lista: cuántas y de qué ancho.
func (c *TarjetaPasos) columnas(ancho float32) (n int, col float32) {
	interior := max(ancho-2*tema.Borde-2*RellenoListaPasos, 0)
	if c.dosColumnas() {
		return 2, (interior - HuecoListaPasos) / 2
	}
	return 1, interior
}

// altoFila es lo que ocupa un paso a ese ancho de columna, con su relleno.
func (c *TarjetaPasos) altoFila(p api.PasoPendiente, col float32) float32 {
	return max(2*RellenoPaso+altoContenidoPaso(c.T, p, col-2*RellenoPaso), AltoMinimoPaso)
}

// filasDeLaRejilla son los altos de las filas de la rejilla: en dos columnas, el mayor de
// cada pareja (la fila de un `grid` mide lo que su celda más alta).
func (c *TarjetaPasos) filasDeLaRejilla(ancho float32) []float32 {
	n, col := c.columnas(ancho)
	var filas []float32
	for i := 0; i < len(c.Pasos); i += n {
		alto := float32(0)
		for j := i; j < min(i+n, len(c.Pasos)); j++ {
			alto = max(alto, c.altoFila(c.Pasos[j], col))
		}
		filas = append(filas, alto)
	}
	return filas
}

// AltoPara es lo que ocupa la tarjeta a ese ancho, sin el `mb-4` de fuera. Es la ÚNICA
// cuenta: Layout recorre lo mismo.
func (c *TarjetaPasos) AltoPara(ancho float32) float32 {
	filas := c.filasDeLaRejilla(ancho)
	alto := 2*tema.Borde + MargenSupCabeceraPasos + c.altoCabecera() + 2*RellenoListaPasos
	for i, f := range filas {
		if i > 0 {
			alto += HuecoListaPasos
		}
		alto += f
	}
	return alto
}

// MinSize declara solo el alto a su ancho: el ancho lo pone la portada.
func (c *TarjetaPasos) MinSize() fyne.Size {
	return fyne.NewSize(0, c.AltoPara(c.Size().Width))
}

// CreateRenderer compone la tarjeta: la superficie de Card, la cabecera y un paso por fila.
func (c *TarjetaPasos) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.tarjeta = ui.NuevaTarjeta("", nil)
	c.tarjeta.SinRelleno = true
	c.titulo = ui.Texto(c.T.T("firstSteps.title", nil), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	c.ocultar = ui.NuevoBoton(c.T.T("firstSteps.hide", nil), func() {
		if c.AlOcultar != nil {
			c.AlOcultar()
		}
	}).Con(ui.Fantasma, ui.TallaSM)
	c.ocultar.Rotulo = c.T.T("firstSteps.hideAria", nil)
	objetos := make([]fyne.CanvasObject, 0, 3+len(c.Pasos))
	objetos = append(objetos, c.tarjeta, c.titulo, c.ocultar)
	c.filas, c.pasos = nil, nil
	for _, p := range c.Pasos {
		paso := p
		contenido := nuevoContenidoPaso(c.T, paso, v)
		fila := ui.NuevaFilaPulsable(contenido.titulo.Texto, contenido, func() {
			if c.AlAbrir != nil {
				c.AlAbrir(paso)
			}
		})
		c.pasos = append(c.pasos, contenido)
		c.filas = append(c.filas, fila)
		objetos = append(objetos, fila)
	}
	return &tarjetaPasosRenderer{c: c, objetos: objetos}
}

type tarjetaPasosRenderer struct {
	c       *TarjetaPasos
	objetos []fyne.CanvasObject
}

func (r *tarjetaPasosRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *tarjetaPasosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tarjetaPasosRenderer) Destroy()                     {}

// Layout coloca la cabecera y la rejilla de pasos. Cada paso mide lo suyo y va arriba de
// su celda: en la web el `<a>` es un bloque dentro del `<li>` que se estira, así que el
// fondo del `:hover` no crece hasta el vecino.
func (r *tarjetaPasosRenderer) Layout(t fyne.Size) {
	c := r.c
	c.tarjeta.Move(fyne.NewPos(0, 0))
	c.tarjeta.Resize(t)

	cab := c.altoCabecera()
	y := tema.Borde + MargenSupCabeceraPasos
	ui.ColocarTexto(c.titulo, tema.TextoSM, tema.Borde+MargenXCabeceraPasos, y+(cab-tema.TextoSM.Linea)/2)
	boton := ui.MedidaBoton(c.ocultar.Texto, ui.TallaSM, 0)
	boton.Height = c.altoBoton()
	c.ocultar.Move(fyne.NewPos(t.Width-tema.Borde-MargenXCabeceraPasos-boton.Width, y+(cab-boton.Height)/2))
	c.ocultar.Resize(boton)

	y += cab + RellenoListaPasos
	x0 := tema.Borde + RellenoListaPasos
	n, col := c.columnas(t.Width)
	for i, alto := range c.filasDeLaRejilla(t.Width) {
		for j := i * n; j < min((i+1)*n, len(c.filas)); j++ {
			x := x0 + float32(j-i*n)*(col+HuecoListaPasos)
			c.filas[j].Move(fyne.NewPos(x, y))
			c.filas[j].Resize(fyne.NewSize(col, c.altoFila(c.Pasos[j], col)))
		}
		y += alto + HuecoListaPasos
	}
}

// Refresh repinta con el tema de ahora.
func (r *tarjetaPasosRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.titulo.Color = tema.Color("text", v)
	c.titulo.Refresh()
	c.tarjeta.Refresh()
	c.ocultar.Refresh()
	for _, p := range c.pasos {
		p.Refresh()
	}
	for _, f := range c.filas {
		f.Refresh()
	}
	r.Layout(c.Size())
}

// ---- Un paso ----

// contenidoPaso es lo de dentro del `<a>`: icono, título con su cuerpo y chevron. La fila
// pulsable pone el relleno, el `:hover` y el foco.
type contenidoPaso struct {
	widget.BaseWidget

	clave   api.ClavePaso
	icono   *iconos.Dibujo
	chevron *iconos.Dibujo
	titulo  *ui.Parrafo
	cuerpo  *ui.Parrafo
}

func nuevoContenidoPaso(t *i18n.Translator, p api.PasoPendiente, v tema.Variante) *contenidoPaso {
	textos := textosPaso[p.Clave]
	c := &contenidoPaso{
		clave:   p.Clave,
		icono:   iconos.Trazo(iconoPaso[p.Clave], LadoIconoPaso, "text-muted", v),
		chevron: iconos.Trazo(iconos.ChevronDerecha, LadoChevronPaso, "text-muted", v),
		titulo:  ui.NuevoParrafo(t.T(textos[0], nil), tema.TextoSM, ui.Medio, "text", v),
		cuerpo:  ui.NuevoParrafo(t.T(textos[1], nil), EscalonCuerpoPaso, ui.Normal, "text-muted", v),
	}
	c.ExtendBaseWidget(c)
	return c
}

// cabeTextoPaso es lo que le queda al texto: el contenido menos los dos iconos y sus huecos.
func cabeTextoPaso(ancho float32) float32 {
	return max(ancho-LadoIconoPaso-LadoChevronPaso-2*HuecoPaso, 0)
}

// altoContenidoPaso es lo que ocupa el contenido de un paso a ese ancho, sin relleno.
func altoContenidoPaso(t *i18n.Translator, p api.PasoPendiente, ancho float32) float32 {
	textos := textosPaso[p.Clave]
	cabe := cabeTextoPaso(ancho)
	titulo := float32(len(ui.Repartir(t.T(textos[0], nil), tema.TextoSM, ui.Medio, cabe))) * tema.TextoSM.Linea
	cuerpo := float32(len(ui.Repartir(t.T(textos[1], nil), EscalonCuerpoPaso, ui.Normal, cabe))) * EscalonCuerpoPaso.Linea
	return max(titulo+DesplazaPaso+cuerpo, DesplazaPaso+LadoIconoPaso)
}

// AltoPara es lo que ocupa a ese ancho: la fila pulsable lo pregunta con `ui.AltoA`.
func (c *contenidoPaso) AltoPara(ancho float32) float32 {
	cabe := cabeTextoPaso(ancho)
	return max(ui.AltoParrafo(c.titulo, cabe)+DesplazaPaso+ui.AltoParrafo(c.cuerpo, cabe), DesplazaPaso+LadoIconoPaso)
}

// MinSize no declara ancho: el contenido ocupa el de su fila.
func (c *contenidoPaso) MinSize() fyne.Size { return fyne.NewSize(0, c.AltoPara(c.Size().Width)) }

func (c *contenidoPaso) CreateRenderer() fyne.WidgetRenderer {
	return &contenidoPasoRenderer{c: c, objetos: []fyne.CanvasObject{c.icono.Objeto(), c.titulo, c.cuerpo, c.chevron.Objeto()}}
}

type contenidoPasoRenderer struct {
	c       *contenidoPaso
	objetos []fyne.CanvasObject
}

func (r *contenidoPasoRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *contenidoPasoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *contenidoPasoRenderer) Destroy()                     {}

// Layout es el `flex items-start gap-3`: icono, textos que crecen y chevron.
func (r *contenidoPasoRenderer) Layout(t fyne.Size) {
	c := r.c
	cabe := cabeTextoPaso(t.Width)
	c.icono.Colocar(0, DesplazaPaso, LadoIconoPaso)
	x := LadoIconoPaso + HuecoPaso
	alto := ui.AltoParrafo(c.titulo, cabe)
	c.titulo.Move(fyne.NewPos(x, 0))
	c.titulo.Resize(fyne.NewSize(cabe, alto))
	c.cuerpo.Move(fyne.NewPos(x, alto+DesplazaPaso))
	c.cuerpo.Resize(fyne.NewSize(cabe, ui.AltoParrafo(c.cuerpo, cabe)))
	c.chevron.Colocar(t.Width-LadoChevronPaso, DesplazaPaso, LadoChevronPaso)
}

// Refresh repinta las tintas con el tema de ahora.
func (r *contenidoPasoRenderer) Refresh() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c.icono.TenirCon(tema.Color("text-muted", v))
	c.chevron.TenirCon(tema.Color("text-muted", v))
	c.titulo.Tinta = tema.Color("text", v)
	c.cuerpo.Tinta = tema.Color("text-muted", v)
	c.titulo.Refresh()
	c.cuerpo.Refresh()
	r.Layout(c.Size())
}

// ---- En la Vista ----

// ConPrimerosPasos conecta la tarjeta con Sytue ID: `idBase` es su raíz (la de «Mi
// cuenta») y `cargar` trae los pasos que quedan, con `hay` en falso si no contestó a
// nada. Sin esto, la portada es la de siempre.
func (v *Vista) ConPrimerosPasos(idBase string, cargar func(context.Context) ([]api.PasoPendiente, bool)) *Vista {
	v.idBase, v.primerosPasos = idBase, cargar
	return v
}

// cargarPrimerosPasos pide los pasos de quien ha entrado, salvo que los ocultara en esta
// organización. Lo que no llega deja la portada sin tarjeta, como en la web.
func (v *Vista) cargarPrimerosPasos(ctx context.Context, r autorizacion.Reclamos) {
	huella := ajustes.HuellaPrimerosPasos(r.Iss, r.Sub, r.Tenant)
	var pasos []api.PasoPendiente
	if v.primerosPasos != nil && huella != "" && !v.pasosOcultos(huella) {
		pasos, _ = v.primerosPasos(ctx)
	}
	v.enUI(func() {
		v.huellaPasos = huella
		v.lanzador.Pasos = pasos
	})
}

// ocultarPrimerosPasos es «Ocultar»: la tarjeta se va ya y no vuelve para esta persona
// en esta organización, en ninguna app de la suite de este equipo. Si no se puede
// guardar, se va igual: volverá en la próxima visita, que no es grave.
func (v *Vista) ocultarPrimerosPasos() {
	huella := v.huellaPasos
	v.enUI(func() { v.lanzador.Pasos = nil })
	go func() {
		if err := v.ocultarPasos(huella); err != nil {
			slog.Warn("hub: no se ha podido recordar «Ocultar» de los primeros pasos", "err", err)
		}
	}()
}

// abrirPaso lleva a donde se arregla: la sección de «Mi cuenta», que solo es web (como
// `ui/barracuenta.go:miCuenta`), o la pantalla del Hub que es la página del portal.
func (v *Vista) abrirPaso(p api.PasoPendiente) {
	if p.Destino == "cuenta" {
		base := v.idBase
		if base == "" {
			base = api.ServiceURL(v.apex, "id")
		}
		v.AbrirURL(base + "/account" + p.Ruta)
		return
	}
	if d, ok := DestinoDeRuta(p.Ruta); ok {
		v.Ir(d)
	}
}
