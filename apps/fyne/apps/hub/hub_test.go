// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

// traductor es el de las pruebas: inglés, que es el idioma BASE de la suite y el
// único que un catálogo no puede tener a medias.
func traductor(t *testing.T) *i18n.Translator {
	t.Helper()
	idioma := i18n.Locale("en")
	if os.Getenv("SYTUE_CAPTURAS_PLAY") != "" {
		idioma = "es" // las capturas de Play van en el idioma por defecto de la ficha
	}
	tr, err := i18n.New(idioma, i18n.Common(), catalogos.Portal())
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// TestLasClavesDeDescripcionDeProductoExisten es lo ÚNICO de i18n que se queda aquí:
// `ClaveDescripcion()` compone la clave en tiempo de ejecución («app.» + la clave de
// marca + «.description»), así que ningún guardián estático la ve.
//
// Lo demás —que ninguna clave literal de este paquete esté inventada, y que el catálogo
// sea la copia generada de la web en los 26 idiomas— lo mira `apps/fyne/catalogos` para
// TODO el módulo: aquí había una copia de esos dos guardianes y, con diecisiete apps
// por delante, habría diecisiete (`docs/CONVENTIONS.md` §13.2).
//
// Visto en ROJO (2026-09-22, en el piloto): cambiando el sufijo por «.descripcion», el
// test nombra las diecisiete claves que se irían a pantalla tal cual. Deshecho.
func TestLasClavesDeDescripcionDeProductoExisten(t *testing.T) {
	tr := traductor(t)
	if len(Productos) < 16 {
		t.Fatalf("solo hay %d productos: la lista se ha quedado corta", len(Productos))
	}
	for _, p := range Productos {
		if c := p.ClaveDescripcion(); tr.T(c, nil) == c {
			t.Errorf("la clave %q no está en el catálogo: se enseñaría tal cual", c)
		}
	}
}

// TestLosGlifosSonLosDeLaWeb: los siete trazados propios del Hub, verbatim de
// `Launcher.tsx`. Si alguien redibuja uno allí, aquí se pone rojo.
//
// Visto en ROJO a mano el 2026-09-22: quitando el último tramo de `GlifoArchivo`, el
// test dice el nombre del glifo y los dos trazados.
func TestLosGlifosSonLosDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/portal/src/components/Launcher.tsx")
	bloque := entre(t, fuente, "const GLYPHS = {", "} as const;")
	// Cada entrada es `nombre: 'trazado',` o `nombre:\n  'trazado',`.
	entrada := regexp.MustCompile(`(?s)(\w+):\s*\n?\s*'([^']+)'`)
	deLaWeb := map[string]string{}
	for _, m := range entrada.FindAllStringSubmatch(bloque, -1) {
		deLaWeb[m[1]] = strings.Join(strings.Fields(m[2]), " ")
	}
	if len(deLaWeb) != len(Glifos) {
		t.Fatalf("la web declara %d glifos y aquí hay %d", len(deLaWeb), len(Glifos))
	}
	for nombre, d := range deLaWeb {
		if Glifos[nombre] != d {
			t.Errorf("el glifo %q:\n  aquí: %s\n  web : %s", nombre, Glifos[nombre], d)
		}
	}
}

func entre(t *testing.T, s, desde, hasta string) string {
	t.Helper()
	i := strings.Index(s, desde)
	if i < 0 {
		t.Fatalf("no está %q en el fichero de la web", desde)
	}
	resto := s[i+len(desde):]
	j := strings.Index(resto, hasta)
	if j < 0 {
		t.Fatalf("no está %q en el fichero de la web", hasta)
	}
	return resto[:j]
}

// TestElCatalogoDeProductosEsElDeLaWeb: los dieciséis productos, en el mismo orden y
// con el mismo nombre que `CATALOGO_ORDENADO`. Es el orden del conmutador y el de la
// rejilla del lanzador, así que un producto añadido allí y no aquí deja una tesela sin
// pintar — que es exactamente lo que le pasó a Notepads en la web.
func TestElCatalogoDeProductosEsElDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/ui/src/lib/apps.ts")
	bloque := entre(t, fuente, "const CATALOGO_ORDENADO", "];")
	entrada := regexp.MustCompile(`key: '([a-z]+)',\s*\n?\s*name: '([^']+)'`)
	encontradas := entrada.FindAllStringSubmatch(bloque, -1)
	claves := make([]string, 0, len(encontradas))
	nombres := make([]string, 0, len(encontradas))
	for _, m := range encontradas {
		claves = append(claves, m[1])
		nombres = append(nombres, m[2])
	}
	if len(claves) != len(Productos) {
		t.Fatalf("la web tiene %d productos (%v) y aquí hay %d", len(claves), claves, len(Productos))
	}
	for i, p := range Productos {
		if p.Marca.Clave() != claves[i] || p.Nombre != nombres[i] {
			t.Errorf("producto %d: aquí %s/%q y en la web %s/%q",
				i, p.Marca.Clave(), p.Nombre, claves[i], nombres[i])
		}
	}
}

// TestLosProductosSoloDeOrganizacionesSonLosDeLaWeb: la tesela que se apaga en una
// cuenta personal es la misma aquí que en el conmutador y el lanzador de la web. Cuando
// entró Sytue Web cada sitio tenía su copia con solo `erp`, y la que no se tocara lo
// habría dejado abierto a quien no puede usarlo.
//
// Visto en ROJO el 2026-09-28: quitando `pages` de SoloOrganizaciones, el test enseña
// las dos listas. Deshecho.
func TestLosProductosSoloDeOrganizacionesSonLosDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/packages/config/src/tokens.ts")
	bloque := entre(t, fuente, "export const PRODUCTOS_SOLO_ORGANIZACION = [", "]")
	coincidencias := regexp.MustCompile(`'([a-z]+)'`).FindAllStringSubmatch(bloque, -1)
	deLaWeb := make([]string, len(coincidencias))
	for i, m := range coincidencias {
		deLaWeb[i] = m[1]
	}
	if len(deLaWeb) == 0 || !slices.Equal(deLaWeb, SoloOrganizaciones) {
		t.Fatalf("solo organizaciones: la web dice %v y aquí %v", deLaWeb, SoloOrganizaciones)
	}
}

// TestPresentablesDejaElHub: la puerta de vuelta a casa no se filtra nunca, y lo que
// vive dentro de otro despliegue se presenta con él.
func TestPresentablesDejaElHub(t *testing.T) {
	vivas := Presentables([]string{"docs"})
	claves := make([]string, 0, len(vivas))
	for _, p := range vivas {
		claves = append(claves, p.Marca.Clave())
	}
	for _, quiero := range []string{"one", "docs", "sheets", "slides", "pdf"} {
		if !slices.Contains(claves, quiero) {
			t.Errorf("con docs levantado falta %q: %v", quiero, claves)
		}
	}
	if slices.Contains(claves, "mail") {
		t.Errorf("mail no está levantado y sale igual: %v", claves)
	}
	if n := len(Presentables(nil)); n != len(Productos) {
		t.Errorf("sin lista se presentan %d y la suite tiene %d", n, len(Productos))
	}
}

// TestElEsquemaEsElDelFyneApp: el esquema con el que las demás aplicaciones llaman al
// Hub se declara en DOS sitios —el `FyneApp.toml`, que es lo que la cadena escribe en
// los cinco sistemas, y `EsquemaDe`, que es lo que el programa usa— y tienen que
// decir lo mismo. Si se separan, el redirect_uri del login deja de coincidir con lo
// registrado en Sytue ID y no se puede entrar.
func TestElEsquemaEsElDelFyneApp(t *testing.T) {
	datos, err := os.ReadFile("../../cmd/hub/FyneApp.toml")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ProductoDe("one")
	quiero := `esquema = "` + EsquemaDe(p) + `"`
	if !strings.Contains(string(datos), quiero) {
		t.Errorf("FyneApp.toml no declara %s", quiero)
	}
}
