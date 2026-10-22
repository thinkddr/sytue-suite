// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tokens

import (
	"image/color"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func rgb(n uint32) color.NRGBA {
	return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 0xff}
}

// Los esperados están sacados A MANO de branding/tokens/colors.css, siguiendo cada
// var() con el dedo: si salieran del parser, el test se comprobaría a sí mismo.
func TestColorEnLosDosTemas(t *testing.T) {
	// `tintaClara` se fue con los bordes: eran sus únicos usuarios y ahora se
	// guardan ya mezclados con la superficie, sin alfa.
	tintaOscura := func(a uint8) color.NRGBA { return color.NRGBA{231, 239, 239, a} }
	casos := []struct {
		nombre        string
		claro, oscuro color.NRGBA
	}{
		{"bg", rgb(0xf1f6f6), rgb(0x0a1011)},
		{"bg-subtle", rgb(0xe8efef), rgb(0x182325)},
		{"surface", rgb(0xffffff), rgb(0x131c1e)},
		{"surface-raised", rgb(0xffffff), rgb(0x1a2628)},
		{"card-blue", rgb(0xe8efff), color.NRGBA{36, 87, 232, 56}},
		{"card-violet", rgb(0xeeebf8), color.NRGBA{101, 70, 197, 64}},
		{"card-mint", rgb(0xe5f0ea), color.NRGBA{37, 118, 91, 56}},
		{"card-rose", rgb(0xf6e5ed), color.NRGBA{182, 64, 104, 56}},
		{"card-amber", rgb(0xf5efca), color.NRGBA{234, 214, 65, 31}},
		{"text", rgb(0x0f1d20), rgb(0xe7efef)},
		{"text-muted", rgb(0x43555a), rgb(0xa6b8ba)},
		{"text-subtle", rgb(0x586a6f), rgb(0x8a9ea1)},
		// La acción principal es el primario 600 en claro y el 400, más claro, en oscuro;
		// su rótulo cambia con ella: blanco en claro y casi negro en oscuro.
		{"primary", rgb(0x0f7680), rgb(0x3cc4cc)},
		{"primary-hover", rgb(0x0c636b), rgb(0x33b0b8)},
		{"primary-active", rgb(0x0a4a50), rgb(0x1a9aa3)},
		{"on-primary", rgb(0xffffff), rgb(0x04161a)},
		{"primary-text", rgb(0x0c636b), rgb(0x7fd3d8)},
		{"primary-500", rgb(0x1a9aa3), rgb(0x1a9aa3)},
		// Un var() que el oscuro arrastra sin redeclararlo: el relleno de los glifos.
		{"glyph-fill", rgb(0x0f7680), rgb(0x3cc4cc)},
		{"accent-700", rgb(0x224656), rgb(0x224656)},
		{"indicator", rgb(0x0f7680), rgb(0x3cc4cc)},
		{"focus-ring", rgb(0x0f7680), rgb(0x7fd3d8)},
		// Se declaran una vez en :root y cambian solas con la tinta del tema.
		// LOS BORDES SE GUARDAN YA MEZCLADOS con su superficie, y por eso aquí
		// llevan un color plano en vez de la tinta con alfa que declara el CSS:
		// Gio compone en espacio lineal y el navegador en sRGB, así que dejarlos
		// con alfa los sacaba a media fuerza en TODA la suite (ver aplanarBordes
		// en colors.go). El vector claro se deriva de rgba(15, 29, 32, α) sobre #ffffff
		// y el oscuro de rgba(231, 239, 239, α) sobre #131c1e, con α de 7/12 % y 6/11 %.
		{"border-subtle", rgb(0xeeefef), rgb(0x1f282a)},
		{"border", rgb(0xe2e4e4), rgb(0x2a3335)},
		// `border-strong` es el CONTORNO DE CONTROL y no sigue a la tinta decorativa:
		// 60 % en claro y 42 % en oscuro, las dos opacidades que WCAG 1.4.11 pide para
		// llegar a 3:1. El aro de la tesela se quedó en el 16 % de antes.
		{"border-strong", rgb(0x6f7779), rgb(0x6c7576)},
		{"product-tile-ring", color.NRGBA{}, tintaOscura(41)},
		// color-mix con transparent en claro (el primario 600 al 12 %) y un rgba
		// literal en oscuro (el 400 al 16 %).
		{"selected", color.NRGBA{0x0f, 0x76, 0x80, 31}, color.NRGBA{0x3c, 0xc4, 0xcc, 41}},
		// El CARRIL: estos no están en colors.css sino en appearance.css, que se lee
		// encima. Sacados a mano de sus bloques `:root` y `[data-theme='dark']`; el
		// indicador y el foco solo se declaran en `:root` y el oscuro los arrastra.
		{"sidebar-bg", rgb(0x0c5f68), rgb(0x0d2a2e)},
		{"sidebar-fg", rgb(0xffffff), rgb(0xe7efef)},
		{"sidebar-active-bg", color.NRGBA{255, 255, 255, 36}, color.NRGBA{127, 211, 216, 41}},
		{"sidebar-indicator", rgb(0xffd45e), rgb(0xffd45e)},
		{"sidebar-focus", rgb(0xffffff), rgb(0xffffff)},
		{"sidebar-primary", rgb(0xffffff), rgb(0x3cc4cc)},
		{"sidebar-on-primary", rgb(0x0c5f68), rgb(0x04161a)},
		// No empieza por `border`, así que no se aplana: va sobre el carril, no sobre
		// `surface`, y lo compone quien lo pinta.
		{"sidebar-border", color.NRGBA{255, 255, 255, 41}, color.NRGBA{255, 255, 255, 31}},
		{"topbar-bg", rgb(0xffffff), rgb(0x0d1415)},
		{"header-bg", rgb(0xf1f6f6), rgb(0x182325)},
		{"on-chrome", rgb(0x0f1d20), rgb(0xe7efef)},
		{"chrome-hover", rgb(0xe8efef), rgb(0x243336)},
		{"overlay", color.NRGBA{15, 29, 32, 92}, color.NRGBA{0, 0, 0, 153}},
		{"danger", rgb(0xc34134), rgb(0xc34134)},
		{"danger-text", rgb(0xa3342a), rgb(0xf08f84)},
		{"danger-bg", rgb(0xfbeae7), rgb(0x3a1a16)},
		// «Info» sigue siendo AZUL aunque el primario sea otro color: ya no es su alias.
		{"info", rgb(0x1a6fbb), rgb(0x1a6fbb)},
		{"info-text", rgb(0x165c9c), rgb(0x7db8ec)},
		{"info-bg", rgb(0xeaf3fb), rgb(0x0f2a44)},
		{"product-mail", rgb(0xdc3450), rgb(0xdc3450)},
		{"avatar-6", rgb(0x6a58b8), rgb(0x6a58b8)},
		{"orgmark-1", rgb(0x9c743a), rgb(0x9c743a)},
		{"orgmark-12", rgb(0xa76c51), rgb(0xa76c51)},
	}
	for _, c := range casos {
		if got := Color(c.nombre, Light); got != c.claro {
			t.Errorf("%s claro = %v, quiero %v", c.nombre, got, c.claro)
		}
		if got := Color(c.nombre, Dark); got != c.oscuro {
			t.Errorf("%s oscuro = %v, quiero %v", c.nombre, got, c.oscuro)
		}
	}
}

func TestTodosLosTokensExistenEnLasTresApariencias(t *testing.T) {
	nombres := Names()
	if len(nombres) < 150 {
		t.Fatalf("solo %d tokens: el parser se está dejando bloques de la hoja", len(nombres))
	}
	for _, n := range nombres {
		for _, v := range []Variant{Dark, Translucent} {
			if _, ok := paletas[v][n]; !ok {
				t.Errorf("%s está en claro y no en la apariencia %d", n, v)
			}
		}
	}
	for _, n := range []string{"ink-rgb", "selected-strength"} {
		if _, ok := paletas[Light][n]; ok {
			t.Errorf("%s no es un color y ha entrado en la paleta", n)
		}
	}
}

// El guardián tiene que fallar: una hoja que no se sabe leer no puede dar una paleta
// a medias.
func TestLaHojaIlegibleNoArranca(t *testing.T) {
	const oscuro = "\n[data-theme='dark'] { --sytue-b: #000000; }\n[data-theme='translucent'] { --sytue-b: #ffffff; }"
	for nombre, css := range map[string]string{
		"var sin definir":   ":root { --sytue-a: var(--sytue-no-existe); }" + oscuro,
		"sintaxis nueva":    ":root { --sytue-a: hsl(200 50% 50%); }" + oscuro,
		"ciclo":             ":root { --sytue-a: var(--sytue-b); --sytue-b: var(--sytue-a); }" + oscuro,
		"sin tema oscuro":   ":root { --sytue-a: #ffffff; }",
		"canal de 256":      ":root { --sytue-a: rgba(256, 0, 0, 1); }" + oscuro,
		"alfa fuera de 1":   ":root { --sytue-a: rgba(0, 0, 0, 1.5); }" + oscuro,
		"hex de tres":       ":root { --sytue-a: #fff; }" + oscuro,
		"bloque sin cerrar": ":root { --sytue-a: #ffffff;",
	} {
		if _, err := parsePaletas(css); err == nil {
			t.Errorf("%s: se ha dado por buena", nombre)
		}
	}
}

// ParseColor es la puerta de los colores que llegan como DATO, y admite más formas que
// la hoja de tokens: las que escribe un navegador en el contenido de la gente. Que las
// dos cosas se separen es justo lo que vigila TestLaHojaIlegibleNoArranca de aquí arriba.
func TestParseColorEntiendeLoQueEscribeUnNavegador(t *testing.T) {
	for css, quiero := range map[string]color.NRGBA{
		"#16734d":          {R: 0x16, G: 0x73, B: 0x4d, A: 0xff},
		"#abc":             {R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff},
		"  #ABC  ":         {R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff},
		"black":            {A: 0xff},
		"White":            {R: 0xff, G: 0xff, B: 0xff, A: 0xff},
		"rgb(22, 163, 74)": {R: 22, G: 163, B: 74, A: 0xff},
		"transparent":      {},
	} {
		if c, ok := ParseColor(css); !ok || c != quiero {
			t.Errorf("ParseColor(%q) = %v, %v; quiero %v", css, c, ok, quiero)
		}
	}
	for _, css := range []string{"", "#ab", "#abcd", "azulito", "rgb(300, 0, 0)", "var(--sytue-primary)"} {
		if c, ok := ParseColor(css); ok {
			t.Errorf("ParseColor(%q) se ha dado por bueno: %v", css, c)
		}
	}
}

func TestPanicoConUnTokenQueNoExiste(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("un token inventado ha devuelto un color")
		}
	}()
	Color("primario-500", Light)
}

// tokens/colors.css es una COPIA (scripts/sync-branding.sh). Si alguien la edita aquí,
// o cambia la fuente y no sincroniza, esto lo dice.
func TestLaCopiaDeLaHojaEsLaDeBranding(t *testing.T) {
	if webref.Read(t, "branding/tokens/colors.css") != coloresCSS {
		t.Error("tokens/colors.css difiere de branding/tokens/colors.css: ejecutar scripts/sync-branding.sh")
	}
	if webref.Read(t, "branding/tokens/appearance.css") != aparienciaCSS {
		t.Error("tokens/appearance.css difiere de branding/tokens/appearance.css: ejecutar scripts/sync-branding.sh")
	}
	if webref.Read(t, "apps/frontend/packages/config/src/theme.css") != temaCSS {
		t.Error("tokens/theme.css difiere de la de @sytue/config: ejecutar scripts/sync-branding.sh")
	}
}

// TestLaSegundaHojaPisaAlOscuroDeLaPrimera: `:root` y `[data-theme='dark']` pesan lo
// mismo en CSS, así que el `:root` de appearance.css, que llega después, gana al bloque
// oscuro de colors.css. Es la razón de que esa hoja repita cada token del carril en sus
// cuatro bloques, y si aquí se apilaran todos los `:root` primero el carril oscuro
// saldría con el valor de la paleta.
//
// Visto en ROJO: saltándose el `:root` de la segunda hoja, el paquete no pasa (el carril
// se queda sin `sidebar-indicator`, que solo se declara ahí, y TestColorEnLosDosTemas
// entra en pánico).
func TestLaSegundaHojaPisaAlOscuroDeLaPrimera(t *testing.T) {
	const primera = `:root { --sytue-a: #111111; --sytue-b: #111111; }
 [data-theme='dark'] { --sytue-a: #222222; --sytue-b: #222222; }
 [data-theme='translucent'] { --sytue-a: #333333; }`
	const segunda = `:root { --sytue-a: #444444; --sytue-b: #444444; }
 [data-theme='dark'] { --sytue-b: #555555; }`
	raw, err := parseCrudas(primera, segunda)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		v      Variant
		token  string
		quiero color.NRGBA
	}{
		{Light, "a", rgb(0x444444)},
		{Dark, "a", rgb(0x444444)}, // el :root de la segunda, no el oscuro de la primera
		{Dark, "b", rgb(0x555555)},
		{Translucent, "a", rgb(0x444444)}, // a la segunda le falta el bloque y no es un error
	} {
		if got := raw[c.v][c.token]; got != c.quiero {
			t.Errorf("apariencia %d, %s = %v; quiero %v", c.v, c.token, got, c.quiero)
		}
	}
}

// TestCrudoGuardaElAlfaQueColorAplana: el borde de la barra y del lateral se compone
// sobre la tinta del cromo, no sobre `surface`, y para eso hace falta el token tal cual.
// Y aplanar para `Color` no puede estropear el crudo: comparten origen.
func TestCrudoGuardaElAlfaQueColorAplana(t *testing.T) {
	crudo := Crudo("border", Light)
	if crudo.A == 0xff || crudo.R != 15 || crudo.G != 29 || crudo.B != 32 {
		t.Errorf("el crudo de `border` es %v y la hoja dice rgba(15, 29, 32, .12)", crudo)
	}
	if Color("border", Light).A != 0xff {
		t.Error("`Color` tiene que seguir dando los bordes aplanados")
	}
	if got, want := SobreSRGB(crudo, Color("surface", Light)), Color("border", Light); got != want {
		t.Errorf("aplanar el crudo sobre `surface` da %v y `Color` %v", got, want)
	}
}

// TestLosTokensSonLosDeLaWeb compara la paleta de las apps nativas con la que pinta la
// web, leída de las hojas que importa `@sytue/config/theme.css` (y con él `@sytue/ui`) —no
// de branding—: colors.css y, encima, appearance.css, en claro y en oscuro, y el oscuro por sus DOS puertas: la elección
// explícita (`[data-theme='dark']`) y la del sistema (`prefers-color-scheme`). Las apps
// nativas pintan la explícita, y quien deja el tema en «Sistema» ve en la web la otra:
// si un token se toca en un bloque y no en el otro, las dos webs dejan de ser la misma
// y el escritorio solo se parece a una.
//
// Visto en ROJO: cambiando `--sytue-text-muted` a #a5b6be solo en el bloque del sistema
// de la copia de @sytue/config, falla con «text-muted».
func TestLosTokensSonLosDeLaWeb(t *testing.T) {
	css := webref.Read(t, "apps/frontend/packages/config/src/tokens/colors.css")
	forma := webref.Read(t, "apps/frontend/packages/config/src/tokens/appearance.css")
	for _, puerta := range []string{oscuroElegido, oscuroDelSistema} {
		crudasWeb, err := parseCrudasCon(puerta, css, forma)
		if err != nil {
			t.Fatalf("%s: %v", puerta, err)
		}
		web := aplanar(crudasWeb)
		for v, nombre := range []string{"claro", "oscuro", "traslúcido"} {
			if len(web[v]) != len(paletas[v]) {
				t.Errorf("%s %s: la web tiene %d tokens y las nativas %d", nombre, puerta, len(web[v]), len(paletas[v]))
			}
			for n, c := range web[v] {
				if got, ok := paletas[v][n]; !ok || got != c {
					t.Errorf("%s (%s) --sytue-%s: nativas %v, web %v", nombre, puerta, n, got, c)
				}
			}
		}
	}
}

// TestElColorEsUnEjeAparteDelModo: la hoja declara el azul cobalto como `data-color`, un
// eje aparte del tema (ADR 0071), y las nativas lo leen de la MISMA hoja. Los esperados
// están sacados a mano de los bloques de cobalto de colors.css y de appearance.css.
//
// VISTO EN ROJO con `ElegirColor` devolviendo siempre el juego de la casa: «elegir el
// cobalto no ha cambiado nada».
func TestElColorEsUnEjeAparteDelModo(t *testing.T) {
	t.Cleanup(func() { ElegirColor("") })
	hay := false
	for _, c := range Colores() {
		hay = hay || c == "cobalto"
	}
	if !hay {
		t.Fatalf("la hoja declara %v y falta el cobalto", Colores())
	}
	if !ElegirColor("cobalto") {
		t.Fatal("elegir el cobalto no ha cambiado nada")
	}
	if ElegirColor("cobalto") {
		t.Error("elegir el color que ya está dice que ha cambiado: se repintaría sin motivo")
	}
	for _, c := range []struct {
		nombre                    string
		claro, oscuro, traslucido color.NRGBA
	}{
		// El bloque claro de cobalto de colors.css; los modos no lo redeclaran.
		{"primary-600", rgb(0x3b5fe0), rgb(0x3b5fe0), rgb(0x3b5fe0)},
		// El carril, de appearance.css. En oscuro colors.css también lo declara, con DOS
		// atributos (#121622): gana el de appearance.css, que lleva los mismos dos y va
		// después.
		{"sidebar-bg", rgb(0x3355d6), rgb(0x141d3d), color.NRGBA{44, 76, 204, 237}},
		// En claro colors.css dice #141a2b con UN atributo: lo pisa el `:root` de
		// appearance.css, que pesa lo mismo y llega después.
		{"sidebar-fg", rgb(0xffffff), rgb(0xe9ecf5), rgb(0xffffff)},
		{"sidebar-on-primary", rgb(0x3355d6), rgb(0xffffff), rgb(0x3355d6)},
		{"sidebar-indicator", rgb(0xffd45e), rgb(0xffd45e), rgb(0xffd45e)},
		// Lo que el color NO toca: los de estado y los de producto.
		{"danger", Color("danger", Light), Color("danger", Dark), Color("danger", Translucent)},
		{"product-mail", rgb(0xdc3450), rgb(0xdc3450), rgb(0xdc3450)},
	} {
		for v, quiero := range []color.NRGBA{c.claro, c.oscuro, c.traslucido} {
			if got := Color(c.nombre, Variant(v)); got != quiero {
				t.Errorf("cobalto, apariencia %d: %s = %v, quiero %v", v, c.nombre, got, quiero)
			}
		}
	}
	// Cualquier otra cosa devuelve el de la casa: su nombre, el vacío o uno que no existe.
	if !ElegirColor("petroleo") || Color("sidebar-bg", Light) != rgb(0x0c5f68) {
		t.Errorf("«petroleo» no devuelve el color de la casa: sidebar-bg = %v", Color("sidebar-bg", Light))
	}
	if ElegirColor("fucsia") || ElegirColor("") {
		t.Error("un color que la hoja no declara, o el vacío, tienen que dejar el de la casa sin cambiar nada")
	}
}

// TestElColorConTemaGanaATodoLoDeUnAtributo: `[data-color][data-theme]` lleva dos
// atributos y gana a cualquier bloque de uno, esté en la hoja que esté; entre los de uno
// manda el orden, hoja a hoja.
//
// VISTO EN ROJO apilando el bloque de dos atributos dentro de la pasada de su hoja: `b`
// oscuro sale #555555, el `:root` de la segunda.
func TestElColorConTemaGanaATodoLoDeUnAtributo(t *testing.T) {
	const primera = `:root { --sytue-a: #111111; --sytue-b: #111111; }
 [data-theme='dark'] { --sytue-a: #222222; }
 [data-theme='translucent'] { --sytue-a: #222222; }
 [data-color='x'] { --sytue-a: #333333; --sytue-b: #333333; }
 [data-color='x'][data-theme='dark'] { --sytue-a: #444444; --sytue-b: #444444; }
 [data-color='x'][data-theme='translucent'] { --sytue-a: #444444; }`
	const segunda = `:root { --sytue-a: #555555; --sytue-b: #555555; }
 [data-color='x'] { --sytue-a: #666666; }
 [data-color='x'][data-theme='dark'] { --sytue-a: #777777; }`
	raw, err := parseCrudasDe(oscuroElegido, pedido{color: "x"}, primera, segunda)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		v      Variant
		token  string
		quiero color.NRGBA
	}{
		{Light, "a", rgb(0x666666)}, // el color de la segunda, que es el último de un atributo
		{Light, "b", rgb(0x555555)}, // el :root de la segunda pisa al color de la primera
		{Dark, "a", rgb(0x777777)},  // dos atributos, y el de la segunda va después
		{Dark, "b", rgb(0x444444)},  // dos atributos de la primera ganan al :root de la segunda
	} {
		if got := raw[c.v][c.token]; got != c.quiero {
			t.Errorf("apariencia %d, %s = %v; quiero %v", c.v, c.token, got, c.quiero)
		}
	}
}

// TestElColorDeCadaApp: con el color «el de cada app» el carril y la acción toman el
// color del producto. La hoja trae la rampa de cada uno en su bloque
// (`[data-color='app'][data-app='drive']`) y el reparto por modo una sola vez; un
// producto sin bloque —el Hub— se queda con la rampa de la casa. Y una app de icono
// ROJO —Mail— solo tiñe el carril: su bloque trae la rampa `rail-*` y deja el primario
// en el de la casa, para que la acción no se confunda con el rojo de peligro (ADR 0072).
// Los esperados, a mano de los bloques de Mail y de Drive de colors.css y del carril de
// cada app de appearance.css.
//
// VISTO EN ROJO sin el bloque del producto en la cascada: Mail pinta el carril petróleo.
func TestElColorDeCadaApp(t *testing.T) {
	t.Cleanup(func() { ElegirColor(""); ElegirApp("") })
	deLaCasa := Color("primary", Light)
	ElegirApp("mail")
	if Color("primary", Light) != deLaCasa {
		t.Error("decir el producto ha cambiado el color sin haber elegido «el de cada app»")
	}
	if !ElegirColor(ColorDeCadaApp) {
		t.Fatal("elegir el color de cada app no ha cambiado nada")
	}
	for _, c := range []struct {
		nombre        string
		claro, oscuro color.NRGBA
	}{
		{"primary-600", rgb(0x0f7680), rgb(0x0f7680)}, // la rampa del primario es la de la casa
		{"primary", rgb(0x0f7680), rgb(0x3cc4cc)},     // y la acción, con ella
		{"sidebar-bg", rgb(0xab283e), rgb(0x3d1921)},  // el carril: su 700, y en oscuro su 950
		{"sidebar-on-primary", rgb(0xab283e), rgb(0x04161a)},
		{"sidebar-indicator", rgb(0xffd45e), rgb(0xffd45e)},
	} {
		if got := Color(c.nombre, Light); got != c.claro {
			t.Errorf("Mail claro: %s = %v, quiero %v", c.nombre, got, c.claro)
		}
		if got := Color(c.nombre, Dark); got != c.oscuro {
			t.Errorf("Mail oscuro: %s = %v, quiero %v", c.nombre, got, c.oscuro)
		}
	}
	// Otro producto, otro color —y Drive, que no es rojo, lo lleva también a la acción—;
	// y el que no tiene rampa propia, el de la casa.
	mail := Color("sidebar-bg", Light)
	if !ElegirApp("drive") || Color("sidebar-bg", Light) == mail {
		t.Error("Drive pinta el carril del color de Mail")
	}
	if got, quiero := Color("primary", Light), rgb(0x16734d); got != quiero {
		t.Errorf("Drive claro: primary = %v, quiero su 600, %v", got, quiero)
	}
	if got, quiero := Color("sidebar-bg", Light), rgb(0x136544); got != quiero {
		t.Errorf("Drive claro: sidebar-bg = %v, quiero su 700, %v", got, quiero)
	}
	ElegirApp("one")
	if got := Color("primary", Light); got != deLaCasa {
		t.Errorf("el Hub, que no tiene rampa en la hoja, pinta %v y es el de la casa, %v", got, deLaCasa)
	}
}
