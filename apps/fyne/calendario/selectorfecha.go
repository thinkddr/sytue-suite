// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El selector de fecha y hora del formulario: lo que el navegador cuelga del icono de un
// `<input type="datetime-local">` (o `date` en un evento de todo el día). No tiene CSS
// que copiar —cada navegador pinta el suyo—, así que se arma con las piezas de la
// suite: el mini-mes del lateral sobre superficie clara y, al lado, la columna de horas
// de cuarto en cuarto que se desplaza, como la de Chromium. El campo se sigue pudiendo
// teclear, y el desplegable abre en lo tecleado.
//
// Elegir un día cambia la fecha y deja la hora; elegir una hora la fija y cierra. En un
// evento de todo el día no hay horas y elegir el día cierra.
//
// ponytail: las flechas del mes mueven también el día marcado (el mini-mes solo tiene un
// cursor); el campo no cambia hasta que se elige uno.

// Medidas del desplegable.
const (
	// pasoHoras son los minutos entre dos horas de la columna.
	pasoHoras = 15
	// anchoMiniSelector es el mini-mes: siete columnas de 32 y su `px-1`.
	anchoMiniSelector = 7*32 + 2*RellenoMiniMes
	// ladoIconoCampo es el botón del icono dentro del campo, y margenIconoCampo lo que
	// lo separa del borde derecho (el `pr-3` del campo menos el aire del botón).
	ladoIconoCampo   = ui.LadoIconoSM
	margenIconoCampo = 3*0.25*tema.Rem - (ui.LadoIconoSM-16)/2
)

// conSelectorFecha envuelve un campo de fecha y hora con el icono que despliega el
// selector. `previa` es la fecha que vale si lo tecleado no se entiende.
func (f *formulario) conSelectorFecha(campo *ui.Entrada, rotulo string, previa func() time.Time) fyne.CanvasObject {
	boton := ui.NuevoBotonIcono(iconos.Calendario, rotulo, func() { f.abrirSelectorFecha(campo, previa) })
	boton.Lado = ladoIconoCampo
	f.iconosFecha = append(f.iconosFecha, boton)
	return container.New(&iconoDentro{boton: boton}, campo, boton)
}

// abrirSelectorFecha despliega el selector bajo el campo, en lo que haya tecleado.
func (f *formulario) abrirSelectorFecha(campo *ui.Entrada, previa func() time.Time) {
	c := fyne.CurrentApp().Driver().CanvasForObject(campo)
	if c == nil || f.e.SoloLectura {
		return
	}
	valor, ok := f.leerFechaHora(campo.Texto(), previa())
	if !ok {
		valor = previa()
	}
	caja := container.NewStack()
	var e *ui.Emergente
	var pintar func(cursor, valor time.Time)
	escribir := func(t time.Time, cerrar bool) {
		campo.PonerTexto(f.textoFecha(t))
		if cerrar {
			e.Cerrar()
			return
		}
		pintar(t, t)
	}
	pintar = func(cursor, valor time.Time) {
		caja.Objects = []fyne.CanvasObject{f.panelFecha(cursor, valor, escribir, func(d int) { pintar(cursor.AddDate(0, d, 0), valor) })}
		caja.Refresh()
		if e != nil {
			e.Recolocar()
		}
	}
	pintar(valor, valor)
	e = ui.MostrarEmergenteEn(c, caja, espacio3, func(ventana, tam fyne.Size) (fyne.Position, fyne.Size) {
		abs := fyne.CurrentApp().Driver().AbsolutePositionForObject(campo)
		return ui.ColocarPopover(abs, campo.Size(), tam, ventana), tam
	})
}

// textoFecha es como se escribe en el campo: fecha y hora, o solo la fecha.
func (f *formulario) textoFecha(t time.Time) string {
	t = t.In(f.p.zona())
	if f.e.TodoElDia {
		return ui.EscribirFecha(t)
	}
	return ui.EscribirFecha(t) + ", " + ui.EscribirHora(t)
}

// panelFecha es lo de dentro: el mes en `cursor` con `valor` elegido y, si hay hora, la
// columna de horas.
func (f *formulario) panelFecha(cursor, valor time.Time, escribir func(time.Time, bool), desplazar func(int)) fyne.CanvasObject {
	p := f.p
	loc := p.zona()
	valor = valor.In(loc)
	mini := NuevoMiniMes(p.locale(), cursor.In(loc))
	mini.Zona, mini.Superficie = loc, true
	mini.RotuloAnterior, mini.RotuloSiguiente = p.t("nav.prevMonth", nil), p.t("nav.nextMonth", nil)
	mini.AlElegir = func(d time.Time) {
		d = d.In(loc)
		escribir(time.Date(d.Year(), d.Month(), d.Day(), valor.Hour(), valor.Minute(), 0, 0, loc), f.e.TodoElDia)
	}
	mini.AlDesplazar = desplazar
	alto := MedidaMiniMes().Height - MargenMiniMes
	mes := container.New(&tamFijo{Tam: fyne.NewSize(anchoMiniSelector, alto)}, mini)
	if f.e.TodoElDia {
		return mes
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var horas []fyne.CanvasObject
	elegida := -1
	for m := 0; m < 24*60; m += pasoHoras {
		t := time.Date(valor.Year(), valor.Month(), valor.Day(), m/60, m%60, 0, 0, loc)
		b := ui.NuevoBoton(ui.EscribirHora(t), func() { escribir(t, true) }).Con(ui.Fantasma, ui.TallaSM)
		if t.Hour() == valor.Hour() && t.Minute() == valor.Minute() {
			b.Variante, elegida = ui.Primario, len(horas)
		}
		horas = append(horas, b)
	}
	columna := container.New(ui.Columna{}, horas...)
	desplaza := container.NewVScroll(columna)
	anchoHoras := horas[0].MinSize().Width + tema.Espacio(2)
	desplaza.SetMinSize(fyne.NewSize(anchoHoras, alto))
	// Abre con la elegida —o la más cercana— a la vista, a un tercio del alto.
	if elegida < 0 {
		elegida = (valor.Hour()*60 + valor.Minute()) / pasoHoras
	}
	desplaza.Offset = fyne.NewPos(0, max(float32(elegida)*horas[0].MinSize().Height-alto/3, 0))
	filete := canvas.NewRectangle(tema.Color("border-subtle", v))
	return container.New(&mesYHoras{}, mes, filete, desplaza)
}

// iconoDentro coloca el botón del icono DENTRO del campo, a la derecha y centrado en su
// alto, donde el navegador pone el del selector.
type iconoDentro struct{ boton *ui.BotonIcono }

func (l *iconoDentro) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (l *iconoDentro) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	lado := l.boton.MinSize()
	l.boton.Move(fyne.NewPos(t.Width-tema.Borde-margenIconoCampo-lado.Width, (t.Height-lado.Height)/2))
	l.boton.Resize(lado)
}

// tamFijo da a su hijo un tamaño fijo.
type tamFijo struct{ Tam fyne.Size }

func (l *tamFijo) MinSize([]fyne.CanvasObject) fyne.Size { return l.Tam }
func (l *tamFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// mesYHoras pone el mes, un filete vertical y la columna de horas, con `gap-2` a cada
// lado del filete.
type mesYHoras struct{}

func (mesYHoras) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, c := objs[0].MinSize(), objs[2].MinSize()
	return fyne.NewSize(a.Width+2*espacio2+tema.Borde+c.Width, max(a.Height, c.Height))
}

func (mesYHoras) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	a, c := objs[0].MinSize(), objs[2].MinSize()
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(a.Width, t.Height))
	objs[1].Move(fyne.NewPos(a.Width+espacio2, 0))
	objs[1].Resize(fyne.NewSize(tema.Borde, t.Height))
	objs[2].Move(fyne.NewPos(a.Width+2*espacio2+tema.Borde, 0))
	objs[2].Resize(fyne.NewSize(c.Width, t.Height))
}
