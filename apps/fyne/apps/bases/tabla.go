// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// opcion es un registro de una tabla enlazada, con el nombre con el que se ofrece.
type opcion struct{ id, etiqueta string }

// elegirTabla cambia la tabla que se mira: sus vistas guardadas, y se vuelve a «todos
// los registros».
func (p *Pantalla) elegirTabla(id string) {
	b := p.abierta
	b.Activa, b.VistaID, b.Vistas, b.Estado = id, "", nil, ""
	b.Campos, b.vent, b.RolTabla, b.Recalculando = nil, sinVentanas(), "", false
	b.rejilla = nuevaRejilla(p.t)
	b.rejilla.AlCambiar = func(r api.BasesRecord, campo api.BasesField, valor any) {
		p.cambiar(r, map[string]any{campo.ID: valor}, nil)
	}
	b.rejilla.AlElegirVarios = p.dialogoVarios
	b.rejilla.AlAbrir = func(r api.BasesRecord) { p.abrirRegistro(r, false) }
	// Intro sobre una celda de adjuntos abre la ficha, que es donde se ven y se suben.
	b.rejilla.AlPegar = p.pegar
	b.rejilla.AlAdjuntos = func(r api.BasesRecord, _ api.BasesField) { p.abrirRegistro(r, false) }
	b.rejilla.AlMover = p.dejarALaVista
	// `overflow-auto`: la rejilla se desplaza de lado y la rueda sigue bajando la página.
	b.lado = ui.NuevoDesplazaLado(b.rejilla, 0, func() fyne.Scrollable {
		if p.scroll == nil {
			return nil
		}
		return p.scroll
	})
	if p.scroll != nil {
		p.scroll.Offset = fyne.Position{}
	}
	p.Repintar()
	if id == "" || p.svc == nil {
		return
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		vistas, err := p.svc.ListViews(ctx, id)
		enHilo(func() {
			if p.abierta == b && b.Activa == id && err == nil {
				b.Vistas = vistas
				p.Repintar()
			}
		})
	})
	p.cargarRegistros("inicio")
}

// elegirVista cambia la vista guardada que se mira ("" = todos los registros).
func (p *Pantalla) elegirVista(id string) {
	b := p.abierta
	b.VistaID, b.vent = id, sinVentanas()
	b.enlaces, b.enlaceNuevo, b.falloEnlaces = nil, "", ""
	if v := b.vista(); v != nil && v.Kind == api.BasesForm && puedeEsquema(b.Datos.Role) {
		p.cargarEnlaces()
	}
	b.rejilla.Poner(b.Campos, nil)
	b.lado.Mostrar(0, 0) // otra vista, otros registros: se vuelve al principio también de lado
	p.Repintar()
	p.cargarRegistros("inicio")
}

// vista es la vista guardada que se está mirando, o nil.
func (b *Base) vista() *api.BasesView {
	for i := range b.Vistas {
		if b.Vistas[i].ID == b.VistaID {
			return &b.Vistas[i]
		}
	}
	return nil
}

// campoDelTablero es el campo de selección por el que agrupa la vista que se mira, si es
// un tablero. Sin él (se borró el campo, o ya no es una elección) la vista se pinta en rejilla.
func (b *Base) campoDelTablero() (api.BasesField, bool) {
	v := b.vista()
	if v == nil || v.Kind != api.BasesKanban {
		return api.BasesField{}, false
	}
	for _, c := range b.Campos {
		if c.ID == v.Config.GroupBy && c.Kind == api.BasesKindSelect {
			return c, true
		}
	}
	return api.BasesField{}, false
}

// rol es el de la tabla si el servicio lo ha dicho, y si no el de la base.
func (b *Base) rol() string {
	if b.RolTabla != "" {
		return b.RolTabla
	}
	return b.Datos.Role
}

// cargarRegistros pide la primera ventana ("inicio"), la que sigue ("mas") o la anterior
// ("antes"): `GET /tables/{id}/records` con el cursor de la página, nunca la tabla entera.
func (p *Pantalla) cargarRegistros(hacia string) {
	b := p.abierta
	if b == nil || b.Activa == "" || p.svc == nil {
		return
	}
	tabla, vista, v, cursor := b.Activa, b.VistaID, b.vent, ""
	switch hacia {
	case "mas":
		if cursor = v.siguiente; cursor == "" {
			return
		}
	case "antes":
		var hay bool
		if cursor, hay = cursorAnterior(v); !hay {
			return
		}
	}
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		pag, err := p.svc.ListRecords(ctx, tabla, vista, cursor, pagina)
		var opciones map[string][]opcion
		if err == nil && hacia == "inicio" {
			opciones, err = p.opcionesDeEnlaces(ctx, pag.Fields)
		}
		enHilo(func() {
			if p.abierta == b && b.Activa == tabla && b.VistaID == vista {
				p.recibirPagina(hacia, v, pag, opciones, err)
			}
		})
	})
}

// opcionesDeEnlaces pide las etiquetas y opciones de los campos de enlace: los registros
// de cada tabla destino.
func (p *Pantalla) opcionesDeEnlaces(ctx context.Context, campos []api.BasesField) (map[string][]opcion, error) {
	opciones := map[string][]opcion{}
	for _, c := range campos {
		if c.Kind != api.BasesKindLink || c.LinkTableID == "" || opciones[c.LinkTableID] != nil {
			continue
		}
		otra, err := p.svc.ListRecords(ctx, c.LinkTableID, "", "", maxOpcionesEnlace)
		if err != nil {
			return nil, err
		}
		lista := make([]opcion, len(otra.Records))
		for i, r := range otra.Records {
			lista[i] = opcion{r.ID, etiquetaDeRegistro(otra.Fields, r)}
		}
		opciones[c.LinkTableID] = lista
	}
	return opciones, nil
}

// recibirPagina pone en la base abierta la página que ha llegado (o su fallo).
func (p *Pantalla) recibirPagina(hacia string, v ventanas, pag api.BasesRecordPage, opciones map[string][]opcion, err error) {
	b := p.abierta
	if err != nil {
		// El cursor es del orden de la vista: si la vista cambió, se vuelve a empezar.
		var e *api.Error
		if hacia != "inicio" && errors.As(err, &e) && e.Status == http.StatusUnprocessableEntity {
			b.vent = sinVentanas()
			p.cargarRegistros("inicio")
			return
		}
		b.Fallo = p.mensajeDeError(err)
		p.Repintar()
		return
	}
	b.Campos, b.RolTabla, b.Recalculando, b.Fallo = pag.Fields, pag.Role, pag.Recalculating, ""
	switch hacia {
	case "mas":
		b.vent = avanzar(v, pag)
	case "antes":
		b.vent = retroceder(v, pag)
	default:
		b.vent = empezar(pag)
	}
	b.Estado = ""
	if hacia != "inicio" {
		desde := b.vent.primera * pagina
		b.Estado = p.t("grid.window", i18n.Vars{"from": desde + 1, "to": desde + len(b.vent.registros())})
	}
	for destino, lista := range opciones {
		b.opciones[destino] = lista
		b.etiquetas[destino] = map[string]string{}
		for _, o := range lista {
			b.etiquetas[destino][o.id] = o.etiqueta
		}
	}
	p.ponerRejilla()
	p.Repintar()
}

// ponerRejilla le da a la rejilla lo que hay en memoria.
func (p *Pantalla) ponerRejilla() {
	b := p.abierta
	g := b.rejilla
	if t := b.tabla(); t != nil {
		g.Tabla = t.Name
	}
	g.PuedeEditar, g.Etiquetas, g.Desde = puedeEscribir(b.rol()), b.etiquetas, b.vent.primera*pagina
	g.Poner(b.Campos, b.vent.registros())
}

// dejarALaVista desplaza lo justo para que la celda con el foco se vea: de lado en la
// rejilla y de arriba abajo en la página.
func (p *Pantalla) dejarALaVista(pos fyne.Position, tam fyne.Size) {
	b := p.abierta
	if b == nil || b.lado == nil || p.scroll == nil {
		return
	}
	b.lado.Mostrar(pos.X, tam.Width)
	d := fyne.CurrentApp().Driver()
	arriba := d.AbsolutePositionForObject(b.rejilla).Y - d.AbsolutePositionForObject(p.scroll).Y + pos.Y
	if arriba < 0 {
		p.scroll.Offset.Y += arriba
	} else if sobra := arriba + tam.Height - p.scroll.Size().Height; sobra > 0 {
		p.scroll.Offset.Y += sobra
	}
	p.scroll.Refresh()
}

// idAleatorio es el identificador (UUID v4) de una operación de celda: repetirla no la
// aplica dos veces.
func idAleatorio() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6], u[8] = u[6]&0x0f|0x40, u[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// cambiar escribe celdas (o los enlaces de un campo) como operaciones con identificador:
// no pide la versión del registro, así que dos personas en celdas distintas no se
// estorban; en la misma gana quien escribe después y se le avisa de que ha pisado algo.
func (p *Pantalla) cambiar(r api.BasesRecord, datos map[string]any, enlaces map[string][]string) {
	b := p.abierta
	tabla := b.Activa
	p.fondo(func() {
		ctx, cancelar := p.ctx()
		defer cancelar()
		var err error
		pisado, ultimo := false, r
		escribir := func(campo string, celda api.BasesCell) {
			if err != nil {
				return
			}
			var res api.BasesCellResult
			if res, err = p.svc.WriteCell(ctx, r.ID, campo, celda, idAleatorio()); err == nil {
				pisado, ultimo = pisado || res.Conflict, res.Record
			}
		}
		for campo, valor := range datos {
			escribir(campo, api.BasesCell{Value: valor, Expected: r.Data[campo]})
		}
		for campo, ids := range enlaces {
			if ids == nil {
				ids = []string{}
			}
			visto := r.Links[campo]
			if visto == nil {
				visto = []string{}
			}
			escribir(campo, api.BasesCell{Links: ids, Expected: visto})
		}
		enHilo(func() {
			if p.abierta == b && b.Activa == tabla {
				p.trasCambiar(ultimo, pisado, err)
			}
		})
	})
}

// trasCambiar pone en la rejilla el registro como ha quedado, y dice si se ha pisado el
// valor de otra persona o si la escritura ha fallado.
func (p *Pantalla) trasCambiar(r api.BasesRecord, pisado bool, err error) {
	b := p.abierta
	b.vent.editar(func(x api.BasesRecord) (api.BasesRecord, bool) {
		if x.ID == r.ID {
			return r, true
		}
		return x, true
	})
	p.ponerRejilla()
	b.Estado = ""
	if pisado {
		b.Estado = p.t("rt.overwrote")
	}
	b.Fallo = ""
	if err != nil {
		b.Fallo = p.mensajeDeError(err)
	}
	p.Repintar() // el tablero se pinta con lo que hay: una tarjeta movida cambia de columna
}

func (p *Pantalla) nuevoRegistro() {
	b := p.abierta
	tabla := b.Activa
	var r api.BasesRecord
	p.hacer(func() (err error) {
		ctx, cancelar := p.ctx()
		defer cancelar()
		r, err = p.svc.CreateRecord(ctx, tabla, nil, nil)
		return err
	}, func() {
		if b.Activa != tabla {
			return
		}
		b.vent.anadir(r)
		b.Estado = p.t("grid.added")
		p.ponerRejilla()
		p.Repintar()
		// Añadir es rellenar: la ficha del registro nuevo se abre sola.
		p.abrirRegistro(r, true)
	})
}

// borrarRegistro pregunta antes. Si otros registros enlazan con él, el propietario puede
// borrarlo quitando esos enlaces, con una segunda pregunta.
func (p *Pantalla) borrarRegistro(r api.BasesRecord) {
	b := p.abierta
	quitar := func() {
		b.vent.editar(func(x api.BasesRecord) (api.BasesRecord, bool) { return x, x.ID != r.ID })
		p.ponerRejilla()
		p.Repintar()
	}
	borrar := func(forzar bool) func() error {
		return func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.DeleteRecord(ctx, r.ID, forzar)
		}
	}
	p.confirmar(p.t("grid.deleteConfirm"), p.t("common.delete"), func() {
		p.fondo(func() {
			err := borrar(false)()
			enHilo(func() {
				var e *api.Error
				switch {
				case err == nil:
					quitar()
				case errors.As(err, &e) && e.Code() == "referenced" && puedeEsquema(b.Datos.Role):
					p.confirmar(p.t("grid.forceConfirm"), p.t("common.delete"), func() { p.hacer(borrar(true), quitar) })
				default:
					b.Fallo = p.mensajeDeError(err)
					p.Repintar()
				}
			})
		})
	})
}

func (p *Pantalla) nuevaTabla() {
	b := p.abierta
	p.preguntar(p.t("table.namePrompt"), "", p.t("table.create"), func(nombre string) {
		var t api.BasesTable
		p.hacer(func() (err error) {
			ctx, cancelar := p.ctx()
			defer cancelar()
			t, err = p.svc.CreateTable(ctx, b.ID, nombre)
			return err
		}, func() {
			b.Activa = t.ID // la estructura, al llegar, la encuentra y la abre
			b.rejilla = nil
			p.cargarEstructura()
		})
	})
}

func (p *Pantalla) renombrarTabla() {
	t := p.abierta.tabla()
	if t == nil {
		return
	}
	id, viejo := t.ID, t.Name
	p.preguntar(p.t("table.namePrompt"), viejo, p.t("ui.save"), func(nombre string) {
		if nombre == viejo {
			return
		}
		p.hacer(func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.RenameTable(ctx, id, nombre)
		}, p.cargarEstructura)
	})
}

func (p *Pantalla) borrarTabla() {
	t := p.abierta.tabla()
	if t == nil {
		return
	}
	id := t.ID
	p.confirmar(p.t("table.deleteConfirm", i18n.Vars{"name": t.Name}), p.t("common.delete"), func() {
		p.hacer(func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.DeleteTable(ctx, id)
		}, p.cargarEstructura)
	})
}

func (p *Pantalla) borrarVista() {
	b := p.abierta
	v := b.vista()
	if v == nil {
		return
	}
	id := v.ID
	p.confirmar(p.t("view.deleteConfirm", i18n.Vars{"name": v.Name}), p.t("common.delete"), func() {
		p.hacer(func() error {
			ctx, cancelar := p.ctx()
			defer cancelar()
			return p.svc.DeleteView(ctx, id)
		}, func() {
			for i, x := range b.Vistas {
				if x.ID == id {
					b.Vistas = append(b.Vistas[:i:i], b.Vistas[i+1:]...)
					break
				}
			}
			p.elegirVista("")
		})
	})
}

// botonSM es un botón de las barras de la tabla: `size="sm"`, con su variante.
func botonSM(texto string, v ui.Variante, alPulsar func()) *ui.Boton {
	return ui.NuevoBoton(texto, alPulsar).Con(v, ui.TallaSM)
}

// accionesDeTabla es el menú «⋯» de la tabla: lo que el propietario hace con ella, las
// vistas para quien escribe y, para cualquiera que la lea, exportarla. Lo que borra va al
// final, en rojo y tras una raya. Se arma en cada apertura: un elemento recuerda el menú
// en el que va.
func (p *Pantalla) accionesDeTabla() []fyne.CanvasObject {
	b := p.abierta
	puedeEditar, esquema := puedeEscribir(b.rol()), puedeEsquema(b.Datos.Role)
	var acciones, peligro []fyne.CanvasObject
	if esquema {
		acciones = append(acciones, ui.NuevoElementoMenu(p.t("table.rename"), iconos.Lapiz, p.renombrarTabla))
	}
	if puedeEditar {
		acciones = append(acciones, ui.NuevoElementoMenu(p.t("view.add"), iconos.Mas, p.dialogoVista))
	}
	if p.Guardar != nil {
		acciones = append(acciones,
			ui.NuevoElementoMenu(p.t("export.csv"), iconos.Descargar, func() { p.exportar("csv") }),
			ui.NuevoElementoMenu(p.t("export.json"), iconos.Descargar, func() { p.exportar("json") }))
	}
	if puedeEditar && b.vista() != nil {
		x := ui.NuevoElementoMenu(p.t("view.delete"), iconos.Papelera, p.borrarVista)
		x.Peligro = true
		peligro = append(peligro, x)
	}
	if esquema {
		x := ui.NuevoElementoMenu(p.t("table.delete"), iconos.Papelera, p.borrarTabla)
		x.Peligro = true
		peligro = append(peligro, x)
	}
	if len(acciones) > 0 && len(peligro) > 0 {
		acciones = append(acciones, ui.NuevoSeparadorMenu())
	}
	return append(acciones, peligro...)
}

// objetoTabla es el contenido de la pestaña activa: una barra con la vista que se mira
// y lo que se puede hacer con la tabla, y debajo la rejilla (o lo que pinte la vista).
//
// La barra es UNA línea: el selector de vista a la izquierda, que se lleva lo que sobra,
// y a la derecha «Añadir campo» y el menú «⋯» con lo demás (renombrar, exportar, las
// vistas, borrar). Antes eran tres filas de botones y enlaces sueltos.
func (p *Pantalla) objetoTabla(v tema.Variante) fyne.CanvasObject {
	b := p.abierta
	puedeEditar := puedeEscribir(b.rol())
	vista := b.vista()

	// Las vistas: «Todos los registros» y las guardadas, en un selector.
	rotulos := make([]string, 0, 1+len(b.Vistas))
	rotulos = append(rotulos, p.t("view.all"))
	elegida := 0
	for i, x := range b.Vistas {
		rotulos = append(rotulos, x.Name+" · "+p.t("view.kind."+x.Kind))
		if x.ID == b.VistaID {
			elegida = i + 1
		}
	}
	vistas := b.Vistas
	selector := ui.NuevoSelector(rotulos, func(i int) {
		if i == 0 {
			p.elegirVista("")
			return
		}
		p.elegirVista(vistas[i-1].ID)
	})
	campo := botonSM(p.t("field.add"), ui.Secundario, p.dialogoCampo).ConIcono(iconos.Mas, 14)
	// `h-8`: el selector, a la altura de los botones de la barra.
	selector.Rotulo, selector.Elegida, selector.Talla = p.t("view.list"), elegida, ui.CampoSM
	selector.Alto = ui.MedidaBotonDe(campo).Height
	barra := []fyne.CanvasObject{container.New(tope{Max: 18 * tema.Rem}, selector)}
	if puedeEsquema(b.Datos.Role) {
		barra = append(barra, campo)
	}

	if len(p.accionesDeTabla()) > 0 {
		nombre := ""
		if t := b.tabla(); t != nil {
			nombre = t.Name
		}
		barra = append(barra, p.menuMas(p.t("actions.more")+" · "+nombre, p.accionesDeTabla))
	}
	hijos := []fyne.CanvasObject{container.New(extremos{Hueco: tema.Espacio(2), Centro: true}, barra...)}

	switch campo, hay := b.campoDelTablero(); {
	case vista != nil && vista.Kind == api.BasesForm:
		hijos = append(hijos, p.vistaFormulario(v, *vista))
	case hay:
		hijos = append(hijos, p.objetoTablero(v, campo, puedeEditar))
	default:
		hijos = append(hijos, p.objetoRejilla(puedeEditar))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, hijos...)
}

// vistaFormulario es lo que se ve de una vista de formulario: los campos que rellenará
// quien tenga el enlace.
func (p *Pantalla) vistaFormulario(v tema.Variante, vista api.BasesView) fyne.CanvasObject {
	b := p.abierta
	campos := camposDeFormulario(b.Campos)
	if len(vista.Config.Fields) > 0 {
		campos = campos[:0:0]
		for _, c := range b.Campos {
			for _, id := range vista.Config.Fields {
				if c.ID == id {
					campos = append(campos, c)
				}
			}
		}
	}
	hijos := make([]fyne.CanvasObject, 0, 1+len(campos))
	hijos = append(hijos, ui.NuevoRotulo(p.t("form.preview"), tema.TextoSM, ui.Normal, "text-muted"))
	for _, c := range campos {
		hijos = append(hijos, ui.NuevoRotulo("•  "+c.Name+" ("+p.t("field.kind."+c.Kind)+")", tema.TextoSM, ui.Normal, "text"))
	}
	previa := container.New(ui.Columna{Hueco: tema.Espacio(1)}, hijos...)
	if !puedeEsquema(b.Datos.Role) {
		return previa
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, previa, p.panelFormulario(v))
}

// objetoRejilla es Rejilla.tsx con lo que la rodea: la ayuda, «Cargar anteriores», la
// tabla, y debajo «Añadir registro» y «Cargar más».
func (p *Pantalla) objetoRejilla(puedeEditar bool) fyne.CanvasObject {
	b := p.abierta
	if len(b.Campos) == 0 {
		return ui.NuevoRotulo(p.t("grid.noFields"), tema.TextoSM, ui.Normal, "text-muted")
	}
	// La ayuda de teclado no dice nada en una pantalla táctil: ahí se toca la celda o se
	// abre la ficha de la fila. Que no se pueda cambiar, en cambio, se dice siempre.
	// Es un párrafo y no un rótulo: en una ventana estrecha parte en líneas en vez de
	// ensanchar la página entera y sacar de la vista lo que va a la derecha.
	var hijos []fyne.CanvasObject
	ayuda := func(clave string) fyne.CanvasObject {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		return ui.NuevoParrafo(p.t(clave), tema.TextoXS, ui.Normal, "text-muted", v).A(p.anchoContenido())
	}
	switch {
	case !puedeEditar:
		hijos = append(hijos, ayuda("grid.readOnly"))
	case !ui.EsTactil():
		hijos = append(hijos, ayuda("grid.help"))
	}
	if b.vent.primera > 0 {
		hijos = append(hijos, container.New(ui.Fila{},
			botonSM(p.t("grid.loadPrevious"), ui.Fantasma, func() { p.cargarRegistros("antes") })))
	}
	hijos = append(hijos, b.lado)
	if len(b.rejilla.Registros) == 0 {
		hijos = append(hijos, ui.NuevoRotulo(p.t("grid.empty"), tema.TextoSM, ui.Normal, "text-muted"))
	}
	pie := []fyne.CanvasObject{}
	if puedeEditar {
		pie = append(pie, botonSM(p.t("grid.addRecord"), ui.Secundario, p.nuevoRegistro).ConIcono(iconos.Mas, 14))
	}
	if b.vent.siguiente != "" {
		pie = append(pie, botonSM(p.t("grid.loadMore"), ui.Fantasma, func() { p.cargarRegistros("mas") }))
	}
	hijos = append(hijos, container.New(ui.Fila{Hueco: tema.Espacio(2)}, pie...))
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, hijos...)
}

// tope pone su hijo al principio de la línea con el ancho que haya, pero no más de Max
// (`w-full max-w-*`): el selector de vistas no cruza una ventana de escritorio entera.
type tope struct{ Max float32 }

func (t tope) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (t tope) Layout(objs []fyne.CanvasObject, tam fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, (tam.Height-m.Height)/2))
	objs[0].Resize(fyne.NewSize(max(min(tam.Width, t.Max), 0), m.Height))
}
