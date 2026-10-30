// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package nube

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// # Lo que cambia en Drive
//
// Windows pide cada carpeta UNA vez (al poblarla); lo que cambie después en Drive —desde
// la web, otro equipo, otra persona— no llega solo. Cada `cadaCuanto` se repasan las
// carpetas ya pobladas contra Drive (`comparar`): lo nuevo entra como marcador, lo que
// Drive ya no tiene se quita, lo renombrado se renombra y lo que tiene contenido nuevo se
// vacía para bajarse otra vez al abrirlo (o en el acto, si estaba fijado).
//
// ponytail: una petición por carpeta poblada y vuelta, hasta `maxCarpetas`; con el flujo
// de cambios de Drive (SSE del Hub) se pediría solo lo que ha cambiado.

const (
	cadaCuanto  = 2 * time.Minute
	maxCarpetas = 500
)

func refrescar(ctx context.Context, dir string) {
	t := time.NewTicker(cadaCuanto)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			refrescarUnaVez(ctx, dir)
		}
	}
}

// refrescarUnaVez pasa por las carpetas pobladas; las que no lo están no se tocan (se
// pedirán enteras al abrirlas).
func refrescarUnaVez(ctx context.Context, dir string) {
	n := 0
	_ = filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || ctx.Err() != nil {
			return nil //nolint:nilerr // una entrada que no se deja leer no para las demás
		}
		if info, e := d.Info(); e == nil {
			if a, ok := info.Sys().(*syscall.Win32FileAttributeData); ok && a.FileAttributes&atributoSinBajar != 0 {
				return filepath.SkipDir
			}
		}
		n++
		if n > maxCarpetas {
			return filepath.SkipAll
		}
		if err := refrescarCarpeta(ctx, ruta); err != nil {
			slog.Info("nube: una carpeta no se ha podido comparar con Drive", "ruta", ruta, "error", err)
		}
		return nil
	})
}

func refrescarCarpeta(ctx context.Context, ruta string) error {
	identidad, err := identidadDeRuta(ruta)
	if err != nil {
		return err
	}
	f, _ := actual()
	if f == nil {
		return nil
	}
	remotas, completa, err := listar(ctx, f, identidad)
	if err != nil {
		return err
	}
	hay, err := locales(ruta)
	if err != nil {
		return err
	}
	c := comparar(remotas, completa, hay)
	if len(c.Nuevas) > 0 {
		crearMarcadores(ruta, c.Nuevas)
	}
	for _, r := range c.Renombrar {
		propios.Store(r.Local.Identidad, true)
		nueva := filepath.Join(ruta, r.Nombre)
		if err := renombrarSinPisar(filepath.Join(ruta, r.Local.Nombre), nueva); err != nil {
			slog.Warn("nube: no se ha podido renombrar lo que se renombró en Drive", "ruta", ruta, "error", err)
		} else {
			// Renombrar le quita el «sincronizado» a un marcador, y entonces el refresco
			// ya no lo actualizaría y el siguiente cierre lo subiría encima de lo de Drive
			// (medido: el contenido nuevo de la web se quedaba sin bajar).
			marcarSincronizado(nueva)
		}
		propios.Delete(r.Local.Identidad)
	}
	for _, l := range c.Quitar {
		if _, ya := sinPapelera.Load(l.Identidad); ya {
			continue
		}
		donde := filepath.Join(ruta, l.Nombre)
		switch err := quitar(donde, l, locales, aPapelera); {
		case errors.Is(err, errPendiente):
			slog.Info("nube: Drive ya no lo tiene, pero dentro hay algo sin subir y se queda", "ruta", donde)
		case err != nil:
			sinPapelera.Store(l.Identidad, true)
			slog.Warn("nube: lo que Drive ya no tiene no ha ido a la papelera y se queda", "ruta", donde, "error", err)
		}
	}
	for _, e := range c.Actualizar {
		vaciar(filepath.Join(ruta, e.Nombre), e)
	}
	marcarSincronizado(ruta)
	return nil
}

// sinPapelera son las identidades que no han podido ir a la papelera de reciclaje: no se
// vuelven a intentar hasta que el Hub arranque otra vez, para no preguntar cada dos
// minutos si borrar para siempre lo que no cabe en ella.
var sinPapelera sync.Map

// renombrarSinPisar es `os.Rename` sin MOVEFILE_REPLACE_EXISTING: si el nombre nuevo ya
// lo tiene otra cosa del disco, falla en vez de sustituirla (FYS-03).
func renombrarSinPisar(de, a string) error {
	p, err := windows.UTF16PtrFromString(de)
	if err != nil {
		return err
	}
	q, err := windows.UTF16PtrFromString(a)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(p, q, 0)
}

var procOperacionShell = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// aPapelera manda `ruta` a la papelera de reciclaje de Windows, como «Eliminar» del
// Explorador: nunca la borra para siempre sin preguntar (FYS-03, decidido por el dueño).
func aPapelera(ruta string) error {
	return enviarAPapelera(ruta, func(op *operacionShell) uintptr {
		r, _, _ := procOperacionShell.Call(uintptr(unsafe.Pointer(op)))
		return r
	})
}

// locales son los marcadores y ficheros que hay en la carpeta del disco.
func locales(ruta string) ([]Local, error) {
	entradas, err := os.ReadDir(ruta)
	if err != nil {
		return nil, err
	}
	out := make([]Local, 0, len(entradas))
	for _, e := range entradas {
		l := Local{Nombre: e.Name(), Carpeta: e.IsDir()}
		if info, err := e.Info(); err == nil {
			if a, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
				l.Modificado = int64(a.LastWriteTime.HighDateTime)<<32 | int64(a.LastWriteTime.LowDateTime)
				l.Tamano = int64(a.FileSizeHigh)<<32 | int64(a.FileSizeLow)
				l.SinBajar = a.FileAttributes&atributoSinBajar != 0
			}
		}
		if h, err := abrir(filepath.Join(ruta, e.Name()), windows.FILE_READ_ATTRIBUTES); err == nil {
			l.Identidad, l.EnSync, _ = marcadorDe(h)
			_ = windows.CloseHandle(h)
		}
		out = append(out, l)
	}
	return out, nil
}

// crearMarcadores pone en la carpeta lo nuevo de Drive.
func crearMarcadores(ruta string, nuevas []Entrada) {
	marcas, vivos := marcadores(nuevas)
	if len(marcas) == 0 {
		return
	}
	base, err := windows.UTF16PtrFromString(ruta)
	if err != nil {
		return
	}
	var hechos uint32
	r, _, _ := procCrear.Call(uintptr(unsafe.Pointer(base)), uintptr(unsafe.Pointer(&marcas[0])), uintptr(len(marcas)), 0,
		uintptr(unsafe.Pointer(&hechos)))
	keepAlive(vivos, marcas)
	if e := hr(r, "CfCreatePlaceholders"); e != nil {
		slog.Warn("nube: Windows no ha aceptado lo nuevo de Drive", "ruta", ruta, "hechos", hechos, "error", e)
	}
	// Dentro de una carpeta fijada, lo nuevo se baja también.
	go aplicar(ruta)
}

// vaciar deja un fichero con la versión nueva de Drive y sin contenido, si el disco no
// tiene nada sin subir; si estaba fijado, se vuelve a bajar.
func vaciar(ruta string, e Entrada) {
	h, err := abrir(ruta, windows.FILE_READ_ATTRIBUTES|windows.WRITE_DAC)
	if err != nil {
		return
	}
	defer func() { _ = windows.CloseHandle(h) }()
	t := hora(e.Modificado)
	if actualizar(h, ruta, e.Identidad, &metadatos{escrito: t, cambiado: t, tamano: e.Tamano}, actualizarVaciar) {
		go aplicar(ruta)
	}
}

// marcarSincronizado le devuelve el «sincronizado» a un marcador que no ha cambiado de
// contenido.
func marcarSincronizado(ruta string) {
	h, err := abrir(ruta, windows.FILE_READ_ATTRIBUTES|windows.WRITE_DAC)
	if err != nil {
		return
	}
	defer func() { _ = windows.CloseHandle(h) }()
	if identidad, _, err := marcadorDe(h); err == nil {
		actualizar(h, ruta, identidad, nil, actualizarEnSync)
	}
}
