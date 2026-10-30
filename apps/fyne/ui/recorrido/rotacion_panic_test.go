// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package recorrido_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

const (
	pruebaPanicRotacion = "SYTUE_TEST_ROTACION_PANIC"
	causaPanicRotacion  = "fallo original del fixture de rotación"
)

// Un listener de tema que deja de responder después de fallar el fixture
// reproduce la reentrada bloqueada del cleanup. El hijo aísla ese listener y
// permite detectar el bloqueo sin detener el resto de las pruebas diez minutos.
func TestRotacionPropagaElPanicSinReentrarEnTema(t *testing.T) {
	if modo := os.Getenv(pruebaPanicRotacion); modo != "" {
		probarPanicDeRotacion(t, modo)
		return
	}
	binario, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, modo := range []string{"recorrido", "montaje"} {
		t.Run(modo, func(t *testing.T) {
			ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancelar()
			cmd := exec.CommandContext(ctx, binario, "-test.run=^TestRotacionPropagaElPanicSinReentrarEnTema$", "-test.v", "-test.timeout=10s")
			for _, variable := range os.Environ() {
				if !strings.HasPrefix(variable, pruebaPanicRotacion+"=") {
					cmd.Env = append(cmd.Env, variable)
				}
			}
			cmd.Env = append(cmd.Env, pruebaPanicRotacion+"="+modo)
			salida, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("el cleanup se bloqueó al volver a cambiar el tema: %s", salida)
			}
			if err != nil || !strings.Contains(string(salida), causaPanicRotacion) {
				t.Fatalf("el hijo no conservó el error original: %v\n%s", err, salida)
			}
		})
	}
}

func probarPanicDeRotacion(t *testing.T, modo string) {
	t.Helper()
	a := test.NewApp()
	a.Settings().SetTheme(tema.Sytue{})
	w := a.NewWindow("fixture de rotación")
	w.SetContent(widget.NewButton("Acción", nil))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(900, 600))
	var falloActivo atomic.Bool
	a.Settings().AddListener(func(fyne.Settings) {
		if falloActivo.Load() {
			select {} // Solo este proceso hijo; su plazo acota la regresión.
		}
	})
	// No registrar Quit/Close: el proceso hijo es dueño del estado de Fyne
	// deliberadamente inutilizado y termina al devolver el resultado del test.
	defer func() {
		if fallo := recover(); fallo != causaPanicRotacion {
			t.Fatalf("panic recuperado=%v; quiero el original", fallo)
		}
		t.Log(causaPanicRotacion)
	}()
	preparar := func() {
		falloActivo.Store(true)
		panic(causaPanicRotacion)
	}
	switch modo {
	case "recorrido":
		cambiar := func(bool) func() { return func() {} }
		recorrido.ComprobarRotacion(t, w, "fixture", cambiar, false, preparar)
	case "montaje":
		montaje.ComprobarRotacion(t, w, "fixture", iconos.Bases, false, preparar)
	default:
		t.Fatalf("modo de prueba desconocido: %s", modo)
	}
	t.Fatal("el fixture no provocó el panic esperado")
}
