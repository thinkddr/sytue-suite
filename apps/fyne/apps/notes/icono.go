// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Paridad web: IconPickerModal.tsx, el «Configurar icono…» del «…» de una nota. Es un
// `Modal size="sm"` con una rejilla de tres columnas (cuatro desde `sm`) y un botón por
// icono; el elegido se guarda en `data.icon` y lo pintan la tarjeta y las dos filas del
// árbol (ya lo hacían: faltaba poder elegirlo).

// IconosNota son los de ICONOS_PREDEFINIDOS EN SU ORDEN: el identificador que se guarda,
// que es también la clave del glifo en `glyphs.tsx`, y el primero, vacío, es quitarlo.
// El rótulo es `icon.<id>` (`icon.none` el vacío), como `rotuloIcono` de la web.
var IconosNota = []string{
	"", "note", "todo", "file", "link", "palette", "pin", "quote",
	"sparkles", "send", "table", "heading", "unlock", "filter",
}

// Medidas del selector, de sus clases.
const (
	// HuecoIconos es el `gap-2` de la rejilla y RellenoIconos su `p-1`.
	HuecoIconos   = 2 * 0.25 * tema.Rem
	RellenoIconos = 1 * 0.25 * tema.Rem
	// PCeldaIcono es el `p-3` de cada botón, y HuecoCeldaIcono su `gap-1.5`.
	PCeldaIcono     = 3 * 0.25 * tema.Rem
	HuecoCeldaIcono = 1.5 * 0.25 * tema.Rem
	// GlifoIcono es el `size={22}`.
	GlifoIcono = 22
)

// RotuloIcono es la clave del rótulo de un icono del selector.
func RotuloIcono(id string) string {
	if id == "" {
		return "icon.none"
	}
	return "icon." + id
}

// conIcono es `data` con el icono cambiado y lo demás intacto (`...dataActual`). Vacío
// lo QUITA, como el `icon: icono || undefined` de la web: JSON.stringify no escribe una
// clave `undefined`.
func conIcono(data json.RawMessage, icono string) (json.RawMessage, error) {
	var m map[string]json.RawMessage
	if len(data) > 0 {
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	delete(m, "icon")
	if icono != "" {
		v, err := json.Marshal(icono)
		if err != nil {
			return nil, err
		}
		m["icon"] = v
	}
	return api.CodificarDatosNotas(m)
}

// ponerIcono guarda el icono sobre el `data` que la nota tiene AHORA, como los adjuntos.
func (p *Pantalla) ponerIcono(id, icono string) error {
	var datos json.RawMessage
	hay := false
	fyne.DoAndWait(func() {
		if n, ok := p.nota(id); ok {
			datos, hay = n.Datos, true
		}
	})
	if !hay {
		return nil
	}
	nuevo, err := conIcono(datos, icono)
	if err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), "")
		return err
	}
	return p.editar(id, Parche{Datos: nuevo}, true)
}

// dialogoIcono abre el selector con el icono actual marcado.
func (p *Pantalla) dialogoIcono(n Nota) {
	v, t := p.variante(), p.tr.T
	var d *ui.Dialogo
	celdas := make([]fyne.CanvasObject, 0, len(IconosNota))
	for _, id := range IconosNota {
		celdas = append(celdas, p.celdaIcono(id, n.Icono == id, func() {
			d.Cerrar()
			lanzar(func() error { return p.ponerIcono(n.ID, id) })
		}, v))
	}
	columnas := 3 // `grid-cols-3 sm:grid-cols-4`, y `sm:` mide la VENTANA
	if p.anchoVentana() >= ui.CorteSM {
		columnas = 4
	}
	rejilla := container.New(ui.Rejilla{Fijas: columnas, Hueco: HuecoIconos}, celdas...)
	cuerpo := container.New(ui.Margen{Izq: RellenoIconos, Arr: RellenoIconos, Der: RellenoIconos, Abj: RellenoIconos}, rejilla)
	d = ui.NuevoDialogo(t("note.setIcon", nil), cuerpo).Con(ui.DialogoSM, nil)
	p.mostrar(d)
}

// celdaIcono es un botón del selector: `rounded-lg border p-3 text-sm`, el glifo de 22
// encima del rótulo (`flex-col items-center gap-1.5`). El elegido va en el primario
// (`border-primary bg-primary/10 text-primary-text font-medium`) con su `ring-1`, que es
// un pelo de fuera que no ocupa sitio.
func (p *Pantalla) celdaIcono(id string, elegido bool, alPulsar func(), v tema.Variante) fyne.CanvasObject {
	rotulo := p.tr.T(RotuloIcono(id), nil)
	tinta, peso := "text", ui.Normal
	if elegido {
		tinta, peso = "primary-text", ui.Medio
	}
	var dibujo fyne.CanvasObject
	if d := GlifoPorNombre(id); d != "" {
		dibujo = Glifo(d, GlifoIcono, tinta, v)
	} else {
		dibujo = lineaTok("—", tema.TextoXS, ui.Normal, "text-muted", v)
	}
	texto := linea(rotulo, tema.TextoSM, peso, tema.Color(tinta, v))
	contenido := container.New(&pilaCentrada{hueco: HuecoCeldaIcono}, dibujo, texto)
	o := NuevaOpcion(rotulo, contenido, alPulsar)
	o.Radio, o.PX, o.PY, o.Borde, o.Superficie = tema.RadioLG, PCeldaIcono, PCeldaIcono, true, "surface"
	if !elegido {
		return o
	}
	primario := tema.Color("primary", v)
	o.Activa, o.BordeActivo, o.FondoActivo = true, primario, velo(primario, 0.1)
	anillo := canvas.NewRectangle(velo(primario, 0))
	anillo.StrokeColor, anillo.StrokeWidth, anillo.CornerRadius = primario, 1, tema.RadioLG+0.5
	return container.New(&conAnillo{}, anillo, o)
}

// pilaCentrada apila sus hijos centrados en horizontal y el bloque entero centrado en
// vertical (`flex flex-col items-center justify-center gap-N`). El texto se recorta al
// ancho (`truncate`), así que no pide más de lo que le den.
type pilaCentrada struct{ hueco float32 }

func (l *pilaCentrada) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var alto float32
	for i, o := range objs {
		if i > 0 {
			alto += l.hueco
		}
		alto += o.MinSize().Height
	}
	return fyne.NewSize(0, alto)
}

func (l *pilaCentrada) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := (t.Height - l.MinSize(objs).Height) / 2
	for _, o := range objs {
		m := o.MinSize()
		ancho := min(m.Width, t.Width)
		o.Move(fyne.NewPos((t.Width-ancho)/2, y))
		o.Resize(fyne.NewSize(ancho, m.Height))
		y += m.Height + l.hueco
	}
}

// conAnillo pone el primero de sus hijos —el `ring-1`— medio píxel por fuera del segundo,
// que es el botón: con el trazo centrado, un trazo de 1 ahí cubre justo el píxel de
// fuera del borde.
type conAnillo struct{}

func (conAnillo) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[1].MinSize() }

func (conAnillo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(-0.5, -0.5))
	objs[0].Resize(fyne.NewSize(t.Width+1, t.Height+1))
	objs[1].Move(fyne.NewPos(0, 0))
	objs[1].Resize(t)
}
