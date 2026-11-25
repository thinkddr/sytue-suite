// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// TestOnWriteSoloTrasUnaMutacionAceptada: el aviso al widget nativo (Config.OnWrite) sale
// tras cada escritura aceptada en el repositorio privado, una vez, y nunca tras una que
// falla: es lo que vuelve a publicar el widget de una app de registros en Matrix, cuyas
// escrituras no pasan por HTTP (ADR 0078).
func TestOnWriteSoloTrasUnaMutacionAceptada(t *testing.T) {
	ctx := t.Context()
	server := &mutationHeadServer{objects: map[storage.Reference]storage.ObjectVersion{}, records: map[storage.Reference]storage.Record{}}
	repo := mutationHeadClient(t, server, mutationHeadJournal(t), "DEVICE_A")
	avisos := 0
	repo.cfg.OnWrite = func() { avisos++ }
	cuerpo := func(nota string) func(context.Context) (string, string, string, []byte, error) {
		return func(context.Context) (string, string, string, []byte, error) {
			return "contact", "person", "live", bytes.Clone([]byte(`{"note":"` + nota + `"}`)), nil
		}
	}
	if _, err := repo.Mutate(ctx, "seed", "person", cuerpo("uno")); err != nil || avisos != 1 {
		t.Fatalf("tras escribir: %v, %d avisos", err, avisos)
	}
	falla := errors.New("no se escribe")
	if _, err := repo.Mutate(ctx, "update", "otra", func(context.Context) (string, string, string, []byte, error) {
		return "", "", "", nil, falla
	}); !errors.Is(err, falla) || avisos != 1 {
		t.Fatalf("una escritura que falla avisa: %v, %d avisos", err, avisos)
	}
	// Reanudar una que no estaba en el diario no escribe nada: no avisa.
	if _, encontrada, err := repo.ResumeAcknowledged(ctx, "update", "nada", func(context.Context, []byte) error { return nil }); err != nil || encontrada || avisos != 1 {
		t.Fatalf("reanudar sin diario: %v, %v, %d avisos", encontrada, err, avisos)
	}
}
