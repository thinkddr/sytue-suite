// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Lienzo es lo que el cromo del editor aloja cuando el documento es una hoja de
// cálculo: la barra de fórmulas, la rejilla y, debajo, la barra de hojas.
//
// # Los seis estados, y de quién son
//
// Cuatro de los seis —carga, error, sin conexión y vacío— NO son del lienzo, ni en la
// web: los pinta `EditorApp` antes de decidir qué lienzo montar (el esqueleto del
// cromo con `role="status"` mientras carga, y un `EmptyState` con reintentar/volver si
// falla), así que el lienzo solo existe cuando ya hay documento. Duplicarlos aquí
// sería pintar dos veces el mismo estado y que se contradigan.
//
// Los dos que SÍ son de aquí:
//
//   - **Éxito**: la rejilla con sus datos, que es el caso normal.
//   - **Permiso denegado**: `SoloLectura`, que es la hoja protegida de la web
//     (`sheet.protect` + `canEditCellContent`). Se ve, se recorre, se copia y se
//     anuncia como de solo lectura; no se escribe. No es un cartel, es el estado de
//     los controles, igual que el `readOnly` del `<input>` de la barra.
//
// # Todo corre en la rutina de la interfaz, y es deliberado
//
// El motor de fórmulas no es seguro para uso concurrente y `fyne.Do` **no marshalla
// nada en el driver de pruebas**, así que un recálculo en segundo plano no estaría
// serializado por nadie. Aquí no hay gorrutinas: se pregunta al motor desde el
// `Layout`, que ya corre donde debe. Cuando haga falta recalcular en frío —un libro
// grande al abrirlo—, se hace con el patrón de `ui/aviso.go`: un cerrojo para los
// datos y otro para el `Refresh()` entero, en ese orden.
type Lienzo struct {
	widget.BaseWidget

	Libro    *Libro
	Barra    *Barra
	Rejilla  *Rejilla
	Pestanas *Pestanas
	// busqueda es la de «Buscar», para repintar o quitar su amarillo al abrir y
	// cerrar el panel; la pone panelesDe.
	busqueda *busqueda

	// T traduce, con la firma de `i18n.Translator.T`. Sin él se enseñan las claves, que
	// es lo que quiere un guardián.
	T func(clave string, vars i18n.Vars) string
	// AlCambiar avisa de que el libro ha cambiado —una celda, un formato, otra hoja
	// activa, una hoja más o menos—: el cromo repinta la barra de estado («Hoja 2 de
	// 3») y la cinta, y quien guarda sabe que hay algo que guardar.
	AlCambiar func()
	// AlSeleccionar avisa de que la celda ancla o el rango se han movido: la cinta
	// enciende sus botones con el formato de la celda nueva.
	AlSeleccionar func()
	// AlAviso enseña un aviso —«Se han quitado 3 duplicados»—, que en la web es un
	// `notify` de la pila de avisos, con su tono. Lo pone quien monta el lienzo.
	AlAviso func(tono ui.TonoAviso, titulo, detalle string)
	// avisoProtegida es cuándo se dijo por última vez «hoja protegida» (el antirrebote).
	avisoProtegida time.Time
	// IA es la consulta a Sytue AI del relleno con IA; nil, sin servicio, y el botón no
	// sale (la web sin `aiEnabled`).
	IA ConsultaIA
	// lanzar corre una consulta fuera del hilo de la interfaz; las pruebas la cambian por
	// una llamada directa.
	lanzar func(func())

	// fx es la fórmula que se está escribiendo (en la celda o en la barra), o nil.
	fx *edicionFx
	// fxAplicando tapa el eco: escribir en un editor desde aquí dispara su OnChanged, y
	// eso no es teclear.
	fxAplicando bool
	// mantenerFx dice que el editor va a perder el foco por un clic que NO acaba la
	// fórmula (señalar una celda, elegir de la lista).
	mantenerFx bool
	sug        *sugerencias
	// valAbierto es la lista de la validación de la celda ancla, abierta.
	valAbierto bool
	// anclaFiltro es de donde cuelga el desplegable del autofiltro (transparente).
	anclaFiltro *canvas.Rectangle
	// Las tarjetas de los gráficos y de las segmentaciones, por id (`flotantes.go`).
	tarjetas  map[string]*TarjetaGrafico
	segmentos map[string]*TarjetaSegmentacion
}

var _ fyne.Widget = (*Lienzo)(nil)

// NuevoLienzo monta la barra y la rejilla sobre un libro, y las ata: al moverse la
// selección o cambiar una celda, la barra se sincroniza.
func NuevoLienzo(l *Libro) *Lienzo {
	c := &Lienzo{
		Libro: l, anclaFiltro: canvas.NewRectangle(color.Transparent), lanzar: func(f func()) { go f() },
		tarjetas: map[string]*TarjetaGrafico{}, segmentos: map[string]*TarjetaSegmentacion{},
	}
	c.ExtendBaseWidget(c)
	l.rotulo = func(clave string) string { return c.t(clave, nil) }
	c.Rejilla = NuevaRejilla(l)
	c.Rejilla.Flotantes = c.flotantes
	c.Barra = NuevaBarra(c.Rejilla)
	c.Pestanas = nuevasPestanas(c)
	c.Rejilla.AlMoverse = func() {
		c.cerrarValores() // cambiar de celda cierra el desplegable, como en la web
		c.sincronizar()
		if c.AlSeleccionar != nil {
			c.AlSeleccionar()
		}
	}
	c.Rejilla.AlEditar = func() { c.sincronizar(); c.avisar() }
	c.Rejilla.AlDeshacer = c.Deshacer
	c.sug = nuevasSugerencias(c)
	c.engancharFx()
	return c
}

// engancharFx conecta la edición de fórmulas con los dos editores: lo que se teclea y
// dónde está el cursor, las teclas que se queda la lista, el señalar en la rejilla y el
// fin de la edición.
func (c *Lienzo) engancharFx() {
	for _, barra := range []bool{false, true} {
		ed := c.editorFx(barra)
		ed.AlCambiar(func(string) { c.sincronizarFx(barra) })
		ed.OnCursorChanged = func() { c.sincronizarFx(barra) }
		previa := ed.Tecla
		ed.Tecla = func(k *fyne.KeyEvent) bool { return c.teclaFx(barra, k) || previa(k) }
	}
	c.Rejilla.Señalar = func(zona int, celda Celda, gesto int) bool {
		if c.señalar(zona, celda, gesto) {
			c.mantenerFx = true
			return true
		}
		return false
	}
	c.Rejilla.AlSeñalar = func() {
		c.mantenerFx = false
		c.devolverFoco()
	}
	c.Rejilla.AlCerrarEdicion = c.terminarFx
	c.Rejilla.AlBotonValores = func() {
		c.valAbierto = !c.valAbierto
		c.Refresh()
	}
	c.Rejilla.AlPulsarFuera = c.cerrarValores
	c.Rejilla.AlBotonFiltro = c.abrirFiltro
	c.Rejilla.AlEsquema = func(valor string) { c.Mandar(CmdEsquema, valor) }
	c.Rejilla.AlProtegida = c.avisarProtegida
	c.Rejilla.AlRechazar = func(msg string) {
		if c.AlAviso != nil {
			c.AlAviso(ui.AvisoAdvertencia, c.t("sheet.msg.badValue", nil), msg)
		}
	}
	c.Barra.AlEnfocar = func(si bool) {
		if si {
			c.sincronizarFx(true)
			return
		}
		if !c.mantenerFx && c.fx != nil && c.fx.barra {
			c.terminarFx()
		}
	}
}

// Deshacer vuelve un paso atrás, o adelante con `rehacer`. La selección se queda donde
// estaba, como en la web; lo que cambia es el libro, que puede tener otra hoja activa o
// una hoja menos.
func (c *Lienzo) Deshacer(rehacer bool) {
	var hecho bool
	if rehacer {
		hecho = c.Libro.Rehacer()
	} else {
		hecho = c.Libro.Deshacer()
	}
	if !hecho {
		return
	}
	c.Rejilla.Cancelar()
	c.sincronizar()
	c.Rejilla.Refresh()
	c.avisar()
}

// sincronizar trae a la barra de fórmulas y a la de hojas lo que ha cambiado en la
// rejilla: la celda ancla, el rango (el resumen) o el contenido.
func (c *Lienzo) sincronizar() {
	c.Barra.Sincronizar()
	c.Pestanas.Refresh()
}

// t traduce con el hueco tapado: sin traductor, la clave.
func (c *Lienzo) t(clave string, vars i18n.Vars) string {
	if c.T == nil {
		return clave
	}
	return c.T(clave, vars)
}

// SoloLectura pone (o quita) el estado de permiso denegado en las tres piezas.
func (c *Lienzo) SoloLectura(si bool) {
	c.Rejilla.SoloLectura = si
	c.sincronizar()
}

// MinSize es la barra de fórmulas, lo mínimo de la rejilla y la barra de hojas.
func (c *Lienzo) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	m := c.Rejilla.MinSize()
	return fyne.NewSize(max(m.Width, MedidaBarra().Width), AltoBarra+m.Height+AltoPestanas)
}

// CreateRenderer apila barra, rejilla y pestañas, y encima la lista o la pista de la
// fórmula en edición con su sombra (`z-[var(--z-popover)]`).
func (c *Lienzo) CreateRenderer() fyne.WidgetRenderer {
	sombra := ui.NuevaSombra(tema.SombraMD, tema.RadioLG)
	return &lienzoRenderer{
		c: c, sombra: sombra,
		objetos: []fyne.CanvasObject{c.Barra, c.Rejilla, c.Pestanas, sombra.Objeto(), c.sug, c.anclaFiltro},
	}
}

type lienzoRenderer struct {
	c       *Lienzo
	sombra  *ui.Sombra
	objetos []fyne.CanvasObject
}

func (r *lienzoRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *lienzoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *lienzoRenderer) Destroy()                     {}

func (r *lienzoRenderer) Layout(t fyne.Size) {
	r.c.Barra.Move(fyne.NewPos(0, 0))
	r.c.Barra.Resize(fyne.NewSize(t.Width, AltoBarra))
	altoRejilla := max(t.Height-AltoBarra-AltoPestanas, 0)
	r.c.Rejilla.Move(fyne.NewPos(0, AltoBarra))
	r.c.Rejilla.Resize(fyne.NewSize(t.Width, altoRejilla))
	r.c.Pestanas.Move(fyne.NewPos(0, AltoBarra+altoRejilla))
	r.c.Pestanas.Resize(fyne.NewSize(t.Width, AltoPestanas))
	r.colocarSugerencias()
}

// colocarSugerencias pone la lista o la pista donde la web: bajo la celda en edición
// (`left: colX[sel.c]`, `top` de la fila siguiente) o bajo el campo de la barra
// (`top-full left-0 mt-1`).
func (r *lienzoRenderer) colocarSugerencias() {
	c := r.c
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if (c.fx == nil && c.valoresAbiertos() == nil) || !c.sug.poner() {
		c.sug.Hide()
		r.sombra.Objeto().Hide()
		return
	}
	var pos fyne.Position
	tam := c.sug.MinSize()
	switch {
	case c.sug.valores:
		// `absolute top-full right-0 mt-0.5` bajo el «▾».
		p, t, _ := c.Rejilla.botonValores()
		pos = fyne.NewPos(p.X+t.Width-tam.Width, AltoBarra+p.Y+t.Height+SeparacionValores)
	case c.fx.barra:
		x, abajo := c.Barra.campoX()
		pos = fyne.NewPos(x, abajo+SeparacionEmergenteFx)
	default:
		rj := c.Rejilla
		vw := rj.vista()
		pos = fyne.NewPos(vw.x(rj.sel.C), AltoBarra+vw.y(rj.sel.F)+vw.alto())
	}
	c.sug.Move(pos)
	c.sug.Resize(tam)
	c.sug.Show()
	r.sombra.Objeto().Show() // la escondió la vuelta de arriba sin sugerencias
	r.sombra.Ajustar(pos, tam, v)
}

func (r *lienzoRenderer) Refresh() {
	r.colocarSugerencias()
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// valoresAbiertos es la validación cuya lista está abierta, o nil.
func (c *Lienzo) valoresAbiertos() *Validacion {
	if !c.valAbierto {
		return nil
	}
	return c.Rejilla.ListaValidacion()
}

// cerrarValores cierra la lista de la validación; devuelve si estaba abierta.
func (c *Lienzo) cerrarValores() bool {
	if !c.valAbierto {
		return false
	}
	c.valAbierto = false
	c.Refresh()
	return true
}

// elegirValor es `pickValue`: escribe el valor en la celda ancla con su formato, cierra
// la lista y devuelve el foco a la rejilla.
func (c *Lienzo) elegirValor(v string) {
	c.valAbierto = false
	c.Rejilla.Escribir(v)
	c.Rejilla.recuperarFoco()
	c.Refresh()
}
