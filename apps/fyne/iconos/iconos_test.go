// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// rutasDeLaWeb saca los `PATHS` de un `.tsx` tal cual están escritos: clave y `d`.
// Vale para `Icon.tsx` y para el `glyphs.tsx` de una app, que declaran el mapa igual.
//
// Se PARSEA en vez de comprobar «¿está esta cadena dentro del fichero?» porque la
// pertenencia solo caza en un sentido. Con el fichero parseado se comparan los dos
// conjuntos, que es lo que hace falta: una ruta que cambie en la web y un icono que
// sobre aquí ponen esto en rojo igual.
func rutasDeLaWeb(t *testing.T, fichero string, minimo int) map[string]string {
	t.Helper()
	fuente := webref.Read(t, fichero)
	i := strings.Index(fuente, "const PATHS = {")
	j := strings.Index(fuente, "} as const;")
	if i < 0 || j < i {
		t.Fatalf("%s ya no declara `const PATHS = { … } as const;`: el espejo hay que rehacerlo", fichero)
	}
	re := regexp.MustCompile(`(?m)^ {2}([A-Za-z_][A-Za-z0-9_]*):\s*\n?\s*'((?:[^'\\]|\\.)*)'`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(fuente[i:j], -1) {
		out[m[1]] = m[2]
	}
	if len(out) < minimo {
		t.Fatalf("solo se han leído %d rutas de %s: el patrón ya no vale", len(out), fichero)
	}
	return out
}

// glifosJSX saca los glifos de un `glyphs.tsx` que declara `const GLYPHS` con JSX en
// vez de `const PATHS` con cadenas, que es como lo escribe Sytue Drive.
//
// Solo lee los que son UN `<path d="…" />`, que es como se declara la mayoría. Los que
// llevan varios elementos (`copy` con su `<rect>`, `more` con tres `<circle>`) se
// quedan fuera a propósito: la entrada de este catálogo tiene un solo `d`, y unirlos a
// mano aquí sería exactamente el «redibujado a ojo» que el guardián existe para
// impedir. El día que haga falta uno de esos, se amplía `entrada`, no el test.
func glifosJSX(t *testing.T, fichero string, minimo int) map[string]string {
	t.Helper()
	fuente := webref.Read(t, fichero)
	i := strings.Index(fuente, "const GLYPHS = {")
	j := strings.Index(fuente, "} as const;")
	if i < 0 || j < i {
		t.Fatalf("%s ya no declara `const GLYPHS = { … } as const;`: el espejo hay que rehacerlo", fichero)
	}
	// El `(\n` del medio es el envoltorio que mete Prettier cuando la línea es larga.
	re := regexp.MustCompile(`(?m)^ {2}([A-Za-z][A-Za-z0-9]*): (?:\(\n\s*)?<path d="([^"]*)" />`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(fuente[i:j], -1) {
		out[m[1]] = m[2]
	}
	if len(out) < minimo {
		t.Fatalf("solo se han leído %d glifos de %s: el patrón ya no vale", len(out), fichero)
	}
	return out
}

// TestElCatalogoEsElDeLaWeb es EL guardián de este paquete: los iconos, ruta a ruta,
// contra el `.tsx` del que sale CADA UNO —`Icon.tsx` los compartidos y el
// `glyphs.tsx` de la app los propios—.
//
// El catálogo compartido se compara por IGUALDAD, no por pertenencia: un icono
// redibujado a ojo, una ruta que la web afine y un icono que se retire allí y
// sobreviva aquí caen los tres, que es lo que no pasó con los doce `middleware.ts`.
//
// De los glifos de una app se comprueban los dos sentidos que tienen sentido —que
// exista allí y que la ruta sea la misma— pero NO que estén todos: de `glyphs.tsx` se
// porta lo que las pantallas usan, y exigir el fichero entero obligaría a traerse la
// barra del editor enriquecido para poder pintar una lista.
//
// Los catorce de Sytue Mail YA NO ESTÁN AQUÍ: se fueron a `apps/mail/glifos.go` el
// 2026-09-22, con su guardián (`TestLosGlifosSonLosDeLaWeb`). Los de Drive siguen
// porque `ui.TeselaTipo` los pide desde el catálogo compartido.
func TestElCatalogoEsElDeLaWeb(t *testing.T) {
	// Los mínimos son el «el patrón ya no vale» de cada fichero, no un objetivo.
	web := map[string]map[string]string{
		FuenteComun: rutasDeLaWeb(t, FuenteComun, 50),
		FuenteDrive: glifosJSX(t, FuenteDrive, 5),
	}
	// La clave sola no identifica un icono: `trash`, `clock` y `tag` están en los dos
	// ficheros con rutas distintas. La pareja (fichero, clave) sí.
	type ref struct{ fichero, clave string }
	nuestros := map[ref]string{}
	for i, e := range catalogo {
		icono := Icono(i)
		if icono == Ninguno {
			continue
		}
		if e.clave == "" {
			t.Errorf("el icono %d no tiene clave: hay un hueco en `catalogo`", i)
			continue
		}
		k := ref{icono.Origen(), e.clave}
		if otro, ya := nuestros[k]; ya {
			t.Errorf("la clave %q de %s está dos veces (%q y %q)", e.clave, k.fichero, otro, e.d)
		}
		nuestros[k] = e.d
	}
	for k, d := range nuestros {
		switch suyo, hay := web[k.fichero][k.clave]; {
		case !hay:
			t.Errorf("el icono %q no existe en %s: o se retiró allí, o aquí se inventó", k.clave, k.fichero)
		case suyo != d:
			t.Errorf("el icono %q tiene otra ruta en %s:\n  web:    %s\n  aquí:   %s", k.clave, k.fichero, suyo, d)
		}
	}
	// El catálogo COMPARTIDO sí tiene que estar entero: es el cromo de la suite.
	for clave := range web[FuenteComun] {
		if _, hay := nuestros[ref{FuenteComun, clave}]; !hay {
			t.Errorf("la web tiene el icono %q y este catálogo no: añádelo como constante", clave)
		}
	}
}

// TestElCatalogoNoTieneHuecos comprueba que cada constante tiene su entrada. Como
// `catalogo` va indexado por el propio Icono, una constante nueva sin su línea deja un
// hueco que solo se vería al pintar esa pantalla.
func TestElCatalogoNoTieneHuecos(t *testing.T) {
	if len(catalogo) != int(numIconos) {
		t.Fatalf("hay %d constantes de icono y el catálogo tiene %d entradas: a la última le falta su línea",
			int(numIconos), len(catalogo))
	}
	for i := Ninguno + 1; i < numIconos; i++ {
		if catalogo[i].d == "" {
			t.Errorf("el icono %d (%s) no tiene ruta", int(i), i)
		}
	}
	if catalogo[Ninguno].d != "" {
		t.Error("Ninguno tiene ruta: el cero del tipo tiene que ser «no hay icono»")
	}
}

// TestLasMarcasSonLasDeBranding compara la copia vendorizada con `branding/marks`, que
// es la fuente de verdad y la que usa también la web. La copia existe solo porque
// `go:embed` no sale de su módulo, así que cualquier diferencia es una copia que se ha
// quedado atrás — exactamente lo que le pasó al conmutador de aplicaciones cuando las
// marcas se rehicieron y él siguió con la familia anterior.
func TestLasMarcasSonLasDeBranding(t *testing.T) {
	// Se localiza la raíz igual que webref, pidiéndole un fichero conocido.
	webref.Read(t, "branding/index.json")
	raiz := raizDelRepo(t)

	origen, err := filepath.Glob(filepath.Join(raiz, "branding", "marks", "*.svg"))
	if err != nil || len(origen) == 0 {
		t.Fatalf("no hay marcas en branding/marks: %v", err)
	}
	esperadas := map[string]bool{}
	for _, ruta := range origen {
		nombre := filepath.Base(ruta)
		if strings.HasSuffix(nombre, "-maskable.svg") {
			continue // son para el lanzador del sistema, no para la interfaz
		}
		esperadas[nombre] = true
		quiero, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		tengo, err := marcasFS.ReadFile("marcas/" + nombre)
		if err != nil {
			t.Errorf("falta la copia de %s: pásale `scripts/sync-branding.sh`", nombre)
			continue
		}
		if !bytes.Equal(tengo, quiero) {
			t.Errorf("marcas/%s no es igual que branding/marks/%s: la copia se ha quedado atrás", nombre, nombre)
		}
	}
	entradas, err := marcasFS.ReadDir("marcas")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entradas {
		if !esperadas[e.Name()] {
			t.Errorf("marcas/%s ya no está en branding/marks: un huérfano no molesta hasta que alguien lo importa", e.Name())
		}
	}
	// Y que las veinte constantes tengan su fichero, y al revés.
	for p := One; p <= Bases; p++ {
		if _, err := marcasFS.ReadFile("marcas/" + p.Clave() + ".svg"); err != nil {
			t.Errorf("la marca de %s no está entre las copias", p.Clave())
		}
		delete(esperadas, p.Clave()+".svg")
	}
	for sobra := range esperadas {
		t.Errorf("hay una marca %s sin constante `Producto`: añádela o el catálogo miente", sobra)
	}
}

func raizDelRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "branding")); err == nil {
			return dir
		}
		padre := filepath.Dir(dir)
		if padre == dir {
			t.Skip("fuera del monorepo")
		}
		dir = padre
	}
}

// TestLasClavesDeProductoSonLasDeLaWeb ata los veinte nombres al catálogo
// canónico del que tokens.ts deriva `ProductKey`.
func TestLasClavesDeProductoSonLasDeLaWeb(t *testing.T) {
	var catalogo struct {
		Producto map[string]json.RawMessage `json:"product"`
	}
	if err := json.Unmarshal([]byte(webref.Read(t, "branding/tokens/colors.json")), &catalogo); err != nil {
		t.Fatal(err)
	}
	for p := One; p <= Bases; p++ {
		if _, existe := catalogo.Producto[p.Clave()]; !existe {
			t.Errorf("%q no es un ProductKey del catálogo canónico", p.Clave())
		}
	}
	// El guardián, invertido: una clave inventada TIENE que no encontrarse.
	if _, existe := catalogo.Producto["notepads"]; existe {
		t.Fatal("una clave inventada se ha encontrado en el catálogo: la comparación no vale")
	}
	// Los alias legados conservan color en la web, pero no son productos nativos.
	for _, legado := range []string{"chat", "meet"} {
		for p := One; p <= Bases; p++ {
			if p.Clave() == legado {
				t.Errorf("%s tiene constante y la web no le da marca: es un host legado que solo redirige", legado)
			}
		}
	}
}

// TestChatYMeetSonRooms: el Hub etiqueta con `chat` los resultados de Rooms; la barra
// de búsqueda los pone bajo Rooms, con su marca, como la web.
func TestChatYMeetSonRooms(t *testing.T) {
	for _, k := range []string{"chat", "meet", "rooms"} {
		if p, ok := ProductoDe(k); !ok || p != Rooms {
			t.Errorf("ProductoDe(%q) = %v, %v; quiero Rooms", k, p, ok)
		}
	}
	if _, ok := ProductoDe("inventado"); ok {
		t.Error("una clave que no existe ha salido con marca")
	}
}
