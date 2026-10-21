// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type libretaFalsa struct {
	cs      []api.Contact
	err     error
	pedidas int
}

func (l *libretaFalsa) All(_ context.Context, limite, offset int) ([]api.Contact, int, error) {
	l.pedidas++
	if limite != PaginaLibreta || offset != 0 {
		return nil, 0, errors.New("página inesperada")
	}
	return l.cs, 0, l.err
}

func libretaDeEjemplo() *libretaFalsa {
	return &libretaFalsa{cs: []api.Contact{
		{FormattedName: "Óscar Ruiz", Emails: []api.ContactEmail{{Email: "oscar@example.org"}}},
		{FormattedName: "Ada Lovelace", Emails: []api.ContactEmail{{Email: "ada@casa.org"}, {Email: "ada@example.org", IsPrimary: true}}},
		{FormattedName: "Sin correo"},
		{Emails: []api.ContactEmail{{Email: "buzon@example.org"}}},
		{FormattedName: "Pablo Díaz", Emails: []api.ContactEmail{{Email: "pablo@example.org"}}},
	}}
}

// TestLaLibretaEsLaDeLaWeb: sin los contactos sin correo, una fila por correo, en el
// orden del idioma (la Ó va con la O), filtrada por nombre o correo; y lo elegido va
// detrás de lo que había, con su coma.
func TestLaLibretaEsLaDeLaWeb(t *testing.T) {
	entradas := EntradasLibreta(libretaDeEjemplo().cs)
	if len(entradas) != 4 || entradas[1].Correos[0] != "ada@example.org" {
		t.Fatalf("entradas: %+v", entradas)
	}
	filas := FilasLibreta("es", entradas, "")
	orden := make([]string, 0, len(filas))
	for _, f := range filas {
		orden = append(orden, f.Correo)
	}
	if got, want := joinComa(orden), "ada@example.org, ada@casa.org, buzon@example.org, oscar@example.org, pablo@example.org"; got != want {
		t.Errorf("orden: %v", orden)
	}
	if f := FilasLibreta("es", entradas, "CASA"); len(f) != 1 || f[0].Correo != "ada@casa.org" {
		t.Errorf("filtrar por correo: %+v", f)
	}
	if f := FilasLibreta("es", entradas, "lovel"); len(f) != 2 {
		t.Errorf("filtrar por nombre: %+v", f)
	}
	if got := AnadirDeLibreta("grace@example.org,  ", []Sugerencia{{"Ada Lovelace", "ada@example.org"}, {"", "buzon@example.org"}}); got != "grace@example.org, Ada Lovelace <ada@example.org>, buzon@example.org, " {
		t.Errorf("añadir: %q", got)
	}
	if got := AnadirDeLibreta("", []Sugerencia{{"", "b@x.org"}}); got != "b@x.org, " {
		t.Errorf("añadir a un campo vacío: %q", got)
	}
}

// TestElegirDeContactosAnadeAlCampoQueLaAbrio: el icono se ve con libreta; se pide
// una vez por redacción; marcar dos y «Añadir 2» las pone en CC, no en Para.
func TestElegirDeContactosAnadeAlCampoQueLaAbrio(t *testing.T) {
	lf := libretaDeEjemplo()
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.ConLibreta(lf, "https://contacts.example")
	b.Redactar()
	rd := b.redaccion
	if !rd.r.libretas[0].Visible() {
		t.Fatal("con libreta, el icono de Contactos no se ve")
	}
	rd.r.verCc = true
	rd.r.libretas[1].AlPulsar() // el icono de la fila de CC
	g := rd.libretaAbierta
	if g == nil || !hayDialogo(w.Canvas(), traductor(t).T("compose.addressBookTitle", nil)) || rd.libreta == nil {
		t.Fatal("la libreta no se abre cargada")
	}
	if !g.anadir.Inerte {
		t.Error("sin nadie marcado se deja añadir")
	}
	filas := FilasLibreta("es", rd.libreta, "")
	g.alternar(filas[3])
	g.alternar(filas[0])
	if g.anadir.Texto != "Añadir 2" || g.anadir.Inerte {
		t.Errorf("botón: %q inerte %v", g.anadir.Texto, g.anadir.Inerte)
	}
	g.elegir()
	if rd.r.B.Cc != "Óscar Ruiz <oscar@example.org>, Ada Lovelace <ada@example.org>, " || rd.r.B.Para != "" {
		t.Errorf("CC %q, Para %q", rd.r.B.Cc, rd.r.B.Para)
	}
	b.verLibreta(rd, rd.r.para)
	if lf.pedidas != 1 {
		t.Errorf("la libreta se ha pedido %d veces en una redacción", lf.pedidas)
	}
}

// TestSinLibretaNoHayIcono: sin Contacts enganchado, las filas no lo ofrecen.
func TestSinLibretaNoHayIcono(t *testing.T) {
	b, _, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.Redactar()
	for _, i := range b.redaccion.r.libretas {
		if i.Visible() {
			t.Fatal("sin libreta, el icono de Contactos se ve")
		}
	}
}

// TestLaCapturaDeLaLibretaSeGuarda (SYTUE_CAPTURAS).
func TestLaCapturaDeLaLibretaSeGuarda(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	b, w, _ := buzonConVentana(t, servicioConTodas(), fyne.NewSize(1280, 800))
	b.ConLibreta(libretaDeEjemplo(), "https://contacts.example")
	b.Redactar()
	guardar(t, filepath.Join(dir, "redactar-libreta.png"), w.Canvas().Capture())
	g := b.verLibreta(b.redaccion, b.redaccion.r.para)
	g.alternar(FilasLibreta("es", b.redaccion.libreta, "")[0])
	guardar(t, filepath.Join(dir, "libreta.png"), w.Canvas().Capture())
}
