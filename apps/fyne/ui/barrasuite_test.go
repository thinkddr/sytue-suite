// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// anchoPrueba es la ventana con la que se midió la web (1440), que es donde entra el
// canalón ancho (`lg:px-6`).
const anchoPrueba float32 = 1440

func traductorPrueba(t *testing.T) *i18n.Translator {
	t.Helper()
	tr, err := i18n.New(i18n.Default, i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// barraPrueba construye la barra SIN Hub, que es como arranca hoy en las trece
// aplicaciones, la monta y la coloca a `anchoPrueba`.
//
// El montaje no es un detalle: sin renderer no hay `Layout`, y entonces todos los
// hijos contestan (0,0) y un guardián de colocación pasa sin haber mirado nada.
func barraPrueba(t *testing.T) *BarraSuite {
	t.Helper()
	return barraPruebaA(t, anchoPrueba)
}

// barraPruebaA es barraPrueba con otra ventana, para los cortes de la hoja.
func barraPruebaA(t *testing.T, ancho float32) *BarraSuite {
	t.Helper()
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
	// Sin esto, la búsqueda que quede esperando despierta DENTRO del test siguiente:
	// el `fyne.Do` del driver de pruebas no marshalla, así que su gorutina conforma
	// texto a la vez que él y el conformador se cae con un índice fuera de rango.
	t.Cleanup(b.Parar)
	tam := fyne.NewSize(ancho, AltoBarraSuite())
	b.Resize(tam)
	test.WidgetRenderer(b).Layout(tam)
	return b
}

// cuantos cuenta los píxeles de ese color, que es como se ve si algo se pinta.
func cuantos(img image.Image, c color.NRGBA) int {
	n := 0
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			if cerca(px(img, x, y), c) && px(img, x, y).A > 0 {
				n++
			}
		}
	}
	return n
}

// TestLaBarraDeLaSuiteEsMasFinaQueLaDeLaWeb es la medida del encargo: de 56 a 45,5.
//
// Los 56 son los MEDIDOS en producción el 2026-09-22 con
// `test/e2e/auditar-topbar.mjs` sobre Sytue Mail a 1440×900, donde además salen los
// controles a 31,5 y repartidos 12,3/12,3. Lo que se quita es lo que sostenía el
// logotipo del selector de aplicaciones: el canalón vertical baja de `py-3.5` a `py-2`.
//
// Se comprueba también PINTADO, que es lo que decide: 45,5 px de CSS a ×2 son 91 filas
// de dispositivo clavadas.
func TestLaBarraDeLaSuiteEsMasFinaQueLaDeLaWeb(t *testing.T) {
	if tema.AltoBarraSuperior != 56 {
		t.Fatalf("la barra de la web mide 56 y `tema.AltoBarraSuperior` dice %v:"+
			" si la web ha cambiado, esta comparación hay que rehacerla", tema.AltoBarraSuperior)
	}
	const quiero float32 = 52
	if AltoBarraSuite() != quiero {
		t.Errorf("la barra nativa mide %v y tiene que medir %v (31,5 del control + 2×7 de `py-2`)",
			AltoBarraSuite(), quiero)
	}
	if adelgaza := tema.AltoBarraSuperior - AltoBarraSuite(); adelgaza != 4 {
		t.Errorf("adelgaza %v y son %v: los 10,5 son el `py-3.5` menos el `py-2`, por dos lados",
			adelgaza, tema.Espacio(3))
	}

	b := barraPrueba(t)
	// El fondo de prueba debe distinguirse del chrome blanco del tema claro.
	lienzo := canvas.NewRectangle(color.NRGBA{R: 240, G: 240, B: 240, A: 255})
	lienzo.Resize(fyne.NewSize(anchoPrueba, 120))
	img := pinta(t, anchoPrueba, 120, lienzo, b)
	// El fondo claro de la barra llega al borde derecho y baja hasta el filete. Por la
	// izquierda no se mide con la caja: la esquina de marca es del color del carril, y
	// el blanco que hay dentro es el del glifo de la marca.
	_, x1, y0, y1 := caja(img, tema.Color(fondoBarra, tema.Claro))
	if y0 != 0 || x1 != int(anchoPrueba*escala)-1 {
		t.Errorf("el fondo de la barra va de la fila %d a la columna %d; quiero de la 0 a la %d", y0, x1, int(anchoPrueba*escala)-1)
	}
	if filas := y1 - y0 + 1; filas != 102 {
		t.Errorf("a ×2 la barra pinta %d filas y 45,5 son 91 clavadas", filas)
	}

	// En oscuro sí se ve, y es lo que la separa del contenido. Va DENTRO de los 45,5 y
	// no participa en el reparto (`shadow-[inset_0_-1px_0_…]`), que es lo que impide
	// que el contenido quede un píxel alto: tiene que ser la ÚLTIMA fila.
	tema.Elegir("oscuro")
	t.Cleanup(func() { tema.Elegir("") })
	b = barraPrueba(t)
	img = pinta(t, anchoPrueba, 120, b)
	// Se mira una columna entre la esquina de marca y el buscador, donde solo hay fondo
	// y filete: buscando el color en toda la imagen salen también los bordes suavizados
	// del campo.
	columna := int((tema.AnchoLateral + 4) * escala)
	fondo, filete := tema.Color(fondoBarra, tema.Oscuro), BordeSobre("border", fondoBarra, tema.Oscuro)
	grosor := int(tema.Borde * escala)
	for y := range 104 {
		quiero, que := fondo, "el fondo"
		if y >= 104-grosor {
			quiero, que = filete, "el filete"
		}
		if got := px(img, columna, y); !cerca(got, quiero) {
			t.Errorf("en oscuro la fila %d de la barra pinta %v y tendría que ser %s %v", y, got, que, quiero)
			break
		}
	}
	if got := px(img, columna, 104); cerca(got, filete) || cerca(got, fondo) {
		t.Errorf("la fila 91 sigue siendo barra (%v): a ×2 la barra son 91 filas", got)
	}
}

// TestElFileteDeLaBarraNoCruzaElCarril: la línea de abajo de la barra es un borde de la
// hoja compuesto sobre SU fondo, y se para en la esquina de marca, que la tapa: el
// carril se lee de arriba abajo sin una raya que lo parta (TopBar.tsx estira la zona de
// marca a todo el alto por lo mismo). Y el menú no lleva filete a la derecha.
//
// VISTO EN ROJO con la esquina pintada antes que el filete: la última fila de la barra
// sale del color del filete también sobre el carril.
func TestElFileteDeLaBarraNoCruzaElCarril(t *testing.T) {
	m, _ := cromoPrueba(t, anchoPrueba, 200)
	img := pinta(t, anchoPrueba, 200, m)
	carril := tema.Color("sidebar-bg", tema.Claro)
	ultima := int(AltoBarraSuite()*escala) - 1
	if got := px(img, int(10*escala), ultima); !cerca(got, carril) {
		t.Errorf("la última fila de la barra pinta %v sobre la esquina de marca y tendría que ser el carril %v", got, carril)
	}
	if got, quiero := px(img, int((tema.AnchoLateral+4)*escala), ultima), BordeSobre("border", fondoBarra, tema.Claro); !cerca(got, quiero) {
		t.Errorf("fuera del carril la última fila de la barra pinta %v y es su filete %v", got, quiero)
	}
	if got := px(img, int(tema.AnchoLateral*escala)-1, int((AltoBarraSuite()+60)*escala)); !cerca(got, carril) {
		t.Errorf("la última columna del menú pinta %v: el carril no lleva filete a la derecha (%v)", got, carril)
	}
}

// cromoPrueba monta la barra sobre un lateral y un contenido, como lo hace cada `main`
// con `Montar`, con la variante que esté elegida. `filas` son las del lateral: vacío
// en el guardián, con destinos en el retrato.
func cromoPrueba(t *testing.T, ancho, alto float32, filas ...fyne.CanvasObject) (fyne.CanvasObject, *BarraSuite) {
	t.Helper()
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
	t.Cleanup(b.Parar)
	var cuerpo fyne.CanvasObject
	if len(filas) > 0 {
		cuerpo = NuevaSeccionLateral("Carpetas", filas...)
	}
	contenido := container.NewBorder(nil, nil, NuevoLateral("Menú", cuerpo), nil,
		canvas.NewRectangle(tema.Color("bg", v)))
	m := b.Montar(contenido)
	m.Resize(fyne.NewSize(ancho, alto))
	return m, b
}

// TestLaEsquinaDeMarcaContinuaElCarril es la forma Carril (dueño, 2026-10-04): el menú
// lateral es de color de arriba abajo, la barra superior es CLARA y solo su esquina de
// marca lleva el color del carril, con el ancho del menú. Sustituye a la regla del
// 2026-09-23, que pintaba la barra entera del color del lateral.
//
// Compara PÍXELES de las piezas pintadas juntas, como las monta una app, y no nombres
// de token. VISTO EN ROJO con `fondoBarra = "sidebar-bg"`: fuera de la esquina la barra
// pinta el color del carril y no el suyo.
func TestLaEsquinaDeMarcaContinuaElCarril(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	vistos := map[color.NRGBA]bool{}
	for _, variante := range []string{"claro", "oscuro"} {
		tema.Elegir(variante)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		m, _ := cromoPrueba(t, anchoPrueba, 200)
		img := pinta(t, anchoPrueba, 200, m)
		// La esquina a media altura y a la izquierda de la marca; el lateral bajo la
		// barra; y la barra justo a la derecha de la esquina, antes del buscador.
		media := int(AltoBarraSuite() / 2 * escala)
		barra := px(img, int(10*escala), media)
		lateral := px(img, int(10*escala), int((AltoBarraSuite()+60)*escala))
		if barra != lateral {
			t.Errorf("en %s la esquina de marca pinta %v y el lateral %v: tienen que ser el mismo color",
				variante, barra, lateral)
		}
		// La esquina mide lo que el menú: su último píxel es carril y el siguiente, barra.
		if got := px(img, int(tema.AnchoLateral*escala)-1, media); got != lateral {
			t.Errorf("en %s la esquina no llega al ancho del menú: en su borde pinta %v", variante, got)
		}
		fuera := px(img, int((tema.AnchoLateral+4)*escala), media)
		if quiero := tema.Color(fondoBarra, v); !cerca(fuera, quiero) || fuera == lateral {
			t.Errorf("en %s la barra pinta %v fuera de la esquina y es `%s`, %v, distinto del carril",
				variante, fuera, fondoBarra, quiero)
		}
		vistos[barra] = true
	}
	// Sin esto, un `Elegir` que no hiciera nada daría el claro dos veces y el oscuro
	// no se habría mirado nunca.
	if len(vistos) != 2 {
		t.Error("claro y oscuro pintan la barra del mismo color: el oscuro no se ha comprobado")
	}
}

// TestLaEsquinaDeMarcaSeCineSinMenuLateral: en Carril, en una app SIN menú lateral (el
// Hub, SyPhotos, la portada de un editor), la esquina de marca pinta solo lo que ocupa la
// marca —su `ps-4 pe-3`— en vez del ancho de un menú que no existe, pero RESERVA lo
// mismo: el buscador no se mueve al pasar de una app a otra. Es
// `.sytue-esquina-marca { width: fit-content }` de appearance.css sobre las dos cajas de
// TopBar.tsx. Con menú lateral, o con un carril de iconos como el del ERP, mide lo que
// el menú.
//
// VISTO EN ROJO pintando siempre lo reservado: a la derecha de la marca sigue el color
// del carril hasta los 260.
func TestLaEsquinaDeMarcaSeCineSinMenuLateral(t *testing.T) {
	conPetroleoClaro(t)
	hoja := webref.Read(t, "apps/frontend/packages/config/src/tokens/appearance.css")
	barra := webref.Read(t, "apps/frontend/packages/ui/src/components/TopBar.tsx")
	for _, c := range []struct{ fuente, trozo, porque string }{
		{hoja, "  [data-forma='carril']:not(:has(nav:is(.bg-sidebar, .bg-rail))) .sytue-esquina-marca {\n    width: fit-content;", "sin menú lateral, la esquina se ciñe (desde 768)"},
		{barra, `<div class="-ms-4 flex shrink-0 self-stretch md:w-[var(--spacing-sidebar)] lg:-ms-6">`, "la caja de fuera reserva el ancho del menú"},
		{barra, `<div class="sytue-esquina-marca bg-sidebar text-sidebar-fg flex w-full items-center gap-2 ps-4 pe-3">`, "la de dentro pinta, con su relleno"},
	} {
		if !strings.Contains(c.fuente, c.trozo) {
			t.Errorf("la web ya no dice %q (%s): la esquina de marca de aquí ha dejado de ser la suya", c.trozo, c.porque)
		}
	}

	// Arriba del todo, por encima de la marca: ahí solo hay fondo.
	const y = 2
	en := func(img image.Image, x float32) color.NRGBA { return px(img, int(x*escala), int(y*escala)) }

	// Sin menú lateral: lo que hay bajo la barra es contenido liso.
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.One)
	t.Cleanup(b.Parar)
	carril, clara := tema.Color(fondoEsquina, tema.Claro), tema.Color(fondoBarra, tema.Claro)
	m := b.Montar(canvas.NewRectangle(tema.Color("bg", tema.Claro)))
	m.Resize(fyne.NewSize(anchoPrueba, 200))
	img := pinta(t, anchoPrueba, 200, m)
	cenida := esquinaCenida(b.marca.Size().Width)
	if cenida >= tema.AnchoLateral/2 {
		t.Fatalf("la esquina ceñida mide %v: no es la marca con su relleno", cenida)
	}
	if got := en(img, cenida-2); !cerca(got, carril) {
		t.Errorf("sin menú, la esquina no llega a la marca: en %v pinta %v y es el carril %v", cenida-2, got, carril)
	}
	for _, x := range []float32{cenida + 2, tema.AnchoLateral - 2} {
		if got := en(img, x); !cerca(got, clara) {
			t.Errorf("sin menú, pasada la marca la barra pinta %v en %v y es la clara %v: la esquina no se ha ceñido", got, x, clara)
		}
	}
	buscador := b.buscador.campo.Position()

	// Con menú lateral, la de siempre; y el buscador, en el mismo sitio.
	conMenu, b2 := cromoPrueba(t, anchoPrueba, 200)
	if got := en(pinta(t, anchoPrueba, 200, conMenu), tema.AnchoLateral-2); !cerca(got, carril) {
		t.Errorf("con menú lateral la esquina no llega a su ancho: en su borde pinta %v", got)
	}
	if got := b2.buscador.campo.Position(); got != buscador {
		t.Errorf("el buscador está en %v con menú y en %v sin él: la esquina reserva siempre lo mismo", got, buscador)
	}

	// Y con el carril de iconos de una app, que también es su menú.
	a := NuevoArmazon(canvas.NewRectangle(tema.Color("bg", tema.Claro)))
	iconos72 := canvas.NewRectangle(carril)
	iconos72.SetMinSize(fyne.NewSize(72, 0))
	a.Carril = iconos72
	b3 := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
	t.Cleanup(b3.Parar)
	conCarril := b3.Montar(a)
	conCarril.Resize(fyne.NewSize(anchoPrueba, 200))
	if got := en(pinta(t, anchoPrueba, 200, conCarril), tema.AnchoLateral-2); !cerca(got, carril) {
		t.Errorf("con carril de iconos la esquina no llega al ancho del menú: en su borde pinta %v", got)
	}
}

// TestRetratoDeLaBarraConElLateral guarda la barra y su armazón para MIRAR el cromo:
// filete, divisor, campana con aviso, avatar y esquinas de la hoja. No afirma nada.
// Con `SYTUE_CAPTURA_CROMO=/ruta/cromo` deja PNGs de los tres modos, a 390 táctil y
// 1280 de escritorio, a ×1, ×1,25, ×1,5, ×2 y ×3. `SYTUE_FORMA=marco` elige Marco.
// Es el pintor de SOFTWARE: no sustituye a una captura del dispositivo con GL.
func TestRetratoDeLaBarraConElLateral(t *testing.T) {
	base := os.Getenv("SYTUE_CAPTURA_CROMO")
	if base == "" {
		t.Skip("sin SYTUE_CAPTURA_CROMO no hay retrato que guardar")
	}
	t.Cleanup(func() { tema.Elegir("") })
	for _, variante := range []string{"claro", "traslucido", "oscuro"} {
		for _, ventana := range []struct {
			ancho, alto float32
			tactil      bool
		}{{390, 360, true}, {1280, 240, false}} {
			t.Run(fmt.Sprintf("%s-%g", variante, ventana.ancho), func(t *testing.T) {
				tema.Elegir(variante)
				tactilAntes := tactilDePrueba
				tactilDePrueba = &ventana.tactil
				t.Cleanup(func() { tactilDePrueba = tactilAntes })

				v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
				activa := NuevaFilaLateral("Recibidos", nil, nil)
				activa.Activa, activa.Contador = true, "17"
				armazon := NuevoArmazon(canvas.NewRectangle(tema.Color("bg", v)))
				armazon.Menu = NuevoLateral("Menú", NuevaSeccionLateral("Carpetas", activa,
					NuevaFilaLateral("Borradores", nil, nil), NuevaFilaLateral("Enviados", nil, nil)))
				b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
				t.Cleanup(b.Parar)
				m := b.Montar(armazon)
				m.Resize(fyne.NewSize(ventana.ancho, ventana.alto))
				b.cuenta.boton.avatar.Nombre = "Ada Lovelace"
				b.cuenta.boton.avatar.Presencia = EnLinea
				b.avisos.sinLeer = 3
				b.Refresh()
				t.Logf("pintor=software marco=%v táctil=%v ventana=%gx%g barra=%g", tema.EsMarco(), ventana.tactil,
					ventana.ancho, ventana.alto, AltoBarraSuite())
				for _, esc := range []float32{1, 1.25, 1.5, 2, 3} {
					nombre := fmt.Sprintf("%s-%s-%g-x%g.png", base, variante, ventana.ancho, esc)
					f, err := os.Create(nombre)
					if err != nil {
						t.Fatal(err)
					}
					if err := png.Encode(f, pintaA(t, esc, ventana.ancho, ventana.alto, m)); err != nil {
						_ = f.Close()
						t.Fatal(err)
					}
					if err := f.Close(); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

// TestLasTresPiezasVanArribaALaDerecha es el encargo, literal: «el buscador,
// Notificaciones y el menú deben quedar arriba a la derecha de cada app».
//
// Se comprueba el orden, que el avatar cierre a la derecha con el canalón de la web
// (21 a 1440) y que las tres estén centradas en la barra —el defecto que la auditoría
// de la web persigue es justo este: un control dos píxeles más arriba que su vecino—.
func TestLasTresPiezasVanArribaALaDerecha(t *testing.T) {
	b := barraPrueba(t)

	campo, campana, avatar := b.buscador.campo, b.avisos.boton, b.cuenta.boton
	if !(campo.Position().X < campana.Position().X && campana.Position().X < avatar.Position().X) {
		t.Errorf("el orden es buscador, campana y avatar, y salen en %v, %v y %v",
			campo.Position().X, campana.Position().X, avatar.Position().X)
	}
	der := anchoPrueba - (avatar.Position().X + avatar.Size().Width)
	if der != CanalonBarraLG {
		t.Errorf("el avatar cierra a %v del cristal y el canalón de la web a 1440 es %v",
			der, CanalonBarraLG)
	}
	for nombre, o := range map[string]fyne.CanvasObject{"campana": campana, "avatar": avatar} {
		arriba := o.Position().Y
		abajo := AltoBarraSuite() - (o.Position().Y + o.Size().Height)
		if arriba != abajo {
			t.Errorf("la %s tiene %v arriba y %v abajo: no está centrada", nombre, arriba, abajo)
		}
	}
	if b.buscador.campo.Size().Width != AnchoBuscadorBarra {
		t.Errorf("el buscador mide %v y el ancho canónico (`max-w-[30rem]` con el rem de 14) es %v",
			b.buscador.campo.Size().Width, AnchoBuscadorBarra)
	}
}

// TestLaMarcaDeLaAppVaALaIzquierda es el encargo del dueño (2026-09-23): «a las apps de
// Fyne les falta su logotipo de cada app (como en el selector de apps en la web)». La
// marca del producto —la de la tesela del selector, a 28 como la de la barra de la
// web— en el hueco `brand`: a `px-6` del cristal a 1440, como el avatar al otro lado (el
// `pl-1` del disparador se come con `-ml-1`), centrada en la barra, y el nombre corto a
// su lado. PINTADA: el color de la tesela de Mail.
//
// VISTO EN ROJO sin la marca en los objetos del renderer: no hay ni un píxel de
// `product-mail` en la barra.
func TestLaMarcaDeLaAppVaALaIzquierda(t *testing.T) {
	b := barraPrueba(t)
	img := pinta(t, anchoPrueba, 120, b)
	x0, x1, y0, y1 := caja(img, tema.Color("product-mail", tema.Claro))
	if x1 < 0 {
		t.Fatal("no hay marca de Mail en la barra")
	}
	mismaCaja(t, "la marca", fyne.NewSize(iconos.MarcaMD, iconos.MarcaMD), x0, x1, y0, y1)
	// Con un píxel de dispositivo de margen: 8,75 de arriba a ×2 es media fila, y la
	// primera fila entera del color de la tesela es la de 9.
	lejos := func(a, b float32) bool { return a-b > 1/escala || b-a > 1/escala }
	x, y := float32(x0)/escala, float32(y0)/escala
	// El canalón de la marca es el `px-4` del MENÚ, no el de la barra, que crece con la
	// ventana: cae en la vertical del botón de la cabecera del menú.
	if quiero := canalonMenu; lejos(x, quiero) {
		t.Errorf("la marca empieza en x=%v y el `ps-4` de la esquina de marca es %v", x, quiero)
	}
	if quiero := (AltoBarraSuite() - iconos.MarcaMD) / 2; lejos(y, quiero) {
		t.Errorf("la marca empieza en y=%v y centrada en la barra son %v", y, quiero)
	}
	if !b.marca.conNombre || b.marca.AccessibilityLabel() != "Sytue Mail" {
		t.Errorf("a 1440 va el nombre (%v) y se anuncia «Sytue Mail» (%q)",
			b.marca.conNombre, b.marca.AccessibilityLabel())
	}
	// Por debajo de `sm:` el nombre se va y la marca se queda, como en la web.
	if b := barraPruebaA(t, 360); b.marca.conNombre {
		t.Error("a 360 el nombre del producto no cabe y la web lo esconde (`max-sm:hidden`)")
	}
}

// TestLaBarraHablaConElCatalogoComun: la barra es de la suite y sus textos son los del
// catálogo común aunque la app redefina la clave. VISTO EN ROJO pasándole a las piezas
// el traductor de la app: el buscador de la suite decía lo que dice el de Rooms.
func TestLaBarraHablaConElCatalogoComun(t *testing.T) {
	app := fstest.MapFS{"en.json": {Data: []byte(`{"search.placeholder": "Search messages"}`)}}
	tr, err := i18n.New(i18n.Default, i18n.Common(), app)
	if err != nil {
		t.Fatal(err)
	}
	b := NuevaBarraSuite(tr, nil, iconos.Rooms)
	t.Cleanup(b.Parar)
	comun := traductorPrueba(t).T("search.placeholder", nil)
	if got := b.buscador.campo.marca; got != comun {
		t.Errorf("el buscador de la suite dice %q y el catálogo común %q", got, comun)
	}
}

// TestLasPiezasDeUIHablanElIdiomaDeLaApp: el aspa de un aviso, la de un diálogo, la
// presencia de un avatar y la tabla vacía salen del catálogo común en el idioma de la
// app, que deja la barra. VISTO EN ROJO con el «Descartar» escrito a mano: en alemán
// seguía diciendo «Descartar».
func TestLasPiezasDeUIHablanElIdiomaDeLaApp(t *testing.T) {
	antes := comun.Load()
	t.Cleanup(func() { comun.Store(antes) })
	de, err := i18n.New("de", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	b := NuevaBarraSuite(de, nil, iconos.Mail)
	t.Cleanup(b.Parar)

	a := NuevoAvatar("Ada Lovelace")
	a.Presencia = NoMolestar
	for que, par := range map[string][2]string{
		"el aspa del aviso":   {NuevoAviso(AvisoInfo, "x", "").RotuloCerrar, de.T("ui.dismissNotification", nil)},
		"el aspa del diálogo": {NuevoDialogo("x", nil).RotuloCerrar, de.T("ui.close", nil)},
		"la tabla vacía":      {NuevaTabla(nil, nil, nil).Vacia, de.T("ui.noResults", nil)},
		"el avatar":           {a.AccessibilityLabel(), "Ada Lovelace · " + de.T("presence.dnd", nil)},
		"la rueda":            {(&Rueda{}).AccessibilityLabel(), de.T("ui.loading", nil)},
	} {
		if par[0] != par[1] {
			t.Errorf("%s dice %q y en alemán es %q", que, par[0], par[1])
		}
	}
}

// TestElBuscadorVaCentradoEnElHueco: el buscador va CENTRADO entre la marca y las
// acciones, como el `flex-1 justify-center` de TopBar.tsx, con su tope de 30 rem y
// encogiendo cuando no cabe. Se comprueba en los cortes de la hoja y entre ellos.
//
// VISTO EN ROJO con la colocación de antes, pegado a las acciones: a 1440 el centro
// del campo caía 237,6 px a la derecha del centro del hueco.
func TestElBuscadorVaCentradoEnElHueco(t *testing.T) {
	for _, ancho := range []float32{1440, 1280, 1024, 900, 768, 640, 500, 360} {
		b := barraPruebaA(t, ancho)
		campo := b.buscador.campo
		desde := AnchoEsquinaMarca(ancho, b.marca.Size().Width) + HuecoBarra
		hasta := b.avisos.boton.Position().X - HuecoBarra
		centro := campo.Position().X + campo.Size().Width/2
		if d := centro - (desde+hasta)/2; d > 0.25 || d < -0.25 {
			t.Errorf("a %v el centro del buscador cae a %+.2f del centro del hueco", ancho, d)
		}
		if quiero := min(AnchoBuscadorBarra, hasta-desde); campo.Size().Width != quiero {
			t.Errorf("a %v el buscador mide %v y en un hueco de %v son %v",
				ancho, campo.Size().Width, hasta-desde, quiero)
		}
	}
}

// TestLaPistaDelAtajoSoloConTeclado: `hidden md:inline-flex`. Por debajo de 768 de
// ventana la web no la pinta, y aquí tampoco.
func TestLaPistaDelAtajoSoloConTeclado(t *testing.T) {
	if !barraPrueba(t).buscador.atajo.Visible() {
		t.Error("a 1440 la pista ⌘K tiene que verse")
	}
	if barraPruebaA(t, 700).buscador.atajo.Visible() {
		t.Error("a 700 la web esconde la pista del atajo y aquí se ve")
	}
}

// TestElPuntoDeAvisoSaleSoloConAvisosSinLeer: el `indicator` de `TopBarButton`.
func TestElPuntoDeAvisoSaleSoloConAvisosSinLeer(t *testing.T) {
	b := barraPrueba(t)
	rojo, _ := colorPuntoAviso(tema.Claro, false)

	if n := cuantos(pinta(t, anchoPrueba, 120, b), rojo); n != 0 {
		t.Errorf("sin avisos sin leer hay %d píxeles de aviso y no debería haber ninguno", n)
	}
	b.avisos.sinLeer = 3
	b.Refresh()
	if n := cuantos(pinta(t, anchoPrueba, 120, b), rojo); n == 0 {
		t.Error("con tres avisos sin leer no se pinta el punto")
	}
}

// TestElMenuDeCuentaLlevaSusPartes comprueba lo que el dueño enseñó en su captura:
// cabecera, presencia con sus cuatro opciones y su marca, apariencia, idioma y «Mi
// cuenta». Sin sesión no hay «Cerrar sesión», y eso también se vigila.
func TestElMenuDeCuentaLlevaSusPartes(t *testing.T) {
	b := barraPrueba(t)
	b.cuenta.estado = sesion.Estado{Sujeto: "ada", Presencia: api.NoMolestar}
	partes := b.cuenta.contenido()

	var opciones, marcadas int
	for _, p := range partes {
		e, es := p.(*ElementoMenu)
		if !es || e.Punto == "" {
			continue
		}
		opciones++
		if e.Marcada {
			marcadas++
		}
	}
	if opciones != 4 {
		t.Errorf("hay %d presencias y son cuatro (En línea, Ausente, No molestar, Desconectado)", opciones)
	}
	if marcadas != 1 {
		t.Errorf("hay %d presencias marcadas y tiene que haber UNA: la que dice el servidor", marcadas)
	}
	if _, es := partes[0].(*fyne.Container); !es {
		t.Errorf("la primera parte es la cabecera con el avatar, y es un %T", partes[0])
	}
	var peligro bool
	for _, p := range b.cuenta.contenido() {
		if e, es := p.(*ElementoMenu); es && e.Peligro {
			peligro = true
		}
	}
	if !peligro {
		t.Error("con sesión tiene que estar «Cerrar sesión», en rojo")
	}
}

// rotulos son los textos de los `Rotulo` que cuelgan de una pieza del menú, para
// mirar lo que DICE sin tener que leer píxeles.
func rotulos(o fyne.CanvasObject) []string {
	switch v := o.(type) {
	case *Rotulo:
		return []string{v.Texto}
	case *fyne.Container:
		var out []string
		for _, h := range v.Objects {
			out = append(out, rotulos(h)...)
		}
		return out
	}
	return nil
}

// TestLaOrganizacionSaleDelTokenYNoDelCorreo: la línea «Organización: …» es el claim
// `tenant` que Sytue ID pone en `GET /me` (`active_org`), igual que en la web.
//
// El caso que importa es justo este —el correo NO es del dominio de la organización—
// porque es el que el apaño anterior contestaba mal: recortaba el dominio del correo y
// decía «Organización: gmail.com» de alguien invitada a `acme`.
func TestLaOrganizacionSaleDelTokenYNoDelCorreo(t *testing.T) {
	b := barraPrueba(t)
	b.cuenta.ficha = api.Profile{Name: "Ada Lovelace", Email: "ada@gmail.com", ActiveOrg: "acme"}
	b.cuenta.estado = sesion.Estado{Sujeto: "ada"}

	dice := rotulos(b.cuenta.contenido()[0])
	quiero := b.cuenta.t.T("account.organization", i18n.Vars{"tenant": "acme"})
	if !slices.Contains(dice, quiero) {
		t.Errorf("la cabecera dice %q y falta %q", dice, quiero)
	}
	sobra := b.cuenta.t.T("account.organization", i18n.Vars{"tenant": "gmail.com"})
	if slices.Contains(dice, sobra) {
		t.Errorf("la organización sale del dominio del correo: %q", sobra)
	}

	// Sin organización activa no hay línea que inventar.
	b.cuenta.ficha.ActiveOrg = ""
	if dice := rotulos(b.cuenta.contenido()[0]); slices.Contains(dice, quiero) {
		t.Errorf("sin `active_org` la cabecera sigue diciendo la organización: %q", dice)
	}
}

// TestLaMarcaDePresenciaSigueAlServidor: el tic y los dos puntos salen del estado que
// dice el Hub, no de lo último que se pulsó. Y `busy` es «No molestar»: el servicio no
// conoce el `dnd` que manda la web.
func TestLaMarcaDePresenciaSigueAlServidor(t *testing.T) {
	b := barraPrueba(t)
	b.cuenta.cambio(sesion.Estado{Sujeto: "ada", Presencia: api.NoMolestar})
	if b.cuenta.boton.avatar.Presencia != NoMolestar {
		t.Errorf("el punto del avatar es %v y el servidor dice `busy`, que es No molestar",
			b.cuenta.boton.avatar.Presencia)
	}
	for _, p := range b.cuenta.contenido() {
		e, es := p.(*ElementoMenu)
		if !es || e.Punto == "" {
			continue
		}
		if quiero := e.Punto == "presence-dnd"; e.Marcada != quiero {
			t.Errorf("«%s» está marcada=%v y tendría que ser %v", e.Texto, e.Marcada, quiero)
		}
	}
}

// TestLaAparienciaMandaSobreLaDelSistema: el grupo segmentado no es un adorno.
func TestLaAparienciaMandaSobreLaDelSistema(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })

	tema.Elegir("oscuro")
	if v := tema.De(temafyne.VariantLight); v != tema.Oscuro {
		t.Errorf("con «Oscuro» elegido, el tema contesta %v aunque el sistema diga claro", v)
	}
	tema.Elegir("claro")
	if v := tema.De(temafyne.VariantDark); v != tema.Claro {
		t.Errorf("con «Claro» elegido, el tema contesta %v aunque el sistema diga oscuro", v)
	}
	tema.Elegir("")
	if tema.De(temafyne.VariantDark) != tema.Oscuro || tema.De(temafyne.VariantLight) != tema.Claro {
		t.Error("con «Sistema» tiene que volver a mandar la variante de Fyne")
	}
}

// TestElSegmentadoMideLoQueDice compara la medida declarada con la pintada. El carril
// es `p-0.5` sobre opciones de `py-1 text-xs`: 28 de alto.
func TestElSegmentadoMideLoQueDice(t *testing.T) {
	s := NuevoSegmentado("Apariencia", []string{"Sistema", "Claro", "Oscuro"}, nil)
	declarada := MedidaSegmentado(s.Opciones)
	if declarada.Height != 28 {
		t.Errorf("el carril mide %v de alto y son 28 (24 + 2×2)", declarada.Height)
	}
	if s.MinSize() != declarada {
		t.Errorf("MinSize dice %v y MedidaSegmentado %v: la medida se declara UNA vez",
			s.MinSize(), declarada)
	}
	s.Resize(declarada)
	img := pinta(t, declarada.Width+20, declarada.Height+20, s)
	x0, x1, y0, y1 := caja(img, tema.Color("bg-subtle", tema.Claro))
	mismaCaja(t, "el grupo segmentado", declarada, x0, x1, y0, y1)
}

// TestElBuscadorSeAbreYSeCierraSinRobarElFoco: el panel de resultados nace escondido,
// aparece al escribir y se recoge con Escape, y el foco se queda SIEMPRE en el campo
// (patrón combobox: hay que poder seguir escribiendo).
func TestElBuscadorSeAbreYSeCierraSinRobarElFoco(t *testing.T) {
	b := barraPrueba(t)
	if b.buscador.panel.Visible() {
		t.Error("el panel de resultados nace abierto y tiene que nacer recogido")
	}
	// Sin Hub, buscar falla: el panel se abre para DECIRLO, con su reintento.
	b.buscador.alEscribir("factura")
	esperar(t, func() bool { return b.buscador.panel.Visible() })
	if !b.buscador.panel.Visible() {
		t.Fatal("al escribir no se abre el panel")
	}
	if !b.buscador.alTeclear(&fyne.KeyEvent{Name: fyne.KeyEscape}) {
		t.Error("Escape con el panel abierto lo tiene que recoger el buscador, no el editor")
	}
	if b.buscador.panel.Visible() {
		t.Error("Escape no ha recogido el panel")
	}
}

// esperar da tiempo a la espera del buscador (250 ms) sin dormir a ciegas.
//
// Cada vuelta pasa por `fyne.DoAndWait`: desde el fork `.4` el driver de pruebas ENCOLA
// los `fyne.Do` que llegan de otras rutinas (el reloj del buscador, la respuesta de la
// red) y los ejecuta en la del test en su siguiente punto de sincronía. Sin él, lo que la
// espera tiene que ver no llega a pasar nunca mientras se espera.
func esperar(t *testing.T, listo func() bool) {
	t.Helper()
	for range 60 {
		fyne.DoAndWait(func() {})
		if listo() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestRetratoDeLaBarra guarda la barra en un PNG para MIRARLA. No afirma nada: los
// cinco fallos que esta sesión encontró pasaban todos los guardianes y solo se vieron
// mirando. Con `SYTUE_CAPTURA=/ruta/barra.png go test -run Retrato ./ui`.
func TestRetratoDeLaBarra(t *testing.T) {
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
	b.cuenta.ficha = api.Profile{Name: "Ada Lovelace", Email: "ada@sytue.eu", ActiveOrg: "corporate"}
	b.cuenta.estado = sesion.Estado{Sujeto: "ada", Presencia: api.EnLinea}
	// Sin `sesion.Suite` no corre `pintar`, que es quien le pasa al avatar el nombre y
	// la presencia; se le ponen a mano para que el retrato sea el de una app abierta y
	// no el de una barra a medio arrancar —tal cual, salía con «?» y sin punto—.
	b.cuenta.boton.avatar.Nombre = "Ada Lovelace"
	b.cuenta.boton.avatar.Presencia = EnLinea
	b.avisos.sinLeer = 3
	b.Resize(fyne.NewSize(anchoPrueba, AltoBarraSuite()))
	img := pintaA(t, 2, anchoPrueba, AltoBarraSuite()+20, b)

	ruta := os.Getenv("SYTUE_CAPTURA")
	if ruta == "" {
		return
	}
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestRetratoDelMenuDeCuenta guarda el menú ABIERTO, que es la pieza que el dueño
// enseñó: cabecera, presencia, apariencia, idioma, «Mi cuenta» y cerrar sesión. Se
// pinta el panel suelto porque la capa de superposición necesita una ventana de
// verdad y aquí no la hay. Con `SYTUE_CAPTURA_MENU=/ruta/menu.png`.
func TestRetratoDelMenuDeCuenta(t *testing.T) {
	b := NuevaBarraSuite(traductorPrueba(t), nil, iconos.Mail)
	b.cuenta.ficha = api.Profile{Name: "Ada Lovelace", Email: "ada@sytue.eu", ActiveOrg: "corporate"}
	b.cuenta.estado = sesion.Estado{Sujeto: "ada", Presencia: api.NoMolestar}

	m := NuevoMenu(b.cuenta.contenido()...)
	tam := MedidaMenu(m, fyne.NewSize(400, 600))
	m.Resize(tam)
	test.WidgetRenderer(m).Layout(tam)
	img := pintaA(t, 2, tam.Width+16, tam.Height+16, m)

	ruta := os.Getenv("SYTUE_CAPTURA_MENU")
	if ruta == "" {
		return
	}
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestElPuntoDelAvatarSigueALaPresencia es el fallo que ningún guardián veía: cambiar
// `Avatar.Presencia` y repintar dejaba el punto del color con el que nació.
//
// Se nota justo donde importa —el avatar de la barra, que vive toda la sesión— y no en
// una lista, donde cada fila construye el suyo. Se comprueba PINTANDO: el color del
// punto es el del token de la presencia nueva.
func TestElPuntoDelAvatarSigueALaPresencia(t *testing.T) {
	a := NuevoAvatar("Ada Lovelace").Con(AvatarMD)
	a.Presencia = Desconectado
	a.Resize(MedidaAvatar(AvatarMD))
	test.WidgetRenderer(a).Layout(MedidaAvatar(AvatarMD))

	if n := cuantos(pinta(t, 60, 60, a), tema.Color("presence-dnd", tema.Claro)); n != 0 {
		t.Fatalf("recién creado con «desconectado» ya hay %d píxeles de «no molestar»", n)
	}
	a.Presencia = NoMolestar
	a.Refresh()
	if n := cuantos(pinta(t, 60, 60, a), tema.Color("presence-dnd", tema.Claro)); n == 0 {
		t.Error("con «No molestar» puesto, el punto sigue del color anterior")
	}
}

// TestElPuntoAparecePorPrimeraVez es el SEGUNDO filo del mismo fallo: el punto solo
// existía si había presencia al construir el renderer, así que en una app que arranca
// sin sesión —las trece— la primera presencia que contestaba el Hub no pintaba nada.
//
// Aquí se construye el avatar y se PINTA sin presencia antes de ponerla: así el
// renderer ya está hecho cuando llega, que es justo el orden que fallaba.
func TestElPuntoAparecePorPrimeraVez(t *testing.T) {
	a := NuevoAvatar("Ada Lovelace").Con(AvatarMD)
	a.Resize(MedidaAvatar(AvatarMD))
	test.WidgetRenderer(a).Layout(MedidaAvatar(AvatarMD))

	verde := tema.Color("presence-online", tema.Claro)
	if n := cuantos(pinta(t, 60, 60, a), verde); n != 0 {
		t.Fatalf("sin presencia ya hay %d píxeles de punto", n)
	}
	a.Presencia = EnLinea
	a.Refresh()
	if n := cuantos(pinta(t, 60, 60, a), verde); n == 0 {
		t.Error("la presencia llegó después de construir el renderer y no hay punto que pintar")
	}
	// Y al revés: quitarla lo esconde, que es lo que pasa al cerrar sesión.
	a.Presencia = SinPresencia
	a.Refresh()
	if n := cuantos(pinta(t, 60, 60, a), verde); n != 0 {
		t.Errorf("sin presencia siguen pintados %d píxeles del punto", n)
	}
}

// TestUnaAppSeQuedaElAtajoDelBuscador: la paleta del ERP gana ⌘K como en la web, donde
// escuchan los dos y la paleta se queda el foco. Sin `CederAtajo`, el atajo lleva al
// buscador de la suite, que es lo de las otras doce apps.
//
// VISTO EN ROJO: con `pulsarAtajo` enfocando siempre el campo —sin mirar `alAtajo`—,
// la app no se entera y el foco acaba en el buscador.
func TestUnaAppSeQuedaElAtajoDelBuscador(t *testing.T) {
	b := barraPrueba(t)
	w := test.NewWindow(b)
	defer w.Close()
	c := w.Canvas()

	b.buscador.pulsarAtajo(c)
	if c.Focused() != b.buscador.campo {
		t.Fatal("sin nadie que se lo quede, ⌘K no lleva al buscador de la suite")
	}
	c.Unfocus()
	llamada := false
	b.CederAtajo(func() { llamada = true })
	b.buscador.pulsarAtajo(c)
	if !llamada || c.Focused() != nil {
		t.Errorf("cedido, ⌘K %s y el foco queda en %T", map[bool]string{true: "avisa a la app", false: "NO avisa a la app"}[llamada], c.Focused())
	}
}

// TestLosResultadosVanAgrupadosPorProducto es GlobalSearch.tsx: un grupo por producto,
// en el orden en que aparece cada uno, con su cabecera —la marca de 15 y el nombre en
// versalitas— y debajo sus filas de título y fragmento, sin marca ni meta por fila. Las
// flechas recorren el orden AGRUPADO. VISTO EN ROJO con las filas sueltas de antes: ni
// una cabecera de grupo, y la marca de 20 y el origen repetidos en cada fila.
func TestLosResultadosVanAgrupadosPorProducto(t *testing.T) {
	b := barraPrueba(t)
	hits := agruparPorOrigen([]api.HubHit{
		{Source: "mail", Title: "Factura de septiembre"},
		{Source: "drive", Title: "factura.pdf"},
		{Source: "mail", Title: "Re: factura", Snippet: "Te la reenvío"},
	})
	if hits[1].Title != "Re: factura" || hits[2].Source != "drive" {
		t.Fatalf("el orden agrupado es %v y tiene que ser mail, mail, drive", hits)
	}
	out := b.buscador.filas(hits, nil, 1, "factura")
	var tipos []string
	for _, o := range out {
		switch x := o.(type) {
		case *cabeceraGrupo:
			tipos = append(tipos, "grupo:"+x.origen)
		case *filaResultado:
			tipos = append(tipos, map[bool]string{true: "fila*", false: "fila"}[x.marcada])
		default:
			tipos = append(tipos, fmt.Sprintf("%T", o))
		}
	}
	if quiero := []string{"grupo:mail", "fila", "fila*", "grupo:drive", "fila"}; !slices.Equal(tipos, quiero) {
		t.Errorf("el panel pinta %v y tiene que pintar %v", tipos, quiero)
	}
	if got := out[0].MinSize().Height; got != tema.Espacio(1.5)+16+tema.Espacio(1) {
		t.Errorf("la primera cabecera de grupo mide %v: `first:pt-1.5`, la marca de 15 y `pb-1`", got)
	}
}

// TestRetratoDeLosResultados guarda el panel del buscador abierto con resultados de dos
// productos, para MIRARLO. Con `SYTUE_CAPTURA_BUSCADOR=/ruta/buscador.png`.
func TestRetratoDeLosResultados(t *testing.T) {
	ruta := os.Getenv("SYTUE_CAPTURA_BUSCADOR")
	if ruta == "" {
		t.Skip("sin SYTUE_CAPTURA_BUSCADOR no hay retrato que guardar")
	}
	b := barraPrueba(t)
	hits := agruparPorOrigen([]api.HubHit{
		{Source: "mail", Title: "Factura de septiembre", Snippet: "Adjunto la factura del mes pasado"},
		{Source: "drive", Title: "factura-2026-09.pdf", Snippet: "Mi unidad › Contabilidad"},
		{Source: "mail", Title: "Re: factura", Snippet: "Te la reenvío con el IVA corregido"},
	})
	b.buscador.panel.Poner(b.buscador.filas(hits, nil, 1, "factura"))
	b.buscador.panel.Show()
	test.WidgetRenderer(b).Layout(b.Size())
	img := pintaA(t, 2, anchoPrueba, 360, b)
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// luminancia y contraste son las de WCAG 2.x sobre sRGB.
func luminancia(c color.NRGBA) float64 {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func contraste(a, b color.NRGBA) float64 {
	la, lb := luminancia(a), luminancia(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// TestElPuntoDeAvisoContrastaConLaBarra: el punto de la campana es un indicador y WCAG
// 1.4.11 le pide 3:1 contra lo que tiene al lado, que es su anillo del color de la
// barra. Se mide con los colores PINTADOS, en los dos temas. VISTO EN ROJO con el
// `danger` de la web: 2,53:1 en claro.
func TestElPuntoDeAvisoContrastaConLaBarra(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, variante := range []string{"claro", "oscuro"} {
		tema.Elegir(variante)
		b := barraPrueba(t)
		b.avisos.sinLeer = 3
		b.Refresh()
		if c := contraste(b.avisos.punto.FillColor.(color.NRGBA), b.avisos.anillo.FillColor.(color.NRGBA)); c < 3 {
			t.Errorf("en %s el punto contrasta %.2f:1 con la barra y WCAG 1.4.11 pide 3:1", variante, c)
		}
	}
}

// TestPresenciaDeEsLaTablaDelMenu: quien pinta presencias fuera de la barra usa la
// misma tabla; un estado desconocido no es ninguno.
func TestPresenciaDeEsLaTablaDelMenu(t *testing.T) {
	if p, ok := PresenciaDe(api.NoMolestar); !ok || p != NoMolestar {
		t.Errorf("`busy` da %v (%v) y es No molestar", p, ok)
	}
	if _, ok := PresenciaDe(api.EstadoPresencia("rarísimo")); ok {
		t.Error("un estado que no existe da presencia")
	}
}

// TestLaBarraEstrechaEsCarrilEntera: por debajo de 768 el menú lateral va en un cajón y
// no se ve, así que la barra ENTERA es el carril, como en la web (`.sytue-topbar-carril`
// de appearance.css): el color de marca de lado a lado, sin línea de abajo, y el
// buscador en velo oscuro con el texto del carril. Al ensanchar vuelve a ser clara con
// su esquina de marca.
//
// VISTO EN ROJO sin el corte en `Layout`: a 360 la barra pinta `topbar-bg` a la derecha
// de la marca.
func TestLaBarraEstrechaEsCarrilEntera(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	const estrecha = 360
	for _, variante := range []string{"claro", "oscuro", "traslucido"} {
		tema.Elegir(variante)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		b := barraPruebaA(t, estrecha)
		b.avisos.sinLeer = 3
		b.Refresh()
		lienzo := canvas.NewRectangle(tema.Color("bg", v))
		lienzo.Resize(fyne.NewSize(estrecha, 120))
		img := pinta(t, estrecha, 120, lienzo, b)
		// Lo pintado, con el carril compuesto sobre el lienzo: en el traslúcido deja
		// pasar un punto.
		carril := tokens.SobreSRGB(tema.Color("sidebar-bg", v), tema.Color("bg", v))
		derecha, ultima := int((estrecha-2)*escala), int(AltoBarraSuite()*escala)-1
		if got := px(img, derecha, int(2*escala)); !cercaDe(got, carril, 2) {
			t.Errorf("en %s, a %d la barra pinta %v junto al borde derecho y es el carril, %v", variante, estrecha, got, carril)
		}
		if got := px(img, derecha, ultima); !cercaDe(got, carril, 2) {
			t.Errorf("en %s la barra de color lleva línea de abajo: su última fila pinta %v", variante, got)
		}
		// El buscador: velo oscuro neutro sobre el carril, no la superficie blanca.
		campo := b.buscador.campo
		p, s := campo.Position(), campo.Size()
		velo := tokens.SobreSRGB(tema.Color("sidebar-field", v), tema.Color("sidebar-bg", v))
		if got := px(img, int((p.X+s.Width-12)*escala), int((p.Y+s.Height/2)*escala)); !cercaDe(got, tokens.SobreSRGB(tema.Color("sidebar-field", v), carril), 3) {
			t.Errorf("en %s el buscador de la barra estrecha pinta %v y es el velo del carril, %v", variante, got, velo)
		}
		// Lo que se lee encima: el texto y la marca de agua del buscador, y el punto de
		// aviso contra su anillo, que es del color de la barra.
		for que, par := range map[string][2]color.NRGBA{
			"el texto del buscador":         {tema.Color("sidebar-fg", v), velo},
			"la marca de agua del buscador": {tokens.SobreSRGB(tema.Color("sidebar-fg-muted", v), velo), velo},
		} {
			if c := contraste(par[0], par[1]); c < 4.5 {
				t.Errorf("en %s %s contrasta %.2f:1 y el mínimo es 4,5:1", variante, que, c)
			}
		}
		anillo := tokens.SobreSRGB(b.avisos.anillo.FillColor.(color.NRGBA), tema.Color("bg", v))
		c := contraste(b.avisos.punto.FillColor.(color.NRGBA), anillo)
		t.Logf("en %s el punto de aviso contrasta %.2f:1 con la barra de color", variante, c)
		if c < 3 {
			t.Errorf("en %s el punto contrasta %.2f:1 con la barra de color y WCAG 1.4.11 pide 3:1", variante, c)
		}

		// Y de vuelta a ancho: la barra clara, con su esquina.
		ancha := fyne.NewSize(anchoPrueba, AltoBarraSuite())
		b.Resize(ancha)
		test.WidgetRenderer(b).Layout(ancha)
		img = pinta(t, anchoPrueba, 120, b)
		if got, quiero := px(img, int((tema.AnchoLateral+4)*escala), int(AltoBarraSuite()/2*escala)), tokens.SobreSRGB(tema.Color(fondoBarra, v), color.NRGBA{R: 255, G: 255, B: 255, A: 255}); !cercaDe(got, quiero, 2) {
			t.Errorf("en %s, al ensanchar, la barra pinta %v fuera de la esquina y es `%s`, %v", variante, got, fondoBarra, quiero)
		}
		if b.buscador.campo.EnCarril {
			t.Errorf("en %s, al ensanchar, el buscador sigue con los colores del carril", variante)
		}
	}
}

// TestBarraSuitePreguntaAntesDeSubirFicheroLocal (FYP-02): Montar registra el diálogo de
// confirmación con casilla «Recordar mi elección en este dispositivo», y pulsar el botón
// primario entrega el estado de la casilla.
func TestBarraSuitePreguntaAntesDeSubirFicheroLocal(t *testing.T) {
	w := fyne.CurrentApp().NewWindow("docus")
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(anchoPrueba, 600))
	tr := traductorPrueba(t)
	b := NuevaBarraSuite(tr, nil, iconos.Docs)
	t.Cleanup(b.Parar)
	w.SetContent(b.ConSinSesionPropia().ConHojaASangre().Montar(canvas.NewRectangle(color.NRGBA{})))

	var recordado *bool
	b.confirmarSubidaLocal(w, "nomina.pdf", func(r bool) {
		recordado = &r
	})
	capas := w.Canvas().Overlays().List()
	if len(capas) != 1 {
		t.Fatalf("capas abiertas = %d, quiero 1", len(capas))
	}
	cp, ok := capas[0].(*capa)
	if !ok {
		t.Fatalf("capa = %T, quiero *capa", capas[0])
	}
	d, ok := cp.panel.(*Dialogo)
	if !ok {
		t.Fatalf("panel = %T, quiero *Dialogo", cp.panel)
	}
	if d.Titulo != tr.T("localFile.confirmUpload.title", nil) {
		t.Errorf("título = %q, quiero %q", d.Titulo, tr.T("localFile.confirmUpload.title", nil))
	}
	col, ok := d.Cuerpo.(*fyne.Container)
	if !ok || len(col.Objects) != 2 {
		t.Fatalf("cuerpo = %T (%d hijos), quiero columna de 2", d.Cuerpo, len(col.Objects))
	}
	casilla, ok := col.Objects[1].(*Eleccion)
	if !ok {
		t.Fatalf("segundo hijo = %T, quiero *Eleccion", col.Objects[1])
	}
	casilla.Tapped(nil)
	if !casilla.Marcada {
		t.Fatal("la casilla de recordar debería haberse marcado al pulsarla")
	}
	d.confirmar.Tapped(nil)
	if recordado == nil || !*recordado {
		t.Fatalf("recordado = %v, quiero true", recordado)
	}
}
