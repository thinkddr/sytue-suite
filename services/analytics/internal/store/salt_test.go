// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// La sal es la pieza sobre la que se sostiene todo el ADR 0026, así que su
// comportamiento al cambiar el día se comprueba SIN PostgreSQL: una garantía que
// solo se verifica con Docker delante acaba no verificándose.
func TestSaltsRotaYBorraAlCambiarElDia(t *testing.T) {
	t.Parallel()
	ahora := time.Date(2026, 7, 29, 23, 59, 0, 0, time.UTC)

	var pedidos []string
	load := func(_ context.Context, day time.Time) ([]byte, error) {
		key := day.Format(time.DateOnly)
		pedidos = append(pedidos, key)
		s := make([]byte, SaltLen)
		for i := range s {
			s[i] = byte(len(pedidos)) // sal distinta por día
		}
		return s, nil
	}
	s := newSalts(load, func() time.Time { return ahora })
	ctx := context.Background()

	hoy, err := s.Today(ctx)
	if err != nil {
		t.Fatalf("sal de hoy: %v", err)
	}
	// Dentro del mismo día se sirve de la caché: una lectura al día, no un
	// viaje a PostgreSQL por evento.
	if _, err := s.Today(ctx); err != nil {
		t.Fatalf("segunda lectura: %v", err)
	}
	if len(pedidos) != 1 {
		t.Fatalf("la sal se pidió %d veces en el mismo día, debía ser 1", len(pedidos))
	}

	// Cambia el día.
	ahora = ahora.Add(2 * time.Minute)
	manana, err := s.Today(ctx)
	if err != nil {
		t.Fatalf("sal del día siguiente: %v", err)
	}
	if bytes.Equal(hoy, manana) {
		t.Fatal("la sal no cambió al cambiar el día")
	}
	if len(pedidos) != 2 || pedidos[1] != "2026-07-30" {
		t.Fatalf("no se pidió la sal del nuevo día: %v", pedidos)
	}

	// Y la de ayer ya no está en memoria: el búfer se sobrescribió con ceros.
	// (La fila de ayer la borra loadOrCreateSalt en la misma transacción.)
	if bytes.Contains(s.salt, hoy) {
		t.Fatal("la sal de ayer sigue en memoria")
	}
}

// Sin sal no se inventa una: es preferible perder eventos a producir un
// identificador que no cumpla la promesa.
func TestSaltsPropagaElError(t *testing.T) {
	t.Parallel()
	fallo := errors.New("postgres caído")
	s := newSalts(func(context.Context, time.Time) ([]byte, error) { return nil, fallo }, time.Now)
	if _, err := s.Today(context.Background()); !errors.Is(err, fallo) {
		t.Fatalf("se esperaba el error del cargador, salió %v", err)
	}
}

func TestSaltsRechazaLongitudInvalida(t *testing.T) {
	t.Parallel()
	s := newSalts(func(context.Context, time.Time) ([]byte, error) { return []byte("corta"), nil }, time.Now)
	if _, err := s.Today(context.Background()); err == nil {
		t.Fatal("una sal de longitud incorrecta debe ser un error")
	}
}
