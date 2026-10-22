// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// pendientesEd son los iconos que `editor/icons.tsx` tiene y este catálogo TODAVÍA no.
//
// Existe para que el guardián pueda comparar por IGUALDAD y no por pertenencia: con
// esta lista, un icono NUEVO en la web no está ni en el catálogo ni aquí, y el test se
// pone rojo. Sin ella habría que comparar solo en un sentido, que es como los doce
// `middleware.ts` de `docs/CONVENTIONS.md` §13.2 acabaron divergiendo.
//
// Bajar de aquí un icono es portarlo: se le da constante y entrada, y se borra su
// línea. Subirlo no: un icono que ya se usa no se retira a esta lista.
var pendientesEd = []string{
	"checkbox", "dropdown",
	"formFill",
	"margins",
}

// rutasDelEditor saca los `PATHS` de `editor/icons.tsx`, igual que rutasDeLaWeb con
// Icon.tsx.
func rutasDelEditor(t *testing.T) map[string]string {
	t.Helper()
	fuente := webref.Read(t, "apps/frontend/packages/office/src/editor/icons.tsx")
	i := strings.Index(fuente, "const PATHS = {")
	j := strings.Index(fuente, "} as const;")
	if i < 0 || j < i {
		t.Fatal("editor/icons.tsx ya no declara `const PATHS = { … } as const;`: el espejo hay que rehacerlo")
	}
	re := regexp.MustCompile(`(?m)^ {2}([A-Za-z_][A-Za-z0-9_]*):\s*\n?\s*'((?:[^'\\]|\\.)*)'`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(fuente[i:j], -1) {
		out[m[1]] = m[2]
	}
	if len(out) < 100 {
		t.Fatalf("solo se han leído %d rutas de editor/icons.tsx: el patrón ya no vale", len(out))
	}
	return out
}

// TestElCatalogoDelEditorEsElDeLaWeb es el guardián del catálogo del editor: cada
// icono portado, ruta a ruta, contra `editor/icons.tsx`; y la suma de lo portado más
// lo pendiente tiene que dar EXACTAMENTE el conjunto de la web.
func TestElCatalogoDelEditorEsElDeLaWeb(t *testing.T) {
	web := rutasDelEditor(t)
	nuestros := map[string]string{}
	for i, e := range catalogoEd {
		if Ed(i) == EdNinguno {
			continue
		}
		if otro, ya := nuestros[e.clave]; ya {
			t.Errorf("la clave %q está dos veces en el catálogo del editor (%q y %q)", e.clave, otro, e.d)
		}
		nuestros[e.clave] = e.d
	}
	for clave, d := range nuestros {
		switch suyo, hay := web[clave]; {
		case !hay:
			t.Errorf("el icono %q no existe en editor/icons.tsx: o se retiró allí, o aquí se inventó", clave)
		case suyo != d:
			t.Errorf("el icono %q tiene otra ruta en editor/icons.tsx:\n  web:  %s\n  aquí: %s", clave, suyo, d)
		}
	}
	for _, clave := range pendientesEd {
		if _, hay := web[clave]; !hay {
			t.Errorf("%q está en pendientesEd y ya no existe en editor/icons.tsx: borra su línea", clave)
		}
		if _, hay := nuestros[clave]; hay {
			t.Errorf("%q está portado Y en pendientesEd: borra su línea de la lista", clave)
		}
	}
	if n := len(nuestros) + len(pendientesEd); n != len(web) {
		t.Errorf("la web tiene %d iconos de editor y aquí se contabilizan %d (%d portados + %d pendientes): "+
			"hay uno nuevo que no está en ningún sitio", len(web), n, len(nuestros), len(pendientesEd))
	}
}

// TestElCatalogoDelEditorNoTieneHuecos comprueba que cada constante tiene su entrada.
func TestElCatalogoDelEditorNoTieneHuecos(t *testing.T) {
	if len(catalogoEd) != int(numEd) {
		t.Fatalf("hay %d constantes de icono de editor y el catálogo tiene %d entradas", int(numEd), len(catalogoEd))
	}
	for i := EdNinguno + 1; i < numEd; i++ {
		if catalogoEd[i].d == "" {
			t.Errorf("el icono de editor %d (%s) no tiene ruta", int(i), i)
		}
	}
}
