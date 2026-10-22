// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las piezas que comparten las seis pantallas del Hub. Ninguna es un widget nuevo de
// la suite: son composiciones de `ui` con las medidas de esta app.

// ---- La cabecera de página ----

// La cabecera que abre las pantallas del Hub: un sobretítulo en versalitas y el H1.
// Medido en producción (inicio y actividad, 1280×800):
//
//	sobretítulo  10,5/14 · 600 · tracking 0,06em · versalitas · token text-subtle
//	H1           31,5/36,75 · 700 · `mt-1.5` = 5,25 por encima
//	intro        12,25/19,25 · 400 · `mt-2` = 7, y solo la llevan tres pantallas
//
// Sin intro son 56 justos (14 + 5,25 + 36,75), que es lo que mide `header.mb-8` del
// lanzador.
// Es `rounded-card`, un radio de la FORMA (16 en Marco, 14 en Carril): una función, que
// la forma se elige con la app abierta.
func RadioTarjeta() float32 { return tema.Forma("radius-card") }

const (
	// MargenTitulo es el `mt-1.5` entre el sobretítulo y el H1.
	MargenTitulo float32 = 1.5 * 0.25 * tema.Rem
	// MargenIntro es el `mt-2` entre el H1 y el párrafo de entrada.
	MargenIntro float32 = 2 * 0.25 * tema.Rem
	// AnchoIntro es el `max-w-2xl` del párrafo de entrada: 42rem con el rem de 14.
	AnchoIntro float32 = 42 * tema.Rem
)

// cabecera es la cabecera de página ya compuesta: no es un widget porque no es
// interactiva ni anunciable por sí misma (los encabezados no son un rol de Fyne 2.8:
// solo hay botón, contenedor, enlace y texto).
type cabecera struct {
	sobre *ui.Versal
	// rotulo es el H1 y titulo, sus líneas ya repartidas: un `<h1>` pasa de línea si no
	// cabe («Buenas tardes, Ana» a 360), y en una sola se salía de la hoja.
	rotulo string
	titulo []*canvas.Text
	intro  []*canvas.Text
	texto  string
}

func nuevaCabecera(v tema.Variante, sobretitulo, titulo, intro string) *cabecera {
	c := &cabecera{
		sobre:  ui.NuevoVersal(sobretitulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v)),
		rotulo: titulo,
		texto:  intro,
	}
	return c
}

// alto es lo que ocupa a un ancho dado: el título y el párrafo se reparten en líneas.
func (c *cabecera) alto(ancho float32) float32 {
	h := tema.TextoXS.Linea + MargenTitulo + float32(len(c.lineasTitulo(ancho)))*ui.EscalonTitulo(1).Linea
	if c.texto != "" {
		h += MargenIntro + float32(len(c.lineas(ancho)))*tema.TextoBase.Linea
	}
	return h
}

// lineasTitulo es el H1 repartido en ese ancho. Siempre hay al menos una: un título
// vacío sigue ocupando su línea.
func (c *cabecera) lineasTitulo(ancho float32) []string {
	if l := ui.Repartir(c.rotulo, ui.EscalonTitulo(1), ui.PesoTitulo(1), ancho); len(l) > 0 {
		return l
	}
	return []string{c.rotulo}
}

func (c *cabecera) lineas(ancho float32) []string {
	return ui.Repartir(c.texto, tema.TextoBase, ui.Normal, min(ancho, AnchoIntro))
}

// objetos son las primitivas, para el `Objects()` del renderer que la contiene.
func (c *cabecera) objetos() []fyne.CanvasObject {
	objs := c.sobre.Objetos()
	for _, l := range c.titulo {
		objs = append(objs, l)
	}
	for _, l := range c.intro {
		objs = append(objs, l)
	}
	return objs
}

// preparar crea los objetos de texto del título y del párrafo para ese ancho. Se llama
// ANTES de objetos(), porque el número de líneas depende del ancho y el renderer
// necesita la lista completa desde el principio.
func (c *cabecera) preparar(v tema.Variante, ancho float32) {
	c.titulo, c.intro = nil, nil
	for _, l := range c.lineasTitulo(ancho) {
		c.titulo = append(c.titulo, ui.Texto(l, ui.EscalonTitulo(1), ui.PesoTitulo(1), tema.Color("text", v)))
	}
	if c.texto == "" {
		return
	}
	for _, l := range c.lineas(ancho) {
		c.intro = append(c.intro, ui.Texto(l, tema.TextoBase, ui.Normal, tema.Color("text-muted", v)))
	}
}

// colocar pone la cabecera arriba de la página, en (x, MargenY) —el `py-8` de la caja
// de contenido—, y devuelve dónde acaba.
//
// ponytail: el H1 lleva `letter-spacing: -0.01em` (−0,315 px por letra a 31,5) y aquí
// no se aplica: `ui.Versal` es el único que sabe repartir tracking y fija el positivo
// del sobretítulo. En «Tus aplicaciones» son 5 px de más al final de un rótulo
// alineado a la izquierda. Se cierra el día que `NuevoVersal` acepte el tracking, que
// es el mismo hueco que ya está apuntado en roadmap.md para `tracking-wide`.
func (c *cabecera) colocar(x float32) float32 { return c.colocarEn(x, MargenY) }

// colocarEn es colocar con otro margen de página: el `marco:py-5` de la portada.
func (c *cabecera) colocarEn(x, y float32) float32 {
	c.sobre.Colocar(x, y)
	y += tema.TextoXS.Linea + MargenTitulo
	for _, l := range c.titulo {
		ui.ColocarTexto(l, ui.EscalonTitulo(1), x, y)
		y += ui.EscalonTitulo(1).Linea
	}
	if len(c.intro) > 0 {
		y += MargenIntro
		for _, l := range c.intro {
			ui.ColocarTexto(l, tema.TextoBase, x, y)
			y += tema.TextoBase.Linea
		}
	}
	return y
}

// ---- La tarjeta de sección ----

// Una `Card` con un `SectionHeader` pegado arriba, que es como el Hub monta sus tres
// tarjetas. No es `ui.Tarjeta`: esa lleva `CardHeader` (`px-4 py-3`, 44,75 de alto) y
// aquí la cabecera es `SectionHeader` (`min-h-12`, 42 medidos) y va a ras del borde,
// sin el `p-4` del cuerpo.
//
// Medido en producción (tarjeta «Hoy», 1280×800): 398,66 × 147,25 = 1 de borde + 42 de
// cabecera + 103,25 de cuerpo + 1 de borde; y el cuerpo son 14 de relleno + 36,75 del
// párrafo + 38,5 del pie de correo + 14.
//
// Y el relleno del cuerpo es 14 SIEMPRE, también donde el JSX pide `p-0` o `p-2`: las
// clases de `CardBody` ganan a las que le pasan, y así se mide en producción (las
// filas de «Reciente» ocupan 1207,98 dentro de una tarjeta de 1237,98). Se copia lo
// que la web HACE, no lo que su JSX parece decir.
const (
	// MargenTarjeta es el `px-4 py-4` del cuerpo de la tarjeta.
	MargenTarjeta float32 = 4 * 0.25 * tema.Rem
	// RadioTarjeta() es el `rounded-lg` de Card.tsx.
)

type tarjeta struct {
	widget.BaseWidget
	sombra   *ui.Sombra
	caja     *ui.Caja
	cabecera *ui.CabeceraApp
	cuerpo   fyne.CanvasObject
}

// nuevaTarjeta compone la tarjeta. `acciones` son los enlaces de la derecha de la
// cabecera.
func nuevaTarjeta(v tema.Variante, titulo string, cuerpo fyne.CanvasObject, acciones ...fyne.CanvasObject) *tarjeta {
	t := &tarjeta{
		// `shadow-sm`, medida en producción sobre las tres tarjetas del lanzador:
		// `rgba(17,35,43,0.08) 0 1px 3px, rgba(17,35,43,0.04) 0 1px 2px`, que es
		// exactamente `tema.SombraSM`.
		sombra:   ui.NuevaSombra(tema.SombraSM, RadioTarjeta()).SinInterior(),
		caja:     ui.NuevaCaja(tema.Color("surface", v), ui.BordeSobre("border-subtle", "surface", v), RadioTarjeta()).ConTrazo(),
		cabecera: ui.NuevaCabeceraApp(titulo, acciones...),
		cuerpo:   cuerpo,
	}
	t.ExtendBaseWidget(t)
	return t
}

// fondo es el de `Card`: la superficie blanca. Fue `.hub-section-card` —gris en reposo
// y azul pastel al pasar el puntero o al enfocar algo de dentro—, que la web retiró con
// la forma Carril: la sección ya no cambia con el puntero.
func (t *tarjeta) fondo() string { return "surface" }

func (t *tarjeta) CreateRenderer() fyne.WidgetRenderer { return &tarjetaRenderer{t: t} }

type tarjetaRenderer struct{ t *tarjeta }

func (r *tarjetaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *tarjetaRenderer) Objects() []fyne.CanvasObject { return r.t.caja.Objetos() }
func (r *tarjetaRenderer) Destroy()                     {}
func (r *tarjetaRenderer) Layout(tam fyne.Size)         { r.t.caja.Redimensionar(tam) }
func (r *tarjetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := r.t.fondo()
	r.t.caja.Pintar(tema.Color(fondo, v), ui.BordeSobre("border-subtle", fondo, v))
}

// AltoCabecera es el `min-h-12` de SectionHeader, con su filete dentro.
func (t *tarjeta) altoCabecera() float32 { return ui.MedidaCabeceraApp(t.cabecera).Height }

// alto es lo que ocupa la tarjeta con un cuerpo de ese alto interior.
func (t *tarjeta) alto(interior float32) float32 {
	return 2*tema.Borde + t.altoCabecera() + interior + 2*MargenTarjeta
}

func (t *tarjeta) objetos() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{t.sombra.Objeto(), t}
	objs = append(objs, t.cabecera)
	if t.cuerpo != nil {
		objs = append(objs, t.cuerpo)
	}
	return objs
}

// colocar pone la tarjeta en (x, y) con ese ancho y un cuerpo de `interior` de alto.
//
// La SOMBRA no reserva sitio y el padre no la recorta, que es lo que hace `box-shadow`
// en CSS: no participa en el layout y se pinta fuera de la caja. En Fyne sale gratis
// porque el pintor no recorta los objetos de un renderer a la caja del widget —lo
// mismo que ya aprovecha `ui.Aro`, y está comprobado en `TestElAroSePintaFueraDeLaCaja`—.
// Lo único que hay que respetar es el ORDEN: la sombra va la primera de `objetos()`,
// detrás del relleno opaco de la caja.
func (t *tarjeta) colocar(x, y, ancho, interior float32) {
	// Medir antes de pintar: una cabecera recién creada todavía tiene ancho cero
	// y supone una fila. En móvil el enlace puede bajar a otra y necesita más papel.
	t.cabecera.Ancho = max(ancho-2*tema.Borde, 0)
	alto := t.alto(interior)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t.sombra.Ajustar(fyne.NewPos(x, y), fyne.NewSize(ancho, alto), v)
	t.Move(fyne.NewPos(x, y))
	t.Resize(fyne.NewSize(ancho, alto))
	t.cabecera.Move(fyne.NewPos(x+tema.Borde, y+tema.Borde))
	t.cabecera.Resize(fyne.NewSize(ancho-2*tema.Borde, t.altoCabecera()))
	if t.cuerpo != nil {
		t.cuerpo.Move(fyne.NewPos(x+tema.Borde+MargenTarjeta, y+tema.Borde+t.altoCabecera()+MargenTarjeta))
		t.cuerpo.Resize(fyne.NewSize(ancho-2*(tema.Borde+MargenTarjeta), interior))
	}
}

// AnchoInterior es lo que le queda al cuerpo de una tarjeta de ese ancho.
func AnchoInterior(ancho float32) float32 {
	return max(ancho-2*(tema.Borde+MargenTarjeta), 0)
}

// ---- Los cinco estados que no son «listo» ----

// Textos son las claves con las que UNA pantalla nombra sus dos estados propios: «no
// hay nada» y «el servicio ha fallado». Los otros tres —cargando, sin conexión y
// permiso denegado— son iguales en toda la suite y salen del catálogo común, así que
// no se declaran aquí.
type Textos struct {
	Vacio, VacioCuerpo string
	Fallo, FalloCuerpo string
	// Accion es la clave del botón del estado vacío; vacía, no hay botón.
	Accion string
}

// Aviso construye el bloque que sustituye al contenido cuando el estado no es Listo.
//
// Los tres estados comunes usan las claves del catálogo compartido y no unas propias:
// `offline.status` / `error.connection` y `forbidden.*` ya existen y los enseña igual
// cualquier otra app de la suite, que es justo lo que hace que un fallo se reconozca.
func Aviso(t *i18n.Translator, v tema.Variante, e Estado, x Textos, accion func()) *ui.Vacio {
	var titulo, cuerpo, boton string
	var icono iconos.Icono
	switch e {
	case SinNada:
		titulo, cuerpo, boton = x.Vacio, x.VacioCuerpo, x.Accion
	case Fallo:
		titulo, cuerpo, boton, icono = x.Fallo, x.FalloCuerpo, "action.retry", iconos.Aviso
	case SinConexion:
		titulo, cuerpo, boton, icono = "offline.status", "error.connection", "action.retry", iconos.Nube
	case Prohibido:
		titulo, cuerpo, boton, icono = "forbidden.title", "forbidden.description", "forbidden.action", iconos.Candado
	case Cargando, Listo:
		return nil
	}
	descripcion := ""
	if cuerpo != "" { // sin cuerpo, sin párrafo, como `EmptyState` sin `description`
		descripcion = t.T(cuerpo, nil)
	}
	vacio := ui.NuevoVacio(t.T(titulo, nil), descripcion)
	if icono != iconos.Ninguno {
		// `<Icon name="warning" size={32} class="text-danger-text" />`: el icono va
		// suelto, sin tesela de fondo, como en EmptyState.tsx.
		//
		// ponytail: la web pone `Illustration name="emptySearch"` de 120 en el estado
		// vacío de la actividad reciente. Las trece ilustraciones de `branding` usan
		// `var(--sytue-primary-500)` sin valor de reserva y eso tumba el fichero
		// entero en oksvg (roadmap.md, «Piezas que faltan»): hasta que se sustituya el
		// token por su hex antes de rasterizar, el vacío va sin dibujo.
		vacio.Ilustracion = Glifo(iconos.Trazo(icono, 32, tintaDe(e), v), 32)
	}
	if boton != "" && accion != nil {
		vacio.Accion = ui.NuevoBoton(t.T(boton, nil), accion).Con(ui.Secundario, ui.TallaSM)
	}
	return vacio
}

// tintaDe es el token del icono de cada estado. El fallo va en `danger-text`, que es
// lo que pinta la web; los otros dos en `text-muted`, que es el del EmptyState.
func tintaDe(e Estado) string {
	if e == Fallo {
		return "danger-text"
	}
	return "text-muted"
}

// Glifo entrega la primitiva de un icono ya con su MINIMO declarado.
//
// Hace falta porque `canvas.Raster` sin mínimo contesta 1×1, y quien lo recibe —el
// estado vacío, una fila de lista— coloca por `MinSize()`: sin esto el icono de 32 se
// pinta de un píxel. `Dibujo.Objeto()` es siempre un `*canvas.Raster`, y así está
// escrito en su documentación.
// El `SetMinSize` que hacía aquí lo hace ya `iconos.nuevoDibujo` desde el 2026-09-22:
// era el mismo apaño en dos sitios y en `apps/drive` faltaba, así que sus estados
// vacíos pintaban la ilustración invisible. `lado` se conserva en la firma para no
// tocar las once llamadas; es el mismo con el que se construyó el dibujo.
func Glifo(d *iconos.Dibujo, _ float32) fyne.CanvasObject { return d.Objeto() }

// ---- Los filetes de `divide-y` ----

// Divisores son los filetes que `divide-y` pinta ENTRE los hijos de una lista: n − 1
// para n filas, del color de `border`.
//
// Hacen falta de verdad y no son decoración: las filas de esta app tienen el fondo
// transparente —solo se tiñe al pasar el puntero—, así que sin un rectángulo el hueco
// de un píxel que el layout deja entre dos filas enseña la superficie de la tarjeta y
// las filas se leen pegadas. El alto YA está contado en la medida de cada lista.
func Divisores(v tema.Variante, n int) []*canvas.Rectangle {
	if n < 2 {
		return nil
	}
	out := make([]*canvas.Rectangle, 0, n-1)
	for range n - 1 {
		out = append(out, canvas.NewRectangle(tema.Color("border", v)))
	}
	return out
}

// ColocarDivisor pone un filete a lo ancho, en (x, y).
func ColocarDivisor(r *canvas.Rectangle, x, y, ancho float32) {
	r.Move(fyne.NewPos(x, y))
	r.Resize(fyne.NewSize(ancho, tema.Borde))
}
