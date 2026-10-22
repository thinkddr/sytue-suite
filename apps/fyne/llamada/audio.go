// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package llamada

import "sync"

// La voz que llega se guarda en una cola por pista y el altavoz las mezcla.
//
// La red entrega los paquetes a golpes, no cada 20 ms: sin colchón, el altavoz se
// quedaría sin muestras en cada hueco y la voz saldría a trozos. Por eso una cola no
// suena hasta tener su colchón dentro, y si se vacía vuelve a esperarlo. Y no crece sin
// fin: por encima de Tope se tira lo más viejo, porque una voz que llega con un segundo
// de retraso es peor que perder un trozo.
//
// El colchón se adapta a la red de cada pista: si se vacía y la voz vuelve enseguida (un
// hueco de la red, no una pausa de quien habla), crece un tramo, hasta ColchonMax; tras
// Calma sonando sin vaciarse, encoge uno, hasta Colchon. Así una conexión móvil que va a
// golpes deja de oírse cortada, y una buena no paga retraso de más.
//
// ponytail: sin estirar ni encoger el audio (el NetEQ de libwebrtc): crecer cuesta un
// silencio de un tramo, que es lo que ya se oía al vaciarse.
const (
	Colchon    = 3 * Tramo   // 60 ms: el de partida y el mínimo
	ColchonMax = 6 * Tramo   // 120 ms
	Tope       = 10 * Tramo  // 200 ms
	Calma      = 250 * Tramo // 5 s sonando sin vaciarse
)

type cola struct {
	mu       sync.Mutex
	datos    []int16
	sonando  bool
	objetivo int // el colchón de ahora
	calma    int // muestras sonadas desde el último vaciado
	vaciada  bool
	hueco    int // muestras que se ha pedido estando vacía
}

func nuevaCola() *cola { return &cola{objetivo: Colchon} }

func (c *cola) escribir(pcm []int16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Un hueco más corto que Tope es la red, no una pausa: el colchón se queda corto.
	if c.vaciada && c.hueco < Tope && c.objetivo < ColchonMax {
		c.objetivo += Tramo
	}
	c.vaciada = false
	c.datos = append(c.datos, pcm...)
	if sobra := len(c.datos) - Tope; sobra > 0 {
		c.datos = append(c.datos[:0], c.datos[sobra:]...)
	}
}

// leer suma a `acc` lo que la cola tenga para este tramo.
func (c *cola) leer(acc []int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.sonando && len(c.datos) < c.objetivo {
		c.hueco += len(acc)
		return
	}
	c.sonando = true
	n := min(len(acc), len(c.datos))
	for i := range n {
		acc[i] += int32(c.datos[i])
	}
	c.datos = append(c.datos[:0], c.datos[n:]...)
	if n < len(acc) {
		c.sonando = false // se vació: a esperar otra vez el colchón
		c.vaciada, c.hueco, c.calma = true, len(acc)-n, 0
		return
	}
	if c.calma += n; c.calma >= Calma && c.objetivo > Colchon {
		c.objetivo -= Tramo
		c.calma = 0
	}
}

// Mezclar escribe en `out` la suma de las colas, recortada a 16 bits.
func Mezclar(out []int16, colas ...*cola) {
	acc := make([]int32, len(out))
	for _, c := range colas {
		c.leer(acc)
	}
	for i, v := range acc {
		out[i] = int16(max(-32768, min(32767, v)))
	}
}
