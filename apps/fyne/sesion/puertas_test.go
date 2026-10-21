// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestSoloSeAbreConElSistemaPorSuPuerta: darle una dirección al sistema (`OpenURL`) y
// enseñar un aviso con el de Fyne (`SendNotification`) se hacen desde UN sitio cada uno,
// que es donde se mira lo que se les pasa. Había diecinueve `OpenURL` repartidos por las
// apps, cada uno fiándose de que el servicio hubiera filtrado la dirección (NAT-11), y
// tres `SendNotification` con texto de otra persona (NAT-12). Uno nuevo fuera de su
// puerta pone esto en rojo.
func TestSoloSeAbreConElSistemaPorSuPuerta(t *testing.T) {
	puertas := map[string]*regexp.Regexp{
		// `.OpenURL` con paréntesis o sin él: `a.OpenURL` como valor también la cruza.
		"nucleo/api/abrir.go":          regexp.MustCompile(`\.OpenURL\b`),
		"fyne/sesion/avisorespaldo.go": regexp.MustCompile(`\.(SendNotification|ScheduleNotification)\b`),
	}
	for _, raiz := range []string{"..", filepath.Join("..", "..", "nucleo")} {
		err := filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return err
			}
			abs, _ := filepath.Abs(ruta)
			fuente, err := os.ReadFile(ruta)
			if err != nil {
				return err
			}
			for puerta, llamada := range puertas {
				if strings.HasSuffix(filepath.ToSlash(abs), puerta) {
					continue
				}
				for n, linea := range strings.Split(string(fuente), "\n") {
					if llamada.MatchString(linea) && !strings.HasPrefix(strings.TrimSpace(linea), "//") {
						t.Errorf("%s:%d llama a %s fuera de su puerta (%s): %s", ruta, n+1, llamada, puerta, strings.TrimSpace(linea))
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// avisosContados es una app que cuenta los avisos de Fyne que se le piden.
type avisosContados struct {
	fyne.App
	n int
}

func (a *avisosContados) SendNotification(*fyne.Notification) { a.n++ }

// TestElRespaldoDelAvisoNoSaleEnWindows: el `SendNotification` de Fyne en Windows mete el
// texto en un guion de PowerShell entre comillas dobles. Con el cuerpo de un aviso del
// Hub, que escribe otra persona, no se llama (NAT-12).
func TestElRespaldoDelAvisoNoSaleEnWindows(t *testing.T) {
	a := &avisosContados{App: test.NewApp()}
	defer a.Quit()
	avisoDeRespaldo("windows", a, "Correo nuevo", "$(lo que sea)")
	if a.n != 0 {
		t.Fatal("en Windows el respaldo ha pasado el texto al aviso de Fyne")
	}
	avisoDeRespaldo("linux", a, "Correo nuevo", "hola")
	if a.n != 1 {
		t.Fatalf("fuera de Windows el respaldo enseña el aviso: %d", a.n)
	}
}
