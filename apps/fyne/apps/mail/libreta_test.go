// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"

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

// fichasFalsas es Contacts con su libreta por defecto: busca por subcadena, como el
// servicio, y apunta lo que se crea.
type fichasFalsas struct {
	contactosFalsos
	cs      []api.Contact
	creados []api.ContactInput
	libreta string
}

func (f *fichasFalsas) Search(_ context.Context, q string, _ int) ([]api.Contact, error) {
	f.buscados = append(f.buscados, q)
	var out []api.Contact
	for _, c := range f.cs {
		for _, e := range c.Emails {
			if strings.Contains(strings.ToLower(e.Email), strings.ToLower(q)) {
				out = append(out, c)
				break
			}
		}
	}
	return out, nil
}

func (f *fichasFalsas) AddressBooks(context.Context) ([]api.AddressBook, error) {
	return []api.AddressBook{{ID: "otra"}, {ID: "porDefecto", IsDefault: true}}, nil
}

func (f *fichasFalsas) Create(_ context.Context, libreta string, in api.ContactInput) (api.Contact, error) {
	f.libreta = libreta
	f.creados = append(f.creados, in)
	c := api.Contact{ID: "0b6f6c2e-6a39-4bd2-9a57-4f2d3c9f5a10", FormattedName: in.FormattedName, Emails: in.Emails}
	f.cs = append(f.cs, c)
	return c, nil
}

// elementoDelMenu es la entrada de «Más acciones» con ese texto, o nil.
func elementoDelMenu(b *Buzon, texto string) *ui.ElementoMenu {
	for _, o := range b.P.Lector.menuMas().Elementos {
		if e, ok := o.(*ui.ElementoMenu); ok && e.Texto == texto {
			return e
		}
	}
	return nil
}

// TestAnadirALaLibretaDesdeElRemitenteCreaUnaSolaFicha: «Más acciones» ofrece «Añadir a
// Contactos»; pulsarlo crea la ficha en la libreta por defecto con nombre y correo,
// avisa con «Ver» (que abre la ficha por `sytue-contacts:`), y en la segunda vuelta la
// entrada ya es «Ver en Contactos» y no se crea nada más.
//
// VISTO EN ROJO (2026-11-13): sin la entrada en `menuMas` no se ofrece. Deshecho.
func TestAnadirALaLibretaDesdeElRemitenteCreaUnaSolaFicha(t *testing.T) {
	s := servicioConTodas()
	b := buzonConAvisos(t, s)
	ab := &abridorFalso{}
	b.Abridor = ab
	f := &fichasFalsas{cs: []api.Contact{{ID: "otro", Emails: []api.ContactEmail{{Email: "xm1@example.org"}}}}}
	b.ConContactos(f)
	b.Arrancar(context.Background())
	b.Abrir(b.P.Bandeja.Conversaciones[0])

	if elementoDelMenu(b, "Ver en Contactos") != nil {
		t.Fatal("la ficha de otra dirección que la contiene no es la suya")
	}
	anadir := elementoDelMenu(b, "Añadir a Contactos")
	if anadir == nil {
		t.Fatal("«Más acciones» no ofrece «Añadir a Contactos»")
	}
	anadir.AlPulsar()
	if len(f.creados) != 1 || f.libreta != "porDefecto" || f.creados[0].FormattedName != "Persona m1" ||
		len(f.creados[0].Emails) != 1 || f.creados[0].Emails[0].Email != "m1@example.org" {
		t.Fatalf("alta: libreta %q, %+v", f.libreta, f.creados)
	}
	avisos := b.P.Avisos.Avisos()
	if len(avisos) != 1 || avisos[0].Titulo != "Añadido a Contactos" || avisos[0].RotuloAccion != "Ver" {
		t.Fatalf("aviso: %+v", avisos)
	}
	avisos[0].AlPulsarAccion()
	if len(ab.abiertas) != 1 || !strings.HasPrefix(ab.abiertas[0], "sytue-contacts:") ||
		!strings.HasSuffix(ab.abiertas[0], "/contacts/0b6f6c2e-6a39-4bd2-9a57-4f2d3c9f5a10") {
		t.Fatalf("«Ver» abre %v", ab.abiertas)
	}

	if elementoDelMenu(b, "Añadir a Contactos") != nil {
		t.Error("en la segunda vuelta sigue ofreciendo «Añadir»")
	}
	ver := elementoDelMenu(b, "Ver en Contactos")
	if ver == nil {
		t.Fatal("en la segunda vuelta no ofrece «Ver en Contactos»")
	}
	ver.AlPulsar()
	if len(ab.abiertas) != 2 || len(f.creados) != 1 {
		t.Errorf("abiertas %v, creados %d", ab.abiertas, len(f.creados))
	}
}

// TestLaLibretaYaTieneAlRemitente: si Contacts ya lo tiene, al abrir el hilo se sabe y
// el menú ofrece «Ver»; y si se pulsara «Añadir» antes de saberlo, no se duplica.
func TestLaLibretaYaTieneAlRemitente(t *testing.T) {
	s := servicioConTodas()
	b := buzonConAvisos(t, s)
	f := &fichasFalsas{cs: []api.Contact{{ID: "ya", Emails: []api.ContactEmail{{Email: "M1@example.org"}}}}}
	b.ConContactos(f)
	b.Arrancar(context.Background())
	c := b.P.Bandeja.Conversaciones[0]
	b.Abrir(c)
	if elementoDelMenu(b, "Ver en Contactos") == nil || elementoDelMenu(b, "Añadir a Contactos") != nil {
		t.Fatal("con ficha, el menú no ofrece «Ver en Contactos»")
	}
	b.anadirAContactos("Persona m1", "m1@example.org")
	if len(f.creados) != 0 {
		t.Errorf("duplica la ficha: %+v", f.creados)
	}
}

// TestLaLibretaNoSeOfreceEnSpamNiANoReplyNiAUnoMismo: las reglas de OfrecerContacto, y
// en la carpeta de Spam el menú no lleva la entrada.
func TestLaLibretaNoSeOfreceEnSpamNiANoReplyNiAUnoMismo(t *testing.T) {
	propias := []string{"yo@sytue.eu"}
	if !OfrecerContacto("ana@example.eu", false, propias) || OfrecerContacto("ana@example.eu", true, propias) ||
		OfrecerContacto("YO@sytue.eu", false, propias) || OfrecerContacto("", false, propias) {
		t.Error("spam, uno mismo o vacío")
	}
	sinRespuesta := []string{
		"no-reply@x.eu", "noreply@x.eu", "No_Reply@x.eu", "do-not-reply@x.eu",
		"noreply-avisos@x.eu", "mailer-daemon@x.eu", "bounces+123@x.eu",
	}
	for _, c := range sinRespuesta {
		if !EsSinRespuesta(c) {
			t.Errorf("%s no se reconoce como sin respuesta", c)
		}
	}
	for _, c := range []string{"ana@x.eu", "noelia@x.eu", "replyall@x.eu"} {
		if EsSinRespuesta(c) {
			t.Errorf("%s no es un no-reply", c)
		}
	}

	s := servicioConTodas()
	s.paginas[""] = append(s.paginas[""], sobre("m9", "h9", "jk", "Premio", true, 0))
	b := buzonConAvisos(t, s)
	b.ConContactos(&fichasFalsas{})
	b.Arrancar(context.Background())
	b.ElegirCarpeta(Carpeta{ID: "jk", Rol: "junk"})
	if len(b.P.Bandeja.Conversaciones) == 0 {
		t.Fatal("Spam sin conversaciones")
	}
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	if elementoDelMenu(b, "Añadir a Contactos") != nil || elementoDelMenu(b, "Ver en Contactos") != nil {
		t.Error("en Spam se ofrece guardar al remitente")
	}
}
