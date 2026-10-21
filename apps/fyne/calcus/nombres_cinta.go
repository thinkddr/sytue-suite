// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El Administrador de nombres (`NameManager.tsx`): el panel que cuelga de su botón en
// «Datos», con los nombres del libro —editar y borrar— y el formulario de uno nuevo, que
// nace con la selección como referencia.

// anchoPanelNombres es el `w-80` del panel.
var anchoPanelNombres = tema.Espacio(80)

// formNombre es lo que se está escribiendo; prev es el nombre que se edita (nil: uno nuevo).
type formNombre struct {
	nombre, ref, hoja, comentario string
	prev                          *Nombre
}

// botonNombres es el botón del Administrador de nombres.
func (lz *Lienzo) botonNombres(c *editor.Contexto) *editor.BotonCinta {
	var b *editor.BotonCinta
	b = editor.NuevoBotonCinta(iconos.EdNombreDefinido, c.Traducir("names.title"), func() {
		lz.abrirNombres(c, b, lz.formNombreNuevo())
	})
	b.Inerte = c.SoloLectura
	return b
}

// formNombreNuevo es el formulario vacío con la selección como referencia
// («Hoja1!$A$1:$B$5»).
func (lz *Lienzo) formNombreNuevo() *formNombre {
	return &formNombre{ref: NormalizarRefNombre(rangoTexto(lz.Rejilla), lz.Libro.Hoja())}
}

// abrirNombres pinta el panel. Se vuelve a llamar —cerrando el anterior— cada vez que
// cambia la lista o lo que se edita.
//
//nolint:gocognit // un panel con su lista y su formulario, como `NameManagerMenu`
func (lz *Lienzo) abrirNombres(c *editor.Contexto, disparador *editor.BotonCinta, f *formNombre) {
	var em *ui.Emergente
	rehacer := func(con *formNombre) {
		em.Cerrar()
		lz.abrirNombres(c, disparador, con)
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	filas := []fyne.CanvasObject{ui.NuevoParrafo(c.Traducir("names.title"), tema.TextoXS, ui.Medio, "text", v)}

	if nombres := lz.Libro.Nombres(); len(nombres) > 0 {
		lista := make([]fyne.CanvasObject, 0, len(nombres))
		for _, n := range nombres {
			ambito := n.Sheet
			if ambito == "" {
				ambito = c.Traducir("names.scope.book")
			}
			editar := editor.NuevoBotonCinta(iconos.EdLapiz, c.Traducir("names.editNamed", i18n.Vars{"name": n.Name}), func() {
				rehacer(&formNombre{nombre: n.Name, ref: n.Ref, hoja: n.Sheet, comentario: n.Comment, prev: &n})
			})
			borrar := editor.NuevoBotonCinta(iconos.EdPapelera, c.Traducir("names.deleteNamed", i18n.Vars{"name": n.Name}), func() {
				b, _ := json.Marshal(PeticionNombre{Name: n.Name, Sheet: n.Sheet})
				c.Mandar(CmdNombreQuitar, string(b))
				if f.prev != nil && *f.prev == n {
					rehacer(lz.formNombreNuevo())
					return
				}
				rehacer(f)
			})
			texto := container.New(ui.Columna{},
				ui.NuevoParrafo(n.Name+" · "+ambito, tema.TextoXS, ui.Medio, "text", v).Recortado(1),
				ui.NuevoParrafo("="+n.Ref, tema.TextoXS, ui.Normal, "text-muted", v).Recortado(1))
			lista = append(lista, container.New(fila{hueco: tema.Espacio(1), elastico: 0}, texto,
				container.New(ladoFijo{ladoBotonRegla}, editar), container.New(ladoFijo{ladoBotonRegla}, borrar)))
		}
		filas = append(filas, conMargen(topeAlto(tema.Espacio(48), container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, lista...))))
	} else {
		filas = append(filas, ui.NuevoParrafo(c.Traducir("names.empty"), tema.Texto2XS, ui.Normal, "text-subtle", v))
	}

	titulo := c.Traducir("common.newName")
	if f.prev != nil {
		titulo = c.Traducir("names.editing", i18n.Vars{"name": f.prev.Name})
	}
	filas = append(filas, overline(titulo))

	etiquetado := func(clave string, o fyne.CanvasObject) fyne.CanvasObject {
		return container.New(ui.Columna{Hueco: tema.Espacio(0.5)},
			ui.NuevoParrafo(c.Traducir(clave), tema.Texto2XS, ui.Normal, "text-subtle", v), o)
	}
	fallo := ui.NuevoParrafo("", tema.Texto2XS, ui.Normal, "danger-text", v)
	guardar := botonPop("", false, nil)
	guardar.Texto = c.Traducir("action.add")
	if f.prev != nil {
		guardar.Texto = c.Traducir("action.save")
	}
	draft := func() Nombre { return Nombre{Name: f.nombre, Ref: f.ref, Sheet: f.hoja} }
	// Un nombre vacío todavía no es un error que enseñar: es que no se ha empezado.
	validar := func() {
		codigo := ""
		if f.nombre != "" {
			codigo = lz.Libro.ErrorDeNombre(draft(), f.prev)
		}
		fallo.Texto = ""
		if codigo != "" {
			fallo.Texto = c.Traducir(ClaveErrorNombre[codigo])
		}
		fallo.Refresh()
		guardar.Inerte = f.nombre == "" || codigo != "" || c.SoloLectura
		guardar.Refresh()
	}
	salvar := func() {
		if guardar.Inerte {
			return
		}
		req := PeticionNombre{Name: f.nombre, Ref: f.ref, Sheet: f.hoja, Comment: f.comentario}
		if f.prev != nil {
			req.Prev = &struct {
				Name  string `json:"name"`
				Sheet string `json:"sheet,omitempty"`
			}{f.prev.Name, f.prev.Sheet}
		}
		b, _ := json.Marshal(req)
		c.Mandar(CmdNombrePoner, string(b))
		rehacer(lz.formNombreNuevo())
	}
	guardar.AlPulsar = salvar
	campo := func(marca, valor string, poner func(string)) *ui.Entrada {
		e := ui.NuevaEntrada(marca).ConPropia(campoPanel)
		e.PonerTexto(valor)
		e.AlCambiar(func(s string) { poner(s); validar() })
		e.AlTeclear = func(k *fyne.KeyEvent) bool {
			if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
				salvar()
				return true
			}
			return false
		}
		return e
	}
	hojas := lz.Libro.Hojas()
	ambitos := append(make([]editor.Opcion, 0, 1+len(hojas)), editor.Opcion{Valor: "", Rotulo: c.Traducir("names.scope.book")})
	for _, h := range hojas {
		ambitos = append(ambitos, editor.Opcion{Valor: h, Rotulo: h})
	}
	filas = append(filas,
		etiquetado("common.name", campo("", f.nombre, func(s string) { f.nombre = s })),
		etiquetado("names.ref", campo("Hoja1!$A$1:$A$10", f.ref, func(s string) { f.ref = s })),
		etiquetado("names.scope", editor.NuevoSelectorCinta(c.Traducir("names.scope"), anchoPanelNombres-2*(rellenoPanel+tema.Borde),
			ambitos, f.hoja, func(s string) { f.hoja = s; rehacer(f) })),
		etiquetado("names.comment", campo("", f.comentario, func(s string) { f.comentario = s })),
		fallo,
	)
	pie := []fyne.CanvasObject{guardar}
	if f.prev != nil {
		pie = append(pie, botonPop(c.Traducir("action.cancel"), false, func() { rehacer(lz.formNombreNuevo()) }))
	}
	filas = append(filas, container.New(fila{hueco: tema.Espacio(1.5), elastico: 0}, pie...))
	validar()
	em = abrirEmergente(disparador, panel{ancho: anchoPanelNombres, hueco: tema.Espacio(1.5), filas: filas}.objeto())
}
