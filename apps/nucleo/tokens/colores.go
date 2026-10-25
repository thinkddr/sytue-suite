// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package tokens son los COLORES de «Tinta y papel», los mismos que pinta la web y
// leídos de la MISMA hoja. No dependen de ningún toolkit: son `image/color.NRGBA`.
//
// Los colores no se copian a mano. colors.css es la copia que deja
// scripts/sync-branding.sh de branding/tokens/colors.css —la fuente de verdad— y aquí
// se interpreta al arrancar: bloque `:root` para el tema claro, y encima las
// reasignaciones de `[data-theme='dark']` y `[data-theme='translucent']`. Un espejo tipado a mano de
// 150 valores es justo lo que tokens.ts ya es en el frontend, y lo que theme.go fue en
// la primera pasada de Fyne: los dos hay que acordarse de tocarlos.
//
// Vive en el núcleo y no en el tema de un toolkit porque HAY DOS temas que lo
// necesitan idéntico, y el parser son trescientas líneas: la segunda copia habría
// divergido igual que divergió el endpoint de auditoría de Mail y Drive
// (docs/CONVENTIONS.md §13.2).
package tokens

import (
	_ "embed"
	"encoding/hex"
	"fmt"
	"image/color"
	"maps"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

//go:embed colors.css
var coloresCSS string

// aparienciaCSS es la hoja de la FORMA (copia de branding/tokens/appearance.css): el
// carril de color del menú lateral. La web la carga después de colors.css y aquí se lee
// igual, encima. De ella solo cuentan los bloques de tokens; sus reglas de clase
// (`.bg-sidebar { … }`) son cascada del navegador, y lo que hacen allí —cambiar el
// indicador o el foco dentro del carril— lo hace en las nativas quien pinta el carril,
// pidiendo `sidebar-indicator` o `sidebar-focus` por su nombre.
//
//go:embed appearance.css
var aparienciaCSS string

// Variant es la apariencia: clara, oscura o traslúcida.
type Variant uint8

// Light y Dark conservan sus valores 0 y 1; Translucent amplía la elección.
const (
	Light Variant = iota
	Dark
	Translucent
)

const variantes = int(Translucent) + 1

// juego son las paletas de UN color y UNA forma de la suite en sus tres apariencias:
// las crudas, las que devuelve `Color` (con los bordes aplanados) y el índice inverso de
// esos bordes.
type juego struct {
	crudas, paletas [variantes]map[string]color.NRGBA
	bordes          [variantes]map[color.NRGBA]color.NRGBA
}

// pedido es lo que decide qué bloques de la hoja valen, además del tema: el color de la
// suite (`data-color`, ADR 0071; "" es el de la casa), el producto de la app (`data-app`,
// que solo cuenta con el color «el de cada app») y la forma (`data-forma`; Marco es la
// hoja sin atributo y Carril, la que se elige).
type pedido struct {
	color, app string
	carril     bool
}

// Se resuelve una vez, al cargar el paquete. Si la hoja trae algo que no se sabe leer,
// el binario no arranca: mejor eso que un color transparente pintado en silencio, que es
// como falla un `var()` sin definir en CSS.
//
// De entrada se lee SOLO lo de la casa —su color y su forma—, que es lo que ve quien no
// ha elegido nada; cada otro color, forma o producto, la primera vez que se pide. Leerlos
// todos al arrancar costaba 50 ms en cada una de las catorce apps (medido el 2026-10-05,
// seis juegos), y que todos se dejan leer lo vigila TestTodosLosJuegosDeLaHojaSeLeen.
// `paletas` son las de la casa.
var (
	coloresDeLaHoja = coloresDeclarados(coloresCSS)
	juegos          = mustJuegos(coloresCSS, aparienciaCSS)
	paletas         = juegos[pedido{}].paletas
	// enVigor es el juego con el que se pinta. Atómico: lo cambia el vigilante de los
	// ajustes compartidos (otra app de la suite puede cambiar el color o la forma) y lo
	// leen los renderers.
	enVigor atomic.Pointer[juego]
)

func init() { enVigor.Store(juegos[pedido{}]) }

// ColorDeCadaApp es el tercer valor del eje de color: el carril y la acción toman el
// color de producto de la app que se está usando (`[data-color='app']` con `data-app`).
const ColorDeCadaApp = "app"

// FormaCarril es la forma que se elige; cualquier otro valor es Marco, la de la casa.
const FormaCarril = "carril"

// eleccion es lo elegido. Bajo su cerrojo va también el mapa de juegos, que crece con el
// primer «color de cada app» de un producto.
var eleccion struct {
	sync.Mutex
	pedido
}

// ElegirColor fija el color de la suite: uno de los que declara la hoja ("cobalto",
// "app") o cualquier otra cosa —"" y "petroleo" incluidos— para el de la casa. Devuelve
// si ha cambiado algo, que es lo que le dice a quien llama si hace falta repintar.
func ElegirColor(nombre string) bool {
	eleccion.Lock()
	defer eleccion.Unlock()
	eleccion.color = nombre
	return ponerEnVigor()
}

// ElegirApp dice de qué producto es esta app, con su clave de catálogo ("mail",
// "drive"…): es el `data-app` de la web, que solo cuenta con el color «el de cada app».
// Cada app nativa es un solo producto, así que lo dice una vez, al montar su barra.
func ElegirApp(clave string) bool {
	eleccion.Lock()
	defer eleccion.Unlock()
	eleccion.app = clave
	return ponerEnVigor()
}

// AppElegida devuelve el producto elegido, también cuando el color actual es fijo.
func AppElegida() string {
	eleccion.Lock()
	defer eleccion.Unlock()
	return eleccion.app
}

// ElegirForma fija la forma de la suite: "carril", o cualquier otra cosa —"" y "marco"
// incluidos— para Marco, la de la casa. Devuelve si ha cambiado algo, como ElegirColor.
func ElegirForma(nombre string) bool {
	eleccion.Lock()
	defer eleccion.Unlock()
	eleccion.carril = nombre == FormaCarril
	return ponerEnVigor()
}

// EsCarril dice si la forma en vigor es Carril. Si no, es Marco.
func EsCarril() bool {
	eleccion.Lock()
	defer eleccion.Unlock()
	return eleccion.carril
}

// ColorElegido es el color de la suite en vigor: uno de los que declara la hoja, o ""
// si es el de la casa.
func ColorElegido() string {
	eleccion.Lock()
	defer eleccion.Unlock()
	return enLaHoja(eleccion.pedido).color
}

// enLaHoja deja el pedido en lo que la hoja sabe pintar: un color que no declara es el
// de la casa, y el producto solo cuenta con «el de cada app».
func enLaHoja(p pedido) pedido {
	if !coloresDeLaHoja[p.color] {
		p.color = ""
	}
	if p.color != ColorDeCadaApp {
		p.app = ""
	}
	return p
}

// ponerEnVigor resuelve el juego de lo elegido. Va con el cerrojo cogido.
func ponerEnVigor() bool {
	p := enLaHoja(eleccion.pedido)
	j := juegos[p]
	if j == nil {
		// La primera vez que se pide. Un producto sin rampa propia en la hoja —el Hub—
		// sale con la de la casa: su bloque no existe y el reparto de `[data-color='app']`
		// da esos valores.
		j = mustJuego(p, coloresCSS, aparienciaCSS)
		juegos[p] = j
	}
	return enVigor.Swap(j) != j
}

// Colores lista los colores de la suite que declara la hoja además del de la casa,
// ordenados.
func Colores() []string {
	out := make([]string, 0, len(coloresDeLaHoja))
	for n := range coloresDeLaHoja {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Color devuelve el token `--sytue-<name>` en el tema pedido: "bg", "primary-500",
// "border-strong", "product-mail", "orgmark-7"… Con el color de la suite que esté en
// vigor (ver ElegirColor).
//
// Un nombre que no existe es un error de quien escribe el widget, no un estado de la
// aplicación, y entra en pánico como regexp.MustCompile.
func Color(name string, v Variant) color.NRGBA {
	c, ok := enVigor.Load().paletas[v][name]
	if !ok {
		panic("tokens: no existe el token de color --sytue-" + name)
	}
	return c
}

// Crudo es el token TAL COMO lo escribe la hoja, con su alfa: sin el aplanado sobre
// `surface` que `Color` les hace a los bordes.
//
// Existe para el borde que NO se pinta sobre la superficie sino sobre el cromo de
// tinta: el filete de la barra y el del lateral. En la web son `rgba(tinta, .12)`
// compuesto sobre su propio fondo —en claro, tinta sobre tinta, invisible—, y el
// aplanado sobre blanco los convertía en una raya clara que partía barra y lateral.
// Quien lo use compone con `SobreSRGB` sobre el fondo que tenga debajo.
func Crudo(name string, v Variant) color.NRGBA {
	c, ok := enVigor.Load().crudas[v][name]
	if !ok {
		panic("tokens: no existe el token de color --sytue-" + name)
	}
	return c
}

// Names lista los tokens de color, ordenados. Son los mismos en las tres apariencias.
func Names() []string {
	out := make([]string, 0, len(paletas[Light]))
	for n := range paletas[Light] {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

var (
	reSelectorTema = regexp.MustCompile(`\[data-(theme|color|app|forma)=["']([a-z]+)["']\]`)
	reComentario   = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reDecl         = regexp.MustCompile(`(?s)--sytue-([a-z0-9-]+)\s*:\s*(.*?);`)
	reVar          = regexp.MustCompile(`var\(--sytue-([a-z0-9-]+)\)`)
	reEspacios     = regexp.MustCompile(`\s+`)
	// El alfa es opcional: `rgb(22, 163, 74)` es lo que un navegador escribe en el
	// contenido de la gente (un `<span style="color: …">` de un documento pegado), y
	// `rgba(…)` lo que usa nuestra hoja de tokens. Las dos formas, un solo lector.
	reRGBA   = regexp.MustCompile(`^rgba?\((\d+), ?(\d+), ?(\d+)(?:, ?([0-9.]+))?\)$`)
	reMezcla = regexp.MustCompile(`^color-mix\(in srgb, ?(.+) ([0-9.]+)%, ?transparent ?\)$`)
	// Lo que la hoja declara y NO es un color: la tinta como terna suelta
	// (`--sytue-ink-rgb: 26, 53, 65`) y la intensidad de la selección (`10%`).
	reNoColor = regexp.MustCompile(`^\d`)
)

// reColorSuite reconoce los colores de la suite que declara la hoja, por sus bloques.
var reColorSuite = regexp.MustCompile(`\[data-color='([a-z]+)'\]`)

// coloresDeclarados son los colores de la suite que declara la hoja, sin el de la casa.
func coloresDeclarados(css string) map[string]bool {
	out := map[string]bool{}
	for _, m := range reColorSuite.FindAllStringSubmatch(normalizar(css), -1) {
		out[m[1]] = true // cada color sale en varios bloques, uno por modo
	}
	return out
}

// mustJuegos lee el juego de la casa; los demás se leen al pedirlos (ver ponerEnVigor).
func mustJuegos(hojas ...string) map[pedido]*juego {
	return map[pedido]*juego{{}: mustJuego(pedido{}, hojas...)}
}

// mustJuego lee las paletas de un pedido.
func mustJuego(p pedido, hojas ...string) *juego {
	crudas, err := parseCrudasDe(oscuroElegido, p, hojas...)
	if err != nil {
		panic(fmt.Sprintf("tokens: colors.css y appearance.css (%+v): %v", p, err))
	}
	j := &juego{crudas: crudas, paletas: aplanar(crudas)}
	j.bordes = indiceDeBordes(j)
	return j
}

// parsePaletas son las paletas que devuelve `Color`: las de la hoja con los bordes ya
// aplanados sobre su superficie.
func parsePaletas(hojas ...string) ([variantes]map[string]color.NRGBA, error) {
	crudas, err := parseCrudas(hojas...)
	if err != nil {
		return crudas, err
	}
	return aplanar(crudas), nil
}

// aplanar copia las paletas y aplana los bordes de la copia: las crudas no se tocan.
func aplanar(crudas [variantes]map[string]color.NRGBA) [variantes]map[string]color.NRGBA {
	var out [variantes]map[string]color.NRGBA
	for i, p := range crudas {
		out[i] = maps.Clone(p)
		aplanarBordes(out[i])
	}
	return out
}

// Las puertas de las apariencias explícitas y del oscuro del sistema.
// La preferencia explícita traslúcida también debe prevalecer sobre el sistema.
const (
	oscuroElegido     = "[data-theme='dark'] {"
	oscuroDelSistema  = ":root:not([data-theme]) {"
	traslucidoElegido = "[data-theme='translucent'] {"
)

// parseCrudas lee las tres paletas tal como las escriben las hojas, en su orden de carga.
func parseCrudas(hojas ...string) ([variantes]map[string]color.NRGBA, error) {
	return parseCrudasCon(oscuroElegido, hojas...)
}

// parseCrudasCon es parseCrudas con el oscuro sacado del bloque que abre `apertura`.
func parseCrudasCon(apertura string, hojas ...string) ([variantes]map[string]color.NRGBA, error) {
	return parseCrudasDe(apertura, pedido{}, hojas...)
}

// normalizar deja la hoja sin comentarios y con los selectores de atributo en una sola
// forma de comillas.
func normalizar(css string) string {
	css = reComentario.ReplaceAllString(css, "")
	return reSelectorTema.ReplaceAllString(css, "[data-$1='$2']")
}

// parseCrudasDe lee las tres paletas de un pedido (el cero es el color y la forma de la
// casa).
//
// Las hojas se apilan como en el navegador. `:root` y `[data-theme='dark']` pesan lo
// mismo, así que gana la regla que llega después: el `:root` de la SEGUNDA hoja pisa al
// bloque oscuro de la primera. Por eso cada hoja pone su `:root` y, encima, su bloque
// del tema, y no todos los `:root` primero. La primera hoja tiene que traer los tres
// bloques; a las siguientes les puede faltar el de un tema.
//
// EL COLOR es un eje aparte del modo (`[data-color='cobalto']`, ADR 0071). Su bloque
// pesa lo mismo que el de un tema y va después en la hoja, así que se apila tras él. Y
// el que combina color y tema (`[data-color='cobalto'][data-theme='dark']`) lleva DOS
// atributos: gana a todos los de uno, esté en la hoja que esté, así que esos van al
// final, también hoja a hoja. Es la razón de que appearance.css declare el carril entero
// en cada modo de cobalto.
//
// Con el color «el de cada app» hay un bloque más de dos atributos, el del producto
// (`[data-color='app'][data-app='mail']`), que solo trae su rampa del primario: `app` es
// su clave, y vacía —o sin bloque en la hoja— deja la rampa de la casa.
//
// LA FORMA es el tercer eje (`data-forma`). Sus bloques van al final de appearance.css y
// solo declaran lo suyo —el ítem activo del menú, el marco y la hoja—: Marco es
// `:root:not([data-forma='carril'])`, con su añadido del traslúcido, y Carril,
// `[data-forma='carril']`.
func parseCrudasDe(apertura string, p pedido, hojas ...string) ([variantes]map[string]color.NRGBA, error) {
	var out [variantes]map[string]color.NRGBA
	limpias := make([]string, len(hojas))
	for n, css := range hojas {
		limpias[n] = normalizar(css)
	}
	delColor, delApp := "", ""
	if p.color != "" {
		delColor = "[data-color='" + p.color + "']"
		if p.app != "" {
			delApp = delColor + "[data-app='" + p.app + "'] {"
		}
	}
	for i, bloque := range []string{"", apertura, traslucidoElegido} {
		decl, err := apilar(limpias, bloque, delColor, delApp)
		if err == nil {
			apilarForma(decl, limpias, p.carril, bloque == traslucidoElegido)
		}
		if err != nil {
			return out, err
		}
		if out[i], err = resolver(decl); err != nil {
			return out, err
		}
	}
	return out, nil
}

// apilar junta las declaraciones que valen para un tema (`bloque`, vacío en el claro),
// un color (`delColor`, vacío en el de la casa) y un producto (`delApp`, la apertura de
// su bloque, o vacía), en el orden de la cascada: primero los bloques de un atributo,
// hoja a hoja, y después los de dos, en el orden en que van en la hoja —el del
// producto antes que el del color con el tema—.
func apilar(hojas []string, bloque, delColor, delApp string) (map[string]string, error) {
	decl := map[string]string{}
	for n, css := range hojas {
		// Cada apariencia reasigna :root por separado. Traslúcido no hereda oscuro:
		// var() se resuelve después de aplicar sus propias reasignaciones.
		aperturas := []string{":root {"}
		if bloque != "" {
			aperturas = append(aperturas, bloque)
		}
		if delColor != "" {
			aperturas = append(aperturas, delColor+" {")
		}
		for _, b := range aperturas {
			reasignado, err := declaraciones(css, b)
			if err != nil && n == 0 {
				return nil, err
			}
			maps.Copy(decl, reasignado)
		}
	}
	var dosAtributos []string
	if delApp != "" {
		dosAtributos = append(dosAtributos, delApp)
	}
	if delColor != "" && bloque != "" {
		dosAtributos = append(dosAtributos, delColor+bloque)
	}
	for _, css := range hojas {
		for _, b := range dosAtributos {
			// A una hoja le puede faltar el bloque de un color en un tema, o el de un
			// producto: no es un error.
			reasignado, _ := declaraciones(css, b)
			maps.Copy(decl, reasignado)
		}
	}
	return decl, nil
}

// Las puertas de la forma en appearance.css.
const (
	formaMarco           = ":root:not([data-forma='carril']) {"
	formaMarcoTraslucido = "[data-theme='translucent']:not([data-forma='carril']) {"
	formaCarril          = "[data-forma='carril'] {"
)

// apilarForma añade a lo ya apilado los bloques de la forma, que van los últimos.
func apilarForma(decl map[string]string, hojas []string, carril, traslucido bool) {
	bloques := make([]string, 0, 2)
	switch {
	case carril:
		bloques = append(bloques, formaCarril)
	case traslucido:
		bloques = append(bloques, formaMarco, formaMarcoTraslucido)
	default:
		bloques = append(bloques, formaMarco)
	}
	for _, css := range hojas {
		for _, b := range bloques {
			// Solo appearance.css los trae: en las demás hojas no es un error.
			reasignado, _ := declaraciones(css, b)
			maps.Copy(decl, reasignado)
		}
	}
}

// resolver expande los var() de las declaraciones y lee cada valor como color. Lo que
// la hoja declara y no es un color se queda fuera.
func resolver(decl map[string]string) (map[string]color.NRGBA, error) {
	out := make(map[string]color.NRGBA, len(decl))
	for nombre, crudo := range decl {
		valor, err := expandir(decl, crudo, 0)
		if err != nil {
			return nil, fmt.Errorf("--sytue-%s: %w", nombre, err)
		}
		if reNoColor.MatchString(valor) {
			continue
		}
		c, err := parseColor(valor)
		if err != nil {
			return nil, fmt.Errorf("--sytue-%s: %w", nombre, err)
		}
		out[nombre] = c
	}
	return out, nil
}

// aplanarBordes deja los bordes OPACOS, ya mezclados con su superficie.
//
// POR QUÉ. Gio compone en espacio LINEAL y el navegador lo hace en sRGB, así que
// el mismo `rgba(ink, .12)` no da el mismo píxel en los dos sitios. Medido sobre
// blanco: la web da (227,230,232) y Gio (241,241,242) —la cuenta sale exacta:
// 0,12 en lineal es 0,8807, que codificado a sRGB son 241—. El efecto es que
// TODOS los bordes y divisores de la suite salían a media fuerza, en todas las
// pantallas, y como se veían «casi bien» nadie lo miró hasta comparar píxeles.
//
// Se arregla aquí y no en cada pantalla: mezclar a mano en veinte sitios es
// garantizar veinte tonos de borde distintos.
//
// SOLO LOS BORDES. El velo (`overlay`) y las sombras siguen con su alfa: van
// sobre contenido arbitrario, así que no hay fondo con el que precomponerlos, y
// ahí la diferencia de espacio es mucho menos visible que en una línea de 1 px.
func aplanarBordes(tokens map[string]color.NRGBA) {
	fondo, ok := tokens["surface"]
	if !ok {
		return // sin superficie no hay con qué mezclar; se deja como estaba
	}
	// Las superficies traslúcidas conservan su alfa al pintar, pero el filete
	// opaco necesita la base compuesta: no puede asumir papel blanco.
	if fondo.A != 0xff {
		if base, existe := tokens["bg"]; existe {
			fondo = SobreSRGB(fondo, base)
		}
	}
	for nombre, c := range tokens {
		if !strings.HasPrefix(nombre, "border") || c.A == 0xff {
			continue
		}
		tokens[nombre] = SobreSRGB(c, fondo)
	}
}

// BordeCrudo reconoce un borde YA aplanado —el que da `Color` para un token `border*`— y
// devuelve el de la hoja, con su alfa. Es para quien recibe el color y no el nombre (una
// caja a la que le pasan su borde) y necesita componerlo sobre otro fondo que `surface`.
func BordeCrudo(c color.NRGBA, v Variant) (color.NRGBA, bool) {
	crudo, ok := enVigor.Load().bordes[v][c]
	return crudo, ok
}

// indiceDeBordes es el índice inverso de los bordes de un juego: aplanado → crudo, por
// tema.
func indiceDeBordes(j *juego) [variantes]map[color.NRGBA]color.NRGBA {
	var m [variantes]map[color.NRGBA]color.NRGBA
	for v := range m {
		m[v] = map[color.NRGBA]color.NRGBA{}
		for n, c := range j.crudas[v] {
			if strings.HasPrefix(n, "border") && c.A != 0xff && c.A != 0 {
				m[v][j.paletas[v][n]] = c
			}
		}
	}
	return m
}

// SobreSRGB compone `fg` sobre `fondo` como lo hace un navegador: interpolando
// los canales tal cual, sin pasar por luz lineal. Devuelve un color opaco.
//
// Es pública porque el aplanado no es solo cosa de los bordes: un toolkit que pinta
// dos capas translúcidas propias —una caja con borde atenuada al 50 %, por ejemplo—
// necesita aplanar la de abajo o el resultado se multiplica en vez de sumarse. Ahí lo
// usa `ui.atenuar`, y así los dos sitios componen con la MISMA cuenta.
func SobreSRGB(fg, fondo color.NRGBA) color.NRGBA {
	a := float64(fg.A) / 255
	mezcla := func(f, b uint8) uint8 {
		return uint8(math.Round(float64(f)*a + float64(b)*(1-a)))
	}
	return color.NRGBA{
		R: mezcla(fg.R, fondo.R),
		G: mezcla(fg.G, fondo.G),
		B: mezcla(fg.B, fondo.B),
		A: 0xff,
	}
}

// SobreConAlfa compone fg sobre fondo en sRGB y conserva el alfa combinado.
// Un borde sobre una superficie rgba sigue dejando ver el canvas; hacerlo opaco
// cambiaría su color cuando el fondo de detrás es un degradado.
func SobreConAlfa(fg, fondo color.NRGBA) color.NRGBA {
	if fondo.A == 0xff {
		return SobreSRGB(fg, fondo)
	}
	a := float64(fg.A) / 255
	b := float64(fondo.A) / 255 * (1 - a)
	total := a + b
	if total == 0 {
		return color.NRGBA{}
	}
	canal := func(f, p uint8) uint8 {
		return uint8(math.Round((float64(f)*a + float64(p)*b) / total))
	}
	return color.NRGBA{
		R: canal(fg.R, fondo.R),
		G: canal(fg.G, fondo.G),
		B: canal(fg.B, fondo.B),
		A: uint8(math.Round(total * 255)),
	}
}

// Aplanar compone `tinta` con la opacidad `a` sobre `fondo` y devuelve el color opaco.
// Es SobreSRGB con el alfa aparte y SIN cuantizar a un byte: `opacity-40` sobre un
// grupo o `bg-bg-subtle/40` son un 0,4 exacto en el navegador, y 0,4 no es ningún
// n/255. Vivía en Sytue Calcus, y el Hub y Calendar tenían cada uno su `mezclar` con
// el orden de los argumentos cambiado y truncando en vez de redondear.
func Aplanar(tinta color.NRGBA, a float64, fondo color.NRGBA) color.NRGBA {
	m := func(t, f uint8) uint8 { return uint8(float64(f) + (float64(t)-float64(f))*a + 0.5) }
	return color.NRGBA{R: m(tinta.R, fondo.R), G: m(tinta.G, fondo.G), B: m(tinta.B, fondo.B), A: 0xff}
}

// ConAlfa es el modificador de opacidad de Tailwind (`border-border/60`): el mismo
// token con su alfa multiplicado por `a`. El token puede venir ya translúcido, y por
// eso multiplica y no sustituye. Estaba escrito tres veces en las apps nativas.
func ConAlfa(c color.NRGBA, a float64) color.NRGBA {
	c.A = uint8(float64(c.A)*a + 0.5)
	return c
}

// declaraciones lee las custom properties del PRIMER bloque que abre `apertura`.
// Ninguno de los dos bloques que interesan anida llaves.
func declaraciones(css, apertura string) (map[string]string, error) {
	i := strings.Index(css, apertura)
	if i < 0 {
		return nil, fmt.Errorf("falta el bloque %q", apertura)
	}
	cuerpo := css[i+len(apertura):]
	j := strings.IndexByte(cuerpo, '}')
	if j < 0 {
		return nil, fmt.Errorf("el bloque %q no cierra", apertura)
	}
	decl := map[string]string{}
	for _, m := range reDecl.FindAllStringSubmatch(cuerpo[:j], -1) {
		// La última declaración gana, como en CSS: `--sytue-primary` aparece dos
		// veces en :root (azul primero, tinta después) y la que vale es la tinta.
		decl[m[1]] = strings.TrimSpace(reEspacios.ReplaceAllString(m[2], " "))
	}
	return decl, nil
}

// expandir sustituye cada var() por su valor, igual que hace el navegador: texto por
// texto, antes de interpretar nada.
func expandir(decl map[string]string, valor string, prof int) (string, error) {
	if prof > 16 {
		return "", fmt.Errorf("var() en ciclo")
	}
	var fallo error
	out := reVar.ReplaceAllStringFunc(valor, func(m string) string {
		nombre := reVar.FindStringSubmatch(m)[1]
		crudo, ok := decl[nombre]
		if !ok {
			fallo = fmt.Errorf("var(--sytue-%s) sin definir", nombre)
			return m
		}
		s, err := expandir(decl, crudo, prof+1)
		if err != nil {
			fallo = err
		}
		return s
	})
	return out, fallo
}

// ParseColor interpreta un color CSS suelto («#16734d», «#abc», «rgba(…)», «black»). Es
// para los colores que llegan como DATO —el de un calendario, el de una forma de una
// diapositiva—, no para los tokens, que van por Color.
//
// Admite MÁS que la hoja de tokens a propósito: `#abc` y los nombres `black`/`white` los
// escribe un navegador en el contenido de la gente, y ahí hay que entenderlos; en
// `colors.css`, en cambio, siguen siendo un error (`TestLaHojaIlegibleNoArranca`), porque
// esa hoja la escribimos nosotros y la queremos en una sola forma.
func ParseColor(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(s)
	switch {
	case strings.EqualFold(s, "black"):
		return color.NRGBA{A: 0xff}, true
	case strings.EqualFold(s, "white"):
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, true
	case len(s) == 4 && s[0] == '#': // `#abc` es `#aabbcc`
		s = string([]byte{'#', s[1], s[1], s[2], s[2], s[3], s[3]})
	}
	c, err := parseColor(s)
	return c, err == nil
}

func parseColor(s string) (color.NRGBA, error) {
	s = strings.NewReplacer("( ", "(", " )", ")").Replace(s)
	switch {
	case s == "transparent":
		return color.NRGBA{}, nil
	case len(s) == 7 && s[0] == '#':
		b, err := hex.DecodeString(s[1:])
		if err != nil {
			return color.NRGBA{}, fmt.Errorf("hex ilegible %q", s)
		}
		return color.NRGBA{R: b[0], G: b[1], B: b[2], A: 0xff}, nil
	}
	if m := reRGBA.FindStringSubmatch(s); m != nil {
		var canal [3]uint8
		for i := range canal {
			n, err := strconv.ParseUint(m[i+1], 10, 8)
			if err != nil {
				return color.NRGBA{}, fmt.Errorf("canal fuera de rango en %q", s)
			}
			canal[i] = uint8(n)
		}
		if m[4] == "" {
			return color.NRGBA{R: canal[0], G: canal[1], B: canal[2], A: 0xff}, nil
		}
		a, err := strconv.ParseFloat(m[4], 64)
		if err != nil || a < 0 || a > 1 {
			return color.NRGBA{}, fmt.Errorf("alfa ilegible en %q", s)
		}
		return color.NRGBA{R: canal[0], G: canal[1], B: canal[2], A: Alfa(a)}, nil
	}
	// Mezclar con `transparent` en sRGB deja el color como está y le escala el alfa:
	// es lo único para lo que la hoja usa color-mix (el tinte de selección).
	if m := reMezcla.FindStringSubmatch(s); m != nil {
		c, err := parseColor(m[1])
		if err != nil {
			return color.NRGBA{}, err
		}
		p, err := strconv.ParseFloat(m[2], 64)
		if err != nil || p < 0 || p > 100 {
			return color.NRGBA{}, fmt.Errorf("porcentaje ilegible en %q", s)
		}
		c.A = Alfa(float64(c.A) / 255 * p / 100)
		return c, nil
	}
	return color.NRGBA{}, fmt.Errorf("valor que no sé leer: %q", s)
}

// alfa redondea como el navegador: 0,12 es 31 (0x1F), no 30.
// Alfa convierte una opacidad de CSS (0..1) en el byte de alfa de NRGBA.
func Alfa(a float64) uint8 { return uint8(math.Round(a * 255)) }
