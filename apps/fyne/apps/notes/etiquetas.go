// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"context"
	"errors"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Paridad web: TagsModal.tsx —el editor de etiquetas CON ESTADO—, el desplegable
// «Filtrar por etiqueta» de la barra y los atajos de estado de NotesApp.tsx.
//
// Una etiqueta aquí es un EJE («Prioridad») con estados excluyentes («Alta», «Baja»), y
// cada estado puede llevar un atajo de un carácter: con una nota seleccionada, pulsarlo
// aplica el estado. Es lo que hace que etiquetar cincuenta notas sea rápido.

// ColoresEtiqueta es la paleta de TagsModal.tsx. El primero es el de un estado nuevo.
var ColoresEtiqueta = []string{"#b0413a", "#8a6a1e", "#16794a", "#0e7490", "#1c58d9", "#86198f", "#565b62"}

// Medidas del editor, de la hoja con el rem de 14.
//
//	fila de estado     `flex items-center gap-1.5`                      5,25
//	icono y atajo      `w-14` y `w-12`                                  49 y 42
//	muestras           `h-5 w-5 rounded-full`, `gap-1`, `ring-offset-1` 17,5, 3,5, 1
//	quitar             `rounded p-1` alrededor de un icono de 14        21
//	fila de la lista   `rounded-md border px-2.5 py-2`, `gap-2`         8,75, 7, 7
//	pastilla           `rounded-full px-2 py-0.5 text-sm`               7, 1,75
const (
	HuecoFilaEstado   = 1.5 * 0.25 * tema.Rem
	AnchoEmojiEstado  = 14 * 0.25 * tema.Rem
	AnchoAtajoEstado  = 12 * 0.25 * tema.Rem
	LadoMuestraEstado = 5 * 0.25 * tema.Rem
	HuecoMuestraEst   = 1 * 0.25 * tema.Rem
	GlifoQuitarEstado = 14
	PXFilaEtiqueta    = 2.5 * 0.25 * tema.Rem
	PYFilaEtiqueta    = 2 * 0.25 * tema.Rem
	PXPastilla        = 2 * 0.25 * tema.Rem
	PYPastilla        = 0.5 * 0.25 * tema.Rem
)

// borradorEtiqueta es lo que el editor va cambiando antes de guardar.
type borradorEtiqueta struct {
	id      string
	nombre  string
	estados []api.TagStateInput
}

// nuevaEtiqueta nace con UN estado: una etiqueta sin estados no se puede aplicar a nada,
// así que no tiene sentido guardarla. Es el `NUEVA()` de la web.
func nuevaEtiqueta() *borradorEtiqueta {
	return &borradorEtiqueta{estados: []api.TagStateInput{{Color: ColoresEtiqueta[0]}}}
}

// borradorDe copia una etiqueta guardada. Los estados conservan su ID: con él el
// servicio los mantiene, y con ellos las notas que ya los llevan.
func borradorDe(tag api.Tag) *borradorEtiqueta {
	b := &borradorEtiqueta{id: tag.ID, nombre: tag.Name}
	for _, s := range tag.States {
		b.estados = append(b.estados, api.TagStateInput{
			ID: ptr(s.ID), Name: s.Name, Emoji: s.Emoji, Color: s.Color, Shortcut: s.Shortcut,
		})
	}
	return b
}

// entrada es lo que se manda: el nombre sin blancos y solo los estados con nombre. `false`
// si no hay nada que guardar, que es cuando la web no hace nada al pulsar «Guardar».
func (b *borradorEtiqueta) entrada() (api.TagInput, bool) {
	nombre := strings.TrimSpace(b.nombre)
	var estados []api.TagStateInput
	for _, s := range b.estados {
		if strings.TrimSpace(s.Name) != "" {
			estados = append(estados, s)
		}
	}
	if nombre == "" || len(estados) == 0 {
		return api.TagInput{}, false
	}
	return api.TagInput{Name: nombre, States: estados}, true
}

// añadirEstado es «Añadir estado»: el color siguiente de la paleta, dando la vuelta.
func (b *borradorEtiqueta) añadirEstado() {
	b.estados = append(b.estados, api.TagStateInput{Color: ColoresEtiqueta[len(b.estados)%len(ColoresEtiqueta)]})
}

// enTelefono es el `max-md:` de la web: la ventana por debajo de 768.
func (p *Pantalla) enTelefono() bool {
	return p.win != nil && p.win.Canvas().Size().Width > 0 && p.win.Canvas().Size().Width < ui.Estrecho
}

// dialogoEtiquetas es TagsModal.tsx: la lista de etiquetas y, al elegir una o crear una
// nueva, su editor en el MISMO diálogo.
func (p *Pantalla) dialogoEtiquetas() { p.abrirEtiquetas(nil) }

// abrirEtiquetas abre el diálogo en la lista o, con `b`, directamente en su editor.
func (p *Pantalla) abrirEtiquetas(b *borradorEtiqueta) {
	if p.win == nil {
		return
	}
	cuerpo := container.New(ui.Columna{})
	d := ui.NuevoDialogo(p.tr.T("tags.title", nil), cuerpo).Con(ui.DialogoMD, nil)
	ancho := anchoCuerpoAjustes(p.win.Canvas().Size())
	var editando *borradorEtiqueta
	var foco fyne.Focusable
	var pintar func()
	editar := func(b *borradorEtiqueta) {
		editando = b
		pintar()
		// `autofocus` en el nombre al abrir el editor, y solo entonces: repintar por
		// elegir un color no debe llevarse el foco de donde esté.
		if b != nil && foco != nil {
			p.win.Canvas().Focus(foco)
		}
	}
	pintar = func() {
		var o, pie fyne.CanvasObject
		if editando == nil {
			o, foco = p.listaEtiquetas(editar, pintar), nil
		} else {
			o, foco, pie = p.formEtiqueta(editando, ancho, pintar, func() { editar(nil) })
		}
		cuerpo.Objects = []fyne.CanvasObject{o}
		cuerpo.Refresh()
		d.CambiarPie(pie)
	}
	p.mostrar(d)
	editar(b)
}

// listaEtiquetas es la vista de lista: cada etiqueta con sus estados, «Cambiar el
// nombre» y «Eliminar», y debajo «Nueva etiqueta».
func (p *Pantalla) listaEtiquetas(editar func(*borradorEtiqueta), repintar func()) fyne.CanvasObject {
	v, t := p.variante(), p.tr.T
	piezas := []fyne.CanvasObject{}
	if len(p.etiquetas) == 0 {
		piezas = append(piezas, lineaTok(t("tags.none", nil), tema.TextoBase, ui.Normal, "text-muted", v))
	} else {
		filas := make([]fyne.CanvasObject, 0, len(p.etiquetas))
		for _, tag := range p.etiquetas {
			pastillas := make([]fyne.CanvasObject, 0, len(tag.States))
			for _, s := range tag.States {
				pastillas = append(pastillas, pastillaEstado(s, v))
			}
			renombrar := ui.NuevoBoton(t("notebook.rename", nil), func() { editar(borradorDe(tag)) }).
				Con(ui.Fantasma, ui.TallaXS)
			borrar := ui.NuevoBoton(t("common.delete", nil), func() {
				p.confirmar(t("tags.delete", nil), t("tags.deleteConfirm", i18n.Vars{"name": tag.Name}),
					t("common.delete", nil), func() {
						go func() {
							if p.borrarEtiqueta(tag.ID) == nil {
								enHilo(repintar)
							}
						}()
					})
			}).Con(ui.Fantasma, ui.TallaXS)
			estados := container.New(ui.FilaAuto{Hueco: tema.Espacio(1), Derecha: len(pastillas), Partir: true, SaltoFila: tema.Espacio(1)}, pastillas...)
			var fila fyne.CanvasObject = container.New(ui.FilaAuto{Hueco: HuecoFilaDialogo, Derecha: 2, Partir: true, SaltoFila: HuecoFilaDialogo, Elastico: 2},
				lineaTok(tag.Name, tema.TextoBase, ui.Medio, "text", v), estados, renombrar, borrar)
			if p.enTelefono() {
				// Los estados en su línea, a todo el ancho, bajo el nombre y sus acciones.
				estados.Layout = ui.FilaAuto{Hueco: tema.Espacio(1), Partir: true, SaltoFila: tema.Espacio(1)}
				fila = container.New(ui.Columna{Hueco: HuecoFilaDialogo},
					container.New(&filaElastica{hueco: HuecoFilaDialogo, elastico: 0},
						lineaTok(tag.Name, tema.TextoBase, ui.Medio, "text", v), renombrar, borrar),
					estados)
			}
			filas = append(filas, conBorde(fila, PXFilaEtiqueta, PYFilaEtiqueta, "surface-raised", v))
		}
		piezas = append(piezas, container.New(ui.Columna{Hueco: 1 * 0.25 * tema.Rem}, filas...))
	}
	piezas = append(piezas, ui.NuevoBoton(t("tags.new", nil), func() { editar(nuevaEtiqueta()) }).
		Con(ui.Secundario, ui.TallaMD))
	return container.New(ui.Columna{Hueco: 2 * 0.25 * tema.Rem}, piezas...)
}

// pastillaEstado es un estado en la lista: el color del estado de fondo y el texto en
// `on-primary`, que es el de la web (`text-[var(--sytue-on-primary)]`).
func pastillaEstado(s api.TagState, v tema.Variante) fyne.CanvasObject {
	rotulo := s.Name
	if s.Emoji != "" {
		rotulo = s.Emoji + " " + s.Name
	}
	fondo := hexO(s.Color, tema.Color("primary", v))
	caja := ui.NuevaCaja(fondo, fondo, tema.RadioPill)
	dentro := container.New(ui.Margen{Izq: PXPastilla, Der: PXPastilla, Arr: PYPastilla, Abj: PYPastilla},
		lineaTok(rotulo, tema.TextoSM, ui.Normal, "on-primary", v))
	return container.New(&enCaja{caja: caja}, append(caja.Objetos(), dentro)...)
}

// conBorde mete un hijo en una caja `rounded-md border` con relleno distinto en cada
// eje: el `px-N py-M` de una fila suelta. El borde ocupa sitio (`border-box`).
func conBorde(dentro fyne.CanvasObject, px, py float32, fondo string, v tema.Variante) fyne.CanvasObject {
	caja := ui.NuevaCaja(tema.Color(fondo, v), tema.Color("border", v), tema.RadioMD)
	g := tema.Borde
	return container.New(&enCaja{caja: caja}, append(caja.Objetos(),
		container.New(ui.Margen{Izq: px + g, Der: px + g, Arr: py + g, Abj: py + g}, dentro))...)
}

// formEtiqueta es la vista de edición: el nombre, un renglón por estado —icono, nombre,
// atajo, color y quitar— y los botones de guardar y cancelar.
func (p *Pantalla) formEtiqueta(b *borradorEtiqueta, ancho float32, repintar, cerrar func()) (forma fyne.CanvasObject, foco fyne.Focusable, pie fyne.CanvasObject) {
	v, t := p.variante(), p.tr.T
	nombre := ui.NuevaEntrada(t("tags.name", nil))
	nombre.Ancho = ancho
	nombre.PonerTexto(b.nombre)
	nombre.AlCambiar(func(s string) { b.nombre = s })

	filas := make([]fyne.CanvasObject, 0, len(b.estados))
	for i := range b.estados {
		filas = append(filas, p.filaEstado(b, i, repintar, v))
	}

	guardar := ui.NuevoBoton(t("common.save", nil), nil)
	guardar.AlPulsar = func() {
		in, ok := b.entrada()
		if !ok || guardar.Cargando {
			return
		}
		guardar.Cargando = true
		guardar.Refresh()
		go func() {
			err := p.guardarEtiqueta(b.id, in)
			enHilo(func() {
				guardar.Cargando = false
				guardar.Refresh()
				if err == nil {
					cerrar()
				}
			})
		}()
	}
	cancelar := ui.NuevoBoton(t("common.cancel", nil), cerrar).Con(ui.Fantasma, ui.TallaMD)
	añadir := ui.NuevoBoton(t("tags.addState", nil), func() { b.añadirEstado(); repintar() }).
		Con(ui.Secundario, ui.TallaSM)

	estados := make([]fyne.CanvasObject, 0, len(filas)+3)
	estados = append(estados, lineaTok(t("tags.states", nil), tema.TextoBase, ui.Medio, "text", v))
	estados = append(estados, filas...)
	estados = append(estados,
		ui.NuevoParrafo(t("tags.shortcutHint", nil), tema.TextoXS, ui.Normal, "text-subtle", v).A(ancho),
		añadir)

	// «Cancelar» y «Guardar» van en el PIE del diálogo, como en la web.
	pie = container.New(ui.Fila{Hueco: HuecoFilaDialogo}, cancelar, guardar)
	forma = container.New(ui.Columna{Hueco: 3 * 0.25 * tema.Rem},
		nombre, container.New(ui.Columna{Hueco: 2 * 0.25 * tema.Rem}, estados...))
	return forma, nombre, pie
}

// filaEstado es un renglón del editor de estados.
//
// ponytail: `text-center` en el icono y el atajo no se hace —`widget.Entry` no alinea al
// centro sin reescribirlo— y el texto va a la izquierda de su caja.
func (p *Pantalla) filaEstado(b *borradorEtiqueta, i int, repintar func(), v tema.Variante) fyne.CanvasObject {
	t := p.tr.T
	s := &b.estados[i]
	emoji := campoCorto("", s.Emoji, 4, AnchoEmojiEstado, func(x string) { s.Emoji = x })
	nombre := ui.NuevaEntrada(t("tags.stateName", nil))
	nombre.PonerTexto(s.Name)
	nombre.AlCambiar(func(x string) { s.Name = x })
	atajo := campoCorto("", s.Shortcut, 1, AnchoAtajoEstado, func(x string) { s.Shortcut = x })

	muestras := make([]fyne.CanvasObject, 0, len(ColoresEtiqueta))
	for _, hex := range ColoresEtiqueta {
		m := NuevaMuestra(hexO(hex, tema.Color("primary", v)), LadoMuestraEstado, HuecoMuestraEst, hex,
			func() { s.Color = hex; repintar() })
		m.Activa = strings.EqualFold(s.Color, hex)
		muestras = append(muestras, m)
	}
	colores := container.New(ui.Fila{Hueco: conCeldaTactil(muestras, 1*0.25*tema.Rem)}, muestras...)
	piezas := []fyne.CanvasObject{emoji, nombre, atajo}
	if !p.enTelefono() {
		piezas = append(piezas, colores)
	}
	if len(b.estados) > 1 {
		quitar := ui.NuevoBotonIcono(iconos.Cerrar, t("common.delete", nil), func() {
			b.estados = append(b.estados[:i:i], b.estados[i+1:]...)
			repintar()
		})
		quitar.Lado, quitar.TamIcono, quitar.Radio = GlifoQuitarEstado+2*0.25*tema.Rem, GlifoQuitarEstado, 0.25*tema.Rem
		if ui.EsTactil() {
			quitar.Lado = ui.SueloTactilBarra // `pointer-coarse:h-11 w-11`
		}
		piezas = append(piezas, quitar)
	}
	fila := container.New(&filaElastica{hueco: HuecoFilaEstado, elastico: 1}, piezas...)
	if !p.enTelefono() {
		return fila
	}
	// En el teléfono, los colores bajan a su línea, bajo el nombre (`max-md:order-last
	// basis-full`): en una, el nombre del estado se quedaba en 28 y no se leía.
	return container.New(ui.Columna{Hueco: HuecoFilaEstado}, fila,
		container.New(ui.Margen{Izq: AnchoEmojiEstado + HuecoFilaEstado}, container.New(ui.Fila{}, colores)))
}

// campoCorto es un `<Input maxLength>` de ancho fijo: el icono de un estado (4) y su
// atajo (1). `widget.Entry` no tiene tope, así que lo que sobra se corta al escribir.
func campoCorto(marca, valor string, tope int, ancho float32, alCambiar func(string)) *ui.Entrada {
	e := ui.NuevaEntrada(marca)
	e.Ancho = ancho
	e.PonerTexto(valor)
	e.AlCambiar(func(s string) {
		if r := []rune(s); len(r) > tope {
			e.PonerTexto(string(r[:tope]))
			return
		}
		alCambiar(s)
	})
	return e
}

// guardarEtiqueta crea (`id` vacío) o sustituye una etiqueta y vuelve a leerlas todas,
// que es lo que hace la web (`setTags(await api.listTags())`).
func (p *Pantalla) guardarEtiqueta(id string, in api.TagInput) error {
	if p.svc == nil || !p.hayRed() {
		return errSinServicio
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if _, err := p.svc.SaveTag(ctx, id, in); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), Motivo(err))
		return err
	}
	p.releerEtiquetas(ctx)
	return nil
}

// borrarEtiqueta borra una etiqueta, sus estados y sus marcas en todas las notas.
func (p *Pantalla) borrarEtiqueta(id string) error {
	if p.svc == nil || !p.hayRed() {
		return errSinServicio
	}
	ctx, cancelar := p.ctx()
	defer cancelar()
	if err := p.svc.DeleteTag(ctx, id); err != nil {
		p.avisar(ui.AvisoPeligro, p.tr.T("note.saveFailed", nil), Motivo(err))
		return err
	}
	p.releerEtiquetas(ctx)
	// Las insignias de las notas salen de la lista de notas: sin volver a leerla, la
	// etiqueta borrada seguiría pintada en ellas.
	p.releerNotas(ctx)
	return nil
}

// releerEtiquetas vuelve a pedir las etiquetas y las deja en la pantalla.
//
//nolint:contextcheck // repintar la pantalla no es de esta petición: lo que guarda al repintar (cerrar el editor) lleva su propio plazo
func (p *Pantalla) releerEtiquetas(ctx context.Context) {
	tags, err := p.svc.ListTags(ctx)
	if err != nil {
		return
	}
	enHilo(func() { p.etiquetas = tags; p.Repintar() })
}

// errSinServicio es una escritura pedida sin servicio o sin red: no se hace nada, y
// quien la llamó no debe cerrar lo que tenga abierto como si hubiera salido.
var errSinServicio = errors.New("notes: sin servicio o sin conexión")
