// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"runtime"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// La barra transversal de la suite: la marca de la app a la izquierda, el buscador
// centrado y los avisos y el menú de cuenta a la derecha, en las trece aplicaciones.
//
// # Qué es y qué no
//
// Es la `TopBar` de la web (`packages/ui/src/components/TopBar.tsx` con lo que monta
// `SytueShell.tsx`) SIN el selector de aplicaciones. No lo lleva porque en nativo cada
// producto es su propia aplicación y de una a otra se va por el sistema, no dentro de
// una ventana (`docs/CONVENTIONS.md` §13.6, excepción 1). Hasta el 2026-09-22 esa
// sección decía que la barra no se portaba; lo corrigió el dueño y esto es la
// corrección.
//
// Lo que SÍ queda del selector es su cara: la marca de la app y su nombre, en el hueco
// `brand` de la web. Allí es el disparador del selector con la marca de la
// organización; aquí es la marca del PRODUCTO —la de la tesela del selector de
// aplicaciones, `ProductIcon`—, sin galón porque no hay nada que desplegar (dueño,
// 2026-09-23: «a las apps de Fyne les falta su logotipo de cada app»).
//
// # Por qué es MÁS FINA, y de cuánto a cuánto
//
// Medido en producción el 2026-09-22 (`test/e2e/auditar-topbar.mjs`, Sytue Mail a
// 1440×900): la barra de la web mide **56** y todos sus controles **31,5**, repartidos
// con **12,3 arriba y 12,3 abajo** —los 12,25 de `py-3.5`, que es lo que hay que dejar
// para que respire una barra que lleva el logotipo de la suite (una imagen de 28 en un
// botón de 31,5) y el nombre del producto—.
//
// Sin el selector, la marca va suelta y no dentro de un botón: sus 28 caben en la fila
// de 31,5 de los controles, y el aire que les corresponde es `py-2`. O sea
// **45,5 = 31,5 + 2×7**, que son
// **10,5 px menos, un 18,75 %**. Y 45,5 cae en medio píxel, así que a ×2 —cualquier
// Retina y cualquier teléfono— son 91 filas de dispositivo CLAVADAS (`tema/medidas.go`).
//
// Con el DEDO no se adelgaza, y no es una excepción sino la misma regla: la web le
// pone a estos botones un suelo de 44 px (`pointer-coarse:min-h-[max(44px,2.75rem)]`)
// porque un blanco de 31,5 no se acierta con el pulgar, y 44 + 2×7 son 58. En un
// teléfono la barra mide eso.
const (
	// LadoControlPuntero es el `h-9 w-9` de los botones de la barra.
	LadoControlPuntero = 9 * 0.25 * tema.Rem
	// SueloTactilBarra es el `pointer-coarse:min-h-[max(44px,2.75rem)]`.
	SueloTactilBarra float32 = 44
	// RellenoBarraY es el `py-2` que sustituye al `py-3.5` de la web.
	RellenoBarraY = 2 * 0.25 * tema.Rem
	// CanalonBarra es el `px-4` y CanalonBarraLG el `lg:px-6`, que entra a partir de
	// CorteCanalonBarra. El canalón crece con la ventana por la misma razón que en la
	// web: a 14 px del cristal en un monitor de 1440 el cromo se lee como pinchado
	// contra el borde.
	CanalonBarra               = 4 * 0.25 * tema.Rem
	CanalonBarraLG             = 6 * 0.25 * tema.Rem
	CorteCanalonBarra          = 1024                  // el `lg:` de la hoja, que mide la VENTANA y en px
	HuecoAccionesBarra         = 1 * 0.25 * tema.Rem   // `gap-1` entre las acciones
	HuecoBarra                 = 3 * 0.25 * tema.Rem   // `gap-3` entre las zonas
	MargenDivisorBarra         = 1 * 0.25 * tema.Rem   // el `mx-1` del filete vertical
	AltoDivisorBarra           = 6 * 0.25 * tema.Rem   // su `h-6`
	MargenCuentaBarra          = 0.5 * 0.25 * tema.Rem // el `ml-0.5` del avatar
	AnchoBuscadorBarra         = 30 * tema.Rem         // el `max-w-[30rem]`: 420 con el rem de 14
	TamGlifoBarra      float32 = 20                    // `<Icon size={20}>`
	// El punto de aviso: `h-2 w-2` en `top-2 right-2` con `ring-2` del fondo de la
	// barra, que es lo que lo despega del glifo de la campana.
	LadoPuntoAviso           = 2 * 0.25 * tema.Rem
	MargenPuntoAviso         = 2 * 0.25 * tema.Rem
	AnilloPuntoAviso float32 = 2
	// AroCarril es el aro de lo que va directo sobre la barra DE COLOR —el punto de aviso y
	// el avatar de la cuenta—: `.sytue-aro-carril` de appearance.css,
	// `box-shadow: 0 0 0 1.5px var(--sytue-sidebar-fg)`. La barra puede ser del mismo tono
	// que el punto o que el disco de la persona (con el color de Mail, rojo sobre rojo).
	AroCarril float32 = 1.5
)

// fondoBarra es el token que pinta la barra: `topbar-bg`, el `bg-topbar` de la web.
//
// Con la forma Carril el color de marca vive en el menú lateral y la barra es clara
// (dueño, 2026-10-04: las nativas van igual que la web). Hasta entonces la barra llevaba
// el color del menú —regla del 2026-09-23, de cuando los dos eran el mismo cromo en L—,
// y con el carril de color eso daba una barra de color con su contenido oscuro encima.
// Lo usa también el anillo del punto de aviso, porque tiene que ser el fondo que hay
// detrás: con otro, el anillo se ve como un aro.
const fondoBarra = "topbar-bg"

// fondoEsquina es el token de la ESQUINA DE MARCA: la zona de la marca pintada del color
// del carril (`rail` en TopBar.tsx, que le pone `bg-sidebar`). Encima va el nombre del
// producto en `sidebar-fg`.
const fondoEsquina = "sidebar-bg"

// La esquina de marca, con las medidas de TopBar.tsx: `ps-4 pe-3` —el `px-4` del menú
// lateral a la izquierda, para que la marca caiga en la vertical del botón de su
// cabecera— y, desde `md:`, el ancho del menú (`md:w-[var(--spacing-sidebar)]`).
const (
	RellenoEsquinaDer         = 3 * 0.25 * tema.Rem
	CorteEsquinaMarca float32 = 768
)

// AnchoEsquinaMarca es lo que RESERVA la esquina de marca en una ventana de ese ancho,
// con una marca que ocupa `marca`: el carril entero desde 768 y, por debajo, lo que pida
// la marca con su relleno. Lo que PINTA puede ser menos: ver `esquinaCenida`.
func AnchoEsquinaMarca(ventana, marca float32) float32 {
	if ventana >= CorteEsquinaMarca {
		return tema.AnchoLateral
	}
	return esquinaCenida(marca)
}

// esquinaCenida es la esquina de marca ceñida a su contenido, la marca con su `ps-4
// pe-3`: la de <768 y, en Carril, la que PINTA una app sin menú lateral
// (`.sytue-esquina-marca { width: fit-content }` en appearance.css). Pintar ahí el ancho
// de un menú que no existe dejaba un bloque de color suelto.
func esquinaCenida(marca float32) float32 {
	return canalonMenu - RellenoMarcaIzq + marca + RellenoEsquinaDer
}

// comun es el catálogo común en el idioma de la app, para los textos PROPIOS de las
// piezas de `ui` que en la web salen de ese catálogo: el aspa de un aviso
// (`ui.dismissNotification`), la de un diálogo (`ui.close`), la presencia de un avatar,
// la casilla de una fila, la tabla vacía. Lo deja la barra, que cada app monta al
// arrancar y que es la pieza que ya sabe el idioma. Estaban escritos a mano en
// castellano y así salían en los veintiséis idiomas.
var comun atomic.Pointer[i18n.Translator]

// comunPorDefecto es el respaldo sin barra —los guardianes—: el inglés de la suite.
var comunPorDefecto = sync.OnceValue(func() *i18n.Translator {
	t, _ := i18n.New(i18n.Default, i18n.Common())
	return t
})

// textoComun es la clave del catálogo común en el idioma de la app.
func textoComun(clave string) string {
	t := comun.Load()
	if t == nil {
		t = comunPorDefecto()
	}
	return t.T(clave, nil)
}

// LadoControlBarra es el lado de los botones de la barra: 31,5 con puntero y el suelo
// táctil de 44 en un teléfono.
func LadoControlBarra() float32 {
	if EsTactil() {
		return SueloTactilBarra
	}
	return LadoControlPuntero
}

// AltoBarraSuite es lo que mide la barra: el control más su `py-2`.
func AltoBarraSuite() float32 { return LadoControlBarra() + 2*RellenoBarraY }

// EsTactil es el `pointer: coarse` de la hoja. Fyne no tiene consultas de medio, pero
// sí sabe si corre en un teléfono, que es donde la suite se maneja con el dedo.
//
// Por el sistema y no por `EsTactil()`, que dice lo mismo pero
// necesita la app: hay medidas que se calculan en el `init` de un paquete, antes de
// `app.New`, y ahí `CurrentDevice` es un puntero nulo (tumbaba Sytue Mail al arrancar).
// Es la ÚNICA pregunta de «¿teléfono?» de la suite: las trece apps la importan en vez
// de llamar a `fyne.CurrentDevice()`, que revienta sin app (lo vigila
// `TestNadiePreguntaAlDispositivo`).
func EsTactil() bool {
	if tactilDePrueba != nil {
		return *tactilDePrueba
	}
	return runtime.GOOS == "android" || runtime.GOOS == "ios"
}

// tactilDePrueba hace de teléfono en un guardián: el driver de pruebas es de escritorio.
var tactilDePrueba *bool

// TactilDePrueba hace de teléfono (o de escritorio) en los guardianes de OTROS paquetes,
// que no llegan a tactilDePrueba; devuelve con qué volver a como estaba. Solo pruebas: en
// una app, quien decide es el sistema.
func TactilDePrueba(si bool) (restaurar func()) {
	antes := tactilDePrueba
	tactilDePrueba = &si
	return func() { tactilDePrueba = antes }
}

// CanalonDeBarra es el relleno lateral para esa anchura de ventana (`px-4 lg:px-6`).
func CanalonDeBarra(ancho float32) float32 {
	if ancho >= CorteCanalonBarra {
		return CanalonBarraLG
	}
	return CanalonBarra
}

// BarraSuite es la barra transversal.
type BarraSuite struct {
	widget.BaseWidget

	marca    *marcaBarra
	buscador *buscadorBarra
	avisos   *avisosBarra
	cuenta   *cuentaBarra

	t *i18n.Translator
	s *sesion.Suite
	// sinSesionPropia: la app pinta ella misma lo que ve quien no ha entrado (ver
	// ConSinSesionPropia) y Montar no pone la página «Iniciar sesión».
	sinSesionPropia bool
	// hojaASangre: en la forma Marco la hoja de esta app va sin separación del borde y
	// solo con las esquinas de arriba (ver ConHojaASangre).
	hojaASangre bool
	// montado es lo que devolvió Montar: la barra le avisa cuando cambia la apariencia,
	// que es cuando hay que volver a colocar el marco y la hoja.
	montado *fyne.Container
	// pintada es la apariencia con la que se colocó el montaje por última vez.
	pintada aparienciaVista
	// buscarPendiente espera a tener sesión y lienzo antes de mostrar el teclado.
	buscarPendiente bool
	busquedaSesion  sesion.Estado
}

// aparienciaVista es lo que decide cómo se pinta el marco: el tema, el color y la forma.
type aparienciaVista struct {
	v     tema.Variante
	color string
	marco bool
}

func aparienciaDeAhora() aparienciaVista {
	return aparienciaVista{tema.De(fyne.CurrentApp().Settings().ThemeVariant()), tema.ColorElegido(), tema.EsMarco()}
}

var (
	_ fyne.Widget     = (*BarraSuite)(nil)
	_ fyne.Accessible = (*BarraSuite)(nil)
)

// NuevaBarraSuite construye la barra sobre lo transversal de esta app.
//
// `s` puede ser nil —en los guardianes lo es— y entonces la barra no pide nada: cada
// pieza enseña su estado de «sin sesión», que es parte del encargo y no un respaldo.
// Una barra que solo se ve bien con datos es una barra que no se ha probado, y hoy
// las trece arrancan sin nadie dentro.
//
// `p` es el producto de la app, que pone su marca y su nombre a la izquierda: el
// `currentApp` de la web (Docus es `iconos.Docs`, Calcus `iconos.Sheets`, Diapos
// `iconos.Slides`, CertiPDF `iconos.PDF` y el Hub, que es el portal, `iconos.One`).
func NuevaBarraSuite(t *i18n.Translator, s *sesion.Suite, p iconos.Producto) *BarraSuite {
	// Los textos de la barra salen SOLO del catálogo común, en el idioma de la app: en
	// la web la monta `SytueShell` con su propio `I18nProvider`. Con el traductor de la
	// app ganaban las claves que la app redefine, y el buscador de la SUITE decía
	// «Buscar mensajes» en Rooms y «Título, descripción, lugar o invitado…» en Calendar.
	if c, err := i18n.New(t.Locale(), i18n.Common()); err == nil {
		t = c
		comun.Store(c)
	}
	// Cada app nativa es UN producto: es el `data-app` de la web, que con el color «el de
	// cada app» decide el carril y la acción. La barra es quien sabe cuál.
	tema.ElegirApp(p.Clave())
	b := &BarraSuite{t: t, s: s}
	b.marca = nuevaMarcaBarra(p)
	b.buscador = nuevoBuscadorBarra(t, s)
	b.avisos = nuevoAvisosBarra(t, s)
	b.cuenta = nuevaCuentaBarra(t, s)
	// La foto cambia EN VIVO: el aviso llega por el flujo de la campana y la cuenta
	// vuelve a pedir la ficha, que trae la versión nueva (o ninguna, si la quitó).
	b.avisos.alFoto = func() { go b.cuenta.pedirFicha() }
	b.ExtendBaseWidget(b)
	b.engancharBusqueda()
	return b
}

// Montar envuelve el contenido de la aplicación con la barra encima y arranca sus
// peticiones. Es lo que cada `main` pone en `SetContent`, en UNA línea.
//
// El contenido va PRIMERO en la lista y la barra después, aunque la barra se pinte
// arriba: en Fyne el orden de la lista es el orden de pintado, y los paneles que
// cuelgan de la barra —los resultados de la búsqueda— tienen que quedar POR ENCIMA del
// contenido. Con la barra primero, la bandeja de Mail se pintaba encima de ellos.
//
// Y se apunta a enseñar la página «no encontrada» (`sesion.AlNoEncontrar`): la pone EN
// LUGAR del contenido, con la barra encima, y su botón devuelve el contenido tal como
// estaba, que es el inicio de la app.
//
// En la forma Marco, además, pinta el marco y la HOJA de lo que no trae armazón —el Hub,
// los editores, la página «Iniciar sesión»—; con armazón, la hoja es suya (ver marco.go).
func (b *BarraSuite) Montar(contenido fyne.CanvasObject) fyne.CanvasObject {
	b.Arrancar()
	m := &montaje{
		marco: nuevoMarco(), aSangre: b.hojaASangre,
		superior:  canvas.NewRectangle(color.NRGBA{}),
		laterales: [2]*canvas.Rectangle{canvas.NewRectangle(color.NRGBA{}), canvas.NewRectangle(color.NRGBA{})},
	}
	objetos := append([]fyne.CanvasObject{m.superior, m.laterales[0], m.laterales[1]}, m.marco.detras()...)
	objetos = append(objetos, contenido)
	m.contenido = len(objetos) - 1
	objetos = append(append(objetos, m.marco.delante()...), b)
	c := container.New(m, objetos...)
	b.montado, b.pintada = c, aparienciaDeAhora()
	poner := func(o fyne.CanvasObject) {
		c.Objects[m.contenido] = o
		c.Refresh()
	}
	if b.s != nil {
		b.s.AlNoEncontrar(func() {
			poner(NuevaPaginaNoEncontrada(b.t, func() { poner(contenido) }))
		})
		b.s.AlConfirmarSubidaLocal(b.confirmarSubidaLocal)
		if !b.sinSesionPropia {
			b.montarSinSesion(poner, contenido)
		}
	}
	return c
}

// confirmarSubidaLocal pregunta antes de subir al servicio un fichero local abierto desde
// el sistema, con una casilla para recordar la elección en este dispositivo (FYP-02).
func (b *BarraSuite) confirmarSubidaLocal(w fyne.Window, nombre string, aceptar func(recordar bool)) {
	var lienzo fyne.Canvas
	if w != nil {
		lienzo = w.Canvas()
	}
	if lienzo == nil && b.montado != nil {
		lienzo = fyne.CurrentApp().Driver().CanvasForObject(b.montado)
	}
	if lienzo == nil {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	escalon := tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	parrafo := NuevoParrafo(b.t.T("localFile.confirmUpload.body", i18n.Vars{"name": nombre}), escalon, Normal, "text", v)
	casilla := NuevaCasilla(b.t.T("localFile.confirmUpload.remember", nil), nil)
	casilla.Escalon = tema.TextoSM
	cuerpo := container.New(Columna{Hueco: tema.Espacio(3)}, parrafo, casilla)
	var d *Dialogo
	d = Confirmacion(
		b.t.T("localFile.confirmUpload.title", nil),
		b.t.T("ui.cancel", nil),
		b.t.T("localFile.confirmUpload.confirm", nil),
		false,
		cuerpo,
		func() { d.Cerrar() },
		func() {
			recordar := casilla.Marcada
			d.Cerrar()
			aceptar(recordar)
		},
	)
	d.RotuloCerrar = b.t.T("ui.close", nil)
	ancho := AnchoCuerpoDialogo(d, lienzo.Size())
	parrafo.A(ancho)
	casilla.AnchoEtiqueta = max(ancho-LadoEleccion-HuecoEleccion, 1)
	d.Mostrar(lienzo)
}

// ContenidoMontado es lo que hay ahora mismo bajo la barra en lo que devolvió Montar: el
// contenido de la app, o la página que Montar haya puesto en su lugar.
func ContenidoMontado(montado fyne.CanvasObject) fyne.CanvasObject {
	c, es := montado.(*fyne.Container)
	if !es {
		return nil
	}
	m, es := c.Layout.(*montaje)
	if !es {
		return nil
	}
	return c.Objects[m.contenido]
}

// conMenuLateral dice si lo montado bajo la barra lleva menú lateral o carril de iconos:
// es el `:has(nav:is(.bg-sidebar, .bg-rail))` de appearance.css.
func (b *BarraSuite) conMenuLateral() bool {
	if b.montado == nil {
		return false
	}
	c := ContenidoMontado(b.montado)
	if lateralEn(c) != nil {
		return true
	}
	a := armazonEn(c)
	return a != nil && (a.Menu != nil || a.Carril != nil)
}

// ConHojaASangre es para los EDITORES (Docus, Calcus, Diapos y CertiPDF): en la forma
// Marco su hoja va a sangre —sin separación del borde y con solo las esquinas de arriba
// redondeadas—, como `sytue-sheet-bleed` en la carcasa de los editores de la web. Un
// folio o una rejilla que flotan dentro de otra hoja que flota pierden sitio y no ganan
// nada.
func (b *BarraSuite) ConHojaASangre() *BarraSuite {
	b.hojaASangre = true
	return b
}

// ConSinSesionPropia es para las apps que SÍ sirven sin sesión y lo dicen ellas: los
// editores (Docus, Calcus, Diapos y CertiPDF) abren ficheros del equipo, y su portada
// ofrece entrar donde iría la lista de recientes. Las demás no tienen nada que enseñar
// sin sesión, y Montar pone la página «Iniciar sesión» en lugar del contenido.
func (b *BarraSuite) ConSinSesionPropia() *BarraSuite {
	b.sinSesionPropia = true
	return b
}

// montarSinSesion cambia el contenido por la página «Iniciar sesión» mientras no haya
// nadie dentro, y lo devuelve al entrar. Solo cuando CAMBIA el estar dentro: AlCambiar
// también llama por el tema o la presencia, y eso no puede quitar la página «no
// encontrada» que hubiera.
func (b *BarraSuite) montarSinSesion(poner func(fyne.CanvasObject), contenido fyne.CanvasObject) {
	pagina := NuevaPaginaSinSesion(b.t, entrarDesde(b.s))
	dentro := b.s.Estado().Dentro()
	if !dentro {
		poner(pagina)
	}
	b.s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() == dentro {
			return
		}
		dentro = e.Dentro()
		if dentro {
			poner(contenido)
		} else {
			poner(pagina)
		}
	})
}

// Arrancar pide los avisos y se apunta a lo transversal. Sin sesión no pide nada.
//
// La presencia y el tema NO se arrancan aquí: los mantiene `sesion.Suite.Arrancar`,
// que ya llama cada `main`, y esta barra solo se entera por `AlCambiar`.
func (b *BarraSuite) Arrancar() {
	b.avisos.arrancar()
	b.cuenta.arrancar()
}

// CederAtajo le da ⌘K / Ctrl+K a la app: al pulsarlo se llama `f` en vez de llevar
// el foco al buscador de la suite.
//
// Existe por la paleta del ERP. En la web escuchan los dos —el buscador global en
// `document` y el ERP en `window`— y gana el ERP, que abre su paleta y se queda el
// foco. Aquí no se puede registrar el mismo atajo dos veces (el segundo pisa al
// primero, y cuál se engancha antes depende de cuándo se coloque la barra), así que
// la barra lo cede de forma explícita.
func (b *BarraSuite) CederAtajo(f func()) { b.buscador.alAtajo = f }

// CederAtajoSi cede ⌘K solo cuando la app lo quiere: f devuelve si lo atendió y, si no,
// se enfoca el buscador. Es el `defaultPrevented` de la web: Notepads lleva ⌘K a su
// captura con un día abierto, y el segundo ⌘K (con la captura ya enfocada) busca.
func (b *BarraSuite) CederAtajoSi(f func() bool) { b.buscador.alAtajoSi = f }

// Parar suelta el flujo de avisos y la búsqueda que estuviera esperando. La llama
// quien cierre la ventana; sin ella siguen hasta que muere el proceso, que en una
// aplicación de una sola ventana es lo mismo.
func (b *BarraSuite) Parar() {
	b.avisos.parar()
	b.buscador.parar()
}

// AccessibilityLabel: es el `role="banner"` de la web.
func (b *BarraSuite) AccessibilityLabel() string { return b.t.T("shell.apps", nil) }

// AccessibilityRole: la barra agrupa.
func (b *BarraSuite) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaBarraSuite es lo que ocupa la barra: de alto, el de la barra; de ancho, lo
// mínimo para que quepan las cuatro piezas con sus huecos y su canalón.
func MedidaBarraSuite() fyne.Size {
	lado := LadoControlBarra()
	ancho := CanalonBarra + // el canalón estrecho: el mínimo es el del teléfono
		AnchoEsquinaMarca(0, MedidaMarcaBarra(0, false).Width) + HuecoBarra + // sin nombre, que es el mínimo
		MedidaEntrada(CampoMD, 0, true).Width + HuecoBarra +
		lado + HuecoAccionesBarra + 2*MargenDivisorBarra + tema.Borde +
		HuecoAccionesBarra + MargenCuentaBarra + lado
	return fyne.NewSize(ancho, AltoBarraSuite())
}

// CreateRenderer construye el fondo, el filete, la esquina de marca y las tres piezas.
func (b *BarraSuite) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &barraSuiteRenderer{
		b:       b,
		fondo:   canvas.NewRectangle(tema.Color(fondoBarra, v)),
		filete:  canvas.NewRectangle(BordeSobre("border", fondoBarra, v)),
		esquina: canvas.NewRectangle(tema.Color(fondoEsquina, v)),
		divisor: canvas.NewRectangle(tema.Color("chrome-divider", v)),
		marco:   nuevoFondoMarco(),
	}
	// La esquina va DESPUÉS del filete: lo tapa, y así la línea de la barra no cruza el
	// carril (la zona de marca de la web se estira a todo el alto por lo mismo).
	r.objetos = append([]fyne.CanvasObject{r.fondo}, r.marco.objetos()...)
	// La pista del atajo va DESPUÉS del campo: se pinta encima de su borde derecho,
	// que es donde la pone la hoja (`absolute right-3`).
	r.objetos = append(r.objetos, r.filete, r.esquina, r.divisor, b.marca,
		b.buscador.campo, b.buscador.atajo)
	r.objetos = append(r.objetos, b.avisos.objetos()...)
	// El panel de resultados va el ÚLTIMO: cuelga por debajo de la barra y se pinta
	// sobre todo lo demás.
	r.objetos = append(r.objetos, b.cuenta.boton, b.buscador.panel)
	r.Refresh() // regla 7: el campo de búsqueda envuelve un `widget.Entry`
	return r
}

type barraSuiteRenderer struct {
	b                               *BarraSuite
	fondo, filete, esquina, divisor *canvas.Rectangle
	objetos                         []fyne.CanvasObject
	titulo                          tituloPuesto
	// carril: la barra ENTERA es de color. En la forma Marco lo es siempre —la barra y
	// el menú son un solo marco—, y en Carril por debajo de 768, donde no hay menú
	// lateral a la vista: la web la pinta del color de marca, con el texto claro y el
	// buscador en velo oscuro (`.sytue-topbar-carril` de appearance.css).
	carril bool
	// marco es el fondo de la barra en la forma Marco: el mismo color continuo —o el
	// mismo degradado, de lado a lado de la ventana— que pinta el cuerpo, para que casen
	// sin costura.
	marco *fondoMarco
}

// tokenFondo es el que pinta la barra: el suyo, el del carril cuando es estrecha, o el
// del marco.
func (r *barraSuiteRenderer) tokenFondo() string {
	switch {
	case tema.EsMarco():
		return "frame-from"
	case r.carril:
		return fondoEsquina
	}
	return fondoBarra
}

// pintar pone los colores de la barra y de sus piezas según sea clara o carril.
func (r *barraSuiteRenderer) pintar(v tema.Variante) {
	r.fondo.FillColor = tema.Color(r.tokenFondo(), v)
	r.fondo.Hidden = tema.EsMarco()
	r.fondo.Refresh()
	if tema.EsMarco() {
		r.marco.pintar(v)
	} else {
		r.marco.esconder()
	}
	// Sin línea de abajo (`box-shadow: none`) y sin esquina propia: ya va sobre la
	// barra de color, y en el traslúcido se apilarían dos velos.
	r.filete.FillColor = BordeSobre("border", fondoBarra, v)
	r.esquina.FillColor = tema.Color(fondoEsquina, v)
	r.divisor.FillColor = tema.Color("chrome-divider", v)
	// En Marco la cabecera es continua: se conserva el hueco, sin la raya de cuenta.
	r.divisor.Hidden = tema.EsMarco()
	if r.carril {
		r.divisor.FillColor = BordeSobre("sidebar-border", fondoEsquina, v)
	}
	for _, o := range []*canvas.Rectangle{r.filete, r.esquina} {
		o.Hidden = r.carril
		o.Refresh()
	}
	r.divisor.Refresh()
	r.b.avisos.enCarril(r.carril)
	r.b.avisos.repintar(v)
	if c := r.b.cuenta.boton; c.carril != r.carril {
		c.carril = r.carril
		c.Refresh()
	}
	if c := r.b.buscador.campo; c.EnCarril != r.carril {
		c.EnCarril = r.carril
		c.Refresh()
	}
}

func (r *barraSuiteRenderer) MinSize() fyne.Size           { return MedidaBarraSuite() }
func (r *barraSuiteRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *barraSuiteRenderer) Destroy()                     {}

// Layout es el `flex items-center gap-3` de TopBar.tsx: la marca a la izquierda y las
// acciones a la derecha, las dos `shrink-0`, y el buscador CENTRADO en el hueco que
// dejan (`flex min-w-0 flex-1 justify-center` con `max-w-[30rem]` dentro), que encoge
// si no cabe. Hasta el 2026-09-23 el buscador iba pegado a las acciones y salía
// corrido a la derecha.
//
// Las acciones van de fuera hacia dentro, como en la web: el avatar con su `ml-0.5`,
// el filete con su `mx-1` y la campana, con `gap-1` entre ellas.
func (r *barraSuiteRenderer) Layout(t fyne.Size) {
	if carril := t.Width > 0 && (t.Width < CorteEsquinaMarca || tema.EsMarco()); carril != r.carril {
		r.carril = carril
		r.pintar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	lado := LadoControlBarra()
	y := (AltoBarraSuite() - lado) / 2
	r.fondo.Resize(t)
	r.marco.colocar(fyne.NewPos(0, 0), t)
	// `shadow-[inset_0_-1px_0_var(--color-border)]`: la línea va DENTRO del alto y no
	// participa en el reparto. Con un borde de verdad, el contenido queda un píxel
	// alto —medido en las once apps de la web antes de arreglarlo allí—.
	r.filete.Move(fyne.NewPos(0, AltoBarraSuite()-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))

	x := t.Width - CanalonDeBarra(t.Width)

	x -= lado
	r.b.cuenta.boton.Move(fyne.NewPos(x, y))
	r.b.cuenta.boton.Resize(fyne.NewSize(lado, lado))

	x -= MargenCuentaBarra + HuecoAccionesBarra + MargenDivisorBarra + tema.Borde
	r.divisor.Move(fyne.NewPos(x, (AltoBarraSuite()-AltoDivisorBarra)/2))
	r.divisor.Resize(fyne.NewSize(tema.Borde, AltoDivisorBarra))

	x -= MargenDivisorBarra + HuecoAccionesBarra + lado
	r.b.avisos.colocar(fyne.NewPos(x, y), lado)

	// La marca, con su nombre solo desde `sm:` (640 de ventana, con el rem de 16).
	r.b.marca.conNombre = t.Width >= CorteNombreMarca
	m := MedidaMarcaBarra(r.b.marca.producto, r.b.marca.conNombre)
	// `-ml-1`: la marca VISIBLE a un canalón del cristal; el `pl-1` del disparador se sale
	// al canalón. El canalón es SIEMPRE el `px-4` del menú lateral y no el de la barra,
	// que crece con la ventana: la marca cae sobre el botón de la cabecera del menú.
	izquierda := canalonMenu - RellenoMarcaIzq
	r.b.marca.Move(fyne.NewPos(izquierda, y))
	r.b.marca.Resize(fyne.NewSize(m.Width, lado))
	r.b.marca.Refresh()
	// La esquina RESERVA siempre lo mismo y, sin menú lateral debajo, PINTA solo lo que
	// ocupa la marca: el buscador no se mueve al pasar de una app a otra (las dos cajas
	// de TopBar.tsx).
	reserva := AnchoEsquinaMarca(t.Width, m.Width)
	pintada := reserva
	if !r.b.conMenuLateral() {
		pintada = esquinaCenida(m.Width)
	}
	r.esquina.Resize(fyne.NewSize(pintada, AltoBarraSuite()))

	// El buscador, centrado en lo que queda entre las dos, hasta su ancho canónico.
	desde, hasta := reserva+HuecoBarra, x-HuecoBarra
	hueco := max(hasta-desde, 0)
	ancho := min(AnchoBuscadorBarra, hueco)
	r.b.buscador.colocar(desde+(hueco-ancho)/2, (AltoBarraSuite()-AltoCampo(CampoMD))/2, ancho, t)
	r.b.atenderBusqueda()
	r.pintarTitulo()
}

func (r *barraSuiteRenderer) Refresh() {
	r.pintar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.Layout(r.b.Size())
	// Si ha cambiado la apariencia —el tema, el color o la forma—, el montaje tiene que
	// volver a colocar el marco y la hoja: es un contenedor, y a un contenedor Fyne no
	// le avisa de un cambio de tema. Se apunta ANTES de refrescarlo, que su refresco
	// vuelve a pasar por aquí.
	if ahora := aparienciaDeAhora(); r.b.montado != nil && ahora != r.b.pintada {
		r.b.pintada = ahora
		r.b.montado.Refresh()
	}
}

// montaje es el reparto de `Montar`: la barra ocupa su alto arriba y el contenido se
// lleva el resto. Los objetos van en orden de PINTADO y no en orden visual: el marco y
// el papel de la hoja, el contenido, las máscaras y la sombra de la hoja, y la barra.
type montaje struct {
	marco     *marco
	superior  *canvas.Rectangle    // el área segura sobre la barra de Carril
	laterales [2]*canvas.Rectangle // prolongación del marco en los bordes seguros del teléfono
	contenido int                  // dónde va el contenido en los objetos
	aSangre   bool
}

var _ fyne.Layout = (*montaje)(nil)

func (m *montaje) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) <= m.contenido {
		return
	}
	contenido, barra := objs[m.contenido], objs[len(objs)-1]
	alto := barra.MinSize().Height
	barra.Move(fyne.NewPos(0, 0))
	barra.Resize(fyne.NewSize(t.Width, alto))
	pos, tam := fyne.NewPos(0, alto), fyne.NewSize(t.Width, max(t.Height-alto, 0))
	// En Carril no hay marco. Con armazón la hoja es suya, pero el montaje sigue
	// pintando el fondo exterior: iOS deja las áreas seguras fuera del contenido.
	if !tema.EsMarco() || armazonEn(contenido) != nil {
		m.marco.esconder()
		m.colocarFondoSeguro(barra, t)
		contenido.Move(pos)
		contenido.Resize(tam)
		return
	}
	// Sin menú a su izquierda, la hoja se separa de los tres bordes (el de arriba es la
	// barra, que es parte del marco). A sangre —los editores, y cualquiera por debajo
	// de 768— ocupa todo y solo redondea arriba.
	aSangre := m.aSangre || t.Width < Estrecho
	hojaPos, hojaTam := pos, tam
	if !aSangre {
		separa := tema.Forma("sheet-gap")
		hojaPos.X += separa
		hojaTam = fyne.NewSize(max(tam.Width-2*separa, 0), max(tam.Height-separa, 0))
	}
	m.marco.colocar(pos, tam, hojaPos, hojaTam, aSangre, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	m.colocarFondoSeguro(barra, t)
	contenido.Move(hojaPos)
	contenido.Resize(hojaTam)
}

func (m *montaje) colocarFondoSeguro(barra fyne.CanvasObject, t fyne.Size) {
	pos, total := fyne.NewPos(0, 0), t
	if c := lienzoDe(barra); EsTactil() && c != nil {
		// Instala el retorno compartido también cuando no se abrió aún ningún
		// diálogo o cajón y el driver entregaría Atrás directamente al sistema.
		asegurarTeclas(c)
		pos, _ = c.InteractiveArea()
		total = c.Size()
	}
	m.colocarFondo(t, pos, total)
}

// colocarFondo prolonga solo la pintura fuera del área segura, sin mover ningún
// control. El degradado conserva el ancho de las barras; los márgenes laterales
// prolongan sus extremos para que también encaje al girar el teléfono.
func (m *montaje) colocarFondo(t fyne.Size, segura fyne.Position, total fyne.Size) {
	m.superior.Hide()
	for _, o := range m.laterales {
		o.Hide()
	}
	if !tema.EsMarco() {
		m.marco.fondo.esconder()
		m.superior.FillColor = tema.Color(fondoEsquina, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		m.superior.Move(fyne.NewPos(0, -segura.Y))
		m.superior.Resize(fyne.NewSize(t.Width, segura.Y))
		esconder(m.superior, segura.Y <= 0)
		m.superior.Refresh()
		return
	}
	f := m.marco.fondo
	f.pintar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	f.colocar(fyne.NewPos(0, -segura.Y), fyne.NewSize(t.Width, total.Height))
	for i, lado := range []struct {
		x, ancho float32
		color    color.NRGBA
	}{{-segura.X, segura.X, f.desde}, {t.Width, max(total.Width-segura.X-t.Width, 0), f.hasta}} {
		o := m.laterales[i]
		o.FillColor = lado.color
		o.Move(fyne.NewPos(lado.x, -segura.Y))
		o.Resize(fyne.NewSize(lado.ancho, total.Height))
		esconder(o, lado.ancho <= 0)
		o.Refresh()
	}
}

func (m *montaje) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) <= m.contenido {
		return fyne.NewSize(0, 0)
	}
	c, b := objs[m.contenido].MinSize(), objs[len(objs)-1].MinSize()
	return fyne.NewSize(max(c.Width, b.Width), c.Height+b.Height)
}

// ---- el botón del avatar ----

// botonAvatar es el disparador del menú de cuenta: el avatar dentro de un botón del
// cromo. No es un `BotonIcono` porque lo que lleva dentro no es un glifo del catálogo
// sino un `Avatar`, con sus iniciales y su punto de presencia.
type botonAvatar struct {
	pulsable

	avatar *Avatar
	// estaAbierto dice si el menú que cuelga del botón está puesto, para teñirlo
	// mientras tanto (`aria-expanded ? 'bg-chrome-hover' : ''`).
	estaAbierto func() bool
	// carril: el botón va sobre la barra de color (la estrecha) y su velo y su foco son
	// los del carril. Lo pone la barra.
	carril bool
}

var (
	_ fyne.Widget       = (*botonAvatar)(nil)
	_ fyne.Tappable     = (*botonAvatar)(nil)
	_ fyne.Focusable    = (*botonAvatar)(nil)
	_ desktop.Hoverable = (*botonAvatar)(nil)
)

func nuevoBotonAvatar(nombre, rotulo string, alPulsar func()) *botonAvatar {
	// `xs` (24) y no `sm` (32): el botón mide 31,5, así que el avatar grande lo
	// desbordaba medio píxel. Lo dice AccountMenu.tsx con su medida.
	b := &botonAvatar{avatar: NuevoAvatar(nombre).Con(AvatarXS)}
	b.Nombre = rotulo
	b.AlPulsar = alPulsar
	b.ExtendBaseWidget(b)
	return b
}

func (b *botonAvatar) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	lado := LadoControlBarra()
	return fyne.NewSize(lado, lado)
}

func (b *botonAvatar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &botonAvatarRenderer{
		b:    b,
		aro:  NuevoAroCon("chrome-focus", tema.RadioMD, SinHueco, v).Abierto(), // transparente en reposo
		caja: NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioMD),
		// El aro del disco sobre la barra de color (`sytue-aro-carril` en AccountMenu.tsx).
		anillo: canvas.NewRectangle(color.NRGBA{}),
	}
	r.objetos = append(r.aro.Antes(r.caja.Objetos()...), r.anillo, b.avatar)
	r.Refresh()
	return r
}

type botonAvatarRenderer struct {
	b       *botonAvatar
	aro     *Aro
	caja    *Caja
	anillo  *canvas.Rectangle
	objetos []fyne.CanvasObject
}

func (r *botonAvatarRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *botonAvatarRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *botonAvatarRenderer) Destroy()                     {}

func (r *botonAvatarRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	a := MedidaAvatar(AvatarXS)
	r.b.avatar.Move(fyne.NewPos((t.Width-a.Width)/2, (t.Height-a.Height)/2))
	r.b.avatar.Resize(a)
	r.anillo.Move(r.b.avatar.Position().SubtractXY(AroCarril, AroCarril))
	r.anillo.Resize(a.AddWidthHeight(2*AroCarril, 2*AroCarril))
	r.anillo.CornerRadius = a.Height/2 + AroCarril
}

func (r *botonAvatarRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	velo, aro := "chrome-hover", "chrome-focus"
	if r.b.carril {
		velo, aro = "sidebar-hover-bg", "sidebar-focus"
	}
	// `rounded-control`, como los demás botones de la barra: de la forma.
	r.caja.CambiarRadio(tema.Forma("radius-control"))
	r.aro.CambiarRadio(tema.Forma("radius-control"))
	fondo := color.NRGBA{}
	if r.b.Encima() || (r.b.estaAbierto != nil && r.b.estaAbierto()) {
		fondo = tema.Color(velo, v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.aro.CambiarToken(aro)
	r.aro.Pintar(v)
	r.anillo.FillColor = tema.Color("sidebar-fg", v)
	esconder(r.anillo, !r.b.carril)
	r.anillo.Refresh()
	r.aro.Ver(r.b.Enfocado())
	r.b.avatar.Refresh()
	r.Layout(r.b.Size())
}

// ---- la marca de la app ----

// Medidas de la marca: el disparador del selector de aplicaciones de la web
// (AppSwitcher.tsx) SIN el selector. `pl-1`, la marca de 28, `gap-1.5`, el nombre en
// `text-sm font-semibold` y `pr-1.5`. El galón no está porque no hay nada que abrir.
const (
	RellenoMarcaIzq = 1 * 0.25 * tema.Rem   // pl-1
	RellenoMarcaDer = 1.5 * 0.25 * tema.Rem // pr-1.5
	HuecoMarca      = 1.5 * 0.25 * tema.Rem // gap-1.5
	// CorteNombreMarca es el `max-sm:hidden` del nombre: `sm:` mide la VENTANA, y con
	// el rem de 16 de las consultas de medio son 640 px.
	CorteNombreMarca float32 = 640
)

// MedidaMarcaBarra es lo que ocupa la marca, con su nombre o sin él. De alto, la fila
// de los controles.
func MedidaMarcaBarra(p iconos.Producto, conNombre bool) fyne.Size {
	ancho := RellenoMarcaIzq + iconos.MarcaMD + RellenoMarcaDer
	if conNombre {
		ancho += HuecoMarca + Medir(p.NombreCorto(), tema.TextoSM, SemiNegrita).Width
	}
	return fyne.NewSize(ancho, LadoControlBarra())
}

// marcaBarra es la marca del producto y su nombre corto («Mail»), sin «Sytue» delante
// porque ya lo dice la marca de al lado. No se pulsa: es identidad, no navegación.
type marcaBarra struct {
	widget.BaseWidget
	producto  iconos.Producto
	conNombre bool
}

var (
	_ fyne.Widget     = (*marcaBarra)(nil)
	_ fyne.Accessible = (*marcaBarra)(nil)
)

func nuevaMarcaBarra(p iconos.Producto) *marcaBarra {
	m := &marcaBarra{producto: p, conNombre: true}
	m.ExtendBaseWidget(m)
	return m
}

// AccessibilityLabel es la marca con su prefijo, «Sytue Mail», que es lo que se lee
// aunque el nombre no quepa a la vista.
func (m *marcaBarra) AccessibilityLabel() string { return m.producto.Nombre() }

// AccessibilityRole: se lee, no se pulsa.
func (m *marcaBarra) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (m *marcaBarra) MinSize() fyne.Size {
	m.ExtendBaseWidget(m)
	return MedidaMarcaBarra(m.producto, m.conNombre)
}

func (m *marcaBarra) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &marcaBarraRenderer{
		m:      m,
		placa:  canvas.NewRectangle(color.NRGBA{}),
		dibujo: iconos.Marca(m.producto, iconos.MarcaMD, v),
		nombre: Texto(m.producto.NombreCorto(), tema.TextoSM, SemiNegrita, tema.Color("sidebar-fg", v)),
	}
	r.placa.CornerRadius = tema.RadioMD
	r.Refresh()
	return r
}

// RebordePlacaMarca es el `p-0.5` de la placa blanca de la marca.
const RebordePlacaMarca = 0.5 * 0.25 * tema.Rem

type marcaBarraRenderer struct {
	m *marcaBarra
	// placa es la placa BLANCA redondeada sobre la que va la tesela del producto cuando
	// el color de la suite es «el de cada app»: el carril es entonces del color de la
	// propia tesela —la de Mail, roja sobre rojo— y sin placa se funde con él. Es lo que
	// la web hace con el logo de una organización (`rounded-md bg-white p-0.5`).
	placa  *canvas.Rectangle
	dibujo *iconos.Dibujo
	nombre *canvas.Text
}

func (r *marcaBarraRenderer) MinSize() fyne.Size { return r.m.MinSize() }
func (r *marcaBarraRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.placa, r.dibujo.Objeto(), r.nombre}
}
func (r *marcaBarraRenderer) Destroy() {}

// Layout centra la marca y el nombre en la fila (`items-center`).
func (r *marcaBarraRenderer) Layout(t fyne.Size) {
	lado := iconos.MarcaMD + 2*RebordePlacaMarca
	r.placa.Move(fyne.NewPos(RellenoMarcaIzq-RebordePlacaMarca, (t.Height-lado)/2))
	r.placa.Resize(fyne.NewSize(lado, lado))
	r.dibujo.Colocar(RellenoMarcaIzq, (t.Height-iconos.MarcaMD)/2, iconos.MarcaMD)
	ColocarTexto(r.nombre, tema.TextoSM, RellenoMarcaIzq+iconos.MarcaMD+HuecoMarca,
		(t.Height-tema.TextoSM.Linea)/2)
}

// Refresh sigue al tema: el anillo de la tesela solo se enciende en oscuro.
func (r *marcaBarraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.dibujo.TenirCon(tema.Color("product-tile-ring", v))
	r.placa.FillColor = tema.Color("neutral-0", v)
	r.placa.Hidden = tema.ColorElegido() != tokens.ColorDeCadaApp
	r.placa.Refresh()
	// Sobre la esquina de marca, que es carril: `text-sidebar-fg`.
	r.nombre.Color = tema.Color("sidebar-fg", v)
	r.nombre.Hidden = !r.m.conNombre
	r.nombre.Refresh()
	r.Layout(r.m.Size())
}
