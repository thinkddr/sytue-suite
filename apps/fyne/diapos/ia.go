// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/htmltext"
	"git.sytue.eu/sytue/one/apps/nucleo/ia"
)

// # Sytue AI en Diapos
//
// El panel «Asistente» es el de los editores (`docus.PanelIA`, el `AiPanel.tsx` común);
// lo propio de una presentación es su destino. Como en la web, el contexto es el texto de
// la caja que se está escribiendo —lo elegido o, sin nada elegido, la caja entera—, y
// «Insertar» no escribe la respuesta en una caja: CREA LÁMINAS detrás de la actual
// (`iaFill` y `iaslides.ts`), porque seiscientas palabras en una caja son una diapositiva
// que no se lee. «Reemplazar» sí escribe en la caja, en lugar de lo elegido.

// PuntosPorDiapo es PUNTOS_POR_DIAPO: los que caben en una lámina antes de que deje de
// leerse.
const PuntosPorDiapo = 6

// DiapoIA es una lámina de la respuesta: su título y los puntos del cuerpo.
type DiapoIA struct {
	Titulo []ia.Trozo
	Cuerpo []ia.Bloque
}

// DiapositivasDe es `diapositivasDe`: cada título abre una lámina y lo de detrás es su
// cuerpo; un apartado de más de seis puntos sigue en otra lámina con el mismo título; sin
// títulos, una sola. Una lámina sin título ni cuerpo no sale.
func DiapositivasDe(bloques []ia.Bloque) []DiapoIA {
	var salida []DiapoIA
	for _, b := range bloques {
		if b.Tipo == ia.H1 || b.Tipo == ia.H2 || b.Tipo == ia.H3 {
			salida = append(salida, DiapoIA{Titulo: b.Trozos})
			continue
		}
		if len(salida) == 0 {
			salida = append(salida, DiapoIA{})
		}
		if act := &salida[len(salida)-1]; len(act.Cuerpo) >= PuntosPorDiapo {
			salida = append(salida, DiapoIA{Titulo: act.Titulo})
		}
		act := &salida[len(salida)-1]
		act.Cuerpo = append(act.Cuerpo, b)
	}
	fuera := salida[:0]
	for _, d := range salida {
		if len(d.Titulo) > 0 || len(d.Cuerpo) > 0 {
			fuera = append(fuera, d)
		}
	}
	return fuera
}

// htmlDeTrozos es `htmlDeTrozos`: el texto ESCAPADO, con `<strong>` o `<em>`. Las únicas
// etiquetas son las que escribe esta función: lo del modelo nunca entra como marcado.
func htmlDeTrozos(trozos []ia.Trozo) string {
	var b strings.Builder
	for _, t := range trozos {
		texto := htmltext.Escape(t.Texto)
		switch {
		case t.Negrita:
			b.WriteString("<strong>" + texto + "</strong>")
		case t.Cursiva:
			b.WriteString("<em>" + texto + "</em>")
		default:
			b.WriteString(texto)
		}
	}
	return b.String()
}

// htmlDeCuerpo es `htmlDeCuerpo`: viñetas si todo el cuerpo es lista, párrafos si no.
func htmlDeCuerpo(cuerpo []ia.Bloque) string {
	if len(cuerpo) == 0 {
		return ""
	}
	lista := true
	for _, b := range cuerpo {
		lista = lista && b.Tipo == ia.Punto
	}
	var s strings.Builder
	if lista {
		s.WriteString("<ul>")
	}
	for _, b := range cuerpo {
		if lista {
			s.WriteString("<li>" + htmlDeTrozos(b.Trozos) + "</li>")
		} else {
			s.WriteString("<p>" + htmlDeTrozos(b.Trozos) + "</p>")
		}
	}
	if lista {
		s.WriteString("</ul>")
	}
	return s.String()
}

// TipoIA: una presentación.
func (e *Editor) TipoIA() string { return "presentation" }

// ContextoIA es el de la caja que se escribe; sin caja, nada (en la web el `root` del
// editor es la caja, y sin ella no hay texto que leer).
func (e *Editor) ContextoIA() (string, bool) {
	if w := e.escribiendo; w != nil {
		return w.folio.ContextoIA()
	}
	return "", false
}

// AplicarIA: «Reemplazar» escribe en la caja, en lugar de lo elegido; «Insertar» crea
// las láminas detrás de la actual (`iaFill`) y deja elegida la primera.
func (e *Editor) AplicarIA(respuesta string, reemplazar bool) string {
	if e.ctx.SoloLectura {
		return ""
	}
	if reemplazar {
		w := e.escribiendo
		if w == nil {
			return ""
		}
		clave := w.folio.AplicarIA(respuesta, true)
		w.consolidar(e)
		return clave
	}
	diapos := DiapositivasDe(ia.BloquesDe(respuesta))
	if len(diapos) == 0 {
		return ""
	}
	e.TerminarEscritura()
	e.cambiar(func() {
		tras := e.activa
		for _, d := range diapos {
			disp := DispTitulo
			if len(d.Titulo) > 0 {
				disp = DispTituloCuerpo
			}
			nueva := NuevaDiapo(disp, e.t)
			var texto []int
			for i, c := range nueva.Cajas {
				if esTexto(c) {
					texto = append(texto, i)
				}
			}
			switch {
			case len(texto) > 1:
				nueva.Cajas[texto[0]].HTML = htmlDeTrozos(d.Titulo)
				nueva.Cajas[texto[1]].HTML = htmlDeCuerpo(d.Cuerpo)
			case len(texto) == 1:
				// Sin título, la disposición de una caja lleva el CUERPO. La web la llena con
				// el título vacío y el cuerpo se pierde; aquí no se pierde nada.
				nueva.Cajas[texto[0]].HTML = htmlDeCuerpo(d.Cuerpo)
			}
			tras = e.m.InsertarTras(tras, nueva)
		}
		// La PRIMERA de las nuevas: quien acaba de generar cinco las repasa desde el principio.
		e.activa++
		e.sel, e.extra, e.selCon = "", nil, ""
	}, true)
	return "ai.slidesAdded"
}
