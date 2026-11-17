// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import "slices"

// Qué acciones de Rooms no existen todavía para un mensaje cifrado de extremo a extremo.
// Gemela de apps/frontend/apps/rooms/src/lib/cifrado.ts.
//
// Programar, encuesta, tarea y pedir acuse —y sus respuestas, votar y confirmar— se
// apoyan en el servicio: él guarda la cola de programados, cuenta los votos y suma los
// acuses. Con el cifrado activo no ve el mensaje, así que esas llamadas fallan o, peor,
// publican una encuesta que nadie puede votar. Decisión del dueño: NO esconderlas, sino
// dejarlas a la vista y apagadas con una línea que diga por qué. Cuando una viaje por
// Matrix, se quita de esta lista y vuelve en todas partes.

// AccionCifrada es una acción que depende de que el servicio lea el mensaje.
type AccionCifrada string

// Las acciones de la lista, con los nombres de la web.
const (
	AccionProgramar      AccionCifrada = "programar"
	AccionEncuesta       AccionCifrada = "encuesta"
	AccionTarea          AccionCifrada = "tarea"
	AccionPedirAcuse     AccionCifrada = "pedirAcuse"
	AccionVotar          AccionCifrada = "votar"
	AccionConfirmarAcuse AccionCifrada = "confirmarAcuse"
)

var noDisponiblesCifrado = []AccionCifrada{
	AccionProgramar, AccionEncuesta, AccionTarea, AccionPedirAcuse, AccionVotar, AccionConfirmarAcuse,
}

// DisponibleConCifrado dice si `a` se puede ofrecer con el cifrado en ese estado. Sin
// cifrado, todo.
func DisponibleConCifrado(a AccionCifrada, cifrado bool) bool {
	return !cifrado || !slices.Contains(noDisponiblesCifrado, a)
}

// conCifrado dice si la sala publica cifrado de extremo a extremo: lo hace en cuanto
// tiene el ciclo de vida del dispositivo Matrix (cmd/rooms lo pone siempre; los
// guardianes sin él prueban la rama sin cifrar). Es la misma pregunta que
// `seguridadMensaje`.
func (sa *Sala) conCifrado() bool { return sa.Cifrado != nil }

// puede es DisponibleConCifrado para esta sala.
func (sa *Sala) puede(a AccionCifrada) bool { return DisponibleConCifrado(a, sa.conCifrado()) }
