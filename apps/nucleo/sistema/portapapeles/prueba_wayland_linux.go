// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android && sytuepruebas

package portapapeles

import (
	"errors"
	"os"
	"slices"
	"time"
)

// servirWaylandParaPrueba hace de OTRA aplicación que ha copiado `datos` como `tipo` en
// Wayland, con el control de datos del compositor: crea la fuente, la pone como selección y
// escribe los datos a quien los pida, hasta que se llama a lo que devuelve. Solo para los
// guardianes (etiqueta sytuepruebas).
func servirWaylandParaPrueba(tipo string, datos []byte) (func(), error) {
	w, err := conectarWayland()
	if err != nil {
		return nil, err
	}
	globales, err := w.globales()
	if err != nil {
		w.cerrar()
		return nil, err
	}
	i := slices.IndexFunc(globales, func(g globalWL) bool { return g.interfaz == "wl_seat" })
	j := slices.IndexFunc(globales, func(g globalWL) bool { return slices.Contains(interfacesControl, g.interfaz) })
	if i < 0 || j < 0 {
		w.cerrar()
		return nil, errors.New("el compositor no tiene control de datos")
	}
	idSeat, idGestor := w.enlazar(&globales[i], 1), w.enlazar(&globales[j], 1)
	fuente := w.nuevo()
	w.pedir(idGestor, 0, u32(fuente)) // create_data_source(id)
	w.pedir(fuente, 0, cadena(tipo))  // offer(mime)
	dispositivo := w.nuevo()
	w.pedir(idGestor, 1, u32(dispositivo), u32(idSeat)) // get_data_device(id, seat)
	w.pedir(dispositivo, 0, u32(fuente))                // set_selection(source)
	if _, _, err := w.seleccion(dispositivo); err != nil {
		w.cerrar()
		return nil, err
	}
	_ = w.c.SetDeadline(time.Time{})
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		for {
			ev, err := w.evento()
			if err != nil {
				return
			}
			if ev.obj == fuente && ev.op == 0 && len(w.fds) > 0 { // send(mime, fd)
				fd := w.fds[0]
				w.fds = w.fds[1:]
				f := os.NewFile(uintptr(fd), "portapapeles")
				_, _ = f.Write(datos)
				_ = f.Close()
			}
		}
	}()
	return func() { w.cerrar(); <-hecho }, nil
}
