// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package red

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// dondeSeVigila son las carpetas de Sytue Office cuyo código abre conexiones: el núcleo
// de la edición (con `local/cuentas`), los clientes de cuentas que comparte con la
// conectada y el servidor (`nucleo/cuentas`), la cáscara y las apps. El portero (`local/red`)
// es el único que marca de verdad y no se mira.
var dondeSeVigila = []string{"apps/nucleo/local", "apps/nucleo/cuentas", "apps/fyne/local", "apps/fyne/cmd-local"}

// prohibidoEnSelector es lo que no se nombra fuera del portero: los clientes y
// transportes de Go por defecto, sus atajos, los marcadores y los resolvedores de
// nombres (una consulta DNS también sale a la red).
var prohibidoEnSelector = map[string][]string{
	"net/http":   {"DefaultClient", "DefaultTransport", "Get", "Post", "PostForm", "Head"},
	"net":        {"Dial", "DialTimeout", "DialTCP", "DialUDP", "DialIP", "DialUnix", "Dialer", "LookupHost", "LookupIP", "LookupAddr", "LookupCNAME", "LookupMX", "LookupNS", "LookupSRV", "LookupTXT", "LookupPort", "DefaultResolver", "Resolver"},
	"crypto/tls": {"Dial", "DialWithDialer", "Dialer"},
}

// excepcionesDeRed son los únicos sitios que nombran algo de lo prohibido, con su motivo.
// Se comparan POR IGUALDAD (CONVENTIONS §13.4): uno que desaparece se borra de aquí, y uno
// nuevo se justifica aquí o pasa por el portero.
var excepcionesDeRed = map[string]string{
	"apps/fyne/local/instancia.go: net.Dialer":                              "el enchufe Unix de la copia única (EnchufeDe): no sale del equipo",
	"apps/fyne/local/cimientos.go: http.DefaultTransport":                   "la cáscara pone `red.Denegar` en su sitio (instalarPortero): lo que no pase por el portero falla cerrado",
	"apps/fyne/local/cimientos.go: http.DefaultClient":                      "ídem",
	"apps/fyne/local/prueba/arranque.go: http.DefaultTransport":             "la prueba de arranque comprueba que la app dejó `red.Denegar` y pone el de antes al acabar; solo la importan pruebas",
	"apps/nucleo/local/asistente/sistema_windows.go: http.Transport propio": "Foundry Local de Windows, SOLO en 127.0.0.1 o localhost (urlFoundry no casa otra cosa): no sale del equipo",
	"apps/nucleo/cuentas/correo/protocol.go: net.Dialer":                    "el respaldo de un Marcador vacío en un paquete que comparten el servidor (que inyecta el suyo, contra SSRF) y la edición conectada; ningún binario de Sytue Office lo enlaza",
	"apps/nucleo/cuentas/dav/dav.go: http.DefaultTransport":                 "el respaldo de un cliente sin transporte en un paquete compartido; la edición le pasa siempre el del portero (`agenda.Registro.cliente`) y, aun sin él, su `http.DefaultTransport` es `red.Denegar`",
	"apps/nucleo/local/chat/slack.go: websocket.Dial":                       "el Socket Mode de Slack con el HTTPClient de la cuenta (`s.HTTP`, el del portero)",
}

// TestTodoClienteDeRedPasaPorElPortero (R-RED, §8.1): fuera del portero, el código de
// Sytue Office no hace un `http.Client{}` sin transporte, ni un `http.Transport` propio, ni
// usa `http.DefaultClient`/`DefaultTransport`, `http.Get`/`Post`, `net.Dial*`,
// `net.Dialer`, `tls.Dial*`, el resolvedor de nombres ni el `Dial` de ninguna biblioteca
// (IMAP, SMTP, WebSocket…). Toda conexión sale de `l.Red.Cliente`/`Transporte`/`Marcar`
// o de `red.Denegar`: así la prueba de arranque (TestRecienInstaladaNoAbreNingunaConexion)
// ve todo lo que se intenta, y lo no configurado falla cerrado aunque falte la cáscara.
//
// VISTO EN ROJO (2026-11-25): con un `http.Get` en `local/nube` y un `&http.Client{}` en
// `local/asistente`, los nombró con su fichero y su línea; y quitando una excepción, dijo
// que sobraba.
func TestTodoClienteDeRedPasaPorElPortero(t *testing.T) {
	raiz := filepath.Join("..", "..", "..", "..")
	hallados := map[string][]string{} // «ruta: qué» → líneas
	for _, dir := range dondeSeVigila {
		err := filepath.WalkDir(filepath.Join(raiz, dir), func(ruta string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(ruta, raiz+string(filepath.Separator)))
			if d.IsDir() {
				if n := d.Name(); n == "testdata" || strings.HasPrefix(n, ".") || rel == "apps/nucleo/local/red" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, ruta, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			for _, h := range clientesSinPortero(fset, f) {
				clave := rel + ": " + h.que
				hallados[clave] = append(hallados[clave], strconv.Itoa(h.linea))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for clave, lineas := range hallados {
		if _, ok := excepcionesDeRed[clave]; !ok {
			t.Errorf("%s (línea %s): sale a la red sin el portero; usa l.Red.Cliente/Transporte/Marcar (o red.Denegar como respaldo), o justifícalo en excepcionesDeRed", clave, strings.Join(lineas, ", "))
		}
	}
	for clave := range excepcionesDeRed {
		if _, ok := hallados[clave]; !ok && !noExportado(raiz, clave) {
			t.Errorf("%s ya no está: quítalo de excepcionesDeRed", clave)
		}
	}
}

// noExportado dice si la excepción es de un fichero que no está porque este es un árbol
// autónomo de Sytue Office: allí solo va lo que algún sistema de la edición compila, así
// que falta lo que ningún binario suyo enlaza (`cuentas/correo/protocol.go`), y su
// excepción la mantiene sytue-one. Se reconoce por no tener la edición conectada al lado
// (sin apps/fyne/cmd) y sí el vendor/ de su espacio de trabajo; en sytue-one es siempre
// falso.
func noExportado(raiz, clave string) bool {
	fichero, _, _ := strings.Cut(clave, ": ")
	_, err := os.Stat(filepath.Join(raiz, filepath.FromSlash(fichero)))
	_, cmd := os.Stat(filepath.Join(raiz, "apps", "fyne", "cmd"))
	_, vendor := os.Stat(filepath.Join(raiz, "vendor", "modules.txt"))
	return errors.Is(err, fs.ErrNotExist) && errors.Is(cmd, fs.ErrNotExist) && vendor == nil
}

// marcador es el nombre de la función con que una biblioteca abre su conexión.
var marcador = regexp.MustCompile(`^Dial(TLS|Insecure|StartTLS|Context|Timeout|WithDialer)?$`)

type hallazgo struct {
	que   string
	linea int
}

// clientesSinPortero son las conexiones que f abre por su cuenta.
func clientesSinPortero(fset *token.FileSet, f *ast.File) []hallazgo {
	importados := map[string]string{} // nombre en el fichero → ruta
	for _, imp := range f.Imports {
		ruta, _ := strconv.Unquote(imp.Path.Value)
		nombre := path.Base(ruta)
		if strings.HasPrefix(nombre, "v") && len(nombre) > 1 && strings.Trim(nombre[1:], "0123456789") == "" {
			nombre = path.Base(path.Dir(ruta)) // `…/websocket/v2` se llama websocket
		}
		if imp.Name != nil {
			nombre = imp.Name.Name
		}
		importados[nombre] = ruta
	}
	// es dice si e es `<paquete>.<nombre>` de la ruta dada.
	es := func(e ast.Expr, ruta, nombre string) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != nombre {
			return false
		}
		x, ok := sel.X.(*ast.Ident)
		return ok && importados[x.Name] == ruta
	}
	var out []hallazgo
	anota := func(n ast.Node, que string) {
		out = append(out, hallazgo{que, fset.Position(n.Pos()).Line})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			switch {
			case es(n.Type, "net/http", "Client"):
				if !slices.ContainsFunc(n.Elts, func(e ast.Expr) bool {
					kv, ok := e.(*ast.KeyValueExpr)
					k, _ := kv.Key.(*ast.Ident)
					return ok && k != nil && k.Name == "Transport"
				}) {
					anota(n, "http.Client sin Transport")
				}
			case es(n.Type, "net/http", "Transport"):
				anota(n, "http.Transport propio")
			}
		case *ast.CallExpr:
			if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "new" && len(n.Args) == 1 && es(n.Args[0], "net/http", "Client") {
				anota(n, "http.Client sin Transport")
			}
		case *ast.ValueSpec:
			if n.Type != nil && len(n.Values) == 0 && es(n.Type, "net/http", "Client") {
				anota(n, "http.Client sin Transport")
			}
		case *ast.SelectorExpr:
			x, ok := n.X.(*ast.Ident)
			if !ok {
				return true
			}
			ruta, ok := importados[x.Name]
			if !ok {
				return true
			}
			if slices.Contains(prohibidoEnSelector[ruta], n.Sel.Name) {
				anota(n, path.Base(ruta)+"."+n.Sel.Name)
			} else if _, nuestro := prohibidoEnSelector[ruta]; !nuestro && marcador.MatchString(n.Sel.Name) {
				// El `Dial` de una biblioteca (imapclient.DialTLS, smtp.Dial,
				// websocket.Dial…) marca por su cuenta.
				anota(n, fmt.Sprintf("%s.%s", x.Name, n.Sel.Name))
			}
		}
		return true
	})
	return out
}

// puertasDePrueba son las funciones que abren una puerta SOLO para las pruebas, con los
// ÚNICOS ficheros que no son de pruebas que las nombran: su definición y `local/prueba`,
// que monta cada app en el driver de pruebas de Fyne. Se comparan por igualdad.
var puertasDePrueba = map[string][]string{
	"VigilarDePrueba(": {"apps/fyne/local/prueba/arranque.go", "apps/nucleo/local/red/vigilar.go"},
	"AppDePrueba(":     {"apps/fyne/local/arranque.go", "apps/fyne/local/prueba/arranque.go"},
}

// TestLasPuertasDePruebaSoloEnPruebas: `red.VigilarDePrueba` (ver cada conexión que se
// intenta) y `local.AppDePrueba` (montar un `main` en el driver de pruebas) no los llama
// ningún código de las apps. `local/prueba`, que sí, no lo enlaza ningún binario
// (TestLasAppsLocalesNoEnlazanCifradoNiMatrix, en `apps/fyne/local`).
//
// VISTO EN ROJO (2026-11-25): con una llamada a VigilarDePrueba en `local/cimientos.go`,
// dijo que la nombraban tres ficheros y quería dos.
func TestLasPuertasDePruebaSoloEnPruebas(t *testing.T) {
	raiz := filepath.Join("..", "..", "..", "..")
	quien := map[string][]string{}
	for _, dir := range []string{"apps/nucleo", "apps/fyne", "services", "platform"} {
		err := filepath.WalkDir(filepath.Join(raiz, dir), func(ruta string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // una carpeta que falta (una copia parcial) no nombra a nadie
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == "testdata" || strings.HasPrefix(n, ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return nil
			}
			datos, err := os.ReadFile(ruta) //nolint:gosec // el propio repositorio
			if err != nil {
				return nil //nolint:nilerr // ídem
			}
			for puerta := range puertasDePrueba {
				if strings.Contains(string(datos), puerta) {
					quien[puerta] = append(quien[puerta], filepath.ToSlash(strings.TrimPrefix(ruta, raiz+string(filepath.Separator))))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for puerta, quiero := range puertasDePrueba {
		got := quien[puerta]
		slices.Sort(got)
		if !slices.Equal(got, quiero) {
			t.Errorf("%s lo nombran fuera de las pruebas:\n  %s\nquiero exactamente:\n  %s", puerta, strings.Join(got, "\n  "), strings.Join(quiero, "\n  "))
		}
	}
}
