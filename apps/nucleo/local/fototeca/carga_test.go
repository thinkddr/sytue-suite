// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen/cargaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/contrato"
)

// # Prueba de carga de la fototeca de SQLite (PLAN-EDICION-LOCAL §5.6, R-MEDIR)
//
// La misma carga que la del almacén de registros (`photos/carga_test.go`): 50.000 fotos
// en 200 álbumes, importadas con ImportarFoto como `cmd-local/photos` (original copiado y
// sus tres derivadas), aquí con su fecha de captura repartida en diez años y la posición
// en una de cada diez. Se mide lo que hace Photos al arrancar —abrir la base y la primera
// página de la línea de tiempo, y los años y los recuerdos que pide la galería— en frío
// en un proceso nuevo, con su memoria; y con la fototeca abierta, una página, buscar, la
// lista de álbumes, las fotos de un álbum, importar una foto en disco y vaciar la
// papelera de una. SYTUE_CARGA_FOTOS cambia el número de fotos (múltiplo de 1.000).
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'CargaFototeca' -v -timeout 0 ./local/fototeca/

const (
	fotosPorAlbum = 250
	sujetoCarga   = "instalacion-carga"
)

func fotosDeCarga(t *testing.T) int {
	t.Helper()
	n := 50_000
	if v := os.Getenv("SYTUE_CARGA_FOTOS"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n <= 0 || n%1000 != 0 {
			t.Fatalf("SYTUE_CARGA_FOTOS=%q: un múltiplo de 1000", v)
		}
	}
	return n
}

// TestCargaFototeca mide la fototeca grande.
func TestCargaFototeca(t *testing.T) {
	if dir, hija := cargaprueba.Hija(); hija {
		cargaprueba.Responder(t, func(r *cargaprueba.Reloj) (func(), error) { return abrirMidiendo(dir, r) })
		return
	}
	cargaprueba.Activa(t)
	n := fotosDeCarga(t)
	carga := fmt.Sprintf("%d fotos, %d álbumes (SQLite)", n, n/fotosPorAlbum)
	dir := cargaprueba.Plantilla(t, fmt.Sprintf("fototeca-%d", n), func(dir string) error {
		return cargaprueba.EnMemoria(dir, func(d string) error { return generar(d, n) })
	})
	veces := cargaprueba.Veces()
	frio := cargaprueba.EnFrio(t, dir, veces)
	abrir := cargaprueba.Suma(frio, "abrir", "primera página")
	arrancar := cargaprueba.Suma(frio, "abrir", "primera página", "años", "recuerdos")
	heap := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.HeapInuse })
	rss := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })

	ctx := t.Context()
	copia := cargaprueba.Copia(t, dir)
	f, err := fototeca.Abrir(ctx, copia, sujetoCarga, fototeca.Opciones{})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fotos, cursor, err := f.Timeline(ctx, "", 100, false, "")
	if err != nil || len(fotos) != 100 {
		t.Fatal(len(fotos), err)
	}
	pagina := cargaprueba.Repetir(t, 4*veces, nil, func(int) error {
		var err error
		fotos, cursor, err = f.Timeline(ctx, cursor, 100, false, "")
		if err == nil && len(fotos) != 100 {
			err = fmt.Errorf("página de %d", len(fotos))
		}
		return err
	})
	buscar := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		// Un nombre que solo lleva una foto (la galería entera hay que mirarla).
		got, _, err := f.Timeline(ctx, "", 60, false, fmt.Sprintf("IMG_%05d", 1+i*997%n))
		if err == nil && len(got) != 1 {
			err = fmt.Errorf("buscar dio %d", len(got))
		}
		return err
	})
	repetidas := cargaprueba.Repetir(t, 4*veces, nil, func(int) error {
		_, err := f.Duplicates(ctx, 50)
		return err
	})
	conLugar := fotos[0]
	for _, x := range fotos {
		if x.Location != nil {
			conLugar = x
			break
		}
	}
	porAqui := cargaprueba.Repetir(t, 4*veces, nil, func(int) error {
		_, err := f.Nearby(ctx, conLugar.ID, 48)
		return err
	})
	albumes, err := f.Albums(ctx)
	if err != nil || len(albumes) != n/fotosPorAlbum {
		t.Fatal(len(albumes), err)
	}
	listaAlbumes := cargaprueba.Repetir(t, 4*veces, nil, func(int) error {
		_, err := f.Albums(ctx)
		return err
	})
	grande := albumes[0]
	for _, a := range albumes {
		if a.Count > grande.Count {
			grande = a
		}
	}
	enAlbum := cargaprueba.Repetir(t, 4*veces, nil, func(int) error {
		got, err := f.AlbumPhotos(ctx, grande.ID)
		if err == nil && len(got) != grande.Count {
			err = fmt.Errorf("el álbum dio %d de %d", len(got), grande.Count)
		}
		return err
	})
	base := contrato.JPEG(64, 48, 99)
	importar := cargaprueba.Repetir(t, veces, nil, func(i int) error {
		datos := binary.BigEndian.AppendUint64(bytes.Clone(base), uint64(n+i))
		_, err := f.ImportarFoto(ctx, uuid.NewString(), "nueva.jpg", "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
		return err
	})
	purgar := cargaprueba.Repetir(t, veces, func(i int) {
		if err := f.Trash(ctx, fotos[i].ID); err != nil {
			t.Fatal(err)
		}
	}, func(i int) error {
		_, err := f.Purgar(ctx, fotos[i].ID)
		return err
	})
	cargaprueba.Informar(t,
		cargaprueba.Fila{Carga: carga, Medida: "abrir en frío (base + primera página)", Valor: abrir.String(), Umbral: "< 4 s", Cumple: abrir.Mediana() < 4000},
		cargaprueba.Fila{Carga: carga, Medida: "· abrir la base", Valor: cargaprueba.Tiempo(frio, "abrir").String()},
		cargaprueba.Fila{Carga: carga, Medida: "· primera página", Valor: cargaprueba.Tiempo(frio, "primera página").String()},
		cargaprueba.Fila{Carga: carga, Medida: "· y los años y los recuerdos (arrancar la galería)", Valor: arrancar.String()},
		cargaprueba.Fila{Carga: carga, Medida: "memoria: montón en uso / pico RSS", Valor: cargaprueba.MiB(heap) + " / " + cargaprueba.MiB(rss), Umbral: "< 400 MB", Cumple: rss.Mediana()*(1<<20) < 400e6},
		cargaprueba.Fila{Carga: carga, Medida: "una página de la línea de tiempo (100)", Valor: pagina.String(), Umbral: "< 100 ms", Cumple: pagina.Mediana() < 100},
		cargaprueba.Fila{Carga: carga, Medida: "buscar por nombre (una entre todas)", Valor: buscar.String()},
		cargaprueba.Fila{Carga: carga, Medida: "repetidas", Valor: repetidas.String()},
		cargaprueba.Fila{Carga: carga, Medida: "«Por aquí»", Valor: porAqui.String()},
		cargaprueba.Fila{Carga: carga, Medida: "lista de álbumes", Valor: listaAlbumes.String()},
		cargaprueba.Fila{Carga: carga, Medida: fmt.Sprintf("las fotos de un álbum (%d)", grande.Count), Valor: enAlbum.String()},
		cargaprueba.Fila{Carga: carga, Medida: "importar una foto (en disco)", Valor: importar.String()},
		cargaprueba.Fila{Carga: carga, Medida: "borrar una para siempre", Valor: purgar.String()},
	)
}

// abrirMidiendo es lo que hace `cmd-local/photos` al arrancar, cronometrado: abrir la
// fototeca, la primera página y lo que pide la galería al lado (años y recuerdos).
func abrirMidiendo(dir string, r *cargaprueba.Reloj) (func(), error) {
	ctx := context.Background()
	var f *fototeca.Fototeca
	err := r.Medir("abrir", func() error {
		var err error
		f, err = fototeca.Abrir(ctx, dir, sujetoCarga, fototeca.Opciones{Formatos: []string{"webp", "avif", "bmp"}})
		return err
	})
	if err != nil {
		return nil, err
	}
	pasos := []struct {
		nombre string
		hacer  func() error
	}{
		{"primera página", func() error { _, _, err := f.Timeline(ctx, "", 100, false, ""); return err }},
		{"años", func() error { _, err := f.TimelineYears(ctx, false); return err }},
		{"recuerdos", func() error { _, err := f.Memories(ctx); return err }},
	}
	for _, p := range pasos {
		if err := r.Medir(p.nombre, p.hacer); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f.Close, nil
}

// generar importa n fotos distintas, con fechas en diez años, y n/250 álbumes de 25 a 250
// fotos (como la carga del almacén de registros).
func generar(dir string, n int) error {
	ctx := context.Background()
	f, err := fototeca.Abrir(ctx, dir, sujetoCarga, fototeca.Opciones{})
	if err != nil {
		return err
	}
	defer f.Close()
	base := contrato.JPEG(64, 48, 120)
	desde := time.Date(2016, 1, 1, 9, 0, 0, 0, time.UTC)
	ids := make([]string, n)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		primero error
	)
	cola := make(chan int)
	for range 2 {
		wg.Go(func() {
			for i := range cola {
				var lugar *contrato.Lugar
				if i%10 == 0 {
					lugar = &contrato.Lugar{Lat: 40 + float64(i%997)/1000, Lon: -3 - float64(i%991)/1000}
				}
				tomada := desde.Add(time.Duration(i) * (10 * 365 * 24 * time.Hour / time.Duration(n)))
				datos := binary.BigEndian.AppendUint64(contrato.ConExif(base, tomada, 1, lugar), uint64(i))
				p, err := f.ImportarFoto(ctx, uuid.NewString(), fmt.Sprintf("IMG_%05d.jpg", i+1), "image/jpeg", bytes.NewReader(datos), int64(len(datos)))
				mu.Lock()
				if err != nil {
					primero = errors.Join(primero, fmt.Errorf("foto %d: %w", i, err))
				}
				ids[i] = p.ID
				mu.Unlock()
			}
		})
	}
	for i := range n {
		cola <- i
	}
	close(cola)
	wg.Wait()
	if primero != nil {
		return primero
	}
	r := rand.New(rand.NewPCG(2026, 11)) //nolint:gosec // G404: datos de prueba repetibles
	for a := range n / fotosPorAlbum {
		album, err := f.CreateAlbum(ctx, fmt.Sprintf("Álbum %d", a+1))
		if err != nil {
			return err
		}
		tanda := ids[(a/4)*1000 : (a/4+1)*1000]
		miembros := make([]string, 0, fotosPorAlbum)
		for _, k := range r.Perm(len(tanda))[:25+r.IntN(fotosPorAlbum-25+1)] {
			miembros = append(miembros, tanda[k])
		}
		if _, err := f.AddToAlbum(ctx, album.ID, miembros); err != nil {
			return err
		}
	}
	return comprobar(ctx, f, n)
}

// comprobar cuenta las fotos y los álbumes de la carga.
func comprobar(ctx context.Context, f *fototeca.Fototeca, n int) error {
	anyos, err := f.TimelineYears(ctx, false)
	if err != nil {
		return err
	}
	total := 0
	for _, y := range anyos {
		total += y.Count
	}
	albumes, err := f.Albums(ctx)
	if err != nil {
		return err
	}
	if total != n || len(albumes) != n/fotosPorAlbum || len(anyos) < 10 {
		return fmt.Errorf("la carga tiene %d fotos en %d años y %d álbumes", total, len(anyos), len(albumes))
	}
	for _, a := range albumes {
		if a.Count < 25 {
			return fmt.Errorf("el álbum %s tiene %d fotos", a.Title, a.Count)
		}
	}
	return nil
}
