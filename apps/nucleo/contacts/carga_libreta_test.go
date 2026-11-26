// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/internal/libretaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts/lector"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen/cargaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

// # Prueba de carga: la libreta SQLite de Contacts (plan B de §5.6)
//
// Los mismos 20.000 contactos de TestCargaContactos (libretaprueba), pasados del almacén
// de registros a la libreta SQLite de Sytue Office (`local/libreta`) como lo hace
// Contacts al abrirse la primera vez. Se mide lo que no cumplía con el motor de
// registros, y lo demás que pesa:
//
//   - pasar la libreta a la base (una vez por instalación);
//   - abrir en frío en un proceso nuevo (la base y la primera pantalla: las libretas y la
//     primera página) y su memoria, y la libreta entera como la carga la pantalla
//     (InBook, de cien en cien);
//   - buscar, dar de alta y editar con 20.000, e importar un CSV de 2.000 filas;
//   - lo que lee otra app (Mail, Calendar): autocompletar en una libreta recién abierta y
//     justo después de que Contacts escriba (antes había que releerla entera), y los
//     cumpleaños.
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'CargaLibretaSQLite' -v -timeout 0 ./contacts/

const sujetoCarga = "instalacion-1"

// TestCargaLibretaSQLite mide la libreta SQLite con 20.000 contactos.
func TestCargaLibretaSQLite(t *testing.T) {
	if dir, hija := cargaprueba.Hija(); hija {
		cargaprueba.Responder(t, func(r *cargaprueba.Reloj) (func(), error) { return abrirLibretaMidiendo(dir, r) })
		return
	}
	cargaprueba.Activa(t)
	viejo := cargaprueba.Plantilla(t, "contactos-20000", func(dir string) error {
		return cargaprueba.EnMemoria(dir, func(dir string) error {
			_, err := libretaprueba.Escribir(dir, sujetoCarga, 20_000)
			return err
		})
	})
	veces := cargaprueba.Veces()
	ctx := t.Context()

	// Pasar a la base: cada vez sobre una copia del almacén de antes.
	var copias []string
	migrar := cargaprueba.Repetir(t, min(veces, 3), func(int) { copias = append(copias, cargaprueba.Copia(t, viejo)) }, func(i int) error {
		x, err := libreta.Abrir(ctx, copias[i], sujetoCarga)
		if err != nil {
			return err
		}
		return x.Cerrar()
	})
	dir := copias[0]

	frio := cargaprueba.EnFrio(t, dir, veces)
	abrir := cargaprueba.Suma(frio, "abrir", "primera pantalla")
	heap := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.HeapInuse })
	rss := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })

	x, err := libreta.Abrir(ctx, dir, sujetoCarga)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Cerrar()
	libros, err := x.AddressBooks(ctx)
	if err != nil || len(libros) == 0 {
		t.Fatal(libros, err)
	}
	libro := libros[0].ID
	fachada, err := api.NewContacts("", nil, nil).WithPrivateData(x)
	if err != nil {
		t.Fatal(err)
	}
	entera := cargaprueba.Repetir(t, veces, nil, func(int) error {
		cs, err := fachada.InBook(ctx, libro)
		if err == nil && len(cs) != 20_000 {
			err = fmt.Errorf("la libreta entera son %d", len(cs))
		}
		return err
	})
	consultas := []string{"garcía", "lucía", "gmail", "bodegas", "zzz-nadie"}
	buscar := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		_, err := x.Search(ctx, consultas[i%len(consultas)], 50)
		return err
	})
	nada := cargaprueba.Repetir(t, 2*veces, nil, func(int) error {
		cs, err := x.Search(ctx, "zzz-nadie", 50)
		if err == nil && len(cs) != 0 {
			err = errors.New("encontró a nadie")
		}
		return err
	})
	r := rand.New(rand.NewPCG(7, 20_000)) //nolint:gosec // G404: datos de prueba repetibles
	var nuevos []api.Contact
	alta := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		c, err := x.Create(ctx, libro, entradaDe(libretaprueba.Realista(r, 20_000+i)))
		nuevos = append(nuevos, c)
		return err
	})
	editar := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		in := nuevos[i].Input()
		in.Note = "Editado"
		_, err := x.Update(ctx, nuevos[i].ID, in, nuevos[i].ETag)
		return err
	})
	csv := csvDe(t, 2_000)
	importar := cargaprueba.Repetir(t, min(veces, 3), nil, func(i int) error {
		vacia, err := x.CreateAddressBook(ctx, fmt.Sprintf("Importada %d", i))
		if err != nil {
			return err
		}
		hechos, saltados, err := x.ImportCSV(ctx, vacia.ID, csv)
		if err == nil && (hechos != 2_000 || saltados != 0) {
			err = fmt.Errorf("importados %d, saltados %d", hechos, saltados)
		}
		return err
	})

	// Otra app (Mail, Calendar) con Contacts abierta y escribiendo.
	primera := cargaprueba.Repetir(t, veces, nil, func(int) error {
		l := lector.Nuevo(dir, sujetoCarga)
		defer l.Cerrar()
		_, err := l.Libreta().Buscar(ctx, "lucía", 8)
		return err
	})
	l := lector.Nuevo(dir, sujetoCarga)
	defer l.Cerrar()
	mail := l.Libreta()
	if _, err := mail.Buscar(ctx, "lucía", 8); err != nil {
		t.Fatal(err)
	}
	trasCambio := cargaprueba.Repetir(t, 4*veces, func(i int) {
		if _, err := x.Create(ctx, libro, api.ContactInput{FormattedName: fmt.Sprintf("Recién Llegada %d", i), Emails: []api.ContactEmail{{Email: fmt.Sprintf("recien%d@ejemplo.es", i)}}}); err != nil {
			t.Fatal(err)
		}
	}, func(i int) error {
		ds, err := mail.Buscar(ctx, fmt.Sprintf("recien%d@", i), 8)
		if err == nil && len(ds) != 1 {
			err = fmt.Errorf("lo recién escrito no sale: %+v", ds)
		}
		return err
	})
	cumples := cargaprueba.Repetir(t, veces, nil, func(int) error {
		_, err := l.Cumpleanos(ctx)
		return err
	})

	const carga = "20.000 contactos (SQLite)"
	cargaprueba.Informar(t,
		cargaprueba.Fila{Carga: carga, Medida: "pasar el almacén de registros a la base (una vez)", Valor: migrar.String()},
		cargaprueba.Fila{Carga: carga, Medida: "abrir en frío (base + primera pantalla)", Valor: abrir.String(), Umbral: "< 2 s", Cumple: abrir.Mediana() < 2000},
		cargaprueba.Fila{Carga: carga, Medida: "· abrir la base", Valor: cargaprueba.Tiempo(frio, "abrir").String()},
		cargaprueba.Fila{Carga: carga, Medida: "· primera pantalla", Valor: cargaprueba.Tiempo(frio, "primera pantalla").String()},
		cargaprueba.Fila{Carga: carga, Medida: "memoria: montón en uso / pico RSS", Valor: cargaprueba.MiB(heap) + " / " + cargaprueba.MiB(rss)},
		cargaprueba.Fila{Carga: carga, Medida: "la libreta entera en la pantalla (InBook)", Valor: entera.String()},
		cargaprueba.Fila{Carga: carga, Medida: "buscar (50 resultados)", Valor: buscar.String(), Umbral: "< 100 ms", Cumple: buscar.Mediana() < 100},
		cargaprueba.Fila{Carga: carga, Medida: "buscar lo que no está (recorre todos)", Valor: nada.String(), Umbral: "< 100 ms", Cumple: nada.Mediana() < 100},
		cargaprueba.Fila{Carga: carga, Medida: "dar de alta un contacto", Valor: alta.String()},
		cargaprueba.Fila{Carga: carga, Medida: "editar un contacto", Valor: editar.String()},
		cargaprueba.Fila{Carga: carga, Medida: "importar un CSV de 2.000 filas", Valor: importar.String()},
		cargaprueba.Fila{Carga: carga, Medida: "Mail: autocompletar recién abierta", Valor: primera.String()},
		cargaprueba.Fila{Carga: carga, Medida: "Mail: autocompletar tras escribir Contacts", Valor: trasCambio.String()},
		cargaprueba.Fila{Carga: carga, Medida: "Calendar: los cumpleaños", Valor: cumples.String()},
	)
}

// abrirLibretaMidiendo es lo que hace `cmd-local/contacts` al arrancar, cronometrado: la
// libreta y la primera pantalla (las libretas y la primera página de la primera).
func abrirLibretaMidiendo(dir string, r *cargaprueba.Reloj) (func(), error) {
	ctx := context.Background()
	var x *libreta.Libreta
	err := r.Medir("abrir", func() error {
		var err error
		x, err = libreta.Abrir(ctx, dir, sujetoCarga)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = r.Medir("primera pantalla", func() error {
		libros, err := x.AddressBooks(ctx)
		if err != nil {
			return err
		}
		if len(libros) == 0 {
			return errors.New("sin libretas")
		}
		_, err = x.InBookPage(ctx, libros[0].ID, "")
		return err
	})
	if err != nil {
		_ = x.Cerrar()
		return nil, err
	}
	return func() { _ = x.Cerrar() }, nil
}
