// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// # Borrar lo de antes de la migración
//
// Migrada la fototeca, lo del almacén de registros se queda en su carpeta (migrar.go)
// hasta que la persona lo borre desde Ajustes → Datos de Sytue Office: el diario
// conservado (`registros-antes-de-sqlite*.jsonl`), lo demás del almacén
// (`almacen.Restos`: el diario transitorio, su clave, el origen…) y, de `objetos/` —que
// es de las dos—, SOLO los ficheros que ya no cita la fototeca: lo que solo citaba el
// diario de antes (las páginas de los álbumes, las derivadas de recetas viejas, lo que se
// ha purgado desde entonces) y lo que un corte dejó sin citar. La base y sus ficheros,
// nunca.
//
// Solo con la migración hecha y comprobada (su marca, con los recuentos de la
// comprobación), con nadie usando el almacén de antes (su cerrojo exclusivo: ni una copia
// de Photos de antes ni nada que lo lea) y, en `objetos/`, con el cerrojo de escritura de
// la fototeca: qué se borra se decide y se borra dentro de una transacción de escritura,
// así que ningún proceso (Photos abierto, el «Traer» del Hub) puede estar poniendo en su
// sitio ni citando a la vez un fichero que se borra. Los temporales de una importación en
// curso (de los últimos edadParaBarrer) no se tocan.

// ErrSinMigrar: la fototeca de esa carpeta no viene de una migración comprobada del
// almacén de antes; no se borra nada.
var ErrSinMigrar = errors.New("fototeca: la fototeca no se ha pasado del almacén de antes (o no se comprobó): no se borra nada")

// MedirAntes dice si la fototeca de dir se migró del almacén de antes (y se comprobó) y
// cuánto ocupa lo que queda de él: lo que BorrarAntes quitaría. Sin migración, Migrada
// es falso y no se mide nada. Solo lee.
func MedirAntes(ctx context.Context, dir string) (almacen.Antes, error) {
	db, err := abrirParaAntes(dir, true)
	if db == nil || err != nil {
		return almacen.Antes{}, err
	}
	defer func() { _ = db.Close() }()
	if ok, err := migradaComprobada(ctx, db); err != nil || !ok {
		return almacen.Antes{}, err
	}
	a := almacen.Antes{Migrada: true}
	rutas, err := rutasDeAntes(dir)
	if err != nil {
		return a, err
	}
	if a.Ficheros, a.Bytes, err = almacen.Medir(rutas...); err != nil {
		return a, err
	}
	// Borrado ya lo de antes, objetos/ no se vuelve a recorrer (con 50.000 fotos son
	// cientos de miles de ficheros) cada vez que se abren los Ajustes: lo que un corte
	// deje sin citar desde entonces lo barre vaciar la papelera.
	if borrados, err := meta(ctx, db, claveAntesBorrados); err != nil || (borrados != "" && len(rutas) == 0) {
		return a, err
	}
	vivos, err := vivosEn(ctx, db)
	if err != nil {
		return a, err
	}
	n, b, err := (&Fototeca{dir: dir, ahora: time.Now}).barrerCarpeta(ctx, vivos, time.Time{}, false)
	a.Ficheros += n
	a.Bytes += b
	return a, err
}

// BorrarAntes borra de la carpeta dir lo que queda del almacén de antes de la migración
// (ver arriba) y dice cuánto se fue. Sin la migración comprobada, ErrSinMigrar; con el
// almacén de antes abierto en otra app, ErrEnUso; y en los dos casos no toca nada. Si
// algo no se puede borrar sigue con lo demás y lo dice; lo que quede se puede borrar
// otra vez.
func BorrarAntes(ctx context.Context, dir string) (almacen.Antes, error) {
	db, err := abrirParaAntes(dir, false)
	if err != nil {
		return almacen.Antes{}, err
	}
	if db == nil {
		return almacen.Antes{}, ErrSinMigrar
	}
	defer func() { _ = db.Close() }()
	if ok, err := migradaComprobada(ctx, db); err != nil || !ok {
		if err == nil {
			err = ErrSinMigrar
		}
		return almacen.Antes{}, err
	}
	hecho := almacen.Antes{Migrada: true}
	rutas, err := rutasDeAntes(dir)
	if err != nil {
		return hecho, err
	}
	var errRestos error
	if len(rutas) > 0 {
		soltar, err := almacen.CerrojoExclusivo(dir)
		if err != nil {
			return hecho, ErrEnUso
		}
		hecho.Ficheros, hecho.Bytes, errRestos = almacen.Borrar(rutas...)
		soltar()
		almacen.QuitarCerrojo(dir)
	}
	// De objetos/, lo que no cita la fototeca, con su cerrojo de escritura; y si se ha ido
	// todo, se apunta (MedirAntes ya no recorre objetos/).
	f := &Fototeca{db: db, dir: dir, ahora: time.Now}
	errObjetos := f.tx(ctx, func(tx *sql.Tx) error {
		vivos, err := vivosEn(ctx, tx)
		if err != nil {
			return err
		}
		n, b, err := f.barrerCarpeta(ctx, vivos, time.Time{}, true)
		hecho.Ficheros += n
		hecho.Bytes += b
		if err != nil || errRestos != nil {
			return err
		}
		return ponerMeta(ctx, tx, claveAntesBorrados, enMarca(f.ahora()))
	})
	return hecho, errors.Join(errRestos, errObjetos)
}

// claveAntesBorrados es cuándo se borró lo de antes de la migración (BorrarAntes).
const claveAntesBorrados = "antes_borrados"

// abrirParaAntes abre la base de la fototeca de dir sin prepararla ni migrar nada: para
// leer, o para escribir con el cerrojo de escritura (BEGIN IMMEDIATE). nil sin error si
// no hay base.
func abrirParaAntes(dir string, leer bool) (*sql.DB, error) {
	ruta := filepath.Join(dir, Fichero)
	if info, err := os.Stat(ruta); err != nil || !info.Mode().IsRegular() {
		return nil, nil //nolint:nilnil,nilerr // sin base no hay migración
	}
	opciones := "_busy_timeout=10000&_txlock=immediate&_synchronous=FULL"
	if leer {
		opciones = "mode=ro&_busy_timeout=2000"
	}
	db, err := sql.Open("sqlite3", (&url.URL{Scheme: "file", Path: filepath.ToSlash(ruta), RawQuery: opciones}).String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// migradaComprobada dice si la base viene de una migración del almacén de antes que se
// comprobó: la marca con su fecha y los recuentos de la comprobación (una fototeca que
// nació sin almacén lleva «sin almacén» y no los tiene).
func migradaComprobada(ctx context.Context, db *sql.DB) (bool, error) {
	hecha, err := meta(ctx, db, claveMigrada)
	if err != nil {
		return false, fmt.Errorf("fototeca: leer la marca de la migración: %w", err)
	}
	if _, err := deMarca(hecha); err != nil {
		return false, nil //nolint:nilerr // sin migración (o sin almacén que migrar) no es un error
	}
	recuentos, err := meta(ctx, db, claveRecuentos)
	return recuentos != "", err
}

// rutasDeAntes son, en dir, las del almacén de antes que no son objetos/: el diario
// conservado (con sus «-2», «-3»… si se migró más de una vez) y lo demás del almacén.
func rutasDeAntes(dir string) ([]string, error) {
	rutas, err := almacen.Restos(dir)
	if err != nil {
		return nil, err
	}
	conservados, err := filepath.Glob(filepath.Join(dir, "registros-antes-de-sqlite*.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, c := range conservados {
		if base := filepath.Base(c); base == DiarioConservado || strings.HasPrefix(base, "registros-antes-de-sqlite-") {
			rutas = append(rutas, c)
		}
	}
	return rutas, nil
}
