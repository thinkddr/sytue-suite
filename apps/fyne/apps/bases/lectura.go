// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

// paridad-web: apps/frontend/apps/bases/src/pages/base/[id].astro
// (la barra de BarraLectura.tsx, sobre la rejilla)

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// esperaBusqueda es la de la web (ESPERA_BUSQUEDA_MS): no se lanza una consulta por letra.
const esperaBusqueda = 300 * time.Millisecond

// lectura es buscar y ordenar «de un momento» (lib/lectura.ts): lo que se teclea o se
// elige encima de la vista sin guardarla. Lo resuelve el servicio (`?q=`, `?sort=`) para
// que abarque la tabla entera y no solo la ventana ya cargada.
type lectura struct {
	Q string
	// Campo por el que ordenar; vacío = el orden de la vista.
	Campo string
	Desc  bool
}

// hay dice si la lectura pide algo más que la vista.
func (l lectura) hay() bool { return l.Q != "" || l.Campo != "" }

// api son los parámetros que viajan; lo que no se ha pedido no se manda, salvo el día
// del dispositivo (`hoyLocal` de la web): lo usan los filtros relativos a hoy, y con el
// del servidor, en UTC, «hoy» sería ayer cada noche en Europa.
func (l lectura) api() api.BasesLectura {
	out := api.BasesLectura{Q: strings.TrimSpace(l.Q), Today: time.Now().Format(time.DateOnly)}
	if l.Campo != "" {
		dir := "asc"
		if l.Desc {
			dir = "desc"
		}
		out.Sort = l.Campo + ":" + dir
	}
	return out
}

// camposOrdenables: se ordena por lo que tiene un valor propio; enlaces y adjuntos
// guardan ids, no lo que se ve.
func camposOrdenables(campos []api.BasesField) []api.BasesField {
	var out []api.BasesField
	for _, c := range campos {
		if c.Kind != api.BasesKindLink && c.Kind != api.BasesKindAttachment {
			out = append(out, c)
		}
	}
	return out
}

// cambiarLectura aplica otra búsqueda u otro orden: se vuelve a la primera ventana.
func (p *Pantalla) cambiarLectura(l lectura) {
	b := p.abierta
	if b == nil {
		return
	}
	l.Q = strings.TrimSpace(l.Q)
	if l == b.lectura {
		return
	}
	b.lectura, b.vent = l, sinVentanas()
	b.rejilla.Poner(b.Campos, nil)
	b.lado.Mostrar(0, 0)
	p.Repintar()
	p.cargarRegistros("inicio")
}

// buscarYa aplica lo escrito en el buscador sin esperar (Intro, o al vencer la espera).
func (p *Pantalla) buscarYa() {
	b := p.abierta
	if b == nil || b.busqueda == nil {
		return
	}
	if b.espera != nil {
		b.espera.Stop()
		b.espera = nil
	}
	l := b.lectura
	l.Q = b.busqueda.Texto()
	p.cambiarLectura(l)
}

// nuevoBuscador es el campo de búsqueda de la tabla. Es el MISMO entre repintados: lo
// escrito y el foco no se pierden mientras llega la página.
func (p *Pantalla) nuevoBuscador(b *Base) *ui.Entrada {
	e := ui.NuevaEntrada(p.t("grid.search"))
	e.Rotulo, e.Icono, e.Busqueda = p.t("grid.search"), iconos.Buscar, true
	e.AlCambiar(func(string) {
		if b.espera != nil {
			b.espera.Stop()
		}
		b.espera = time.AfterFunc(esperaBusqueda, func() {
			enHilo(func() {
				if p.abierta == b && b.busqueda == e {
					p.buscarYa()
				}
			})
		})
	})
	e.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			p.buscarYa()
			return true
		}
		return false
	}
	return e
}

// barraLectura es BarraLectura.tsx: el buscador, que se lleva lo que sobra, «Ordenar
// por» y, con un campo elegido, el botón que da la vuelta al orden.
func (p *Pantalla) barraLectura() fyne.CanvasObject {
	b := p.abierta
	ordenables := camposOrdenables(b.Campos)
	rotulos := make([]string, 0, 1+len(ordenables))
	rotulos = append(rotulos, p.t("grid.sortView"))
	elegida := 0
	for i, c := range ordenables {
		rotulos = append(rotulos, c.Name)
		if c.ID == b.lectura.Campo {
			elegida = i + 1
		}
	}
	orden := ui.NuevoSelector(rotulos, func(i int) {
		l := b.lectura
		l.Campo = ""
		if i > 0 {
			l.Campo = ordenables[i-1].ID
		}
		p.cambiarLectura(l)
	})
	// `basis-40`: el selector no encoge con el nombre del campo elegido.
	orden.Rotulo, orden.Elegida, orden.Ancho = p.t("grid.sortBy"), elegida, 10*tema.Rem
	derecha := []fyne.CanvasObject{orden}
	if b.lectura.Campo != "" {
		rotulo, icono := p.t("grid.sortAsc"), iconos.ChevronArriba
		if b.lectura.Desc {
			rotulo, icono = p.t("grid.sortDesc"), iconos.ChevronAbajo
		}
		derecha = append(derecha, ui.NuevoBotonIcono(icono, rotulo, func() {
			l := b.lectura
			l.Desc = !l.Desc
			p.cambiarLectura(l)
		}))
	}
	// `flex-1`: el buscador se lleva lo que sobra de la línea.
	return container.New(extremos{Hueco: tema.Espacio(2), Centro: true},
		append([]fyne.CanvasObject{b.busqueda}, derecha...)...)
}
