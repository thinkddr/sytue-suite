// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La ficha de un registro: todos sus campos en un formulario, uno debajo de otro.
//
// Rellenar una tabla celda a celda obliga a ir de lado por la rejilla, y en un teléfono
// eso es una columna a la vista y un doble toque por valor. La ficha pide cada campo con
// su control (fecha, casillas, opciones) y lo guarda todo de una vez. Se abre con el
// botón de cada fila y, al añadir un registro, sola: añadir es rellenar.

// fichaRegistro es lo que se lleva escrito en la ficha de un registro.
type fichaRegistro struct {
	p *Pantalla
	b *Base
	r api.BasesRecord
	// n es el número de la fila (desde 1), que da nombre al diálogo.
	n           int
	nuevo       bool
	puedeEditar bool

	d *ui.Dialogo
	// Los controles, por id de campo. Son los MISMOS entre un rehacer y el siguiente:
	// avisar de un valor que no encaja no puede borrar lo ya escrito.
	entradas   map[string]*ui.Entrada
	casillas   map[string]*ui.Eleccion
	selectores map[string]*ui.Selector
	// opcionesDe son las de cada selector, en su orden ("" es la primera, vaciar).
	opcionesDe map[string][]string
	varios     map[string]map[string]bool
	invalidos  map[string]bool
	// subiendo y avisoAdjuntos son de los campos de adjuntos: si hay una subida en
	// curso, y lo último que ha pasado (un fallo, dónde ha quedado una descarga).
	subiendo      bool
	avisoAdjuntos string
}

// abrirRegistro abre la ficha de un registro. `nuevo` es que se acaba de añadir.
func (p *Pantalla) abrirRegistro(r api.BasesRecord, nuevo bool) {
	b := p.abierta
	if b == nil || len(b.Campos) == 0 {
		return
	}
	n := b.vent.primera*pagina + 1
	for i, x := range b.vent.registros() {
		if x.ID == r.ID {
			n += i
			break
		}
	}
	f := &fichaRegistro{
		p: p, b: b, r: r, n: n, nuevo: nuevo, puedeEditar: puedeEscribir(b.rol()),
		entradas: map[string]*ui.Entrada{}, casillas: map[string]*ui.Eleccion{},
		selectores: map[string]*ui.Selector{}, opcionesDe: map[string][]string{},
		varios: map[string]map[string]bool{}, invalidos: map[string]bool{},
	}
	f.armarControles()
	f.d = ui.NuevoDialogo(p.t("grid.row", i18n.Vars{"n": n}), container.New(ui.Columna{})).Con(ui.DialogoMD, f.pie())
	f.d.RotuloCerrar = p.t("ui.close")
	f.d.AlCerrar = f.cerrar
	p.ficha = f
	for _, c := range b.Campos { // el foco, en el primer campo que se escribe
		if e, ok := f.entradas[c.ID]; ok {
			f.d.Foco = e
			break
		}
	}
	f.rehacer()
	f.d.Mostrar(p.ventana.Canvas())
}

// cerrar cierra la ficha: lo que se suelte después ya no va a este registro.
func (f *fichaRegistro) cerrar() {
	f.d.Cerrar()
	if f.p.ficha == f {
		f.p.ficha = nil
	}
}

// editable dice si el campo se escribe con «Guardar»: lo calculado no, y los adjuntos
// tampoco (se suben y se borran en el momento, grupoAdjuntos).
func (f *fichaRegistro) editable(c api.BasesField) bool {
	return f.puedeEditar && c.Kind != api.BasesKindFormula && c.Kind != api.BasesKindAttachment
}

// armarControles crea el control de cada campo que se puede cambiar, con su valor.
func (f *fichaRegistro) armarControles() {
	for _, c := range f.b.Campos {
		if !f.editable(c) {
			continue
		}
		switch c.Kind {
		case api.BasesKindCheckbox:
			f.casillas[c.ID] = ui.NuevaCasilla(c.Name, nil)
			f.casillas[c.ID].Marcada = f.r.Data[c.ID] == true
		case api.BasesKindSelect:
			opciones := append([]string{""}, c.Choices...)
			actual, _ := f.r.Data[c.ID].(string)
			// Un valor que ya no es una opción se sigue ofreciendo: abrir y guardar la
			// ficha no puede borrarlo sin que nadie lo haya tocado.
			if actual != "" && !slices.Contains(opciones, actual) {
				opciones = append(opciones, actual)
			}
			rotulos := slices.Clone(opciones)
			rotulos[0] = "—"
			s := ui.NuevoSelector(rotulos, nil)
			s.Rotulo, s.Elegida = c.Name, max(slices.Index(opciones, actual), 0)
			f.selectores[c.ID], f.opcionesDe[c.ID] = s, opciones
		case api.BasesKindMultiselect, api.BasesKindLink:
			marcados := map[string]bool{}
			elegidos := cadenasDe(f.r.Data[c.ID])
			if c.Kind == api.BasesKindLink {
				elegidos = f.r.Links[c.ID]
			}
			for _, id := range elegidos {
				marcados[id] = true
			}
			f.varios[c.ID] = marcados
		default:
			e := ui.NuevaEntrada("")
			if c.Kind == api.BasesKindDate {
				e = ui.CampoFecha(ui.CampoMD)
			}
			e.Rotulo = c.Name
			e.PonerTexto(textoParaEditar(c, f.r.Data[c.ID]))
			e.AlCambiar(func(string) {
				if f.invalidos[c.ID] {
					delete(f.invalidos, c.ID)
					e.Invalida = false
					e.Refresh()
				}
			})
			e.AlTeclear = func(k *fyne.KeyEvent) bool {
				if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
					return false
				}
				f.guardar(false)
				return true
			}
			f.entradas[c.ID] = e
		}
	}
}

// opcionesVarios son las que se marcan en una selección múltiple o en un enlace.
func (f *fichaRegistro) opcionesVarios(c api.BasesField) []opcion {
	if c.Kind == api.BasesKindLink {
		return f.b.opciones[c.LinkTableID]
	}
	out := make([]opcion, len(c.Choices))
	for i, o := range c.Choices {
		out[i] = opcion{o, o}
	}
	return out
}

// rehacer vuelve a armar el cuerpo con los controles de siempre y el aviso de ahora.
func (f *fichaRegistro) rehacer() {
	p := f.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := p.anchoDialogo(560)
	hijos := make([]fyne.CanvasObject, 0, len(f.b.Campos))
	for _, c := range f.b.Campos {
		hijos = append(hijos, f.grupo(c, v, ancho))
	}
	// Borrar va al final del cuerpo y no en el pie: ahí, junto a «Guardar», un toque
	// de más se lleva el registro.
	if f.puedeEditar && !f.nuevo {
		borrar := ui.NuevoBoton(p.t("grid.deleteRecord", i18n.Vars{"n": f.n}), func() {
			f.cerrar()
			p.borrarRegistro(f.r)
		}).Con(ui.Fantasma, ui.TallaSM).ConIcono(iconos.Papelera, 14)
		hijos = append(hijos, container.New(ui.Fila{}, borrar))
	}
	f.d.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...))
}

// grupo es el rótulo de un campo y su control (o su valor, si no se cambia).
func (f *fichaRegistro) grupo(c api.BasesField, v tema.Variante, ancho float32) fyne.CanvasObject {
	p := f.p
	rotulo := c.Name
	errorDe := ""
	if f.invalidos[c.ID] {
		errorDe = p.t("grid.invalidValue")
	}
	if e, ok := f.entradas[c.ID]; ok {
		return ui.GrupoCampo(rotulo, false, e, "", errorDe)
	}
	if s, ok := f.selectores[c.ID]; ok {
		return ui.GrupoCampo(rotulo, false, s, "", "")
	}
	if x, ok := f.casillas[c.ID]; ok {
		return x // la casilla ya lleva el nombre del campo
	}
	if c.Kind == api.BasesKindAttachment {
		return f.grupoAdjuntos(c, v, ancho)
	}
	if marcados, ok := f.varios[c.ID]; ok {
		opciones := f.opcionesVarios(c)
		if len(opciones) == 0 {
			return ui.GrupoCampo(rotulo, false, ui.NuevoRotulo(p.t("pick.none"), tema.TextoSM, ui.Normal, "text-muted"), "", "")
		}
		casillas := make([]fyne.CanvasObject, len(opciones))
		for i, o := range opciones {
			x := ui.NuevaCasilla(o.etiqueta, func(si bool) { marcados[o.id] = si })
			x.Marcada = marcados[o.id]
			casillas[i] = x
		}
		return ui.GrupoCampo(rotulo, false, container.New(ui.Columna{Hueco: tema.Espacio(2)}, casillas...), "", "")
	}
	// Lo que no se cambia aquí se lee: el valor, o una raya si no lo hay.
	texto := f.b.rejilla.textoCelda(f.r, c)
	if c.Kind == api.BasesKindCheckbox {
		texto = p.t("view.unchecked")
		if f.r.Data[c.ID] == true {
			texto = p.t("view.checked")
		}
	}
	if texto == "" {
		texto = "—"
	}
	ayuda := ""
	if c.Kind == api.BasesKindFormula && f.puedeEditar {
		ayuda = p.t("grid.formulaReadOnly")
	}
	return ui.GrupoCampo(rotulo, false, ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, "text", v).A(ancho), ayuda, "")
}

// pie son los botones de la ficha. Quien solo lee, la cierra; quien escribe, guarda. En
// un registro recién creado, «Añadir registro» guarda y abre otro: se rellena una tabla
// sin volver a la rejilla entre uno y otro.
func (f *fichaRegistro) pie() fyne.CanvasObject {
	p := f.p
	if !f.puedeEditar {
		cerrar := ui.NuevoBoton(p.t("ui.close"), func() { f.cerrar() })
		return container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cerrar)
	}
	guardar := ui.NuevoBoton(p.t("common.save"), func() { f.guardar(false) })
	if f.nuevo {
		otro := ui.NuevoBoton(p.t("grid.addRecord"), func() { f.guardar(true) }).Con(ui.Fantasma, ui.TallaMD)
		return container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, otro, guardar)
	}
	cancelar := ui.NuevoBoton(p.t("common.cancel"), func() { f.cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	return container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, guardar)
}

// cambios son los valores que difieren de los del registro. `vale` falso: alguno no
// encaja en su campo, y queda marcado.
func (f *fichaRegistro) cambios() (datos map[string]any, enlaces map[string][]string, vale bool) {
	datos, enlaces, vale = map[string]any{}, map[string][]string{}, true
	for _, c := range f.b.Campos {
		if c.Kind == api.BasesKindLink {
			if ids, hay := f.marcados(c); hay && !slices.Equal(ids, f.r.Links[c.ID]) && (len(ids) > 0 || len(f.r.Links[c.ID]) > 0) {
				enlaces[c.ID] = ids
			}
			continue
		}
		valor, hay, bien := f.valorDe(c)
		if !bien {
			vale = false
			continue
		}
		// Se compara como se lee: un 21 que se escribe «21» no es un cambio.
		if hay && textoDeValor(f.r.Data[c.ID]) != textoDeValor(valor) {
			datos[c.ID] = valor
		}
	}
	return datos, enlaces, vale
}

// valorDe es lo que dice el control de un campo (nil es vacío). `hay` falso: el campo no
// se cambia desde la ficha. `vale` falso: lo escrito no encaja, y el campo se marca.
func (f *fichaRegistro) valorDe(c api.BasesField) (valor any, hay, vale bool) {
	if e, ok := f.entradas[c.ID]; ok {
		valor, vale = valorEscrito(c, e.Texto())
		if !vale {
			f.invalidos[c.ID] = true
			e.Invalida = true
			e.Refresh()
		}
		return valor, true, vale
	}
	if s, ok := f.selectores[c.ID]; ok {
		if o := f.opcionesDe[c.ID][s.Elegida]; o != "" {
			valor = o
		}
		return valor, true, true
	}
	if x, ok := f.casillas[c.ID]; ok {
		if x.Marcada {
			valor = true
		}
		return valor, true, true
	}
	if ids, ok := f.marcados(c); ok {
		if len(ids) > 0 {
			valor = ids
		}
		return valor, true, true
	}
	return nil, false, true
}

// marcados son las opciones marcadas de una selección múltiple o de un enlace, en el
// orden de las opciones y no en el de los toques.
func (f *fichaRegistro) marcados(c api.BasesField) ([]string, bool) {
	marcados, ok := f.varios[c.ID]
	if !ok {
		return nil, false
	}
	ids := []string{}
	for _, o := range f.opcionesVarios(c) {
		if marcados[o.id] {
			ids = append(ids, o.id)
		}
	}
	return ids, true
}

// guardar envía lo cambiado y cierra. Con `otro`, añade además un registro y abre su
// ficha.
func (f *fichaRegistro) guardar(otro bool) {
	datos, enlaces, vale := f.cambios()
	if !vale {
		f.rehacer()
		return
	}
	f.cerrar()
	if len(datos) > 0 || len(enlaces) > 0 {
		f.p.cambiar(f.r, datos, enlaces)
	}
	if otro {
		f.p.nuevoRegistro()
	}
}
