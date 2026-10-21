// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// «Ir a» (Ctrl/⌘+P o +O), el AbrirRapido.tsx de la web: escribir el nombre de un cuaderno
// o de una nota, flechas e Intro, sin tocar el ratón. Los cuadernos y las notas del
// cuaderno abierto salen al instante; las de los demás, de la búsqueda del servicio (solo
// las indexables, las mismas que encuentra el buscador).

// OpcionIrA es una fila de «Ir a».
type OpcionIrA struct {
	Cuaderno bool
	ID       string
	// EnCuaderno es el cuaderno de una nota (el propio, en un cuaderno).
	EnCuaderno string
	Titulo     string
	Detalle    string
}

// Los topes de AbrirRapido.tsx.
const (
	maxIrA          = 8
	minimoRemotoIrA = 2
	esperaIrA       = 200 * time.Millisecond
)

// opcionesIrA son las filas para una consulta: los cuadernos, luego las notas del
// cuaderno abierto y al final las que trae la búsqueda, sin repetir.
func opcionesIrA(q string, cuadernos []Cuaderno, notas []Nota, remotas []api.NoteHit, sinTitulo string) []OpcionIrA {
	nombre := map[string]string{}
	var out []OpcionIrA
	for _, c := range cuadernos {
		nombre[c.ID] = c.Titulo
	}
	for _, c := range cuadernos {
		if len(out) < maxIrA && buscar.Contiene(c.Titulo, q) {
			out = append(out, OpcionIrA{Cuaderno: true, ID: c.ID, EnCuaderno: c.ID, Titulo: c.Titulo})
		}
	}
	vistas := map[string]bool{}
	locales := 0
	for _, n := range notas {
		titulo := cmp.Or(n.Titulo, principioDoc(n.Doc, 60), sinTitulo)
		if locales == maxIrA || (n.Cifrada && n.Contenido == "") || !buscar.Contiene(titulo, q) {
			continue
		}
		vistas[n.ID] = true
		locales++
		out = append(out, OpcionIrA{ID: n.ID, EnCuaderno: n.Cuaderno, Titulo: titulo, Detalle: nombre[n.Cuaderno]})
	}
	remotos := 0
	for _, h := range remotas {
		if remotos == maxIrA || vistas[h.ID] {
			continue
		}
		remotos++
		out = append(out, OpcionIrA{ID: h.ID, EnCuaderno: h.NotebookID, Titulo: cmp.Or(h.Title, sinTitulo), Detalle: h.Notebook})
	}
	return out
}

// abrirIrA abre el diálogo.
func (p *Pantalla) abrirIrA() {
	if p.win == nil {
		return
	}
	v, t := p.variante(), p.tr.T
	campo := ui.NuevaEntrada(t("quickOpen.placeholder", nil))
	campo.Rotulo = t("quickOpen.placeholder", nil)
	lista := container.NewStack()
	var (
		d       *ui.Dialogo
		q       string
		idx     int
		remotas []api.NoteHit
		vuelta  int
		reloj   *time.Timer
	)
	filas := func() []OpcionIrA {
		return opcionesIrA(strings.TrimSpace(q), p.cuadernos, p.notas, remotas, t("note.new", nil))
	}
	elegir := func(o OpcionIrA) {
		if reloj != nil {
			reloj.Stop()
		}
		vuelta++
		d.Cerrar()
		if o.Cuaderno {
			for _, c := range p.cuadernos {
				if c.ID == o.ID {
					p.vista = VistaCuaderno
					p.abrirCuaderno(c)
				}
			}
			return
		}
		p.abrirResultado(api.NoteHit{ID: o.ID, NotebookID: o.EnCuaderno})
	}
	var pintar func()
	pintar = func() {
		ops := filas()
		idx = max(min(idx, len(ops)-1), 0)
		lista.Objects = []fyne.CanvasObject{p.filasIrA(ops, idx, q, elegir, func(i int) { idx = i; pintar() }, v)}
		lista.Refresh()
		if d != nil {
			d.Recolocar()
		}
	}
	campo.AlCambiar(func(s string) {
		q, idx = s, 0
		if reloj != nil {
			reloj.Stop()
		}
		vuelta++
		remotas = nil
		consulta := strings.TrimSpace(s)
		if len([]rune(consulta)) >= minimoRemotoIrA && p.svc != nil && p.hayRed() {
			mia := vuelta
			reloj = time.AfterFunc(esperaIrA, func() {
				ctx, cancelar := p.ctx()
				defer cancelar()
				r, err := p.svc.Search(ctx, consulta, maxIrA, false)
				fyne.Do(func() {
					if mia != vuelta || err != nil {
						return
					}
					remotas = r
					pintar()
				})
			})
		}
		pintar()
	})
	campo.AlTeclear = func(k *fyne.KeyEvent) bool {
		switch k.Name {
		case fyne.KeyDown:
			idx = min(idx+1, len(filas())-1)
		case fyne.KeyUp:
			idx = max(idx-1, 0)
		case fyne.KeyReturn, fyne.KeyEnter:
			if ops := filas(); idx < len(ops) {
				elegir(ops[idx])
			}
			return true
		default:
			return false
		}
		pintar()
		return true
	}
	pintar()
	cuerpo := container.New(ui.Columna{Hueco: tema.Espacio(2)}, campo, lista,
		lineaTok(t("quickOpen.hint", nil), tema.TextoXS, ui.Normal, "text-subtle", v))
	d = ui.NuevoDialogo(t("quickOpen.title", nil), cuerpo).Con(ui.DialogoSM, nil)
	p.mostrar(d)
	p.win.Canvas().Focus(campo)
}

// filasIrA son las filas, con el rótulo de su grupo al cambiar de tipo: `rounded-md border
// p-1`, filas `px-2 py-1.5 text-sm gap-2.5`, la activa en `bg-selected`.
func (p *Pantalla) filasIrA(ops []OpcionIrA, activa int, q string, elegir func(OpcionIrA), encima func(int), v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	if len(ops) == 0 {
		return lineaTok(t("quickOpen.empty", nil), tema.TextoSM, ui.Normal, "text-muted", v)
	}
	var filas []fyne.CanvasObject
	for i, o := range ops {
		if i == 0 || ops[i-1].Cuaderno != o.Cuaderno {
			clave := "quickOpen.notes"
			if o.Cuaderno {
				clave = "quickOpen.notebooks"
			}
			filas = append(filas, container.New(ui.Margen{Izq: tema.Espacio(2), Arr: tema.Espacio(1.5), Abj: tema.Espacio(0.5)},
				lineaTok(t(clave, nil), tema.TextoXS, ui.Normal, "text-subtle", v)))
		}
		glifo := NotaD
		if o.Cuaderno {
			glifo = CuadernoD
		}
		piezas := []fyne.CanvasObject{
			Glifo(glifo, 15, "text-muted", v),
			ui.NuevoParrafo(o.Titulo, tema.TextoSM, ui.Normal, "text", v).Recortado(1).Resaltando(q, colorResalte(v)),
		}
		if o.Detalle != "" {
			piezas = append(piezas, lineaTok(o.Detalle, tema.TextoXS, ui.Normal, "text-subtle", v))
		}
		op := NuevaOpcion(o.Titulo, container.New(&filaElastica{hueco: tema.Espacio(2.5), elastico: 1}, piezas...), func() { elegir(o) })
		op.PX, op.PY, op.Radio, op.Fila = tema.Espacio(2), tema.Espacio(1.5), tema.Forma("radius-control"), true
		if i == activa {
			op.Activa, op.FondoActivo = true, tema.Color("selected", v)
		}
		op.AlCambiarEstado(func() {
			if op.Encima() && i != activa {
				encima(i)
			}
		})
		filas = append(filas, op)
	}
	marco := canvas.NewRectangle(tema.Color("surface-raised", v))
	marco.StrokeColor, marco.StrokeWidth, marco.CornerRadius = tema.Color("border", v), tema.Borde, tema.RadioMD
	return container.NewStack(marco,
		container.New(ui.Margen{Izq: tema.Espacio(1), Der: tema.Espacio(1), Arr: tema.Espacio(1), Abj: tema.Espacio(1)},
			container.New(ui.Columna{}, filas...)))
}
