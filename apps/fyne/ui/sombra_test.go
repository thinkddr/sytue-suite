// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// El panel de prueba y el hueco que se le deja alrededor para que la sombra quepa
// entera. 120 da de sobra para el nivel más grande (`xl`: 3σ = 72 más 24 de
// desplazamiento).
var (
	panelPrueba = fyne.NewSize(200, 120)
	huecoPrueba = float32(120)
)

// alfaDeSombra despeja el alfa REAL de la sombra a partir del gris pintado sobre blanco.
//
// No vale leer `1 − v/255`: la sombra de la hoja no es negra, es marina (17,35,43). Un
// píxel a alfa `a` sale en `v = c·a + 255·(1−a)`, así que el alfa se despeja con el
// canal y el color del token. Medirlo como si fuese negro da un 7 % de menos y hace
// fallar un guardián que debería estar verde.
func alfaDeSombra(v uint32, canal uint8) float64 {
	return (255 - float64(v>>8)) / (255 - float64(canal))
}

// perfilDeSombra baja por el centro del panel desde su borde inferior y devuelve el alfa
// de la sombra en cada paso, junto con el paso en px de CSS.
func perfilDeSombra(t *testing.T, n tema.NivelSombra, radio, esc float32, v tema.Variante) []float64 {
	t.Helper()

	s := NuevaSombra(n, radio)
	s.Ajustar(fyne.NewPos(huecoPrueba, huecoPrueba), panelPrueba, v)

	img := pintaA(t, esc,
		panelPrueba.Width+2*huecoPrueba, panelPrueba.Height+2*huecoPrueba, s.Objeto())

	canal := tema.Sombra(n, v)[0].Color.R
	x := int((huecoPrueba + panelPrueba.Width/2) * esc)
	base := int((huecoPrueba + panelPrueba.Height) * esc)

	var prof []float64
	for d := 0; base+d < img.Bounds().Dy(); d++ {
		r, _, _, _ := img.At(x, base+d).RGBA()
		prof = append(prof, alfaDeSombra(r, canal))
	}
	return prof
}

// cruce devuelve a cuántos px de CSS del principio el perfil baja de `nivel`.
func cruce(prof []float64, nivel float64, esc float32) float64 {
	for i, a := range prof {
		if a <= nivel {
			return float64(i) / float64(esc)
		}
	}
	return -1
}

// TestLaSombraTieneElDesenfoqueYElDesplazamientoDeLaHoja es EL guardián de las sombras.
//
// Comprueba contra los números de `theme.css`, no contra los que calcule esta
// implementación, las dos cosas que definen una sombra:
//
//   - **El desplazamiento.** Una gaussiana vale exactamente la mitad justo encima del
//     borde que difumina, así que el punto donde el alfa cruza el 50 % del token TIENE
//     que caer `DesplY` px por debajo del panel: 24 en `xl`, 12 en `lg`, 8 en `md`.
//   - **El desenfoque.** CSS pinta un `blur-radius` de r con una gaussiana de desviación
//     r/2, así que de ese cruce al 15,87 % —una desviación típica— hay que contar 24 px
//     en `xl`, 16 en `lg` y 12 en `md`.
//
// La tolerancia del σ es del 10 %: tres desenfoques de caja son una aproximación —la
// misma que usan los navegadores— y las cajas son de ancho entero.
//
// # Contrastado contra un navegador de verdad
//
// El 2026-09-21, con Chromium a ×2 pintando ESTA misma escena con el `box-shadow`
// literal de la hoja, y midiendo el perfil con este mismo método:
//
//	nivel   σ Chromium   σ nuestra   σ que pide CSS
//	xl          24,50       24,00        24,0
//	lg          14,00       15,00        16,0
//	md          12,00       12,00        12,0
//
// Dos cosas que conviene saber antes de apretar esta tolerancia: la nuestra se separa
// del navegador entre un 0 y un 7 %, y el propio Chromium **no clava la fórmula del
// estándar** —en `lg` pinta 14,0 donde r/2 son 16,0—. O sea que exigir aquí menos de un
// 10 % sería exigir más parecido con la fórmula que el que tiene la referencia.
func TestLaSombraTieneElDesenfoqueYElDesplazamientoDeLaHoja(t *testing.T) {
	casos := []struct {
		nombre string
		nivel  tema.NivelSombra
		radio  float32
	}{
		{"shadow-xl del diálogo", tema.SombraXL, tema.RadioXL},
		{"shadow-lg del aviso", tema.SombraLG, tema.RadioLG},
		{"shadow-md del menú", tema.SombraMD, tema.RadioLG},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			// Se mide geometría en oscuro: el alfa claro .12 se cuantiza en pocos niveles
			// y desplaza los cruces hasta 2px. La hoja mantiene la geometría en ambos
			// temas; tema.TestLasSombrasSonLasDeLaWeb vigila sus capas.
			// Los números que se exigen salen de la HOJA, por el mismo camino que los
			// usaría un widget, no de la imagen que se está midiendo.
			capa := tema.Sombra(c.nivel, tema.Oscuro)[0]
			lleno := float64(capa.Color.A) / 255

			prof := perfilDeSombra(t, c.nivel, c.radio, escala, tema.Oscuro)

			mitad := cruce(prof, 0.5*lleno, escala)
			if mitad < 0 {
				t.Fatalf("el perfil nunca baja del 50%% del token: la sombra no se desvanece")
			}
			if d := math.Abs(mitad - float64(capa.DesplY)); d > 1.5 {
				t.Errorf("el alfa cae a la mitad a %.2f px del panel y theme.css desplaza %v: sobran %.2f",
					mitad, capa.DesplY, d)
			}

			unaSigma := cruce(prof, 0.1587*lleno, escala)
			if unaSigma < 0 {
				t.Fatalf("el perfil no llega al 15,87%% del token dentro de la imagen")
			}
			sigma := unaSigma - mitad
			quiero := float64(capa.Desviacion())
			if math.Abs(sigma-quiero)/quiero > 0.10 {
				t.Errorf("σ pintada %.2f px y CSS pide %.1f (blur %v entre 2): %.0f%% de error",
					sigma, quiero, capa.Desenfoque, math.Abs(sigma-quiero)/quiero*100)
			}
		})
	}
}

// TestLaSombraNoCambiaDeTamanoConLaEscala es la razón por la que esto NO usa
// `canvas.Rectangle.Shadow`, convertida en guardián.
//
// La sombra de serie de Fyne encoge cuando sube la densidad —medido con el pintor de
// software y BlurRadius 24: σ de 7,00 px a ×1, 5,00 a ×2 y 4,00 a ×3— porque multiplica
// por la escala un parámetro que en realidad es σ²/2. La nuestra se declara en px de CSS
// y tiene que medir lo mismo en las tres.
func TestLaSombraNoCambiaDeTamanoConLaEscala(t *testing.T) {
	capa := tema.Sombra(tema.SombraMD, tema.Claro)[0]
	lleno := float64(capa.Color.A) / 255

	escalas := []float32{1, 2, 3}
	sigmas := make([]float64, 0, len(escalas))
	for _, esc := range escalas {
		prof := perfilDeSombra(t, tema.SombraMD, tema.RadioLG, esc, tema.Claro)
		mitad := cruce(prof, 0.5*lleno, esc)
		unaSigma := cruce(prof, 0.1587*lleno, esc)
		if mitad < 0 || unaSigma < 0 {
			t.Fatalf("a ×%v el perfil no cruza: mitad=%v sigma=%v", esc, mitad, unaSigma)
		}
		sigmas = append(sigmas, unaSigma-mitad)
	}

	// A ×1 el perfil se lee en pasos de 1 px, así que no se le puede pedir más
	// precisión que eso; a ×2 y ×3, mejor.
	for i, s := range sigmas[1:] {
		if math.Abs(s-sigmas[0]) > 1.01 {
			t.Errorf("σ a ×%d es %.2f y a ×1 es %.2f: la sombra cambia de tamaño con el DPI",
				i+2, s, sigmas[0])
		}
	}
}

// TestLaSombraSalePorFueraDelPanel comprueba el cableado, que es lo que un perfil no ve:
// que la imagen es MÁS GRANDE que el panel y empieza antes que él.
//
// Sin esto se puede tener una sombra perfecta metida dentro del panel, que es justo
// donde el `IsClip` del renderer se la come entera.
func TestLaSombraSalePorFueraDelPanel(t *testing.T) {
	s := NuevaSombra(tema.SombraXL, tema.RadioXL)
	s.Ajustar(fyne.NewPos(huecoPrueba, huecoPrueba), panelPrueba, tema.Claro)

	caja, pos := s.Objeto().Size(), s.Objeto().Position()
	if caja.Width <= panelPrueba.Width || caja.Height <= panelPrueba.Height {
		t.Errorf("la imagen de la sombra mide %v y el panel %v: no sobresale", caja, panelPrueba)
	}
	if pos.X >= huecoPrueba || pos.Y >= huecoPrueba {
		t.Errorf("la sombra empieza en %v y el panel en (%v,%v): no está por fuera",
			pos, huecoPrueba, huecoPrueba)
	}
}

// TestElTemaOscuroTieneSuPropiaSombra: la hoja cambia el color y sube el alfa en oscuro,
// con la misma geometría — y deja `xs` SIN redefinir, así que ese sigue siendo el marina
// del claro. Las tres cosas son intencionadas y se fijan aquí.
func TestElTemaOscuroTieneSuPropiaSombra(t *testing.T) {
	claro := tema.Sombra(tema.SombraXL, tema.Claro)[0]
	oscuro := tema.Sombra(tema.SombraXL, tema.Oscuro)[0]

	if oscuro.Color.R != 0 || oscuro.Color.G != 0 || oscuro.Color.B != 0 {
		t.Errorf("en oscuro theme.css pone rgba(0,0,0,·) y sale %v", oscuro.Color)
	}
	if oscuro.Color.A <= claro.Color.A {
		t.Errorf("en oscuro el alfa sube (0,6 contra 0,18) y sale %d contra %d",
			oscuro.Color.A, claro.Color.A)
	}
	if oscuro.DesplY != claro.DesplY || oscuro.Desenfoque != claro.Desenfoque {
		t.Errorf("la geometría es la MISMA en los dos temas: claro %v/%v, oscuro %v/%v",
			claro.DesplY, claro.Desenfoque, oscuro.DesplY, oscuro.Desenfoque)
	}

	// `xs` no se redefine en oscuro: hereda el marina del claro.
	if tema.Sombra(tema.SombraXS, tema.Oscuro)[0] != tema.Sombra(tema.SombraXS, tema.Claro)[0] {
		t.Error("theme.css no redefine --shadow-xs en oscuro, así que tiene que heredar el del claro")
	}
}

// TestDesenfocarImagenEsLaGaussianaDeCSS: un borde blanco-negro desenfocado con σ = 12
// queda a la mitad justo en el borde y, a una σ de distancia, en Φ(1) = 84 % hacia el
// lado de su color, que es lo que pinta un `blur(12px)`. Y un plano uniforme no cambia.
func TestDesenfocarImagenEsLaGaussianaDeCSS(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 200, 4))
	for y := range 4 {
		for x := range 200 {
			v := uint8(0)
			if x >= 100 {
				v = 255
			}
			src.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	out := DesenfocarImagen(src, 12)
	for _, c := range []struct {
		x    int
		want float64
	}{{100, 0.5}, {112, 0.84}, {88, 0.16}, {5, 0}, {195, 1}} {
		// El centro del píxel x es x+0,5: el borde cae entre 99 y 100.
		got := float64(out.NRGBAAt(c.x, 2).R) / 255
		if math.Abs(got-c.want) > 0.04 {
			t.Errorf("en x=%d: %.3f, quería %.2f", c.x, got, c.want)
		}
	}
	if a := out.NRGBAAt(100, 2).A; a != 255 {
		t.Errorf("el desenfoque sale con alfa %d", a)
	}
}

// TestAjustarNoEnciendeLaSombraQueSeEscondio: la visibilidad es de quien la lleva. Antes
// Ajustar la encendía siempre, y la barra de acciones de Rooms, apagada, dejaba su
// sombra junto a cada mensaje en una ventana real. VISTO EN ROJO con el `Show()` de
// siempre.
func TestAjustarNoEnciendeLaSombraQueSeEscondio(t *testing.T) {
	s := NuevaSombra(tema.SombraSM, tema.RadioMD)
	tam := fyne.NewSize(120, 30)
	s.Objeto().Hide()
	s.Ajustar(fyne.NewPos(0, 0), tam, tema.Claro)
	if s.Objeto().Visible() {
		t.Error("Ajustar ha encendido una sombra que su dueño había escondido")
	}
	// La que escondió ella por falta de tamaño sí la vuelve a enseñar.
	s.Objeto().Show()
	s.Ajustar(fyne.NewPos(0, 0), fyne.NewSize(0, 0), tema.Claro)
	s.Ajustar(fyne.NewPos(0, 0), tam, tema.Claro)
	if !s.Objeto().Visible() {
		t.Error("tras tener tamaño otra vez, la sombra sigue escondida")
	}
}
