// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// FuenteGlifos es el `.tsx` del que salen los catorce glifos de esta app.
const FuenteGlifos = "apps/frontend/apps/mail/src/components/glyphs.tsx"

// TestLosGlifosSonLosDeLaWeb es el guardián de `glifos.go`, mudado con ellos desde
// `apps/fyne/iconos`: al sacar los glifos de Mail del catálogo compartido, dejarlos sin
// guardián habría sido cambiar duplicación por deriva silenciosa, que es peor.
//
// Se comprueban los dos sentidos que tienen sentido —que la clave exista allí y que la
// ruta sea la MISMA— pero no que estén los 44: de `glyphs.tsx` se porta lo que las
// pantallas usan, y exigir el fichero entero obligaría a traerse la barra del editor
// enriquecido para poder pintar la bandeja.
//
// VISTO EN ROJO (2026-09-22, en el piloto): cambiando un `2` por un `3` en `ClipD`, el
// test imprime las dos rutas y señala `paperclip`. Deshecho.
func TestLosGlifosSonLosDeLaWeb(t *testing.T) {
	web := rutasDeLaWeb(t)
	for clave, nuestra := range glifosPropios {
		switch suya, hay := web[clave]; {
		case !hay:
			t.Errorf("el glifo %q no existe en %s: o se retiró allí, o aquí se inventó", clave, FuenteGlifos)
		case suya != nuestra:
			t.Errorf("el glifo %q tiene otra ruta en la web:\n  web:  %s\n  aquí: %s", clave, suya, nuestra)
		}
	}
	// Los doce del triaje, los diez de la barra del redactor que el común no tiene, los
	// siete de la tabla, el embudo de «Opciones de búsqueda» y la campana tachada de
	// «Silenciar conversación». Los catorce que repetían un nombre del común ya no
	// cuentan.
	if len(glifosPropios) != 31 {
		t.Errorf("hay %d glifos propios y eran 31: si se porta uno más, dilo aquí", len(glifosPropios))
	}
}

// TestNingunGlifoPropioRepiteUnNombreDelComun es el espejo del guardián de la web
// (`glifos-al-comun.test.ts`): un nombre que también está en `Icon.tsx` es el MISMO
// concepto, y se pinta con la ruta del catálogo compartido, no con una copia de aquí.
// Y los que la pantalla pide por esos nombres tienen que ser, ruta a ruta, los del
// común.
//
// VISTO EN ROJO (2026-11-13): con `"trash": PapeleraD` de vuelta en `glifosPropios`, el
// test nombra «trash»; y con la papelera vieja de Mail en `PapeleraD`, dice que no es
// la del catálogo.
func TestNingunGlifoPropioRepiteUnNombreDelComun(t *testing.T) {
	for clave := range glifosPropios {
		if _, es := iconos.Comun(clave); es {
			t.Errorf("%q es un icono del catálogo compartido: se pinta con su ruta, no con una propia", clave)
		}
	}
	delComun := map[string]string{
		"trash": PapeleraD, "clock": RelojD, "tag": EtiquetaD, "download": DescargaD,
		"bold": NegritaD, "italic": CursivaD, "underline": SubrayadoD, "strikethrough": TachadoD,
		"quote": CitaD, "link": EnlaceD, "undo": DeshacerD, "redo": RehacerD,
		"table": TablaD, "image": ImagenD,
	}
	for clave, d := range delComun {
		i, es := iconos.Comun(clave)
		if !es || i.Ruta() != d {
			t.Errorf("%q no es la ruta del catálogo compartido:\n  común: %s\n  aquí:  %s", clave, i.Ruta(), d)
		}
	}
}

// rutasDeLaWeb lee el `const PATHS = { … } as const;` de `glyphs.tsx`.
func rutasDeLaWeb(t *testing.T) map[string]string {
	t.Helper()
	fuente := webref.Read(t, FuenteGlifos)
	i := strings.Index(fuente, "const PATHS = {")
	j := strings.Index(fuente, "} as const;")
	if i < 0 || j < i {
		t.Fatalf("%s ya no declara `const PATHS = { … } as const;`: el espejo hay que rehacerlo", FuenteGlifos)
	}
	re := regexp.MustCompile(`(?m)^ {2}([A-Za-z_][A-Za-z0-9_]*):\s*\n?\s*'((?:[^'\\]|\\.)*)'`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(fuente[i:j], -1) {
		out[m[1]] = m[2]
	}
	if len(out) < 30 {
		t.Fatalf("solo se han leído %d rutas de %s: el patrón ya no vale", len(out), FuenteGlifos)
	}
	return out
}

// glifoPorNombre es la ruta de un `GlyphName` de la web, como lo resuelve allí `Glyph`:
// los propios de esta tabla y, si no, el del catálogo compartido. Vacío si no está.
func glifoPorNombre(nombre string) string {
	if d, ok := glifosPropios[nombre]; ok {
		return d
	}
	if i, ok := iconos.Comun(nombre); ok {
		return i.Ruta()
	}
	return ""
}
