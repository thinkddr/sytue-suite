// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los guardianes de este paquete corren en Carril (ver `tema.FormaDeLosGuardianes`): sus
// medidas se tomaron con esa forma. Los de este fichero son los de MARCO, la forma por
// defecto, y la eligen ellos.
func enMarco(t *testing.T) {
	t.Helper()
	tema.ElegirForma("marco")
	t.Cleanup(tema.FormaDeLosGuardianes)
}

// Las medidas de geometría usan un fondo uniforme conocido. Una sesión abierta
// por otro test puede haber aplicado «el de cada app», ahora predeterminado, y
// montar otra barra cambia entonces el color de producto durante la prueba.
func conPetroleoClaro(t *testing.T) {
	t.Helper()
	tema.Elegir("claro")
	tema.ElegirColor("petroleo")
	tema.ElegirApp("")
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirApp("") })
}

// contenidoDePrueba es un rectángulo opaco de un color que no es de la hoja: lo que una
// app pinta de lado a lado, esquinas incluidas.
var magenta = color.NRGBA{R: 255, B: 255, A: 255}

func contenidoDePrueba() *canvas.Rectangle { return canvas.NewRectangle(magenta) }

// mira compara un píxel, en px de CSS, con un color.
func mira(t *testing.T, img image.Image, que string, x, y float32, quiero color.NRGBA) {
	t.Helper()
	r, g, b, a := img.At(int(x*escala), int(y*escala)).RGBA()
	got := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	// Con holgura: junto a la hoja el marco lleva encima su sombra (`shadow-md`), que lo
	// oscurece unos pocos niveles. Lo que se distingue aquí —marco, papel, contenido—
	// está a cientos.
	if !cercaDe(got, quiero, 8) {
		t.Errorf("%s: en (%v, %v) pinta %v y quiero %v", que, x, y, got, quiero)
	}
}

// TestLaHojaFlotaEnElMarco es la forma Marco de una app con menú: el marco de color
// continuo detrás del menú y, a su derecha, la HOJA —separada del borde derecho y del
// de abajo, con las cuatro esquinas redondeadas— en la que va el contenido. El
// contenido es un rectángulo que pinta hasta sus esquinas: el arco lo dan las máscaras.
//
// VISTO EN ROJO sin las máscaras en los objetos del armazón: la esquina de la hoja sale
// del color del contenido.
func TestLaHojaFlotaEnElMarco(t *testing.T) {
	conPetroleoClaro(t)
	enMarco(t)
	const ancho, alto = 1000, 600
	a := NuevoArmazon(contenidoDePrueba())
	a.Menu = NuevoLateral("Menú", nil)
	a.Resize(fyne.NewSize(ancho, alto))
	_ = test.WidgetRenderer(a)
	a.Refresh()
	img := pinta(t, ancho, alto, a)

	marco := tema.Color("frame-from", tema.Claro)
	separa, radio := tema.Forma("sheet-gap"), tema.Forma("radius-sheet")
	if separa != 10 || radio != 16 {
		t.Fatalf("la hoja se separa %v y redondea %v; la tabla de theme.css dice 10 y 16", separa, radio)
	}
	x0, x1, y1 := tema.AnchoLateral, ancho-separa, alto-separa
	mira(t, img, "el menú deja ver el marco", 5, 300, marco)
	mira(t, img, "el centro de la hoja es el contenido", (x0+x1)/2, y1/2, magenta)
	mira(t, img, "a la derecha de la hoja, el marco", x1+separa/2, 300, marco)
	mira(t, img, "debajo de la hoja, el marco", (x0+x1)/2, y1+separa/2, marco)
	// Las cuatro esquinas: a un píxel del vértice es marco; pasado el arco, contenido.
	for que, p := range map[string]fyne.Position{
		"arriba a la izquierda": {X: x0 + 1, Y: 1}, "arriba a la derecha": {X: x1 - 2, Y: 1},
		"abajo a la izquierda": {X: x0 + 1, Y: y1 - 2}, "abajo a la derecha": {X: x1 - 2, Y: y1 - 2},
	} {
		mira(t, img, "la esquina de "+que+" es del marco", p.X, p.Y, marco)
	}
	mira(t, img, "pasado el arco, la hoja", x0+radio, radio, magenta)
	if l := lateralEn(a.Menu); l == nil || !l.SinFondo {
		t.Error("en Marco el menú tiene que dejar ver el marco, que ya es de su color")
	}

	// En estrecho va a sangre: sin separación y con las cuatro esquinas. Y el
	// menú, que es un cajón, conserva su fondo.
	a.Resize(fyne.NewSize(600, alto))
	a.Refresh()
	img = pinta(t, 600, alto, a)
	// Bajo la barra móvil, que prolonga el marco y deja la hoja debajo.
	mira(t, img, "a sangre, el borde derecho es contenido", 598, 300, magenta)
	mira(t, img, "a sangre, el borde de abajo es contenido", 300, alto-2, magenta)
	mira(t, img, "a sangre, la esquina de abajo es del marco", AnchoCarrilMovil+1, alto-2, marco)
	mira(t, img, "a sangre, arriba sí", 1, 1, marco)
	if l := lateralEn(a.Menu); !l.SinFondo {
		t.Error("el menú móvil desplazado tiene que prolongar el color del marco")
	}

	// En Carril no hay nada de esto: el contenido llega a la esquina.
	tema.ElegirForma("carril")
	a.Resize(fyne.NewSize(ancho, alto))
	a.Refresh()
	img = pinta(t, ancho, alto, a)
	mira(t, img, "en Carril el contenido llega a su esquina", x0+1, 1, magenta)
	mira(t, img, "en Carril el contenido llega al borde derecho", ancho-2, 300, magenta)
	mira(t, img, "en Carril la esquina inferior sigue siendo contenido", x0+1, alto-2, magenta)
}

// La navegación móvil pertenece al marco de color. Se mira la unión real entre
// barra, navegación y hoja: ocultar solo el filete dejaba toda una banda blanca.
func TestElMarcoMovilProlongaLaBarraHastaLaHoja(t *testing.T) {
	enMarco(t)
	tactilAntes := tactilDePrueba
	tactil := true
	tactilDePrueba = &tactil
	t.Cleanup(func() {
		tactilDePrueba = tactilAntes
		tema.Elegir("")
		tema.ElegirColor("")
		tema.ElegirApp("")
	})
	for _, variante := range []string{"claro", "oscuro", "traslucido"} {
		tema.Elegir(variante)
		for _, colorSuite := range []string{"petroleo", "cobalto", "app"} {
			tema.ElegirColor(colorSuite)
			a := NuevoArmazon(contenidoDePrueba())
			a.Menu, a.Titulo = NuevoLateral("Menú", nil), "Bandeja"
			b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
			montado := b.Montar(a)
			montado.Resize(fyne.NewSize(390, 360))
			r := test.WidgetRenderer(a).(*armazonRenderer)
			nav := test.WidgetRenderer(a.barra).(*barraMovilRenderer)
			if r.marco.hoja.Position() != fyne.NewPos(AnchoCarrilMovil, 0) || nav.fondo.Visible() {
				t.Fatalf("%s/%s: la hoja empieza en %v y el fondo opaco de navegación es visible=%v",
					variante, colorSuite, r.marco.hoja.Position(), nav.fondo.Visible())
			}
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			_, tinta := nav.boton.colores(v)
			if tinta != tema.Color("sidebar-fg", v) || nav.boton.TokenAro != "sidebar-focus" {
				t.Error("el botón de navegación no usa la tinta y el foco del marco")
			}
			for _, esc := range []float32{1, 2, 3} {
				img := pintaA(t, esc, 390, 360, montado)
				for _, x := range []float32{2, AnchoCarrilMovil / 2, AnchoCarrilMovil - 2} {
					quiero := r.marco.fondo.colorEn(x, 390)
					for _, y := range []float32{AltoBarraSuite() - 1, AltoBarraSuite(), AltoBarraSuite() + 1, AltoBarraSuite() + MinCabeceraApp + 20, 300} {
						got := px(img, int(x*esc), int(y*esc))
						if !cercaDe(got, quiero, 3) {
							t.Errorf("%s/%s ×%g: banda en (%g,%g): %v, quiero marco %v", variante, colorSuite, esc, x, y, got, quiero)
						}
					}
				}
				// La hoja empieza junto al mando, a la altura de la cabecera.
				x, y := AnchoCarrilMovil+1, AltoBarraSuite()+1
				if got := px(img, int(x*esc), int(y*esc)); !cercaDe(got, r.marco.fondo.colorEn(x, 390), 3) {
					t.Errorf("la esquina de la hoja no está junto al carril: %v", got)
				}
			}
			b.Parar()
		}
	}
}

// El lienzo móvil reserva las áreas seguras incluso con SetPadded(false). Solo
// extendemos el fondo; los controles conservan su posición y tamaño interactivos.
func TestElMarcoCubreLasAreasSegurasSinEstirarElDegradado(t *testing.T) {
	enMarco(t)
	tema.Elegir("traslucido")
	t.Cleanup(func() { tema.Elegir("") })
	for _, area := range []struct {
		pos          fyne.Position
		derecha, pie float32
	}{{fyne.NewPos(0, 24), 0, 34}, {fyne.NewPos(44, 0), 44, 34}, {fyne.NewPos(44, 0), 0, 16}, {fyne.NewPos(0, 0), 44, 16}} {
		segura := area.pos
		total := fyne.NewSize(480, 380)
		tam := fyne.NewSize(total.Width-segura.X-area.derecha, total.Height-segura.Y-area.pie)
		a := NuevoArmazon(contenidoDePrueba())
		a.Menu = NuevoLateral("Menú", nil)
		b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
		montado := b.Montar(a).(*fyne.Container)
		montado.Resize(tam)
		montado.Move(segura)
		m := montado.Layout.(*montaje)
		posBarra, tamBarra := b.Position(), b.Size()
		m.colocarFondo(tam, segura, total)
		if b.Position() != posBarra || b.Size() != tamBarra {
			t.Fatal("extender el fondo ha movido los controles")
		}
		for _, esc := range []float32{2, 3} {
			img := pintaA(t, esc, total.Width, total.Height, montado)
			for _, p := range []fyne.Position{{X: 1, Y: 1}, {X: 240, Y: 1}, {X: 478, Y: 1}, {X: 1, Y: 378}, {X: 240, Y: 378}, {X: 478, Y: 378}} {
				quiero := m.marco.fondo.colorEn(p.X-segura.X, tam.Width)
				if got := px(img, int(p.X*esc), int(p.Y*esc)); !cercaDe(got, quiero, 3) {
					t.Errorf("área segura %v ×%g en %v: %v, quiero %v", segura, esc, p, got, quiero)
				}
			}
		}
		tema.ElegirForma("carril")
		montado.Refresh()
		if m.marco.fondo.degradado.Visible() || m.laterales[0].Visible() || m.laterales[1].Visible() {
			t.Error("al elegir Carril sigue pintado el marco exterior")
		}
		tema.ElegirForma("marco")
		b.Parar()
	}
}

func TestCarrilProlongaLaBarraSobreElAreaSegura(t *testing.T) {
	tema.ElegirForma("carril")
	t.Cleanup(tema.FormaDeLosGuardianes)
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
	t.Cleanup(b.Parar)
	montado := b.Montar(contenidoDePrueba()).(*fyne.Container)
	segura, total := fyne.NewPos(0, 24), fyne.NewSize(390, 380)
	montado.Resize(fyne.NewSize(390, 322))
	montado.Move(segura)
	m := montado.Layout.(*montaje)
	m.colocarFondo(montado.Size(), segura, total)
	img := pinta(t, total.Width, total.Height, montado)
	if techo, barra := px(img, 2, 2), px(img, 2, int((segura.Y+1)*escala)); !cercaDe(techo, barra, 2) {
		t.Errorf("el área segura de Carril pinta %v y la barra %v", techo, barra)
	}
}

func TestElArmazonNoDaAlturasNegativasConTecladoAlGirar(t *testing.T) {
	a := NuevoArmazon(contenidoDePrueba())
	a.Panel = contenidoDePrueba()
	a.Resize(fyne.NewSize(700, 20))
	r := test.WidgetRenderer(a).(*armazonRenderer)
	r.Layout(a.Size())
	for _, o := range []fyne.CanvasObject{a.Panel, a.Contenido} {
		if o.Size().Height < 0 || o.Size().Width < 0 {
			t.Errorf("con teclado y poca altura el armazón asigna un tamaño negativo: %v", o.Size())
		}
	}
}

// TestElArmazonConCarrilDeIconos: el ERP lleva en la web un carril de iconos y el menú en
// un cajón también en ancho (`.erp-rail` y `#erp-cajon` de ErpApp.tsx). El carril va a
// la izquierda, sobre el marco; la hoja empieza donde él acaba y flota como la de todos;
// el menú no ocupa sitio: sale encima con su velo, sin mover el contenido, y no se cierra
// solo en ancho. Por debajo de 768 el carril no se pinta y la hoja va a sangre.
//
// VISTO EN ROJO sin el ramo de `Carril` en Layout: el contenido empieza en 260, a la
// derecha de un menú que no tendría que verse.
func TestElArmazonConCarrilDeIconos(t *testing.T) {
	enMarco(t)
	const ancho, alto, anchoCarril = 1000, 600, 72
	antes := SinMovimiento
	SinMovimiento = true // el cajón, ya en su sitio
	t.Cleanup(func() { SinMovimiento = antes })
	cian := color.NRGBA{G: 255, B: 255, A: 255}
	carril, menu, contenido := canvas.NewRectangle(color.NRGBA{}), canvas.NewRectangle(cian), contenidoDePrueba()
	carril.SetMinSize(fyne.NewSize(anchoCarril, 0))
	menu.SetMinSize(fyne.NewSize(tema.AnchoLateral, 0))
	a := NuevoArmazon(contenido)
	a.Carril, a.Menu = carril, menu
	a.SinBarraMovil = true // como el ERP: el cajón lo abre su propio botón
	a.Resize(fyne.NewSize(ancho, alto))
	_ = test.WidgetRenderer(a)
	a.Refresh()

	marco := tema.Color("frame-from", tema.Claro)
	separa := tema.Forma("sheet-gap")
	hoja := fyne.NewSize(ancho-anchoCarril-separa, alto-separa)
	if contenido.Position() != fyne.NewPos(anchoCarril, 0) || contenido.Size() != hoja {
		t.Errorf("la hoja está en %v y mide %v; empieza donde acaba el carril, (%v, 0), y mide %v", contenido.Position(), contenido.Size(), anchoCarril, hoja)
	}
	if !carril.Visible() || carril.Size() != fyne.NewSize(anchoCarril, alto) || menu.Visible() {
		t.Errorf("en ancho se ve el carril (%v, %v) y el menú, que es un cajón, no (%v)", carril.Visible(), carril.Size(), menu.Visible())
	}
	img := pinta(t, ancho, alto, a)
	mira(t, img, "el carril deja ver el marco", anchoCarril/2, 300, marco)
	mira(t, img, "la esquina de la hoja es del marco", anchoCarril+1, 1, marco)
	mira(t, img, "el centro de la hoja es el contenido", 500, 300, magenta)

	a.AbrirCajon()
	if !a.CajonAbierto || !menu.Visible() || menu.Position() != fyne.NewPos(0, 0) || menu.Size() != fyne.NewSize(tema.AnchoLateral, alto) {
		t.Errorf("el cajón abierto está en %v y mide %v (visible %v): sale desde el borde, sobre el carril", menu.Position(), menu.Size(), menu.Visible())
	}
	if contenido.Position() != fyne.NewPos(anchoCarril, 0) || contenido.Size() != hoja {
		t.Errorf("abrir el cajón ha movido la hoja a %v (%v)", contenido.Position(), contenido.Size())
	}
	a.Resize(fyne.NewSize(ancho+100, alto))
	if !a.CajonAbierto {
		t.Error("con carril el cajón no se cierra solo en ancho: es su menú también ahí")
	}
	a.CerrarCajon()

	a.Resize(fyne.NewSize(360, alto))
	a.Refresh()
	if carril.Visible() || contenido.Position() != fyne.NewPos(AnchoCarrilMovil, 0) || contenido.Size() != fyne.NewSize(360-AnchoCarrilMovil, alto) {
		t.Errorf("a 360 el carril de escritorio se ve (%v) y la hoja está en %v con %v; debe dejar sitio al mando móvil", carril.Visible(), contenido.Position(), contenido.Size())
	}
}

// TestMontarPintaLaHojaDeLoQueNoTraeArmazon: el Hub, los editores y la página «Iniciar
// sesión» no tienen armazón, así que la hoja se la pone Montar: separada de los tres
// bordes (sin menú a su izquierda, también de ese) o a sangre, que es como la piden los
// editores. Con armazón no la pinta: es suya.
//
// VISTO EN ROJO con `armazonEn` devolviendo siempre nil: con armazón el contenido
// arranca a 10 px del borde y salen dos hojas.
func TestMontarPintaLaHojaDeLoQueNoTraeArmazon(t *testing.T) {
	conPetroleoClaro(t)
	enMarco(t)
	const ancho, alto = 1000, 500
	marco := tema.Color("frame-from", tema.Claro)
	monta := func(b *BarraSuite, contenido fyne.CanvasObject) (fyne.CanvasObject, float32) {
		t.Cleanup(b.Parar)
		m := b.Montar(contenido)
		m.Resize(fyne.NewSize(ancho, alto))
		return m, AltoBarraSuite()
	}

	// Sin armazón: la hoja, con su separación.
	contenido := contenidoDePrueba()
	m, barra := monta(NuevaBarraSuite(traductorPrueba(t), nil, iconos.One), contenido)
	img := pinta(t, ancho, alto, m)
	if got := contenido.Position(); got != fyne.NewPos(10, barra) {
		t.Errorf("sin menú la hoja empieza en %v y se separa 10 del borde izquierdo: (10, %v)", got, barra)
	}
	if got := contenido.Size(); got != fyne.NewSize(ancho-20, alto-barra-10) {
		t.Errorf("la hoja mide %v y quiero %v", got, fyne.NewSize(ancho-20, alto-barra-10))
	}
	mira(t, img, "a la izquierda de la hoja, el marco", 5, 300, marco)
	mira(t, img, "la esquina de la hoja es del marco", 11, barra+1, marco)
	mira(t, img, "dentro, el contenido", 500, 300, magenta)

	// A sangre, como los editores: todo el sitio, con las cuatro esquinas.
	sangre := contenidoDePrueba()
	m, barra = monta(NuevaBarraSuite(traductorPrueba(t), nil, iconos.Docs).ConHojaASangre(), sangre)
	img = pinta(t, ancho, alto, m)
	if got := sangre.Position(); got != fyne.NewPos(0, barra) || sangre.Size() != fyne.NewSize(ancho, alto-barra) {
		t.Errorf("a sangre la hoja ocupa %v %v y quiero todo lo de debajo de la barra", got, sangre.Size())
	}
	mira(t, img, "a sangre, la esquina de arriba es del marco", 1, barra+1, marco)
	mira(t, img, "a sangre, la esquina de abajo es del marco", 1, alto-2, marco)

	// Con armazón, la hoja es suya: Montar le da todo el sitio y no pinta nada.
	arm := NuevoArmazon(contenidoDePrueba())
	arm.Menu = NuevoLateral("Menú", nil)
	m, barra = monta(NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail), container.NewStack(arm))
	pinta(t, ancho, alto, m)
	if got := arm.Position(); got.X != 0 || arm.Size().Width != ancho {
		t.Errorf("con armazón el contenido va de %v con %v de ancho y quiero de lado a lado", got, arm.Size().Width)
	}
	if got := arm.Contenido.Position().X; got != tema.AnchoLateral {
		t.Errorf("la hoja del armazón empieza en x=%v y quiero donde acaba el menú, %v", got, tema.AnchoLateral)
	}
	_ = barra
}

// TestLaBarraEsParteDelMarco: en Marco la barra superior es de color de lado a lado, a
// cualquier ancho —la barra y el menú son un solo marco—, sin línea de abajo y con el
// buscador en velo oscuro. Es la misma barra entera de color que Carril lleva por debajo
// de 768.
//
// VISTO EN ROJO sin `tema.EsMarco()` en el corte de `Layout`: a 1440 la barra pinta
// `topbar-bg` fuera de la esquina de marca.
func TestLaBarraEsParteDelMarco(t *testing.T) {
	enMarco(t)
	b := barraPrueba(t)
	img := pinta(t, anchoPrueba, 120, b)
	marco := tema.Color("frame-from", tema.Claro)
	mira(t, img, "la barra, fuera de la esquina de marca", tema.AnchoLateral+4, 4, marco)
	mira(t, img, "la barra, junto al borde derecho", anchoPrueba-3, 4, marco)
	mira(t, img, "la barra no lleva línea de abajo", tema.AnchoLateral+4, AltoBarraSuite()-0.5, marco)
	if !b.buscador.campo.EnCarril {
		t.Error("el buscador de la barra de Marco va en velo, como el de la barra estrecha")
	}
	// Los botones de la barra son `rounded-control`: 4 px también en Marco.
	if got, quiero := b.avisos.boton.Radio, tema.Forma("radius-control"); got != quiero || quiero != 4 {
		t.Errorf("la campana redondea %v y `rounded-control` en Marco es %v (4)", got, quiero)
	}
}

// TestLasCabecerasDeMarcoNoLlevanFiletes mide las dos rayas decorativas que se
// retiraron de Marco: el divisor de la cuenta y el filete de la barra móvil. En Carril
// siguen pintadas. Se cambia la forma en caliente, sin mover los controles, y se
// compara el píxel de la raya con el fondo contiguo a varias escalas de dispositivo.
func TestLasCabecerasDeMarcoNoLlevanFiletes(t *testing.T) {
	tactilAntes := tactilDePrueba
	t.Cleanup(func() {
		tactilDePrueba = tactilAntes
		tema.Elegir("")
		tema.FormaDeLosGuardianes()
	})
	for _, variante := range []string{"claro", "traslucido", "oscuro"} {
		tema.Elegir(variante)
		for _, ancho := range []float32{390, 1280} {
			tactil := ancho == 390
			tactilDePrueba = &tactil
			b := barraPruebaA(t, ancho)
			r := test.WidgetRenderer(b).(*barraSuiteRenderer)
			movil := NuevaBarraMovil()
			movil.AlMenu = func() {}
			movil.Move(fyne.NewPos(0, AltoBarraSuite()))
			movil.Resize(fyne.NewSize(ancho, MedidaBarraMovil().Height))
			antesPos, antesTam := r.divisor.Position(), r.divisor.Size()
			for _, esc := range []float32{1.25, 2, 3} {
				for _, forma := range []string{"marco", "carril", "marco"} {
					tema.ElegirForma(forma)
					b.Refresh()
					movil.Refresh()
					img := pintaA(t, esc, ancho, AltoBarraSuite()+MedidaBarraMovil().Height, b, movil)
					p, tam := r.divisor.Position(), r.divisor.Size()
					if p != antesPos || tam != antesTam {
						t.Errorf("%s %s %g: ocultar la raya ha movido el divisor de %v/%v a %v/%v", variante, forma, ancho,
							antesPos, antesTam, p, tam)
					}
					x := int((p.X + tam.Width/2) * esc)
					divisor, fondoBarra := px(img, x, int((p.Y+tam.Height/2)*esc)), px(img, x, 0)
					x = int(ancho / 2 * esc)
					ultima := int(math.Round(float64((movil.Position().Y+movil.Size().Height)*esc))) - 1
					filete := px(img, x, ultima)
					papel := px(img, x, int((movil.Position().Y+movil.Size().Height/2)*esc))
					for _, c := range []struct {
						nombre         string
						pintado, fondo color.NRGBA
					}{{"divisor de cuenta", divisor, fondoBarra}, {"filete móvil", filete, papel}} {
						iguales := cerca(c.pintado, c.fondo)
						if iguales != (forma == "marco") {
							t.Errorf("%s %s %g a ×%g: %s pinta %v sobre %v; Marco lo deja continuo y Carril conserva la raya",
								variante, forma, ancho, esc, c.nombre, c.pintado, c.fondo)
						}
					}
				}
			}
		}
	}
}

// TestElPuntoDeAvisoLlevaAroSobreLaBarraDeColor: sobre la barra de color el punto de la
// campana lleva un aro blanco, como una muestra de color. Se mide en los tres colores de
// la suite —con «el de cada app», en Mail, cuyo carril es ROJO, y en Drive— y en los tres
// modos: el punto contra su aro y el aro contra la barra, los dos a 3:1 (WCAG 1.4.11).
//
// VISTO EN ROJO con el rojo claro sin aro (el de antes): en cobalto queda a 2,6:1 de la
// barra, y en el de Mail, rojo sobre rojo.
func TestElPuntoDeAvisoLlevaAroSobreLaBarraDeColor(t *testing.T) {
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirApp("") })
	enMarco(t)
	for _, c := range []struct{ color, app string }{{"", ""}, {"cobalto", ""}, {"app", "mail"}, {"app", "drive"}, {"app", "calendar"}} {
		tema.ElegirColor(c.color)
		tema.ElegirApp(c.app)
		for _, variante := range []string{"claro", "oscuro", "traslucido"} {
			tema.Elegir(variante)
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			punto, aro := colorPuntoAviso(v, true)
			// La barra: los dos extremos del marco, que en el traslúcido es un degradado.
			for _, barra := range []color.NRGBA{tema.Color("frame-from", v), tema.Color("frame-to", v)} {
				if r := contraste(punto, aro); r < 3 {
					t.Errorf("%s %s %s: el punto contrasta %.2f:1 con su aro", c.color, c.app, variante, r)
				}
				if r := contraste(aro, barra); r < 3 {
					t.Errorf("%s %s %s: el aro contrasta %.2f:1 con la barra %v", c.color, c.app, variante, r, barra)
				}
			}
		}
	}
}

// TestElAvatarLlevaAroSobreLaBarraDeColor: el disco de la persona puede ser del tono de
// la barra, así que sobre la de color lleva el aro del texto del carril
// (`sytue-aro-carril` en AccountMenu.tsx); sobre la barra clara, no.
//
// VISTO EN ROJO sin el `esconder` del Refresh: el aro sale también en la barra clara.
func TestElAvatarLlevaAroSobreLaBarraDeColor(t *testing.T) {
	b := nuevoBotonAvatar("Ana Ruiz", "Cuenta", nil)
	r := test.WidgetRenderer(b).(*botonAvatarRenderer)
	b.Resize(b.MinSize())
	if r.anillo.Visible() {
		t.Error("sobre la barra clara el avatar no lleva aro")
	}
	b.carril = true
	b.Refresh()
	disco := MedidaAvatar(AvatarXS)
	if !r.anillo.Visible() || r.anillo.Size().Width != disco.Width+2*AroCarril {
		t.Errorf("sobre la barra de color el aro mide %v y se ve=%v; quiero el disco (%v) más %v por lado",
			r.anillo.Size(), r.anillo.Visible(), disco, AroCarril)
	}
	if quiero := tema.Color("sidebar-fg", tema.Claro); r.anillo.FillColor != quiero {
		t.Errorf("el aro es %v; quiero el texto del carril, %v", r.anillo.FillColor, quiero)
	}
}

// TestLaTeselaVaSobrePlacaBlancaConElColorDeCadaApp: con «el de cada app» el carril es
// del color de la propia tesela del producto, y la tesela va sobre una placa blanca
// redondeada, como el logo de una organización. Con los otros colores no la lleva.
func TestLaTeselaVaSobrePlacaBlancaConElColorDeCadaApp(t *testing.T) {
	conPetroleoClaro(t)
	b := barraPrueba(t)
	r := test.WidgetRenderer(b.marca).(*marcaBarraRenderer)
	if !r.placa.Hidden {
		t.Error("con el color de la casa la tesela no lleva placa")
	}
	tema.ElegirColor("app")
	b.marca.Refresh()
	if r.placa.Hidden || r.placa.FillColor != color.Color(color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("con «el de cada app» la tesela va sobre placa blanca: oculta=%v, color=%v", r.placa.Hidden, r.placa.FillColor)
	}
	if got, quiero := r.placa.Size().Width, iconos.MarcaMD+2*RebordePlacaMarca; got != quiero {
		t.Errorf("la placa mide %v y son la tesela y su `p-0.5`: %v", got, quiero)
	}
}

// TestElMenuDeMarcoVaEnPildoras: en Marco el ítem del menú es `rounded-nav` —una
// píldora—, con 14 de relleno y SIN barra de señal; el activo, el velo del menú con su
// texto (`--sytue-nav-active-*`). El botón de la cabecera usa la esquina compartida de 4 px.
//
// VISTO EN ROJO con el radio fijo de Carril en `FilaLateral.radio`.
func TestElMenuDeMarcoVaEnPildoras(t *testing.T) {
	enMarco(t)
	f := NuevaFilaLateral("Recibidos", iconoFalso(), nil)
	f.Activa = true
	m := MedidaFilaLateral(f)
	f.Resize(fyne.NewSize(AnchoUtilMenu, m.Height))
	r := test.WidgetRenderer(f).(*filaRenderer)
	f.Refresh()
	if got := f.radio(); got != 999 || r.caja.radio != 999 {
		t.Errorf("la fila redondea %v (caja %v) y `rounded-nav` en Marco es 999", got, r.caja.radio)
	}
	if RellenoNav() != 14 {
		t.Errorf("el relleno del ítem es %v y `--sytue-nav-pad` en Marco son 14", RellenoNav())
	}
	if f.Icono.Position().X != RellenoNav() {
		t.Errorf("el icono empieza en x=%v y quiero el relleno del ítem, %v", f.Icono.Position().X, RellenoNav())
	}
	if r.barra == nil || !r.barra.Hidden {
		t.Error("en Marco el ítem activo no lleva barra de señal: ya es una píldora")
	}
	fondo, texto := f.colores(tema.Claro)
	if fondo != tema.Color("nav-active-bg", tema.Claro) || texto != tema.Color("nav-active-fg", tema.Claro) {
		t.Errorf("el activo pinta %v con texto %v y son `nav-active-bg` y `nav-active-fg`", fondo, texto)
	}
	// Y al volver a Carril, sin reconstruir nada: la fila sigue a la forma.
	tema.ElegirForma("carril")
	f.Refresh()
	if r.caja.radio != tema.RadioMD || r.barra.Hidden || RellenoNav() != 10 {
		t.Errorf("de vuelta en Carril la fila redondea %v, barra oculta=%v y relleno %v", r.caja.radio, r.barra.Hidden, RellenoNav())
	}

	// Los botones: `rounded-control`.
	tema.ElegirForma("marco")
	b := NuevoBoton("Redactar", nil)
	rb := test.WidgetRenderer(b).(*botonRenderer)
	if radioTalla(b.Talla) != 4 || rb.caja.radio != 4 {
		t.Errorf("el botón redondea %v y en Marco son 4 px", rb.caja.radio)
	}
	tema.ElegirForma("carril")
	b.Refresh()
	if rb.caja.radio != 4 {
		t.Errorf("de vuelta en Carril el botón redondea %v y son 4 px", rb.caja.radio)
	}
}

// TestElIconoDeLaFilaSigueAlRotulo: el icono de un destino del menú lleva la tinta de su
// rótulo —atenuada en reposo, clara al pasar y en el activo—, que es lo que hace
// `currentColor` en la web. La app lo pasa ya hecho, con una tinta fija: la fila lo
// encuentra por su primitiva (`iconos.De`). Uno que no es del menú no se toca.
//
// VISTO EN ROJO sin el teñido en el `Refresh` de la fila: al pasar el puntero el icono
// se queda atenuado.
func TestElIconoDeLaFilaSigueAlRotulo(t *testing.T) {
	v := tema.Claro
	d := iconos.Trazo(iconos.Buscar, GlifoMenu, "sidebar-fg-muted", v)
	if iconos.De(d.Objeto()) != d || iconos.De(canvas.NewRectangle(magenta)) != nil {
		t.Fatal("`iconos.De` no devuelve el dibujo de su primitiva, o se lo inventa")
	}
	f := NuevaFilaLateral("Recibidos", d.Objeto(), nil)
	f.Resize(fyne.NewSize(AnchoUtilMenu, MedidaFilaLateral(f).Height))
	test.WidgetRenderer(f)
	f.Refresh()
	tinta := d.Tinta
	if got := tinta(); got != tema.Color("sidebar-fg-muted", v) {
		t.Errorf("en reposo el icono lleva %v y es la tinta atenuada", got)
	}
	f.MouseIn(nil)
	if got := tinta(); got != tema.Color("sidebar-fg", v) {
		t.Errorf("al pasar el puntero el icono lleva %v y es `sidebar-fg`", got)
	}
	f.MouseOut()
	f.Activa = true
	f.Refresh()
	if got := tinta(); got != tema.Color("nav-active-fg", v) {
		t.Errorf("en el activo el icono lleva %v y es la tinta del activo", got)
	}
	// Un icono con color propio —el de un calendario, una estrella— no es del menú.
	propio := iconos.TrazoConTinta(iconos.Buscar, GlifoMenu, magenta)
	g := NuevaFilaLateral("Personal", propio.Objeto(), nil)
	g.Resize(fyne.NewSize(AnchoUtilMenu, MedidaFilaLateral(g).Height))
	test.WidgetRenderer(g)
	g.Refresh()
	if got := propio.Tinta(); got != magenta {
		t.Errorf("la fila ha teñido de %v un icono con color propio", got)
	}
}

// TestLosTitulosDeLaHoja: dentro de la hoja de Marco los títulos van un 10 % más grandes
// y un escalón más gruesos (`--sytue-title-scale`, `-weight`); en Carril, como siempre.
// Los cuerpos, de `--text-3xl` a `--text-lg`.
func TestLosTitulosDeLaHoja(t *testing.T) {
	bases := []tema.Escalon{tema.Texto3XL, tema.Texto2XL, tema.TextoXL, tema.TextoLG}
	for i, base := range bases {
		if got := EscalonTitulo(i + 1); got != base {
			t.Errorf("Carril, h%d: %v y quiero %v", i+1, got, base)
		}
	}
	if PesoTitulo(1) != Negrita || PesoTitulo(2) != Negrita || PesoTitulo(3) != SemiNegrita || PesoTitulo(4) != SemiNegrita {
		t.Error("en Carril los títulos son 700 y los subtítulos 600")
	}
	enMarco(t)
	for i, base := range bases {
		got := EscalonTitulo(i + 1)
		if d := got.Tam - base.Tam*1.1; d > 0.01 || d < -0.01 || got.Linea <= base.Linea {
			t.Errorf("Marco, h%d: %v y quiero el de Carril (%v) por 1,1", i+1, got, base)
		}
	}
	if PesoTitulo(1) != ExtraNegrita || PesoTitulo(2) != ExtraNegrita || PesoTitulo(3) != Negrita || PesoTitulo(4) != Negrita {
		t.Error("en Marco los títulos son 800 y los subtítulos 700")
	}
	if ExtraNegrita.Cara() == Negrita.Cara() || ExtraNegrita.Cara() != tema.ExtraNegra {
		t.Error("el 800 tiene que pintarse con Figtree ExtraBold, no con la Bold")
	}
}

// TestLaFilaDeListaEnMarco: la fila de `ListItem` es en Marco un píxel más alta por
// arriba y por abajo, sin barra a la izquierda, y su velo —el de la elegida o el del
// cursor— un bloque redondeado que se separa 6 de los bordes. En Carril, lo de siempre.
func TestLaFilaDeListaEnMarco(t *testing.T) {
	velo, barra := canvas.NewRectangle(magenta), canvas.NewRectangle(magenta)
	ColocarVeloFila(velo, barra, 300, 50)
	if BarraFila() != 2 || RellenoYFila() != AltoFila || InicioFila() != 2+MargenFila ||
		velo.Position().X != 0 || velo.Size().Width != 300 || velo.CornerRadius != 0 || barra.Hidden || barra.Size().Width != 2 {
		t.Errorf("Carril: barra %v, relleno %v, inicio %v, velo en %v de %v con radio %v",
			BarraFila(), RellenoYFila(), InicioFila(), velo.Position().X, velo.Size().Width, velo.CornerRadius)
	}
	carril := MedidaFilaLista(true, 0)
	enMarco(t)
	ColocarVeloFila(velo, barra, 300, 50)
	if BarraFila() != 0 || RellenoYFila() != AltoFila+1 || InicioFila() != MargenFila ||
		velo.Position().X != 6 || velo.Size().Width != 288 || velo.CornerRadius != 10 || !barra.Hidden {
		t.Errorf("Marco: barra %v, relleno %v, inicio %v, velo en %v de %v con radio %v",
			BarraFila(), RellenoYFila(), InicioFila(), velo.Position().X, velo.Size().Width, velo.CornerRadius)
	}
	if got := MedidaFilaLista(true, 0); got != carril+2 {
		t.Errorf("la fila mide %v en Marco y %v en Carril: tienen que ser dos píxeles más", got, carril)
	}
	if got, quiero := AltoFilaTabla(), 2*(AltoCelda+1)+tema.TextoBase.Linea+tema.Borde; got != quiero {
		t.Errorf("la fila de tabla mide %v y en Marco son %v", got, quiero)
	}
}

// TestLaTarjetaDeRecursoEsDeColorPlenoEnMarco: una tarjeta que dice su tono es, en Marco,
// de color pleno —el relleno sólido del tono, su tinta y sin borde—; en Carril, y sin
// tono en las dos, el panel blanco. Y su esquina es `rounded-card`.
func TestLaTarjetaDeRecursoEsDeColorPlenoEnMarco(t *testing.T) {
	v := tema.Claro
	panel, recurso := NuevaTarjeta("Resumen", nil), NuevaTarjeta("Resumen", nil)
	recurso.Tono = TarjetaVioleta
	if recurso.rolFondo() != "surface" || radioTarjeta() != tema.RadioLG {
		t.Errorf("en Carril la tarjeta con tono pinta `%s` con radio %v", recurso.rolFondo(), radioTarjeta())
	}
	enMarco(t)
	if panel.rolFondo() != "surface" || panel.ColorTexto(v) != tema.Color("text", v) {
		t.Error("sin tono la tarjeta es un panel blanco también en Marco")
	}
	if recurso.rolFondo() != "card-solid-violet" || recurso.ColorTexto(v) != tema.Color("on-card-solid-violet", v) || recurso.borde(v).A != 0 {
		t.Errorf("en Marco la tarjeta con tono pinta `%s`, tinta %v y borde %v", recurso.rolFondo(), recurso.ColorTexto(v), recurso.borde(v))
	}
	if radioTarjeta() != 16 {
		t.Errorf("`rounded-card` en Marco son 16 y sale %v", radioTarjeta())
	}
}

// TestElSelectorDeCuentaGuardaLaForma: el selector de forma va junto al de color, con
// las claves de la web (`account.forma*`). Guarda la elección en los ajustes compartidos
// y la pinta; sin elección enseña la que esté en vigor —la de la organización o Marco—.
//
// VISTO EN ROJO sin `Ajustes.Forma` en lo que el selector escribe.
func TestElSelectorDeCuentaGuardaLaForma(t *testing.T) {
	suite := suiteConSesion(t)
	t.Cleanup(func() { tema.FormaDeLosGuardianes() })
	c := nuevaCuentaBarra(traductorPrueba(t), suite)
	// «Organización», Marco y Carril; sin elección, la primera.
	if s := c.forma(suite.Estado()); len(s.Opciones) != 3 || s.Elegida != 0 || s.Rotulo != c.t.T("account.forma", nil) {
		t.Fatalf("sin elección el selector ofrece «Organización», Marco y Carril y marca la primera: %+v", s)
	}
	c.forma(suite.Estado()).Elegir(2)
	if e := ajustes.Leer(); e.Forma != "carril" || tema.EsMarco() {
		t.Fatalf("elegir Carril ha guardado %q y marco=%v", e.Forma, tema.EsMarco())
	}
	if elegido := c.forma(suite.Estado()).Elegida; elegido != 2 {
		t.Errorf("reabrir el selector marca %d, quiero Carril", elegido)
	}
	c.forma(suite.Estado()).Elegir(1)
	if e := ajustes.Leer(); e.Forma != "marco" || !tema.EsMarco() {
		t.Errorf("elegir Marco ha guardado %q y marco=%v", e.Forma, tema.EsMarco())
	}
	// «Organización» borra la elección: se vuelve a seguir a la suya.
	c.forma(suite.Estado()).Elegir(0)
	if e := ajustes.Leer(); e.Forma != "" || c.forma(suite.Estado()).Elegida != 0 {
		t.Errorf("elegir «Organización» ha dejado la forma en %q; quiero que borre la elección", e.Forma)
	}
	// Una cuenta personal no tiene a quién seguir: no la ve, y sin elección propia el
	// selector marca lo que se está pintando.
	if s := nuevaCuentaBarra(traductorPrueba(t), nil).forma(sesion.Estado{Forma: "carril"}); len(s.Opciones) != 2 || s.Elegida != 1 {
		t.Errorf("una cuenta personal ve %d opciones y marca %d; quiero Marco y Carril, y Carril marcado", len(s.Opciones), s.Elegida)
	}
	// Con organización y sin elección propia marca «Organización», la tenga como la tenga.
	if s := nuevaCuentaBarra(traductorPrueba(t), nil).forma(sesion.Estado{Forma: "carril", ConOrganizacion: true}); s.Elegida != 0 {
		t.Error("quien sigue a su organización tiene que ver marcada «Organización»")
	}
	for _, locale := range i18n.Locales {
		tr, err := i18n.New(locale, i18n.Common())
		if err != nil {
			t.Fatal(err)
		}
		if s := nuevaCuentaBarra(tr, nil).forma(sesion.Estado{}); s.Opciones[0] == "" || s.Opciones[0] == "account.forma.marco" {
			t.Errorf("%s: falta «Marco» traducido: %q", locale, s.Opciones[0])
		}
	}
}

// TestElMarcoDelTraslucidoEsUnDegradado: en el traslúcido el marco es un degradado
// horizontal del primario, de lado a lado de la ventana, y las máscaras de la hoja pintan
// en cada esquina el color que el degradado tiene AHÍ. En claro y en oscuro es de un
// solo color y el degradado ni se genera. Y el lienzo ambiental no se pinta: lo que hay
// detrás del contenido es el papel de la hoja.
func TestElMarcoDelTraslucidoEsUnDegradado(t *testing.T) {
	enMarco(t)
	t.Cleanup(func() { tema.Elegir("") })
	f := nuevoFondoMarco()
	f.pintar(tema.Claro)
	if f.degradado.Visible() || !f.liso.Visible() {
		t.Error("en claro el marco es de un color: sobra el degradado")
	}
	tema.Elegir("traslucido")
	v := tema.Translucido
	f.pintar(v)
	desde, hasta := tema.Color("frame-from", v), tema.Color("frame-to", v)
	if desde == hasta || !f.degradado.Visible() || f.liso.Visible() {
		t.Fatalf("en traslúcido el marco va de %v a %v en degradado", desde, hasta)
	}
	if f.colorEn(0, 1000) != desde || f.colorEn(1000, 1000) != hasta {
		t.Errorf("el degradado va de %v a %v y quiero de %v a %v", f.colorEn(0, 1000), f.colorEn(1000, 1000), desde, hasta)
	}
	if medio := f.colorEn(500, 1000); medio == desde || medio == hasta {
		t.Error("a mitad de camino el marco es uno de sus extremos")
	}
	fondo := NuevoFondoSuite(v)
	if r := test.WidgetRenderer(fondo).(*fondoSuiteRenderer); !r.raster.Hidden {
		t.Error("en Marco traslúcido el lienzo ambiental no se ve: detrás del contenido va el papel de la hoja")
	}
	if tokens.EsCarril() {
		t.Fatal("el test no está en Marco")
	}
}

// La hoja móvil conserva sus cuatro arcos aunque el contenido opaco llegue al borde.
// A sangre sigue ocupando todo el ancho y el pie, sin añadir huecos ni sombras.
// SYTUE_CAPTURA_DIR guarda los seis retratos de 390 px, a ×2, con el helper común.
func TestLasEsquinasInferioresDeMarcoMovil(t *testing.T) {
	enMarco(t)
	tactilAntes, tactil := tactilDePrueba, true
	tactilDePrueba = &tactil
	t.Cleanup(func() {
		tactilDePrueba = tactilAntes
		tema.Elegir("")
		tema.ElegirColor("")
		tema.ElegirApp("")
	})
	const alto float32 = 420
	for _, variante := range []string{"claro", "oscuro", "traslucido"} {
		for _, ancho := range []float32{320, 390} {
			for _, tipo := range []string{"armazon", "editor"} {
				t.Run(fmt.Sprintf("%s-%s-%g", variante, tipo, ancho), func(t *testing.T) {
					tema.Elegir(variante)
					tema.ElegirColor("petroleo")
					var contenido fyne.CanvasObject = contenidoDePrueba()
					var a *Armazon
					if tipo == "armazon" {
						a = NuevoArmazon(contenido)
						a.Menu = NuevoLateral("Menú", nil)
						contenido = a
					}
					b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
					t.Cleanup(b.Parar)
					if tipo == "editor" {
						b.ConHojaASangre()
					}
					montado := b.Montar(contenido).(*fyne.Container)
					montado.Resize(fyne.NewSize(ancho, alto))
					m := montado.Layout.(*montaje).marco
					if a != nil {
						m = test.WidgetRenderer(a).(*armazonRenderer).marco
					}
					for _, sombra := range m.sombra.teselas {
						if sombra.Visible() {
							t.Error("a sangre no añade sombra al pie")
						}
					}
					for _, esc := range []float32{1, 2, 3} {
						img := pintaA(t, esc, ancho, alto, montado)
						izquierda := m.hoja.Position().X
						for _, x := range []float32{izquierda + 1, ancho - 2} {
							if got, quiero := px(img, int(x*esc), int((alto-2)*esc)), m.fondo.colorEn(x, ancho); !cercaDe(got, quiero, 3) {
								t.Errorf("×%g esquina inferior x=%g: %v, quiero marco %v", esc, x, got, quiero)
							}
						}
						// El centro del pie y el borde tras el arco siguen siendo contenido.
						for _, p := range []fyne.Position{{X: ancho / 2, Y: alto - 2}, {X: izquierda + 16, Y: alto - 2}, {X: izquierda + 1, Y: alto - 17}} {
							if got := px(img, int(p.X*esc), int(p.Y*esc)); !cercaDe(got, magenta, 3) {
								t.Errorf("×%g se perdió contenido tras el arco en %v: %v", esc, p, got)
							}
						}
						if ancho == 390 && esc == 2 {
							guardar(t, fmt.Sprintf("marco-inferior-%s-%s.png", tipo, variante), img)
						}
					}
				})
			}
		}
	}
}
