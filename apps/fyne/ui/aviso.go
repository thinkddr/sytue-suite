// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Toast.tsx

import (
	"slices"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # Un aviso NO va en las superposiciones, y esto no es una preferencia
//
// `internal/driver/util.go` de Fyne v2.8.1, en `FindObjectAtPositionMatching`:
//
//	if overlay != nil {
//	    WalkVisibleObjectTree(overlay, findFunc, nil)
//	} else {
//	    for _, root := range roots { … }
//	}
//
// Con UNA capa puesta, el puntero no visita el contenido: ni siquiera se le pregunta.
// Así que una capa para los avisos deja la aplicación entera sin poder pulsar mientras
// haya un aviso en pantalla — que es la misma trampa del velo permanente, por otro
// camino y sin velo que la delate.
//
// Por eso la pila de avisos es un HERMANO del contenido, no una capa. Y funciona
// porque el recorrido de las raíces se queda con la ÚLTIMA coincidencia, no con la
// primera: un aviso puesto detrás del contenido gana donde lo tapa, y donde no lo tapa
// no estorba. Es exactamente el `pointer-events-none` del contenedor con
// `pointer-events-auto` en cada aviso.
//
// El diálogo y el menú SÍ son capas, y ahí bloquear es lo correcto: un diálogo modal
// tiene que dejar inerte lo de debajo.
//
// Con un diálogo abierto, los avisos van en una banda desplazable de SU contenido,
// entre la cabecera y el cuerpo: no pueden cubrir campos ni botones. Los menús sin
// diálogo conservan el préstamo a la capa. Al cerrar el último diálogo los avisos
// persistentes vuelven a su esquina.

// TonoAviso son los cuatro tonos de Toast.tsx. El valor cero es `info`, como el
// `toneConfig[t.tone ?? 'info']` de allí.
type TonoAviso uint8

// Los cuatro tonos.
const (
	AvisoInfo TonoAviso = iota
	AvisoExito
	AvisoAdvertencia
	AvisoPeligro
)

// tono es una fila de `toneConfig`: el glifo, el token de la barra y el del icono.
type tono struct {
	icono        iconos.Icono
	barra, trazo string
}

var tonos = [...]tono{
	AvisoInfo:        {iconos.Informacion, "info", "info-text"},
	AvisoExito:       {iconos.Tic, "success", "success-text"},
	AvisoAdvertencia: {iconos.Aviso, "warning", "warning-text"},
	AvisoPeligro:     {iconos.Peligro, "danger", "danger-text"},
}

// Las medidas de Toast.tsx, en px de CSS.
const (
	// AnchoPilaAvisos es `max-w-sm`, que son 24rem.
	AnchoPilaAvisos = 24 * tema.Rem
	// MargenPilaAvisos es el mínimo de 1rem de los márgenes de Toast.tsx.
	// El lienzo móvil ya descuenta el área segura: aquí no se suma de nuevo.
	MargenPilaAvisos = 4 * 0.25 * tema.Rem
	// HuecoPilaAvisos es el `gap-2` entre dos avisos.
	HuecoPilaAvisos = 2 * 0.25 * tema.Rem
)

var (
	anchoBarraAviso = tema.Espacio(1)   // w-1 de la banda de tono: 3,5
	huecoAviso      = tema.Espacio(3)   // gap-3 entre las cuatro piezas: 10,5
	margenYAviso    = tema.Espacio(3)   // py-3 del icono y del bloque de texto
	margenDerAviso  = tema.Espacio(3)   // pr-3
	ladoCerrarAviso = tema.Espacio(7)   // h-7 w-7 del aspa: 24,5
	arribaCerrar    = tema.Espacio(1)   // mt-1 del aspa
	sepDescripcion  = tema.Espacio(0.5) // mt-0.5
	sepAccion       = tema.Espacio(1.5) // mt-1.5
)

// DuracionDe es la vida de un aviso: 5 s, salvo advertencia y peligro, que PERSISTEN
// hasta que alguien los cierra.
//
// No es un capricho: un error que se autodescarta puede perderse sin ser leído.
func DuracionDe(t TonoAviso) time.Duration {
	if t == AvisoAdvertencia || t == AvisoPeligro {
		return 0
	}
	return 5 * time.Second
}

// Aviso es una notificación de Toast.tsx.
type Aviso struct {
	widget.BaseWidget
	Tono   TonoAviso
	Titulo string
	// Descripcion es la segunda línea, en `text-sm` apagado. Vacía: sin segunda línea.
	Descripcion string
	// RotuloAccion y AlPulsarAccion son el enlace de acción («Deshacer»). Vacío: sin
	// acción.
	RotuloAccion   string
	AlPulsarAccion func()
	// Duracion, a cero, es la que le toca a su tono (ver DuracionDe). Negativa: que
	// no se vaya solo.
	Duracion time.Duration
	// RotuloCerrar es el nombre accesible del aspa, YA traducido.
	RotuloCerrar string

	pila  *PilaAvisos
	ancho float32
	// fundido es `sytue-toast-in` con la opacidad de grupo de la tarjeta (fundido.go).
	fundido *fundido
	aspa    *BotonIcono
	// sombra es el `shadow-[var(--shadow-lg)]` de Toast.tsx. La pinta la PILA, de
	// hermano y detrás: el aviso recorta (`avisoRenderer.IsClip`) y se comería la suya.
	sombra *Sombra
}

var _ fyne.Widget = (*Aviso)(nil)

// NuevoAviso crea el aviso.
func NuevoAviso(t TonoAviso, titulo, descripcion string) *Aviso {
	a := &Aviso{Tono: t, Titulo: titulo, Descripcion: descripcion, RotuloCerrar: textoComun("ui.dismissNotification")}
	// `rounded-lg` de la tarjeta del aviso, el mismo radio que su Caja.
	a.sombra = NuevaSombra(tema.SombraLG, tema.RadioLG)
	a.ExtendBaseWidget(a)
	return a
}

// anchoTextoAviso es la columna `flex-1` del texto con la tarjeta a `ancho`: lo que
// dejan los bordes, la banda, el glifo, el aspa, el `pr-3` y los tres `gap-3`.
func anchoTextoAviso(ancho float32) float32 {
	return max(ancho-2*tema.Borde-anchoBarraAviso-3*huecoAviso-iconos.TallaMD-ladoCerrarAviso-margenDerAviso, 0)
}

// Los dos párrafos del aviso: el título en `text-base font-semibold` y la descripción
// en `text-sm`. Son `<p>` y pasan de línea, que un error largo recortado no se lee.
func (a *Aviso) parrafoTitulo() *Parrafo {
	return &Parrafo{Texto: a.Titulo, Escalon: tema.TextoBase, Peso: SemiNegrita}
}

func (a *Aviso) parrafoDescripcion() *Parrafo {
	return &Parrafo{Texto: a.Descripcion, Escalon: tema.TextoSM, Peso: Normal}
}

// AltoBloqueTextoAviso es lo que ocupa la columna de texto de un aviso con la tarjeta a
// `ancho`: `py-3` por arriba y por abajo, el título en las líneas que ocupe, y la
// descripción y la acción si las lleva, cada una con su separación.
func AltoBloqueTextoAviso(a *Aviso, ancho float32) float32 {
	w := anchoTextoAviso(ancho)
	alto := 2*margenYAviso + max(AltoParrafo(a.parrafoTitulo(), w), tema.TextoBase.Linea)
	if a.Descripcion != "" {
		alto += sepDescripcion + AltoParrafo(a.parrafoDescripcion(), w)
	}
	if a.RotuloAccion != "" {
		alto += sepAccion + tema.TextoSM.Linea
	}
	return alto
}

// MedidaAviso es lo que va a ocupar el aviso. Como el resto del paquete, es la ÚNICA
// cuenta.
//
// El alto sale de `items-start`: la tarjeta no tiene relleno vertical propio, así que
// mide lo que su pieza más alta, más sus dos bordes. Las tres piezas que compiten son
// la columna de texto, el icono con su `py-3` (39) y el aspa con su `mt-1` (28).
func MedidaAviso(a *Aviso, ancho float32) fyne.Size {
	alto := max(AltoBloqueTextoAviso(a, ancho), 2*margenYAviso+iconos.TallaMD, arribaCerrar+ladoCerrarAviso)
	return fyne.NewSize(ancho, alto+2*tema.Borde)
}

// CreateRenderer construye la tarjeta: caja con borde, banda de tono, glifo, texto y
// aspa.
func (a *Aviso) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := tonos[a.Tono]
	r := &avisoRenderer{
		a:      a,
		caja:   NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioLG),
		barra:  canvas.NewRectangle(tema.Color(t.barra, v)),
		titulo: NuevoParrafo(a.Titulo, tema.TextoBase, SemiNegrita, "text", v),
	}
	// El aspa de un aviso NO es Button.tsx: Toast.tsx la escribe a mano —`h-7 w-7
	// rounded-sm text-text-subtle hover:text-text`, sin velo de fondo—, así que es un
	// `BotonIcono` con esos tokens y no un `Boton` con `IconoSolo`.
	a.aspa = &BotonIcono{
		Icono: iconos.Cerrar, Lado: ladoCerrarAviso, TamIcono: iconos.TallaSM,
		Radio: tema.RadioSM, Trazo: "text-subtle", TrazoEncima: "text",
		Rotulo: a.RotuloCerrar, AlPulsar: a.Cerrar, fueraDelTab: a.prestado,
	}
	a.aspa.ExtendBaseWidget(a.aspa)

	r.ico = iconos.Trazo(t.icono, iconos.TallaMD, t.trazo, v)
	r.objetos = append(r.caja.Objetos(), r.barra, r.ico.Objeto(), r.titulo)
	if a.Descripcion != "" {
		r.descripcion = NuevoParrafo(a.Descripcion, tema.TextoSM, Normal, "text-muted", v)
		r.objetos = append(r.objetos, r.descripcion)
	}
	if a.RotuloAccion != "" {
		// Un `<button>` con pinta de enlace: ejecuta la acción y cierra el aviso.
		r.accion = NuevoEnlace(a.RotuloAccion, tema.TextoSM, SemiNegrita, "primary-text", func() {
			if a.AlPulsarAccion != nil {
				a.AlPulsarAccion()
			}
			a.Cerrar()
		})
		r.accion.Boton = true
		r.accion.fueraDelTab = a.prestado
		r.objetos = append(r.objetos, r.accion)
	}
	r.objetos = append(r.objetos, a.aspa)
	return r
}

// prestado excluye del Tab los avisos sobre un menú sin diálogo. En el flujo de un
// diálogo, sus controles sí participan en el recorrido del propio diálogo.
func (a *Aviso) prestado() bool {
	return a.pila != nil && a.pila.capaEncima() != nil && a.pila.dialogoEncima() == nil
}

// Cerrar quita el aviso de su pila.
func (a *Aviso) Cerrar() {
	if a.pila != nil {
		a.pila.Descartar(a)
	}
}

type avisoRenderer struct {
	a                   *Aviso
	caja                *Caja
	barra               *canvas.Rectangle
	ico                 *iconos.Dibujo
	titulo, descripcion *Parrafo
	accion              *Enlace
	objetos             []fyne.CanvasObject
}

// IsClip es el `overflow-hidden` de la tarjeta.
func (r *avisoRenderer) IsClip() {}

func (r *avisoRenderer) MinSize() fyne.Size { return MedidaAviso(r.a, r.a.ancho) }

func (r *avisoRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *avisoRenderer) Destroy() {}

// Layout reparte las cuatro piezas de izquierda a derecha con su `gap-3`.
func (r *avisoRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	b := tema.Borde

	// ponytail: la banda de tono es un rectángulo RECTO, y la tarjeta tiene la
	// esquina de 8. En la web el `overflow-hidden` se la recorta con el arco; aquí el
	// recorte de Fyne es rectangular, así que las dos esquinas de la izquierda salen
	// cuadradas. Son unos 2 px² de color en cada una. Se cierra cuando el paquete
	// tenga una primitiva con radios por esquina, no antes: un rectángulo redondeado
	// de 3,5 de ancho y radio 7 sale lenteja, y se ve más que el defecto.
	r.barra.Move(fyne.NewPos(b, b))
	r.barra.Resize(fyne.NewSize(anchoBarraAviso, t.Height-2*b))

	x := b + anchoBarraAviso + huecoAviso
	r.ico.Colocar(x, b+margenYAviso, iconos.TallaMD)
	x += iconos.TallaMD + huecoAviso

	// `flex-1`: la columna de texto se queda con lo que sobra.
	ancho := anchoTextoAviso(t.Width)
	y := b + margenYAviso
	for _, p := range []*Parrafo{r.titulo, r.descripcion} {
		if p == nil {
			continue
		}
		if p == r.descripcion {
			y += sepDescripcion
		}
		alto := AltoParrafo(p, ancho)
		p.Move(fyne.NewPos(x, y))
		p.Resize(fyne.NewSize(ancho, alto))
		y += alto
	}
	if r.accion != nil {
		y += sepAccion
		r.accion.Move(fyne.NewPos(x, y))
		r.accion.Resize(MedidaEnlace(r.accion))
	}

	r.a.aspa.Resize(fyne.NewSize(ladoCerrarAviso, ladoCerrarAviso))
	r.a.aspa.Move(fyne.NewPos(t.Width-b-margenDerAviso-ladoCerrarAviso, b+arribaCerrar))
}

func (r *avisoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("surface-raised", v), tema.Color("border", v))
	r.barra.FillColor = tema.Color(tonos[r.a.Tono].barra, v)
	r.barra.Refresh()
	r.ico.Tenir(tonos[r.a.Tono].trazo, v)
	r.titulo.Texto, r.titulo.Tinta = r.a.Titulo, tema.Color("text", v)
	r.titulo.Refresh()
	if r.descripcion != nil {
		r.descripcion.Tinta = tema.Color("text-muted", v)
		r.descripcion.Refresh()
	}
	if r.accion != nil {
		r.accion.Refresh()
	}
	r.Layout(r.a.Size())
}

// PilaAvisos es el `ToastProvider`: la esquina inferior derecha donde se apilan los
// avisos, con el más nuevo abajo.
//
// Va de HERMANO del contenido, no de capa: ver el comentario de arriba del fichero.
// Quien monta la ventana hace
//
//	container.NewStack(contenido, pila)
//
// y se la pasa a quien tenga que avisar.
// El reloj de cada aviso dispara en SU PROPIA rutina, así que `avisos` y `relojes` van
// bajo cerrojo. Hasta el fork v2.8.1-sytue.4, el driver de pruebas ejecutaba el
// `fyne.Do` del reloj en esa rutina y no en la del test, y el aviso que sobrevivía a su
// test tumbaba al siguiente que el `-shuffle` pusiera detrás. Ahora lo encola (lo
// guarda `TestElFyneDoDeOtraRutinaEsperaAlTest`); el cerrojo se queda porque `Avisos()`
// se puede leer desde cualquier rutina.
type PilaAvisos struct {
	widget.BaseWidget
	// mu guarda los datos: la lista, los relojes y el recorrido que los coloca.
	mu sync.Mutex
	// muPintar serializa el `Refresh()` ENTERO, que es otra cosa: `BaseWidget.Refresh`
	// escribe `themeCache` sin sincronizar (widget.go:125 de Fyne v2.8.1), así que dos
	// rutinas refrescando el mismo widget es carrera aunque los datos estén a salvo.
	//
	// Son DOS cerrojos y no uno porque `Refresh` vuelve a entrar por `Objects()` y
	// `Layout()`: sostener `mu` mientras se refresca se autobloquea. El orden es
	// siempre muPintar → mu, nunca al revés.
	muPintar sync.Mutex
	avisos   []*Aviso
	relojes  map[*Aviso]*time.Timer
}

var _ fyne.Widget = (*PilaAvisos)(nil)

// NuevaPilaAvisos crea la pila, vacía.
func NuevaPilaAvisos() *PilaAvisos {
	p := &PilaAvisos{relojes: map[*Aviso]*time.Timer{}}
	p.ExtendBaseWidget(p)
	return p
}

// Notificar añade el aviso y le pone su reloj, si le toca.
func (p *PilaAvisos) Notificar(a *Aviso) {
	a.pila = p
	d := a.Duracion
	if d == 0 {
		d = DuracionDe(a.Tono)
	}

	p.registrar()
	p.mu.Lock()
	p.avisos = append(p.avisos, a)
	if d > 0 {
		// `fyne.Do` y no una llamada directa: el reloj dispara en OTRA rutina, y
		// desde Fyne 2.6 tocar la interfaz fuera de la principal es carrera.
		p.relojes[a] = time.AfterFunc(d, func() { fyne.Do(func() { p.Descartar(a) }) })
	}
	p.mu.Unlock()

	p.refrescar()
	// `sytue-toast-in`: 180 ms subiendo desde 8 px más abajo y desde transparente, con la
	// opacidad de GRUPO de la tarjeta entera (fundido.go).
	fundir(lienzoDe(p), a, a.sombra, DuracionAviso, DesplazamientoAviso, 1, 0.5, func(f *fundido) {
		p.mu.Lock()
		a.fundido = f
		p.mu.Unlock()
		p.refrescar()
	})
}

// pilas son las pilas de avisos de cada ventana, para que una capa se la lleve encima.
var pilas porLienzo[PilaAvisos]

// registrar apunta la pila en su ventana en cuanto la tiene.
func (p *PilaAvisos) registrar() {
	if c := lienzoDe(p); c != nil {
		pilas.poner(c, p)
	}
}

// pilaDe es la pila de avisos de esa ventana, si tiene.
func pilaDe(c fyne.Canvas) *PilaAvisos { return pilas.de(c) }

// capaEncima es la capa de la suite que hay encima en la ventana de la pila, si hay una:
// es la que se lleva los avisos.
func (p *PilaAvisos) capaEncima() *capa {
	c := lienzoDe(p)
	if c == nil {
		return nil
	}
	cp, _ := c.Overlays().Top().(*capa)
	return cp
}

// objetosAvisos son las sombras y los avisos en orden de pintado: la sombra de cada uno
// justo DELANTE de él, para que la tape su propia tarjeta y no la del de al lado.
func (p *PilaAvisos) objetosAvisos() []fyne.CanvasObject {
	avisos := p.instantanea()
	out := make([]fyne.CanvasObject, 0, 2*len(avisos))
	for _, a := range avisos {
		out = append(out, a.sombra.Objeto(), a)
		p.mu.Lock()
		f := a.fundido.objeto()
		p.mu.Unlock()
		if f != nil {
			out = append(out, f)
		}
	}
	return out
}

// Descartar quita el aviso y para su reloj.
func (p *PilaAvisos) Descartar(a *Aviso) {
	p.mu.Lock()
	for i, x := range p.avisos {
		if x == a {
			p.avisos = append(p.avisos[:i], p.avisos[i+1:]...)
			break
		}
	}
	if t := p.relojes[a]; t != nil {
		t.Stop()
		delete(p.relojes, a)
	}
	p.mu.Unlock()

	p.refrescar()
}

// refrescar es el ÚNICO `Refresh` de la pila, y va con su propio cerrojo: el reloj de un
// aviso refresca desde su rutina y el `fyne.Do` que lo envuelve NO lo lleva a la
// principal en el driver de pruebas —`DoFromGoroutine` llama a la función tal cual
// (test/driver.go:55)—, así que sin esto dos rutinas escriben el `themeCache` del
// widget a la vez.
//
// `mu` tiene que estar SUELTO al entrar aquí: dentro, Fyne vuelve a pedirlo por
// `Objects()` y `Layout()`.
func (p *PilaAvisos) refrescar() {
	p.muPintar.Lock()
	defer p.muPintar.Unlock()
	p.Refresh()
	d := p.dialogoEncima()
	if d != nil {
		d.Refresh()
	}
	if cp := p.capaEncima(); cp != nil && cp.panel != fyne.CanvasObject(d) {
		cp.Refresh()
	}
}

// Avisos son los que hay en pantalla, del más viejo al más nuevo.
func (p *PilaAvisos) Avisos() []*Aviso { return p.instantanea() }

// instantanea copia la lista bajo el cerrojo para que quien la recorra lo haga fuera de
// él. Es el ÚNICO sitio por el que leen el renderer y Avisos: sin la copia, un reloj que
// descarte a mitad de un recorrido lo deja leyendo un hueco.
func (p *PilaAvisos) instantanea() []*Aviso {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.avisos)
}

// CreateRenderer construye la pila. No pinta fondo: es un hueco transparente del
// tamaño de la ventana con los avisos colocados en su esquina.
func (p *PilaAvisos) CreateRenderer() fyne.WidgetRenderer { return &pilaRenderer{p: p} }

type pilaRenderer struct{ p *PilaAvisos }

func (r *pilaRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }

// Objects son los avisos, salvo que haya una capa encima: entonces los pinta ella.
func (r *pilaRenderer) Objects() []fyne.CanvasObject {
	if r.p.capaEncima() != nil {
		return nil
	}
	return r.p.objetosAvisos()
}

func (r *pilaRenderer) Destroy() {}

// Layout apila los avisos desde abajo a la derecha, con el más nuevo el último —que es
// el orden en que la web los pinta dentro de su `flex-col`.
func (r *pilaRenderer) Layout(t fyne.Size) {
	r.p.registrar()
	if r.p.capaEncima() != nil {
		return // los coloca la capa, en coordenadas de la ventana
	}
	r.p.colocar(t)
}

// colocar pone los avisos en la esquina de abajo a la derecha de un hueco `t`: el de la
// pila o, prestados, el de la capa, que es la ventana entera.
func (p *PilaAvisos) colocar(t fyne.Size) {
	// Toast.tsx resta los márgenes laterales antes del tope de 24rem. `t` ya es
	// el área interactiva, tanto en la pila como en la capa que toma sus avisos.
	ancho := min(AnchoPilaAvisos, max(t.Width-2*MargenPilaAvisos, 0))
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x := t.Width - MargenPilaAvisos - ancho
	y := t.Height - MargenPilaAvisos

	// Bajo el cerrojo, y NO por `instantanea`: este bucle ESCRIBE en cada aviso
	// (`ancho`, Resize, Move), y el reloj de otro aviso puede estar recorriéndolo a la
	// vez desde su propia rutina —`Descartar` refresca donde dispara—. Copiar la lista
	// solo protege la lista; hay que serializar el RECORRIDO entero.
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := len(p.avisos) - 1; i >= 0; i-- {
		a := p.avisos[i]
		tam := MedidaAviso(a, ancho)
		y -= tam.Height
		colocarAviso(a, ancho, fyne.NewPos(x, y), v)
		y -= HuecoPilaAvisos
	}
}

func (r *pilaRenderer) Refresh() { r.Layout(r.p.Size()) }

func colocarAviso(a *Aviso, ancho float32, pos fyne.Position, v tema.Variante) {
	a.ancho = ancho
	tam := MedidaAviso(a, ancho)
	a.Resize(tam)
	a.Move(pos)
	if a.fundido.objeto() == nil {
		a.sombra.Ajustar(pos, tam, v)
	}
}
