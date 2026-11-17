// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pantalla

import (
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestElMuestrarioPintaTodasLasSecciones pinta cada sección en las 16 combinaciones del
// tema —2 colores × 4 modos × 2 formas, las del muestrario de la web— con el pintor de
// software, sin servidor gráfico: el lienzo mide lo que dice, el papel es el token del
// modo, la sección tiene algo encima del papel y las de capa abren su capa.
//
// Con `SYTUE_CAPTURAS=/dir` deja `<combinación>/<sección>.png` de cada una, con el nombre
// de combinación de la web (`cobalto-dark-carril`): es la cámara para revisar la paridad
// con la web sin ventana, una carpeta al lado de la otra.
func TestElMuestrarioPintaTodasLasSecciones(t *testing.T) {
	destino := os.Getenv("SYTUE_CAPTURAS")
	t.Cleanup(func() {
		tema.Elegir("")
		tema.ElegirColor("")
		tema.FormaDeLosGuardianes()
	})
	for _, comb := range combinacionesDelBarrido() {
		v := comb.Aplicar()
		if marco := comb.Forma == "marco"; tema.EsMarco() != marco {
			t.Fatalf("%s: la forma en vigor no es la de la combinación", comb.Nombre())
		}
		for _, clave := range Secciones() {
			img := pintarSeccion(t, comb.Nombre(), clave, v)
			if destino != "" {
				guardarPNG(t, filepath.Join(destino, comb.Nombre(), clave+".png"), img)
			}
		}
	}
	if _, _, ok := Muestrario("no-existe", tema.Claro); ok {
		t.Error("Muestrario acepta una sección que no existe")
	}
}

// combinacionesDelBarrido son las 16; con `-race`, una por modo que alterna color y
// forma, así cada valor de cada eje sigue pasando por todas las secciones (ver conRaza).
func combinacionesDelBarrido() []Combinacion {
	todas := Combinaciones()
	if !conRaza {
		return todas
	}
	var out []Combinacion
	for i, m := range modosMuestrario {
		c := Combinacion{coloresMuestrario[i%2], m, formasMuestrario[(i/2)%2]}
		if !slices.Contains(todas, c) {
			panic("combinación fuera del recorrido: " + c.Nombre())
		}
		out = append(out, c)
	}
	return out
}

// pintarSeccion monta la sección `clave` en la variante `v` (con el color y la forma ya
// aplicados), la captura y comprueba lo que toda sección tiene que cumplir.
func pintarSeccion(t *testing.T, comb, clave string, v tema.Variante) image.Image {
	t.Helper()
	contenido, abrir, ok := Muestrario(clave, v)
	if !ok {
		t.Fatalf("Secciones() da %q y Muestrario no la conoce", clave)
	}
	// La ventana de verdad no baja del mínimo de su contenido: si la sección no
	// cabe en el lienzo, `cmd/demo` la abre más grande que la captura.
	if m := contenido.MinSize(); m.Width > AnchoMuestrario || m.Height > AltoMuestrario {
		t.Errorf("%s en %s: la sección pide %v y el lienzo es %dx%d", clave, comb, m, AnchoMuestrario, AltoMuestrario)
	}
	w := test.NewWindow(contenido)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(AnchoMuestrario, AltoMuestrario))
	if abrir != nil {
		abrir(w.Canvas())
		if w.Canvas().Overlays().Top() == nil {
			t.Errorf("%s en %s: abrir no ha puesto ninguna capa sobre el lienzo", clave, comb)
		}
	}
	img := w.Canvas().Capture()
	w.Close()

	if b := img.Bounds(); b.Dx() != AnchoMuestrario || b.Dy() != AltoMuestrario {
		t.Fatalf("%s en %s: la captura mide %dx%d y el lienzo %dx%d", clave, comb, b.Dx(), b.Dy(), AnchoMuestrario, AltoMuestrario)
	}
	papel := tema.Color("bg", v)
	// El rincón de abajo a la derecha queda siempre libre; el diálogo y la paleta lo velan.
	if esquina := enNRGBA(img, AnchoMuestrario-2, AltoMuestrario-2); clave != "dialogo" && clave != "paleta" && papel.A == 0xff && !casi(esquina, papel) {
		t.Errorf("%s en %s: el papel pinta %v y el token `bg` es %v", clave, comb, esquina, papel)
	}
	// Por debajo de la cabecera, la sección tiene que pintar algo: una que se
	// queda en blanco porque un constructor pide algo que no le llega no avisa.
	if n := pixelesDistintos(img, papel, 90, 2000); n < 2000 {
		t.Errorf("%s en %s: solo %d píxeles distintos del papel bajo la cabecera; la sección no pinta", clave, comb, n)
	}
	return img
}

// TestLasCombinacionesSonLasDeLaWeb: los mismos 2 × 4 × 2 ejes, con los nombres de la web
// para las capturas, y cada eje llega de verdad a lo que se pinta.
//
// VISTO EN ROJO quitando `tema.ElegirColor` de `Aplicar`: «petroleo y cobalto pintan el
// mismo primary».
func TestLasCombinacionesSonLasDeLaWeb(t *testing.T) {
	t.Cleanup(func() {
		tema.Elegir("")
		tema.ElegirColor("")
		tema.FormaDeLosGuardianes()
	})
	combs := Combinaciones()
	if len(combs) != 16 {
		t.Fatalf("hay %d combinaciones y son 2 colores × 4 modos × 2 formas", len(combs))
	}
	nombres := map[string]bool{}
	for _, c := range combs {
		nombres[c.Nombre()] = true
	}
	for _, n := range []string{"petroleo-light-marco", "cobalto-dark-carril", "petroleo-translucent-marco", "cobalto-system-carril"} {
		if !nombres[n] {
			t.Errorf("falta la combinación %q, el nombre que le da la web", n)
		}
	}
	if len(nombres) != 16 {
		t.Errorf("las combinaciones repiten nombre: %d distintos", len(nombres))
	}
	primario := map[string][4]uint8{}
	for _, color := range coloresMuestrario {
		v := Combinacion{color, "claro", "marco"}.Aplicar()
		primario[color] = enNRGBA(image.NewUniform(tema.Color("primary", v)), 0, 0)
	}
	if primario["petroleo"] == primario["cobalto"] {
		t.Errorf("petroleo y cobalto pintan el mismo primary %v: el color no llega a los tokens", primario["petroleo"])
	}
	if v := (Combinacion{"petroleo", "oscuro", "carril"}).Aplicar(); v != tema.Oscuro || tema.EsMarco() {
		t.Errorf("oscuro y carril dan la variante %v con marco=%v", v, tema.EsMarco())
	}
}

// TestCadaConstructorDeUIEstaEnElMuestrario es el guardián: lee con `go/parser` los
// `func Nuevo*` exportados de `ui` y exige que cada uno salga en `muestrario.go` o en
// `fueraDelMuestrario`, con su motivo. Y al revés: una exclusión de un constructor que
// ya no existe, o que ya sale en el muestrario, es una exclusión rancia.
//
// VISTO EN ROJO quitando `ui.NuevaInsignia` de la sección de insignias: «NuevaInsignia
// no sale en el muestrario».
func TestCadaConstructorDeUIEstaEnElMuestrario(t *testing.T) {
	constructores := constructoresDeUI(t)
	if len(constructores) < 50 {
		t.Fatalf("solo %d constructores en ../ui: el guardián no está leyendo el paquete", len(constructores))
	}
	usados := usadosEnElMuestrario(t)
	for _, c := range constructores {
		_, fuera := fueraDelMuestrario[c]
		switch {
		case usados[c] && fuera:
			t.Errorf("%s ya sale en el muestrario: quítalo de fueraDelMuestrario", c)
		case !usados[c] && !fuera:
			t.Errorf("%s no sale en el muestrario: añádelo a su sección (o a fueraDelMuestrario, con el motivo)", c)
		}
	}
	for c, motivo := range fueraDelMuestrario {
		if !slices.Contains(constructores, c) {
			t.Errorf("fueraDelMuestrario nombra %s, que ya no es un constructor de ui", c)
		}
		if strings.TrimSpace(motivo) == "" {
			t.Errorf("fueraDelMuestrario deja fuera %s sin motivo", c)
		}
	}
}

// fueraDelMuestrario son los constructores de `ui` que no tienen sitio en el muestrario,
// con el porqué. Casi todos son de dos clases: piezas internas de otro widget, que ya se
// ven dentro de él, o el armazón de una app, que necesita su sesión y su traductor y
// sale en las capturas de cada app (`TestLasCapturasDeLaApp`).
var fueraDelMuestrario = map[string]string{
	"NuevaAccionLateral":             "va en el pie del lateral de una app; sale en las capturas de cada app",
	"NuevaAccionLateralCon":          "variante de NuevaAccionLateral con glifo propio de la app",
	"NuevaAsaArrastre":               "es el asa de una fila arrastrable; sola no se ve",
	"NuevaBarraDeLote":               "barra de selección múltiple sobre una lista; sale en Drive y Mail",
	"NuevaBarraMovil":                "barra inferior del teléfono, a ancho de ventana; la cubre armazon_tactil_test.go",
	"NuevaBarraSuite":                "necesita el traductor y la sesión de la suite",
	"NuevaCabeceraApp":               "cabecera de una app, a ancho de ventana; sale en las capturas de cada app",
	"NuevaCaja":                      "primitiva de relleno y borde: la usan todos los widgets del muestrario",
	"NuevaFilaLateral":               "fila del lateral, sobre su tinta; el muestrario de aros (aros.go) la enseña",
	"NuevaFilaPulsable":              "envoltorio pulsable de un contenido ajeno; no tiene aspecto propio",
	"NuevaMuestraColor":              "pastilla del selector de color de Calendar y Notes; sale en sus capturas",
	"NuevaPaginaNoEncontrada":        "página entera con el traductor de la suite",
	"NuevaPaginaSinSesion":           "página entera con el traductor de la suite",
	"NuevaPiezaCasilla":              "es la caja dibujada de la casilla y el radio, que salen en «conmutador»",
	"NuevaPista":                     "el globo sale al pasar el puntero, y el pintor de pruebas no tiene puntero",
	"NuevaRuedaCon":                  "variante de NuevaRueda con un color suelto en vez de un token",
	"NuevaSeccionLateral":            "sección del lateral de una app; sale en las capturas de cada app",
	"NuevaSeccionLateralArrastrable": "variante arrastrable de NuevaSeccionLateral",
	"NuevaSemanaFecha":               "tira de la semana del calendario del teléfono, con su idioma y su zona",
	"NuevaSombra":                    "primitiva: la sombra de las tarjetas, avisos y menús del muestrario",
	"NuevaSombraCon":                 "variante de NuevaSombra con capas a medida",
	"NuevaTabla":                     "lista virtual que pide su alto; la paridad la cubre datos_test.go",
	"NuevaTeselaTipo":                "no es un objeto de lienzo: es una pieza que pintan las filas de Drive de «filas»",
	"NuevaTiraDeslizable":            "contenedor de desplazamiento lateral; sin contenido de app no enseña nada",
	"NuevoArmazon":                   "marco entero de una app; sale en las capturas de cada app",
	"NuevoAro":                       "aro de foco: lo mide el muestrario de aros (aros.go)",
	"NuevoAroCon":                    "variante de NuevoAro con otro token: también en aros.go",
	"NuevoBordeDiscontinuo":          "primitiva de la zona de soltar archivos; sale en Drive",
	"NuevoBotonBarraMovil":           "botón de NuevaBarraMovil; va con ella",
	"NuevoDesplazaLado":              "contenedor de desplazamiento; no tiene aspecto propio",
	"NuevoEditorPlano":               "editor interno de Entrada y AreaTexto, que salen en «campos»",
	"NuevoEditorPlanoDesplazable":    "variante de NuevoEditorPlano",
	"NuevoEditorPlanoMultilinea":     "variante de NuevoEditorPlano",
	"NuevoEspaciado":                 "texto con espaciado de letra de las cabeceras de tabla; pieza interna",
	"NuevoFlotante":                  "capa flotante genérica sobre la que van el menú y los globos",
	"NuevoFondoSuite":                "fondo del modo traslúcido de una ventana entera; sale en las capturas de cada app",
	"NuevoGif":                       "necesita un GIF decodificado; lo usa Rooms",
	"NuevoImportarDatos":             "necesita el traductor y el servicio de integraciones",
	"NuevoLateral":                   "lateral entero de una app; sale en las capturas de cada app",
	"NuevoMesFecha":                  "calendario del selector de fecha, con su idioma y su zona; sale en Calendar",
	"NuevoParrafoConTinta":           "variante de NuevoParrafo con un color suelto en vez de un token",
	"NuevoTextoTabular":              "texto de cifras tabulares, pieza interna de tablas y contadores",
	"NuevoVelo":                      "el velo de detrás del diálogo, que sale en «dialogo»",
	"NuevoVersal":                    "texto en versalitas, pieza interna de cabeceras y rótulos",
	"NuevoVersalCon":                 "variante de NuevoVersal con otro espaciado",
}

// constructoresDeUI son los `func Nuev*` exportados y sin receptor de `../ui`, sin tests.
func constructoresDeUI(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	ficheros, err := filepath.Glob(filepath.Join("..", "ui", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range ficheros {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		arbol, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range arbol.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name.IsExported() && strings.HasPrefix(fn.Name.Name, "Nuev") {
				out = append(out, fn.Name.Name)
			}
		}
	}
	slices.Sort(out)
	return out
}

// usadosEnElMuestrario son los `ui.X` que nombra `muestrario.go`.
func usadosEnElMuestrario(t *testing.T) map[string]bool {
	t.Helper()
	arbol, err := parser.ParseFile(token.NewFileSet(), "muestrario.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	usados := map[string]bool{}
	ast.Inspect(arbol, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if paquete, ok := sel.X.(*ast.Ident); ok && paquete.Name == "ui" {
				usados[sel.Sel.Name] = true
			}
		}
		return true
	})
	return usados
}

func enNRGBA(img image.Image, x, y int) [4]uint8 {
	r, g, b, a := img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).RGBA()
	return [4]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)} //nolint:gosec // G115: 16 bits a 8
}

// casi compara un píxel con un token, con un escalón de holgura por el redondeo.
func casi(p [4]uint8, c interface{ RGBA() (r, g, b, a uint32) }) bool {
	q := enNRGBA(image.NewUniform(c), 0, 0)
	for i := range 3 {
		if d := int(p[i]) - int(q[i]); d > 1 || d < -1 {
			return false
		}
	}
	return true
}

// pixelesDistintos cuenta los píxeles por debajo de `desde` que no son el papel, hasta
// `tope`: al test le basta saber que la sección pinta, y contar los 450 000 de cada una
// de las 272 capturas (16 combinaciones × 17 secciones) con `-race` eran minutos.
func pixelesDistintos(img image.Image, papel interface{ RGBA() (r, g, b, a uint32) }, desde, tope int) int {
	q := enNRGBA(image.NewUniform(papel), 0, 0)
	n := 0
	b := img.Bounds()
	for y := b.Min.Y + desde; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := enNRGBA(img, x-b.Min.X, y-b.Min.Y)
			for i := range 3 {
				if d := int(p[i]) - int(q[i]); d > 1 || d < -1 {
					n++
					break
				}
			}
			if n >= tope {
				return n
			}
		}
	}
	return n
}

func guardarPNG(t *testing.T, ruta string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o750); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(ruta) //nolint:gosec // G304: la ruta la pone quien lanza el test
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
