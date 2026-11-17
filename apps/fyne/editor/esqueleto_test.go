// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// renderEsqueleto monta el esqueleto a ese tamaño y devuelve su renderer ya colocado.
func renderEsqueleto(t *testing.T, w, h float32) (*EsqueletoEditor, *esqueletoEditorRenderer) {
	t.Helper()
	e := NuevoEsqueletoEditor(&Contexto{Tipo: Documento, Zoom: 100, Estado: Cargando})
	r := test.TempWidgetRenderer(t, e).(*esqueletoEditorRenderer)
	e.Resize(fyne.NewSize(w, h))
	r.Layout(fyne.NewSize(w, h))
	return e, r
}

// TestElEsqueletoDelEditorMideComoLaWeb ata cada bloque a las constantes de
// `EditorApp.tsx`: las cinco pestañas de 48/64/56/72/60, los cuatro grupos de
// 120/96/140/88, el carril con tres de 32 y el folio `w-[min(70%,720px)]` a 32 de la
// banda. Las bandas miden lo mismo que las del editor de verdad, que es lo que evita el
// salto al montar.
//
// VISTO EN ROJO con el «Abriendo…» de antes: no había esqueleto que medir.
func TestElEsqueletoDelEditorMideComoLaWeb(t *testing.T) {
	_, r := renderEsqueleto(t, 1280, 800)

	if want := []float32{48, 64, 56, 72, 60}; !slices.Equal(AnchosPestanasEsqueleto, want) {
		t.Fatalf("las pestañas del esqueleto son %v y en la web %v", AnchosPestanasEsqueleto, want)
	}
	for i, b := range r.pestanas {
		if b.Size().Width != AnchosPestanasEsqueleto[i] || b.Size().Height != tema.Espacio(3) {
			t.Errorf("la pestaña %d mide %v y la web dice %vx%v", i, b.Size(), AnchosPestanasEsqueleto[i], tema.Espacio(3))
		}
		if y := b.Position().Y; y < AltoCabecera || y+b.Size().Height > AltoCabecera+AltoDeTira() {
			t.Errorf("la pestaña %d está en y=%v, fuera de su tira", i, y)
		}
	}
	x := MargenEsqueleto
	for i, b := range r.pestanas {
		if b.Position().X != x {
			t.Errorf("la pestaña %d empieza en x=%v y con `px-3 gap-3` toca en %v", i, b.Position().X, x)
		}
		x += AnchosPestanasEsqueleto[i] + tema.Espacio(3)
	}
	if want := []float32{120, 96, 140, 88}; !slices.Equal(AnchosGruposEsqueleto, want) {
		t.Fatalf("los grupos del esqueleto son %v y en la web %v", AnchosGruposEsqueleto, want)
	}
	for i, b := range r.grupos {
		if b.Size().Width != AnchosGruposEsqueleto[i] || b.Size().Height != tema.Espacio(10) {
			t.Errorf("el grupo %d mide %v", i, b.Size())
		}
	}
	if len(r.carril) != 3 || r.carril[0].Size().Width != 32 {
		t.Errorf("el carril lleva %d bloques", len(r.carril))
	}
	if len(r.texto) != 8 {
		t.Errorf("el folio lleva %d líneas y la web 8", len(r.texto))
	}

	// El folio: 70 % de la mesa, con tope en 720, centrado y a `mt-8` de la banda.
	mesa := float32(1280) - AnchoCarril
	if got := r.papel.Size().Width; got != 720 {
		t.Errorf("a 1280 el folio mide %v y `min(70%%, 720px)` da 720", got)
	}
	if got, want := r.papel.Position().X, AnchoCarril+(mesa-720)/2; got != want {
		t.Errorf("el folio empieza en x=%v y centrado toca en %v", got, want)
	}
	if got, want := r.papel.Position().Y, AltoCabecera+AltoCinta()+tema.Espacio(8); got != want {
		t.Errorf("el folio empieza en y=%v y la web lo pone en %v", got, want)
	}
	if got := AnchoFolioEsqueleto(800); got != 560 {
		t.Errorf("sobre una mesa de 800 el folio mide %v y el 70 %% es 560", got)
	}
	// Las líneas, al 90/75/60 % del interior del folio.
	dentro := float32(720) - 2*tema.Espacio(12)
	for i, b := range r.texto {
		if got, want := b.Size().Width, dentro*float32(90-(i%3)*15)/100; got-want > 0.01 || want-got > 0.01 {
			t.Errorf("la línea %d mide %v y la web %v", i, got, want)
		}
	}
	// La barra de estado: el último borde va a `--editor-status-h` del fondo.
	if got := r.lineas[4].Position().Y; got != 800-AltoEstado {
		t.Errorf("el borde de la barra de estado está en y=%v", got)
	}
}

// TestElEsqueletoCedeEnUnTelefono: a 360 los grupos no caben y ceden en proporción
// (`flex-shrink`), sin salirse por la derecha.
func TestElEsqueletoCedeEnUnTelefono(t *testing.T) {
	_, r := renderEsqueleto(t, 360, 740)
	ultimo := r.grupos[len(r.grupos)-1]
	if d := ultimo.Position().X + ultimo.Size().Width; d > 360-MargenEsqueleto+0.01 {
		t.Errorf("el último grupo acaba en %v y el ancho útil acaba en %v", d, 360-MargenEsqueleto)
	}
	if r.grupos[0].Size().Width <= r.grupos[1].Size().Width {
		t.Error("al ceder, el grupo de 120 tiene que seguir siendo más ancho que el de 96")
	}
	if got := r.papel.Size().Width; got != (360-AnchoCarril)*0.7 {
		t.Errorf("a 360 el folio mide %v", got)
	}
}

// TestElMarcoCargandoEnseñaElEsqueleto: mientras el documento llega, el marco entero es
// el esqueleto (como la web, que no monta el editor), y al estar listo vuelve el editor.
func TestElMarcoCargandoEnseñaElEsqueleto(t *testing.T) {
	obj := ui.NuevoVacio("lienzo", "")
	ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: Cargando, Lienzo: &lienzoFijo{obj}}
	m := NuevoMarco(ctx, Genericas(), nil)
	r := test.TempWidgetRenderer(t, m)
	if _, ok := m.cubre.(*EsqueletoEditor); !ok {
		t.Fatalf("cargando, el marco enseña %T y no el esqueleto", m.cubre)
	}
	objs := r.Objects()
	if len(objs) != 2 || objs[1] != m.cubre {
		t.Errorf("cargando, el marco pinta %d objetos además del esqueleto", len(objs)-1)
	}
	// Recomponer mientras carga (el título que llega, el chip) no rehace el esqueleto:
	// cada uno nuevo arrancaría otro pulso por bloque.
	antes := m.cubre
	ctx.Titulo = "Informe"
	ctx.Refrescar()
	if m.cubre != antes {
		t.Error("recomponer cargando rehízo el esqueleto")
	}
	ctx.Estado = Listo
	ctx.Refrescar()
	if m.cubre != nil || m.centro != fyne.CanvasObject(obj) {
		t.Error("listo el documento, el esqueleto tiene que dejar sitio al editor")
	}
}

// TestElErrorOfreceReintentarYVolver: el error es el EmptyState de tono error con las
// dos salidas de la web, y «Reintentar» vuelve a pedir el documento.
//
// VISTO EN ROJO con la pantalla de antes: un `ui.Vacio` de texto, sin glifo ni botones.
func TestElErrorOfreceReintentarYVolver(t *testing.T) {
	reintentos, vueltas := 0, 0
	ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: Error, Reintentar: func() { reintentos++ }}
	m := NuevoMarco(ctx, Genericas(), func() { vueltas++ })
	if m.cubre == nil {
		t.Fatal("con el documento sin abrir sigue enseñándose el editor vacío")
	}
	v := vacioDe(m.cubre)
	if v == nil {
		t.Fatalf("el error no es un EmptyState: %T", m.cubre)
	}
	if v.Tono != ui.VacioError {
		t.Error("el error tiene que llevar el tono de error (el aviso en `danger`)")
	}
	reintentar, ok := v.Accion.(*ui.Boton)
	if !ok || reintentar.Texto != "editor.error.retry" {
		t.Fatalf("la acción principal no es «Reintentar»: %#v", v.Accion)
	}
	volver, ok := v.AccionSecundaria.(*ui.Boton)
	if !ok || volver.Texto != "editor.error.back" {
		t.Fatalf("la secundaria no es «Volver»: %#v", v.AccionSecundaria)
	}
	test.Tap(reintentar)
	test.Tap(volver)
	if reintentos != 1 || vueltas != 1 {
		t.Errorf("«Reintentar» se llamó %d veces y «Volver» %d", reintentos, vueltas)
	}

	// Sin a dónde reintentar (un fichero suelto que no se pudo leer), no hay botón.
	sin := vacioDe(PantallaFallo(&Contexto{Tipo: Documento, Estado: Error}))
	if sin.Accion != nil || sin.AccionSecundaria != nil {
		t.Error("sin Reintentar ni Volver no tiene que salir ningún botón")
	}
}

func vacioDe(o fyne.CanvasObject) *ui.Vacio {
	if c, ok := o.(*fyne.Container); ok {
		for _, h := range c.Objects {
			if v, ok := h.(*ui.Vacio); ok {
				return v
			}
		}
	}
	return nil
}

// TestElEsqueletoYElErrorEnClaroYOscuro pinta los dos estados en los dos temas. Con
// `SYTUE_CAPTURAS` deja las imágenes para mirarlas.
func TestElEsqueletoYElErrorEnClaroYOscuro(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, variante := range []string{"claro", "oscuro"} {
		tema.Elegir(variante)
		for _, e := range []Estado{Cargando, Error} {
			ctx := &Contexto{Tipo: Documento, Zoom: 100, Estado: e, Reintentar: func() {}}
			m := NuevoMarco(ctx, Genericas(), func() {})
			m.Resize(fyne.NewSize(1280, 800))
			img := pinta(t, 1280, 800, m)
			// El fondo es `surface` de esa variante, no el blanco del lienzo de prueba.
			if got, want := px(img, 4, int(escala*(AltoCabecera+AltoCinta()+40))), tema.Color("surface", tema.De(0)); !cerca(got, want) {
				t.Errorf("%s/%d: el fondo pinta %v y `surface` es %v", variante, e, got, want)
			}
			if dir := os.Getenv("SYTUE_CAPTURAS"); dir != "" {
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("editor-estado-%d-%s.png", e, variante)))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, img)
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
