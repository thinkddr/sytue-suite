// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaPapeleraDiceCuadernoYCuandoSeVacia: cada fila lleva el cuaderno de donde viene
// la nota y «Se borra en N días»; la que no caduca (retención legal o purga apagada) lo
// dice sin decir por qué. Es lo mismo que comprueba `papelera.test.tsx` en la web.
func TestLaPapeleraDiceCuadernoYCuandoSeVacia(t *testing.T) {
	ahora := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	en30 := ahora.Add(30*24*time.Hour - time.Second)
	p := NuevaPapelera(trInterno(t))
	p.Ahora = func() time.Time { return ahora }
	p.Fase = Exito
	p.Notas = DeNotasPapelera([]api.TrashedNote{
		{Note: api.Note{ID: "x1", Title: "Billetes"}, NotebookTitle: "Viaje", DeletedAt: ahora, PurgeAt: &en30},
		{Note: api.Note{ID: "x2", Title: "Contrato"}, NotebookTitle: "Legal", DeletedAt: ahora},
	})
	var textos string
	fyne.DoAndWait(func() {
		w := test.NewTempWindow(t, p.Objeto(tema.Claro))
		w.Resize(fyne.NewSize(900, 600))
		textos = strings.Join(textosDe(w.Content()), "|")
	})
	for _, quiero := range []string{"Viaje · Se borra en 30 días", "Legal · No se borra automáticamente"} {
		if !strings.Contains(textos, quiero) {
			t.Errorf("la papelera no dice %q: %s", quiero, textos)
		}
	}
}

func TestCaducidadDePapelera(t *testing.T) {
	ahora := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	en := func(d time.Duration) *time.Time { x := ahora.Add(d); return &x }
	for _, c := range []struct {
		seBorra *time.Time
		clave   string
		dias    int
	}{
		{nil, "trash.kept", 0},
		{en(-time.Minute), "trash.purgeSoon", 0},
		{en(3 * time.Hour), "trash.purgeIn", 1},
		{en(30*24*time.Hour - time.Minute), "trash.purgeIn", 30},
	} {
		if clave, dias := caducidadPapelera(c.seBorra, ahora); clave != c.clave || dias != c.dias {
			t.Errorf("caducidadPapelera(%v) = %s, %d; quería %s, %d", c.seBorra, clave, dias, c.clave, c.dias)
		}
	}
}
