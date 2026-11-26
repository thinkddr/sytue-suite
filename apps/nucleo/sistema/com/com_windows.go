// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package com

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Objeto es una interfaz COM tal como la da el sistema: lo primero que hay en su
// memoria es el puntero a su tabla de métodos. Se usa siempre como *Objeto (el puntero
// que devuelve una función COM en su parámetro de salida `void **`).
type Objeto struct {
	tabla *[1 << 10]uintptr
}

// Los tres primeros métodos de toda interfaz (IUnknown).
const (
	metodoRelease = 2
)

// Llamar llama al método número `metodo` de la tabla (0, 1 y 2 son QueryInterface,
// AddRef y Release; los de cada interfaz van detrás, en el orden de su cabecera) con
// el propio objeto delante. Devuelve lo que devuelva el método (un HRESULT casi
// siempre: ver [HR]).
//
// Los punteros de Go que se pasen convertidos a uintptr en la llamada se quedan vivos y
// quietos hasta que vuelve (como `(*windows.LazyProc).Call`).
//
//go:uintptrescapes
func (o *Objeto) Llamar(metodo int, args ...uintptr) uintptr {
	if o == nil || o.tabla == nil {
		return uintptr(eNoInterfaz)
	}
	todos := make([]uintptr, 0, 1+len(args))
	todos = append(todos, uintptr(unsafe.Pointer(o)))
	todos = append(todos, args...)
	r, _, _ := syscall.SyscallN(o.tabla[metodo], todos...)
	return r
}

// Soltar es Release. Sobre nil no hace nada.
func (o *Objeto) Soltar() {
	if o != nil && o.tabla != nil {
		o.Llamar(metodoRelease)
	}
}

// eNoInterfaz es E_NOINTERFACE: lo que se devuelve al llamar sobre nil.
const eNoInterfaz = 0x80004002

// Error es un HRESULT de fallo, con lo que se estaba haciendo.
type Error struct {
	Que string
	HR  uint32
}

func (e *Error) Error() string { return fmt.Sprintf("%s: HRESULT 0x%08X", e.Que, e.HR) }

// HR pasa un HRESULT a error: nil si es de éxito (S_OK, S_FALSE…), un *Error si no.
func HR(r uintptr, que string) error {
	if int32(uint32(r)) >= 0 { //nolint:gosec // G115: un HRESULT son 32 bits con signo
		return nil
	}
	return &Error{Que: que, HR: uint32(r)} //nolint:gosec // G115: un HRESULT son los 32 bits bajos
}

// ---- Funciones de las DLL ----

// Funcion es una función exportada de una DLL del sistema que puede no estar (Windows
// N sin el paquete de características multimedia, una versión vieja): se busca al
// llamarla y, si no está, es un error.
type Funcion struct{ p *windows.LazyProc }

// NuevaFuncion prepara la función `nombre` de la DLL del sistema `dll` (de System32,
// nunca de la carpeta de la app). No carga nada todavía.
func NuevaFuncion(dll *windows.LazyDLL, nombre string) Funcion {
	return Funcion{p: dll.NewProc(nombre)}
}

// Esta dice si la DLL y la función existen en este equipo.
func (f Funcion) Esta() bool { return f.p != nil && f.p.Find() == nil }

// Llamar llama a la función; si no está, devuelve el error de búsqueda y no llama.
//
//go:uintptrescapes
func (f Funcion) Llamar(args ...uintptr) (uintptr, error) {
	if f.p == nil {
		return 0, errors.New("com: función sin preparar")
	}
	if err := f.p.Find(); err != nil {
		return 0, err
	}
	r, _, _ := syscall.SyscallN(f.p.Addr(), args...)
	return r, nil
}

// ---- El hilo COM ----

// Apartamento es el modelo de hilos de COM con que se inicia un [Hilo].
type Apartamento uint32

const (
	// Multihilo (MTA): el de Media Foundation.
	Multihilo Apartamento = windows.COINIT_MULTITHREADED
	// Unhilo (STA): el que prefiere el Shell (miniaturas, diálogos).
	Unhilo Apartamento = windows.COINIT_APARTMENTTHREADED | windows.COINIT_DISABLE_OLE1DDE
)

// Hilo es un hilo del sistema propio y fijo, con COM iniciado en su apartamento, que
// hace en orden lo que se le pide. Las rutinas de Go saltan de hilo en hilo y los
// objetos COM viven en el apartamento del hilo que los creó; además un hilo de la
// app puede estar ya en otro apartamento (el de la ventana, por OLE). Con un Hilo por
// trabajo nada de eso importa.
type Hilo struct {
	mu       sync.Mutex
	trabajos chan func()
	cerrado  bool
}

var errHiloCerrado = errors.New("com: el hilo ya está cerrado")

// NuevoHilo arranca el hilo e inicia COM en él. Un error es que COM no arranca.
func NuevoHilo(a Apartamento) (*Hilo, error) {
	h := &Hilo{trabajos: make(chan func())}
	listo := make(chan error, 1)
	go func() {
		// Sin UnlockOSThread: al acabar esta rutina el hilo se cierra con ella, y su
		// apartamento también.
		runtime.LockOSThread()
		if err := iniciarCOM(a); err != nil {
			listo <- err
			return
		}
		defer windows.CoUninitialize()
		listo <- nil
		for f := range h.trabajos {
			f()
		}
	}()
	if err := <-listo; err != nil {
		return nil, err
	}
	return h, nil
}

// iniciarCOM es CoInitializeEx, sin pánico si ole32 no respondiera.
func iniciarCOM(a Apartamento) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("com: CoInitializeEx: %v", r)
		}
	}()
	// S_FALSE (ya iniciado en este hilo) también vale; x/sys lo da como Errno(1).
	if e := windows.CoInitializeEx(0, uint32(a)); e != nil && !errors.Is(e, syscall.Errno(windows.S_FALSE)) {
		return fmt.Errorf("com: CoInitializeEx: %w", e)
	}
	return nil
}

// Hacer hace f en el hilo y espera a que acabe. Un pánico dentro de f vuelve como error.
func (h *Hilo) Hacer(f func() error) error {
	hecho := make(chan error, 1)
	trabajo := func() {
		defer func() {
			if r := recover(); r != nil {
				hecho <- fmt.Errorf("com: %v", r)
			}
		}()
		hecho <- f()
	}
	h.mu.Lock()
	if h.cerrado {
		h.mu.Unlock()
		return errHiloCerrado
	}
	// Con el cerrojo tomado: Cerrar no cierra el canal mientras se entrega.
	h.trabajos <- trabajo
	h.mu.Unlock()
	return <-hecho
}

// Cerrar acaba el hilo (cuando termine lo que esté haciendo). Se puede llamar más de
// una vez.
func (h *Hilo) Cerrar() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.cerrado {
		h.cerrado = true
		close(h.trabajos)
	}
}
