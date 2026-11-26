// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts_test

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/internal/libretaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen/cargaprueba"
)

// # Prueba de carga del almacén: Contacts (PLAN-EDICION-LOCAL §5.6, R-MEDIR)
//
// 20.000 contactos realistas (libretaprueba, la misma libreta de rapidez_test.go) en el
// almacén en claro: apertura en frío en un proceso nuevo (preparar el motor y la
// primera pantalla: las libretas y la primera página de la libreta) y su memoria, y
// buscar con el repositorio (lo que usan las sugerencias de Mail y el Hub; la lista de
// la pantalla filtra lo que ya tiene cargado). Las cifras están en SYTUE-OFFICE.md:
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'CargaContactos' -v -timeout 0 ./contacts/

// TestCargaContactos mide 20.000 contactos.
func TestCargaContactos(t *testing.T) {
	if dir, hija := cargaprueba.Hija(); hija {
		cargaprueba.Responder(t, func(r *cargaprueba.Reloj) (func(), error) { return abrirContactosMidiendo(dir, r) })
		return
	}
	cargaprueba.Activa(t)
	dir := cargaprueba.Plantilla(t, "contactos-20000", func(dir string) error {
		return cargaprueba.EnMemoria(dir, func(dir string) error {
			_, err := libretaprueba.Escribir(dir, "instalacion-1", 20_000)
			return err
		})
	})
	veces := cargaprueba.Veces()
	frio := cargaprueba.EnFrio(t, dir, veces)
	abrir := cargaprueba.Suma(frio, "preparar", "primera pantalla")
	heap := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.HeapInuse })
	rss := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })

	// Buscar, con la libreta ya abierta: un apellido frecuente, un nombre, un trozo de
	// correo, una empresa y algo que no está.
	repo := abrirEn(t, cargaprueba.Copia(t, dir))
	ctx := t.Context()
	if _, err := repo.AddressBooks(ctx); err != nil {
		t.Fatal(err)
	}
	consultas := []string{"garcía", "lucía", "gmail", "bodegas", "zzz-nadie"}
	buscar := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		_, err := repo.Search(ctx, consultas[i%len(consultas)], 50)
		return err
	})
	cargaprueba.Informar(t,
		cargaprueba.Fila{Carga: "20.000 contactos", Medida: "abrir en frío (motor + primera pantalla)", Valor: abrir.String(), Umbral: "< 2 s", Cumple: abrir.Mediana() < 2000},
		cargaprueba.Fila{Carga: "20.000 contactos", Medida: "· preparar el motor", Valor: cargaprueba.Tiempo(frio, "preparar").String()},
		cargaprueba.Fila{Carga: "20.000 contactos", Medida: "· primera pantalla", Valor: cargaprueba.Tiempo(frio, "primera pantalla").String()},
		cargaprueba.Fila{Carga: "20.000 contactos", Medida: "memoria: montón en uso / pico RSS", Valor: cargaprueba.MiB(heap) + " / " + cargaprueba.MiB(rss)},
		cargaprueba.Fila{Carga: "20.000 contactos", Medida: "buscar (repositorio, 50 resultados)", Valor: buscar.String(), Umbral: "< 100 ms", Cumple: buscar.Mediana() < 100},
	)
}

// abrirContactosMidiendo es lo que hace `cmd-local/contacts` al arrancar, cronometrado:
// el almacén, el repositorio y su catálogo, y las libretas con la primera página.
func abrirContactosMidiendo(dir string, r *cargaprueba.Reloj) (func(), error) {
	ctx := context.Background()
	var (
		rec  *almacen.Recursos
		repo *contacts.Repository
	)
	err := r.Medir("preparar", func() error {
		var err error
		if rec, err = almacen.Abrir(ctx, dir, "contacts", "instalacion-1"); err != nil {
			return err
		}
		if repo, err = contacts.New(rec.Config); err != nil {
			return err
		}
		return almacen.Preparar(ctx, repo)
	})
	if err != nil {
		return nil, err
	}
	cerrar := func() { repo.Close(); rec.Cerrar() }
	err = r.Medir("primera pantalla", func() error {
		libros, err := repo.AddressBooks(ctx)
		if err != nil {
			return err
		}
		if len(libros) == 0 {
			return errors.New("sin libretas")
		}
		_, err = repo.InBookPage(ctx, libros[0].ID, "")
		return err
	})
	if err != nil {
		cerrar()
		return nil, err
	}
	return cerrar, nil
}
