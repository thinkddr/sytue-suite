// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ErrSesionCerrada cancela los ejecutores de almacenamiento cifrado cuando la
// sesión termina o cambia en esta u otra app del equipo.
var ErrSesionCerrada = errors.New("sesion: sesión cerrada")

// PendientesAlmacenamiento cuenta todo lo que las apps guardaron sin subir en
// el almacén local de este equipo: capturas y borradores de Notepads más los
// diarios de subidas cifradas por bloques (`storage-file-uploads-v1`).
func PendientesAlmacenamiento() int {
	n, _ := contarAlmacen(notas.EspacioCapturas, notas.EspacioBorradores, storage.PendingUploadsNamespace)
	return n
}

// SubirPendientes intenta enviar lo que se puede drenar automáticamente con la
// sesión actual y devuelve cuánto sigue pendiente en disco antes de salir.
func (s *Suite) SubirPendientes(ctx context.Context) int {
	_ = s.SubirNotepads(ctx)
	return PendientesAlmacenamiento()
}

// IniciarTareaAlmacenamiento enlaza un ejecutor de almacenamiento cifrado a la
// vida de la sesión. SalirConError cancela su contexto y espera a que `fin` se
// ejecute antes de vaciar la caché local y olvidar las claves del llavero, de
// modo que una tarea en curso no pueda recrear diarios ni usar claves tras salir.
func (s *Suite) IniciarTareaAlmacenamiento(padre context.Context) (context.Context, func()) {
	if padre == nil {
		padre = context.Background()
	}
	s.tareasAlmacenMu.Lock()
	if s.almacenCerrando {
		s.tareasAlmacenMu.Unlock()
		ctx, cancelar := context.WithCancelCause(padre)
		cancelar(ErrSesionCerrada)
		return ctx, func() {}
	}
	ctx, cancelar := context.WithCancelCause(padre)
	if s.tareasAlmacen == nil {
		s.tareasAlmacen = make(map[uint64]context.CancelCauseFunc)
	}
	s.tareasAlmacenSeq++
	id := s.tareasAlmacenSeq
	s.tareasAlmacen[id] = cancelar
	s.tareasAlmacenWG.Add(1)
	s.tareasAlmacenMu.Unlock()

	fin := sync.OnceFunc(func() {
		cancelar(nil)
		s.tareasAlmacenMu.Lock()
		delete(s.tareasAlmacen, id)
		s.tareasAlmacenMu.Unlock()
		s.tareasAlmacenWG.Done()
	})
	return ctx, fin
}

func (s *Suite) cancelarTareasAlmacenamiento() {
	s.tareasAlmacenMu.Lock()
	pendientes := make([]context.CancelCauseFunc, 0, len(s.tareasAlmacen))
	for id, cancelar := range s.tareasAlmacen {
		pendientes = append(pendientes, cancelar)
		delete(s.tareasAlmacen, id)
	}
	s.tareasAlmacenMu.Unlock()
	for _, cancelar := range pendientes {
		cancelar(ErrSesionCerrada)
	}
}

func (s *Suite) detenerTareasAlmacenamiento() func() {
	s.tareasAlmacenMu.Lock()
	s.almacenCerrando = true
	pendientes := make([]context.CancelCauseFunc, 0, len(s.tareasAlmacen))
	for id, cancelar := range s.tareasAlmacen {
		pendientes = append(pendientes, cancelar)
		delete(s.tareasAlmacen, id)
	}
	s.tareasAlmacenMu.Unlock()

	for _, cancelar := range pendientes {
		cancelar(ErrSesionCerrada)
	}
	s.tareasAlmacenWG.Wait()

	return sync.OnceFunc(func() {
		s.tareasAlmacenMu.Lock()
		s.almacenCerrando = false
		s.tareasAlmacenMu.Unlock()
	})
}
