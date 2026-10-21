// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLasRepeticionesSonLasDeLaWeb: cada una vuelve a leerse como la misma, las RRULE
// son las de client/recordatorios.ts y una ajena se respeta.
func TestLasRepeticionesSonLasDeLaWeb(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	cuando := time.Date(2026, 10, 3, 9, 30, 0, 0, madrid)
	for _, rep := range Repeticiones {
		in, ok := EntradaRecordatorio(cuando, false, rep, 3, "")
		if !ok {
			t.Fatalf("%s no vale", rep)
		}
		r := api.NoteReminder{RRule: in.RRule, AfterDays: in.AfterDays}
		if got := RepeticionDe(r); got != rep {
			t.Errorf("%s se lee como %s", rep, got)
		}
		if in.TZID != "Europe/Madrid" || !in.DueAt.Equal(cuando) {
			t.Errorf("%s: zona %q, instante %v", rep, in.TZID, in.DueAt)
		}
	}
	if in, _ := EntradaRecordatorio(cuando, false, "weekdays", 0, ""); in.RRule != "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR" {
		t.Errorf("laborables es %q", in.RRule)
	}
	ajena := "FREQ=YEARLY;BYMONTH=3"
	if RepeticionDe(api.NoteReminder{RRule: ajena}) != "custom" {
		t.Error("una RRULE ajena no se reconoce como propia")
	}
	if in, _ := EntradaRecordatorio(cuando, false, "custom", 0, ajena); in.RRule != ajena {
		t.Errorf("una RRULE ajena se pierde al guardar: %q", in.RRule)
	}
}

// TestFlexibleYTodoElDia: la flexible pide de 1 a 365 días y no lleva RRULE; la de todo
// el día avisa a las 9 de su día.
func TestFlexibleYTodoElDia(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	dia := time.Date(2026, 10, 3, 0, 0, 0, 0, madrid)
	for _, n := range []int{0, 366} {
		if _, ok := EntradaRecordatorio(dia, false, "flexible", n, ""); ok {
			t.Errorf("la flexible de %d días vale", n)
		}
	}
	in, ok := EntradaRecordatorio(dia, true, "flexible", 2, "")
	if !ok || in.RRule != "" || in.AfterDays != 2 || !in.AllDay {
		t.Fatalf("flexible de todo el día: %+v", in)
	}
	if quiero := time.Date(2026, 10, 3, HoraTodoElDia, 0, 0, 0, madrid); !in.DueAt.Equal(quiero) {
		t.Errorf("todo el día avisa a las %v y tocaba %v", in.DueAt.In(madrid), quiero)
	}
}

// TestCompletarUnaTareaQueRepiteTraeLaSiguiente: marcarla hecha guarda la nota y, si el
// servicio dice que creó la siguiente (`repeated_into`), vuelve a pedir las notas y los
// recordatorios, que es donde está. Lo dice el servicio y no mis recordatorios: la que
// repite puede ser por el de otra persona, como aquí «a», que no tiene ninguno mío.
func TestCompletarUnaTareaQueRepiteTraeLaSiguiente(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"PUT /notes/a":   `{"id":"a","notebook_id":"c1","kind":"todo","checked":true,"repeated_into":"a2"}`,
		"PUT /notes/b":   `{"id":"b","notebook_id":"c1","kind":"todo","checked":true}`,
		"GET /reminders": `{"reminders":[]}`,
		"GET /notebooks/c1/notes/page": `{"notes":[{"id":"a","notebook_id":"c1","kind":"todo","checked":true},` +
			`{"id":"a2","notebook_id":"c1","kind":"todo"}]}`,
	}, api.Note{ID: "a", NotebookID: "c1", Kind: "todo"}, api.Note{ID: "b", NotebookID: "c1", Kind: "todo"})
	p.ponerRecordatorios(map[string]api.NoteReminder{"b": {NoteID: "b", RRule: "FREQ=DAILY"}})
	n, _ := p.nota("b")
	p.lienzo.AlMarcar(n, true)
	esperar(t, func() bool { return slices.Contains(falso.rutas(), "PUT /notes/b") })
	n, _ = p.nota("a")
	p.lienzo.AlMarcar(n, true)
	esperar(t, func() bool {
		_, llego := p.nota("a2")
		return llego && slices.Contains(falso.rutas(), "GET /reminders")
	})
	rutas := falso.rutas()
	if !slices.Contains(rutas, "GET /notebooks/c1/notes/page") {
		t.Errorf("completar una que repite no relee las notas: %v", rutas)
	}
	if i := slices.Index(rutas, "GET /notebooks/c1/notes/page"); i < slices.Index(rutas, "PUT /notes/a") {
		t.Errorf("relee antes de guardar: %v", rutas)
	}
	if slices.Index(rutas, "PUT /notes/b") > slices.Index(rutas, "PUT /notes/a") {
		t.Errorf("orden inesperado: %v", rutas)
	}
}

// TestElIndicadorSaleEnLaTarjetaYEnElArbol: con un recordatorio la tarjeta lleva su
// indicador en la cabecera, y sin él no.
func TestElIndicadorSaleEnLaTarjetaYEnElArbol(t *testing.T) {
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "a", NotebookID: "c1", Title: "Regar"}, api.Note{ID: "b", NotebookID: "c1"})
	p.ponerRecordatorios(map[string]api.NoteReminder{"a": {NoteID: "a", DueAt: time.Now().Add(time.Hour), RRule: "FREQ=DAILY"}})
	a, _ := p.nota("a")
	b, _ := p.nota("b")
	if p.lienzo.tarjeta(a).Recordatorio == nil || p.lienzo.tarjeta(b).Recordatorio != nil {
		t.Error("el indicador no va con su nota")
	}
	if p.lienzo.tarjeta(a).AlRecordatorio == nil {
		t.Error("el indicador de la tarjeta no abre el diálogo")
	}
}
