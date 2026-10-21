// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los diálogos de una base abierta (Dialogos.tsx y DialogoVista.tsx): elegir varios
// valores, crear un campo y crear una vista. Todos son el Dialogo de la suite con
// «Cancelar» y su acción en el pie.

// anchoDialogo es lo que queda dentro de un diálogo de ese ancho máximo (560 el `md`,
// 400 el `sm`): `ui.AnchoCuerpoTalla`, la cuenta del propio diálogo. Era una copia que
// restaba el margen de la ventana también en la hoja del teléfono, que va a todo el ancho
// y sin filetes (310 en 390 en vez de 342), y no restaba los filetes en el escritorio.
func (p *Pantalla) anchoDialogo(maximo float32) float32 {
	talla := ui.DialogoMD
	if maximo <= 400 {
		talla = ui.DialogoSM
	}
	return ui.AnchoCuerpoTalla(talla, p.tam)
}

// dialogo arma un diálogo `md` con «Cancelar» y la acción, y lo devuelve con el botón de
// la acción para poder apagarlo mientras falte algo.
func (p *Pantalla) dialogo(titulo string, cuerpo fyne.CanvasObject, rotulo string, accion func()) (*ui.Dialogo, *ui.Boton) {
	var d *ui.Dialogo
	cancelar := ui.NuevoBoton(p.t("common.cancel"), func() { d.Cerrar() }).Con(ui.Fantasma, ui.TallaMD)
	hacer := ui.NuevoBoton(rotulo, accion)
	d = ui.NuevoDialogo(titulo, cuerpo).Con(ui.DialogoMD, container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, hacer))
	d.RotuloCerrar = p.t("ui.close")
	return d, hacer
}

// enDialogo lanza la escritura de un diálogo: si sale bien lo cierra y sigue; si no, le
// deja el mensaje al diálogo, que sigue abierto.
func (p *Pantalla) enDialogo(d *ui.Dialogo, f func() error, fallo func(string), despues func()) {
	d.Ocupado(true)
	p.fondo(func() {
		err := f()
		enHilo(func() {
			d.Ocupado(false)
			if err != nil {
				fallo(p.mensajeDeError(err))
				return
			}
			d.Cerrar()
			despues()
		})
	})
}

// dialogoVarios es elegir varios: las opciones de una selección múltiple o los registros
// de un enlace.
func (p *Pantalla) dialogoVarios(r api.BasesRecord, campo api.BasesField) {
	b := p.abierta
	esEnlace := campo.Kind == api.BasesKindLink
	var opciones []opcion
	elegidos := cadenasDe(r.Data[campo.ID])
	if esEnlace {
		opciones, elegidos = b.opciones[campo.LinkTableID], r.Links[campo.ID]
	} else {
		for _, o := range campo.Choices {
			opciones = append(opciones, opcion{o, o})
		}
	}
	marcados := map[string]bool{}
	for _, id := range elegidos {
		marcados[id] = true
	}
	var cuerpo fyne.CanvasObject = ui.NuevoRotulo(p.t("pick.none"), tema.TextoSM, ui.Normal, "text-muted")
	if len(opciones) > 0 {
		casillas := make([]fyne.CanvasObject, len(opciones))
		for i, o := range opciones {
			c := ui.NuevaCasilla(o.etiqueta, func(si bool) { marcados[o.id] = si })
			c.Marcada = marcados[o.id]
			casillas[i] = c
		}
		cuerpo = container.New(ui.Columna{Hueco: tema.Espacio(2)}, casillas...)
	}
	var d *ui.Dialogo
	d, _ = p.dialogo(campo.Name, cuerpo, p.t("common.save"), func() {
		d.Cerrar()
		// En el orden de las opciones, no en el de los clics.
		ids := []string{}
		for _, o := range opciones {
			if marcados[o.id] {
				ids = append(ids, o.id)
			}
		}
		if esEnlace {
			p.cambiar(r, nil, map[string][]string{campo.ID: ids})
			return
		}
		var valor any
		if len(ids) > 0 {
			valor = ids
		}
		p.cambiar(r, map[string]any{campo.ID: valor}, nil)
	})
	d.Mostrar(p.ventana.Canvas())
}

// tiposDeCampo son los del selector de «Añadir campo», en el orden de la web.
var tiposDeCampo = []string{
	api.BasesKindText, api.BasesKindNumber, api.BasesKindDate, api.BasesKindCheckbox, api.BasesKindSelect,
	api.BasesKindMultiselect, api.BasesKindLink, api.BasesKindFormula, api.BasesKindAttachment,
}

// lineas son las líneas con algo de un área de texto: una opción por línea.
func lineas(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// formCampo es lo que se lleva escrito en «Añadir campo».
type formCampo struct {
	p      *Pantalla
	b      *Base
	otras  []api.BasesTable
	tipos  []string
	d      *ui.Dialogo
	crear  *ui.Boton
	nombre *ui.Entrada
	// opciones son las de una elección (una por línea) y formula, la expresión con nombres.
	opciones, formula        *ui.AreaTexto
	tipo, destino, resultado *ui.Selector
	fallo                    string
}

// dialogoCampo es crear un campo nuevo (solo el propietario).
func (p *Pantalla) dialogoCampo() {
	b := p.abierta
	f := &formCampo{p: p, b: b}
	for _, t := range b.Tablas {
		if t.ID != b.Activa {
			f.otras = append(f.otras, t)
		}
	}
	// Un enlace necesita otra tabla a la que apuntar: sin ella, el tipo no se ofrece.
	f.tipos = slices.DeleteFunc(slices.Clone(tiposDeCampo), func(k string) bool { return k == api.BasesKindLink && len(f.otras) == 0 })
	rotulos := make([]string, len(f.tipos))
	for i, k := range f.tipos {
		rotulos[i] = p.t("field.kind." + k)
	}
	destinos := make([]string, len(f.otras))
	for i, t := range f.otras {
		destinos[i] = t.Name
	}
	f.nombre = ui.NuevaEntrada("")
	f.nombre.Rotulo = p.t("field.name")
	f.opciones = ui.NuevaAreaTexto("")
	f.opciones.Rotulo = p.t("field.choices")
	f.formula = ui.NuevaAreaTexto("")
	f.formula.Rotulo, f.formula.Filas = p.t("field.kind.formula"), 3
	f.tipo = ui.NuevoSelector(rotulos, func(int) { f.rehacer() })
	f.tipo.Rotulo = p.t("field.type")
	f.destino = ui.NuevoSelector(destinos, nil)
	f.destino.Rotulo = p.t("field.linkTo")
	f.resultado = ui.NuevoSelector([]string{p.t("field.kind.number"), p.t("field.kind.text")}, nil)
	f.resultado.Rotulo = p.t("field.formulaResult")
	f.d, f.crear = p.dialogo(p.t("field.add"), container.New(ui.Columna{}), p.t("field.create"), f.enviar)

	f.nombre.AlCambiar(func(string) { f.ajustar() })
	f.opciones.AlCambiar(func(string) { f.ajustar() })
	// El aviso de «ese campo no existe» sale y se va según se escribe.
	hayAviso := false
	f.formula.AlCambiar(func(string) {
		if aviso := f.formulaDesconocida(); aviso != hayAviso {
			hayAviso = aviso
			f.rehacer()
			return
		}
		f.ajustar()
	})
	f.d.Foco = f.nombre
	f.rehacer()
	f.d.Mostrar(p.ventana.Canvas())
}

// formulaDesconocida dice si la fórmula escrita nombra un campo que no existe.
func (f *formCampo) formulaDesconocida() bool {
	_, ok := formulaAIds(f.formula.Texto(), f.b.Campos, f.b.Tablas)
	return !ok && strings.TrimSpace(f.formula.Texto()) != ""
}

// campo es el campo que se crearía con lo escrito, y si ya se puede crear.
func (f *formCampo) campo() (api.BasesNewField, bool) {
	c := api.BasesNewField{Name: strings.TrimSpace(f.nombre.Texto()), Kind: f.tipos[f.tipo.Elegida]}
	vale := c.Name != ""
	switch c.Kind {
	case api.BasesKindSelect, api.BasesKindMultiselect:
		c.Choices = lineas(f.opciones.Texto())
		vale = vale && len(c.Choices) > 0
	case api.BasesKindLink:
		c.LinkTableID = f.otras[f.destino.Elegida].ID
	case api.BasesKindFormula:
		// Se escribe con nombres y se envía con ids.
		ids, ok := formulaAIds(f.formula.Texto(), f.b.Campos, f.b.Tablas)
		c.Formula, c.Result = ids, []string{api.BasesKindNumber, api.BasesKindText}[f.resultado.Elegida]
		vale = vale && ok && strings.TrimSpace(f.formula.Texto()) != ""
	}
	return c, vale
}

func (f *formCampo) ajustar() {
	_, vale := f.campo()
	f.crear.Inerte = !vale
	f.crear.Refresh()
}

// rehacer vuelve a armar el cuerpo: lo que se pide depende del tipo elegido.
func (f *formCampo) rehacer() {
	p := f.p
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	hijos := []fyne.CanvasObject{
		ui.GrupoCampo(p.t("field.name"), true, f.nombre, "", ""),
		ui.GrupoCampo(p.t("field.type"), false, f.tipo, "", ""),
	}
	switch f.tipos[f.tipo.Elegida] {
	case api.BasesKindSelect, api.BasesKindMultiselect:
		hijos = append(hijos, ui.GrupoCampo(p.t("field.choices"), false, f.opciones, p.t("field.choicesHelp"), ""))
	case api.BasesKindLink:
		hijos = append(hijos, ui.GrupoCampo(p.t("field.linkTo"), false, f.destino, "", ""))
	case api.BasesKindFormula:
		desconocido := ""
		if f.formulaDesconocida() {
			desconocido = p.t("field.formulaUnknown")
		}
		hijos = append(hijos,
			container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
				ui.GrupoCampo(p.t("field.kind.formula"), true, f.formula, "", desconocido),
				ui.NuevoParrafo(p.t("field.formulaHelp"), tema.TextoXS, ui.Normal, "text-muted", v).A(p.anchoDialogo(560))),
			ui.NuevoParrafo(strings.Join(referenciasDeFormula(f.b.Campos, f.b.Tablas), "  "), tema.TextoXS, ui.Mono, "text-muted", v).A(p.anchoDialogo(560)),
			ui.GrupoCampo(p.t("field.formulaResult"), false, f.resultado, "", ""))
	}
	if f.fallo != "" {
		hijos = append(hijos, alerta(f.fallo, v))
	}
	f.d.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...))
	f.ajustar()
}

func (f *formCampo) enviar() {
	c, vale := f.campo()
	if !vale {
		return
	}
	p, tabla := f.p, f.b.Activa
	p.enDialogo(f.d, func() error {
		ctx, cancelar := p.ctx()
		defer cancelar()
		_, err := p.svc.CreateField(ctx, tabla, c)
		return err
	}, func(m string) { f.fallo = m; f.rehacer() }, func() {
		p.cargarEstructura()
		p.cargarRegistros("inicio")
	})
}

// formVista es lo que se lleva escrito en «Añadir vista».
type formVista struct {
	p *Pantalla
	b *Base
	// Los campos por lo que valen: una fórmula se filtra y se ordena como su resultado.
	selecciones, ordenables, filtrables []api.BasesField
	tipos                               []string
	borrador                            borradorVista
	d                                   *ui.Dialogo
	crear                               *ui.Boton
	nombre, valor                       *ui.Entrada
	// selectores son los del diálogo, por su nombre: cada uno es el MISMO objeto de un
	// rehacer al siguiente. Elegir una opción rehace el cuerpo, y el foco tiene que seguir
	// en el selector que se acaba de usar, no en uno que ya no está en la ventana.
	selectores map[string]*ui.Selector
	fallo      string
}

// dialogoVista es crear una vista guardada: rejilla (con un filtro y un orden
// opcionales), tablero (por un campo de selección) o formulario (con los campos que pide).
func (p *Pantalla) dialogoVista() {
	b := p.abierta
	f := &formVista{
		p: p, b: b, selectores: map[string]*ui.Selector{},
		borrador: borradorVista{tipo: api.BasesGrid, filtroOp: "eq", ordenDir: "asc"},
		// Un tablero necesita un campo de elección única: sin él, el tipo no se ofrece.
		tipos: []string{api.BasesGrid, api.BasesForm},
	}
	for _, c := range b.Campos {
		c = efectivo(c)
		if c.Kind == api.BasesKindSelect {
			f.selecciones = append(f.selecciones, c)
		}
		if c.Kind != api.BasesKindLink && c.Kind != api.BasesKindMultiselect && c.Kind != api.BasesKindAttachment {
			f.ordenables = append(f.ordenables, c)
		}
		if len(operadoresDe(c)) > 0 {
			f.filtrables = append(f.filtrables, c)
		}
	}
	if len(f.selecciones) > 0 {
		f.tipos = []string{api.BasesGrid, api.BasesKanban, api.BasesForm}
		f.borrador.agrupar = f.selecciones[0].ID
	}
	f.nombre = ui.NuevaEntrada("")
	f.nombre.Rotulo = p.t("view.name")
	f.valor = ui.NuevaEntrada("")
	f.valor.Rotulo, f.valor.Ancho = p.t("view.filterValue"), 12*tema.Rem
	f.d, f.crear = p.dialogo(p.t("view.add"), container.New(ui.Columna{}), p.t("view.create"), f.enviar)
	f.nombre.AlCambiar(func(string) { f.ajustar() })
	f.valor.AlCambiar(func(s string) { f.borrador.filtroValor = s; f.ajustar() })
	f.d.Foco = f.nombre
	f.rehacer()
	f.d.Mostrar(p.ventana.Canvas())
}

func (f *formVista) ajustar() {
	f.borrador.nombre = strings.TrimSpace(f.nombre.Texto())
	_, vale := configDeBorrador(f.borrador, f.b.Campos)
	f.crear.Inerte = !vale || f.borrador.nombre == ""
	f.crear.Refresh()
}

// selector es el del diálogo con ese nombre, con sus opciones de ahora. Los de una fila
// (`flex flex-wrap gap-2`) miden lo que su opción más larga.
func (f *formVista) selector(clave string, opciones []string, elegida int, alElegir func(int)) *ui.Selector {
	rotulo := f.p.t(clave)
	s := f.selectores[rotulo]
	if s == nil {
		s = ui.NuevoSelector(nil, nil)
		s.Rotulo, f.selectores[rotulo] = rotulo, s
	}
	s.Opciones, s.Elegida, s.Ancho = opciones, max(elegida, 0), 0
	s.AlElegir = func(i int) { alElegir(i); f.rehacer() }
	for _, o := range opciones {
		s.Ancho = max(s.Ancho, ui.Medir(o, tema.TextoBase, ui.Normal).Width+3.5*tema.Rem)
	}
	s.Refresh()
	return s
}

func (f *formVista) titulo(clave string) fyne.CanvasObject {
	return ui.NuevoRotulo(f.p.t(clave), tema.TextoSM, ui.SemiNegrita, "text")
}

// nombresDe son los nombres de esos campos, tras las opciones que vayan delante.
func nombresDe(campos []api.BasesField, delante ...string) []string {
	out := slices.Clone(delante)
	for _, c := range campos {
		out = append(out, c.Name)
	}
	return out
}

func indiceDe(campos []api.BasesField, id string) int {
	return slices.IndexFunc(campos, func(c api.BasesField) bool { return c.ID == id })
}

// rehacer vuelve a armar el cuerpo: lo que se pide depende del tipo de vista.
func (f *formVista) rehacer() {
	p := f.p
	rotulosTipo := make([]string, len(f.tipos))
	for i, k := range f.tipos {
		rotulosTipo[i] = p.t("view.kind." + k)
	}
	hijos := []fyne.CanvasObject{
		ui.GrupoCampo(p.t("view.name"), true, f.nombre, "", ""),
		ui.GrupoCampo(p.t("view.kind"), false,
			f.selector("view.kind", rotulosTipo, slices.Index(f.tipos, f.borrador.tipo), func(i int) { f.borrador.tipo = f.tipos[i] }), "", ""),
	}
	if f.borrador.tipo == api.BasesKanban {
		hijos = append(hijos, ui.GrupoCampo(p.t("view.groupBy"), false,
			f.selector("view.groupBy", nombresDe(f.selecciones), indiceDe(f.selecciones, f.borrador.agrupar),
				func(i int) { f.borrador.agrupar = f.selecciones[i].ID }), p.t("view.groupByHelp"), ""))
	}
	if f.borrador.tipo == api.BasesForm {
		hijos = append(hijos, f.camposDelFormulario())
	} else {
		hijos = append(hijos, f.filtro(), f.orden())
	}
	if f.fallo != "" {
		hijos = append(hijos, alerta(f.fallo, tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
	}
	f.d.CambiarCuerpo(container.New(ui.Columna{Hueco: tema.Espacio(4)}, hijos...))
	f.ajustar()
}

// camposDelFormulario son las casillas de los campos que pedirá el formulario.
func (f *formVista) camposDelFormulario() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	campos := camposDeFormulario(f.b.Campos)
	grupo := make([]fyne.CanvasObject, 0, 2+len(campos))
	grupo = append(grupo, f.titulo("view.formFields"),
		ui.NuevoParrafo(f.p.t("view.formFieldsHelp"), tema.TextoXS, ui.Normal, "text-muted", v).A(f.p.anchoDialogo(560)))
	for _, c := range campos {
		casilla := ui.NuevaCasilla(c.Name, func(si bool) {
			f.borrador.campos = slices.DeleteFunc(f.borrador.campos, func(id string) bool { return id == c.ID })
			if si {
				f.borrador.campos = append(f.borrador.campos, c.ID)
			}
			f.ajustar()
		})
		casilla.Marcada = slices.Contains(f.borrador.campos, c.ID)
		grupo = append(grupo, casilla)
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, grupo...)
}

// elegirCampoDelFiltro cambia el campo por el que se filtra (0 = ninguno) y deja la
// condición y el valor como al principio.
func (f *formVista) elegirCampoDelFiltro(i int) {
	f.borrador.filtroCampo, f.borrador.filtroOp, f.borrador.filtroValor = "", "eq", ""
	f.valor.PonerTexto("")
	f.valor.PonerMarcaDeAgua("")
	if i == 0 {
		return
	}
	c := f.filtrables[i-1]
	f.borrador.filtroCampo = c.ID
	// «es» si el tipo lo tiene; si no, su primera condición.
	if ops := operadoresDe(c); !slices.Contains(ops, "eq") {
		f.borrador.filtroOp = ops[0]
	}
	if c.Kind == api.BasesKindDate {
		f.valor.PonerMarcaDeAgua("2026-12-31") // viaja en ISO, como el `type="date"` de la web
	}
}

// filtro es el campo por el que se filtra, la condición de su tipo y, si la condición lo
// pide, el valor.
func (f *formVista) filtro() fyne.CanvasObject {
	p, b := f.p, &f.borrador
	hijos := make([]fyne.CanvasObject, 0, 3)
	hijos = append(hijos, f.titulo("view.filter"),
		f.selector("view.filterField", nombresDe(f.filtrables, p.t("view.none")), indiceDe(f.filtrables, b.filtroCampo)+1, f.elegirCampoDelFiltro))
	i := indiceDe(f.filtrables, b.filtroCampo)
	if i < 0 {
		return container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...)
	}
	c := f.filtrables[i]
	ops := operadoresDe(c)
	rotulosOp := make([]string, len(ops))
	for j, op := range ops {
		rotulosOp[j] = p.t("view.op." + op)
	}
	linea := []fyne.CanvasObject{f.selector("view.filterOp", rotulosOp, slices.Index(ops, b.filtroOp), func(j int) { b.filtroOp = ops[j] })}
	if b.filtroOp != "empty" && b.filtroOp != "notempty" {
		linea = append(linea, f.valorDelFiltro(c))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, append(hijos, container.New(ui.Fila{Hueco: tema.Espacio(2)}, linea...))...)
}

// valorDelFiltro es con qué se compara: una de las opciones, marcada o sin marcar, o lo
// que se escriba.
func (f *formVista) valorDelFiltro(c api.BasesField) fyne.CanvasObject {
	p, b := f.p, &f.borrador
	switch c.Kind {
	case api.BasesKindSelect, api.BasesKindMultiselect:
		opciones := append([]string{""}, c.Choices...)
		return f.selector("view.filterValue", opciones, slices.Index(opciones, b.filtroValor), func(j int) { b.filtroValor = opciones[j] })
	case api.BasesKindCheckbox:
		if b.filtroValor == "" {
			b.filtroValor = "true"
		}
		marcas := []string{"true", "false"}
		return f.selector("view.filterValue", []string{p.t("view.checked"), p.t("view.unchecked")},
			slices.Index(marcas, b.filtroValor), func(j int) { b.filtroValor = marcas[j] })
	}
	return f.valor
}

// orden es el campo por el que se ordena y, si lo hay, el sentido.
func (f *formVista) orden() fyne.CanvasObject {
	p, b := f.p, &f.borrador
	linea := []fyne.CanvasObject{f.selector("view.sortField", nombresDe(f.ordenables, p.t("view.none")), indiceDe(f.ordenables, b.ordenCampo)+1, func(i int) {
		b.ordenCampo = ""
		if i > 0 {
			b.ordenCampo = f.ordenables[i-1].ID
		}
	})}
	if b.ordenCampo != "" {
		sentidos := []string{"asc", "desc"}
		linea = append(linea, f.selector("view.sortDir", []string{p.t("view.asc"), p.t("view.desc")},
			slices.Index(sentidos, b.ordenDir), func(i int) { b.ordenDir = sentidos[i] }))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, f.titulo("view.sort"), container.New(ui.Fila{Hueco: tema.Espacio(2)}, linea...))
}

func (f *formVista) enviar() {
	p, b := f.p, f.b
	cfg, vale := configDeBorrador(f.borrador, b.Campos)
	if !vale || f.borrador.nombre == "" {
		return
	}
	tabla, creada := b.Activa, api.BasesView{}
	var vistas []api.BasesView
	p.enDialogo(f.d, func() (err error) {
		ctx, cancelar := p.ctx()
		defer cancelar()
		if creada, err = p.svc.CreateView(ctx, tabla, f.borrador.nombre, f.borrador.tipo, cfg); err != nil {
			return err
		}
		vistas, err = p.svc.ListViews(ctx, tabla)
		return err
	}, func(m string) { f.fallo = m; f.rehacer() }, func() {
		if p.abierta == b && b.Activa == tabla {
			b.Vistas = vistas
			p.elegirVista(creada.ID)
		}
	})
}
