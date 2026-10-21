// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// TestElAtrasDeAndroidLlegaComoLaTecla: el «atrás» del receptor de Android 13+ va a la
// ranura de teclas de la ventana si nada está enfocado, y al control enfocado si lo hay,
// como la tecla que ya no llega. Y el nombre JNI que exporta Go es el del método nativo
// del Java: si no coinciden, Android lanza UnsatisfiedLinkError al pulsar «atrás».
//
// VISTO EN ROJO sin la rama del control enfocado: «atrás» se va a la ranura.
func TestElAtrasDeAndroidLlegaComoLaTecla(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	campo := widget.NewEntry()
	w := a.NewWindow("x")
	w.SetContent(campo)
	var ranura []fyne.KeyName
	w.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) { ranura = append(ranura, k.Name) })
	atrasEn(a, w.Canvas())
	if len(ranura) != 1 || ranura[0] != mobile.KeyBack {
		t.Fatalf("sin foco, la ranura recibió %v", ranura)
	}
	w.Canvas().Focus(campo)
	atrasEn(a, w.Canvas())
	if len(ranura) != 1 {
		t.Error("con un campo enfocado, «atrás» fue a la ranura y no al campo")
	}

	java, err := os.ReadFile("../android/java/eu/sytue/nativo/Atras.java")
	if err != nil {
		t.Fatal(err)
	}
	goF, err := os.ReadFile("atras_android.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(java), "package eu.sytue.nativo;") || !strings.Contains(string(java), "static native void pulsado();") ||
		!strings.Contains(string(goF), "//export Java_eu_sytue_nativo_Atras_pulsado") {
		t.Error("el método nativo de Atras.java y la función que exporta Go ya no se llaman igual")
	}
}

func TestElAtrasDelSistemaVaALaVentanaDeDelante(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)
	editor := a.NewWindow("Editor")
	editor.SetContent(widget.NewLabel("Editor"))
	presentacion := a.NewWindow("Presentación")
	presentacion.SetContent(widget.NewLabel("Presentación"))
	var editorRecibe, presentacionRecibe int
	editor.Canvas().SetOnTypedKey(func(*fyne.KeyEvent) { editorRecibe++ })
	presentacion.Canvas().SetOnTypedKey(func(*fyne.KeyEvent) { presentacionRecibe++ })
	atrasEnVentanaActual(a)
	if editorRecibe != 0 || presentacionRecibe != 1 {
		t.Fatalf("Atrás no llegó a la ventana superior: editor=%d presentación=%d", editorRecibe, presentacionRecibe)
	}
	presentacion.Close()
	atrasEnVentanaActual(a)
	if editorRecibe != 1 || presentacionRecibe != 1 {
		t.Fatalf("tras cerrar la presentación, Atrás no volvió al editor: %d/%d", editorRecibe, presentacionRecibe)
	}
}
