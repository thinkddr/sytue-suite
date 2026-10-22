// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLasIlustracionesSonLasDeBranding compara la copia vendorizada con
// `branding/illustrations`, igual que se hace con las marcas: la copia existe solo
// porque `go:embed` no sale de su módulo, así que cualquier diferencia es una copia que
// se ha quedado atrás.
func TestLasIlustracionesSonLasDeBranding(t *testing.T) {
	webref.Read(t, "branding/index.json")
	raiz := raizDelRepo(t)

	origen, err := filepath.Glob(filepath.Join(raiz, "branding", "illustrations", "*.svg"))
	if err != nil || len(origen) == 0 {
		t.Fatalf("no hay ilustraciones en branding/illustrations: %v", err)
	}
	esperadas := map[string]bool{}
	for _, ruta := range origen {
		nombre := filepath.Base(ruta)
		esperadas[nombre] = true
		quiero, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}
		tengo, err := ilustracionesFS.ReadFile("ilustraciones/" + nombre)
		if err != nil {
			t.Errorf("falta la copia de %s: pásale `scripts/sync-branding.sh`", nombre)
			continue
		}
		if !bytes.Equal(tengo, quiero) {
			t.Errorf("la copia de %s no es la de branding: pásale `scripts/sync-branding.sh`", nombre)
		}
	}
	hay, _ := ilustracionesFS.ReadDir("ilustraciones")
	for _, e := range hay {
		if !esperadas[e.Name()] {
			t.Errorf("%s ya no está en branding/illustrations y sigue aquí: es una huérfana", e.Name())
		}
	}
}

// TestLasIlustracionesSePintan es el defecto que las tenía a las trece inutilizables:
// `oksvg` no entiende `var(--sytue-*)` y ABORTA EL FICHERO ENTERO, así que un estado
// vacío salía sin dibujo. Aquí se pintan las doce y se cuenta tinta.
func TestLasIlustracionesSePintan(t *testing.T) {
	for i := Ilustracion(0); i < numIlustraciones; i++ {
		m := i.MedidaNatural()
		if m.Width <= 0 || m.Height <= 0 {
			t.Errorf("%s: el SVG no declara width/height y su medida natural sale %v", i, m)
			continue
		}
		for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
			svg := conTokens(i.leer(), v)
			if bytes.Contains(svg, []byte("var(--sytue-")) {
				t.Errorf("%s: quedan `var(--sytue-*)` sin resolver, y oksvg aborta el fichero", i)
				continue
			}
			img := rasteriza(svg, int(m.Width), int(m.Height))
			if n := conTinta(img); n < 100 {
				t.Errorf("%s en tema %v pinta %d píxeles: está en blanco", i, v, n)
			}
		}
	}
}

// TestLosTokensDeUnaIlustracionSonLosDeLaHoja comprueba que la sustitución usa el
// COLOR de la paleta y no un valor de reserva ni un gris cualquiera.
//
// `empty-mail` traza su rejilla con `--sytue-border`, así que en la fila de esa línea
// tiene que aparecer ese color y no otro.
func TestLosTokensDeUnaIlustracionSonLosDeLaHoja(t *testing.T) {
	svg := string(conTokens(VacioMail.leer(), tema.Claro))
	c := tema.Color("border", tema.Claro)
	quiero := hex(c)
	if !strings.Contains(svg, quiero) {
		t.Errorf("empty-mail no lleva %s, que es `--sytue-border` en claro", quiero)
	}
	// Y en oscuro tiene que ser OTRO, o la sustitución estaría ignorando la variante.
	if oscuro := hex(tema.Color("border", tema.Oscuro)); oscuro != quiero &&
		strings.Contains(string(conTokens(VacioMail.leer(), tema.Oscuro)), quiero) {
		t.Error("la ilustración pinta el color del tema claro también en oscuro")
	}
}

// TestElValorDeReservaSeUsaCuandoElTokenNoEstaEnLaHoja: el navegador hace eso con
// `var(--x, #hex)`, y aquí también, porque si no un trazo desaparecería en silencio.
func TestElValorDeReservaSeUsaCuandoElTokenNoEstaEnLaHoja(t *testing.T) {
	got := string(conTokens([]byte(`<path stroke="var(--sytue-no-existe-este, #abcdef)"/>`), tema.Claro))
	if !strings.Contains(got, "#abcdef") {
		t.Errorf("con un token desconocido sale %q y el valor de reserva era #abcdef", got)
	}
	// Y sin reserva se deja tal cual, para que `rasteriza` se queje en vez de callar.
	got = string(conTokens([]byte(`<path stroke="var(--sytue-no-existe-este)"/>`), tema.Claro))
	if !strings.Contains(got, "var(--sytue-no-existe-este)") {
		t.Errorf("sin valor de reserva se ha tragado el token desconocido: %q", got)
	}
}

func hex(c interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, b, _ := c.RGBA()
	const d = "0123456789abcdef"
	n := func(v uint32) string { v >>= 8; return string([]byte{d[v>>4], d[v&15]}) }
	return "#" + n(r) + n(g) + n(b)
}

func conTinta(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x2000 {
				n++
			}
		}
	}
	return n
}
