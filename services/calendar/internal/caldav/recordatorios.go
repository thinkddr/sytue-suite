// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package caldav

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/emersion/go-ical"
)

// Los recordatorios que la API REST sabe expresar son los que la web deja editar
// (`parseReminders` de apps/frontend/apps/calendar/src/lib/ical.ts): un VALARM DISPLAY
// —o sin ACTION, que es DISPLAY— cuyo TRIGGER es una duración NEGATIVA (o cero)
// relativa al inicio, en minutos. El resto —un aviso por correo, un disparo a una hora
// fija, uno relativo al FINAL o uno «después»— lo puso otro cliente CalDAV y se
// conserva tal cual: la API ni lo enseña ni lo pisa.

// duracionAntes es la misma expresión que `durationToMinutesBefore` de la web: sin
// semanas (`-P1W` no es editable allí y tampoco aquí, para que las dos cuenten igual).
var duracionAntes = regexp.MustCompile(`^(-)?P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

// minutosEditables son los minutos ANTES del inicio de un VALARM editable; ok=false si
// no lo es y se conserva tal cual.
func minutosEditables(alarma *ical.Component) (int, bool) {
	if a := alarma.Props.Get(ical.PropAction); a != nil && !strings.EqualFold(strings.TrimSpace(a.Value), "DISPLAY") {
		return 0, false
	}
	disparo := alarma.Props.Get(ical.PropTrigger)
	if disparo == nil ||
		strings.EqualFold(disparo.Params.Get(ical.ParamValue), string(ical.ValueDateTime)) ||
		strings.EqualFold(disparo.Params.Get(ical.ParamRelated), "END") {
		return 0, false
	}
	m := duracionAntes.FindStringSubmatch(strings.TrimSpace(disparo.Value))
	if m == nil {
		return 0, false
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	total := n(m[2])*1440 + n(m[3])*60 + n(m[4]) + (n(m[5])+30)/60
	if total != 0 && m[1] == "" {
		return 0, false // «después» del inicio: la web no lo edita
	}
	return total, true
}

// Recordatorios son los minutos antes del inicio de los recordatorios editables del
// VEVENT maestro, en su orden. Un recurso ilegible no tiene ninguno.
func Recordatorios(data string) []int {
	out := []int{}
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return out
	}
	if m := masterComponent(cal); m != nil {
		for _, hijo := range m.Children {
			if hijo.Name != ical.CompAlarm {
				continue
			}
			if minutos, ok := minutosEditables(hijo); ok {
				out = append(out, minutos)
			}
		}
	}
	return out
}

// disparoAntes es `triggerFromMinutes` de la web: -PT15M, -PT2H, -P1D; -PT0M en el
// momento del evento.
func disparoAntes(minutos int) string {
	switch {
	case minutos <= 0:
		return "-PT0M"
	case minutos%1440 == 0:
		return "-P" + strconv.Itoa(minutos/1440) + "D"
	case minutos%60 == 0:
		return "-PT" + strconv.Itoa(minutos/60) + "H"
	}
	return "-PT" + strconv.Itoa(minutos) + "M"
}

// ConRecordatorios añade al VEVENT maestro de `data` un VALARM DISPLAY por cada minuto,
// con `descripcion` de texto: el `buildAlarm` de la web, para que un recordatorio puesto
// desde una app nativa sea el mismo bloque que si se hubiera puesto en la web.
func ConRecordatorios(data string, minutos []int, descripcion string) (string, error) {
	if len(minutos) == 0 {
		return data, nil
	}
	cal, err := ical.NewDecoder(strings.NewReader(data)).Decode()
	if err != nil {
		return "", fmt.Errorf("ical: decodificar el recurso: %w", err)
	}
	m := masterComponent(cal)
	if m == nil {
		return data, nil
	}
	if descripcion == "" {
		descripcion = "Recordatorio"
	}
	for _, min := range minutos {
		a := ical.NewComponent(ical.CompAlarm)
		a.Props.SetText(ical.PropAction, "DISPLAY")
		a.Props.SetText(ical.PropDescription, descripcion)
		disparo := ical.NewProp(ical.PropTrigger)
		disparo.Value = disparoAntes(min)
		a.Props.Set(disparo)
		m.Children = append(m.Children, a)
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return "", fmt.Errorf("ical: re-serializar el recurso: %w", err)
	}
	return buf.String(), nil
}
