// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// figura saca cada forma autocerrada de un SVG: `<path d="…" />`, `<circle … />`,
// `<rect … />`.
var figura = regexp.MustCompile(`<(\w+)\s+([^>]*?)\s*/>`)

// siguienteGlifo es la línea con la que empieza la entrada siguiente de `GLYPHS`.
var siguienteGlifo = regexp.MustCompile(`\n {2}[a-zA-Z]+: `)

// figuras normaliza un cuerpo a la lista de sus formas con los espacios colapsados: el
// mismo dibujo con otro reflow de Prettier sigue siendo igual, y un atributo cambiado
// sigue siendo distinto.
func figuras(s string) []string {
	ms := figura.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m[1]+" "+strings.Join(strings.Fields(m[2]), " "))
	}
	return out
}

// entradaDeLaWeb es lo que hay entre `  nombre: ` y la entrada siguiente del objeto
// `GLYPHS`. Los comentarios de en medio no casan con `figura`, así que no estorban.
func entradaDeLaWeb(t *testing.T, fuente, nombre string) string {
	t.Helper()
	i := strings.Index(fuente, "\n  "+nombre+": ")
	if i < 0 {
		t.Fatalf("glyphs.tsx ya no declara %q: el espejo hay que rehacerlo", nombre)
	}
	resto := fuente[i+1:]
	fin := strings.Index(resto, "} as const")
	if loc := siguienteGlifo.FindStringIndex(resto); len(loc) == 2 && loc[0] < fin {
		fin = loc[0]
	}
	return resto[:fin]
}

// TestLosGlifosSonLosDeLaWeb compara cada cuerpo de `glifos.go`, forma a forma y por
// IGUALDAD de la lista, contra el `.tsx` del que sale.
//
// VISTO EN ROJO (2026-09-22, en el frontal): cambiando el `r="1"` del segundo círculo
// de `glifoMas` por `r="2"`, el test nombra «more» y enseña las dos listas. Deshecho.
func TestLosGlifosSonLosDeLaWeb(t *testing.T) {
	const fuente = "apps/frontend/apps/drive/src/components/glyphs.tsx"
	web := webref.Read(t, fuente)
	for nombre, cuerpo := range glifosWeb {
		suyas, mias := figuras(entradaDeLaWeb(t, web, nombre)), figuras(cuerpo)
		if len(suyas) == 0 {
			t.Errorf("%s: no se ha leído ni una figura de %s: el patrón ya no vale", nombre, fuente)
			continue
		}
		if strings.Join(suyas, " | ") != strings.Join(mias, " | ") {
			t.Errorf("%s no es el de la web:\n  web:  %v\n  aquí: %v", nombre, suyas, mias)
		}
	}
}

// TestNingunGlifoPropioRepiteUnNombreDelComun es el espejo del guardián de la web
// (`glifos-al-comun.test.ts`): `trash`, `download`, `link`, `lock` o `users` son del
// catálogo compartido y Drive los pinta con SU ruta, no con una copia con otro trazado.
//
// VISTO EN ROJO (2026-11-13): con `"trash": glifoPapelera` de vuelta en `glifosWeb`, el
// test nombra «trash».
func TestNingunGlifoPropioRepiteUnNombreDelComun(t *testing.T) {
	for clave := range glifosWeb {
		if _, es := iconos.Comun(clave); es {
			t.Errorf("%q es un icono del catálogo compartido: se pinta con su ruta, no con una propia", clave)
		}
	}
	delComun := map[string]string{
		"upload": glifoSubir, "download": glifoDescargar, "link": glifoEnlace, "clock": glifoReloj,
		"trash": glifoPapelera, "mail": glifoCorreo, "users": glifoUsuarios, "lock": glifoCandado,
	}
	for clave, cuerpo := range delComun {
		i, es := iconos.Comun(clave)
		if !es || strings.Join(figuras(cuerpo), "") != strings.Join(figuras(`<path d="`+i.Ruta()+`" />`), "") {
			t.Errorf("%q no es el dibujo del catálogo compartido: %s", clave, cuerpo)
		}
	}
}
