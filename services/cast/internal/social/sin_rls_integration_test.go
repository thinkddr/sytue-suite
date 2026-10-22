// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package social

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Con un rol que se salta la RLS, los comentarios y el chat de un vídeo no se leen
// ni se borran nombrando otra organización, y la lectura sin sesión solo enseña
// los de un vídeo abierto: lo sostiene el SQL, no la política.
func TestSinRLSLoSocialEsDeLaOrganizacionDelVideo(t *testing.T) {
	h := montar(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := h.PoolSinRLS(t)
	st := &store{pool: pool}

	deOrg := videoEn(t, pool, "acme", "ana", "org", "ready")
	abierto := videoEn(t, pool, "acme", "ana", "public", "ready")
	autora := Autor{Sub: "ana", Name: "Ana"}
	c, err := st.insertarComentario(ctx, deOrg, autora, "solo para acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.insertarComentario(ctx, abierto, autora, "para cualquiera", nil); err != nil {
		t.Fatal(err)
	}
	m, err := st.insertarChat(ctx, deOrg, autora, "hola")
	if err != nil {
		t.Fatal(err)
	}

	// El mismo vídeo, nombrado desde otra organización.
	deFuera := deOrg
	deFuera.Tenant = "globex"
	if cs, err := st.comentarios(ctx, deFuera, "", nil, 10); err != nil || len(cs) != 0 {
		t.Errorf("globex lee %d comentarios de un vídeo de acme (%v)", len(cs), err)
	}
	if ms, err := st.historialChat(ctx, deFuera, "", time.Time{}, 10); err != nil || len(ms) != 0 {
		t.Errorf("globex lee %d mensajes del chat de un vídeo de acme (%v)", len(ms), err)
	}
	if err := st.borrarComentario(ctx, deFuera, c.ID, "zoe", true); !errors.Is(err, ErrNoExiste) {
		t.Errorf("globex borra un comentario de acme: %v", err)
	}
	if err := st.borrarChat(ctx, deFuera, m.ID, "zoe", true); !errors.Is(err, ErrNoExiste) {
		t.Errorf("globex borra un mensaje de acme: %v", err)
	}
	if _, err := st.insertarComentario(ctx, deFuera, Autor{Sub: "zoe"}, "respuesta", &c.ID); !errors.Is(err, ErrNoExiste) {
		t.Errorf("globex responde a un comentario de acme: %v", err)
	}

	// Sin sesión: los de un vídeo abierto, y no los de uno de organización.
	if cs, err := st.comentariosPublicos(ctx, abierto.ID, nil, 10); err != nil || len(cs) != 1 {
		t.Errorf("comentarios públicos de un vídeo abierto: %d (%v)", len(cs), err)
	}
	if cs, err := st.comentariosPublicos(ctx, deOrg.ID, nil, 10); err != nil || len(cs) != 0 {
		t.Errorf("la lectura sin sesión enseña %d comentarios de un vídeo de organización (%v)", len(cs), err)
	}
	// Y en su organización siguen ahí.
	if cs, err := st.comentarios(ctx, deOrg, "", nil, 10); err != nil || len(cs) != 1 {
		t.Errorf("acme ya no ve su comentario: %d (%v)", len(cs), err)
	}
}
