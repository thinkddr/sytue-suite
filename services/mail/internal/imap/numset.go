// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imap

import (
	"math"

	"github.com/emersion/go-imap/v2"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// seqNum convierte un índice/contador int no-negativo a número de secuencia
// IMAP (uint32, RFC 9051) con clamp defensivo frente a overflow (gosec G115).
func seqNum(n int) uint32 {
	if n < 0 {
		return 0
	}
	if uint64(n) > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(n) //nolint:gosec // G115: acotado a [0, MaxUint32] por los guards.
}

// selected empareja un mensaje con su número de secuencia IMAP (1-based) dentro
// del buzón seleccionado. El orden de secuencia sigue el UID ascendente.
type selected struct {
	Seq uint32
	Msg domain.Message
}

// resolveSet selecciona, de la lista de mensajes de un buzón ordenada por UID
// ascendente, los que caen dentro del conjunto numSet. Si el conjunto es de
// secuencia (SeqSet) el índice es la posición 1-based; si es de UID (UIDSet) se
// compara contra el UID del mensaje. Implementa la semántica de RFC 9051 §9
// incluyendo el comodín "*" (el mayor número/UID presente), representado como 0
// en los rangos de go-imap.
//
// Es lógica pura (sin I/O): testeable de forma unitaria.
func resolveSet(msgs []domain.Message, numSet imap.NumSet) []selected {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]selected, 0, len(msgs))
	switch set := numSet.(type) {
	case imap.SeqSet:
		lastSeq := seqNum(len(msgs))
		for i, m := range msgs {
			seq := seqNum(i + 1)
			if seqSetContains(set, seq, lastSeq) {
				out = append(out, selected{Seq: seq, Msg: m})
			}
		}
	case imap.UIDSet:
		var maxUID imap.UID
		for _, m := range msgs {
			if imap.UID(m.UID) > maxUID {
				maxUID = imap.UID(m.UID)
			}
		}
		for i, m := range msgs {
			if uidSetContains(set, imap.UID(m.UID), maxUID) {
				out = append(out, selected{Seq: seqNum(i + 1), Msg: m})
			}
		}
	default:
		// Conjunto desconocido: no seleccionar nada (deny-by-default).
	}
	return out
}

// seqSetContains resuelve la pertenencia de un número de secuencia, tratando el
// comodín "*" (Start/Stop == 0) como lastSeq.
func seqSetContains(set imap.SeqSet, seq, lastSeq uint32) bool {
	for _, rng := range set {
		start, stop := normalizeRange(rng.Start, rng.Stop, lastSeq)
		if seq >= start && seq <= stop {
			return true
		}
	}
	return false
}

func uidSetContains(set imap.UIDSet, uid, maxUID imap.UID) bool {
	for _, rng := range set {
		start, stop := normalizeRange(uint32(rng.Start), uint32(rng.Stop), uint32(maxUID))
		if uint32(uid) >= start && uint32(uid) <= stop {
			return true
		}
	}
	return false
}

// normalizeRange traduce un rango (Start,Stop) con posible comodín "*" (0) a un
// intervalo [lo,hi] cerrado y ordenado. Un valor 0 se sustituye por star (el
// mayor presente); si ambos extremos existen pero están invertidos, se ordenan.
func normalizeRange(start, stop, star uint32) (lo, hi uint32) {
	if start == 0 {
		start = star
	}
	if stop == 0 {
		stop = star
	}
	if start <= stop {
		return start, stop
	}
	return stop, start
}
