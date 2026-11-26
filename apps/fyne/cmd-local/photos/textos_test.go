// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// TestLosTextosDePhotosEstanEnTodosLosIdiomas: los textos propios de Photos tienen las
// MISMAS claves en los 26 idiomas (PLAN §5.14, C-12), con los huecos del inglés: los de
// formato (%d, %s) los mira Faltan, y los «{count}» que sustituye la Vista, esta prueba.
func TestLosTextosDePhotosEstanEnTodosLosIdiomas(t *testing.T) {
	for _, f := range (local.Propios{Base: textos, Traducciones: traducciones}).Faltan() {
		t.Error(f)
	}
	for clave, v := range textos {
		if llaves(v[0]) != llaves(v[1]) {
			t.Errorf("%q no lleva los mismos huecos en español y en inglés", clave)
		}
	}
	for loc, tr := range traducciones {
		for clave, v := range tr {
			if quiero := llaves(textos[clave][1]); llaves(v) != quiero {
				t.Errorf("%s: %q lleva %q y no %q", loc, clave, llaves(v), quiero)
			}
		}
	}
}

func TestLosTextosDePhotosSalenEnSuIdioma(t *testing.T) {
	p := local.Propios{Base: textos, Traducciones: traducciones}
	for loc, quiero := range map[i18n.Locale]string{"es": "Reproducir", "en": "Play", "de": "Abspielen", "pl": "Odtwórz", "pt-BR": "Reproduzir", "xx": "Play"} {
		if got := p.En(loc, "video.reproducir"); got != quiero {
			t.Errorf("%s: %q; quiero %q", loc, got, quiero)
		}
	}
}

// Al exportar, que el original enlazado ya no esté se dice en el idioma de la app; los
// demás motivos, como los da el sistema, una foto por línea.
func TestLosMotivosDeFalloDeExportarSeDicenEnSuIdioma(t *testing.T) {
	falta := traducciones["de"]["original.falta"]
	got := motivosDeFallo([]error{
		fmt.Errorf("%s: %w", "Mandelbrot.jpg", privatePhotos.ErrOriginalExterno),
		fmt.Errorf("%s: %w", "Vida.jpg", os.ErrPermission),
	}, falta)
	if quiero := "Mandelbrot.jpg: " + falta + "\nVida.jpg: " + os.ErrPermission.Error(); got != quiero {
		t.Fatalf("%q; quiero %q", got, quiero)
	}
}

var reLlave = regexp.MustCompile(`\{[a-z]+\}`)

// llaves son los huecos «{nombre}» de un texto, ordenados.
func llaves(s string) string {
	l := reLlave.FindAllString(s, -1)
	slices.Sort(l)
	return strings.Join(l, " ")
}
