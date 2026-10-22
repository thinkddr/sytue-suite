// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package catalogos

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// catalogo es, por SUBÁRBOL de `apps/fyne`, qué catálogo de app ve el código que hay
// ahí. Lo que no case con ninguna fila solo ve el común (`i18n.Common()`), que es lo
// correcto para `ui`, `tema` e `iconos`: piezas de la suite, no de un producto.
//
// El prefijo más LARGO gana. Cada lista es la composición real en su cmd:
// Hub carga Console antes de Portal para el catálogo OAuth nativo compartido.
var catalogo = map[string][]string{
	// El panel compartido carga Mail para cuentas y recibe el catálogo de su app.
	"vinculados":   {"mail", "calendar", "contacts", "notes"},
	"apps/mail":    {"mail"},
	"apps/hub":     {"console", "portal"},
	"apps/drive":   {"drive"},
	"apps/photos":  {"photos"},
	"apps/rooms":   {"rooms"},
	"cmd/rooms":    {"rooms"},
	"apps/notes":   {"notes"},
	"cmd/mail":     {"mail"},
	"cmd/hub":      {"console", "portal"},
	"cmd/drive":    {"drive"},
	"cmd/photos":   {"photos"},
	"cmd/notes":    {"notes"},
	"calendario":   {"calendar"},
	"contactos":    {"contacts"},
	"cmd/calendar": {"calendar"},
	"cmd/contacts": {"contacts"},
	"apps/erp":     {"erp"},
	"cmd/erp":      {"erp"},
	"apps/cast":    {"cast"},
	"cmd/cast":     {"cast"},
	"apps/bases":   {"bases"},
	"cmd/bases":    {"bases"},
	// Los cuatro editores comparten el catálogo de la ofimática (`@sytue/office`).
	"diapos":        {"office"},
	"editor":        {"office"},
	"calcus":        {"office"},
	"apps/certipdf": {"office"},
	"cmd/docus":     {"office"},
	"cmd/diapos":    {"office"},
	"cmd/calcus":    {"office"},
	"cmd/certipdf":  {"office"},
}

// fuenteWeb es dónde está en la web el catálogo de un directorio: el de una app, en
// esa app; el de la ofimática, en su paquete (`apps/frontend/packages/office`), que es
// el que comparten Docus, Calcus, Diapos y CertiPDF. La misma regla, en
// `scripts/gen-i18n.ts`.
func fuenteWeb(app string) string {
	if app == "office" {
		return "apps/frontend/packages/office"
	}
	return "apps/frontend/apps/" + app
}

// TestLosCatalogosSonLosDeLaWeb comprueba que cada catálogo embebido es la copia
// GENERADA de su `messages/en.ts` y no una edición a mano: las mismas claves que la
// web, y ni una de más.
//
// El directorio se llama como la app de la WEB justo para esto: la fuente con la que
// comparar sale del nombre, sin tabla que mantener.
//
// VISTO EN ROJO (2026-09-22, en el piloto): añadiendo `"zzz.inventada": "x"` a
// `mail/en.json` el test dice que esa clave no está en en.ts; y borrando
// `"action.add"` de `docs/en.json`, que hay que regenerar. Deshechos los dos.
func TestLosCatalogosSonLosDeLaWeb(t *testing.T) {
	apps := appsEmbebidas(t)
	if len(apps) < 4 {
		t.Fatalf("solo se ven %d catálogos (%v): el embed ha dejado de recogerlos", len(apps), apps)
	}
	// La web declara las claves entre comillas simples y con dos espacios de sangría:
	// `  'reader.subject':`.
	re := regexp.MustCompile(`(?m)^ {2}'([^']+)':`)
	for _, app := range apps {
		t.Run(app, func(t *testing.T) {
			fuente := webref.Read(t, fuenteWeb(app)+"/src/i18n/messages/en.ts")
			web := map[string]bool{}
			for _, m := range re.FindAllStringSubmatch(fuente, -1) {
				web[m[1]] = true
			}
			if len(web) < 100 {
				t.Fatalf("solo se han leído %d claves de %s/en.ts: el patrón ya no vale", len(web), app)
			}
			nuestro := claves(t, de(app), "en.json")
			for k := range nuestro {
				if !web[k] {
					t.Errorf("la clave %q no está en en.ts: el catálogo se ha editado a mano", k)
				}
			}
			for k := range web {
				if !nuestro[k] {
					t.Errorf("la clave %q está en en.ts y no en el JSON: regenerar con\n"+
						"  `bun apps/fyne/scripts/gen-i18n.ts %s` EN LINUX", k, app)
				}
			}
		})
	}
}

// TestLosIdiomasDeUnCatalogoTienenLoMismo: a un idioma al que le falte una clave se le
// cae al inglés en silencio, y eso es media pantalla en otro idioma sin que nada avise.
// Lo que NO se exige es que estén los 26: un idioma que la web todavía no tenga no se
// escribe a propósito (`i18n.New` carga siempre el inglés de respaldo).
func TestLosIdiomasDeUnCatalogoTienenLoMismo(t *testing.T) {
	for _, app := range appsEmbebidas(t) {
		sub := de(app)
		en := claves(t, sub, "en.json")
		for _, l := range i18n.Locales {
			if l == i18n.Default {
				continue
			}
			if _, err := fs.Stat(sub, string(l)+".json"); err != nil {
				continue // ese idioma no está generado todavía: cae al inglés
			}
			otro := claves(t, sub, string(l)+".json")
			for c := range en {
				if !otro[c] {
					t.Errorf("%s: a %s le falta %q", app, l, c)
				}
			}
			for c := range otro {
				if !en[c] {
					t.Errorf("%s: %s tiene %q y el inglés no", app, l, c)
				}
			}
		}
	}
}

// pendientes son las claves que el código nativo pide HOY y no existen en ningún
// catálogo. No son deuda teórica: `i18n.Translator.T` devuelve la clave cuando no la
// encuentra, así que cada una de estas SALE EN PANTALLA tal cual, con su punto y todo.
//
// Eran 59 el 2026-09-22, el día que este guardián se escribió. Quedan CINCO, y las
// otras 54 no se arreglaron escribiendo cadenas nuevas: se arreglaron apuntando al
// nombre que la clave ya tenía en la web. El editor nativo se había inventado un
// espacio de nombres entero —`ribbon.bold`, `ribbon.group.fuente`, `panel.close`,
// `estado.error.titulo`— para cosas que en `apps/frontend/packages/office` ya se llaman
// `format.bold`, `group.font`, `rail.close` y `editor.error.title`. Inventar la clave
// era el error; escribir la cadena habría sido duplicar la que ya existe.
//
// Tres de las cinco que quedan son NOMBRES ACCESIBLES de contenedor (`rail.label`,
// `header.label`, `status.label`). No tienen cadena en la web y no es un descuido: allí
// el carril, la cabecera y la barra de estado son `<aside>`, `<header>` y `<footer>`, y
// su nombre lo da el rol del landmark, no un texto. Fyne no tiene landmarks, así que
// aquí hace falta una cadena de verdad — y escribirla es una decisión de producto sobre
// la web, no un renombrado. Las otras dos (`panel.soon.*`) son el hueco de los cinco
// paneles que el port todavía no pinta y que en la web sí están.
//
// Se compara POR IGUALDAD: esta lista también se pone en rojo cuando una clave se
// resuelve y nadie borra su fila, que es como una lista de excepciones se convierte en
// un sitio donde esconder cosas.
var pendientes = map[string]string{
	"header.label":      "editor/cabecera.go",
	"panel.soon.texto":  "editor/marco.go",
	"panel.soon.titulo": "editor/marco.go",
	"rail.label":        "editor/carril.go",
	"status.label":      "editor/estado.go",
}

// TestLasClavesQueElCodigoPideExisten es el guardián de i18n de TODO `apps/fyne`.
//
// Por qué hace falta: `i18n.Translator.T` no explota con una clave que no existe —la
// devuelve tal cual— así que un dedazo no se ve al compilar ni al pasar los tests de
// píxel; se ve al abrir esa pantalla, en el idioma de esa persona. Es el mismo motivo
// por el que `tokens` entra en pánico con un color desconocido.
//
// Por qué CENTRAL y no uno por app: porque uno por app es lo que estaba pasando —el
// Hub escribió el suyo, Diapos el suyo con la lista a mano— y con diecisiete apps por
// delante eso son diecisiete copias que divergen (`docs/CONVENTIONS.md` §13.2). Aquí
// una app nueva no tiene que escribir nada: en cuanto su paquete existe, se le mira.
//
// Cómo encuentra las claves: todo literal de cadena con FORMA de clave en los `.go` que
// no son de test. Es más ancho que buscar solo las llamadas a `T(...)`, y a propósito:
// así entran también las tablas de claves —`ClaveTitulo()` del Hub, `claves` de
// Diapos— que una búsqueda por llamada no ve. Lo que no puede ver ninguna de las dos es
// una clave compuesta en tiempo de ejecución.
//
// VISTO EN ROJO (2026-09-22, en el piloto): cambiando `list.region` por `list.regionX`
// en `apps/mail/bandeja.go`, el test lo nombra con su fichero. Deshecho.
func TestLasClavesQueElCodigoPideExisten(t *testing.T) {
	comun := i18n.Common()
	porApp := map[string]map[string]bool{}
	for _, app := range appsEmbebidas(t) {
		porApp[app] = claves(t, de(app), "en.json")
	}
	delComun := claves(t, comun, "en.json")

	for prefijo, apps := range catalogo {
		for _, app := range apps {
			if _, hay := porApp[app]; !hay {
				t.Fatalf("la fila %q apunta al catálogo %q, que no está embebido: el guardián\n"+
					"  daría por buena cualquier clave de ese subárbol", prefijo, app)
			}
		}
	}

	faltan := map[string]string{}
	vistas := 0
	for _, u := range clavesDelArbol(t) {
		vistas++
		if delComun[u.clave] {
			continue
		}
		if slices.ContainsFunc(appDe(u.fichero), func(app string) bool { return porApp[app][u.clave] }) {
			continue
		}
		faltan[u.clave] = u.fichero
	}
	if vistas < 200 {
		t.Fatalf("solo se han visto %d claves en apps/fyne: el buscador está mirando el árbol equivocado", vistas)
	}

	for c, donde := range faltan {
		if _, sabido := pendientes[c]; !sabido {
			t.Errorf("%s pide la clave %q y no está en ningún catálogo: saldría en pantalla tal cual.\n"+
				"  Las claves no se inventan: se escriben en apps/frontend/apps/<app>/src/i18n/messages\n"+
				"  y se regeneran con `bun apps/fyne/scripts/gen-i18n.ts <app>` EN LINUX.", donde, c)
		}
	}
	// Por igualdad, como el resto de listas de excepciones del repositorio: una fila
	// que ya no corresponde a ninguna clave rota es una excepción huérfana esperando a
	// que alguien la reutilice sin pensar.
	for c, quien := range pendientes {
		if _, sigue := faltan[c]; !sigue {
			t.Errorf("pendientes guarda %q (%s) y esa clave ya existe: bórrala de la lista.", c, quien)
		}
	}
}

// ---- Ayudantes ----

// usoDeClave es una clave literal y el fichero, relativo a `apps/fyne`, donde aparece.
type usoDeClave struct{ clave, fichero string }

// formaDeClave es la de las claves de `@sytue/i18n`: minúscula inicial, camelCase y al
// menos un punto («reader.plainTextOnly», «slides.box.chart»).
var formaDeClave = regexp.MustCompile(`^[a-z][A-Za-z0-9]*(\.[A-Za-z0-9_]+)+$`)

// noSonClaves son las extensiones que la forma de clave no distingue de una: un
// «en.json» es indistinguible de «grupo.subgrupo» mirando solo el texto.
var noSonClaves = []string{".json", ".png", ".pdf", ".p12", ".pem", ".svg", ".go", ".ts", ".tsx", ".css", ".xml", ".toml", ".jar", ".eu", ".com", ".org", ".io", ".us"}

// literalesQueNoSonClaves son identificadores de almacenamiento que tienen la forma
// de una clave i18n, pero no se pasan al traductor. Mantenerlos fuera evita que el
// guardián pida inventar un texto que nunca se mostraría en pantalla.
var literalesQueNoSonClaves = map[string]bool{
	"calendar.recordatorios.anunciados": true,
	// El tipo de aviso que Rooms le manda al Hub por un directo (`rooms/avisosdirectos.go`).
	"rooms.message": true,
	// El tipo del aviso del Hub que desbloquea en vivo la franja de solo lectura
	// (`peticion.Opciones.Concedido`): se compara con el `type` que llega, no se
	// traduce. Su título sale de `notifications.kind.<tipo>` del catálogo común.
	"calendar.access.granted": true,
	"drive.access.granted":    true,
	"erp.access.granted":      true,
	"mail.access.granted":     true,
	"notes.access.granted":    true,
	// Clave de `fyne.Preferences` con la última versión anunciada en la campana
	// (`sesion/actualizar.go`): se guarda, no se traduce.
	"actualizacion.enLaCampana": true,
	// Tipos de aviso del Hub (`sesion/actualizar.go`, `ui/barraavisos.go`): se mandan o
	// se comparan con el `type`; el título sale de `notifications.kind.<tipo>`.
	"one.update.available":  true,
	"mail.message.received": true,
	// El programa que abre la carpeta de Drive de Windows (`cmd/hub/nube.go`).
	"explorer.exe": true,
}

// clavesDelArbol recorre `apps/fyne` y devuelve cada literal con forma de clave con el
// fichero donde está. Los `_test.go` quedan fuera: un test usa claves inventadas a
// propósito, para comprobar que el respaldo funciona.
func clavesDelArbol(t *testing.T) []usoDeClave {
	t.Helper()
	var usos []usoDeClave
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			if d.Name() == "testdata" || d.Name() == "locales" || d.Name() == "build" {
				return fs.SkipDir
			}
			return nil
		case !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go"):
			return nil
		}
		arbol, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("..", p)
		rel = filepath.ToSlash(rel)
		ast.Inspect(arbol, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil || !formaDeClave.MatchString(v) {
				return true
			}
			// `eu.sytue.calendar` es el identificador de paquete de `app.NewWithID`, en
			// DNS inverso: tiene forma de clave y no lo es. Va por prefijo y no por una
			// lista de TLD al principio, porque `io.button` —importar y exportar— SÍ es
			// una clave del catálogo de Contacts. Los nombres de D-Bus (`org.freedesktop.…`,
			// el portal de Wayland en `llamada/camara`) son lo mismo.
			if strings.HasPrefix(v, "eu.sytue.") || strings.HasPrefix(v, "org.freedesktop.") ||
				literalesQueNoSonClaves[v] ||
				slices.ContainsFunc(noSonClaves, func(e string) bool { return strings.HasSuffix(v, e) }) {
				return true
			}
			usos = append(usos, usoDeClave{v, rel})
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("recorriendo apps/fyne: %v", err)
	}
	return usos
}

// appDe dice qué catálogo de app ve un fichero, por el prefijo más largo de
// `catalogo`. Vacío = solo ve el común, que es lo correcto para `ui`, `tema` e
// `iconos`.
func appDe(fichero string) []string {
	var mejor []string
	largo := -1
	for prefijo, app := range catalogo {
		if strings.HasPrefix(fichero, prefijo+"/") && len(prefijo) > largo {
			mejor, largo = app, len(prefijo)
		}
	}
	return mejor
}

// appsEmbebidas son los directorios que `//go:embed */*.json` ha recogido, que es la
// lista de catálogos de verdad y no una escrita a mano al lado.
func appsEmbebidas(t *testing.T) []string {
	t.Helper()
	entradas, err := fs.ReadDir(catalogos, ".")
	if err != nil {
		t.Fatal(err)
	}
	var apps []string
	for _, e := range entradas {
		if e.IsDir() {
			apps = append(apps, e.Name())
		}
	}
	slices.Sort(apps)
	return apps
}

func claves(t *testing.T, sub fs.FS, fichero string) map[string]bool {
	t.Helper()
	datos, err := fs.ReadFile(sub, fichero)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(datos, &m); err != nil {
		t.Fatalf("%s: %v", fichero, err)
	}
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}
