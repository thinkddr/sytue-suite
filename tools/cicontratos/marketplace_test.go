// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// El paquete de Teams del marketplace: la guardia (verificar-marketplace.sh)
// rechaza lo que no cumple el contrato de iconos y preparar-teams.sh adapta el
// outline sin tocar su alfa, color.png ni el manifiesto.

const (
	marketplaceGuardia  = ".github/actions/toolchain/verificar-marketplace.sh"
	marketplacePreparar = ".github/actions/toolchain/preparar-teams.sh"
)

// marketplaceBanco es el directorio de trabajo de la prueba, con su teams.zip.
type marketplaceBanco struct {
	t       *testing.T
	trabajo string
	zip     string
}

// orden ejecuta un programa en el directorio de trabajo; tiene que salir bien.
func (b marketplaceBanco) orden(programa string, args ...string) guiontest.Resultado {
	b.t.Helper()
	r := guiontest.Ejecutar(b.t, guiontest.Opciones{Dir: b.trabajo}, programa, args...)
	if r.Codigo != 0 {
		b.t.Fatalf("%s %v salió con %d: %s", programa, args, r.Codigo, r.Error)
	}
	return r
}

// empaquetar rehace teams.zip con esos ficheros del directorio de trabajo.
func (b marketplaceBanco) empaquetar(ficheros ...string) {
	b.t.Helper()
	if err := os.RemoveAll(b.zip); err != nil {
		b.t.Fatal(err)
	}
	b.orden("zip", append([]string{"-q", "teams.zip"}, ficheros...)...)
}

func (b marketplaceBanco) guion(ruta string) guiontest.Resultado {
	b.t.Helper()
	return guiontest.Guion(b.t, guiontest.Opciones{Dir: b.trabajo}, ruta, b.zip)
}

// rechaza: la guardia no puede aceptar el teams.zip tal como está.
func (b marketplaceBanco) rechaza(que string) {
	b.t.Helper()
	r := b.guion(marketplaceGuardia)
	b.t.Run("rechaza "+que, func(t *testing.T) {
		if r.Codigo == 0 {
			t.Fatalf("la guardia acepta %s", que)
		}
	})
}

// acepta: ese guion tiene que salir bien con el teams.zip tal como está.
func (b marketplaceBanco) acepta(que, ruta string) {
	b.t.Helper()
	r := b.guion(ruta)
	b.t.Run(que, func(t *testing.T) {
		if r.Codigo != 0 {
			t.Fatalf("%s: %s salió con %d: %s", que, ruta, r.Codigo, r.Todo())
		}
	})
}

// delZip devuelve un fichero del teams.zip, byte a byte.
func (b marketplaceBanco) delZip(nombre string) string {
	b.t.Helper()
	return b.orden("unzip", "-p", "teams.zip", nombre).Salida
}

func (b marketplaceBanco) leer(nombre string) string {
	b.t.Helper()
	return guiontest.Leer(b.t, filepath.Join(b.trabajo, nombre))
}

// Los casos comparten el directorio y van en el orden del guion original.
func TestMarketplaceLaGuardiaYLaAdaptacionDelPaqueteDeTeams(t *testing.T) {
	// El original daba estas herramientas por hechas porque solo corría en
	// release-marketplace, que instala ImageMagick 7. Dentro de `go test ./...`
	// corre también donde no lo hay, y ahí la propia guardia lo rechazaría todo
	// (el primer «rechaza» salía verde por el motivo equivocado): se salta, y se ve.
	guiontest.Necesita(t, "magick", "zip", "unzip")
	trabajo := t.TempDir()
	b := marketplaceBanco{t: t, trabajo: trabajo, zip: filepath.Join(trabajo, "teams.zip")}
	guiontest.Escribir(t, filepath.Join(trabajo, "manifest.json"), "{}\n")
	outlineBlanco := []string{"-size", "32x32", "xc:none", "-fill", "white", "-draw", "rectangle 4,4 27,27", "outline.png"}

	b.empaquetar("manifest.json")
	b.rechaza("el ZIP sin iconos que generaba IM6")

	b.orden("magick", "-size", "192x192", "xc:orange", "color.png")
	b.orden("magick", outlineBlanco...)
	b.empaquetar("manifest.json", "color.png", "outline.png")
	b.acepta("acepta dos PNG válidos con outline blanco y transparente", marketplaceGuardia)

	b.orden("magick", "-size", "32x32", "xc:none", "-fill", "orange", "-draw", "rectangle 4,4 27,27", "outline.png")
	b.orden("magick", "outline.png", "-alpha", "extract", "antes.png")
	colorOriginal := b.leer("color.png")
	b.empaquetar("manifest.json", "color.png", "outline.png")
	b.rechaza("un outline coloreado")

	b.acepta("preparar-teams.sh adapta el outline coloreado", marketplacePreparar)
	guiontest.Escribir(t, filepath.Join(trabajo, "despues.png"), b.delZip("outline.png"))
	b.orden("magick", "despues.png", "-alpha", "extract", "despues-alfa.png")
	alfa := guiontest.Ejecutar(t, guiontest.Opciones{Dir: trabajo}, "magick", "compare", "-metric", "AE", "antes.png", "despues-alfa.png", "null:")
	colorDespues, manifiestoDespues := b.delZip("color.png"), b.delZip("manifest.json")
	t.Run("la adaptación conserva el alfa, color.png y el manifiesto", func(t *testing.T) {
		if alfa.Codigo != 0 {
			t.Errorf("la adaptación cambió el alfa del outline: %s", alfa.Todo())
		}
		if colorDespues != colorOriginal {
			t.Error("la adaptación cambió color.png")
		}
		if manifiestoDespues != b.leer("manifest.json") {
			t.Errorf("la adaptación cambió el manifiesto: %q", manifiestoDespues)
		}
	})
	b.acepta("preparar-teams.sh sobre un paquete ya adaptado no hace nada y sale bien", marketplacePreparar)

	b.orden("magick", "-size", "31x32", "xc:none", "outline.png")
	b.empaquetar("manifest.json", "color.png", "outline.png")
	b.rechaza("un icono con dimensiones incorrectas")

	b.orden("magick", "-size", "32x32", "xc:white", "outline.png")
	b.empaquetar("manifest.json", "color.png", "outline.png")
	b.rechaza("un outline sin transparencia")

	b.orden("magick", outlineBlanco...)
	guiontest.Escribir(t, filepath.Join(trabajo, "inesperado.png"), b.leer("outline.png"))
	b.empaquetar("manifest.json", "color.png", "outline.png", "inesperado.png")
	b.rechaza("un fichero inesperado en la raíz")
}
