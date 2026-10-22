// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

/*
Los seis DIÁLOGOS de la cinta (`Dialogos` de `Paneles.tsx`), sobre el `Modal` de la
suite (`ui.Dialogo`): el título `pdf.dialog.<nombre>.title`, `lg` para Herramientas y
Comparar y `md` para el resto. Los botones van DENTRO del cuerpo, como en la web
(`BotonesDialogo`), no en el pie del Modal.
*/

// nombreDialogo es la parte de la clave de cada uno.
var nombreDialogo = map[Dialogo]string{
	DialogoOrganizar: "organizar", DialogoHerramientas: "herramientas", DialogoCapas: "capas",
	DialogoComparar: "comparar", DialogoProteger: "proteger", DialogoFirmar: "firmar",
	DialogoPedirFirma: "pedirFirma",
}

// dialogoAbierto es el diálogo que se ve: su Modal y cómo se pinta su cuerpo, que se
// vuelve a pintar entero cuando cambia su estado (una opción, «aplicando»).
type dialogoAbierto struct {
	p      *Pantalla
	dlg    *ui.Dialogo
	cuerpo *fyne.Container
	talla  ui.TallaDialogo
	pintar func(ancho float32) fyne.CanvasObject
	// enviar es el botón de aplicar del cuerpo pintado, si lo lleva: lo pone
	// botonesDialogo y lo usa Intro (ver conIntro).
	enviar func()
	// pie son los botones de la herramienta, si los lleva: van en el pie del diálogo; y
	// botonAplicar, el principal.
	pie          fyne.CanvasObject
	botonAplicar *ui.Boton
}

// abrirDialogo es lo que hace un botón de texto de la cinta.
func (p *Pantalla) abrirDialogo(d Dialogo) {
	if p.Contenido == nil || p.doc == nil {
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz)
	if c == nil {
		return
	}
	a := &dialogoAbierto{p: p, cuerpo: container.New(ui.Columna{}), talla: ui.DialogoMD}
	if d == DialogoHerramientas || d == DialogoComparar {
		a.talla = ui.DialogoLG
	}
	switch d {
	case DialogoOrganizar:
		o := &dialogoOrganizar{a: a, op: "extraer", grados: 90}
		a.pintar = o.vista
	case DialogoCapas:
		k := &dialogoCapas{a: a, op: "marca-agua"}
		a.pintar = k.vista
	case DialogoProteger:
		a.pintar = nuevoDialogoProteger(a).vista
	case DialogoComparar:
		a.pintar = (&dialogoComparar{a: a}).vista
	case DialogoHerramientas:
		a.pintar = nuevoDialogoHerramientas(a).vista
	case DialogoFirmar:
		a.pintar = nuevoDialogoFirmar(a).vista
	case DialogoPedirFirma:
		a.pintar = nuevoDialogoPedirFirma(a).vista
	case SinDialogo:
		return
	}
	titulo := p.t("pdf.dialog." + nombreDialogo[d] + ".title")
	if d == DialogoFirmar {
		titulo = p.t("pdf.menu.sign") // «Firmar», como el menú: la herramienta tiene cuatro modos
	}
	a.dlg = ui.NuevoDialogo(titulo, a.cuerpo).Con(a.talla, nil)
	p.dialogo = a
	a.rehacer()
	a.dlg.Mostrar(c)
}

// ancho es el del cuerpo: el del Modal menos sus filetes y su `px-6`.
func (a *dialogoAbierto) ancho() float32 {
	lienzo := fyne.Size{}
	if c := fyne.CurrentApp().Driver().CanvasForObject(a.p.raiz); c != nil {
		lienzo = c.Size()
	}
	return ui.AnchoCuerpoDialogo(&ui.Dialogo{Talla: a.talla}, lienzo)
}

// estrecha es el `max-sm:` de la web: la ventana por debajo de 640.
func (a *dialogoAbierto) estrecha() bool {
	c := fyne.CurrentApp().Driver().CanvasForObject(a.p.raiz)
	return c != nil && c.Size().Width > 0 && c.Size().Width < ui.CorteSM
}

func (a *dialogoAbierto) rehacer() {
	a.enviar, a.pie, a.botonAplicar = nil, nil, nil
	cuerpo := a.pintar(a.ancho())
	sobreElDialogo(cuerpo)
	if a.enviar != nil {
		conIntro(cuerpo, a.enviar)
	}
	a.cuerpo.Objects = []fyne.CanvasObject{cuerpo}
	a.cuerpo.Refresh()
	if a.dlg != nil {
		a.dlg.CambiarPie(a.pie)
	}
}

// sobreElDialogo dice a cada botón del cuerpo que va sobre el panel del Modal
// (`bg-surface-raised`), que en oscuro no es la superficie de los paneles: un botón con
// borde y sin fondo se pintaba con la de los paneles y salía más oscuro que en la web.
func sobreElDialogo(o fyne.CanvasObject) {
	switch x := o.(type) {
	case *Accion:
		x.Sobre = "surface-raised"
	case *fyne.Container:
		for _, h := range x.Objects {
			sobreElDialogo(h)
		}
	}
}

// conIntro es el `onSubmit` del `<form>` de la web: Intro en un campo de una línea envía
// el formulario, que es pulsar su botón de aplicar. Un campo que ya atiende Intro por su
// cuenta se queda como está; un área de texto no es una `ui.Entrada` y sigue partiendo
// líneas.
func conIntro(o fyne.CanvasObject, enviar func()) {
	switch x := o.(type) {
	case *ui.Entrada:
		if x.AlTeclear == nil {
			x.AlTeclear = func(k *fyne.KeyEvent) bool {
				if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
					return false
				}
				enviar()
				return true
			}
		}
	case *fyne.Container:
		for _, h := range x.Objects {
			conIntro(h, enviar)
		}
	}
}

func (a *dialogoAbierto) cerrar() {
	if a.dlg != nil {
		a.dlg.Cerrar()
	}
	if a.p.dialogo == a {
		a.p.dialogo = nil
	}
}

// aplicar es el `onAplicar(…).then(onCerrar)` de la web: aplica y, haya ido bien o
// mal, cierra (el fallo lo dice la franja roja).
func (a *dialogoAbierto) aplicar(c api.PDFCambio) {
	a.p.Aplicar(c, "", func(bool) { a.cerrar() })
	a.rehacer()
}

func (a *dialogoAbierto) t(clave string) string { return a.p.t(clave) }

// ---- piezas de los diálogos ----

// conRotulo es `<label class="text-text-muted flex flex-col gap-1 text-xs">`: el
// rótulo encima y el control debajo.
func conRotulo(rotulo string, controles ...fyne.CanvasObject) *fyne.Container {
	return container.New(ui.Columna{Hueco: tema.Espacio(1)},
		append([]fyne.CanvasObject{ui.NuevoRotulo(rotulo, tema.TextoXS, ui.Normal, "text-muted")}, controles...)...)
}

// campoDialogo es `border-border bg-surface text-text rounded-md border px-2 py-1.5
// text-sm`: 32 px de alto (18 de línea, 12 de relleno y 2 de borde).
func campoDialogo() ui.CampoPropio {
	return ui.CampoPropio{MargenX: tema.Espacio(2), MargenY: tema.Espacio(1.5), Texto: tema.TextoSM, Borde: "border", MarcaDelTexto: true}
}

// entradaDialogo es un `<input>` de los diálogos, a todo el ancho.
func entradaDialogo(marca string, ancho float32) *ui.Entrada {
	e := ui.NuevaEntrada(marca).ConPropia(campoDialogo())
	e.Radio, e.Ancho = tema.RadioMD, ancho
	return e
}

// altoSelectNativoSM es el `<select>` nativo con `py-1.5 text-sm`, medido en
// Chromium con Figtree cargada y raíz de 16 px: 31 px (line-height normal del UA).
const altoSelectNativoSM float32 = 31

// selectorDialogo es un `<select>` de los diálogos, con la flecha del navegador.
func selectorDialogo(opciones []string, elegida int, ancho float32, alElegir func(int)) *ui.Selector {
	s := ui.NuevoSelector(opciones, alElegir)
	s.Elegida = elegida
	c := campoDialogo()
	c.Radio = tema.RadioMD
	s.Propio = &ui.SelectorPropio{Campo: c, Alto: altoSelectNativoSM}
	s.Ancho = ancho
	return s
}

// notaDialogo es `<p class="text-text-subtle text-2xs">`.
func notaDialogo(texto string, ancho float32) *ui.Parrafo {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return ui.NuevoParrafo(texto, tema.Texto2XS, ui.Normal, "text-subtle", v).A(ancho)
}

// botonesDialogo es `BotonesDialogo`: «Cancelar» fantasma y «Aplicar» primario,
// apagado mientras se trabaja.
//
// Van en el PIE del diálogo, con los botones de la suite (a todo el ancho en el
// teléfono). Eran dos botones propios de 30 al final del cuerpo, fuera de la vista en el
// teléfono.
func (a *dialogoAbierto) botonesDialogo(aplicar func()) {
	cancelar := ui.NuevoBoton(a.t("pdf.cancel"), a.cerrar).Con(ui.Fantasma, ui.TallaMD)
	si := ui.NuevoBoton(a.t("pdf.apply"), aplicar).Con(ui.Primario, ui.TallaMD)
	si.Inerte = a.p.Trabajando
	// Los campos que se conservan entre repintados guardan esta función: mira el estado
	// al pulsar, no el del botón de entonces.
	a.enviar = func() {
		if !a.p.Trabajando {
			aplicar()
		}
	}
	a.pie, a.botonAplicar = container.New(ui.Fila{Hueco: ui.HuecoPieDialogo}, cancelar, si), si
}

// opcionesDeRadio es la rejilla de `<label>` con un radio escondido dentro: una opción
// por casilla, la elegida con `border-border-strong bg-selected text-text`.
func opcionesDeRadio(rotulos []string, elegida, columnas int, ancho float32, alElegir func(int)) fyne.CanvasObject {
	hueco := tema.Espacio(2)
	cada := (ancho - hueco*float32(columnas-1)) / float32(columnas)
	casillas := make([]fyne.CanvasObject, len(rotulos))
	for i, r := range rotulos {
		k := i
		o := NuevaAccion(r, func() { alElegir(k) }).ConBorde()
		o.Peso, o.Ancho, o.FondoEncima, o.Tinta = ui.Normal, cada, "", "text-muted"
		if i == elegida {
			o.Borde, o.Fondo, o.Tinta = "border-strong", "selected", "text"
		}
		casillas[i] = o
	}
	return container.New(ui.Rejilla{Fijas: columnas, Hueco: hueco}, casillas...)
}

// columnasDeRejilla es `grid-cols-2 sm:grid-cols-4`: el `sm` se mide sobre la ventana.
func (a *dialogoAbierto) columnasDeRejilla() int {
	if c := fyne.CurrentApp().Driver().CanvasForObject(a.p.raiz); c != nil && c.Size().Width < corteSM {
		return 2
	}
	return 4
}

// corteSM es el `sm:` de Tailwind: 40rem de 16, medido sobre la ventana.
const corteSM = 640

// ---- Organizar ----

// dialogoOrganizar es `Organizar`: extraer, borrar, girar o combinar, siempre en copia.
type dialogoOrganizar struct {
	a       *dialogoAbierto
	op      string
	grados  int
	paginas *ui.Entrada
	otros   *ui.AreaTexto
}

var opsOrganizar = []string{"extraer", "borrar", "rotar", "combinar"}

func (o *dialogoOrganizar) vista(ancho float32) fyne.CanvasObject {
	a := o.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := a.p.Contenido
	rotulos := make([]string, len(opsOrganizar))
	elegida := 0
	for i, op := range opsOrganizar {
		rotulos[i] = a.t("pdf.op." + op)
		if op == o.op {
			elegida = i
		}
	}
	partes := []fyne.CanvasObject{
		ui.NuevoParrafo(a.p.tv("pdf.dialog.organizar.help", i18n.Vars{"total": c.Total}), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho),
		opcionesDeRadio(rotulos, elegida, a.columnasDeRejilla(), ancho, func(i int) { o.op = opsOrganizar[i]; a.rehacer() }),
	}
	if o.op == "combinar" {
		if o.otros == nil {
			o.otros = ui.NuevaAreaTexto("/Documentos/anexo-1.pdf\n/Fotos/recibo.jpg")
			o.otros.Filas = 3
			o.otros.ConPropia(campoDialogo())
			o.otros.Radio, o.otros.Ancho = tema.RadioMD, ancho
		}
		pista := notaDialogo(a.p.tv("pdf.dialog.organizar.othersHint", i18n.Vars{"n": len(o.rutasEscritas())}), ancho)
		o.otros.AlCambiar(func(string) {
			pista.Texto = a.p.tv("pdf.dialog.organizar.othersHint", i18n.Vars{"n": len(o.rutasEscritas())})
			pista.Refresh()
		})
		partes = append(partes, conRotulo(a.t("pdf.dialog.organizar.others"), o.otros, pista))
	} else {
		if o.paginas == nil {
			o.paginas = entradaDialogo("1,3,5-8", ancho)
		}
		controles := []fyne.CanvasObject{o.paginas}
		if tramos := tramosDelIndice(c.Indice, c.Total); len(tramos) > 1 {
			controles = append(controles, container.New(ui.Margen{Arr: tema.Espacio(1)}, container.New(ui.Columna{Hueco: tema.Espacio(1)},
				ui.NuevoRotulo(a.t("pdf.split.byOutline"), tema.Texto2XS, ui.Normal, "text-subtle"),
				o.botonesDeTramos(tramos, ancho))))
		}
		partes = append(partes, conRotulo(a.t("pdf.dialog.organizar.pages"), controles...))
	}
	if o.op == "rotar" {
		grados := []int{90, 180, 270}
		opciones := []string{"90°", "180°", "270°"}
		elegido := 0
		for i, g := range grados {
			if g == o.grados {
				elegido = i
			}
		}
		partes = append(partes, conRotulo(a.t("pdf.dialog.organizar.degrees"),
			selectorDialogo(opciones, elegido, ancho, func(i int) { o.grados = grados[i] })))
	}
	partes = append(partes, notaDialogo(a.t("pdf.dialog.copyNote"), ancho))
	a.botonesDialogo(o.aplicar)
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

// rutasEscritas son las líneas no vacías de «los demás»: el orden es el escrito.
func (o *dialogoOrganizar) rutasEscritas() []string {
	if o.otros == nil {
		return nil
	}
	var r []string
	for _, l := range strings.Split(o.otros.Texto(), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			r = append(r, l)
		}
	}
	return r
}

// botonesDeTramos son los capítulos del índice, cada uno con su rango: un clic lo pone
// en el campo. `text-2xs rounded border px-1.5 py-1 max-w-[14rem] truncate`.
func (o *dialogoOrganizar) botonesDeTramos(tramos []tramo, ancho float32) fyne.CanvasObject {
	botones := make([]fyne.CanvasObject, len(tramos))
	for i, tr := range tramos {
		rango := tr.rango
		b := nuevoBotonTramo(tr.titulo, rango, o.paginas.Texto() == rango, func() {
			o.paginas.PonerTexto(rango)
			o.a.rehacer()
		})
		botones[i] = b
	}
	return container.New(&filaQueEnvuelve{hueco: tema.Espacio(1), ancho: ancho}, botones...)
}

func (o *dialogoOrganizar) aplicar() {
	// El `required` de las páginas: vacías, se marca el campo en vez de no hacer nada.
	if o.op != "combinar" && o.paginas != nil && !o.paginas.Requerida() {
		return
	}
	if c, ok := o.cambio(); ok {
		o.a.aplicar(c)
	}
}

// cambio es lo que manda el formulario; sin lo `required`, nada.
func (o *dialogoOrganizar) cambio() (api.PDFCambio, bool) {
	campos := map[string]string{"op": o.op}
	var rutas []string
	if o.op == "combinar" {
		// Combinar no pregunta páginas: van todas, y el orden es el escrito.
		if rutas = o.rutasEscritas(); len(rutas) == 0 {
			return api.PDFCambio{}, false
		}
	} else {
		p := ""
		if o.paginas != nil {
			p = o.paginas.Texto()
		}
		// La web no recorta: `required` solo mira que no esté vacío.
		if p == "" {
			return api.PDFCambio{}, false
		}
		campos["paginas"] = p
	}
	if o.op == "rotar" {
		campos["grados"] = strconv.Itoa(o.grados)
	}
	return api.PDFCambio{Op: "paginas", Campos: campos, Rutas: rutas, Sufijo: o.a.t("pdf.suffix." + o.op)}, true
}

// tramo es un capítulo del índice con sus páginas («1-4», o «5» si es una).
type tramo struct{ titulo, rango string }

// tramosDelIndice es `tramosDelIndice` de `api.ts`: cada capítulo de primer nivel con
// destino llega hasta el siguiente, y el último hasta el final.
func tramosDelIndice(indice []Marcador, total int) []tramo {
	var caps []Marcador
	for _, m := range indice {
		if m.Nivel == 0 && m.Pagina >= 0 {
			caps = append(caps, m)
		}
	}
	r := make([]tramo, len(caps))
	for i, m := range caps {
		desde, hasta := m.Pagina+1, total
		if i+1 < len(caps) {
			hasta = max(desde, caps[i+1].Pagina)
		}
		rango := strconv.Itoa(desde)
		if hasta != desde {
			rango += "-" + strconv.Itoa(hasta)
		}
		r[i] = tramo{titulo: m.Titulo, rango: rango}
	}
	return r
}

// ---- Capas ----

// dialogoCapas es `Capas`: marca de agua, fondo, numeración, Bates, encabezado o pie.
type dialogoCapas struct {
	a     *dialogoAbierto
	op    string
	texto *ui.Entrada
}

var opsCapas = []struct{ op, clave string }{
	{"marca-agua", "pdf.op.watermark"},
	{"fondo", "pdf.op.background"},
	{"numerar", "pdf.op.number"},
	{"bates", "pdf.op.bates"},
	{"encabezado", "pdf.op.header"},
	{"pie", "pdf.op.footer"},
}

func (k *dialogoCapas) vista(ancho float32) fyne.CanvasObject {
	a := k.a
	opciones := make([]string, len(opsCapas))
	elegida := 0
	for i, o := range opsCapas {
		opciones[i] = a.t(o.clave)
		if o.op == k.op {
			elegida = i
		}
	}
	if k.texto == nil {
		k.texto = entradaDialogo("", ancho)
	}
	rotulo := "pdf.dialog.capas.text"
	if k.op == "numerar" || k.op == "bates" {
		rotulo = "pdf.dialog.capas.prefix"
	}
	partes := []fyne.CanvasObject{
		conRotulo(a.t("pdf.dialog.capas.what"), selectorDialogo(opciones, elegida, ancho, func(i int) {
			k.op = opsCapas[i].op
			a.rehacer()
		})),
		conRotulo(a.t(rotulo), k.texto),
	}
	// La diferencia entre marca de agua y fondo está en el resultado: se dice ANTES.
	switch k.op {
	case "marca-agua":
		partes = append(partes, notaDialogo(a.t("pdf.dialog.capas.watermarkNote"), ancho))
	case "fondo":
		partes = append(partes, notaDialogo(a.t("pdf.dialog.capas.backgroundNote"), ancho))
	}
	partes = append(partes, notaDialogo(a.t("pdf.dialog.copyNote"), ancho))
	a.botonesDialogo(k.aplicar)
	return container.New(ui.Columna{Hueco: tema.Espacio(3)}, partes...)
}

func (k *dialogoCapas) aplicar() {
	if k.op != "numerar" && k.op != "bates" && k.texto != nil && !k.texto.Requerida() {
		return
	}
	if c, ok := k.cambio(); ok {
		k.a.aplicar(c)
	}
}

// cambio es lo que manda: el texto es obligatorio salvo al numerar, donde es prefijo.
func (k *dialogoCapas) cambio() (api.PDFCambio, bool) {
	texto := ""
	if k.texto != nil {
		texto = k.texto.Texto()
	}
	if texto == "" && k.op != "numerar" && k.op != "bates" {
		return api.PDFCambio{}, false
	}
	return api.PDFCambio{Op: "paginas", Campos: map[string]string{"op": k.op, "texto": texto}, Sufijo: k.a.t("pdf.suffix.stamped")}, true
}

// nuevoBotonTramo es un capítulo: `border-border text-text-muted hover:bg-bg-subtle
// text-2xs max-w-[14rem] truncate rounded border px-1.5 py-1`, con el rango detrás en
// `text-text-subtle ml-1`; el elegido, `bg-selected text-text`. Si no cabe, se recorta
// por el final como el `truncate`: primero el rango y luego el título.
func nuevoBotonTramo(titulo, rango string, elegido bool, alPulsar func()) fyne.CanvasObject {
	e, px, sep := tema.Texto2XS, tema.Espacio(1.5), tema.Espacio(1)
	// El rango va con `tabular-nums`.
	wt, wr := ui.Medir(titulo, e, ui.Normal).Width, medirTabular(rango, e, ui.Normal)
	ancho := min(wt+sep+wr+2*px+2*tema.Borde, tema.Espacio(56))
	fondo := NuevaAccion("", alPulsar).ConBorde()
	fondo.Nombre = titulo + " · " + rango
	fondo.Escalon, fondo.PX, fondo.PY, fondo.Radio, fondo.Ancho, fondo.FondoEncima = e, 0, tema.Espacio(1), 0.25*tema.Rem, ancho, "bg-subtle"
	tinta := "text-muted"
	if elegido {
		fondo.Fondo, tinta = "selected", "text"
	}
	cabe := ancho - 2*px - 2*tema.Borde
	var dentro []fyne.CanvasObject
	switch {
	case wt+sep+wr <= cabe:
		dentro = []fyne.CanvasObject{ui.NuevoRotulo(titulo, e, ui.Normal, tinta), nuevoRotuloTabular(rango, e, ui.Normal, "text-subtle")}
	case wt+sep < cabe:
		dentro = []fyne.CanvasObject{ui.NuevoRotulo(titulo, e, ui.Normal, tinta), rotuloRecortado(rango, e, ui.Normal, "text-subtle", cabe-wt-sep)}
	default:
		dentro = []fyne.CanvasObject{rotuloRecortado(titulo, e, ui.Normal, tinta, cabe)}
	}
	texto := container.New(ui.Margen{Izq: tema.Borde + px, Arr: tema.Borde + tema.Espacio(1)}, container.New(ui.Fila{Hueco: sep}, dentro...))
	return container.NewStack(fondo, texto)
}

// filaQueEnvuelve es `flex flex-wrap gap-*`: los hijos a su medida, de izquierda a
// derecha, y a la línea siguiente el que no cabe. huecoY es el `gap-y-*` si difiere.
type filaQueEnvuelve struct{ hueco, huecoY, ancho float32 }

func (f *filaQueEnvuelve) colocar(objs []fyne.CanvasObject, mover bool) fyne.Size {
	var x, y, alto float32
	for _, o := range objs {
		m := o.MinSize()
		if x > 0 && x+m.Width > f.ancho {
			hy := f.huecoY
			if hy == 0 {
				hy = f.hueco
			}
			x, y, alto = 0, y+alto+hy, 0
		}
		if mover {
			o.Move(fyne.NewPos(x, y))
			o.Resize(m)
		}
		x += m.Width + f.hueco
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(f.ancho, y+alto)
}

func (f *filaQueEnvuelve) MinSize(objs []fyne.CanvasObject) fyne.Size { return f.colocar(objs, false) }

func (f *filaQueEnvuelve) Layout(objs []fyne.CanvasObject, _ fyne.Size) { f.colocar(objs, true) }
