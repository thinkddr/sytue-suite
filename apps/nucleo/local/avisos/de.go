// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package avisos

import (
	"cmp"
	"fmt"
	"net/url"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// Los recordatorios de Calendar y Notepads los programa cada app y, en Linux con la
// app cerrada, los avisa el Hub leyendo sus almacenes (HU-06). Tienen que ser los
// MISMOS —clave, texto y enlace—: si no, el Hub avisaría otra vez lo que la app ya
// avisó, o abriría otra cosa. Por eso se forman aquí, una vez (R-NO-COPIAR).

// DeEvento es el recordatorio de una ocurrencia de un evento, `minutos` antes de su
// inicio. Lleva la hora (o la fecha, si es de todo el día) y el lugar; pulsado, abre el
// evento en Calendar.
func DeEvento(id, titulo, lugar string, inicio time.Time, todoElDia bool, minutos int) Aviso {
	cuerpo := inicio.Local().Format("15:04")
	if todoElDia {
		cuerpo = inicio.UTC().Format("2006-01-02")
	}
	if lugar != "" {
		cuerpo += " · " + lugar
	}
	return Aviso{
		Clave:  fmt.Sprintf("%s/%s/%d", id, inicio.UTC().Format(time.RFC3339), minutos),
		Titulo: titulo,
		Cuerpo: cuerpo,
		Cuando: inicio.Add(-time.Duration(minutos) * time.Minute),
		Enlace: espacio.Esquema("calendar") + ":/abrir?" + url.Values{"clase": {"evento"}, "id": {id}}.Encode(),
	}
}

// DeNota es el recordatorio de una nota que vence en `vence`; pulsado, abre la nota en
// Notepads. Sin título, el de la app.
func DeNota(id, titulo string, vence time.Time, todoElDia bool) Aviso {
	cuerpo := vence.Local().Format("15:04")
	if todoElDia {
		cuerpo = vence.UTC().Format("2006-01-02")
	}
	return Aviso{
		Clave:  id + "/" + vence.UTC().Format(time.RFC3339),
		Titulo: cmp.Or(titulo, "Sytue Notepads"),
		Cuerpo: cuerpo,
		Cuando: vence,
		Enlace: espacio.Esquema("notes") + ":/abrir?" + url.Values{"clase": {"nota"}, "id": {id}}.Encode(),
	}
}
