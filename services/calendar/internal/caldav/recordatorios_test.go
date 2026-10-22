// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/calendar/internal/alarm"
)

// conAlarmas mete bloques VALARM en el maestro de `sinRecordatorios`.
func conAlarmas(bloques ...string) string {
	return strings.Replace(sinRecordatorios, "END:VEVENT", strings.Join(bloques, "")+"END:VEVENT", 1)
}

func valarm(lineas ...string) string {
	return "BEGIN:VALARM\r\n" + strings.Join(lineas, "\r\n") + "\r\nEND:VALARM\r\n"
}

// Los mismos casos que `parseReminders` en apps/frontend/apps/calendar/src/lib/ical.test.ts:
// la web y la API tienen que contar los mismos recordatorios en el mismo evento.
func TestRecordatoriosCuentaLoQueLaWebEdita(t *testing.T) {
	data := conAlarmas(
		valarm("ACTION:DISPLAY", "TRIGGER:-PT15M"),
		valarm("ACTION:DISPLAY", "TRIGGER:-P1D"),
		valarm("TRIGGER:-PT2H"),                   // sin ACTION: DISPLAY
		valarm("ACTION:DISPLAY", "TRIGGER:PT0S"),  // en el momento
		valarm("ACTION:EMAIL", "TRIGGER:-PT1H"),   // por correo: se conserva
		valarm("ACTION:DISPLAY", "TRIGGER:PT15M"), // «después»: se conserva
		valarm("ACTION:DISPLAY", "TRIGGER:-P1W"),  // semanas: la web no las edita
		valarm("ACTION:DISPLAY", "TRIGGER;RELATED=END:-PT5M"),
		valarm("ACTION:DISPLAY", "TRIGGER;VALUE=DATE-TIME:20260701T080000Z"),
	)
	if got, want := Recordatorios(data), []int{15, 1440, 120, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Recordatorios = %v, quiero %v", got, want)
	}
	if got := Recordatorios("esto no es iCalendar"); got == nil || len(got) != 0 {
		t.Fatalf("un recurso ilegible debe dar la lista vacía (no nula): %#v", got)
	}
}

// Ida y vuelta: lo que escribe ConRecordatorios se lee igual, es el bloque de `buildAlarm`
// de la web y la cola de avisos lo resuelve al instante que toca.
func TestConRecordatoriosEsElBloqueDeLaWeb(t *testing.T) {
	out, err := ConRecordatorios(sinRecordatorios, []int{15, 60, 1440, 0, 90}, "Standup, con coma")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Recordatorios(out), []int{15, 60, 1440, 0, 90}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ida y vuelta = %v, quiero %v\n%s", got, want, out)
	}
	for _, s := range []string{"TRIGGER:-PT15M", "TRIGGER:-PT1H", "TRIGGER:-P1D", "TRIGGER:-PT0M", "TRIGGER:-PT90M", "ACTION:DISPLAY", `DESCRIPTION:Standup\, con coma`} {
		if !strings.Contains(out, s) {
			t.Fatalf("falta %q en el recurso:\n%s", s, out)
		}
	}
	inicio := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	disparos, err := alarm.Triggers(out, inicio, inicio.Add(time.Hour))
	if err != nil || len(disparos) != 5 || !disparos[1].At.Equal(inicio.Add(-time.Hour)) {
		t.Fatalf("la cola no ve los recordatorios: %v %+v", err, disparos)
	}
	if same, _ := ConRecordatorios(sinRecordatorios, nil, "x"); same != sinRecordatorios {
		t.Fatal("sin recordatorios el recurso tiene que volver tal cual")
	}
}

// Con `propios`, los editables viejos se sustituyen por los nuevos —aunque sean cero— y
// los que la API no sabe expresar se conservan.
func TestCarryOverConPropiosSustituyeLosEditablesYConservaLosAjenos(t *testing.T) {
	correo := valarm("ACTION:EMAIL", "TRIGGER:-PT30M")
	anterior := conAlarmas(valarm("ACTION:DISPLAY", "TRIGGER:-PT15M"), correo)

	nuevo, _ := ConRecordatorios(sinRecordatorios, []int{60}, "x")
	out, err := CarryOver(nuevo, anterior, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := Recordatorios(out); !reflect.DeepEqual(got, []int{60}) {
		t.Fatalf("editables = %v, quiero [60]\n%s", got, out)
	}
	if !strings.Contains(out, "ACTION:EMAIL") {
		t.Fatalf("se ha perdido el aviso por correo que puso otro cliente:\n%s", out)
	}

	// Quitarlos todos: cero editables, y el de correo sigue.
	out, err = CarryOver(sinRecordatorios, anterior, true)
	if err != nil || len(Recordatorios(out)) != 0 || strings.Count(out, "BEGIN:VALARM") != 1 {
		t.Fatalf("quitar los recordatorios: %v\n%s", err, out)
	}

	// Sin `propios` (un cliente que no sabe de recordatorios) no se toca nada.
	out, err = CarryOver(sinRecordatorios, anterior, false)
	if err != nil || strings.Count(out, "BEGIN:VALARM") != 2 {
		t.Fatalf("sin recordatorios en la petición se han perdido: %v\n%s", err, out)
	}
}
