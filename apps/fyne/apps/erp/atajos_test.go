// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLaHojaDeAtajosEsLaDeLaWeb compara las filas con `GRUPOS_ERP` de `lib/atajos.ts`,
// en el mismo orden: grupos y descripciones. Una fila nueva en un lado y no en el otro
// sale aquí en rojo.
func TestLaHojaDeAtajosEsLaDeLaWeb(t *testing.T) {
	t.Parallel()
	fuente := webref.Read(t, "apps/frontend/apps/erp/src/lib/atajos.ts")
	inicio := strings.Index(fuente, "GRUPOS_ERP:")
	if inicio < 0 {
		t.Fatal("no está `GRUPOS_ERP` en atajos.ts: ¿ha cambiado su forma?")
	}
	filas := regexp.MustCompile(`(?:titulo|descripcion): '([a-zA-Z.]+)'`).FindAllStringSubmatch(fuente[inicio:], -1)
	web := make([]string, 0, len(filas))
	for _, m := range filas {
		web = append(web, m[1])
	}
	var nativa []string
	for _, g := range gruposDelERP {
		nativa = append(nativa, g.titulo)
		for _, f := range g.filas {
			nativa = append(nativa, f.descripcion)
		}
	}
	if len(web) == 0 || !slices.Equal(web, nativa) {
		t.Errorf("la hoja de Fyne es\n  %v\ny la de la web\n  %v", nativa, web)
	}
}

// TestLasTeclasDeLaHojaSonLasDeLaWeb: además de las descripciones, cada fila enseña
// las mismas teclas que su gemela de `GRUPOS_ERP`. Sin esto, «n» cambiada por «c» en
// un lado (o un «Esc» sin traducir en el otro) pasaba el test de las descripciones.
func TestLasTeclasDeLaHojaSonLasDeLaWeb(t *testing.T) {
	t.Parallel()
	fuente := webref.Read(t, "apps/frontend/apps/erp/src/lib/atajos.ts")
	inicio := strings.Index(fuente, "GRUPOS_ERP:")
	if inicio < 0 {
		t.Fatal("no está `GRUPOS_ERP` en atajos.ts: ¿ha cambiado su forma?")
	}
	fila := regexp.MustCompile(`teclas: \([^)]*\) => \[([^\]]*)\],\s*descripcion: '([a-zA-Z.]+)'`)
	tecla := regexp.MustCompile(`^(?:(atajoDelSistema|t)\()?'([^']+)'\)?$`)
	// Con `t` la identidad, una tecla con nombre sale como su clave en los dos lados.
	identidad := func(k string) string { return k }
	web := map[string][]string{}
	for _, m := range fila.FindAllStringSubmatch(fuente[inicio:], -1) {
		var teclas []string
		for _, trozo := range strings.Split(m[1], ",") {
			partes := tecla.FindStringSubmatch(strings.TrimSpace(trozo))
			if partes == nil {
				t.Fatalf("tecla de la web que el test no sabe leer: %q", trozo)
			}
			k := partes[2]
			if partes[1] == "atajoDelSistema" {
				k = ui.AtajoDelSistema(k)
			}
			teclas = append(teclas, k)
		}
		web[m[2]] = teclas
	}
	filas := 0
	for _, g := range gruposDelERP {
		for _, f := range g.filas {
			filas++
			if got, want := f.teclas(identidad), web[f.descripcion]; !slices.Equal(got, want) {
				t.Errorf("%s: Fyne enseña %v y la web %v", f.descripcion, got, want)
			}
		}
	}
	if len(web) != filas {
		t.Errorf("la web tiene %d filas con teclas y Fyne %d", len(web), filas)
	}
}

// TestInterrogacionAbreLaHojaDeAtajos: «?» sin foco abre la hoja, traducida y con las
// teclas del sistema, también sin ninguna lista abierta; Escape la cierra. Con la paleta
// abierta o con otra capa delante, la tecla no apila nada.
func TestInterrogacionAbreLaHojaDeAtajos(t *testing.T) {
	p := pantallaDePrueba(t)
	w := test.NewWindow(container.NewStack())
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	c := w.Canvas()
	fyne.DoAndWait(func() {
		p.Atajos(c, nil)
		c.OnTypedRune()('a')
	})
	if c.Overlays().Top() != nil {
		t.Fatal("una letra cualquiera abre una capa")
	}
	fyne.DoAndWait(func() { c.OnTypedRune()('?') })
	capa := c.Overlays().Top()
	if capa == nil {
		t.Fatal("«?» no abre la hoja de atajos")
	}
	var textos []string
	recorrer(capa, func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Tecla:
			textos = append(textos, x.Texto)
		case *canvas.Text:
			textos = append(textos, x.Text)
		}
	})
	todo := strings.Join(textos, "|")
	for _, quiero := range []string{
		p.t("shortcuts.title"), p.t("shortcuts.palette"), p.t("shortcuts.new"), p.t("shortcuts.close"),
		p.t("shortcuts.help"), ui.AtajoDelSistema("Ctrl+K"), "Esc", "?", "/",
	} {
		if !strings.Contains(todo, quiero) {
			t.Errorf("la hoja no lleva %q: %s", quiero, todo)
		}
	}
	if strings.Contains(todo, "shortcuts.") {
		t.Errorf("la hoja enseña claves sin traducir: %s", todo)
	}

	n := len(c.Overlays().List())
	fyne.DoAndWait(func() { c.OnTypedRune()('?') })
	if got := len(c.Overlays().List()); got != n {
		t.Errorf("«?» con la hoja abierta apila otra capa: %d → %d", n, got)
	}

	fyne.DoAndWait(func() {
		escape := &fyne.KeyEvent{Name: fyne.KeyEscape}
		if f := c.Focused(); f != nil {
			f.TypedKey(escape)
		} else {
			c.OnTypedKey()(escape)
		}
	})
	if c.Overlays().Top() != nil {
		t.Error("Escape no cierra la hoja de atajos")
	}

	fyne.DoAndWait(func() {
		p.AbrirPaleta()
		c.Unfocus()
		c.OnTypedRune()('?')
	})
	if c.Overlays().Top() != nil {
		t.Error("con la paleta abierta «?» abre la hoja encima")
	}
}
