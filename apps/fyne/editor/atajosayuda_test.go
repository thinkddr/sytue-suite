// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// La hoja «Atajos de teclado» (`atajosayuda.go`), gemela de `atajosAyuda.ts`.
//
// VISTO EN ROJO (2026-11-12): sin la hoja, Ctrl+/ no era de nadie y Vista no la ofrecía.

// TestLaHojaDeAtajosTieneTodosLosDelRegistro: para cada tipo, las filas del registro son
// exactamente sus atajos, en la pestaña y el grupo de su comando y en el orden de la cinta.
func TestLaHojaDeAtajosTieneTodosLosDelRegistro(t *testing.T) {
	for _, kind := range []string{office.Documento, office.Hoja, office.Presentacion} {
		var quiero, tengo []string
		for _, id := range office.IDsEnOrden() {
			def, _ := office.BuscarComando(id)
			if !def.Existe(kind) {
				continue
			}
			for _, a := range def.Shortcuts {
				quiero = append(quiero, id+" "+a.Keys)
			}
		}
		orden := []string{}
		for _, p := range office.PestanasDe(kind) {
			orden = append(orden, p.ID)
		}
		ultima := -1
		for _, g := range AtajosDelRegistro(kind) {
			i := slices.Index(orden, g.Tab)
			if i < ultima {
				t.Errorf("%s: la pestaña %q sale fuera del orden de la cinta", kind, g.Tab)
			}
			ultima = i
			for _, f := range g.Filas {
				def, _ := office.BuscarComando(f.ID)
				if def.Tab != g.Tab || def.Group != g.Group {
					t.Errorf("%s: %s va en %s › %s y su comando es de %s › %s", kind, f.ID, g.Tab, g.Group, def.Tab, def.Group)
				}
				tengo = append(tengo, f.ID+" "+f.Keys)
			}
		}
		if len(quiero) < 5 {
			t.Fatalf("%s: solo %d atajos en el registro: ha dejado de leerse", kind, len(quiero))
		}
		slices.Sort(quiero)
		slices.Sort(tengo)
		if !slices.Equal(quiero, tengo) {
			t.Errorf("%s: la hoja no son los atajos del registro\n  registro: %v\n  hoja:     %v", kind, quiero, tengo)
		}
	}
}

// TestLosRotulosDelValorSonLosDeLaWeb: cada atajo con valor tiene su rótulo, y el mismo que
// en `atajosAyuda.ts`; si no, Ctrl+X diría «Copiar» en una de las dos.
func TestLosRotulosDelValorSonLosDeLaWeb(t *testing.T) {
	var conValor []string
	for _, id := range office.IDsEnOrden() {
		def, _ := office.BuscarComando(id)
		for _, a := range def.Shortcuts {
			if a.Value != "" {
				conValor = append(conValor, id+":"+a.Value)
			}
		}
	}
	web := map[string]string{}
	fuente := webref.Read(t, "apps/frontend/packages/office/src/editor/atajosAyuda.ts")
	for _, m := range regexp.MustCompile(`'([A-Za-z]+:[A-Za-z0-9]+)': '([A-Za-z0-9.]+)'`).FindAllStringSubmatch(fuente, -1) {
		web[m[1]] = m[2]
	}
	if !reflect.DeepEqual(web, RotuloDelValor) {
		t.Errorf("RotuloDelValor no es el de la web:\n  web:  %v\n  aquí: %v", web, RotuloDelValor)
	}
	for _, k := range conValor {
		if _, ok := RotuloDelValor[k]; !ok {
			t.Errorf("el atajo con valor %s no tiene rótulo en la hoja", k)
		}
	}
}

// TestLaHojaDiceLosAtajosDelMarco: cada combinación que el marco nativo atiende sale en la
// hoja, y la hoja no promete ninguna que no atienda.
func TestLaHojaDiceLosAtajosDelMarco(t *testing.T) {
	fila := map[fyne.KeyName]string{
		fyne.KeyP: "Ctrl+P", fyne.KeyPlus: "Ctrl++", fyne.KeyEqual: "Ctrl++",
		fyne.KeyMinus: "Ctrl+-", fyne.Key0: "Ctrl+0", fyne.KeySlash: AtajoAyuda,
		fyne.KeyC: "Ctrl+Shift+C",
	}
	usadas := map[string]bool{}
	for _, a := range AtajosDelEditor() {
		k, ok := fila[a.KeyName]
		if !ok {
			t.Errorf("%s lo atiende el marco y la hoja no lo enseña", a.ShortcutName())
		}
		usadas[k] = true
	}
	for _, f := range append(slices.Clone(AtajosDelMarco), AtajosDeDocus...) {
		if !usadas[f.Keys] {
			t.Errorf("la hoja enseña %s y el marco no lo atiende", f.Keys)
		}
	}
}

// TestLaHojaDeDocusDiceGuardarYBuscar: el folio de Docus atiende Ctrl/Cmd+S y Ctrl/Cmd+F
// (`docus/escribir.go`), así que su hoja los enseña; Calcus y Diapos no los tienen.
func TestLaHojaDeDocusDiceGuardarYBuscar(t *testing.T) {
	fuente := webref.Read(t, "apps/fyne/editor/docus/escribir.go")
	for _, caso := range []string{"s.KeyName == fyne.KeyS", "s.KeyName == fyne.KeyF"} {
		if !strings.Contains(fuente, caso) {
			t.Fatalf("el folio ya no atiende %q: quítalo de AtajosDelFolio", caso)
		}
	}
	teclas := func(tipo Tipo) []string {
		ctx := &Contexto{Tipo: tipo, T: traductorPrueba(t)}
		var out []string
		for _, a := range ctx.gruposDeAtajos(func(k string, _ func(string) string) string { return k })[0].Atajos {
			out = append(out, a.Teclas...)
		}
		return out
	}
	if got := teclas(Documento); !slices.Equal(got[:2], []string{"Ctrl+S", "Ctrl+F"}) {
		t.Errorf("Docus: el marco empieza por %v; quiero Ctrl+S y Ctrl+F", got)
	}
	// El recuento (Ctrl+Mayús+C) solo lo atiende Docus.
	if got := teclas(Documento); !slices.Contains(got, "Ctrl+Shift+C") {
		t.Errorf("Docus: la hoja no enseña el recuento: %v", got)
	}
	for _, tipo := range []Tipo{Hoja, Presentacion} {
		if got := teclas(tipo); slices.Contains(got, "Ctrl+Shift+C") {
			t.Errorf("%v: la hoja promete el recuento y nadie lo atiende", tipo)
		}
	}
	for _, tipo := range []Tipo{Hoja, Presentacion} {
		if got := teclas(tipo); slices.Contains(got, "Ctrl+S") || slices.Contains(got, "Ctrl+F") {
			t.Errorf("%v: la hoja promete Ctrl+S o Ctrl+F y nadie los atiende: %v", tipo, got)
		}
	}
}

// TestElTextoDeLasTeclas es `textoAtajo`: símbolos en un Mac, nombres del idioma en lo demás.
func TestElTextoDeLasTeclas(t *testing.T) {
	tr := func(k string) string {
		return map[string]string{"shortcut.ctrl": "Strg", "shortcut.shift": "Umschalt"}[k]
	}
	for _, c := range []struct {
		mac          bool
		keys, quiero string
	}{
		{false, "Ctrl+Shift+X", "Strg+Umschalt+X"},
		{false, "Ctrl+Alt+1", "Strg+Alt+1"},
		{true, "Ctrl+Shift+X", "⌘⇧X"},
		{true, "Ctrl+Alt+1", "⌘⌥1"},
		{true, "Ctrl+Y", "⌘⇧Z"},
		{false, "F6", "F6"},
	} {
		goos := "linux"
		if c.mac {
			goos = "darwin"
		}
		if got := TextoAtajoPara(goos, c.keys, tr); got != c.quiero {
			t.Errorf("%v %q: %q; quiero %q", c.mac, c.keys, got, c.quiero)
		}
	}
}

// TestLaHojaDeAtajosSeAbreConCtrlBarraYDesdeVista: Ctrl/Cmd+/ en la ventana la abre y la
// cierra, Escape también la cierra, y «Vista › Mostrar» tiene el botón que la abre.
func TestLaHojaDeAtajosSeAbreConCtrlBarraYDesdeVista(t *testing.T) {
	ctx := &Contexto{Tipo: Presentacion, Estado: Listo, Zoom: 100, T: traductorPrueba(t)}
	ctx.AtajosPropios = func() []ui.GrupoDeAtajos {
		return []ui.GrupoDeAtajos{{Titulo: "Presentar › Navegar"}}
	}
	m := NuevoMarco(ctx, Genericas(), nil)
	w := test.NewTempWindow(t, m)
	w.Resize(fyne.NewSize(1280, 800))
	var lienzo interface{ TypedShortcut(fyne.Shortcut) }
	if v := reflect.ValueOf(w.Canvas()).Elem(); v.Kind() == reflect.Struct && v.NumField() > 0 {
		lienzo, _ = v.Field(0).Interface().(interface{ TypedShortcut(fyne.Shortcut) })
	}
	if lienzo == nil {
		t.Fatal("el lienzo de prueba no recibe atajos")
	}
	barra := &desktop.CustomShortcut{KeyName: fyne.KeySlash, Modifier: fyne.KeyModifierShortcutDefault}
	lienzo.TypedShortcut(barra)
	if ctx.hojaAtajos == nil || !ctx.hojaAtajos.Abierto() {
		t.Fatal("Ctrl+/ no abre la hoja de atajos")
	}
	if ctx.hojaAtajos.Titulo != "Atajos de teclado" {
		t.Errorf("el título de la hoja: %q", ctx.hojaAtajos.Titulo)
	}
	grupos := ctx.GruposDeAtajos()
	if grupos[0].Titulo != "Todo el editor" || grupos[len(grupos)-1].Titulo != "Presentar › Navegar" {
		t.Errorf("el marco va primero y lo propio al final: %q … %q", grupos[0].Titulo, grupos[len(grupos)-1].Titulo)
	}
	lienzo.TypedShortcut(barra)
	if ctx.hojaAtajos != nil && ctx.hojaAtajos.Abierto() {
		t.Fatal("Ctrl+/ otra vez no cierra la hoja")
	}
	// Con Mayús no es la hoja: en un teclado español Ctrl+Mayús+7 escribe «/».
	if ctx.AtajoDelEditor(&desktop.CustomShortcut{KeyName: fyne.KeySlash, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}) {
		t.Error("Ctrl+Mayús+/ no es el atajo de la hoja")
	}

	// Desde Vista › Mostrar, y Escape la cierra.
	var boton *BotonCinta
	for _, b := range botonesCinta(cuerpoVista(ctx)) {
		if b.Rotulo == "Atajos de teclado" {
			boton = b
		}
	}
	if boton == nil || boton.Icono != iconos.EdTeclado || boton.Inerte {
		t.Fatalf("Vista no tiene el botón de la hoja de atajos: %+v", boton)
	}
	test.Tap(boton)
	d := ctx.hojaAtajos
	if d == nil || !d.Abierto() {
		t.Fatal("el botón de Vista no abre la hoja")
	}
	d.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if d.Abierto() {
		t.Error("Escape no cierra la hoja")
	}
}

// botonesCinta son los botones de una banda de la cinta, grupo a grupo.
func botonesCinta(o fyne.CanvasObject) []*BotonCinta {
	switch v := o.(type) {
	case *BotonCinta:
		return []*BotonCinta{v}
	case *Grupo:
		var out []*BotonCinta
		for _, c := range v.Controles {
			out = append(out, botonesCinta(c)...)
		}
		return out
	case *fyne.Container:
		var out []*BotonCinta
		for _, c := range v.Objects {
			out = append(out, botonesCinta(c)...)
		}
		return out
	}
	return nil
}

// traductorPrueba es el catálogo de verdad: la hoja tiene que encontrar sus claves.
func traductorPrueba(t *testing.T) func(string, i18n.Vars) string {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatalf("no se pudo cargar el catálogo: %v", err)
	}
	return tr.T
}
