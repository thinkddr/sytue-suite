// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// contactosFalsos contesta con lo que tenga para cada búsqueda.
type contactosFalsos struct {
	por      map[string][]api.Contact
	buscados []string
}

func (c *contactosFalsos) Search(_ context.Context, q string, limite int) ([]api.Contact, error) {
	c.buscados = append(c.buscados, q)
	if limite != MaxSugerencias {
		return nil, nil
	}
	return c.por[q], nil
}

// esperasDeSugerencias aparta las esperas de 200 ms del autocompletado: las demás (el
// autoguardado) no son de estas pruebas.
func esperasDeSugerencias(b *Buzon) *[]func() {
	var xs []func()
	b.Esperar = func(d time.Duration, f func()) func() {
		if d == EsperaSugerencias {
			xs = append(xs, f)
		}
		return func() {}
	}
	return &xs
}

// TestLasSugerenciasSonLasDelBFF: una por dirección, la principal primero, sin
// repetir sin mayúsculas y ocho como mucho.
func TestLasSugerenciasSonLasDelBFF(t *testing.T) {
	cs := make([]api.Contact, 0, 12)
	cs = append(cs,
		api.Contact{FormattedName: " Grace Hopper ", Emails: []api.ContactEmail{{Email: "g@casa.org"}, {Email: "grace@example.org", IsPrimary: true}}},
		api.Contact{FormattedName: "Otra Grace", Emails: []api.ContactEmail{{Email: "GRACE@example.org"}, {Email: " "}}},
	)
	for i := range 10 {
		cs = append(cs, api.Contact{Emails: []api.ContactEmail{{Email: "x" + strconvInt(i) + "@example.org"}}})
	}
	xs := SugerenciasDe(cs)
	if len(xs) != MaxSugerencias {
		t.Fatalf("salen %d y el tope es %d", len(xs), MaxSugerencias)
	}
	if xs[0] != (Sugerencia{"Grace Hopper", "grace@example.org"}) || xs[1].Correo != "g@casa.org" || xs[2].Correo != "x0@example.org" {
		t.Errorf("orden o repetidas: %+v", xs[:3])
	}
	if xs[0].Ficha() != "Grace Hopper <grace@example.org>" || xs[2].Ficha() != "x0@example.org" {
		t.Errorf("fichas: %q %q", xs[0].Ficha(), xs[2].Ficha())
	}
}

// TestParaSugiereContactosYSeEligenConElTeclado: con dos letras y a los 200 ms se
// pregunta; una respuesta vieja no pisa a la nueva; ↓ e Intro eligen; Escape cierra;
// y pulsar una fila la elige sin esperar al clic entero.
func TestParaSugiereContactosYSeEligenConElTeclado(t *testing.T) {
	grace := []api.Contact{{FormattedName: "Grace Hopper", Emails: []api.ContactEmail{{Email: "grace@example.org"}}}}
	gr := append(slices.Clone(grace), api.Contact{Emails: []api.ContactEmail{{Email: "grupo@example.org"}}})
	cf := &contactosFalsos{por: map[string][]api.Contact{"gr": gr, "gra": grace}}
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	pendientes := esperasDeSugerencias(b)
	b.ConContactos(cf)
	b.Redactar()
	rd := b.redaccion
	campo := rd.r.para
	w.Canvas().Focus(campo.editor)

	campo.editor.PonerTexto("g")
	if len(*pendientes) != 0 {
		t.Fatal("con una letra ya se busca")
	}
	campo.editor.PonerTexto("gr")
	campo.editor.PonerTexto("gra")
	if len(*pendientes) != 2 {
		t.Fatalf("esperas: %d", len(*pendientes))
	}
	(*pendientes)[1]() // llega primero la nueva…
	if !campo.SugerenciasAbiertas() || rd.r.sugerencias == nil {
		t.Fatal("no se enseña la lista")
	}
	(*pendientes)[0]() // …y luego la vieja, que se descarta
	if strings.Join(cf.buscados, ",") != "gra,gr" || len(campo.sugerencias) != 1 {
		t.Errorf("buscados %v, sugerencias %+v", cf.buscados, campo.sugerencias)
	}

	campo.tecla(&fyne.KeyEvent{Name: fyne.KeyDown})
	if campo.activa != 0 {
		t.Fatalf("↓ deja la activa en %d", campo.activa)
	}
	campo.tecla(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if rd.r.B.Para != "Grace Hopper <grace@example.org>, " || campo.SugerenciasAbiertas() || rd.r.sugerencias != nil {
		t.Fatalf("elegir con Intro: Para %q, abierta %v", rd.r.B.Para, campo.SugerenciasAbiertas())
	}

	*pendientes = nil
	campo.editor.PonerTexto("gr")
	(*pendientes)[0]()
	campo.tecla(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if campo.SugerenciasAbiertas() {
		t.Error("Escape no cierra la lista")
	}
	*pendientes = nil
	campo.editor.PonerTexto("gra")
	(*pendientes)[0]()
	campo.filasSug[0].MouseDown(&desktop.MouseEvent{})
	if strings.Count(rd.r.B.Para, "grace@example.org") != 2 {
		t.Errorf("pulsar una fila no la elige: %q", rd.r.B.Para)
	}
}

// TestLaCapturaDeSugerenciasSeGuarda (SYTUE_CAPTURAS).
func TestLaCapturaDeSugerenciasSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	cf := &contactosFalsos{por: map[string][]api.Contact{"gr": {
		{FormattedName: "Grace Hopper", Emails: []api.ContactEmail{{Email: "grace@example.org"}}},
		{Emails: []api.ContactEmail{{Email: "grupo-obra@example.org"}}},
	}}}
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	pendientes := esperasDeSugerencias(b)
	b.ConContactos(cf)
	b.Redactar()
	campo := b.redaccion.r.para
	w.Canvas().Focus(campo.editor)
	campo.editor.PonerTexto("gr")
	(*pendientes)[len(*pendientes)-1]()
	campo.senalar(0)
	guardar(t, filepath.Join(dir, "sugerencias.png"), w.Canvas().Capture())
}
