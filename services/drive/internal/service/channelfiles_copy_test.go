// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de «Guardar en mi Drive» (CopyChannelFileToMyDrive). Lo que se guarda aquí es
// QUIÉN autoriza la copia: la membresía del canal, y no el permiso del ámbito personal.
// Un adjunto vive en la carpeta del canal, invisible para el resto de la organización, y
// colgar esta operación del permiso de «Mi unidad» dejaría copiar lo que no se puede ver.
//
// Se prueban los dos caminos de DENEGACIÓN, que son los que importan aquí y además los
// únicos aislables: el camino feliz pasa por `s.team`, que es el store concreto y no una
// interfaz, así que vive en el test del handler (con el servicio doblado) y en los e2e.
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// chatFake es la verja de membresía: `deja` decide si el sujeto está en el canal.
type chatFake struct {
	deja      bool
	preguntas int
}

func (c *chatFake) CanAccessChannel(_ context.Context, _ uuid.UUID, _ string) error {
	c.preguntas++
	if c.deja {
		return nil
	}
	return domain.ErrNotFound
}

func TestCopiarAdjuntoDeCanalAjenoNiExiste(t *testing.T) {
	chat := &chatFake{deja: false}
	s := newSvc(&fakeStore{}, newMapBlobs(), nil)
	s.chat = chat

	_, err := s.CopyChannelFileToMyDrive(tenantCtx(t), uuid.New(), uuid.New(), "bob")
	// ErrNotFound y no ErrPermission: no se filtra si el canal o el fichero existen.
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, quiero ErrNotFound", err)
	}
	if chat.preguntas != 1 {
		t.Fatalf("se consultó la verja %d veces, quiero 1", chat.preguntas)
	}
}

// TestCopiarAdjuntoSinVerjaFallaCerrado: sin authorizer configurado no se copia nada. Es
// el mismo criterio que ya aplica ChannelFile — un despliegue con la verja sin conectar
// no puede convertirse en una puerta abierta a los adjuntos de cualquier canal.
func TestCopiarAdjuntoSinVerjaFallaCerrado(t *testing.T) {
	s := newSvc(&fakeStore{}, newMapBlobs(), nil) // s.chat == nil

	if _, err := s.CopyChannelFileToMyDrive(tenantCtx(t), uuid.New(), uuid.New(), "bob"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin verja debe fallar cerrado, got %v", err)
	}
}
