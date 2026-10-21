// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Reglas de correo» (`FiltersModal.tsx`)
//
// La lista de reglas —activar y desactivar con su interruptor, borrar— y un ASISTENTE
// de tres pasos para crear una: qué correos, qué hacer con ellos y el resumen contado
// en una frase. Las aplica el servidor al correo que va llegando.
//
// «Crear regla» desde un correo abre el asistente con el remitente puesto, que es lo
// que dice el comentario de la web (`initial`: «abre el asistente ya relleno»). La web
// NO lo hace: su efecto de carga pone `step` a null después del que lo abría, así que
// enseña la lista y el remitente se pierde al pulsar «Crear regla» (dicho en el
// informe).

// GestorReglas es lo que el diálogo le pide al servidor. Lo cumple *api.Mail.
type GestorReglas interface {
	Filters(ctx context.Context) ([]api.MailFilter, error)
	CreateFilter(ctx context.Context, f api.MailFilter) (api.MailFilter, error)
	UpdateFilter(ctx context.Context, f api.MailFilter) error
	DeleteFilter(ctx context.Context, id string) error
}

var _ GestorReglas = (*api.Mail)(nil)

// BorradorRegla es el `Draft` del asistente.
type BorradorRegla struct {
	Nombre, De, Asunto string
	// MoverA es la carpeta de destino; vacía, no mueve.
	MoverA           string
	Leido, Destacado bool
}

func (d BorradorRegla) conCondicion() bool {
	return strings.TrimSpace(d.De) != "" || strings.TrimSpace(d.Asunto) != ""
}

func (d BorradorRegla) conAccion() bool { return d.MoverA != "" || d.Leido || d.Destacado }

// Las frases de la regla. `carpeta` pone nombre a un id de carpeta (`mailboxName`).

// CondicionRegla es `conditionText`.
func CondicionRegla(t *i18n.Translator, l i18n.Locale, de, asunto string) string {
	var partes []string
	if de != "" {
		partes = append(partes, t.T("rules.conditionFrom", i18n.Vars{"value": de}))
	}
	if asunto != "" {
		partes = append(partes, t.T("rules.conditionSubject", i18n.Vars{"value": asunto}))
	}
	if len(partes) == 0 {
		return t.T("rules.conditionAny", nil)
	}
	return i18n.FormatList(l, partes)
}

// AccionRegla es `actionText`.
func AccionRegla(t *i18n.Translator, l i18n.Locale, carpeta func(string) string, mover string, leido, destacado bool) string {
	var partes []string
	if mover != "" {
		partes = append(partes, t.T("rules.actionMove", i18n.Vars{"folder": carpeta(mover)}))
	}
	if leido {
		partes = append(partes, t.T("rules.actionRead", nil))
	}
	if destacado {
		partes = append(partes, t.T("rules.actionFlag", nil))
	}
	if len(partes) == 0 {
		return t.T("rules.actionNone", nil)
	}
	return i18n.FormatList(l, partes)
}

// FraseRegla es `ruleSentence`: la regla entera en una frase del catálogo, porque el
// orden de «quién» y «qué hago» cambia entre lenguas.
func FraseRegla(t *i18n.Translator, l i18n.Locale, carpeta func(string) string, d BorradorRegla) string {
	de, asunto := strings.TrimSpace(d.De), strings.TrimSpace(d.Asunto)
	var quien string
	switch {
	case de != "" && asunto != "":
		quien = t.T("rules.whoFromSubject", i18n.Vars{"from": de, "subject": asunto})
	case de != "":
		quien = t.T("rules.whoFrom", i18n.Vars{"from": de})
	default:
		quien = t.T("rules.whoSubject", i18n.Vars{"subject": asunto})
	}
	var hace []string
	if d.MoverA != "" {
		hace = append(hace, t.T("rules.doMove", i18n.Vars{"folder": carpeta(d.MoverA)}))
	}
	if d.Leido {
		hace = append(hace, t.T("rules.doRead", nil))
	}
	if d.Destacado {
		hace = append(hace, t.T("rules.doFlag", nil))
	}
	if len(hace) == 0 {
		return t.T("rules.sentenceNothing", i18n.Vars{"who": quien})
	}
	return t.T("rules.sentence", i18n.Vars{"who": quien, "actions": i18n.FormatList(l, hace)})
}

// eleccionRol es `ROLE_CHOICE`: las carpetas con consecuencia propia tienen su tarjeta.
var eleccionRol = map[string]struct {
	titulo, descripcion string
	icono               iconos.Icono
}{
	"archive": {"rules.archiveTitle", "rules.archiveDescription", iconos.Cubo},
	"junk":    {"rules.junkTitle", "rules.junkDescription", iconos.Aviso},
	"trash":   {"rules.trashTitle", "rules.trashDescription", iconos.Cerrar},
}

// rolesConTarjeta es el orden de las tarjetas: el de las carpetas en el lateral.
var rolesConTarjeta = []string{"archive", "junk", "trash"}

// Medidas de la web.
const (
	huecoAsistente   float32 = 6 * 0.25 * tema.Rem   // `gap-6` del Wizard
	descAsistenteMax float32 = 42 * tema.Rem         // `max-w-2xl` de la descripción
	ladoBorrarRegla  float32 = 7 * 0.25 * tema.Rem   // `h-7 w-7`
	colResumen       float32 = 11 * tema.Rem         // `minmax(0,11rem)`
	margenYResumen   float32 = 2.5 * 0.25 * tema.Rem // `py-2.5`
)

type gestorReglas struct {
	b       *Buzon
	d       *ui.Dialogo
	caja    *fyne.Container
	ancho   float32
	ventana float32
	estado  Estado
	lista   []api.MailFilter

	// El asistente: paso < 0 es la lista.
	paso      int
	borrador  BorradorRegla
	otra      bool // «Moverlo a otra carpeta»: el destino sale del desplegable
	guardando bool

	de, asunto, nombre *ui.Entrada
	leido, destacado   *ui.Eleccion
	selector           *ui.Selector
	pasos              *ui.Pasos
}

// verReglas abre «Reglas de correo». Con un borrador, directamente el asistente.
func (b *Buzon) verReglas(inicial *BorradorRegla) *gestorReglas {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return nil
	}
	t := b.P.T
	g := &gestorReglas{b: b, ancho: AnchoCuerpoDialogo(ui.DialogoLG, lienzo.Size().Width), ventana: lienzo.Size().Width, paso: -1}
	g.de = ui.NuevaEntrada(t.T("rules.fromPlaceholder", nil))
	g.asunto = ui.NuevaEntrada(t.T("rules.subjectPlaceholder", nil))
	g.nombre = ui.NuevaEntrada(t.T("rules.namePlaceholder", nil))
	g.de.AlCambiar(func(s string) { g.borrador.De = s; g.pintar() })
	g.asunto.AlCambiar(func(s string) { g.borrador.Asunto = s; g.pintar() })
	g.nombre.AlCambiar(func(s string) { g.borrador.Nombre = s })
	g.leido = ui.NuevaCasilla(t.T("rules.alsoRead", nil), func(si bool) { g.borrador.Leido = si; g.pintar() })
	g.destacado = ui.NuevaCasilla(t.T("rules.alsoFlag", nil), func(si bool) { g.borrador.Destacado = si; g.pintar() })
	g.pasos = ui.NuevosPasos(t.T("rules.stepWhichTitle", nil), t.T("rules.stepActionTitle", nil), t.T("rules.stepSummaryTitle", nil))
	g.pasos.Ventana = g.ventana
	g.pasos.RotuloIr = func(n int) string { return t.T("ui.wizard.goToStep", i18n.Vars{"n": n}) }
	g.pasos.AlElegir = g.irA
	g.caja = container.New(ui.Columna{})
	g.cargar()
	if inicial != nil {
		g.empezar(*inicial)
	} else {
		g.abrirDialogo()
	}
	return g
}

// abrirDialogo pone el diálogo de la lista (con «Cerrar») o el del asistente (con su
// botonera en el pie, ver asistente). Son dos diálogos
// porque el pie de uno no se puede quitar en marcha; se ven como uno, sin animar.
func (g *gestorReglas) abrirDialogo() {
	lienzo := g.b.P.Lienzo()
	if lienzo == nil {
		return
	}
	if g.d != nil {
		g.d.Cerrar()
	}
	t := g.b.P.T
	if g.paso < 0 {
		cerrar := ui.NuevoBoton(t.T("common.close", nil), nil).Con(ui.Secundario, ui.TallaMD)
		g.d = ui.NuevoDialogo(t.T("rules.title", nil), g.caja).Con(ui.DialogoLG, cerrar)
		cerrar.AlPulsar = g.d.Cerrar
	} else {
		g.d = ui.NuevoDialogo(t.T("rules.newTitle", nil), g.caja).Con(ui.DialogoLG, nil)
		g.d.Foco = g.de
	}
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.pintar()
	g.d.Mostrar(lienzo)
}

func (g *gestorReglas) cargar() {
	g.estado = Cargando
	g.pintar()
	pedir(g.b, g.b.S.Filters, func(xs []api.MailFilter, err error) {
		if err != nil {
			g.estado = Error
		} else {
			g.lista, g.estado = xs, Listo
		}
		g.pintar()
	})
}

// empezar es `startWizard` (o el asistente ya relleno).
func (g *gestorReglas) empezar(d BorradorRegla) {
	g.borrador, g.otra, g.paso = d, false, 0
	g.de.PonerTexto(d.De)
	g.asunto.PonerTexto(d.Asunto)
	g.nombre.PonerTexto(d.Nombre)
	g.leido.Marcada, g.destacado.Marcada = d.Leido, d.Destacado
	g.borrador = d // PonerTexto dispara AlCambiar, pero el borrador ya es el bueno
	g.abrirDialogo()
}

// irA cambia de paso dentro del asistente.
func (g *gestorReglas) irA(n int) {
	g.paso = n
	g.d.Foco = nil
	g.pintar()
}

// volver es «Volver a las reglas»: el asistente se cierra y se ve la lista.
func (g *gestorReglas) volver() {
	g.paso = -1
	g.abrirDialogo()
}

// carpeta es `mailboxName`.
func (g *gestorReglas) carpeta(id string) string {
	for _, m := range g.b.carpetas {
		if m.ID == id {
			return RotuloCarpeta(g.b.P.T, m.Name, m.Role)
		}
	}
	return g.b.P.T.T("rules.someFolder", nil)
}

func (g *gestorReglas) pintar() {
	if g.paso >= 0 {
		g.caja.Objects = []fyne.CanvasObject{g.asistente()}
	} else {
		g.caja.Objects = []fyne.CanvasObject{g.vistaLista()}
	}
	g.caja.Refresh()
	if g.d != nil {
		g.d.Recolocar()
	}
}

// ---- La lista ----

func (g *gestorReglas) vistaLista() fyne.CanvasObject {
	t, l := g.b.P.T, g.b.P.Locale
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	crear := func() *ui.Boton {
		b := ui.NuevoBoton(t.T("rules.create", nil), func() { g.empezar(BorradorRegla{}) }).Con(ui.Primario, ui.TallaMD)
		b.Icono = iconos.Mas
		return b
	}
	var dentro fyne.CanvasObject
	switch {
	case g.estado == Cargando:
		rueda := ui.NuevaRueda(16, "text-subtle", v)
		rueda.Rotulo = t.T("rules.loading", nil)
		dentro = container.New(ui.Margen{Arr: tema.Espacio(4), Abj: tema.Espacio(4)},
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, rueda,
				ui.NuevoRotulo(t.T("common.loading", nil), tema.TextoSM, ui.Normal, "text-subtle")))
	case g.estado == Error:
		icono := iconos.Trazo(iconos.Aviso, 16, "text", v)
		dentro = container.New(ui.Margen{Arr: tema.Espacio(2), Abj: tema.Espacio(2)},
			container.New(ui.Columna{Hueco: tema.Espacio(2)},
				container.New(ui.Fila{Hueco: tema.Espacio(2)}, container.New(ui.Margen{Arr: tema.Espacio(0.5)}, icono.Objeto()),
					ui.NuevoRotulo(t.T("rules.loadError", nil), tema.TextoSM, ui.Normal, "text")),
				container.New(ui.Fila{}, ui.NuevoBoton(t.T("common.retry", nil), g.cargar).Con(ui.Secundario, ui.TallaSM))))
	case len(g.lista) == 0:
		vacio := ui.NuevoVacio(t.T("rules.emptyTitle", nil), t.T("rules.emptyDescription", nil))
		vacio.Ilustracion = iconos.Trazo(iconos.Ajustes, 32, "text-subtle", v).Objeto()
		accion := crear()
		accion.Texto = t.T("rules.emptyAction", nil)
		vacio.Accion = accion
		dentro = vacio
	default:
		filas := make([]fyne.CanvasObject, 0, len(g.lista))
		for _, f := range g.lista {
			filas = append(filas, g.fila(f, t, l, v))
		}
		dentro = enCajaConFiletes(filas)
	}
	return seccionCon(t.T("rules.yoursTitle", nil), t.T("rules.yoursDescription", nil), g.ancho, crear(), dentro)
}

// fila es una regla: su nombre (o su condición) y «condición → acción», el interruptor
// y el aspa de borrar, que es de 24,5 y se tiñe de peligro al pasar.
func (g *gestorReglas) fila(f api.MailFilter, t *i18n.Translator, l i18n.Locale, v tema.Variante) fyne.CanvasObject {
	cond := CondicionRegla(t, l, f.MatchFrom, f.MatchSubject)
	nombre := f.Name
	if nombre == "" {
		nombre = cond
	}
	sw := ui.NuevoConmutador(t.T("rules.enabled", nil), func(bool) { g.alternar(f) })
	sw.Escalon, sw.Encendido = tema.TextoSM, f.Enabled
	borrar := ui.NuevoBotonIcono(iconos.Cerrar, t.T("rules.deleteRule", i18n.Vars{"rule": nombre}), func() { g.borrar(f) })
	borrar.Lado, borrar.TamIcono, borrar.Radio = ladoBorrarRegla, 16, tema.RadioSM
	borrar.Trazo, borrar.TrazoEncima, borrar.FondoEncima = "text-subtle", "danger-text", "bg-subtle"
	if ui.EsTactil() {
		borrar.Lado = ui.SueloTactilBarra // `pointer-coarse:min-*-11`, como la web
	}
	margen := ui.Margen{Izq: margenXFilaEtiqueta, Der: margenXFilaEtiqueta, Arr: margenYFilaEtiqueta, Abj: margenYFilaEtiqueta}
	regla := cond + " → " + AccionRegla(t, l, g.carpeta, f.MoveTo, f.MarkRead, f.MarkFlagged)
	if enTelefono(g.b) {
		// `max-md:flex-col`: la regla entera arriba (la condición y la acción parten de
		// línea) y «Activa» y borrar debajo. Al lado se quedaban en 169 y no se leían.
		return container.New(margen, container.New(ui.Columna{Hueco: tema.Espacio(1)},
			textoRecortado(nombre, tema.TextoSM, ui.Medio, "text", v),
			ui.NuevoParrafo(regla, tema.TextoXS, ui.Normal, "text-subtle", v),
			container.New(extremos{centro: true}, sw, borrar)))
	}
	derecha := container.New(ui.Fila{Hueco: tema.Espacio(2)}, sw, borrar)
	texto := container.New(ui.Columna{},
		textoRecortado(nombre, tema.TextoSM, ui.Medio, "text", v),
		textoRecortado(regla, tema.TextoXS, ui.Normal, "text-subtle", v))
	return container.New(margen, container.New(extremos{}, texto, derecha))
}

// alternar es `handleToggle`: la regla ENTERA con el estado cambiado (el servidor
// sustituye), optimista y de vuelta si falla.
func (g *gestorReglas) alternar(f api.MailFilter) {
	f.Enabled = !f.Enabled
	g.ponerEnLista(f)
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.UpdateFilter(ctx, f)
	}, func(_ struct{}, err error) {
		if err != nil {
			f.Enabled = !f.Enabled
			g.ponerEnLista(f)
			g.b.avisar(ui.AvisoPeligro, "rules.toggleFailed", nil)
		}
	})
}

func (g *gestorReglas) ponerEnLista(f api.MailFilter) {
	for i := range g.lista {
		if g.lista[i].ID == f.ID {
			g.lista[i] = f
		}
	}
	g.pintar()
}

// borrar es `handleDelete`: optimista, y de vuelta si falla.
func (g *gestorReglas) borrar(f api.MailFilter) {
	antes := slices.Clone(g.lista)
	g.lista = slices.DeleteFunc(g.lista, func(x api.MailFilter) bool { return x.ID == f.ID })
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.DeleteFilter(ctx, f.ID)
	}, func(_ struct{}, err error) {
		if err != nil {
			g.lista = antes
			g.pintar()
			g.b.avisar(ui.AvisoPeligro, "rules.deleteFailed", nil)
		}
	})
}

// ---- El asistente ----

// destinos son las carpetas a las que se puede mover el correo que llega: todas menos
// Borradores y Enviados. Las de sistema con consecuencia propia van en su tarjeta; las
// demás, en el desplegable de «otra carpeta».
func (g *gestorReglas) destinos() (roles map[string]api.Mailbox, otras []api.Mailbox) {
	roles = map[string]api.Mailbox{}
	for _, m := range g.b.carpetas {
		switch _, propio := eleccionRol[m.Role]; {
		case m.Role == "drafts" || m.Role == "sent":
		case propio:
			roles[m.Role] = m
		default:
			otras = append(otras, m)
		}
	}
	return roles, otras
}

func (g *gestorReglas) puedeSeguir() bool {
	switch g.paso {
	case 0:
		return g.borrador.conCondicion()
	case 1:
		return g.borrador.conAccion() && (!g.otra || g.borrador.MoverA != "")
	}
	return g.borrador.conCondicion() && g.borrador.conAccion()
}

func (g *gestorReglas) asistente() fyne.CanvasObject {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	titulos := [][2]string{
		{"rules.stepWhichHeading", "rules.stepWhichDescription"},
		{"rules.stepActionHeading", "rules.stepActionDescription"},
		{"rules.stepSummaryHeading", "rules.stepSummaryDescription"},
	}[g.paso]
	g.pasos.Actual = g.paso
	g.pasos.AlElegir = g.irA
	if g.guardando {
		g.pasos.AlElegir = nil
	}
	g.pasos.Refresh()
	cabeza := container.New(ui.Columna{Hueco: tema.Espacio(1.5)},
		ui.NuevoParrafo(t.T(titulos[0], nil), tema.TextoXL, ui.SemiNegrita, "text", v).A(g.ancho),
		ui.NuevoParrafo(t.T(titulos[1], nil), tema.TextoBase, ui.Normal, "text-muted", v).A(min(g.ancho, descAsistenteMax)))

	var contenido fyne.CanvasObject
	switch g.paso {
	case 0:
		contenido = g.pasoQue()
	case 1:
		contenido = g.pasoAccion()
	default:
		contenido = g.pasoResumen(v)
	}

	// La botonera: `border-t border-border-subtle pt-4`, cancelar a la izquierda y
	// «Atrás» y el principal empujados a la derecha.
	cancelar := ui.NuevoBoton(t.T("rules.backToRules", nil), g.volver).Con(ui.Fantasma, ui.TallaMD)
	cancelar.Inerte = g.guardando
	derecha := []fyne.CanvasObject{}
	if g.paso > 0 {
		atras := ui.NuevoBoton(t.T("ui.wizard.back", nil), func() { g.irA(g.paso - 1) }).Con(ui.Secundario, ui.TallaMD)
		atras.Icono, atras.TamIcono, atras.Inerte = iconos.ChevronIzquierda, 16, g.guardando
		derecha = append(derecha, atras)
	}
	siguiente := ui.NuevoBoton(t.T("ui.wizard.next", nil), func() { g.irA(g.paso + 1) }).Con(ui.Primario, ui.TallaMD)
	if g.paso == 2 {
		siguiente.Texto, siguiente.AlPulsar = t.T("rules.create", nil), g.crear
	}
	siguiente.Inerte, siguiente.Cargando = !g.puedeSeguir(), g.guardando
	derecha = append(derecha, siguiente)
	// Va en el PIE del diálogo, con su filete: dentro del cuerpo, en el teléfono el paso
	// 2 dejaba «Continuar» fuera de la vista. Es el `sticky bottom-0` de Wizard.tsx
	// dentro de un diálogo.
	if g.d != nil {
		g.d.CambiarPie(container.New(anchoLibre{}, container.New(extremos{hueco: tema.Espacio(2), centro: true, desborda: true}, container.New(ui.Fila{}, cancelar),
			container.New(ui.Fila{Hueco: tema.Espacio(2)}, derecha...))))
	}

	return container.New(ui.Columna{Hueco: huecoAsistente}, g.pasos, cabeza, contenido)
}

// campoRegla es un FieldGroup con su pista repartida al ancho del campo.
func (g *gestorReglas) campoRegla(rotulo string, control fyne.CanvasObject, pista string, tono ui.TonoAyuda) *fyne.Container {
	c := ui.GrupoCampo(rotulo, false, control, "", "")
	p := ui.NuevaPistaCampo(pista).ConTono(tono)
	p.Ancho = g.ancho
	c.Add(p)
	return c
}

func (g *gestorReglas) pasoQue() fyne.CanvasObject {
	t := g.b.P.T
	pista, tono := t.T("rules.subjectHintBoth", nil), ui.AyudaNeutra
	if g.borrador.conCondicion() {
		pista, tono = t.T("rules.subjectHintOk", nil), ui.AyudaExito
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(4)},
		g.campoRegla(t.T("rules.fromLabel", nil), g.de, t.T("rules.fromHint", nil), ui.AyudaNeutra),
		g.campoRegla(t.T("rules.subjectLabel", nil), g.asunto, pista, tono))
}

func (g *gestorReglas) pasoAccion() fyne.CanvasObject {
	t := g.b.P.T
	roles, otras := g.destinos()
	tarjeta := func(titulo, desc string, icono iconos.Icono, elegida bool, elegir func()) fyne.CanvasObject {
		c := ui.NuevaTarjetaEleccion(titulo, desc, func() { elegir(); g.pintar() })
		c.Icono, c.Elegida, c.Ancho = icono, elegida, g.ancho
		return c
	}
	grupo := []fyne.CanvasObject{
		tarjeta(t.T("rules.keepInboxTitle", nil), t.T("rules.keepInboxDescription", nil), iconos.Informacion,
			!g.otra && g.borrador.MoverA == "", func() { g.otra, g.borrador.MoverA = false, "" }),
	}
	for _, rol := range rolesConTarjeta {
		m, hay := roles[rol]
		if !hay {
			continue
		}
		e := eleccionRol[rol]
		grupo = append(grupo, tarjeta(t.T(e.titulo, nil), t.T(e.descripcion, nil), e.icono,
			!g.otra && g.borrador.MoverA == m.ID, func() { g.otra, g.borrador.MoverA = false, m.ID }))
	}
	if len(otras) > 0 {
		grupo = append(grupo, tarjeta(t.T("rules.otherFolderTitle", nil), t.T("rules.otherFolderDescription", nil), iconos.Menu,
			g.otra, func() { g.otra, g.borrador.MoverA = true, "" }))
	}
	objs := []fyne.CanvasObject{container.New(ui.Columna{Hueco: tema.Espacio(2)}, grupo...)}

	if g.otra {
		opciones := make([]string, 0, 1+len(otras))
		opciones = append(opciones, t.T("rules.chooseFolder", nil))
		elegida := 0
		for i, m := range otras {
			opciones = append(opciones, RotuloCarpeta(t, m.Name, m.Role))
			if m.ID == g.borrador.MoverA {
				elegida = i + 1
			}
		}
		g.selector = ui.NuevoSelector(opciones, func(i int) {
			g.borrador.MoverA = ""
			if i > 0 {
				g.borrador.MoverA = otras[i-1].ID
			}
			g.pintar()
		})
		g.selector.Elegida, g.selector.Ancho = elegida, g.ancho
		pista, tono := t.T("rules.targetPending", nil), ui.AyudaPeligro
		if g.borrador.MoverA != "" {
			pista, tono = t.T("rules.targetChosen", i18n.Vars{"folder": g.carpeta(g.borrador.MoverA)}), ui.AyudaNeutra
		}
		objs = append(objs, g.campoRegla(t.T("rules.targetFolder", nil), g.selector, pista, tono))
	}

	// «Además»: el `legend` con `mb-1` y las dos casillas a `gap-2`.
	g.leido.Marcada, g.destacado.Marcada = g.borrador.Leido, g.borrador.Destacado
	g.leido.Refresh()
	g.destacado.Refresh()
	objs = append(objs, container.New(ui.Columna{Hueco: tema.Espacio(1)},
		ui.NuevoRotulo(t.T("rules.alsoLegend", nil), tema.TextoSM, ui.SemiNegrita, "text"),
		container.New(ui.Columna{Hueco: tema.Espacio(2)}, g.leido, g.destacado)))

	if m, hay := roles["trash"]; hay && g.borrador.MoverA == m.ID {
		objs = append(objs, ui.NuevaAyudaEnLinea(t.T("rules.trashWarningBody", nil)).
			ConTono(ui.AyudaAviso).ConTitulo(t.T("rules.trashWarningTitle", nil)))
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(4)}, objs...)
}

func (g *gestorReglas) pasoResumen(v tema.Variante) fyne.CanvasObject {
	t, l := g.b.P.T, g.b.P.Locale
	d := g.borrador
	filas := []filaResumen{
		{t.T("rules.summaryWhich", nil), CondicionRegla(t, l, strings.TrimSpace(d.De), strings.TrimSpace(d.Asunto)), func() { g.irA(0) }},
		{t.T("rules.summaryWhat", nil), AccionRegla(t, l, g.carpeta, d.MoverA, d.Leido, d.Destacado), func() { g.irA(1) }},
		{t.T("rules.summaryWhen", nil), t.T("rules.summaryWhenValue", nil), nil},
	}
	return container.New(ui.Columna{Hueco: tema.Espacio(4)},
		ui.NuevaAyudaEnLinea(FraseRegla(t, l, g.carpeta, d)).ConTono(ui.AyudaInfo),
		resumenRevision(t.T("ui.review.edit", nil), filas, g.ancho, g.ventana, v),
		g.campoRegla(t.T("rules.nameLabel", nil), g.nombre, t.T("rules.nameHint", nil), ui.AyudaNeutra))
}

// crear es `handleAdd`: la regla nueva, activa; al volver, la lista con ella.
func (g *gestorReglas) crear() {
	d := g.borrador
	if !d.conCondicion() || !d.conAccion() || g.guardando {
		return
	}
	f := api.MailFilter{
		Name: strings.TrimSpace(d.Nombre), Enabled: true,
		MatchFrom: strings.TrimSpace(d.De), MatchSubject: strings.TrimSpace(d.Asunto),
		MoveTo: d.MoverA, MarkRead: d.Leido, MarkFlagged: d.Destacado,
	}
	g.guardando = true
	g.pintar()
	pedir(g.b, func(ctx context.Context) (api.MailFilter, error) {
		return g.b.S.CreateFilter(ctx, f)
	}, func(_ api.MailFilter, err error) {
		g.guardando = false
		if err != nil {
			g.pintar()
			g.b.avisar(ui.AvisoPeligro, "rules.createFailed", nil)
			return
		}
		t := g.b.P.T
		g.b.P.Avisar(ui.AvisoExito, t.T("rules.created", nil), t.T("rules.createdDescription", nil))
		g.borrador, g.otra = BorradorRegla{}, false
		g.volver()
		g.cargar()
	})
}

// ---- Piezas ----

// seccionCon es `SettingsSection plain` con `actions`: el título y la descripción a la
// izquierda y la acción a la derecha. La cabecera es `flex-wrap`, pero el lado de los
// textos es `flex-1` (base 0), así que no parte: en el móvil la descripción se estrecha.
func seccionCon(titulo, descripcion string, ancho float32, accion fyne.CanvasObject, dentro ...fyne.CanvasObject) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relajado := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	izq := ancho - tema.Espacio(3) - accion.MinSize().Width
	cabeza := container.New(ui.Columna{Hueco: tema.Espacio(1)},
		ui.NuevoRotulo(titulo, tema.TextoLG, ui.SemiNegrita, "text"),
		ui.NuevoParrafo(descripcion, relajado, ui.Normal, "text-muted", v).A(izq))
	return container.New(ui.Columna{Hueco: huecoDentroSeccion},
		append([]fyne.CanvasObject{container.New(extremos{}, cabeza, accion)}, dentro...)...)
}

// textoRecortado es una línea con `truncate`: se corta por letras, con puntos
// suspensivos, al ancho que le den (no por palabras, que es lo que hace `line-clamp`).
func textoRecortado(texto string, e tema.Escalon, p ui.Peso, token string, v tema.Variante) fyne.CanvasObject {
	return container.New(recorte{texto: texto, e: e, p: p}, ui.Texto(texto, e, p, tema.Color(token, v)))
}

type recorte struct {
	texto string
	e     tema.Escalon
	p     ui.Peso
}

func (r recorte) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, r.e.Linea) }

func (r recorte) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	c, ok := objs[0].(*canvas.Text)
	if !ok {
		return
	}
	ui.Truncar(c, r.texto, r.e, r.p, t.Width)
	ui.ColocarTexto(c, r.e, 0, 0)
}

// anchoLibre no declara ancho: el hijo se coloca al ancho que haya aunque pida más.
// Es la botonera del asistente, que en la web se sale por la derecha a 360 en vez de
// ensanchar el diálogo (visto en la web: «Crear regla» asoma 26 px por fuera).
type anchoLibre struct{}

func (anchoLibre) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, objs[0].MinSize().Height)
}

func (anchoLibre) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(max(t.Width, objs[0].MinSize().Width), t.Height))
}

// filaResumen es un `ReviewItem`: rótulo, valor y, si lo lleva, a qué paso vuelve su
// «Editar».
type filaResumen struct {
	rotulo, valor string
	editar        func()
}

// resumenRevision es `ReviewSummary`: `dl` con borde y filetes, cada fila —rótulo y
// valor— en `px-3 py-2.5`; desde `sm:`, en rejilla con el rótulo a 11rem y la columna
// de «Editar» detrás (`auto`); en el móvil, en fila con «Editar» a la derecha. Todo a
// la línea base (`items-baseline`). Vive en Mail porque es la única pantalla que lo pide.
func resumenRevision(editar string, filas []filaResumen, ancho, ventana float32, v tema.Variante) fyne.CanvasObject {
	interior := ancho - 2*tema.Borde - 2*margenXFilaEtiqueta
	objs := make([]fyne.CanvasObject, 0, len(filas))
	for _, f := range filas {
		rot := ui.NuevoRotulo(f.rotulo, tema.TextoSM, ui.Normal, "text-muted")
		col1, huecos := rot.MinSize().Width, float32(1)
		if ventana >= ui.CorteSM {
			col1, huecos = colResumen, 2
		}
		var boton *ui.Enlace
		anchoBoton := float32(0)
		if f.editar != nil {
			boton = ui.NuevoEnlace(editar, tema.TextoSM, ui.SemiNegrita, "primary-text", f.editar)
			boton.Boton = true
			boton.Nombre = editar + " " + f.rotulo // el `sr-only` que sigue al rótulo
			anchoBoton = boton.MinSize().Width
			if ventana < ui.CorteSM {
				huecos = 2
			}
		}
		wv := max(interior-col1-huecos*tema.Espacio(3)-anchoBoton, 0)
		valor := ui.NuevoParrafo(f.valor, tema.TextoSM, ui.Normal, "text", v).A(wv)
		celdas := []fyne.CanvasObject{rot, valor}
		if boton != nil {
			celdas = append(celdas, boton)
		}
		objs = append(objs, container.New(ui.Margen{Izq: margenXFilaEtiqueta, Der: margenXFilaEtiqueta, Arr: margenYResumen, Abj: margenYResumen},
			container.New(filaDeResumen{col1: col1}, celdas...)))
	}
	caja := ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD)
	dentro := container.New(ui.Columna{}, intercalar(objs, v)...)
	return container.New(&enCaja{caja: caja}, append(caja.Objetos(), dentro)...)
}

// intercalar pone el filete `divide-border-subtle` entre las filas.
func intercalar(filas []fyne.CanvasObject, v tema.Variante) []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, 2*len(filas))
	for i, f := range filas {
		if i > 0 {
			r := canvas.NewRectangle(tema.Color("border-subtle", v))
			r.SetMinSize(fyne.NewSize(0, tema.Borde))
			out = append(out, r)
		}
		out = append(out, f)
	}
	return out
}

// filaDeResumen coloca el rótulo en su columna, el valor detrás a `gap-3` y, si lo
// hay, «Editar» al final.
//
// Con «Editar» la fila CRECE (medido en la web: 33,25 → 36,75 sin contar el filete): el
// botón vive en un `<dd>` sin clase de texto, cuyo puntal es el de `text-base` (19,25
// de línea), y al alinear a la línea base (`items-baseline`) ese puntal baja 3 px el
// texto de toda la fila. El botón queda a la altura del texto.
type filaDeResumen struct{ col1 float32 }

// bajoEditar es lo que baja el texto en una fila con «Editar» (medido: 11,75 − 8,75).
const bajoEditar float32 = 3

func (f filaDeResumen) conEditar(objs []fyne.CanvasObject) bool { return len(objs) > 2 }

func (f filaDeResumen) MinSize(objs []fyne.CanvasObject) fyne.Size {
	alto := max(objs[1].MinSize().Height, tema.TextoSM.Linea)
	if f.conEditar(objs) {
		alto = max(bajoEditar+alto, tema.TextoBase.Linea)
	}
	return fyne.NewSize(0, alto)
}

func (f filaDeResumen) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y, der := float32(0), float32(0)
	if f.conEditar(objs) {
		y = bajoEditar
		b := objs[2].MinSize()
		der = b.Width + tema.Espacio(3)
		objs[2].Move(fyne.NewPos(t.Width-b.Width, y))
		objs[2].Resize(fyne.NewSize(b.Width, tema.TextoSM.Linea))
	}
	objs[0].Move(fyne.NewPos(0, y))
	objs[0].Resize(fyne.NewSize(f.col1, tema.TextoSM.Linea))
	x := f.col1 + tema.Espacio(3)
	objs[1].Move(fyne.NewPos(x, y))
	objs[1].Resize(fyne.NewSize(max(t.Width-x-der, 0), objs[1].MinSize().Height))
}
