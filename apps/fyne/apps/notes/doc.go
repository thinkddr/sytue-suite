// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor/docus"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Paridad web: la VISTA PREVIA de una nota `doc` —el `<iframe>` de NoteCard.tsx, `h-64
// w-full rounded-md border bg-white`, con el documento de Sytue Docs dentro—.
//
// En la web el iframe carga el export HTML del documento desde el BFF, aislado. Aquí no
// hay iframe que aislar: el contenido se pide a Docs con la misma sesión (el registro
// `doc:html`, lo que el export vuelca) y lo pinta `docus.Lectura`, el maquetador del
// propio editor de Docs, sin cursor ni edición. Se DESPLAZA dentro de su caja, como el
// iframe.
//
// ponytail: el iframe pinta el export SIN hoja de estilos, con la del navegador (Times,
// `margin: 8px`); aquí va con la letra y las medidas del editor de Docs, que es como la
// persona lo escribió. Solo se conserva el `margin: 8px` del `<body>`.

// docRemoto es cómo va el HTML de un documento: cargando, listo o fallido.
type docRemoto struct {
	html  string
	err   error
	listo bool
}

// Medidas de la vista previa.
const (
	// AltoVistaDoc es el `h-64` del iframe.
	AltoVistaDoc = 64 * 0.25 * tema.Rem
	// MargenVistaDoc es el `margin: 8px` del `<body>` que el export no quita.
	MargenVistaDoc float32 = 8
)

// docDe es el de ese documento tal como vaya. La primera vez lo pide.
func (p *Pantalla) docDe(id string) docRemoto {
	if r, ok := p.docs[id]; ok || p.Docs == nil {
		return r
	}
	if p.docs == nil {
		p.docs = map[string]docRemoto{}
	}
	p.docs[id] = docRemoto{}
	go p.cargarDoc(id)
	return docRemoto{}
}

// cargarDoc lo pide a Docs y, al tenerlo, repinta.
func (p *Pantalla) cargarDoc(id string) {
	ctx, cancelar := p.ctx()
	defer cancelar()
	c, err := p.Docs.Content(ctx, id, api.DocContentKey(api.DocText))
	enHilo(func() {
		if p.docs == nil {
			p.docs = map[string]docRemoto{}
		}
		p.docs[id] = docRemoto{html: c.Value, err: err, listo: true}
		p.Repintar()
	})
}

// vistaDoc es la caja blanca con el documento dentro, desplazable. Mientras llega, o si
// no llega (sin permiso, borrado), la caja vacía, como el iframe con su error.
func (t *Tarjeta) vistaDoc(id string, v tema.Variante) fyne.CanvasObject {
	fondo := canvas.NewRectangle(color.White) // `bg-white`: el papel, también en oscuro
	fondo.StrokeColor, fondo.StrokeWidth, fondo.CornerRadius = tema.Color("border", v), tema.Borde, tema.RadioMD
	piezas := []fyne.CanvasObject{fondo}
	if t.Documento != nil && id != "" {
		if r := t.Documento(id); r.listo && r.err == nil {
			m := MargenVistaDoc
			s := container.NewVScroll(container.New(ui.Margen{Izq: m, Der: m, Arr: m, Abj: m}, docus.NuevaLectura(r.html)))
			piezas = append(piezas, s)
		}
	}
	return container.New(&vistaDocLayout{}, piezas...)
}

// vistaDocLayout es la caja de alto fijo, con el contenido dentro del borde.
type vistaDocLayout struct{}

func (vistaDocLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, AltoVistaDoc) }

func (vistaDocLayout) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
	for _, o := range objs[1:] {
		o.Move(fyne.NewPos(tema.Borde, tema.Borde))
		o.Resize(fyne.NewSize(t.Width-2*tema.Borde, t.Height-2*tema.Borde))
	}
}
