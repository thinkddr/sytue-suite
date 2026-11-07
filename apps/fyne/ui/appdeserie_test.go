// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNingunTestMontaUnaAppDeSerie: en este paquete la app de pruebas es la que monta
// TestMain, con el tema de la suite. `test.NewTempApp` la sustituye por una de serie, y
// su limpieza no devuelve la anterior: monta OTRA de serie. Todo lo que corra detrás se
// queda sin Figtree ni las medidas de la suite, y con `-shuffle` el que cae es otro test
// en otro fichero, cada vez uno distinto.
//
// Visto: TestLaLetraEsFigtree (aviso_test.go), y en ci-desktop de v3.19.1 (semilla
// 1794070548876265296) TestElAreaAvanzaUnaLineaDeCSSPorFila y
// TestNingunDesplazamientoPintaSombraDeBorde, detrás de
// TestLaCasillaDelDespertarSoloConElModoDeFondo. Para otra identidad de app, se envuelve
// la que hay: `fyne.SetCurrentApp(appRooms{fyne.CurrentApp()})`.
//
// VISTO EN ROJO devolviendo `test.NewTempApp(t)` a barracuenta_despertar_test.go.
func TestNingunTestMontaUnaAppDeSerie(t *testing.T) {
	ficheros, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	llamada := regexp.MustCompile(`\btest\.(NewTempApp|NewApp)\(`)
	for _, f := range ficheros {
		datos, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, linea := range strings.Split(string(datos), "\n") {
			codigo, _, _ := strings.Cut(linea, "//")
			if !llamada.MatchString(codigo) {
				continue
			}
			if f == "ui_test.go" || f == "lateral_rotacion_test.go" {
				// TestMain monta la de la suite; la rotación crea la suya con el tema
				// puesto y devuelve la anterior.
				continue
			}
			t.Errorf("%s:%d monta una app de serie: %s", f, i+1, strings.TrimSpace(linea))
		}
	}
}
