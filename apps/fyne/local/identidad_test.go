// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// appsLocales son las quince apps de Sytue Office (PLAN-EDICION-LOCAL §5.2): su carpeta
// en `cmd-local` y su producto, la clave de la web con que se llama a `local.Abrir` y se
// forma su esquema (`sytueoffice-<producto>`). Por igualdad: una app nueva se añade aquí.
var appsLocales = map[string]string{
	"bases": "bases", "calcus": "sheets", "calendar": "calendar", "cast": "cast", "certipdf": "pdf",
	"contacts": "contacts", "diapos": "slides", "docus": "docs", "drive": "drive", "erp": "erp",
	"hub": "one", "mail": "mail", "notes": "notes", "photos": "photos", "rooms": "rooms",
}

// identidad es lo que un FyneApp.toml dice de su app.
type identidad struct {
	nombre, id, icono, esquema, bundle string
	extensiones, extra                 []string
}

var reCampo = regexp.MustCompile(`(?m)^\s*(Name|ID|Icon|esquema|bundle|extensiones|esquemas_extra)\s*=\s*"([^"]*)"`)

// identidades lee los FyneApp.toml de un árbol (`cmd` o `cmd-local`), por carpeta.
func identidades(t *testing.T, arbol string) map[string]identidad {
	t.Helper()
	tomls, _ := filepath.Glob(filepath.Join("..", arbol, "*", "FyneApp.toml"))
	out := map[string]identidad{}
	for _, f := range tomls {
		crudo, err := os.ReadFile(f) //nolint:gosec // el propio repositorio
		if err != nil {
			t.Fatal(err)
		}
		var id identidad
		for _, m := range reCampo.FindAllStringSubmatch(string(crudo), -1) {
			switch m[1] {
			case "Name":
				id.nombre = m[2]
			case "ID":
				id.id = m[2]
			case "Icon":
				id.icono = m[2]
			case "esquema":
				id.esquema = m[2]
			case "bundle":
				id.bundle = m[2]
			case "extensiones":
				id.extensiones = strings.Split(m[2], ",")
			case "esquemas_extra":
				id.extra = strings.Split(m[2], ",")
			}
		}
		out[filepath.Base(filepath.Dir(f))] = id
	}
	return out
}

// sinLaConectada dice si este es un árbol autónomo de Sytue Office: sin la edición conectada
// al lado (sin apps/fyne/cmd) y con el vendor/ de su espacio de trabajo en la raíz. Allí no
// hay con qué comparar lo de la edición con lo de la conectada (identidades, color de los
// iconos): lo vigila sytue-one, de donde sale ese árbol. En sytue-one es siempre falso, y una
// app conectada que falte sigue siendo rojo.
func sinLaConectada() bool {
	_, cmd := os.Stat(filepath.Join("..", "cmd"))
	_, vendor := os.Stat(filepath.Join("..", "..", "..", "vendor", "modules.txt"))
	return errors.Is(cmd, fs.ErrNotExist) && vendor == nil
}

// llamadasConTexto son los primeros argumentos de texto de cada `<paquete>.<funcion>(…)`
// en los ficheros del paquete de dir que no son de pruebas, con el argumento n-ésimo.
func llamadasConTexto(t *testing.T, dir, paquete, funcion string, n int) []string {
	t.Helper()
	ficheros, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	var out []string
	for _, f := range ficheros {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		arbol, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(arbol, func(nodo ast.Node) bool {
			c, ok := nodo.(*ast.CallExpr)
			if !ok || len(c.Args) <= n {
				return true
			}
			sel, ok := c.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != funcion {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); !ok || x.Name != paquete {
				return true
			}
			if lit, ok := c.Args[n].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				v, _ := strconv.Unquote(lit.Value)
				out = append(out, v)
			} else {
				out = append(out, "<no es un texto>")
			}
			return true
		})
	}
	return out
}

// TestCadaAppLocalDeclaraSuIdentidad (§8.1) es, para `cmd-local`, el espejo de
// `sesion/esquemas_test.go`: las quince apps, cada una con el identificador, el esquema,
// el instalador y el nombre de la edición en su FyneApp.toml; su `main` abre la cáscara
// con SU producto (el del esquema) y crea la app con su identificador; lo que declara en
// «Abrir con» lo atiende (AlAbrirFichero); y su icono es la marca de su producto.
//
// VISTO EN ROJO (2026-11-25): con `local.Abrir(a, "docs")` en Calcus y con el ID de Notes
// en `eu.sytue.notes`, nombró las dos apps y lo que esperaba.
func TestCadaAppLocalDeclaraSuIdentidad(t *testing.T) {
	ids := identidades(t, "cmd-local")
	sinConectada := sinLaConectada()
	if sinConectada {
		t.Log("sin apps/fyne/cmd (el árbol público): el color de cada icono contra el de la conectada lo mira sytue-one")
	}
	carpetas := make([]string, 0, len(ids))
	for c := range ids {
		carpetas = append(carpetas, c)
	}
	slices.Sort(carpetas)
	quiero := make([]string, 0, len(appsLocales))
	for c := range appsLocales {
		quiero = append(quiero, c)
	}
	slices.Sort(quiero)
	if !slices.Equal(carpetas, quiero) {
		t.Fatalf("cmd-local tiene %v; la edición son %v (appsLocales)", carpetas, quiero)
	}
	for app, producto := range appsLocales {
		id := ids[app]
		dir := filepath.Join("..", "cmd-local", app)
		if quiero := espacio.PrefijoID + app; id.id != quiero {
			t.Errorf("cmd-local/%s: ID %q; quiero %q", app, id.id, quiero)
		}
		if quiero := espacio.Esquema(producto); id.esquema != quiero {
			t.Errorf("cmd-local/%s: esquema %q; quiero %q", app, id.esquema, quiero)
		}
		if id.bundle != "office" {
			t.Errorf("cmd-local/%s: va en el instalador %q, no en el de la edición (office)", app, id.bundle)
		}
		if !strings.HasPrefix(id.nombre, "Sytue ") || !strings.HasSuffix(id.nombre, " Office") {
			t.Errorf("cmd-local/%s: se llama %q; quiero «Sytue <producto> Office»", app, id.nombre)
		}
		if got := llamadasConTexto(t, dir, "local", "Abrir", 1); !slices.Equal(got, []string{producto}) {
			t.Errorf("cmd-local/%s: local.Abrir con %q; quiero una vez, con %q", app, got, producto)
		}
		for _, got := range llamadasConTexto(t, dir, "local", "NuevaApp", 0) {
			if got != "" && got != id.id {
				t.Errorf("cmd-local/%s: local.NuevaApp(%q) y su ID es %q", app, got, id.id)
			}
		}
		for _, got := range llamadasConTexto(t, dir, "app", "NewWithID", 0) {
			if got != id.id {
				t.Errorf("cmd-local/%s: app.NewWithID(%q) y su ID es %q", app, got, id.id)
			}
		}
		if len(id.extensiones) > 0 && !paqueteContiene(t, dir, ".AlAbrirFichero(") {
			t.Errorf("cmd-local/%s declara %v en «Abrir con» y no llama a AlAbrirFichero: se ofrecería y no abriría nada", app, id.extensiones)
		}
		// El icono: un PNG cuadrado y grande, de la marca de SU producto (el color de la
		// tesela es el del icono de la app conectada, que vigila sesion/esquemas_test.go).
		local, err := leerPNG(filepath.Join(dir, id.icono))
		if err != nil {
			t.Errorf("cmd-local/%s: icono %q: %v", app, id.icono, err)
			continue
		}
		if b := local.Bounds(); b.Dx() != b.Dy() || b.Dx() < 512 {
			t.Errorf("cmd-local/%s: el icono es de %v; quiero cuadrado y de 512 o más", app, b.Size())
		}
		if sinConectada {
			continue
		}
		conectada, err := leerPNG(filepath.Join("..", "cmd", app, "Icon.png"))
		if err != nil {
			t.Errorf("cmd/%s: %v", app, err)
			continue
		}
		if a, b := teselaDe(local), teselaDe(conectada); a != b {
			t.Errorf("cmd-local/%s: el icono es de otro color (%v) que la marca de su producto (%v)", app, a, b)
		}
	}
}

func leerPNG(ruta string) (image.Image, error) {
	f, err := os.Open(ruta) //nolint:gosec // el propio repositorio
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// teselaDe es el color del cuadrado del icono, a un octavo de su esquina (dentro de la
// tesela y fuera del dibujo).
func teselaDe(img image.Image) [3]uint8 {
	b := img.Bounds()
	r, g, bl, _ := img.At(b.Min.X+b.Dx()/8, b.Min.Y+b.Dy()/8).RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8)}
}

func paqueteContiene(t *testing.T, dir, texto string) bool {
	t.Helper()
	ficheros, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, f := range ficheros {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		crudo, err := os.ReadFile(f) //nolint:gosec // el propio repositorio
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(crudo), texto) {
			return true
		}
	}
	return false
}

// rePrefijos son los prefijos de los ejecutables que pone `empaquetar.sh`: el de la
// edición (`SYTUE_ARBOL=cmd-local`) y el de la conectada.
var rePrefijos = regexp.MustCompile(`if \[ "\$ARBOL" = cmd-local \]; then PREFIJO="\$\{SYTUE_PREFIJO:-([^}]*)\}"; else PREFIJO="\$\{SYTUE_PREFIJO:-([^}]*)\}"; fi`)

// TestLaIdentidadLocalNoChocaConLaConectada (R-CONVIVE, §8.1): las dos ediciones se
// instalan a la vez sin pisarse. Identificadores (los del paquete de Android, el bundle
// de Apple y la clave de Windows), esquemas, ejecutables, nombres (la carpeta `.app`, la
// entrada del menú y la del instalador) e instaladores de `cmd-local` son disjuntos de
// los de `cmd`; ningún esquema local empieza por `sytue-` (la conectada decide por ese
// prefijo, `api/abrir.go`), y los esquemas de más que se registran (`mailto:`) no son de
// la suite.
//
// VISTO EN ROJO (2026-11-25): con Mail de la edición en `eu.sytue.mail` y el esquema
// `sytue-mail`, dijo que chocaban con `cmd/mail` y que el esquema empezaba por `sytue-`;
// con el prefijo de `empaquetar.sh` en `sytue-`, que los ejecutables chocaban.
func TestLaIdentidadLocalNoChocaConLaConectada(t *testing.T) {
	locales, conectadas := identidades(t, "cmd-local"), identidades(t, "cmd")
	if sinLaConectada() {
		// En sytue-office: lo propio de la edición, sí; el cruce con la conectada, en sytue-one.
		t.Log("sin apps/fyne/cmd (el árbol público): solo lo de la edición, sin cruzarlo con la conectada")
		conectadas = nil
	} else if len(conectadas) < len(appsLocales) {
		t.Fatalf("%d apps en cmd-local y %d en cmd", len(locales), len(conectadas))
	}
	if len(locales) != len(appsLocales) {
		t.Fatalf("%d apps en cmd-local y %d en cmd", len(locales), len(conectadas))
	}
	guion, err := os.ReadFile(filepath.Join("..", "scripts", "empaquetar.sh"))
	if err != nil {
		t.Fatal(err)
	}
	m := rePrefijos.FindStringSubmatch(string(guion))
	if m == nil {
		t.Fatal("empaquetar.sh ya no dice el prefijo de los ejecutables de cada árbol")
	}
	prefijoLocal, prefijoConectada := m[1], m[2]
	if prefijoLocal != espacio.PrefijoEjecutable {
		t.Errorf("empaquetar.sh pone %q a los ejecutables de la edición y espacio.PrefijoEjecutable es %q", prefijoLocal, espacio.PrefijoEjecutable)
	}
	// Cada clase de dato: lo de la conectada, y lo de cada app local.
	conectada := map[string]map[string]string{"ID": {}, "esquema": {}, "ejecutable": {}, "nombre": {}, "instalador": {}}
	for app, id := range conectadas {
		for clase, v := range map[string]string{"ID": id.id, "esquema": id.esquema, "ejecutable": prefijoConectada + app, "nombre": id.nombre, "instalador": id.bundle} {
			if v != "" {
				conectada[clase][v] = "cmd/" + app
			}
		}
	}
	vistos := map[string]string{}
	for app, id := range locales {
		for clase, v := range map[string]string{"ID": id.id, "esquema": id.esquema, "ejecutable": prefijoLocal + app, "nombre": id.nombre, "instalador": id.bundle} {
			if v == "" {
				t.Errorf("cmd-local/%s no declara su %s", app, clase)
				continue
			}
			if otra, ok := conectada[clase][v]; ok {
				t.Errorf("cmd-local/%s: su %s %q es el de %s", app, clase, v, otra)
			}
			if otra, ok := vistos[clase+" "+v]; ok && clase != "instalador" {
				t.Errorf("cmd-local/%s: su %s %q es también el de cmd-local/%s", app, clase, v, otra)
			}
			vistos[clase+" "+v] = app
		}
		if strings.HasPrefix(id.esquema, "sytue-") || !strings.HasPrefix(id.esquema, espacio.PrefijoEsquema) {
			t.Errorf("cmd-local/%s: el esquema %q no es de la edición (%s…; nunca sytue-…)", app, id.esquema, espacio.PrefijoEsquema)
		}
		if !strings.HasPrefix(id.id, espacio.PrefijoID) {
			t.Errorf("cmd-local/%s: el ID %q no empieza por %s", app, id.id, espacio.PrefijoID)
		}
		for _, e := range id.extra {
			if strings.HasPrefix(strings.ToLower(e), "sytue") {
				t.Errorf("cmd-local/%s registra el esquema de más %q, que es de la suite", app, e)
			}
		}
	}
	for app, id := range conectadas {
		if strings.HasPrefix(id.id, espacio.PrefijoID) || strings.HasPrefix(id.esquema, espacio.PrefijoEsquema) {
			t.Errorf("cmd/%s usa la identidad de la edición local (%s, %s)", app, id.id, id.esquema)
		}
	}
}

// appsSinPruebaDeArranque son las apps de `cmd-local` que aún no tienen su
// TestRecienInstaladaNoAbreNingunaConexion, con el motivo. Por igualdad.
var appsSinPruebaDeArranque = map[string]string{}

// TestCadaAppLocalSePruebaAlArrancar: cada app de `cmd-local` tiene la prueba de arranque
// (`local/prueba.Arrancar` con SU main, TestRecienInstaladaNoAbreNingunaConexion) y pide
// la app a [NuevaApp], que es lo que deja montarla en el driver de pruebas.
//
// VISTO EN ROJO (2026-11-25): borrando `cmd-local/docus/arranque_test.go`, la nombró; y
// quitando a Calendar de las excepciones, también.
func TestCadaAppLocalSePruebaAlArrancar(t *testing.T) {
	var sin []string
	for app := range appsLocales {
		dir := filepath.Join("..", "cmd-local", app)
		crudo, _ := os.ReadFile(filepath.Join(dir, "arranque_test.go")) //nolint:gosec // el propio repositorio
		prueba := strings.Contains(string(crudo), "func TestRecienInstaladaNoAbreNingunaConexion(") &&
			strings.Contains(string(crudo), `prueba.Arrancar(t, "`+app+`", main)`)
		pide := len(llamadasConTexto(t, dir, "local", "NuevaApp", 0)) == 1 && len(llamadasConTexto(t, dir, "app", "NewWithID", 0)) == 0 &&
			!paqueteContiene(t, dir, "app.New()")
		if !prueba || !pide {
			sin = append(sin, app)
		}
	}
	slices.Sort(sin)
	quiero := make([]string, 0, len(appsSinPruebaDeArranque))
	for app := range appsSinPruebaDeArranque {
		quiero = append(quiero, app)
	}
	slices.Sort(quiero)
	if !slices.Equal(sin, quiero) {
		t.Errorf("sin prueba de arranque (arranque_test.go con prueba.Arrancar, y la app de local.NuevaApp): %v; quiero exactamente %v (appsSinPruebaDeArranque)", sin, quiero)
	}
}
