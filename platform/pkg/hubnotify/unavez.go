// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubnotify

import (
	"sync"
	"time"
)

// UnaVez deja pasar cada clave una sola vez por ventana. Es el freno de los avisos que
// alguien puede provocar a voluntad sobre otra persona: compartir, retirar y volver a
// compartir le mandaría un «X ha compartido… contigo» por vuelta.
//
// ponytail: en memoria, o sea POR RÉPLICA: con N réplicas pasan como mucho N por
// ventana. Si hiciera falta exacto, una tabla con la última vez por clave.
type UnaVez struct {
	ventana time.Duration
	mu      sync.Mutex
	vistas  map[string]time.Time
}

// NuevaUnaVez crea el freno con esa ventana.
func NuevaUnaVez(ventana time.Duration) *UnaVez {
	return &UnaVez{ventana: ventana, vistas: map[string]time.Time{}}
}

// Primera dice si `clave` no ha pasado en la ventana, y la apunta. Un freno nil deja
// pasar todo (los guardianes que montan los manejadores a mano).
func (u *UnaVez) Primera(clave string) bool {
	if u == nil {
		return true
	}
	ahora := time.Now()
	u.mu.Lock()
	defer u.mu.Unlock()
	for k, t := range u.vistas {
		if ahora.Sub(t) >= u.ventana {
			delete(u.vistas, k)
		}
	}
	if _, ya := u.vistas[clave]; ya {
		return false
	}
	u.vistas[clave] = ahora
	return true
}
