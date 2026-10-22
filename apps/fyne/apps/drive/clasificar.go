// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Clasificar
//
// El `Modal` de `labelItem` en FileBrowser.tsx, talla sm y sin pie: el nombre del
// fichero, y una fila por etiqueta de la taxonomía de la organización que dice lo que
// IMPLICA —una que bloquea el enlace público no es lo mismo que una informativa—, porque
// el efecto no se deduce del nombre y quien clasifica tiene que saberlo ANTES. Si ya
// lleva una, «Quitar la clasificación» al final, en rojo.
//
// La taxonomía se pide UNA vez por pantalla y se guarda: la web la tiene en un estado del
// explorador. Un fallo al leerla deja la lista VACÍA, no un error: el diálogo dirá que la
// organización todavía no ha definido clasificaciones, que es lo que le sirve a quien lo
// abre.

// Medidas de una fila: `px-3 py-2`, la columna de la marca `w-4` con el icono de 14 un
// `mt-0.5` más abajo, y `gap-2.5` hasta el texto.
const (
	MargenXEtiqueta  float32 = 3 * 0.25 * tema.Rem
	MargenYEtiqueta  float32 = 2 * 0.25 * tema.Rem
	ColumnaMarca     float32 = 4 * 0.25 * tema.Rem
	LadoMarca        float32 = 14
	BajaMarca        float32 = 0.5 * 0.25 * tema.Rem
	HuecoEtiqueta    float32 = 2.5 * 0.25 * tema.Rem
	SeparaNombreEtiq float32 = 3 * 0.25 * tema.Rem // `mb-3` del nombre del fichero
	// AltoCargaEtiquetas es el `flex justify-center py-6` con la rueda de 20.
	AltoCargaEtiquetas float32 = 20 + 2*6*0.25*tema.Rem
)

// dialogoClasificar es el estado del diálogo.
type dialogoClasificar struct {
	p       *Pantalla
	el      Elemento
	d       *ui.Dialogo
	cuerpo  *fyne.Container
	ocupado bool
	ancho   float32
}

// pedirClasificar abre el diálogo de un fichero y lo devuelve (para el guardián).
func (p *Pantalla) pedirClasificar(el Elemento) *dialogoClasificar {
	if p.win == nil || p.svc == nil || el.Carpeta || el.ID == "" {
		return nil
	}
	dc := &dialogoClasificar{p: p, el: el, cuerpo: container.NewStack()}
	dc.ancho = AnchoCuerpoDialogo(ui.DialogoSM, p.explor.Ventana)
	dc.d = ui.NuevoDialogo(p.tr.T("label.title", nil), dc.cuerpo).Con(ui.DialogoSM, nil)
	dc.d.RotuloCerrar = p.tr.T("ui.close", nil)
	dc.pintar()
	dc.d.Mostrar(p.win.Canvas())
	if p.taxonomia == nil {
		p.enSegundoPlano(func() {
			ctx, cancelar := p.ctx()
			defer cancelar()
			ls, err := p.svc.SensitivityLabels(ctx)
			if err != nil || ls == nil {
				ls = []api.EtiquetaDrive{}
			}
			enHilo(func() { p.taxonomia = ls; dc.pintar() })
		})
	}
	return dc
}

// aplicar pone la etiqueta (o la quita, con "") y recarga la vista para que la fila la
// enseñe.
func (dc *dialogoClasificar) aplicar(id string) {
	if dc.ocupado || dc.p.explor.SinRed() {
		return
	}
	dc.ocupado = true
	dc.pintar()
	nombre := ""
	for _, l := range dc.p.taxonomia {
		if l.ID == id {
			nombre = l.Name
		}
	}
	dc.p.enSegundoPlano(func() {
		ctx, cancelar := dc.p.ctx()
		defer cancelar()
		err := dc.p.svc.SetLabel(ctx, dc.el.ID, id)
		enHilo(func() {
			dc.ocupado = false
			switch {
			case err != nil:
				dc.p.avisar(ui.AvisoPeligro, dc.p.tr.T("label.failed", nil), "")
				dc.pintar()
				return
			case id == "":
				dc.p.avisar(ui.AvisoExito, dc.p.tr.T("label.cleared", nil), "")
			default:
				dc.p.avisar(ui.AvisoExito, dc.p.tr.T("label.applied", i18n.Vars{"name": nombre}), "")
			}
			dc.d.Cerrar()
		})
		if err == nil {
			dc.p.recargarVista()
		}
	})
}

// pintar rehace el cuerpo: el nombre, y la rueda, el vacío o la lista.
func (dc *dialogoClasificar) pintar() {
	v := dc.p.variante()
	t := func(k string) string { return dc.p.tr.T(k, nil) }
	nombre := ui.Texto(dc.el.Nombre, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
	ui.Truncar(nombre, dc.el.Nombre, tema.TextoSM, ui.Normal, dc.ancho)
	var dentro fyne.CanvasObject
	switch ls := dc.p.taxonomia; {
	case ls == nil:
		dentro = container.New(&altoFijo{alto: AltoCargaEtiquetas}, container.NewCenter(ui.NuevaRueda(20, "text", v)))
	case len(ls) == 0:
		x := ui.NuevoVacio(t("label.emptyTitle"), t("label.emptyBody"))
		// `<Icon name="lock" size={28} />`: un glifo, que el estado vacío tiñe de `text-subtle`.
		x.Glifo = iconos.Trazo(iconos.Candado, 28, "text-subtle", v)
		dentro = vacioEnLinea(x)
	default:
		filas := make([]fyne.CanvasObject, 0, len(ls)+1)
		actual := ""
		if dc.el.Etiqueta != nil {
			actual = dc.el.Etiqueta.Name
		}
		for _, l := range ls {
			clave := "label.informational"
			if l.BlockExternalShare {
				clave = "label.blocks"
			}
			id := l.ID
			f := nuevaFilaEtiqueta(l.Name, t(clave), func() { dc.aplicar(id) })
			f.marcada, f.Inerte = actual == l.Name, dc.ocupado
			filas = append(filas, f)
		}
		separas := make([]float32, 0, len(filas)+1)
		for i := range filas {
			sep := tema.Espacio(1) // `gap-1`
			if i == 0 {
				sep = 0
			}
			separas = append(separas, sep)
		}
		if dc.el.Etiqueta != nil {
			quitar := nuevaFilaEtiqueta(t("label.remove"), "", func() { dc.aplicar("") })
			quitar.peligro, quitar.Inerte = true, dc.ocupado
			filas = append(filas, quitar)
			separas = append(separas, tema.Espacio(1)+tema.Espacio(1)) // `gap-1` y su `mt-1`
		}
		dentro = container.New(&pilaSeparada{separas: separas}, filas...)
	}
	dc.cuerpo.Objects = []fyne.CanvasObject{container.New(&pilaSeparada{separas: []float32{0, SeparaNombreEtiq}},
		lineaTexto(nombre, tema.TextoSM), dentro)}
	dc.cuerpo.Refresh()
	dc.d.Recolocar()
}

// filaEtiqueta es una opción de la lista: `flex items-start gap-2.5 rounded-md px-3 py-2
// text-left hover:bg-bg-subtle focus-visible:bg-bg-subtle disabled:opacity-60`, con la
// marca de la actual, el nombre en `text-sm font-medium` y lo que implica en `text-xs`.
// La de quitar es la misma caja con el rótulo en `danger-text` y sin marca ni detalle.
type filaEtiqueta struct {
	ui.Pulsable
	nombre, detalle string
	marcada         bool
	peligro         bool
}

func nuevaFilaEtiqueta(nombre, detalle string, alPulsar func()) *filaEtiqueta {
	f := &filaEtiqueta{nombre: nombre, detalle: detalle}
	f.Nombre, f.AlPulsar = nombre, alPulsar
	if detalle != "" {
		f.Nombre = nombre + ", " + detalle
	}
	f.ExtendBaseWidget(f)
	return f
}

// MinSize: el alto de las líneas con su relleno; el ancho lo pone la lista.
func (f *filaEtiqueta) MinSize() fyne.Size {
	alto := 2*MargenYEtiqueta + tema.TextoSM.Linea
	if f.detalle != "" {
		alto += tema.TextoXS.Linea
	}
	if f.peligro && ui.EsTactil() {
		alto = max(alto, ui.SueloTactilBarra) // «Quitar la etiqueta», `pointer-coarse:min-h-11`
	}
	return fyne.NewSize(0, alto)
}

func (f *filaEtiqueta) CreateRenderer() fyne.WidgetRenderer {
	r := &filaEtiquetaRenderer{f: f, velo: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD)}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	peso := ui.Medio
	r.nombre = ui.Texto(f.nombre, tema.TextoSM, peso, tema.Color("text", v))
	r.objetos = append(r.velo.Objetos(), r.nombre)
	if f.detalle != "" {
		r.detalle = ui.Texto(f.detalle, tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
		r.objetos = append(r.objetos, r.detalle)
	}
	if f.marcada {
		r.marca = iconos.Trazo(iconos.Tic, LadoMarca, "primary-text", v)
		r.objetos = append(r.objetos, r.marca.Objeto())
	}
	r.Refresh()
	return r
}

type filaEtiquetaRenderer struct {
	f               *filaEtiqueta
	velo            *ui.Caja
	nombre, detalle *canvas.Text
	marca           *iconos.Dibujo
	objetos         []fyne.CanvasObject
}

func (r *filaEtiquetaRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaEtiquetaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaEtiquetaRenderer) Destroy()                     {}

func (r *filaEtiquetaRenderer) Layout(t fyne.Size) {
	r.velo.Redimensionar(t)
	x := MargenXEtiqueta
	if !r.f.peligro { // la de quitar no lleva la columna de la marca
		if r.marca != nil {
			r.marca.Colocar(x, MargenYEtiqueta+BajaMarca, LadoMarca)
		}
		x += ColumnaMarca + HuecoEtiqueta
	}
	cabe := max(t.Width-x-MargenXEtiqueta, 0)
	ui.Truncar(r.nombre, r.f.nombre, tema.TextoSM, ui.Medio, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoSM, x, MargenYEtiqueta)
	if r.detalle != nil {
		ui.Truncar(r.detalle, r.f.detalle, tema.TextoXS, ui.Normal, cabe)
		ui.ColocarTexto(r.detalle, tema.TextoXS, x, MargenYEtiqueta+tema.TextoSM.Linea)
	}
}

// Refresh: el velo `bg-subtle` al pasar o con el foco (sin anillo, como en la web) y el
// `opacity-60` mientras se aplica otra.
func (r *filaEtiquetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	alfa := func(c color.NRGBA) color.NRGBA {
		if r.f.Inerte {
			c.A = uint8(float64(c.A)*0.6 + 0.5)
		}
		return c
	}
	velo := color.NRGBA{}
	if !r.f.Inerte && (r.f.Encima() || r.f.Enfocado()) {
		velo = tema.Color("bg-subtle", v)
	}
	r.velo.Pintar(velo, color.NRGBA{})
	tinta := "text"
	if r.f.peligro {
		tinta = "danger-text"
	}
	r.nombre.Color = alfa(tema.Color(tinta, v))
	if r.detalle != nil {
		r.detalle.Color = alfa(tema.Color("text-muted", v))
	}
	if r.marca != nil {
		r.marca.Tenir("primary-text", v)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}
