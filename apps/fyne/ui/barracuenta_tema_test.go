// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestElSelectorDeCuentaGuardaLasTresAparienciasYElSistema(t *testing.T) {
	suite := suiteConSesion(t)
	t.Cleanup(func() { tema.Elegir("") })
	c := nuevaCuentaBarra(traductorPrueba(t), suite)
	// En el orden de `APARIENCIAS` de AccountMenu.tsx.
	valores := []string{"claro", "traslucido", "oscuro", ""}
	claves := []string{"light", "translucent", "dark", "system"}
	rotulos := make([]string, len(claves))
	for i, clave := range claves {
		rotulos[i] = c.t.T("account.appearance."+clave, nil)
	}
	for i, valor := range valores {
		s := c.apariencia(suite.Estado())
		if !slices.Equal(s.Opciones, rotulos) || s.Rotulo != c.t.T("account.appearance", nil) {
			t.Fatalf("el selector no ofrece las cuatro opciones traducidas: %+v", s)
		}
		s.Elegir(i)
		if e := ajustes.Leer(); e.Tema != valor {
			t.Fatalf("elegir %d ha guardado %q, quiero %q", i, e.Tema, valor)
		}
		if elegido := c.apariencia(suite.Estado()).Elegida; elegido != i {
			t.Errorf("reabrir el selector marca %d, quiero %d", elegido, i)
		}
	}
}

func TestLasAparienciasTraducidasCabenEnElMenuDeCuenta(t *testing.T) {
	for _, locale := range i18n.Locales {
		t.Run(string(locale), func(t *testing.T) {
			tr, err := i18n.New(locale, i18n.Common())
			if err != nil {
				t.Fatal(err)
			}
			c := nuevaCuentaBarra(tr, nil)
			s := c.apariencia(sesion.Estado{Tema: "traslucido"})
			const clave = "account.appearance.translucent"
			if s.Elegida != 1 || s.Opciones[1] == "" || s.Opciones[1] == clave {
				t.Fatalf("falta la apariencia translúcida traducida: %q", s.Opciones[1])
			}
			// `w-68` = 272, y la banda con su `px-2.5` no pasa de ahí en ningún idioma: lo
			// que no cabe en una línea baja a la siguiente, no ensancha el menú.
			if ancho := c.preferencias(sesion.Estado{Tema: "traslucido", ConOrganizacion: true}).MinSize().Width; ancho != 272 {
				t.Errorf("la banda mide %v y tiene que caber en los 272 px del menú", ancho)
			}
			// Y en los dos idiomas en que se midió, las cuatro apariencias van en UNA línea,
			// que es para lo que el menú creció 16 px.
			if locale == "es" || locale == "en" {
				s.Ancho = anchoPreferenciasCuenta
				if alto := s.MinSize().Height; alto != AltoSegmento+2*RellenoSegmentado {
					t.Errorf("las cuatro apariencias ocupan %v de alto: no caben en una línea de %v", alto, anchoPreferenciasCuenta)
				}
			}
		})
	}
}

// TestElSelectorDeCuentaGuardaElColorDeLaSuite: el color es un eje aparte del modo (ADR
// 0071) y su selector va junto al de la apariencia, con las tres opciones de la web
// (`THEME_COLORS`). Elegir una la guarda en los ajustes compartidos, la pinta y la deja
// marcada al reabrir; el modo no se toca. Y las tres salen traducidas en los veintiséis
// idiomas, no con su clave.
//
// VISTO EN ROJO sin `Ajustes.Color` en lo que el selector escribe: elegir «Azul cobalto»
// no guarda nada y el primario sigue en el de la casa.
func TestElSelectorDeCuentaGuardaElColorDeLaSuite(t *testing.T) {
	suite := suiteConSesion(t)
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor("") })
	tema.ElegirApp("") // otra barra de este proceso pudo decir su producto
	c := nuevaCuentaBarra(traductorPrueba(t), suite)
	deLaCasa := tema.Color("primary", tema.Claro)
	// Delante de los tres va «Organización», que es no elegir: es lo que marca quien no
	// ha elegido (la sesión de prueba no es de una cuenta personal).
	if s := c.color(suite.Estado()); len(s.Opciones) != 1+len(coloresAjuste) || s.Elegida != 0 || s.Opciones[0] != c.t.T("account.followOrg", nil) || s.Rotulo != c.t.T("account.color", nil) {
		t.Fatalf("sin elección el selector tiene que ofrecer «Organización» y los tres colores, y marcar la primera: %+v", s)
	}
	for i, valor := range coloresAjuste {
		c.color(suite.Estado()).Elegir(1 + i)
		if e := ajustes.Leer(); e.Color != valor || e.Tema != suite.Estado().Tema {
			t.Fatalf("elegir %d ha guardado color=%q tema=%q; quiero color=%q y el tema sin tocar", i, e.Color, e.Tema, valor)
		}
		if elegido := c.color(suite.Estado()).Elegida; elegido != 1+i {
			t.Errorf("reabrir el selector marca %d, quiero %d", elegido, 1+i)
		}
		if cambia := tema.Color("primary", tema.Claro) != deLaCasa; cambia != (valor == "cobalto") {
			// «app» sin producto —la barra de prueba no monta ninguno— es el de la casa.
			t.Errorf("con %q el primario pinta %v (el de la casa es %v)", valor, tema.Color("primary", tema.Claro), deLaCasa)
		}
	}
	for _, locale := range i18n.Locales {
		tr, err := i18n.New(locale, i18n.Common())
		if err != nil {
			t.Fatal(err)
		}
		s := nuevaCuentaBarra(tr, nil).color(sesion.Estado{Color: "app"})
		if s.Elegida != 2 || s.Opciones[2] == "" || s.Opciones[2] == "account.color.app" {
			t.Errorf("%s: falta «el de cada app» traducido: %q", locale, s.Opciones[2])
		}
		// Cada opción lleva su muestra, y la de «el de cada app» son cuatro colores.
		if len(s.Muestras) != len(s.Opciones) || len(s.Muestras[2]) != 4 || len(s.Muestras[0]) != 1 {
			t.Errorf("%s: las muestras de color son %v", locale, s.Muestras)
		}
	}
}

// TestElMenuDeCuentaEsElDeAccountMenuTsx lee las clases DEL FUENTE de la web: el menú de
// cuenta se copia a mano, y cuando la web lo cambió (más ancho, las preferencias en
// grupos segmentados con el rótulo a la izquierda, el idioma en una línea) el de aquí se
// quedó como estaba sin que nada lo dijera.
func TestElMenuDeCuentaEsElDeAccountMenuTsx(t *testing.T) {
	menu := webref.Read(t, "apps/frontend/packages/shell/src/AccountMenu.tsx")
	idioma := webref.Read(t, "apps/frontend/packages/ui/src/components/LanguageSelect.tsx")
	for _, c := range []struct{ fuente, donde, trozo, porque string }{
		{menu, "AccountMenu.tsx", `<div class="w-68">`, "el ancho del contenido del menú: 272"},
		{menu, "AccountMenu.tsx", `class="flex flex-col gap-2 px-2.5 py-2"`, "la banda de preferencias: filas a 8 y su relleno"},
		{menu, "AccountMenu.tsx", `class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2"`, "la fila: rótulo a la izquierda, grupo a la derecha, y baja si no cabe"},
		{menu, "AccountMenu.tsx", `class="bg-bg-subtle flex flex-wrap rounded-md p-0.5"`, "el carril del grupo, que también envuelve"},
		{menu, "AccountMenu.tsx", `'sytue-focus-ring rounded-control flex items-center gap-1.5 px-2 py-1 text-xs font-medium`, "una opción, con el hueco de su muestra"},
		{menu, "AccountMenu.tsx", `class="size-2.5 rounded-full"`, "la muestra de color"},
		{menu, "AccountMenu.tsx", `const APARIENCIAS = ['light', 'translucent', 'dark', 'system'] as const;`, "las apariencias y su orden"},
		{menu, "AccountMenu.tsx", `conOrg ? [SEGUIR, ...valores] : valores`, "«Organización» delante, y solo con organización"},
		{idioma, "LanguageSelect.tsx", `'flex items-center justify-between gap-3'`, "el idioma en una línea"},
		{idioma, "LanguageSelect.tsx", `class="w-40 max-w-[60%]"`, "el ancho del desplegable de idioma"},
	} {
		if !strings.Contains(c.fuente, c.trozo) {
			t.Errorf("%s ya no dice %q (%s): el menú de aquí ha dejado de ser el suyo", c.donde, c.trozo, c.porque)
		}
	}
	if AnchoCuenta != 272 || HuecoPreferencias != 8 || MargenPreferencias != 10 ||
		HuecoFilaPreferencia != 12 || SaltoFilaPreferencia != 8 ||
		LadoMuestra != 10 || HuecoMuestra != 6 || AnchoIdiomaCuenta != 160 || TopeIdiomaCuenta != 0.6 {
		t.Error("las medidas del menú de cuenta ya no son las de AccountMenu.tsx y LanguageSelect.tsx")
	}
	claves := make([]string, len(temasAjuste))
	for i, m := range temasAjuste {
		claves[i] = m.clave
	}
	if !slices.Equal(claves, []string{"light", "translucent", "dark", "system"}) {
		t.Errorf("las apariencias van en el orden %v y la web dice light, translucent, dark, system", claves)
	}

	// Pintado: el rótulo a la izquierda y el grupo a la derecha de SU fila, y el idioma en
	// una sola línea con su desplegable a la derecha.
	c := nuevaCuentaBarra(traductorPrueba(t), nil)
	fila := c.fila("account.forma", c.forma(sesion.Estado{ConOrganizacion: true})).(*fyne.Container)
	fila.Resize(fila.MinSize())
	fila.Layout.Layout(fila.Objects, fila.Size())
	rotulo, grupo := fila.Objects[0], fila.Objects[1]
	if rotulo.Position().X != 0 || grupo.Position().X+grupo.Size().Width != anchoPreferenciasCuenta || fila.MinSize().Height != grupo.MinSize().Height {
		t.Errorf("la fila de la forma pone el rótulo en %v y el grupo en %v (%v de ancho) dentro de %v", rotulo.Position(), grupo.Position(), grupo.Size().Width, fila.MinSize())
	}
	// El color, con «Organización» y sus muestras, no cabe en una línea: su grupo baja y
	// reparte las opciones en dos filas, sin pasar del ancho.
	delColor := c.fila("account.color", c.color(sesion.Estado{ConOrganizacion: true})).(*fyne.Container)
	if g := delColor.Objects[1].MinSize(); g.Width > anchoPreferenciasCuenta || g.Height <= AltoSegmento+2*RellenoSegmentado {
		t.Errorf("el grupo del color mide %v: tiene que envolver dentro de %v", g, anchoPreferenciasCuenta)
	}
}

// TestElSegmentadoDelMenuDiceCualEstaElegida: dentro de un menú la web pinta el carril
// del grupo con `menu-hover` —`.sytue-menu-surface` reasigna `--sytue-bg-subtle` en
// appearance.css— y la sombra de la píldora, como todo `box-shadow`, no pasa por debajo
// de ella. Aquí el carril era `bg-subtle` a secas y la sombra iba entera: en traslúcido,
// un velo blanco al 26 % sobre un menú casi blanco con otro al 66 % encima, y lo elegido
// solo se distinguía por la tinta (1,010:1 el carril contra el menú y 1,005:1 la píldora
// contra el carril, medidos en ventana real; la web da 1,106 y 1,098). En oscuro el carril
// tampoco se veía (1,04:1).
//
// Se mide con los colores PINTADOS del grupo tal como lo monta el menú, en los tres
// modos y los dos colores, contra la composición de la web. El suelo de 1,08 es el
// escalón más corto de la web (1,098:1, la píldora en traslúcido) con un nivel de
// redondeo de margen.
//
// VISTO EN ROJO sin `EnMenu` (en los tres modos: el carril no es `menu-hover`) y con la
// sombra entera bajo la píldora (traslúcido: sale dos niveles más oscura, 1,079:1).
func TestElSegmentadoDelMenuDiceCualEstaElegida(t *testing.T) {
	hoja := webref.Read(t, "apps/frontend/packages/config/src/tokens/appearance.css")
	if !strings.Contains(hoja, ".sytue-menu-surface,\n.sytue-popover-surface {\n  --color-bg-subtle: var(--sytue-menu-hover);") ||
		!strings.Contains(hoja, "  --sytue-bg-subtle: var(--sytue-menu-hover);") {
		t.Error("appearance.css ya no reasigna `bg-subtle` a `menu-hover` dentro de un menú: el carril del grupo de aquí ha dejado de ser el suyo")
	}
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor("") })
	const margen = 10
	for _, paleta := range []string{"petroleo", "cobalto"} {
		for _, variante := range []string{"claro", "traslucido", "oscuro"} {
			tema.ElegirColor(paleta)
			tema.Elegir(variante)
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			c := nuevaCuentaBarra(traductorPrueba(t), nil)
			g := c.fila("account.forma", c.forma(sesion.Estado{ConOrganizacion: true})).(*fyne.Container).Objects[1].(*Segmentado)
			g.Resize(g.MinSize())
			g.Move(fyne.NewPos(margen, margen))
			menu := tema.Color("menu-surface", v)
			img := pintaSobre(t, menu, g.MinSize().Width+2*margen, g.MinSize().Height+2*margen, g)
			// En el relleno de arriba de cada opción (`py-1`), donde no llega el rótulo.
			sitios, _ := g.reparto()
			en := func(i int) color.NRGBA {
				return px(img, int((margen+sitios[i].X+g.anchoOpcion(i)/2)*escala), int((margen+sitios[i].Y+2)*escala))
			}
			pista, pildora := en((g.Elegida+1)%len(g.Opciones)), en(g.Elegida)
			quieroPista := tema.Color("menu-hover", v)
			if quieroPildora := tokens.SobreSRGB(tema.Color("surface", v), quieroPista); !cerca(pista, quieroPista) || !cerca(pildora, quieroPildora) {
				t.Errorf("%s %s: el carril pinta %v y la píldora %v; en la web son %v y %v", paleta, variante, pista, pildora, quieroPista, quieroPildora)
			}
			if a, b := contraste(pista, menu), contraste(pildora, pista); a < 1.08 || b < 1.08 {
				t.Errorf("%s %s: el carril contrasta %.3f:1 con el menú y la píldora %.3f:1 con el carril; por debajo de 1,08 no se ve cuál está elegida", paleta, variante, a, b)
			}
		}
	}
}

// TestLaMuestraDeColorVaEnSectores: la muestra de «el de cada app» son cuatro colores
// de producto en cuartos, desde las doce y en el sentido del reloj, sin fundido (el
// `conic-gradient` de `themeColorSwatch`); la de un color, un disco liso. Fuera del
// círculo no pinta.
func TestLaMuestraDeColorVaEnSectores(t *testing.T) {
	a, b := color.NRGBA{R: 200, A: 255}, color.NRGBA{G: 200, A: 255}
	c, d := color.NRGBA{B: 200, A: 255}, color.NRGBA{R: 200, G: 200, A: 255}
	img := nuevaMuestra([]color.NRGBA{a, b, c, d}).Generator(40, 40)
	for _, p := range []struct {
		x, y   int
		quiero color.NRGBA
		donde  string
	}{
		{28, 12, a, "arriba a la derecha"},
		{28, 28, b, "abajo a la derecha"},
		{12, 28, c, "abajo a la izquierda"},
		{12, 12, d, "arriba a la izquierda"},
		{0, 0, color.NRGBA{}, "fuera del círculo"},
	} {
		if got := color.NRGBAModel.Convert(img.At(p.x, p.y)).(color.NRGBA); got != p.quiero {
			t.Errorf("%s la muestra pinta %v y quiero %v", p.donde, got, p.quiero)
		}
	}
	liso := nuevaMuestra([]color.NRGBA{a}).Generator(40, 40)
	if got := color.NRGBAModel.Convert(liso.At(12, 28)).(color.NRGBA); got != a {
		t.Errorf("la muestra de un solo color pinta %v en un cuarto; quiero %v en todos", got, a)
	}
}
