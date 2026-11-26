// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func hueco(desde, hasta string) api.Hueco {
	d, _ := time.Parse("15:04", desde)
	h, _ := time.Parse("15:04", hasta)
	dia := time.Date(2031, 3, 4, 0, 0, 0, 0, time.UTC)
	return api.Hueco{
		Start: dia.Add(time.Duration(d.Hour())*time.Hour + time.Duration(d.Minute())*time.Minute),
		End:   dia.Add(time.Duration(h.Hour())*time.Hour + time.Duration(h.Minute())*time.Minute),
	}
}

// Quitar los ratos ocupados de los invitados deja lo libre de todos en que cabe el
// evento: un hueco se parte, se acorta o desaparece.
func TestRestarOcupados(t *testing.T) {
	libres := []api.Hueco{hueco("09:00", "12:00"), hueco("13:00", "14:00"), hueco("15:00", "17:00")}
	ocupados := []api.Hueco{hueco("10:00", "10:30"), hueco("13:15", "13:45"), hueco("08:00", "09:30"), hueco("16:30", "18:00")}
	got := RestarOcupados(libres, ocupados, 30*time.Minute)
	quiero := []api.Hueco{hueco("09:30", "10:00"), hueco("10:30", "12:00"), hueco("15:00", "16:30")}
	if !slices.Equal(got, quiero) {
		t.Fatalf("%v\nquiero %v", got, quiero)
	}
	if got := RestarOcupados(libres, nil, time.Hour); !slices.Equal(got, libres) {
		t.Fatal("sin ocupados, lo libre se queda")
	}
}

// textosDe son los rótulos de los párrafos y textos de un árbol.
func textosDe(o fyne.CanvasObject) string {
	var b strings.Builder
	buscarObjeto(o, func(x fyne.CanvasObject) bool {
		switch v := x.(type) {
		case *ui.Parrafo:
			b.WriteString(v.Texto + "\n")
		case *ui.FilaPulsable:
			b.WriteString(v.Nombre + "\n")
		}
		return false
	})
	return b.String()
}

// Sytue Office: sin personas, con Invitar, el formulario de un evento de un calendario
// de una CUENTA vuelve a tener invitados —con el autocompletado de la libreta, la pista
// de por dónde sale la invitación y la ocupación de los invitados en «buscar un hueco»—,
// y el de un calendario de este equipo dice dónde se puede invitar. Guardar sigue sin
// pedir al servicio que mande nada (lo manda la sincronización de la cuenta), y lo
// añadido en otro calendario no se guarda en uno de este equipo.
func TestInvitarDesdeCalendarioDeCuenta(t *testing.T) {
	a, _ := appConVentana(t, &escrituras{})
	a.P.SinPersonas = true
	var pedidas []string
	var preguntados []string
	a.P.Invitar = &Invitar{
		En:       func(cal string) bool { return cal == "cuenta" },
		Pista:    func(cal string) string { return "Sale por el servidor de " + cal },
		PistaSin: "Para invitar, elige un calendario de una cuenta",
		Sugerir: func(_ context.Context, q string, n int) ([]SugerenciaInvitado, error) {
			pedidas = append(pedidas, q)
			return []SugerenciaInvitado{{Nombre: "Luis Pérez", Correo: "luis@example.org"}, {Correo: "lucia@example.org"}}, nil
		},
		Ocupacion: func(_ context.Context, cal string, correos []string, desde, hasta time.Time) (map[string][]api.Hueco, error) {
			preguntados = append(preguntados, cal+":"+strings.Join(correos, ","))
			return map[string][]api.Hueco{"luis@example.org": {{Start: time.Date(2031, 3, 4, 7, 30, 0, 0, time.UTC), End: time.Date(2031, 3, 4, 8, 0, 0, 0, time.UTC)}}}, nil
		},
		ResumenOcupacion: func(con, sin []string) string {
			return "con " + strings.Join(con, ",") + "; sin " + strings.Join(sin, ",")
		},
	}
	destinos := []api.Calendar{{ID: "local", DisplayName: "Personal", IsDefault: true}, {ID: "cuenta", DisplayName: "Trabajo (iCloud)"}}
	inicio := time.Date(2031, 3, 4, 7, 0, 0, 0, time.UTC)
	e := Edicion{Inicio: inicio, Fin: inicio.Add(30 * time.Minute), Zona: "UTC"}
	f := a.P.nuevoFormulario(e, destinos, a.accionesFormulario(e))

	// En el de este equipo: sin campo, con la frase.
	if f.invitaAqui() || !strings.Contains(textosDe(f.cuerpo), "Para invitar, elige un calendario de una cuenta") || enArbol(f.cuerpo, f.correo) {
		t.Fatal("en un calendario de este equipo no se invita y se dice dónde")
	}
	// Se elige el de la cuenta: el campo, con su pista.
	f.calendario.AlElegir(1)
	if !f.invitaAqui() || !enArbol(f.cuerpo, f.correo) || !strings.Contains(textosDe(f.cuerpo), "Sale por el servidor de cuenta") {
		t.Fatalf("en el de la cuenta sale el campo con su pista: %q", textosDe(f.cuerpo))
	}

	// El autocompletado: con dos letras, la libreta; las flechas señalan e Intro elige.
	f.correo.PonerTexto("l")
	if len(pedidas) != 0 || len(f.sugerencias) != 0 {
		t.Fatal("con una letra no se pregunta")
	}
	f.correo.PonerTexto("lu")
	if len(f.sugerencias) != 2 || !strings.Contains(textosDe(f.cuerpo), "Luis Pérez <luis@example.org>") {
		t.Fatalf("sugerencias: %+v", f.sugerencias)
	}
	f.correo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
	f.correo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyUp})
	f.correo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !slices.Equal(f.e.Invitados, []string{"luis@example.org"}) || f.correo.Texto() != "" || len(f.sugerencias) != 0 {
		t.Fatalf("Intro elige la señalada: %v %q", f.e.Invitados, f.correo.Texto())
	}
	// Quien ya está invitado no se vuelve a sugerir; una dirección entera se añade tal cual.
	f.correo.PonerTexto("lu")
	if len(f.sugerencias) != 1 || f.sugerencias[0].Correo != "lucia@example.org" {
		t.Fatalf("sin repetir: %+v", f.sugerencias)
	}
	f.correo.PonerTexto("nadie@example.org")
	f.correo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !slices.Equal(f.e.Invitados, []string{"luis@example.org", "nadie@example.org"}) {
		t.Fatalf("a mano: %v", f.e.Invitados)
	}

	// «Buscar un hueco» cuenta con la ocupación de los invitados que la cuenta sabe.
	f.construirHuecos()
	f.alternarHuecos()
	if !slices.Equal(preguntados, []string{"cuenta:luis@example.org,nadie@example.org"}) {
		t.Fatalf("ocupación preguntada: %v", preguntados)
	}
	if !slices.Equal(f.bh.con, []string{"luis@example.org"}) || !slices.Equal(f.bh.sin, []string{"nadie@example.org"}) {
		t.Fatalf("con y sin: %v %v", f.bh.con, f.bh.sin)
	}
	// El primer hueco (07:00–09:00) se parte por lo que Luis tiene ocupado (07:30–08:00).
	if len(f.bh.lista) != 3 || !f.bh.lista[0].End.Equal(time.Date(2031, 3, 4, 7, 30, 0, 0, time.UTC)) ||
		!f.bh.lista[1].Start.Equal(time.Date(2031, 3, 4, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("huecos: %v", f.bh.lista)
	}
	if !strings.Contains(textosDe(f.panelHuecos()), "con luis@example.org; sin nadie@example.org") {
		t.Fatalf("panel: %q", textosDe(f.panelHuecos()))
	}

	// Guardar: los invitados viajan, pero no se pide al servicio que mande nada.
	in := f.e.EntradaDe(a.P)
	if len(in.Attendees) != 2 || in.Send || in.Attendees[0].PartStat != "NEEDS-ACTION" {
		t.Fatalf("entrada: %+v", in)
	}
	// Vuelta al de este equipo antes de guardar: lo añadido no se queda.
	var guardada Edicion
	f.a.Guardar = func(e Edicion, listo func(bool)) { guardada = e; listo(false) }
	f.calendario.AlElegir(0)
	f.titulo.PonerTexto("Comida")
	f.enviar()
	if guardada.CalendarioID != "local" || len(guardada.Invitados) != 0 {
		t.Fatalf("guardada en el de este equipo: %+v", guardada)
	}

	// La conectada no cambia aunque tenga Invitar: invitados siempre, sin libreta ni pista.
	b := appDePrueba(t, &servicioFalso{})
	b.P.Invitar = a.P.Invitar
	fb := b.P.nuevoFormulario(e, destinos, b.accionesFormulario(e))
	if !fb.invitaAqui() || fb.a.Sugerir != nil || fb.a.Ocupacion != nil || fb.pistaInvitar() != "" || fb.pistaSinInvitar() != "" {
		t.Fatal("la edición conectada no mira Invitar")
	}
}

// enArbol dice si o está dentro de raiz.
func enArbol(raiz, o fyne.CanvasObject) bool {
	hay := false
	buscarObjeto(raiz, func(x fyne.CanvasObject) bool {
		if x == o {
			hay = true
		}
		return hay
	})
	return hay
}
