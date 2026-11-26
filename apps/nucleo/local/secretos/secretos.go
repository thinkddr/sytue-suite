// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package secretos guarda las claves de API y los tokens de las cuentas de Sytue Office.
//
// Van al llavero del sistema con el servicio `eu.sytue.office` (distinto del de la
// edición conectada). Donde no hay llavero —un Linux sin Secret Service, por ejemplo— van
// a ficheros 0600 en una carpeta 0700 de los ajustes, y [Almacen.EnLlavero] lo dice para
// que Ajustes lo enseñe (R-CLARO). Nunca se escriben en los JSON de ajustes ni en el log.
package secretos

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
)

// Almacen guarda secretos por nombre.
type Almacen struct {
	servicio string
	reserva  string

	mu       sync.Mutex
	llavero  bool
	probado  bool
	sinLlave bool // para pruebas: no tocar el llavero real
}

// Nuevo guarda en el llavero servicio, con reserva en la carpeta reserva.
func Nuevo(servicio, reserva string) *Almacen {
	return &Almacen{servicio: servicio, reserva: reserva}
}

// SoloFichero no usa el llavero (pruebas, o si la persona lo prefiere).
func SoloFichero(reserva string) *Almacen {
	return &Almacen{reserva: reserva, sinLlave: true, probado: true}
}

func (a *Almacen) hayLlavero() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.probado {
		return a.llavero
	}
	a.probado = true
	// Se prueba una vez: escribir y borrar una entrada de prueba.
	if err := keyring.Set(a.servicio, "prueba", "1"); err == nil {
		_ = keyring.Delete(a.servicio, "prueba")
		a.llavero = true
	}
	return a.llavero
}

// Servicio es el del llavero del sistema donde van los secretos (vacío con SoloFichero).
func (a *Almacen) Servicio() string { return a.servicio }

// Reserva es la carpeta de los secretos donde no hay llavero del sistema.
func (a *Almacen) Reserva() string { return a.reserva }

// EnLlavero dice si los secretos van al llavero del sistema (o a ficheros 0600).
func (a *Almacen) EnLlavero() bool { return !a.sinLlave && a.hayLlavero() }

func (a *Almacen) fichero(nombre string) string {
	sum := sha256.Sum256([]byte(nombre))
	return filepath.Join(a.reserva, hex.EncodeToString(sum[:12]))
}

// Leer devuelve el secreto nombre, o "" si no hay.
func (a *Almacen) Leer(nombre string) (string, error) {
	if a.EnLlavero() {
		v, err := keyring.Get(a.servicio, nombre)
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return v, err
	}
	raw, err := os.ReadFile(a.fichero(nombre))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(raw), err
}

// Guardar escribe el secreto nombre; vacío lo borra.
func (a *Almacen) Guardar(nombre, valor string) error {
	if valor == "" {
		return a.Borrar(nombre)
	}
	if a.EnLlavero() {
		return keyring.Set(a.servicio, nombre, valor)
	}
	if err := os.MkdirAll(a.reserva, 0o700); err != nil {
		return err
	}
	if err := biblioteca.EscribirAtomico(a.fichero(nombre), []byte(valor)); err != nil {
		return err
	}
	return os.Chmod(a.fichero(nombre), 0o600)
}

// Borrar quita el secreto nombre.
func (a *Almacen) Borrar(nombre string) error {
	if a.EnLlavero() {
		err := keyring.Delete(a.servicio, nombre)
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return err
	}
	err := os.Remove(a.fichero(nombre))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
