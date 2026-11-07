// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestElEstadoDelInvitadoEsGuestStatus: lo que no es sí, quizá o no es «a la espera»,
// y la lista va como `sortGuests` —quien va, quizá, a la espera y no—, estable.
func TestElEstadoDelInvitadoEsGuestStatus(t *testing.T) {
	for partstat, quiero := range map[string]string{
		"ACCEPTED": "accepted", "tentative": "tentative", "Declined": "declined",
		"NEEDS-ACTION": "pending", "DELEGATED": "pending", "": "pending",
	} {
		if got := estadoInvitado(partstat); got != quiero {
			t.Errorf("estadoInvitado(%q) = %q, quiero %q", partstat, got, quiero)
		}
	}
	l := invitadosOrdenados([]api.Attendee{
		{Email: "no@x", PartStat: "DECLINED"},
		{Email: "espera1@x"},
		{Email: "si@x", PartStat: "ACCEPTED"},
		{Email: "quiza@x", PartStat: "TENTATIVE"},
		{Email: "espera2@x", PartStat: "NEEDS-ACTION"},
	})
	orden := make([]string, 0, len(l))
	for _, a := range l {
		orden = append(orden, a.Email)
	}
	if quiero := []string{"si@x", "quiza@x", "espera1@x", "espera2@x", "no@x"}; !slices.Equal(orden, quiero) {
		t.Errorf("el orden es %v y quiero %v", orden, quiero)
	}
}

// reunion es la ocurrencia e1 con organizador y tres invitados que han contestado
// distinto.
func reunion(t *testing.T, a *App) Ocurrencia {
	t.Helper()
	o := ocurrenciaDe(t, a, "e1")
	o.Event.Organizer = "mailto:ada@sytue.eu"
	o.Event.Attendees = []api.Attendee{
		{Email: "yo@sytue.eu", PartStat: "NEEDS-ACTION"},
		{Email: "carla@sytue.eu", PartStat: "DECLINED"},
		{Email: "bob@sytue.eu", CommonName: "Bob", PartStat: "ACCEPTED"},
	}
	return o
}

func buscarParrafo(o fyne.CanvasObject, texto string) *ui.Parrafo {
	x := buscarObjeto(o, func(c fyne.CanvasObject) bool {
		p, ok := c.(*ui.Parrafo)
		return ok && p.Texto == texto
	})
	p, _ := x.(*ui.Parrafo)
	return p
}

func insignias(o fyne.CanvasObject) []string {
	var out []string
	buscarObjeto(o, func(c fyne.CanvasObject) bool {
		if i, ok := c.(*ui.Insignia); ok {
			out = append(out, i.Texto)
		}
		return false // recorrer el árbol entero
	})
	return out
}

// TestLaVistaRapidaEnsenaQuienOrganizaYQuienVa es GuestList.tsx en el panel: el
// organizador sin «mailto:», el rótulo «Personas invitadas» y cada invitado —por su nombre si lo
// trae— con la insignia de su respuesta, en el orden de `sortGuests`.
func TestLaVistaRapidaEnsenaQuienOrganizaYQuienVa(t *testing.T) {
	a, w := appConVentana(t, &escrituras{})
	ancla := container.NewWithoutLayout()
	ancla.Move(fyne.NewPos(700, 200))
	ancla.Resize(fyne.NewSize(140, 18))
	a.raiz.Add(container.NewWithoutLayout(ancla))
	a.abrirVistaRapida(reunion(t, a), ancla)
	guardar(t, w.Canvas().Capture(), "calendario-vista-rapida-invitados")
	for _, s := range []string{"Organizador", "ada@sytue.eu", "Personas invitadas", "Bob", "yo@sytue.eu", "carla@sytue.eu"} {
		if buscarParrafo(a.vista, s) == nil {
			t.Errorf("la vista rápida no enseña %q", s)
		}
	}
	if got, quiero := insignias(a.vista), []string{"Asistirá", "Sin responder", "No asistirá"}; !slices.Equal(got, quiero) {
		t.Errorf("las respuestas son %v y quiero %v", got, quiero)
	}

	// Un evento sin organizador ni invitados no gana una sección vacía.
	o := ocurrenciaDe(t, a, "s1")
	a.abrirVistaRapida(o, ancla)
	if buscarParrafo(a.vista, "Personas invitadas") != nil || buscarParrafo(a.vista, "Organizador") != nil {
		t.Error("un evento personal enseña la sección de invitados")
	}
}

// TestLasPildorasDelFormularioDicenLaRespuesta: al editar, cada invitado que ya
// contestó lleva su insignia y el organizador sale sobre las píldoras; el añadido ahora
// no tiene respuesta y no la enseña.
func TestLasPildorasDelFormularioDicenLaRespuesta(t *testing.T) {
	a, _ := appConVentana(t, &escrituras{})
	e := a.P.EdicionDe(reunion(t, a))
	e.Invitados = append(e.Invitados, "nuevo@sytue.eu")
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	bloque := f.bloqueInvitados()
	if got, quiero := insignias(bloque), []string{"Sin responder", "No asistirá", "Asistirá"}; !slices.Equal(got, quiero) {
		t.Errorf("las píldoras dicen %v y quiero %v", got, quiero)
	}
	if buscarParrafo(bloque, "Organizador") == nil || buscarParrafo(bloque, "ada@sytue.eu") == nil {
		t.Error("el formulario no dice quién organiza")
	}
}

// TestUnInvitadoSinPartstatSaleSinResponder: un ATTENDEE externo sin PARTSTAT ya
// estaba en el evento, así que su píldora dice «Sin responder», como en la web; el
// añadido ahora sigue sin insignia.
func TestUnInvitadoSinPartstatSaleSinResponder(t *testing.T) {
	a, _ := appConVentana(t, &escrituras{})
	o := reunion(t, a)
	o.Event.Attendees = []api.Attendee{{Email: "externo@example.com"}}
	e := a.P.EdicionDe(o)
	e.Invitados = append(e.Invitados, "nuevo@sytue.eu")
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	if got, quiero := insignias(f.bloqueInvitados()), []string{"Sin responder"}; !slices.Equal(got, quiero) {
		t.Errorf("las píldoras dicen %v y quiero %v", got, quiero)
	}
}
