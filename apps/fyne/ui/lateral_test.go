// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Los números de este fichero son los MEDIDOS en el DOM de producción
// (corporate.sytue.eu, 1280×800, rem 14), no los que calcula este paquete. Un guardián
// que compare la implementación consigo misma pasa siempre; y uno que compare anchos
// RELATIVOS al texto también, aunque el texto salga un 6 % más ancho y la caja 4 px de
// más, que es como se coló ese fallo la vez anterior.

// Lo medido en `drive/archivos.ancho.json`, fila «Mi unidad» del menú:
//
//	alto 33,25 · ancho 231 · padding 7px 8.75px · radio 6 · fuente 12,25/19,25 peso 500
//
// El ancho útil ganó un píxel con la forma Carril: el menú ya no lleva filete a la
// derecha (Sidebar.tsx), así que son los 260 menos el canalón de los dos lados.
const (
	altoFilaWeb  = 38
	anchoUtilWeb = 228
	// El carril entero, del esqueleto de la misma captura: 260×744 en (0,56).
	anchoCarrilWeb = 260
)

// iconoFalso es un cuadrado que hace de glifo: los widgets de navegación no traen
// juego de iconos, lo recibe cada pantalla.
func iconoFalso() fyne.CanvasObject {
	return canvas.NewRectangle(color.NRGBA{R: 255, A: 255})
}

// TestLaFilaDelMenuMide33Coma25 es el guardián de la pieza más repetida del cromo:
// seis aplicaciones montan este menú y la fila es la unidad con la que se construye.
//
// Comprueba lo declarado contra la web, lo declarado contra `MinSize` y lo declarado
// contra los píxeles.
func TestLaFilaDelMenuMide33Coma25(t *testing.T) {
	f := NuevaFilaLateral("Mi unidad", iconoFalso(), nil)
	declarada := MedidaFilaLateral(f)
	if declarada.Height != altoFilaWeb {
		t.Errorf("la fila dice medir %v de alto y en producción mide %v", declarada.Height, float32(altoFilaWeb))
	}
	if got := f.MinSize(); got != declarada {
		t.Errorf("MinSize() = %v y MedidaFilaLateral dice %v: hay dos cuentas donde tiene que haber una", got, declarada)
	}

	// Y lo pintado, con la fila activa, que es la que tiene fondo opaco que rastrear.
	f.Activa = true
	f.Refresh()
	caj := fyne.NewSize(anchoUtilWeb, declarada.Height)
	f.Resize(caj)
	fondo := canvas.NewRectangle(tema.Color("sidebar-bg", tema.Claro))
	fondo.Resize(fyne.NewSize(caj.Width+20, caj.Height+20))
	img := pinta(t, caj.Width+20, caj.Height+20, fondo, f)
	x0, x1, y0, y1 := caja(img, tokens.SobreSRGB(tema.Color("sidebar-active-bg", tema.Claro), tema.Color("sidebar-bg", tema.Claro)))
	mismaCaja(t, "la fila activa", caj, x0, x1, y0, y1)
}

// TestElMenuEsUnCarrilDeColor ata la forma a lo que se ve y no a un valor de la hoja,
// que es del eje de color y cambia con él: el fondo del menú NO es el de la barra ni el
// del contenido, el ítem activo es un VELO sobre ese fondo (translúcido en la hoja) y lo
// que se lee encima —rótulo en reposo, activo y la barra de «estás aquí»— contrasta con
// él en los tres modos.
//
// VISTO EN ROJO leyendo solo colors.css, donde el menú sigue claro: el fondo del menú
// es el de la barra.
func TestElMenuEsUnCarrilDeColor(t *testing.T) {
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro, tema.Translucido} {
		// Lo que se ve: en el traslúcido el carril deja pasar un punto del lienzo.
		carril := tokens.SobreSRGB(tema.Color("sidebar-bg", v), tema.Color("bg", v))
		if carril == tokens.SobreSRGB(tema.Color(fondoBarra, v), tema.Color("bg", v)) {
			t.Errorf("modo %d: el menú pinta %v, lo mismo que la barra: no hay carril", v, carril)
		}
		if tema.Color("sidebar-active-bg", v).A == 0xff {
			t.Errorf("modo %d: el ítem activo no es un velo sobre el carril", v)
		}
		activo := tokens.SobreSRGB(tema.Color("sidebar-active-bg", v), carril)
		for _, c := range []struct {
			que          string
			tinta, fondo color.NRGBA
			minimo       float64
		}{
			{"el rótulo en reposo", tokens.SobreSRGB(tema.Color("sidebar-fg-muted", v), carril), carril, 4.5},
			{"el rótulo al pasar", tokens.SobreSRGB(tema.Color("sidebar-fg", v), tokens.SobreSRGB(tema.Color("sidebar-hover-bg", v), carril)), tokens.SobreSRGB(tema.Color("sidebar-hover-bg", v), carril), 4.5},
			{"el rótulo activo", tema.Color("sidebar-active-fg", v), activo, 4.5},
			{"la barra de «estás aquí»", tema.Color("sidebar-indicator", v), activo, 3},
			{"el aro de foco", tema.Color("sidebar-focus", v), carril, 3},
			{"el botón principal", tema.Color("sidebar-primary", v), carril, 3},
			{"el rótulo del botón principal", tema.Color("sidebar-on-primary", v), tema.Color("sidebar-primary", v), 4.5},
		} {
			got := contraste(c.tinta, c.fondo)
			t.Logf("modo %d: %s, %.2f:1", v, c.que, got)
			if got < c.minimo {
				t.Errorf("modo %d: %s contrasta %.2f:1 y el mínimo es %v:1", v, c.que, got, c.minimo)
			}
		}
	}
}

// TestLaFilaActivaLlevaLaBarraDeSenal es el «estás aquí» de `docs/design.md` §3.2:
// `before:top-2 before:bottom-2 before:left-0 before:w-[3px]`, en el ámbar del carril
// (`--sytue-sidebar-indicator`).
//
// Sin ella el velo del ítem activo solo se distingue del carril por un paso de tono, que
// es exactamente el defecto que la barra vino a arreglar. VISTO EN ROJO con el
// `indicator` del tema, que es el del contenido: no hay un píxel ámbar en la fila.
func TestLaFilaActivaLlevaLaBarraDeSenal(t *testing.T) {
	f := NuevaFilaLateral("Recibidos", iconoFalso(), nil)
	f.Activa = true
	m := MedidaFilaLateral(f)
	m.Width = anchoUtilWeb
	f.Resize(m)
	img := pinta(t, m.Width+10, m.Height+10, f)

	x0, x1, y0, y1 := caja(img, tema.Color("sidebar-indicator", tema.Claro))
	if x0 != 0 {
		t.Errorf("la barra de señal empieza en x=%d y `left-0` es el borde de la fila", x0)
	}
	if ancho := float32(x1-x0+1) / escala; ancho != AnchoSenal() || ancho != 3 {
		t.Errorf("la barra mide %v de ancho y en Carril `w-[var(--sytue-nav-indicator)]` son 3 (%v)", ancho, AnchoSenal())
	}
	// Un píxel de CSS de margen y no medio: la barra es `rounded-full` y las esquinas
	// redondeadas de Fyne SÍ llevan antialias (los lados rectos no), así que sus filas
	// extremas salen a media cobertura y el rastreo por color no las cuenta.
	alto := float32(y1-y0+1) / escala
	if quiero := m.Height - 2*tema.Espacio(2); alto < quiero-1 || alto > quiero+1 {
		t.Errorf("la barra mide %v de alto y con `top-2 bottom-2` sobre %v son %v", alto, m.Height, quiero)
	}
}

// TestLaFilaCompactaMide28Coma5 es la fila del PIE del menú —«Ver estos archivos en tu
// ordenador», el medidor de espacio—, medida en producción en Sytue Drive.
//
// Los 28,5 son la prueba de que en esa fila el glifo va SIN la caja de `h-5 w-5`: con
// ella el alto lo pondría el texto (15,75 + 10,5 = 26,25) y no el icono (18 + 10,5).
// Es la diferencia que separa visualmente el pie de la lista de destinos de arriba.
func TestLaFilaCompactaMide28Coma5(t *testing.T) {
	f := NuevaFilaLateral("Abrir en el ordenador", iconoFalso(), nil)
	f.Compacta = true
	if got := MedidaFilaLateral(f).Height; got != 30 {
		t.Errorf("la fila compacta dice medir %v y en producción mide 28,5", got)
	}
	// Y sin icono manda el texto: `text-sm` (15,75) más `py-1.5` (5,25 × 2).
	f.Icono = nil
	if got := MedidaFilaLateral(f).Height; got != 30 {
		t.Errorf("la fila compacta sin icono mide %v y su texto con `py-1.5` son 26,25", got)
	}
}

// TestElCarrilMide260YDejaSuCanalon es el esqueleto: el `<nav>` ocupa 260 y, sin filete
// a la derecha, con el canalón de `px-4` deja los 260 menos dos canalones de ancho útil.
// Mientras el menú llevó su filete era un píxel menos.
func TestElCarrilMide260YDejaSuCanalon(t *testing.T) {
	fila := NuevaFilaLateral("Mi unidad", iconoFalso(), nil)
	seccion := NuevaSeccionLateral("Carpetas", fila)
	menu := NuevoLateral("Navegación principal", seccion)

	if got := MedidaLateral(menu).Width; got != anchoCarrilWeb {
		t.Errorf("el carril mide %v y en producción mide %v", got, float32(anchoCarrilWeb))
	}
	menu.Resize(fyne.NewSize(anchoCarrilWeb, 400))
	_ = test.WidgetRenderer(menu) // fuerza el Layout
	menu.Refresh()
	pinta(t, anchoCarrilWeb+10, 410, menu)

	if got := fila.Size().Width; got != anchoUtilWeb {
		t.Errorf("la fila recibe %v de ancho dentro del carril y en producción mide %v", got, float32(anchoUtilWeb))
	}
	if got := seccion.Position().X; got != tema.Espacio(4) {
		t.Errorf("el cuerpo empieza en x=%v y `px-4` con el rem de 14 son %v", got, tema.Espacio(4))
	}
	// Plegado, el carril es el token de 64.
	menu.Plegado = true
	if got := MedidaLateral(menu).Width; got != 64 {
		t.Errorf("el carril plegado mide %v y `--spacing-sidebar-collapsed` son 64", got)
	}
}

// TestElEncabezadoDeSeccionCreceConSusAcciones fija las DOS alturas que el encabezado
// de grupo tiene en producción, y las dos están medidas:
//
//	22,75  «Ubicaciones» (Drive), «Vistas» y «Libretas» (Contacts): solo el rótulo
//	32,75  «Carpetas» y «Etiquetas» (Mail), «Mis calendarios», «Cuadernos» (Notepads):
//	       con acciones, que miden 24 y son más altas que las versalitas
//
// Las dos salen de `pt-1 pb-1.5` sobre el elemento más alto de la línea. El botón era
// de 21 (`h-6 w-6` con el rem de 14, y la segunda medía 29,75) hasta que la web le puso
// el suelo de 24 px (PR #834): el encabezado con acciones crece los 3 px.
func TestElEncabezadoDeSeccionCreceConSusAcciones(t *testing.T) {
	s := NuevaSeccionLateral("Carpetas")
	if got := AltoEncabezadoSeccion(s); got != 26 {
		t.Errorf("el encabezado mide %v y en producción mide 22,75", got)
	}
	s.Acciones = []fyne.CanvasObject{NuevaAccionLateral("Gestionar carpetas", iconoFalso(), nil)}
	if got := AltoEncabezadoSeccion(s); got != 34 {
		t.Errorf("el encabezado con acciones mide %v y en la web mide 32,75", got)
	}
	if got := MedidaAccionLateral(); got.Width != 24 || got.Height != 24 {
		t.Errorf("la acción mide %v y SIDEBAR_ACTION_CLASS son 24×24", got)
	}
}

// TestLaSeccionSeparaSusFilasUnoComa75 es el `gap-0.5` medido, y el `mb-4` de debajo.
func TestLaSeccionSeparaSusFilasUnoComa75(t *testing.T) {
	a := NuevaFilaLateral("Recibidos", nil, nil)
	b := NuevaFilaLateral("Enviados", nil, nil)
	s := NuevaSeccionLateral("", a, b)
	s.Resize(MedidaSeccion(s))
	_ = test.WidgetRenderer(s)
	s.Refresh()
	pinta(t, 260, 120, s)

	sep := b.Position().Y - (a.Position().Y + a.Size().Height)
	if sep != 2 {
		t.Errorf("entre dos filas hay %v y el `gap-0.5` medido es 1,75", sep)
	}
	alto := MedidaSeccion(s).Height
	if quiero := 2*altoFilaWeb + 2 + tema.Espacio(4); alto != quiero {
		t.Errorf("la sección mide %v y dos filas con su hueco y el `mb-4` son %v", alto, quiero)
	}
}

// TestElPuntoDeEtiquetaVaCentradoEnLaCajaDelIcono: `grid h-5 w-5` con un
// `h-2.5 w-2.5 rounded-full` dentro. El color es un DATO (el de la etiqueta de correo),
// no un token, y por eso la fila lo recibe en crudo.
func TestElPuntoDeEtiquetaVaCentradoEnLaCajaDelIcono(t *testing.T) {
	rojo := color.NRGBA{R: 224, G: 106, B: 54, A: 255}
	f := NuevaFilaLateral("Facturas", nil, nil)
	f.Punto = rojo
	m := MedidaFilaLateral(f)
	m.Width = anchoUtilWeb
	f.Resize(m)
	img := pinta(t, m.Width+10, m.Height+10, f)

	x0, x1, y0, y1 := caja(img, rojo)
	lado := float32(x1-x0+1) / escala
	if lado < PuntoEtiqueta-1 || lado > PuntoEtiqueta+1 {
		t.Errorf("el punto mide %v y `h-2.5` con el rem de 14 son %v", lado, PuntoEtiqueta)
	}
	// Centrado en la caja de 17,5 que arranca en `px-2.5`.
	centro := (float32(x0+x1) / 2) / escala
	if quiero := tema.Espacio(2.5) + CajaIcono/2; centro < quiero-1 || centro > quiero+1 {
		t.Errorf("el punto se centra en x=%v y su caja lo pone en %v", centro, quiero)
	}
	if medio := (float32(y0+y1) / 2) / escala; medio < m.Height/2-1 || medio > m.Height/2+1 {
		t.Errorf("el punto se centra en y=%v y la fila mide %v de alto", medio, m.Height)
	}
}

// TestElContadorEsUnCirculoDe18 es `<Badge count>`: `h-[18px] min-w-[18px] px-1`. Con
// un dígito tiene que quedar redondo, que es de lo que vive el `min-w`.
func TestElContadorEsUnCirculoDe18(t *testing.T) {
	if got := MedidaContador("3"); got.Width != 18 || got.Height != 18 {
		t.Errorf("el contador de un dígito mide %v y `h-[18px] min-w-[18px]` son 18×18", got)
	}
	ancho := MedidaContador("128").Width
	if ancho <= 18 {
		t.Errorf("el contador de tres dígitos mide %v: con `px-1` tiene que pasar de los 18", ancho)
	}
	// Y la fila con contador sigue midiendo 33,25: 18 cabe en la caja de línea de 19,25.
	f := NuevaFilaLateral("Recibidos", iconoFalso(), nil)
	f.Contador = "17"
	if got := MedidaFilaLateral(f).Height; got != altoFilaWeb {
		t.Errorf("la fila con contador mide %v y en producción mide %v", got, float32(altoFilaWeb))
	}
}

// TestElContadorLlevaElRolPrimarioYSuRotulo: el relleno de `<Badge count>` es
// `bg-primary` con `text-on-primary`, que conmutan JUNTOS; dentro del carril, el
// primario de allí. Fue el escalón 600 fijo con `on-primary` encima.
//
// VISTO EN ROJO con `primary-600`: en oscuro el número, casi negro, queda a 4,1:1.
func TestElContadorLlevaElRolPrimarioYSuRotulo(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, variante := range []string{"claro", "oscuro", "traslucido"} {
		tema.Elegir(variante)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		for _, sobreCromo := range []bool{false, true} {
			c := NuevoContador("7")
			c.SobreCromo = sobreCromo
			c.Resize(MedidaContador("7"))
			img := pinta(t, 30, 30, c)
			relleno, rotulo := c.tokens()
			// El centro del contador es el número; el borde izquierdo a media altura, relleno.
			if got, quiero := px(img, int(2*escala), int(9*escala)), tema.Color(relleno, v); !cerca(got, quiero) {
				t.Errorf("en %s (carril=%v) el contador pinta %v y `%s` es %v", variante, sobreCromo, got, relleno, quiero)
			}
			if r := contraste(tema.Color(rotulo, v), tema.Color(relleno, v)); r < 4.5 {
				t.Errorf("en %s (carril=%v) el número contrasta %.2f:1 con su relleno", variante, sobreCromo, r)
			}
		}
	}
	if r, _ := NuevoContador("7").tokens(); r != "primary" {
		t.Errorf("fuera del carril el contador va sobre %q y es el rol `primary`", r)
	}
}

// TestElRotuloEnVersalitasLlevaSuTracking: `sytue-overline` declara
// `letter-spacing: 0.06em`, que a 10,5 px son 0,63 por letra. Fyne no tiene
// `letter-spacing`, así que si esto se queda en cero el rótulo sale apelmazado y varios
// píxeles más estrecho —justo lo que la hoja arregló a propósito—.
func TestElRotuloEnVersalitasLlevaSuTracking(t *testing.T) {
	const rotulo = "Carpetas" // 8 letras
	seco := Medir(Versalitas(rotulo), tema.TextoXS, SemiNegrita).Width
	con := MedirVersal(rotulo, tema.TextoXS, SemiNegrita)
	if quiero := seco + 8*0.72; con < quiero-0.01 || con > quiero+0.01 {
		t.Errorf("CARPETAS mide %v con tracking y 0,06em a 10,5 px son 0,63 por letra: %v", con, quiero)
	}
}

// TestLaFilaSeActivaConElRatonYConElTeclado. El teclado no es un extra: la fila es un
// destino de navegación y sin Intro ni Espacio el menú no se puede recorrer sin ratón.
func TestLaFilaSeActivaConElRatonYConElTeclado(t *testing.T) {
	veces := 0
	f := NuevaFilaLateral("Enviados", nil, func() { veces++ })
	test.Tap(f)
	f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	f.TypedRune(' ')
	if veces != 3 {
		t.Errorf("la fila se ha activado %d veces y tenía que hacerlo con el ratón, con Intro y con Espacio", veces)
	}
	f.Inerte = true
	test.Tap(f)
	f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if veces != 3 {
		t.Errorf("una fila inerte se ha activado: %d", veces)
	}
}

// TestLaFilaSeAnunciaConSuRotulo es la razón de que esto sea un widget y no un
// contenedor: `*fyne.Container` se anuncia como «Container» y no se puede cambiar, así
// que un menú hecho de contenedores es un menú mudo.
func TestLaFilaSeAnunciaConSuRotulo(t *testing.T) {
	f := NuevaFilaLateral("Borradores", nil, nil)
	if got := f.AccessibilityLabel(); got != "Borradores" {
		t.Errorf("la fila se anuncia como %q y su rótulo es «Borradores»", got)
	}
	if got := f.AccessibilityRole(); got != fyne.AccessibleRoleButton {
		t.Errorf("la fila se anuncia con el rol %q y se activa como un botón", got)
	}
	a := NuevaAccionLateral("Gestionar carpetas", nil, nil)
	if got := a.AccessibilityLabel(); got != "Gestionar carpetas" {
		t.Errorf("la acción es solo un icono y se anuncia como %q", got)
	}
	m := NuevoLateral("Navegación principal", nil)
	if got := m.AccessibilityLabel(); got != "Navegación principal" {
		t.Errorf("el carril se anuncia como %q y su `aria-label` es «Navegación principal»", got)
	}
}

// TestLaSeccionPlegadaNoPintaSusFilasPeroConservaElAire: la web deja el `mb-4` cuando
// una sección se pliega, y sin eso los grupos plegados se pegan unos a otros.
func TestLaSeccionPlegadaNoPintaSusFilasPeroConservaElAire(t *testing.T) {
	s := NuevaSeccionLateral("Ajustes", NuevaFilaLateral("Perfil", nil, nil))
	s.AlPlegar = func() {}
	s.Abierta = false
	alto := MedidaSeccion(s).Height
	if quiero := AltoEncabezadoSeccion(s) + tema.Espacio(4); alto != quiero {
		t.Errorf("la sección plegada mide %v y solo tiene que ocupar su rótulo y el `mb-4`: %v", alto, quiero)
	}
	s.Abierta = true
	if MedidaSeccion(s).Height <= alto {
		t.Error("desplegar la sección no ha añadido el alto de su fila")
	}
}

// TestElRotuloPlegableSePulsaEnVeinticuatro: el «Ajustes» de Mail medía 70,8×14 y es
// un botón. Su zona pulsable crece a 24 sin mover nada: la cabecera sigue en 22,75 y el
// texto donde estaba. VISTO EN ROJO sin el `max(…, LadoIconoSM)`: mide 14.
func TestElRotuloPlegableSePulsaEnVeinticuatro(t *testing.T) {
	s := NuevaSeccionLateral("Ajustes", NuevaFilaLateral("Perfil", nil, nil))
	s.AlPlegar = func() {}
	r := test.WidgetRenderer(s).(*seccionRenderer)
	r.Layout(fyne.NewSize(anchoUtilWeb, MedidaSeccion(s).Height))
	if got := r.plegador.Size().Height; got != 24 {
		t.Errorf("el rótulo plegable se pulsa en %v de alto y tiene que ser 24", got)
	}
	// El centro del rótulo no se ha movido: sigue en `pt-1` + media línea.
	if got, quiero := r.plegador.Position().Y+12, tema.Espacio(1)+tema.TextoXS.Linea/2; got != quiero {
		t.Errorf("el centro del rótulo cae en %v y caía en %v", got, quiero)
	}
	if got := AltoEncabezadoSeccion(s); got != 26 {
		t.Errorf("la cabecera mide %v y tiene que seguir en 22,75", got)
	}
}

// TestElRotuloLargoPasaADosLineas: `line-clamp-2` y no `truncate` (web, PR #844). La
// fila crece de 33,25 a 52,5 —dos líneas de 19,25 y `py-2`— solo cuando hace falta, y a
// partir de la tercera línea va la elipsis. VISTO EN ROJO con `Lineas: 1`: 33,25.
func TestElRotuloLargoPasaADosLineas(t *testing.T) {
	corta := NuevaFilaLateral("Perfil", iconoFalso(), nil)
	if got := MedidaFilaLateral(corta).Height; got != 38 {
		t.Errorf("una fila de una línea mide %v y son 33,25", got)
	}
	larga := NuevaFilaLateral("Freigegebene Postfächer und Stellvertretungen", iconoFalso(), nil)
	if got := MedidaFilaLateral(larga); got.Height != 60 || got.Width > AnchoUtilMenu {
		t.Errorf("un rótulo que no cabe hace la fila de %v; en la web mide 52,5 de alto y no pasa de %v de ancho", got, AnchoUtilMenu)
	}
	m := larga.metrica()
	cabe := AnchoUtilMenu - m.px - m.pxDer - m.cajaIcono - m.hueco
	lineas := larga.lineasRotulo(m, cabe)
	if len(lineas) != 2 || strings.HasSuffix(lineas[1], "…") {
		t.Errorf("el rótulo sale en %q: dos líneas enteras", lineas)
	}
	muyLarga := NuevaFilaLateral(strings.Repeat("Stellvertretung ", 12), iconoFalso(), nil)
	if l := muyLarga.lineasRotulo(m, cabe); len(l) != 2 || !strings.HasSuffix(l[1], "…") {
		t.Errorf("con más de dos líneas sale %q: la segunda acaba en elipsis", l)
	}
}

// TestElAroDelMenuEsBlancoYNoAzul. Sobre la tinta del carril, el azul de `focus-ring`
// no llega a 3:1 contra el ítem activo, y por eso Sidebar.tsx pide ahí
// `chrome-focus`, que es blanco. Es la razón de que el aro tome el token del control
// en vez de tenerlo fijo.
func TestElAroDelMenuUsaElTokenDelCromo(t *testing.T) {
	f := NuevaFilaLateral("Recibidos", nil, nil)
	m := MedidaFilaLateral(f)
	m.Width = anchoUtilWeb
	f.Resize(m)
	f.Move(fyne.NewPos(10, 10))
	f.FocusGained()
	f.Refresh()
	img := pinta(t, m.Width+40, m.Height+40, f)

	// El aro sobresale 2 px por cada lado: en x = 8 hay aro y en x = 6, lienzo.
	blanco := tema.Color("sidebar-focus", tema.Claro)
	if got := px(img, int(8*escala), int((10+m.Height/2)*escala)); !cerca(got, blanco) {
		t.Errorf("el aro del menú pinta %v y `sidebar-focus` es %v", got, blanco)
	}
	if blanco == tema.Color("focus-ring", tema.Claro) {
		t.Error("el foco del carril es el del contenido: sobre el color del menú no se vería")
	}
}

// TestElGalonDelRotuloPlegableEsElDeLaWeb: el chevron no se redibuja con líneas, se
// rasteriza del MISMO SVG que sirve el navegador —con sus extremos redondos—, y al
// plegar se cambia el dibujo, no la matriz de rotación.
func TestElGalonDelRotuloPlegableEsElDeLaWeb(t *testing.T) {
	if got := galonDe(true); got != iconos.ChevronAbajo {
		t.Errorf("la sección abierta pinta %v y la web pinta un chevron hacia abajo", got)
	}
	if got := galonDe(false); got != iconos.ChevronDerecha {
		t.Errorf("la sección plegada pinta %v y el `-rotate-90` de la web ES el chevron a la derecha", got)
	}
	// Que los dos existen en el catálogo ya no hace falta comprobarlo: son constantes
	// tipadas, así que un nombre que no esté no compila. Lo que sigue haciendo falta es
	// que su RUTA sea la de la web, y eso lo mira `iconos.TestElCatalogoEsElDeLaWeb`.
}

// TestElIconoDeLaAccionVaASuMedida: Mail, Calendar y Notepads le pasan el `<Icon
// size={16}>` de la web, centrado en la caja de 24. VISTO EN ROJO con el 17,5 forzado.
func TestElIconoDeLaAccionVaASuMedida(t *testing.T) {
	ico := canvas.NewRectangle(color.NRGBA{R: 255, A: 255})
	ico.SetMinSize(fyne.NewSize(16, 16))
	a := NuevaAccionLateral("Gestionar carpetas", ico, nil)
	a.Resize(MedidaAccionLateral())
	test.WidgetRenderer(a).Layout(MedidaAccionLateral())
	if ico.Size() != fyne.NewSize(16, 16) || ico.Position() != fyne.NewPos(4, 4) {
		t.Errorf("el icono mide %v en %v y va a 16, centrado en 24 (4, 4)", ico.Size(), ico.Position())
	}
}

// TestLaFilaTachadaLlevaSuRaya: la tarea hecha del árbol de Notepads va en
// `line-through`, a 0,3 em sobre la línea base (el `yStrikeoutPosition` de Figtree) y
// del ancho del rótulo. VISTO EN ROJO sin la raya: la fila no tenía cómo tacharse.
func TestLaFilaTachadaLlevaSuRaya(t *testing.T) {
	f := NuevaFilaLateral("Comprar pan", nil, nil)
	f.Tachada = true
	tam := fyne.NewSize(AnchoUtilMenu, MedidaFilaLateral(f).Height)
	f.Resize(tam)
	r := test.WidgetRenderer(f).(*filaRenderer)
	r.Layout(tam)
	if !r.tacha.Visible() {
		t.Fatal("la fila tachada no pinta su raya")
	}
	if got, quiero := r.tacha.Size().Width, Medir("Comprar pan", tema.TextoBase, Medio).Width; got != quiero {
		t.Errorf("la raya mide %v y el rótulo %v", got, quiero)
	}
	f.Tachada = false
	r.Layout(tam)
	if r.tacha.Visible() {
		t.Error("sin tachar, la raya sigue a la vista")
	}
}

// TestElPliegueVaEnSuHuecoYNoApagaLaFila: el chevron de una rama va en su hueco de 13 a
// la izquierda de la marca, y pasarle el puntero deja la fila «encima» (con sus acciones
// a la vista), como el `:hover` de un padre. VISTO EN ROJO con el chevron encima de la
// fila: el puntero sobre él apagaba la fila.
func TestElPliegueVaEnSuHuecoYNoApagaLaFila(t *testing.T) {
	pliegue := NuevoBotonIcono(iconos.ChevronAbajo, "Plegar", nil)
	pliegue.Lado = 21
	acciones := canvas.NewRectangle(color.NRGBA{R: 255, A: 255})
	acciones.SetMinSize(fyne.NewSize(20, 20))
	f := NuevaFilaLateral("Proyectos", iconoFalso(), nil)
	f.Rama = &RamaLateral{Sangria: 7, RellenoY: 5.25, Pliegue: pliegue, LadoPliegue: 13, Cola: acciones, ColaAlPasar: acciones}
	tam := fyne.NewSize(AnchoUtilMenu, MedidaFilaLateral(f).Height)
	f.Resize(tam)
	test.WidgetRenderer(f).Layout(tam)
	if x := pliegue.Position().X + pliegue.Size().Width/2; x != 7+13.0/2 {
		t.Errorf("el chevron se centra en x=%v y su hueco empieza en la sangría: %v", x, 7+13.0/2)
	}
	if x := f.Icono.Position().X; x != 7+13+tema.Espacio(1.5) {
		t.Errorf("la marca empieza en %v y va tras el hueco y el `gap-1.5`: %v", x, 7+13+tema.Espacio(1.5))
	}
	pliegue.MouseIn(nil)
	if fondo, _ := f.colores(tema.Claro); fondo.A == 0 || !acciones.Visible() {
		t.Error("con el puntero en el chevron la fila deja de estar encima")
	}
}

// TestElRotuloDeSeccionSeCentraConSusAcciones es el `flex items-center` del encabezado
// de SidebarSection: con la rueda o el «+» (24) la línea mide 24 y el rótulo va en su
// centro, en y = 3,5 + 5 = 8,5; y el rótulo plegable, igual. VISTO EN ROJO con el rótulo
// pegado arriba: 5 px alto en las secciones con acciones (lo midió Notepads).
func TestElRotuloDeSeccionSeCentraConSusAcciones(t *testing.T) {
	s := NuevaSeccionLateral("Cuadernos")
	s.Acciones = []fyne.CanvasObject{NuevaAccionLateral("Nuevo cuaderno", iconoFalso(), nil)}
	tam := MedidaSeccion(s)
	tam.Width = AnchoUtilMenu
	s.Resize(tam)
	r := test.WidgetRenderer(s).(*seccionRenderer)
	r.Layout(tam)
	ref := Texto("C", tema.TextoXS, SemiNegrita, color.NRGBA{})
	ColocarTexto(ref, tema.TextoXS, 0, tema.Espacio(1)+(LadoAccionLateral-tema.TextoXS.Linea)/2)
	if got := r.rotulo.letras[0].Position().Y; got != ref.Position().Y {
		t.Errorf("el rótulo está en y=%v y centrado con las acciones va en %v", got, ref.Position().Y)
	}

	p := NuevaSeccionLateral("Etiquetas")
	p.AlPlegar, p.Abierta = func() {}, true
	p.Acciones = s.Acciones
	p.Resize(tam)
	rp := test.WidgetRenderer(p).(*seccionRenderer)
	rp.Layout(tam)
	if y := rp.plegador.Position().Y; y != tema.Espacio(1) {
		t.Errorf("el rótulo plegable está en y=%v y centrado con las acciones va en %v", y, tema.Espacio(1))
	}
}

// TestLasAccionesDeLaSeccionSalenAlPasar es el `opacity-0 group-hover/cat:opacity-100
// group-focus-within/cat:opacity-100` de las categorías de Sytue Rooms: el «…» no se ve
// hasta que el puntero pasa por el ENCABEZADO o la acción tiene el foco, y sigue en su
// sitio. VISTO EN ROJO sin `AccionesAlPasar`: se veía siempre.
func TestLasAccionesDeLaSeccionSalenAlPasar(t *testing.T) {
	ico := iconoFalso()
	mas := NuevaAccionLateral("Acciones de la categoría", ico, nil)
	s := NuevaSeccionLateral("Proyectos", NuevaFilaLateral("general", nil, nil))
	s.Acciones = []fyne.CanvasObject{mas}
	s.AccionesAlPasar = true
	tam := fyne.NewSize(AnchoUtilMenu, MedidaSeccion(s).Height)
	s.Resize(tam)
	test.WidgetRenderer(s).Refresh()
	test.WidgetRenderer(mas).Refresh()
	if ico.Visible() {
		t.Fatal("sin el puntero encima, el «…» de la categoría se ve")
	}
	s.MouseIn(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(20, 5)}})
	if !ico.Visible() {
		t.Error("con el puntero en el encabezado, el «…» no sale")
	}
	s.MouseMoved(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(20, tam.Height-3)}})
	if ico.Visible() {
		t.Error("con el puntero en las filas, y no en el encabezado, el «…» sigue a la vista")
	}
	mas.FocusGained()
	if !ico.Visible() {
		t.Error("con el foco en el «…» tiene que verse (`group-focus-within`)")
	}
}

// TestLaAccionLateralTineSuGlifoAlPasar es `text-sidebar-fg-muted hover:text-sidebar-fg`
// de SIDEBAR_ACTION_CLASS: el glifo lo tiñe la acción, con su transición, y a la mitad
// cuando está inerte. VISTO EN ROJO con el glifo en el color que traía, que no cambiaba.
func TestLaAccionLateralTineSuGlifoAlPasar(t *testing.T) {
	v := tema.Claro
	a := NuevaAccionLateralCon("Gestionar carpetas", iconos.Trazo(iconos.Ajustes, 16, "sidebar-fg-muted", v), nil)
	r := test.WidgetRenderer(a).(*accionRenderer)
	if got := r.tinta.destino; got != tema.Color("sidebar-fg-muted", v) {
		t.Errorf("en reposo el glifo es %v y es `sidebar-fg-muted`", got)
	}
	a.MouseIn(nil)
	if got := r.tinta.visto; got != tema.Color("sidebar-fg", v) {
		t.Errorf("al pasar el ratón el glifo es %v y es `sidebar-fg`", got)
	}
	if r.tinta.anim == nil {
		t.Error("el glifo salta de color: falta el `transition-colors`")
	}
	a.MouseOut()
	a.Inerte = true
	a.Refresh()
	medio := tema.Color("sidebar-fg-muted", v)
	medio.A /= 2
	if got, quiero := r.tinta.visto, tokens.SobreSRGB(medio, tema.Color("sidebar-bg", v)); !cerca(got, quiero) {
		t.Errorf("inerte, el glifo es %v y va al 50 %% sobre la tinta del carril (%v)", got, quiero)
	}
}

// TestLaAccionLateralGiraMientrasCarga es el `animate-spin` del «Actualizar» de Mail: una
// animación mientras gira, ninguna cuando para. VISTO EN ROJO sin `girar()`.
func TestLaAccionLateralGiraMientrasCarga(t *testing.T) {
	a := NuevaAccionLateralCon("Actualizar", iconos.Trazo(iconos.Rotar, 16, "sidebar-fg-muted", tema.Claro), nil)
	a.Girando = true
	test.WidgetRenderer(a)
	if a.giro == nil {
		t.Fatal("con Girando la acción no gira")
	}
	a.Girando = false
	a.Refresh()
	if a.giro != nil {
		t.Error("al acabar la carga la acción sigue girando")
	}
}

// TestLaAccionDeLaFilaVaAlBordeDerecho es la `action` de SidebarItem: `absolute right-1`
// centrada en alto, y la fila con `pr-9` para dejarle su hueco. VISTO EN ROJO con la
// acción en la `Cola` que usaba Mail: caía tras el contador, 5 px más a la izquierda.
func TestLaAccionDeLaFilaVaAlBordeDerecho(t *testing.T) {
	f := NuevaFilaLateral("Recibidos", nil, nil)
	f.Contador = "3"
	f.Accion = NuevaAccionLateralCon("Actualizar", iconos.Trazo(iconos.Rotar, 16, "sidebar-fg-muted", tema.Claro), nil)
	f.Resize(fyne.NewSize(AnchoUtilMenu, MedidaFilaLateral(f).Height))
	r := test.WidgetRenderer(f).(*filaRenderer)
	r.Layout(f.Size())
	if got, quiero := f.Accion.Position(), fyne.NewPos(AnchoUtilMenu-tema.Espacio(1)-LadoAccionLateral,
		(altoFilaWeb-LadoAccionLateral)/2); got != quiero {
		t.Errorf("la acción está en %v y va a `right-1` centrada: %v", got, quiero)
	}
	if got := f.metrica().pxDer; got != tema.Espacio(9) {
		t.Errorf("con acción, el relleno derecho es %v y es `pr-9` (%v)", got, tema.Espacio(9))
	}
	if der := r.contador.Position().X + r.contador.Size().Width; der > AnchoUtilMenu-tema.Espacio(9)+0.01 {
		t.Errorf("el contador acaba en %v y se mete bajo la acción: el hueco empieza en %v", der, AnchoUtilMenu-tema.Espacio(9))
	}
}

// TestLaFilaPlegadaEnsenaSuRotuloAlPasar es el `title={label}` de SidebarItem: en el
// carril plegado solo queda el icono. Y la pista no le roba el `:hover` a la fila. VISTO
// EN ROJO sin la pista del carril.
func TestLaFilaPlegadaEnsenaSuRotuloAlPasar(t *testing.T) {
	f := NuevaFilaLateral("Recibidos", canvas.NewRectangle(color.NRGBA{A: 255}), nil)
	f.Plegada = true
	w := test.NewTempWindow(t, f)
	w.Resize(fyne.NewSize(64, 40))
	test.MoveMouse(w.Canvas(), fyne.NewPos(20, 15))
	if !f.encima {
		t.Fatal("con la pista del carril, la fila ha perdido su `:hover`")
	}
	time.Sleep(RetardoPista + 150*time.Millisecond)
	fyne.DoAndWait(func() {})
	if f.titulo == nil || !f.titulo.Abierta() || f.titulo.Rotulo != "Recibidos" {
		t.Error("la fila plegada no enseña su rótulo al pasar el ratón")
	}
}

// TestTodoBotonPrincipalDelLateralLlevaElPrimarioDelCarril: dentro de `bg-sidebar` la web
// reasigna `--color-primary` y su rótulo para todo lo que cuelga, no solo para la
// cabecera (appearance.css). Un principal o un secundario del cuerpo pasan al primario
// del carril —blanco con el rótulo de su color en claro—; un fantasma no cambia.
// VISTO EN ROJO sin `azulSobreTinta`.
func TestTodoBotonPrincipalDelLateralLlevaElPrimarioDelCarril(t *testing.T) {
	principal, fantasma := NuevoBoton("Reintentar", nil), NuevoBoton("Ocultar", nil).Con(Fantasma, TallaSM)
	secundario := NuevoBoton("Reintentar", nil).Con(Secundario, TallaXS)
	l := NuevoLateral("Menú", container.NewVBox(NuevaSeccionLateral("Carpetas", principal, secundario), fantasma))
	test.WidgetRenderer(l)
	// Uno del color de marca sobre el carril, que es de ese color, no se vería: el secundario también cambia.
	if r, _, _ := secundario.colores(tema.Claro); !secundario.SobreCromo || r != tema.Color("sidebar-primary", tema.Claro) {
		t.Errorf("el secundario del lateral pinta %v y es `sidebar-primary`", r)
	}
	if !principal.SobreCromo {
		t.Error("el botón principal del cuerpo del lateral sigue en la tinta del carril")
	}
	if fantasma.SobreCromo {
		t.Error("un fantasma del lateral ha cambiado de velos: en la web no usa `primary`")
	}
	relleno, rotulo, _ := principal.colores(tema.Claro)
	if relleno != tema.Color("sidebar-primary", tema.Claro) || rotulo != tema.Color("sidebar-on-primary", tema.Claro) {
		t.Errorf("el principal del lateral pinta %v con rótulo %v y es `sidebar-primary` con `sidebar-on-primary`", relleno, rotulo)
	}
}

// TestElBotonDeLaCabeceraLlevaElAroDelCromo: sobre el carril claro el azul de `focus-ring`
// se queda en 3,4:1 («Crear evento», medido en producción); la cabecera lo pasa al blanco
// de `chrome-focus`, como la de Sidebar.tsx. Un botón fuera del menú sigue en azul.
func TestElBotonDeLaCabeceraLlevaElAroDelCromo(t *testing.T) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	crear, suelto := NuevoBoton("Crear evento", nil), NuevoBoton("Guardar", nil)
	NuevoLateral("Menú", container.NewVBox()).ConCabecera(crear)
	for _, c := range []struct {
		b     *Boton
		token string
	}{{crear, "sidebar-focus"}, {suelto, "focus-ring"}} {
		r := test.WidgetRenderer(c.b).(*botonRenderer)
		if got := r.aro.anillo.FillColor; got != tema.Color(c.token, v) {
			t.Errorf("el aro de «%s» pinta %v y es `%s`", c.b.Texto, got, c.token)
		}
	}
}

// TestLaFilaInerteSeApagaSobreLaTinta: `disabled:opacity-50` deja ver lo que hay detrás,
// que en el menú es su tinta. Compuesto sobre `surface`, un rótulo casi blanco se
// quedaba casi blanco: aclaraba en vez de apagar. VISTO EN ROJO con `atenuar`.
func TestLaFilaInerteSeApagaSobreLaTinta(t *testing.T) {
	v := tema.Claro
	f := NuevaFilaLateral("Próximamente", nil, nil)
	f.Inerte = true
	r := test.WidgetRenderer(f).(*filaRenderer)
	medio := tema.Color("sidebar-fg-muted", v) // en reposo la fila va atenuada
	medio.A /= 2
	if got, quiero := r.rotulo.Color.(color.NRGBA), tokens.SobreSRGB(medio, tema.Color("sidebar-bg", v)); !cerca(got, quiero) {
		t.Errorf("el rótulo inerte es %v y va al 50 %% sobre la tinta del carril (%v)", got, quiero)
	}
}

// La sección sobre la que se va a soltar lleva el `ring-2 ring-indicator ring-inset`:
// 2 px por DENTRO del borde, sin el `mb-4`; sin Diana, nada.
//
// VISTO EN ROJO (2026-09-24, en el frontal, saboteado a mano): con el aro puesto en 0 y
// no metido 1 px, el trazo centrado acaba en x=459 y no en el borde (462), y un píxel de
// CSS por encima del `mb-4`.
func TestLaDianaEsUnAroPorDentro(t *testing.T) {
	s := NuevaSeccionLateral("Clientes", NuevaFilaLateral("general", iconoFalso(), nil))
	m := MedidaSeccion(s)
	m.Width = anchoUtilWeb
	s.Resize(m)
	if x0, _, _, _ := caja(pinta(t, m.Width, m.Height, s), tema.Color("sidebar-indicator", tema.Claro)); x0 != 9999 {
		t.Fatalf("sin Diana ya hay indicador en x=%d", x0)
	}
	s.Diana = true
	s.Refresh()
	x0, x1, y0, y1 := caja(pinta(t, m.Width, m.Height, s), tema.Color("sidebar-indicator", tema.Claro))
	if x0 != 0 || y0 != 0 || float32(x1+1)/escala != m.Width {
		t.Errorf("el aro va de (%d,%d) a x=%d; quiero de (0,0) al borde %v", x0, y0, x1, m.Width*escala)
	}
	if abajo := float32(y1+1) / escala; abajo != m.Height-margenAbajo {
		t.Errorf("el aro baja hasta %v y el `mb-4` queda fuera: %v", abajo, m.Height-margenAbajo)
	}
}

// TestElBotonDeIconoDelCarrilLlevaElRellenoAmbar: un `BotonIcono` que pinta con un token
// del menú lateral va dentro del carril, y su glifo lleva el relleno de allí como
// cualquier otro icono del menú (`--sytue-glyph-fill` dentro de `bg-sidebar`). Con otro
// token, el del tema.
//
// VISTO EN ROJO sin `DeToken` en el botón: en claro la tinta del carril es el blanco puro
// y el aro de la lupa salía relleno de blanco.
func TestElBotonDeIconoDelCarrilLlevaElRellenoAmbar(t *testing.T) {
	v := tema.Claro
	carril := tema.Color("sidebar-bg", v)
	ambar := tokens.SobreSRGB(tokens.ConAlfa(tema.Color("sidebar-indicator", v), 0.34), carril)
	pintar := func(trazo string) image.Image {
		b := NuevoBotonIcono(iconos.Buscar, "Buscar", nil)
		b.Trazo, b.TrazoEncima = trazo, ""
		fondo := canvas.NewRectangle(carril)
		fondo.Resize(fyne.NewSize(40, 40))
		b.Resize(b.MinSize())
		return pinta(t, 40, 40, fondo, b)
	}
	// Con holgura: el relleno va al 34 % dentro del mapa de bits del icono y el pintor
	// de pruebas lo compone en bytes premultiplicados.
	hay := func(img image.Image) bool {
		for y := range img.Bounds().Dy() {
			for x := range img.Bounds().Dx() {
				if cercaDe(px(img, x, y), ambar, 6) {
					return true
				}
			}
		}
		return false
	}
	if !hay(pintar("sidebar-fg")) {
		t.Errorf("con `sidebar-fg` el aro de la lupa no lleva el relleno ámbar del carril (%v)", ambar)
	}
	if hay(pintar("text")) {
		t.Error("fuera del carril el glifo también sale con el relleno ámbar")
	}
}
