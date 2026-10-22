// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/calendario"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLaCarcasaDelERPEsLaDeLaWeb lee del FUENTE de la web lo que aquí se copia a mano: la
// carcasa (marco, carril de iconos, hoja y cajón de módulos), las medidas y los estados
// del carril, que son tokens de forma, y lo que app.css le pone al cajón por su id.
// Cuando la web metió el ERP en la forma Marco, el de aquí se quedó a sangre y con el
// menú fijo sin que nada lo dijera.
func TestLaCarcasaDelERPEsLaDeLaWeb(t *testing.T) {
	app := webref.Read(t, "apps/frontend/apps/erp/src/components/ErpApp.tsx")
	hoja := webref.Read(t, "apps/frontend/apps/erp/src/styles/app.css")
	forma := webref.Read(t, "apps/frontend/packages/config/src/tokens/appearance.css")
	medidas := webref.Read(t, "apps/frontend/packages/config/src/theme.css")
	for _, c := range []struct{ fuente, donde, trozo, porque string }{
		{app, "ErpApp.tsx", `<div class="erp-workspace sytue-frame sytue-safe-workspace flex `, "la raíz es el marco dentro del área segura"},
		{app, "ErpApp.tsx", `<nav class="erp-rail bg-rail" aria-label={t('nav.menu')}>`, "el carril de iconos"},
		{app, "ErpApp.tsx", `<div class="sytue-sheet flex min-h-0 min-w-0 flex-1">`, "`main` va dentro de la hoja, y la ficha a su lado"},
		{app, "ErpApp.tsx", `'fixed top-[var(--spacing-topbar)] bottom-0 left-0',`, "el menú de módulos es un cajón a cualquier ancho"},
		{app, "ErpApp.tsx", `abierto ? 'sytue-drawer-in' : 'hidden',`, "y cerrado no se ve"},
		{app, "ErpApp.tsx", `class="mb-3 ml-auto flex"`, "el aspa del cajón, a la derecha"},
		{app, "ErpApp.tsx", `rounded-nav relative w-full px-3 py-1.5 text-left text-base [font-weight:var(--sytue-nav-active-weight)]`, "la entrada activa: radio y peso de la forma"},
		{app, "ErpApp.tsx", `before:w-[min(2px,var(--sytue-nav-indicator))]`, "su barra de «estás aquí», la de la forma con tope de 2"},
		{app, "ErpApp.tsx", `sytue-focus-ring-chrome rounded-control mb-4 flex w-full items-center justify-between border px-3 py-2 text-sm`, "«Ir a…», con el radio de control"},
		{app, "ErpApp.tsx", `sytue-focus-ring-chrome rounded-control mb-4 inline-flex h-8 w-full items-center justify-center gap-1.5 border px-3 text-sm font-semibold`, "«Sytue AI», ídem"},
		{app, "ErpApp.tsx", `<h1 class="text-text sytue-h4">{modelo.etiqueta}</h1>`, "el título de la lista"},
		{app, "ErpApp.tsx", `<h1 class="text-text sytue-h4 tracking-tight">`, "el saludo de la portada"},
		{app, "ErpApp.tsx", `<h2 class="text-text sytue-h4 truncate">`, "el título de la ficha, que va dentro de la hoja"},
		{app, "ErpApp.tsx", `'bg-surface-raised border-border rounded-card flex flex-wrap items-center justify-between gap-3 border px-4 py-2.5 shadow-sm',`, "la barra de la lista, con el radio de tarjeta"},
		{app, "ErpApp.tsx", `<div class="border-border bg-surface rounded-card overflow-x-auto border shadow-xs max-md:hidden">`, "la tabla, ídem"},
		{hoja, "app.css", ".erp-workspace #erp-cajon {\n  padding: 1.25rem 1rem;", "el relleno del cajón"},
		{hoja, "app.css", ".erp-workspace #erp-cajon > button,\n.erp-workspace #erp-cajon li > button {\n  min-height: 2.75rem;", "el alto mínimo de sus botones y entradas"},
		{hoja, "app.css", "    gap: 0.625rem;\n    width: 4.5rem;\n    flex: 0 0 4.5rem;\n    padding-block: 1.5rem;", "el carril: 72 de ancho, 24 arriba y abajo, hueco de 10"},
		{hoja, "app.css", "    width: 2.75rem;\n    min-height: 2.75rem;", "la acción: 44 × 44"},
		{hoja, "app.css", "    border-radius: var(--radius-nav);\n    --sytue-glyph-fill: var(--sytue-rail-item-fill);\n    background: var(--sytue-rail-item-bg);\n    color: var(--sytue-primary-text);", "la acción en reposo"},
		{hoja, "app.css", "  .erp-rail-action:hover {\n    background: var(--sytue-rail-item-hover);", "con el puntero encima"},
		{hoja, "app.css", "  .erp-rail-action[aria-current='page'] {\n    --sytue-glyph-fill: var(--sytue-sidebar-indicator);\n    background: var(--sytue-rail-active-bg);\n    color: var(--sytue-rail-active-fg);", "la activa: «estás aquí» es el relleno del icono"},
		{hoja, "app.css", "  .erp-rail-action[aria-current='page']::before {\n    content: '';\n    position: absolute;\n    inset-block: 0.5rem;\n    inset-inline-start: -0.875rem;\n    width: var(--sytue-rail-indicator);\n    border-radius: var(--radius-pill);\n    background: var(--sytue-sidebar-indicator);", "la barra de «estás aquí»: en el borde del carril, a plena opacidad"},
		{medidas, "theme.css", "  --sytue-rail-indicator: 3px;", "su ancho en Marco"},
		{medidas, "theme.css", "  --sytue-rail-indicator: 0px;", "y en Carril, donde no hay"},
		{hoja, "app.css", "    inset-block-start: -0.3rem;\n    inset-inline-end: -0.3rem;\n    min-width: 1.125rem;\n    height: 1.125rem;\n    padding-inline: 0.2rem;", "la cuenta: 18, 4,8 por fuera de la esquina"},
		{hoja, "app.css", "    background: var(--sytue-danger);", "la cuenta, en rojo de peligro"},
		{hoja, "app.css", "    color: var(--sytue-neutral-0);\n    font-size: 0.6875rem;\n    font-weight: 600;", "y su cifra, blanca de 11/600"},
		{forma, "appearance.css", "  --sytue-rail-item-bg: transparent;\n  --sytue-rail-item-hover: var(--sytue-sidebar-hover-bg);\n  --sytue-rail-item-fill: transparent;\n  --sytue-rail-active-bg: var(--sytue-sidebar-active-bg);\n  --sytue-rail-active-fg: var(--sytue-sidebar-active-fg);", "el carril de iconos en Marco"},
		{forma, "appearance.css", "  --sytue-rail-item-bg: var(--sytue-sidebar-hover-bg);\n  --sytue-rail-item-hover: var(--sytue-sidebar-active-bg);\n  --sytue-rail-item-fill: var(--sytue-sidebar-indicator);\n  --sytue-rail-active-bg: var(--sytue-sidebar-primary);\n  --sytue-rail-active-fg: var(--sytue-sidebar-on-primary);", "y en Carril"},
	} {
		if !strings.Contains(c.fuente, c.trozo) {
			t.Errorf("%s ya no dice %q (%s): la carcasa del ERP de aquí ha dejado de ser la suya", c.donde, c.trozo, c.porque)
		}
	}
	// Cada icono del carril, por su nombre en la web y en su orden.
	p := pantallaDePrueba(t)
	p.rellenarMenu()
	c := p.carril()
	nombres := map[iconos.Icono]string{
		iconos.Menu: "menu", iconos.Rejilla: "grid", iconos.Tic: "check", iconos.Edificio: "building",
		iconos.Auditoria: "audit", iconos.Buscar: "search", iconos.IA: "ai",
	}
	desde := 0
	for _, a := range append(append([]*AccionCarril{}, c.Arriba...), c.Abajo...) {
		marca := `<Icon name="` + nombres[a.Icono] + `" size={20} />`
		i := strings.Index(app[desde:], marca)
		if i < 0 {
			t.Fatalf("el carril de la web no lleva %s después de los anteriores: el de aquí va en otro orden o le sobra", marca)
		}
		desde += i + len(marca)
	}
	if len(c.Arriba) != 5 || len(c.Abajo) != 2 {
		t.Errorf("el carril lleva %d acciones arriba y %d abajo; son menú, inicio, tareas, banco y obligaciones, y el buscador y Sytue AI", len(c.Arriba), len(c.Abajo))
	}

	for _, m := range []struct {
		que         string
		got, quiero float32
	}{
		{"AnchoCarril", AnchoCarril, 72},
		{"CarrilPY", CarrilPY, 24},
		{"HuecoCarril", HuecoCarril, 10},
		{"LadoAccionCarril", LadoAccionCarril, 44},
		{"LadoIconoCarril", LadoIconoCarril, 20},
		{"LadoCuentaCarril", LadoCuentaCarril, 18},
		{"FueraCuentaCarril", FueraCuentaCarril, 4.8},
		{"CuentaCarrilPX", CuentaCarrilPX, 3.2},
		{"la cifra de la cuenta", EscalonCuentaCarril.Tam, 11},
		{"su caja de línea", EscalonCuentaCarril.Linea, 16.5},
		{"MenuPX", MenuPX, 16},
		{"MenuPY", MenuPY, 20},
		{"AltoMinimoCajon", AltoMinimoCajon, 44},
		{"AnchoIndicador", AnchoIndicador, 2},
		{"IndicadorCarrilX", IndicadorCarrilX, -14},
		{"IndicadorCarrilY", IndicadorCarrilY, 8},
	} {
		if !casi(m.got, m.quiero) {
			t.Errorf("%s = %v y la hoja dice %v", m.que, m.got, m.quiero)
		}
	}
}

func casi(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// cercaDe compara dos colores con dos niveles de margen: el alfa se cuantiza.
func cercaDe(a, b color.NRGBA) bool {
	return dif(uint32(a.R), uint32(b.R)) <= 2 && dif(uint32(a.G), uint32(b.G)) <= 2 && dif(uint32(a.B), uint32(b.B)) <= 2
}

func pixel(img image.Image, x, y float32) color.NRGBA {
	r, g, b, a := img.At(int(x), int(y)).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

// TestElCarrilDeIconosColocaDondeLaWeb: las posiciones medidas en un navegador con las
// hojas de main (2026-10-05), a 480 de alto, con cinco acciones arriba y dos abajo: 14 de
// margen, la primera en 24, una cada 54 y las de abajo pegadas al fondo.
func TestElCarrilDeIconosColocaDondeLaWeb(t *testing.T) {
	c := &CarrilIconos{}
	for range 5 {
		c.Arriba = append(c.Arriba, NuevaAccionCarril(iconos.Rejilla, "x", false, nil))
	}
	c.Abajo = []*AccionCarril{NuevaAccionCarril(iconos.Buscar, "x", false, nil), NuevaAccionCarril(iconos.IA, "x", false, nil)}
	o := c.Objeto(tema.Claro)
	if got := o.MinSize().Width; got != 72 {
		t.Errorf("el carril mide %v de ancho y son 72", got)
	}
	o.Resize(fyne.NewSize(72, 480))
	acciones := o.(*fyne.Container).Objects[1:] // tras el fondo, cada una en su pista
	for i, y := range []float32{24, 78, 132, 186, 240, 358, 412} {
		a := acciones[i]
		if a.Position() != fyne.NewPos(14, y) || a.Size() != fyne.NewSize(44, 44) {
			t.Errorf("la acción %d está en %v y mide %v; la web la pone en (14, %v) con 44 × 44", i, a.Position(), a.Size(), y)
		}
		if p, ok := a.(*ui.Pista); !ok || p.Rotulo != "x" || !p.Derecha {
			t.Errorf("la acción %d no lleva su `title` en una pista a la derecha: sin rótulo, es lo único que dice qué es", i)
		}
	}
}

// TestElCarrilDeIconosPintaSusEstados: reposo, puntero encima y activo, en las dos formas
// y los tres modos, con los colores PINTADOS contra los tokens de forma compuestos sobre
// lo que tienen debajo (el marco en Marco, el carril en Carril). Y lo que dice «estás
// aquí» donde no hay rótulo: en Marco, una barra ámbar de 3 a plena opacidad en el borde
// del carril, que tiene que contrastar 3:1 con el marco (WCAG 1.4.11); el relleno del
// icono activo va al 34 % (`fill-opacity="0.34"` de Glifo.tsx) y no basta. En Carril no
// hay barra: el activo ya es un cuadro macizo. La cuenta, en su sitio y en rojo.
//
// VISTO EN ROJO pintando el activo con `rail-item-bg` y sin `Acento` en el icono: el velo
// del activo no sale y el relleno de la rejilla se queda en el del marco. Y sin pintar la
// barra: en su sitio se ve el marco.
func TestElCarrilDeIconosPintaSusEstados(t *testing.T) {
	t.Cleanup(func() { tema.Elegir(""); tema.FormaDeLosGuardianes() })
	blanco := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for _, forma := range []string{"marco", "carril"} {
		for _, variante := range []string{"claro", "traslucido", "oscuro"} {
			tema.ElegirForma(forma)
			tema.Elegir(variante)
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			debajo := "rail"
			if forma == "marco" {
				debajo = "frame-from"
			}
			// Aplanado sobre blanco: en traslúcido el carril es un velo.
			base := tokens.SobreSRGB(tema.Color(debajo, v), blanco)
			sobreBase := func(token string) color.NRGBA { return tokens.SobreSRGB(tema.Color(token, v), base) }

			reposo := NuevaAccionCarril(iconos.Rejilla, "Inicio", false, nil)
			encima := NuevaAccionCarril(iconos.Rejilla, "Inicio", false, nil)
			activa := NuevaAccionCarril(iconos.Rejilla, "Tareas", true, nil)
			activa.Contador = 3
			fondo := canvas.NewRectangle(base)
			fondo.Resize(fyne.NewSize(100, 200))
			for i, a := range []*AccionCarril{reposo, encima, activa} {
				a.Move(fyne.NewPos(20, 20+60*float32(i)))
				a.Resize(a.MinSize())
			}
			w := test.NewWindow(container.NewWithoutLayout(fondo, reposo, encima, activa))
			w.SetPadded(false)
			w.Resize(fyne.NewSize(100, 200))
			encima.MouseIn(&desktop.MouseEvent{})
			img := w.Canvas().Capture()
			w.Close()

			// El fondo de la acción se mira a la izquierda del icono; su relleno, dentro
			// del primer cuadro de la rejilla: el ámbar del carril al 34 % sobre ese fondo.
			ambar := tema.Color("sidebar-indicator", v)
			ambar.A = uint8(math.Round(0.34 * 255))
			rellenoReposo := sobreBase("rail-item-bg")
			if forma == "carril" { // en Carril el icono lleva el ámbar también en reposo
				rellenoReposo = tokens.SobreSRGB(ambar, rellenoReposo)
			}
			for _, c := range []struct {
				que    string
				x, y   float32
				quiero color.NRGBA
			}{
				{"el fondo en reposo", 25, 42, sobreBase("rail-item-bg")},
				{"el fondo con el puntero encima", 25, 102, sobreBase("rail-item-hover")},
				{"el fondo del activo", 25, 162, sobreBase("rail-active-bg")},
				{"el relleno del icono en reposo", 37, 37, rellenoReposo},
				{"el relleno del icono activo", 37, 157, tokens.SobreSRGB(ambar, sobreBase("rail-active-bg"))},
				{"la cuenta", 20 + 44 + FueraCuentaCarril - LadoCuentaCarril + 4, 140 - FueraCuentaCarril + 13, tema.Color("danger", v)},
			} {
				if got := pixel(img, c.x, c.y); !cercaDe(got, c.quiero) {
					t.Errorf("%s %s: %s pinta %v y quiero %v", forma, variante, c.que, got, c.quiero)
				}
			}
			r := test.WidgetRenderer(activa).(*accionCarrilRenderer)
			if got := r.icono.Tinta(); got != tema.Color("rail-active-fg", v) {
				t.Errorf("%s %s: el icono activo va en %v y es `rail-active-fg`, %v", forma, variante, got, tema.Color("rail-active-fg", v))
			}
			if got := test.WidgetRenderer(reposo).(*accionCarrilRenderer).icono.Tinta(); got != tema.Color("sidebar-fg", v) {
				t.Errorf("%s %s: el icono en reposo va en %v y es el del carril, %v", forma, variante, got, tema.Color("sidebar-fg", v))
			}
			// «Estás aquí»: la barra, en el borde del carril (14 a la izquierda de la acción).
			barra := r.barra.Objetos()[0].(*canvas.Rectangle)
			enBarra := pixel(img, 20-14+1, 140+22)
			if forma == "marco" {
				entero := tema.Color("sidebar-indicator", v)
				if p := barra.Position(); !casi(p.X, -14) || !casi(p.Y, 8) || barra.Size() != fyne.NewSize(3, 28) || !cercaDe(enBarra, entero) {
					t.Errorf("marco %s: la barra de «estás aquí» está en %v, mide %v y pinta %v; la web la pone en (−14, 8) con 3 × 28 y en %v", variante, p, barra.Size(), enBarra, entero)
				}
				la, lb := calendario.Luminancia(enBarra), calendario.Luminancia(base)
				if c := (max(la, lb) + 0.05) / (min(la, lb) + 0.05); c < 3 {
					t.Errorf("marco %s: la barra de «estás aquí» contrasta %.2f:1 con el marco y WCAG 1.4.11 pide 3:1", variante, c)
				}
			} else if _, _, _, a := barra.FillColor.RGBA(); a != 0 || barra.Size().Width != 0 || !cercaDe(enBarra, base) {
				t.Errorf("carril %s: el activo no lleva barra (ya es un cuadro macizo) y pinta %v con %v de ancho", variante, enBarra, barra.Size().Width)
			}
			// La cuenta: 18 × 18, 4,8 por fuera de la esquina de arriba a la derecha.
			caja := r.cuenta.Objetos()[0]
			if p := caja.Position(); !casi(p.X, 44+4.8-18) || !casi(p.Y, -4.8) || caja.Size() != fyne.NewSize(18, 18) {
				t.Errorf("%s %s: la cuenta está en %v y mide %v; la web la pone en (30,8, −4,8) con 18 × 18", forma, variante, p, caja.Size())
			}
		}
	}
}

// TestElERPLlevaLaFormaMarco: la pantalla entera, que es lo que vio el dueño. En Marco el
// carril de iconos deja ver el marco y `main` va en la hoja, que empieza donde acaba el
// carril, se separa del borde derecho y del de abajo y redondea sus esquinas. El menú de
// módulos es un cajón que abre el primer botón del carril y cierra su aspa. Por debajo de
// 768 no hay carril: manda el botón de `main`.
//
// VISTO EN ROJO con `Armazon.SinHoja`, que es como estaba: la esquina de `main` y el
// borde derecho salen del papel y no del marco.
func TestElERPLlevaLaFormaMarco(t *testing.T) {
	tema.ElegirForma("marco")
	antes := ui.SinMovimiento
	ui.SinMovimiento = true // el cajón, ya en su sitio
	t.Cleanup(func() { tema.FormaDeLosGuardianes(); ui.SinMovimiento = antes })
	const ancho, alto = 1280, 744
	p := pantallaDePrueba(t)
	p.Fase = Exito
	w := test.NewWindow(p.Objeto())
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ancho, alto))
	img := w.Canvas().Capture()
	guardarPNG(t, img, "erp-marco.png")

	marco, papel := tema.Color("frame-from", tema.Claro), tema.Color("bg", tema.Claro)
	separa := tema.Forma("sheet-gap")
	for _, c := range []struct {
		que    string
		x, y   float32
		quiero color.NRGBA
	}{
		{"el carril de iconos deja ver el marco", 4, 400, marco},
		{"la esquina de la hoja es del marco", AnchoCarril + 1, 1, marco},
		{"dentro de la hoja, el papel", AnchoCarril + 24, 400, papel},
		{"a la derecha de la hoja, el marco", ancho - separa/2, 400, marco},
		{"debajo de la hoja, el marco", 700, alto - separa/2, marco},
	} {
		// Con holgura: junto a la hoja el marco lleva encima su sombra.
		if got := pixel(img, c.x, c.y); dif(uint32(got.R), uint32(c.quiero.R))+dif(uint32(got.G), uint32(c.quiero.G))+dif(uint32(got.B), uint32(c.quiero.B)) > 24 {
			t.Errorf("%s: en (%v, %v) pinta %v y quiero %v", c.que, c.x, c.y, got, c.quiero)
		}
	}

	var acciones []*AccionCarril
	recorrer(w.Content(), func(o fyne.CanvasObject) {
		if a, ok := o.(*AccionCarril); ok {
			acciones = append(acciones, a)
		}
	})
	if len(acciones) != 7 || acciones[0].Icono != iconos.Menu || !acciones[1].Activa {
		t.Fatalf("en ancho hay %d acciones en el carril; quiero siete, la primera el menú y la de inicio activa", len(acciones))
	}
	if p.cajon || p.arm.Menu.Visible() {
		t.Error("el menú de módulos es un cajón: cerrado no se ve")
	}
	// La entrada activa del cajón: en Marco no hay barra de «estás aquí» (lo dicen el
	// velo y la negrita) y en Carril sí, de 2. VISTO EN ROJO en ventana real: con ancho
	// 0 y su color, el pintor de GL dejaba una raya ámbar de dos píxeles.
	activa := NuevaEntrada("Tickets", true, nil)
	activa.Resize(fyne.NewSize(227, AltoEntrada))
	barra := test.WidgetRenderer(activa).(*entradaRenderer).barra.Objetos()[0].(*canvas.Rectangle)
	if _, _, _, a := barra.FillColor.RGBA(); a != 0 || barra.Size().Width != 0 || activa.peso() != ui.PesoNavActivo() {
		t.Errorf("en Marco la entrada activa pinta una barra de %v con alfa %d; no lleva barra, y su peso es el de la forma", barra.Size().Width, a)
	}
	tema.ElegirForma("carril")
	conBarra := NuevaEntrada("Tickets", true, nil)
	conBarra.Resize(fyne.NewSize(227, AltoEntrada))
	barra = test.WidgetRenderer(conBarra).(*entradaRenderer).barra.Objetos()[0].(*canvas.Rectangle)
	if _, _, _, a := barra.FillColor.RGBA(); a == 0 || barra.Size().Width != 2 {
		t.Errorf("en Carril la entrada activa lleva su barra de 2; pinta %v con alfa %d", barra.Size().Width, a)
	}
	tema.ElegirForma("marco")
	acciones[0].AlPulsar()
	if !p.cajon || !p.arm.CajonAbierto || !p.arm.Menu.Visible() {
		t.Errorf("el botón de menú del carril no abre el cajón (cajon %v, armazón %v)", p.cajon, p.arm.CajonAbierto)
	}
	p.menu.AlCerrar()
	if p.cajon || p.arm.CajonAbierto {
		t.Error("el aspa no cierra el cajón")
	}

	// Por debajo de 768: sin carril, y con el botón de `main`.
	q := pantallaDePrueba(t)
	q.Ventana = fyne.NewSize(360, 740)
	e := test.NewWindow(q.Objeto())
	defer e.Close()
	e.SetPadded(false)
	e.Resize(q.Ventana)
	var enCarril, botones int
	recorrer(e.Content(), func(o fyne.CanvasObject) {
		switch o.(type) {
		case *AccionCarril:
			enCarril++
		case *BotonCajon:
			botones++
		}
	})
	if enCarril != 0 || botones != 1 {
		t.Errorf("a 360 hay %d acciones de carril y %d botones del cajón; quiero 0 y 1", enCarril, botones)
	}
}
