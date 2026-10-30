// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js

package llamada

// En js/wasm no hay llamada nativa: lksdk (su paquete `e2ee`) y pion no compilan ahí, y
// ese destino no se publica —solo se compila para comprobar tipos sin CGO—. Esto deja la
// misma API para que `apps/rooms` compile; con Disponible en false, la app no ofrece la
// llamada nativa y la abre en la web.

// Disponible dice que la llamada nativa NO existe en este destino.
const Disponible = false

// Llamada es el hueco: nunca se conecta.
type Llamada struct{}

// Entrar dice que aquí no hay llamada nativa.
func Entrar(string, string, Opciones) (*Llamada, error) { return nil, ErrSinLlamadaNativa }

// Micro no hace nada.
func (*Llamada) Micro(bool) error { return ErrSinLlamadaNativa }

// Participantes no hay.
func (*Llamada) Participantes() []Participante { return nil }

// Reconectando nunca.
func (*Llamada) Reconectando() bool { return false }

// PuedePublicar nunca en el destino sin transporte nativo.
func (*Llamada) PuedePublicar() bool { return false }

// Camara no hace nada.
func (*Llamada) Camara(bool) error { return ErrSinLlamadaNativa }

// Pantalla no hace nada.
func (*Llamada) Pantalla(bool) error { return ErrSinLlamadaNativa }

// CambiarAudio no hace nada.
func (*Llamada) CambiarAudio(Dispositivo) error { return ErrSinLlamadaNativa }

// Identidad no hay.
func (*Llamada) Identidad() string { return "" }

// Grabando nunca.
func (*Llamada) Grabando() bool { return false }

// Senal no sale.
func (*Llamada) Senal(Senal) error { return ErrSinLlamadaNativa }

// Colgar no hace nada.
func (*Llamada) Colgar() {}
