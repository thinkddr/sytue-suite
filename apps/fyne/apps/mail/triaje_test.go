// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioConTodas es el de ejemplo con las seis carpetas del sistema: el triaje
// depende de cuáles hay y de en cuál se está.
func servicioConTodas() *servicioFalso {
	s := servicioDeEjemplo()
	s.carpetas = append(s.carpetas,
		api.Mailbox{ID: "jk", Name: "Junk", Role: "junk"},
		api.Mailbox{ID: "dr", Name: "Drafts", Role: "drafts"},
	)
	return s
}

// buzonConAvisos es buzonDePrueba con la pila de avisos vaciada al acabar: un aviso de
// éxito deja un reloj de 5 s vivo, y ese reloj no puede sobrevivir a su test.
func buzonConAvisos(t *testing.T, s Servicio) *Buzon {
	t.Helper()
	b, _ := buzonDePrueba(t, s)
	t.Cleanup(func() {
		for _, a := range b.P.Avisos.Avisos() {
			b.P.Avisos.Descartar(a)
		}
	})
	return b
}

func carpetaPorRol(b *Buzon, rol string) Carpeta {
	c, _ := b.carpetaDeRol(rol)
	return c
}

// TestElTriajeEsElDeLaVista: las reglas de `onArchive` … `moveTargets` de MailApp.tsx,
// carpeta por carpeta. Un botón de más es una acción que no hace nada («Mover a la
// papelera» estando en ella); uno de menos, una que no se puede hacer.
func TestElTriajeEsElDeLaVista(t *testing.T) {
	b := buzonConAvisos(t, servicioConTodas())
	b.Arrancar(context.Background())
	a := b.P.Acciones

	// Recibidos: archivar, papelera y spam; ni «no es spam» ni borrado definitivo.
	if a.Archivar == nil || a.Papelera == nil || a.Spam == nil || a.NoSpam != nil || a.Purgar != nil {
		t.Errorf("en Recibidos: archivar %v, papelera %v, spam %v, no spam %v, purgar %v",
			a.Archivar != nil, a.Papelera != nil, a.Spam != nil, a.NoSpam != nil, a.Purgar != nil)
	}
	destinos := make([]string, 0, len(a.Destinos))
	for _, d := range a.Destinos {
		destinos = append(destinos, d.ID)
	}
	if slices.Contains(destinos, "in") || slices.Contains(destinos, "dr") || len(destinos) != 4 {
		t.Errorf("«Mover a…» ofrece %v: ni la carpeta actual ni Borradores", destinos)
	}

	b.ElegirCarpeta(carpetaPorRol(b, "trash"))
	if a.Papelera != nil || a.Purgar == nil || a.NoSpam != nil {
		t.Errorf("en la Papelera: papelera %v, purgar %v, no spam %v", a.Papelera != nil, a.Purgar != nil, a.NoSpam != nil)
	}
	b.ElegirCarpeta(carpetaPorRol(b, "junk"))
	if a.Spam != nil || a.NoSpam == nil || a.Purgar == nil {
		t.Errorf("en Spam: spam %v, no spam %v, purgar %v", a.Spam != nil, a.NoSpam != nil, a.Purgar != nil)
	}
	// Con una etiqueta no se está EN ninguna carpeta: se ofrecen todas.
	b.ElegirEtiqueta(Etiqueta{ID: "e1", Nombre: "Obra"})
	if a.Papelera == nil || a.Spam == nil || a.Purgar != nil || len(a.Destinos) != 5 {
		t.Errorf("con una etiqueta: papelera %v, spam %v, purgar %v, %d destinos",
			a.Papelera != nil, a.Spam != nil, a.Purgar != nil, len(a.Destinos))
	}
}

// TestArchivarMueveElHiloEnteroEnUnaLlamada: el hilo sale de la lista al pulsar, sus
// DOS mensajes van en una sola petición, y el número del lateral se vuelve a pedir.
func TestArchivarMueveElHiloEnteroEnUnaLlamada(t *testing.T) {
	s := servicioConTodas()
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	antes := s.pidio("Mailboxes")

	h1 := b.P.Bandeja.Conversaciones[0]
	b.P.Acciones.Archivar(h1)

	if !slices.Contains(s.llamadas, "MoveMany(ar,m1,m3)") || s.pidio("MoveMany") != 1 {
		t.Errorf("archivar: %v; tenía que ser un MoveMany con los dos mensajes del hilo", s.llamadas)
	}
	for _, c := range b.P.Bandeja.Conversaciones {
		if c.HiloID == "h1" {
			t.Fatal("el hilo archivado sigue en Recibidos")
		}
	}
	if s.pidio("Mailboxes") != antes+1 {
		t.Error("tras mover no se han vuelto a pedir las carpetas")
	}
}

// TestUnTriajeRechazadoVuelveYAvisa: si el servidor dice que no, la lista vuelve a
// como estaba y aparece un aviso de peligro con el nombre de la carpeta.
//
// VISTO EN ROJO (2026-09-22): sin restaurar `b.mensajes` en el error, el hilo
// desaparece para siempre de la vista aunque siga en Recibidos.
func TestUnTriajeRechazadoVuelveYAvisa(t *testing.T) {
	s := servicioConTodas()
	s.errTriaje = &api.Error{Status: http.StatusBadGateway}
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())

	b.P.Acciones.Papelera(b.P.Bandeja.Conversaciones[0])
	if len(b.P.Bandeja.Conversaciones) != 2 {
		t.Errorf("tras el rechazo hay %d conversaciones y eran 2", len(b.P.Bandeja.Conversaciones))
	}
	avisos := b.P.Avisos.Avisos()
	if len(avisos) != 1 || avisos[0].Tono != ui.AvisoPeligro || !strings.Contains(avisos[0].Titulo, "Papelera") {
		t.Fatalf("avisos: %+v", avisos)
	}

	// Y destacar: optimista, y de vuelta al rechazo.
	h2 := b.P.Bandeja.Conversaciones[1]
	b.P.Acciones.Destacar(h2)
	if b.P.Bandeja.Conversaciones[1].Destacada {
		t.Error("el destacado rechazado se ha quedado puesto")
	}
}

// TestQuitarLaEtiquetaQueSeMiraSacaElHilo: en la vista de una etiqueta, quitársela a
// un hilo lo saca de la lista, y la petición lleva todos sus mensajes.
func TestQuitarLaEtiquetaQueSeMiraSacaElHilo(t *testing.T) {
	s := servicioConTodas()
	etiquetado := sobre("m9", "h9", "in", "Obra nueva", true, 0)
	etiquetado.Keywords = append(etiquetado.Keywords, "e1")
	s.porEtiqueta = []api.MailMessage{etiquetado}
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	b.ElegirEtiqueta(Etiqueta{ID: "e1", Nombre: "Obra"})
	if len(b.P.Bandeja.Conversaciones) != 1 {
		t.Fatalf("la etiqueta trae %d conversaciones", len(b.P.Bandeja.Conversaciones))
	}
	c := b.P.Bandeja.Conversaciones[0]
	if got := EtiquetasDe(c, b.P.Bandeja.Etiquetas); len(got) != 1 || got[0].ID != "e1" {
		t.Fatalf("la conversación lleva los chips %+v", got)
	}
	b.P.Acciones.Etiquetar(c, "e1", false)
	if len(b.P.Bandeja.Conversaciones) != 0 {
		t.Error("quitar la etiqueta que se mira no saca el hilo de la lista")
	}
	if !slices.Contains(s.llamadas, "ApplyLabel(e1,false,m9)") {
		t.Errorf("etiquetar: %v", s.llamadas)
	}
}

// TestPurgarPreguntaYBorraUnoAUno: en la Papelera, el borrado definitivo abre la
// confirmación y, confirmado, borra cada mensaje; un 404 cuenta como hecho.
func TestPurgarPreguntaYBorraUnoAUno(t *testing.T) {
	s := servicioConTodas()
	s.paginas[""] = []api.MailMessage{
		sobre("t1", "ht", "tr", "Viejo", true, 0),
		sobre("t2", "ht", "tr", "Viejo", true, time.Hour),
	}
	b := buzonConAvisos(t, s)
	w := test.NewWindow(b.P.Objeto())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 744))
	b.Arrancar(context.Background())
	b.ElegirCarpeta(carpetaPorRol(b, "trash"))

	b.P.Acciones.Purgar(b.P.Bandeja.Conversaciones[0])
	d := dialogoAbierto(t, w.Canvas())
	if s.pidio("Delete") != 0 {
		t.Fatal("se ha borrado sin confirmar")
	}
	botonConfirmar(d).Tapped(nil)
	if s.pidio("Delete") != 2 {
		t.Errorf("borrados: %v; tenían que ser los dos mensajes del hilo", s.llamadas)
	}
	if av := b.P.Avisos.Avisos(); len(av) != 1 || av[0].Tono != ui.AvisoExito {
		t.Errorf("tras borrar, los avisos son %+v", av)
	}
}

// TestPurgarCuentaUn404ComoHecho: el mensaje ya no está, que es lo que se pedía.
func TestPurgarCuentaUn404ComoHecho(t *testing.T) {
	s := servicioConTodas()
	s.errTriaje = &api.Error{Status: http.StatusNotFound}
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	b.purgar(b.P.Bandeja.Conversaciones[:1], nil, nil)
	if av := b.P.Avisos.Avisos(); len(av) != 1 || av[0].Tono != ui.AvisoExito {
		t.Errorf("un 404 al borrar se ha contado como fallo: %+v", av)
	}
	s.errTriaje = errors.New("sin red")
	b.purgar(b.P.Bandeja.Conversaciones[:1], nil, nil)
	if av := b.P.Avisos.Avisos(); len(av) != 2 || av[1].Tono != ui.AvisoPeligro {
		t.Errorf("un fallo de verdad no avisa como fallo: %+v", av)
	}
}

// TestLaBajaSoloSeOfreceSiElServidorLaDa: el botón aparece con la respuesta del
// servidor, y al darse de baja se va.
func TestLaBajaSoloSeOfreceSiElServidorLaDa(t *testing.T) {
	s := servicioConTodas()
	s.baja = api.MailBaja{OneClick: true}
	b := buzonConAvisos(t, s)
	b.Arrancar(context.Background())
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	if !b.P.Lector.Baja {
		t.Fatal("el servidor ofrece baja y el lector no la enseña")
	}
	b.darseDeBaja(b.P.Lector.Conversacion.Mensajes[0])
	if b.P.Lector.Baja || s.pidio("DoUnsubscribe") != 1 {
		t.Errorf("tras darse de baja: botón %v, llamadas %v", b.P.Lector.Baja, s.llamadas)
	}
}

// dialogoAbierto saca el diálogo de las superposiciones del lienzo.
func dialogoAbierto(t *testing.T, c fyne.Canvas) *ui.Dialogo {
	t.Helper()
	var d *ui.Dialogo
	var mirar func(fyne.CanvasObject)
	mirar = func(o fyne.CanvasObject) {
		if d != nil {
			return
		}
		if x, ok := o.(*ui.Dialogo); ok {
			d = x
			return
		}
		for _, h := range hijos(o) {
			mirar(h)
		}
	}
	for _, o := range c.Overlays().List() {
		mirar(o)
	}
	if d == nil {
		t.Fatal("no hay ningún diálogo abierto")
	}
	return d
}

type hubFalso struct {
	tipo   string
	params map[string]string
}

func (h *hubFalso) ResolveIntent(_ context.Context, tipo string, params map[string]string) (api.HubIntent, error) {
	h.tipo, h.params = tipo, params
	return api.HubIntent{URL: "https://calendar.sytue.eu/?action=create", Target: "calendar"}, nil
}

type abridorFalso struct{ abiertas []string }

func (a *abridorFalso) OpenURL(u *url.URL) error {
	a.abiertas = append(a.abiertas, u.String())
	return nil
}

// TestCrearEventoPideElIntentYAbreCalendar: los invitados son remitente y
// destinatarios sin repetir, y el enlace se abre en SU app (el esquema de Calendar).
func TestCrearEventoPideElIntentYAbreCalendar(t *testing.T) {
	s := servicioConTodas()
	b := buzonConAvisos(t, s)
	h, a := &hubFalso{}, &abridorFalso{}
	b.ConHub(h, a)
	b.Arrancar(context.Background())
	b.Abrir(b.P.Bandeja.Conversaciones[0])
	if b.P.Lector.AlCrearEvento == nil {
		t.Fatal("con Hub no se ofrece «Crear evento»")
	}
	b.P.Lector.AlCrearEvento(b.P.Lector.Conversacion.Mensajes[0])
	if h.tipo != "create-event-from-mail" || h.params["attendees"] != "m1@example.org,ada@example.org" {
		t.Errorf("intent %q con %v", h.tipo, h.params)
	}
	if len(a.abiertas) != 1 || !strings.HasPrefix(a.abiertas[0], "sytue-calendar:") {
		t.Errorf("abiertas: %v; tenía que abrirse Sytue Calendar por su esquema", a.abiertas)
	}
}

// TestElGestorDeEtiquetasCreaRenombraYBorra: una nueva con su color, un nombre repetido
// no se crea, y renombrar manda nombre y color juntos.
func TestElGestorDeEtiquetasCreaRenombraYBorra(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 800))
	b.verEtiquetas()
	if !hayDialogo(w.Canvas(), traductor(t).T("labels.title", nil)) {
		t.Fatal("no se abre «Etiquetas»")
	}
	g := &gestorEtiquetas{b: b, nombre: ui.NuevaEntrada(""), color: "teal", caja: container.New(ui.Columna{})}
	g.lista = []api.MailLabel{{ID: "e1", Name: "Obra", Color: "teal"}}
	g.nombre.PonerTexto("obra")
	if !g.duplicada() {
		t.Error("«obra» y «Obra» son la misma etiqueta")
	}
	g.crear()
	if s.pidio("CreateLabel") != 0 {
		t.Error("se ha creado una etiqueta repetida")
	}
	g.nombre.PonerTexto("Facturas")
	g.crear()
	if !slices.Contains(s.llamadas, "CreateLabel(Facturas,teal)") {
		t.Errorf("crear: %v", s.llamadas)
	}
	g.renombrar(g.lista[0], "Obra nueva")
	if !slices.Contains(s.llamadas, "UpdateLabel(e1,Obra nueva,teal)") {
		t.Errorf("renombrar: %v", s.llamadas)
	}
	g.borrar(g.lista[0])
	if !slices.Contains(s.llamadas, "DeleteLabel(e1)") {
		t.Errorf("borrar: %v", s.llamadas)
	}
}
