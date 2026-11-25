// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package authn

import (
	"context"
	"sync"
)

// Testigo deja ver, a un middleware que va POR FUERA de la autenticación, con
// qué claims se autenticó la petición por dentro.
//
// Existe por la traza de acceso (platform/pkg/auditlog): su middleware tiene que
// envolver el router entero para conocer la ruta que casó, y la autenticación va
// por dentro, en cada grupo de rutas, con el contexto derivado que el de fuera no
// ve. Into es el único sitio por el que pasa cualquier forma de autenticarse
// —JWT, contraseña de aplicación por Basic, token de API de Cloud—, así que es
// ahí donde se anota.
type Testigo struct {
	mu     sync.Mutex
	claims Claims
	visto  bool
}

type testigoKey struct{}

// ConTestigo devuelve un contexto con un Testigo vacío.
func ConTestigo(ctx context.Context) (context.Context, *Testigo) {
	t := &Testigo{}
	return context.WithValue(ctx, testigoKey{}, t), t
}

// Claims devuelve los últimos claims anotados y si los hubo.
func (t *Testigo) Claims() (Claims, bool) {
	if t == nil {
		return Claims{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.claims, t.visto
}

func (t *Testigo) anotar(c Claims) {
	t.mu.Lock()
	t.claims, t.visto = c, true
	t.mu.Unlock()
}
