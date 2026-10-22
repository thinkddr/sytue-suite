// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"

	"github.com/google/uuid"
)

// vistas decide si una visita a la ficha cuenta como vista: una por (vídeo,
// persona o IP, hora), en memoria de esta réplica.
//
// ponytail: contador simple por réplica —con dos pods una misma persona puede
// contar dos veces en la misma hora—; analítica de verdad después. No se guarda
// quién: la clave es un hash de (sub o IP) con sal por proceso, y las entradas de
// horas pasadas se barren al pasar.
type vistas struct {
	mu    sync.Mutex
	sal   [16]byte
	hora  int64
	vista map[[32]byte]struct{}
	ahora func() time.Time
}

// maxVistasPorHora acota el mapa (unos 40 MB en el peor caso).
const maxVistasPorHora = 500_000

func nuevasVistas() *vistas {
	v := &vistas{vista: map[[32]byte]struct{}{}, ahora: time.Now}
	binary.LittleEndian.PutUint64(v.sal[:8], uint64(time.Now().UnixNano())) //nolint:gosec // sal de proceso, no criptografía
	return v
}

// nueva dice si es la primera vez en esta hora que esa persona (o IP) mira ese vídeo.
func (v *vistas) nueva(video uuid.UUID, quien Quien) bool {
	origen := quien.Sub
	if origen == "" {
		origen = quien.IP
	}
	if origen == "" {
		return false
	}
	h := sha256.New()
	h.Write(v.sal[:])
	h.Write(video[:])
	h.Write([]byte(origen))
	var clave [32]byte
	copy(clave[:], h.Sum(nil))

	hora := v.ahora().Unix() / 3600
	v.mu.Lock()
	defer v.mu.Unlock()
	if hora != v.hora {
		v.hora = hora
		v.vista = map[[32]byte]struct{}{}
	}
	if _, ya := v.vista[clave]; ya {
		return false
	}
	// Tope duro: dejar de contar es mejor que crecer sin límite si alguien
	// recorre la portada desde miles de IP en la misma hora.
	if len(v.vista) >= maxVistasPorHora {
		return false
	}
	v.vista[clave] = struct{}{}
	return true
}
