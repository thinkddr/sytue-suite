// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPendientesAlmacenamientoCuentaDiariosCifradosYNotepads(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	alm, err := Almacen()
	if err != nil {
		t.Fatal(err)
	}
	if err := alm.Write("ada/", notas.EspacioBorradores, "nb-1", "borrador"); err != nil {
		t.Fatal(err)
	}
	if err := alm.Write("ada/", storage.PendingUploadsNamespace, "job-1", map[string]string{"id": "job-1"}); err != nil {
		t.Fatal(err)
	}

	if got := PendientesNotepads(); got != 1 {
		t.Fatalf("PendientesNotepads = %d; quiero 1", got)
	}
	if got := PendientesAlmacenamiento(); got != 2 {
		t.Fatalf("PendientesAlmacenamiento = %d; quiero 2", got)
	}

	if err := s.SalirConError(); err != nil {
		t.Fatal(err)
	}
	if got := PendientesAlmacenamiento(); got != 0 {
		t.Fatalf("tras SalirConError quedan %d pendientes en disco", got)
	}
}

func TestSalirCancelaYEsperaTareasAlmacenamientoAntesDeVaciarCache(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	alm, err := Almacen()
	if err != nil {
		t.Fatal(err)
	}

	enCurso := make(chan struct{})
	cancelada := make(chan struct{})
	soltarLimpieza := make(chan struct{})

	ctx, fin := s.IniciarTareaAlmacenamiento(context.Background())
	go func() {
		defer fin()
		close(enCurso)
		<-ctx.Done()
		close(cancelada)
		<-soltarLimpieza
		// Simula un último volcado del ejecutor antes de devolver el control a `fin()`:
		// como SalirConError espera a `fin()` ANTES de vaciarAlmacen, este rastro tiene
		// que desaparecer cuando SalirConError termine.
		_ = alm.Write("ada/", storage.PendingUploadsNamespace, "job-en-vuelo", map[string]string{"id": "job-en-vuelo"})
	}()

	<-enCurso
	cierre := make(chan error, 1)
	go func() {
		cierre <- s.SalirConError()
	}()

	select {
	case <-cancelada:
	case <-time.After(5 * time.Second):
		t.Fatal("SalirConError no canceló la tarea activa de almacenamiento")
	}
	if !errors.Is(context.Cause(ctx), ErrSesionCerrada) {
		t.Fatalf("causa de cancelación = %v; quiero %v", context.Cause(ctx), ErrSesionCerrada)
	}

	// Mientras SalirConError está esperando a que terminen las tareas, cualquier
	// intento nuevo recibe un contexto ya cancelado y no se cuela detrás del vaciado.
	ctxTardia, finTardia := s.IniciarTareaAlmacenamiento(context.Background())
	defer finTardia()
	if !errors.Is(context.Cause(ctxTardia), ErrSesionCerrada) {
		t.Fatalf("tarea iniciada durante el cierre no nació cancelada: %v", context.Cause(ctxTardia))
	}

	select {
	case err := <-cierre:
		t.Fatalf("SalirConError terminó sin esperar a la tarea activa: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	close(soltarLimpieza)
	select {
	case err := <-cierre:
		if err != nil {
			t.Fatalf("SalirConError: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SalirConError no terminó tras finalizar la tarea")
	}

	if got := PendientesAlmacenamiento(); got != 0 {
		t.Fatalf("el rastro escrito antes de fin() sobrevivió a vaciarAlmacen: %d", got)
	}

	// Tras completarse el cierre, una sesión posterior vuelve a admitir tareas.
	ctxNueva, finNueva := s.IniciarTareaAlmacenamiento(context.Background())
	defer finNueva()
	if err := ctxNueva.Err(); err != nil {
		t.Fatalf("tras completar el cierre las tareas nuevas siguen bloqueadas: %v", err)
	}
}
