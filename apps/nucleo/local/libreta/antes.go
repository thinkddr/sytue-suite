// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// # Borrar lo de antes de la migración
//
// Pasada la libreta a la base, el almacén de registros de antes se queda en la carpeta,
// sin tocar (migrar.go), hasta que la persona lo borre desde Ajustes → Datos de Sytue
// Office. Desde la marca nadie lo lee ni lo escribe (Contacts, Mail, Calendar y el Hub
// leen la base), así que se borra entero: su diario, `objetos/` (la libreta no guarda
// ficheros: todo está en la base) y lo demás del almacén (`almacen.Restos`). La base y lo
// que no es del almacén (`agenda/`, el estado de las cuentas de CardDAV, Google y
// Microsoft) se quedan. Solo con la migración hecha y comprobada (la marca, con la huella
// de las filas que se comprobó) y con nadie usando el almacén (su cerrojo exclusivo).

// ErrEnUso: alguien tiene abierto el almacén de antes (una copia de Contacts de antes);
// no se borra nada.
var ErrEnUso = errors.New("libreta: el almacén de antes está abierto en otra app")

// MedirAntes dice si la libreta de dir se pasó del almacén de antes (y se comprobó) y
// cuánto ocupa lo que queda de él: lo que BorrarAntes quitaría. Sin migración, Migrada es
// falso y no se mide nada. Solo lee.
func MedirAntes(ctx context.Context, dir string) (almacen.Antes, error) {
	ok, err := migradaDeRegistros(ctx, dir)
	if err != nil || !ok {
		return almacen.Antes{}, err
	}
	a := almacen.Antes{Migrada: true}
	rutas, err := rutasDeAntes(dir)
	if err != nil {
		return a, err
	}
	a.Ficheros, a.Bytes, err = almacen.Medir(rutas...)
	return a, err
}

// BorrarAntes borra de la carpeta dir lo que queda del almacén de antes (ver arriba) y
// dice cuánto se fue. Sin la migración comprobada, ErrSinMigrar; con el almacén abierto
// en otra app, ErrEnUso; y en los dos casos no toca nada. Si algo no se puede borrar
// sigue con lo demás y lo dice; lo que quede se puede borrar otra vez.
func BorrarAntes(ctx context.Context, dir string) (almacen.Antes, error) {
	ok, err := migradaDeRegistros(ctx, dir)
	if err != nil {
		return almacen.Antes{}, err
	}
	if !ok {
		return almacen.Antes{}, ErrSinMigrar
	}
	hecho := almacen.Antes{Migrada: true}
	rutas, err := rutasDeAntes(dir)
	if err != nil || len(rutas) == 0 {
		return hecho, err
	}
	soltar, err := almacen.CerrojoExclusivo(dir)
	if err != nil {
		return hecho, ErrEnUso
	}
	hecho.Ficheros, hecho.Bytes, err = almacen.Borrar(rutas...)
	soltar()
	almacen.QuitarCerrojo(dir)
	return hecho, err
}

// migradaDeRegistros dice si la libreta de dir tiene la marca de haber pasado a ella un
// almacén de registros, con la huella de lo que se comprobó.
func migradaDeRegistros(ctx context.Context, dir string) (bool, error) {
	x, err := AbrirLectura(ctx, dir, "lector")
	if errors.Is(err, ErrSinMigrar) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = x.Cerrar() }()
	inf, ok, err := x.Migracion(ctx)
	if err != nil || !ok {
		return false, err
	}
	return inf.Origen == "registros" && inf.Huella != "", nil
}

// rutasDeAntes son, en dir, las del almacén de antes: lo suyo (almacen.Restos) y
// objetos/, que la libreta no usa.
func rutasDeAntes(dir string) ([]string, error) {
	rutas, err := almacen.Restos(dir)
	if err != nil {
		return nil, err
	}
	objetos := filepath.Join(dir, "objetos")
	if _, err := os.Lstat(objetos); err == nil {
		rutas = append(rutas, objetos)
	}
	return rutas, nil
}
