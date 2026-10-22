// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"fmt"
	"image/color"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// enVentanaReal lo pone `ventanareal_test.go` (`-tags ventanareal`).
var enVentanaReal func(string) error

// TestMain arranca la aplicación de pruebas con NUESTRO tema. Sin él, `canvas.Text`
// resolvería la letra de serie y todas las medidas serían las de otra fuente.
//
// `test.NewApp()` y no `test.NewTempApp(t)`: el temporal se desmonta al acabar el test
// y deja sin tema a los que vengan después en otro fichero.
func TestMain(m *testing.M) {
	if dir := os.Getenv("SYTUE_VENTANA_REAL"); dir != "" && enVentanaReal != nil {
		if err := enVentanaReal(dir); err != nil {
			fmt.Println("VENTANA REAL: FALLO:", err)
			os.Exit(1)
		}
		fmt.Println("VENTANA REAL: bien")
		os.Exit(0)
	}
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	m.Run()
}

const (
	lienzoWeb  = "apps/frontend/packages/office/src/editor/sheets/SheetCanvas.tsx"
	rejillaWeb = "apps/frontend/packages/office/src/editor/sheets/rejilla.ts"
	hojaWeb    = "apps/frontend/packages/office/src/editor/editor.css"
)

// TestLaGeometriaSaleDeSheetCanvas compara las constantes de este paquete con las de
// la web, leyendo su fichero.
//
// Es la diferencia entre un espejo y una copia: un número escrito a mano que nadie
// contrasta se desincroniza en el primer cambio de la web y nadie se entera hasta que
// alguien compara capturas.
func TestLaGeometriaSaleDeSheetCanvas(t *testing.T) {
	// La rejilla de aquí es la MÍNIMA de la web: allí crece con los datos y el
	// desplazamiento hasta los límites de Excel, y aquí todavía no (deuda de paridad).
	rejilla := webref.Read(t, rejillaWeb)
	for _, c := range []struct {
		nombre  string
		nuestra float32
	}{
		{"MIN_COLS", Columnas},
		{"MIN_ROWS", Filas},
		{"MAX_COLS", MaxColumnas},
	} {
		if suya := constanteJSEn(t, rejilla, rejillaWeb, c.nombre); suya != c.nuestra {
			t.Errorf("%s vale %v en rejilla.ts y aquí %v", c.nombre, suya, c.nuestra)
		}
	}

	fuente := webref.Read(t, lienzoWeb)
	for _, c := range []struct {
		nombre  string
		nuestra float32
	}{
		{"HEAD_W", AnchoCabecera},
		{"CELL_W", AnchoCelda},
		{"CELL_H", AltoCelda},
		{"OUTLINE_STEP", PasoEsquema},
		{"OUTLINE_PAD", AireEsquema},
	} {
		if suya := constanteJS(t, fuente, c.nombre); suya != c.nuestra {
			t.Errorf("%s vale %v en SheetCanvas.tsx y aquí %v", c.nombre, suya, c.nuestra)
		}
	}
}

// TestLasClasesDeLaRejillaSiguenSiendoLasMismas vigila lo que NO es una constante: las
// clases de Tailwind de la celda, de la cabecera y de la barra de fórmulas, que es de
// donde salen el relleno, el escalón de letra y la caja de línea.
//
// Compara cadenas literales a propósito. Es tosco y es la gracia: si alguien cambia
// `text-sm` por `text-xs` en la celda, este test lo dice, y ninguna medida derivada lo
// haría — el alto de la celda seguiría siendo 24.
func TestLasClasesDeLaRejillaSiguenSiendoLasMismas(t *testing.T) {
	fuente := webref.Read(t, lienzoWeb)
	for _, c := range []struct {
		trozo, porQue string
	}{
		{
			"h-[24px] shrink-0 select-none border-b border-r border-border text-center text-xs leading-[24px]",
			"la cabecera: alto de celda, `text-xs` y caja de línea de 24",
		},
		{
			"h-[24px] w-[96px] shrink-0 overflow-hidden border-r border-b border-[var(--editor-grid-line)]",
			"la celda: su caja, el recorte y la línea de la cuadrícula",
		},
		{"px-1 text-sm leading-[24px] whitespace-nowrap", "la celda: `px-1`, `text-sm` y caja de línea de 24"},
		{"w-[96px] cursor-cell", "la cabecera de columna mide lo que la celda"},
		{"w-[40px] cursor-cell", "la cabecera de fila mide `HEAD_W`"},
		{"sytue-grid-head-hl", "el realce de la cabecera dentro del rango"},
		{"flex h-8 shrink-0 items-center gap-2 border-b px-2", "la barra de fórmulas: `h-8`, `gap-2`, `px-2`"},
		{"h-6 w-16 shrink-0 items-center justify-center rounded-sm border text-xs font-medium tabular-nums", "el rótulo de la celda activa: `w-16` y `text-xs`"},
		{"h-5 w-full min-w-0 rounded-sm bg-transparent px-1 text-sm", "el campo de la fórmula"},
		{"inset 0 0 0 2px", "el marco de la celda ancla son 2 px por dentro"},
		{"color-mix(in srgb, ${ACCENT} 12%", "el rango mezcla editor-select al 12 %"},
	} {
		if !strings.Contains(fuente, c.trozo) {
			t.Errorf("SheetCanvas.tsx ya no contiene %q (%s): revisad la medida que lo copia", c.trozo, c.porQue)
		}
	}
}

// TestLosColoresDelEditorSonLosDeSuHoja comprueba el papel, la línea de la cuadrícula
// y el fondo del lienzo contra `editor.css`, que es donde viven: NO son tokens de
// tema, y por eso no los puede vigilar el guardián de `apps/nucleo/tokens`.
//
// La línea se compara ya aplanada sobre el papel, que es lo que este paquete guarda.
func TestLosColoresDelEditorSonLosDeSuHoja(t *testing.T) {
	hoja := webref.Read(t, hojaWeb)

	if got := hexCSS(t, hoja, "--editor-page-bg"); got != Papel {
		t.Errorf("`--editor-page-bg` es %v y Papel es %v", got, Papel)
	}
	superficie := webref.Read(t, "apps/frontend/packages/office/src/editor/canvas/superficies.css")
	for _, token := range []string{"--editor-canvas-bg: var(--sytue-neutral-150)", "--editor-canvas-bg: var(--sytue-surface-sunken)", "--editor-select: var(--sytue-primary-500)"} {
		if !strings.Contains(superficie, token) {
			t.Errorf("la superficie ya no contiene %q", token)
		}
	}
	if got := FondoLienzo(tema.Claro); got != tema.Color("neutral-150", tema.Claro) {
		t.Errorf("el lienzo claro no usa neutral-150: %v", got)
	}
	if got := FondoLienzo(tema.Oscuro); got != tema.Color("surface-sunken", tema.Oscuro) {
		t.Errorf("el lienzo oscuro no usa surface-sunken: %v", got)
	}
	acabado := webref.Read(t, "apps/frontend/packages/office/src/editor/sheets/superficie-hoja.css")
	if !strings.Contains(acabado, "var(--sytue-accent-800) 17%") {
		t.Fatal("la línea de cuadrícula ya no es marina al 17 %")
	}
	quiero := tokens.Aplanar(tema.Color("accent-800", tema.Claro), 0.17, Papel)
	if LineaRejilla != quiero {
		t.Errorf("la línea de cuadrícula es %v, se esperaba %v", LineaRejilla, quiero)
	}
	// Los píxeles independientes de rejilla_test y estilo_test salen de estos
	// valores de CSS; no se calculan con los helpers del pintor que comprueban.
	paleta := webref.Read(t, "branding/tokens/colors.css")
	for _, c := range []struct {
		nombre string
		color  color.NRGBA
	}{
		{"--sytue-primary-500", color.NRGBA{R: 26, G: 154, B: 163, A: 255}},
		{"--sytue-accent-800", color.NRGBA{R: 26, G: 53, B: 65, A: 255}},
		{"--sytue-bg-subtle", color.NRGBA{R: 232, G: 239, B: 239, A: 255}},
	} {
		if got := hexCSS(t, paleta, c.nombre); got != c.color {
			t.Errorf("%s es %v: revisar los píxeles de Calcus derivados de %v", c.nombre, got, c.color)
		}
	}
	for _, trozo := range []string{"var(--editor-select) var(--sytue-selected-strength)", "var(--sytue-bg-subtle)", "inset 0 -2px 0 var(--editor-select)", "inset -2px 0 0 var(--editor-select)"} {
		if !strings.Contains(acabado, trozo) {
			t.Errorf("superficie-hoja.css ya no contiene %q: revisar el realce de las cabeceras", trozo)
		}
	}
	if !strings.Contains(paleta, "--sytue-selected-strength: 12%") {
		t.Error("la cabecera clara ya no mezcla la selección al 12 %")
	}
}

// TestLosEscalonesLlevanLaCajaDeLineaDeLaClase es la trampa que más se cuela al
// portar: `text-sm` trae SU caja de línea (1,125rem = 18), pero la celda la pisa
// con `leading-[24px]`. Usar `tema.TextoSM` tal cual dejaría cada fila 6 px más
// baja que en la web y el error se acumularía fila a fila.
func TestLosEscalonesLlevanLaCajaDeLineaDeLaClase(t *testing.T) {
	if EscalonCelda.Tam != tema.TextoSM.Tam {
		t.Errorf("la celda es `text-sm` (%v) y aquí mide %v", tema.TextoSM.Tam, EscalonCelda.Tam)
	}
	if EscalonCelda.Linea != AltoCelda {
		t.Errorf("la celda lleva `leading-[24px]` y aquí la caja de línea es %v", EscalonCelda.Linea)
	}
	if EscalonCelda.Linea == tema.TextoSM.Linea {
		t.Error("la caja de línea de la celda ha vuelto a ser la de `text-sm`: `leading-[24px]` la pisa")
	}
	if EscalonCabecera.Tam != tema.TextoXS.Tam || EscalonCabecera.Linea != AltoCelda {
		t.Errorf("la cabecera es `text-xs leading-[24px]` y aquí es %v", EscalonCabecera)
	}

	// Y el caso contrario, que es el que se falló: el `<input>` de la fórmula NO lleva
	// clase de interlineado, así que ahí manda el escalón entero. El `leading-5` que
	// se le copió es del div de detrás —el que tinta las referencias—, y con él la
	// letra quedaría 2 px más abajo de donde la pone el navegador.
	//
	// theme.css fija `text-sm` en 0,8125rem/1,125rem: con raíz de 16 son 13px/18px.
	if EscalonFx != tema.TextoSM {
		t.Errorf("el campo de la fórmula es `text-sm` sin interlineado (%v) y aquí es %v", tema.TextoSM, EscalonFx)
	}
	fuente := webref.Read(t, lienzoWeb)
	if regexp.MustCompile(`z-\[1\] h-5 [^"]*leading-`).MatchString(fuente) {
		t.Error("el input de la fórmula ha ganado una clase `leading-`: EscalonFx vuelve a tener caja de línea propia")
	}
}

// TestLasMedidasEnRemSalenDelRemDe16 fija los números que la web escribe como clases
// de Tailwind de SheetCanvas.tsx, con raíz de 16 px en theme.css.
func TestLasMedidasEnRemSalenDelRemDe16(t *testing.T) {
	for _, c := range []struct {
		clase           string
		nuestra, quiero float32
	}{
		{"px-1 de la celda", MargenCelda, 4},
		{"h-8 de la barra", AltoBarra, 32},
		{"w-16 del rótulo", AnchoRef, 64},
		{"px-2 de la barra", MargenBarra, 8},
		{"h-5 del campo", AltoEntradaFx, 20},
	} {
		if c.nuestra != c.quiero {
			t.Errorf("`%s` con el rem de 16 son %v y aquí %v", c.clase, c.quiero, c.nuestra)
		}
	}
}

// constanteJS lee `const NOMBRE = 42;` del fuente de la web.
func constanteJS(t *testing.T, fuente, nombre string) float32 {
	t.Helper()
	return constanteJSEn(t, fuente, lienzoWeb, nombre)
}

// constanteJSEn es constanteJS sobre otro fichero de la web, para el mensaje de error.
func constanteJSEn(t *testing.T, fuente, ruta, nombre string) float32 {
	t.Helper()
	m := regexp.MustCompile(fmt.Sprintf(`const %s\s*=\s*([\d_]+)`, nombre)).FindStringSubmatch(fuente)
	if m == nil {
		t.Fatalf("%s ya no declara `const %s`: la geometría se ha movido de sitio", ruta, nombre)
	}
	v, err := strconv.Atoi(strings.ReplaceAll(m[1], "_", ""))
	if err != nil {
		t.Fatal(err)
	}
	return float32(v)
}

// hexCSS lee `--nombre: #rrggbb;` de la hoja.
func hexCSS(t *testing.T, hoja, nombre string) color.NRGBA {
	t.Helper()
	m := regexp.MustCompile(nombre + `:\s*#([0-9a-fA-F]{6})`).FindStringSubmatch(hoja)
	if m == nil {
		t.Fatalf("`%s` ya no es un hex en %s", nombre, hojaWeb)
	}
	v, err := strconv.ParseUint(m[1], 16, 32)
	if err != nil {
		t.Fatal(err)
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}
