// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen/cargaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/fototeca"
	"git.sytue.eu/sytue/one/apps/nucleo/photos"
)

// # Prueba de carga del almacén: Photos (PLAN-EDICION-LOCAL §5.6, R-MEDIR)
//
// La fototeca de Sytue Office ya no vive aquí sino en SQLite (`local/fototeca`, el plan B
// de §5.6, con su propia prueba de carga): esta mide el repositorio de la conectada sobre
// el almacén de registros, como estaba (el «antes» de la tabla de SYTUE-OFFICE.md), y la
// migración de esa fototeca a SQLite (TestCargaMigrarFotos).
//
// 50.000 fotos en 200 álbumes, importadas con el repositorio de la edición conectada
// (ImportarFoto, como `cmd-local/photos`: original retenido y sus tres derivadas) sobre
// el puerto real en disco: apertura en frío en un proceso nuevo (preparar el motor y la
// primera página de la línea de tiempo) y su memoria, y una página de la línea de
// tiempo con la galería abierta. SYTUE_CARGA_FOTOS cambia el número de fotos (múltiplo
// de 1.000; los álbumes son uno por cada 250). Las cifras están en SYTUE-OFFICE.md:
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'CargaFotos' -v -timeout 0 ./photos/
//
// # Cómo se genera
//
// Importar una foto en disco son unas cuarenta escrituras sincronizadas (150 ms en este
// equipo) y cada escritura del motor vuelve a cargar el catálogo entero, así que
// importar 50.000 seguidas llevaría días. Se importan por TANDAS de 1.000, cada una en un
// almacén propio en memoria (tmpfs), con el repositorio de verdad y sus álbumes; luego
// se juntan en uno: cada tanda es un almacén entero y válido del mismo sujeto, con
// referencias únicas (UUID) y objetos por su huella, y su diario, sin el recibo de la
// importación vacía de la primera apertura, es lo que habría escrito importarlas todas
// en el mismo. Lo comprueba abrir el resultado con el repositorio, que valida el
// catálogo entero, y contar las fotos y los álbumes.

const (
	fotosPorTanda = 1_000
	fotosPorAlbum = 250
	sujetoFotos   = "instalacion-carga"
)

// fotosDeCarga es SYTUE_CARGA_FOTOS, o 50.000.
func fotosDeCarga(t *testing.T) int {
	t.Helper()
	n := 50_000
	if v := os.Getenv("SYTUE_CARGA_FOTOS"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n <= 0 || n%fotosPorTanda != 0 {
			t.Fatalf("SYTUE_CARGA_FOTOS=%q: un múltiplo de %d", v, fotosPorTanda)
		}
	}
	return n
}

// TestCargaFotos mide la fototeca grande.
func TestCargaFotos(t *testing.T) {
	if dir, hija := cargaprueba.Hija(); hija {
		cargaprueba.Responder(t, func(r *cargaprueba.Reloj) (func(), error) { return abrirFotosMidiendo(dir, r) })
		return
	}
	cargaprueba.Activa(t)
	n := fotosDeCarga(t)
	carga := fmt.Sprintf("%d fotos, %d álbumes", n, n/fotosPorAlbum)
	dir := cargaprueba.Plantilla(t, fmt.Sprintf("fotos-%d", n), func(dir string) error { return generarFotos(dir, n) })
	veces := cargaprueba.Veces()
	frio := cargaprueba.EnFrio(t, dir, veces)
	abrir := cargaprueba.Suma(frio, "preparar", "primera página")
	heap := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.HeapInuse })
	rss := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })

	// Con la galería abierta (solo se lee: sobre la carga misma): las páginas siguientes
	// de la línea de tiempo, como al desplazarse, y la lista de álbumes.
	ctx := t.Context()
	repo, cerrar := abrirFotosEn(t, dir)
	defer cerrar()
	fotos, cursor, err := repo.Timeline(ctx, "", 100, false, "")
	if err != nil || len(fotos) != 100 {
		t.Fatal(len(fotos), err)
	}
	pagina := cargaprueba.Repetir(t, 4*veces, nil, func(int) error {
		var err error
		fotos, cursor, err = repo.Timeline(ctx, cursor, 100, false, "")
		if err == nil && len(fotos) != 100 {
			err = fmt.Errorf("página de %d", len(fotos))
		}
		return err
	})
	albumes, err := repo.Albums(ctx)
	if err != nil || len(albumes) != n/fotosPorAlbum {
		t.Fatal(len(albumes), err)
	}
	listaAlbumes := cargaprueba.Repetir(t, 4*veces, nil, func(int) error {
		_, err := repo.Albums(ctx)
		return err
	})
	cargaprueba.Informar(t,
		cargaprueba.Fila{Carga: carga, Medida: "abrir en frío (motor + primera página)", Valor: abrir.String(), Umbral: "< 4 s", Cumple: abrir.Mediana() < 4000},
		cargaprueba.Fila{Carga: carga, Medida: "· preparar el motor", Valor: cargaprueba.Tiempo(frio, "preparar").String()},
		cargaprueba.Fila{Carga: carga, Medida: "· primera página", Valor: cargaprueba.Tiempo(frio, "primera página").String()},
		cargaprueba.Fila{Carga: carga, Medida: "memoria: montón en uso / pico RSS", Valor: cargaprueba.MiB(heap) + " / " + cargaprueba.MiB(rss), Umbral: "< 400 MB", Cumple: rss.Mediana()*(1<<20) < 400e6},
		cargaprueba.Fila{Carga: carga, Medida: "una página de la línea de tiempo (100)", Valor: pagina.String(), Umbral: "< 100 ms", Cumple: pagina.Mediana() < 100},
		cargaprueba.Fila{Carga: carga, Medida: "lista de álbumes", Valor: listaAlbumes.String()},
	)
}

// TestCargaMigrarFotos mide la migración de la fototeca grande del almacén de registros a
// SQLite (`local/fototeca`, el plan B de §5.6), que Photos hace una sola vez, la primera
// vez que se abre: en frío en un proceso nuevo, sobre una copia de la carga (migrar
// aparta el diario), con su memoria. Después, la fototeca migrada se abre como cualquier
// otra (su propia prueba de carga, `local/fototeca/carga_test.go`).
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'CargaMigrarFotos' -v -timeout 0 ./photos/
func TestCargaMigrarFotos(t *testing.T) {
	if dir, hija := cargaprueba.Hija(); hija {
		cargaprueba.Responder(t, func(r *cargaprueba.Reloj) (func(), error) { return migrarMidiendo(t, dir, r) })
		return
	}
	cargaprueba.Activa(t)
	n := fotosDeCarga(t)
	carga := fmt.Sprintf("%d fotos, %d álbumes", n, n/fotosPorAlbum)
	dir := cargaprueba.Plantilla(t, fmt.Sprintf("fotos-%d", n), func(dir string) error { return generarFotos(dir, n) })
	// Cada vez copia la carga entera: con dos basta para la mediana.
	frio := cargaprueba.EnFrio(t, dir, min(cargaprueba.Veces(), 2))
	migrar := cargaprueba.Tiempo(frio, "migrar")
	heap := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.HeapInuse })
	rss := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })
	cargaprueba.Informar(t,
		cargaprueba.Fila{Carga: carga, Medida: "migrar a SQLite (una vez: abrir y validar el almacén, volcar, comprobar)", Valor: migrar.String()},
		cargaprueba.Fila{Carga: carga, Medida: "· y abrir la migrada con su primera página", Valor: cargaprueba.Tiempo(frio, "abrir la migrada").String()},
		cargaprueba.Fila{Carga: carga, Medida: "memoria al migrar: montón en uso / pico RSS", Valor: cargaprueba.MiB(heap) + " / " + cargaprueba.MiB(rss)},
	)
}

// migrarMidiendo copia la carga dir (sin medirlo) y la migra abriendo la fototeca de
// SQLite como Photos; después la abre otra vez, ya migrada.
func migrarMidiendo(t *testing.T, dir string, r *cargaprueba.Reloj) (func(), error) {
	ctx := context.Background()
	copia := filepath.Join(t.TempDir(), "photos")
	if err := cargaprueba.CopiarArbol(dir, copia); err != nil {
		return nil, err
	}
	err := r.Medir("migrar", func() error {
		f, err := fototeca.Abrir(ctx, copia, sujetoFotos, fototeca.Opciones{Formatos: []string{"webp", "avif", "bmp"}})
		if err == nil {
			f.Close()
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	var f *fototeca.Fototeca
	err = r.Medir("abrir la migrada", func() error {
		var err error
		if f, err = fototeca.Abrir(ctx, copia, sujetoFotos, fototeca.Opciones{}); err != nil {
			return err
		}
		_, _, err = f.Timeline(ctx, "", 100, false, "")
		return err
	})
	if err != nil {
		return nil, err
	}
	return f.Close, nil
}

// abrirFotosMidiendo es lo que hace `cmd-local/photos` al arrancar, cronometrado.
func abrirFotosMidiendo(dir string, r *cargaprueba.Reloj) (func(), error) {
	ctx := context.Background()
	var (
		rec  *almacen.Recursos
		repo *photos.Repository
	)
	err := r.Medir("preparar", func() error {
		var err error
		if rec, err = almacen.Abrir(ctx, dir, "photos", sujetoFotos); err != nil {
			return err
		}
		if repo, err = photos.New(rec.Config); err != nil {
			return err
		}
		repo.AdmitirFormatos("webp", "avif", "bmp")
		repo.LiberarAlPurgar()
		return almacen.Preparar(ctx, repo)
	})
	if err != nil {
		return nil, err
	}
	cerrar := func() { repo.Close(); rec.Cerrar() }
	err = r.Medir("primera página", func() error {
		_, _, err := repo.Timeline(ctx, "", 100, false, "")
		return err
	})
	if err != nil {
		cerrar()
		return nil, err
	}
	return cerrar, nil
}

func abrirFotosEn(t *testing.T, dir string) (*photos.Repository, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "photos", sujetoFotos)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := photos.New(rec.Config)
	if err != nil {
		rec.Cerrar()
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		repo.Close()
		rec.Cerrar()
		t.Fatal("preparar:", err)
	}
	return repo, func() { repo.Close(); rec.Cerrar() }
}

// generarFotos importa n fotos por tandas en memoria y las junta en dir (ver arriba).
func generarFotos(dir string, n int) error {
	tmp, quitar, err := cargaprueba.CarpetaEnMemoria()
	if err != nil {
		if tmp, err = os.MkdirTemp("", "sytue-carga-fotos-"); err != nil {
			return err
		}
		quitar = func() { _ = os.RemoveAll(tmp) }
	}
	defer quitar()
	tandas := make([]string, n/fotosPorTanda)
	for i := range tandas {
		tandas[i] = filepath.Join(tmp, strconv.Itoa(i))
	}
	// Dos a la vez: la CPU se comparte.
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		primero error
	)
	cola := make(chan int)
	for range 2 {
		wg.Go(func() {
			for i := range cola {
				if err := generarTanda(tandas[i], i); err != nil {
					mu.Lock()
					primero = errors.Join(primero, fmt.Errorf("tanda %d: %w", i, err))
					mu.Unlock()
				}
			}
		})
	}
	for i := range tandas {
		cola <- i
	}
	close(cola)
	wg.Wait()
	if primero != nil {
		return primero
	}
	if err := juntarTandas(dir, tandas); err != nil {
		return err
	}
	// La prueba de que lo juntado es un almacén válido: el repositorio lo abre (valida el
	// catálogo entero) y están todas las fotos y todos los álbumes.
	return comprobarFototeca(dir, n)
}

// generarTanda importa fotosPorTanda fotos distintas y sus álbumes en un almacén nuevo.
func generarTanda(dir string, tanda int) error {
	ctx := context.Background()
	rec, err := almacen.Abrir(ctx, dir, "photos", sujetoFotos)
	if err != nil {
		return err
	}
	defer rec.Cerrar()
	repo, err := photos.New(rec.Config)
	if err != nil {
		return err
	}
	defer repo.Close()
	if err := almacen.Preparar(ctx, repo); err != nil {
		return err
	}
	base, err := jpegDeCarga()
	if err != nil {
		return err
	}
	ids := make([]string, 0, fotosPorTanda)
	for i := range fotosPorTanda {
		global := tanda*fotosPorTanda + i
		// Cada original es distinto (su huella también); las derivadas salen iguales.
		foto := binary.BigEndian.AppendUint64(bytes.Clone(base), uint64(global))
		p, err := repo.ImportarFoto(ctx, uuid.NewString(), fmt.Sprintf("IMG_%05d.jpg", global+1), "image/jpeg", bytes.NewReader(foto), int64(len(foto)))
		if err != nil {
			return fmt.Errorf("foto %d: %w", global, err)
		}
		ids = append(ids, p.ID)
	}
	r := rand.New(rand.NewPCG(2026, uint64(tanda))) //nolint:gosec // G404: datos de prueba repetibles
	for a := range fotosPorTanda / fotosPorAlbum {
		album, err := repo.CreateAlbum(ctx, fmt.Sprintf("Álbum %d", tanda*fotosPorTanda/fotosPorAlbum+a+1))
		if err != nil {
			return err
		}
		// Álbumes de 25 a 250 fotos de la tanda.
		miembros := make([]string, 0, fotosPorAlbum)
		for _, k := range r.Perm(len(ids))[:25+r.IntN(fotosPorAlbum-25+1)] {
			miembros = append(miembros, ids[k])
		}
		if _, err := repo.AddToAlbum(ctx, album.ID, miembros); err != nil {
			return err
		}
	}
	return nil
}

// jpegDeCarga es una foto pequeña de 64×48.
func jpegDeCarga() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 120, 255})
		}
	}
	var b bytes.Buffer
	err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90})
	return b.Bytes(), err
}

// juntarTandas escribe en destino un almacén con todas las tandas: el diario de cada
// una seguido (sin el recibo de la importación vacía de las demás: el de la primera es
// el del almacén), los objetos de todas y el estado de la primera.
func juntarTandas(destino string, tandas []string) error {
	if err := os.MkdirAll(filepath.Join(destino, "objetos"), 0o700); err != nil {
		return err
	}
	for _, nombre := range []string{"origen.json", "diario", "diario.clave"} {
		if err := cargaprueba.CopiarArbol(filepath.Join(tandas[0], nombre), filepath.Join(destino, nombre)); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(filepath.Join(destino, "registros.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	escritor := bufio.NewWriterSize(f, 1<<20)
	for i, tanda := range tandas {
		if err := volcarDiario(escritor, filepath.Join(tanda, "registros.jsonl"), i > 0); err != nil {
			f.Close()
			return err
		}
		objetos := filepath.Join(tanda, "objetos")
		err := filepath.WalkDir(objetos, func(ruta string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
				return err
			}
			rel, err := filepath.Rel(objetos, ruta)
			if err != nil {
				return err
			}
			return cargaprueba.CopiarFichero(ruta, filepath.Join(destino, "objetos", rel))
		})
		if err != nil {
			f.Close()
			return err
		}
	}
	if err := escritor.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// volcarDiario copia las líneas del diario ruta en w; sinRecibo quita las del recibo de
// la importación (los objetos `photos_snapshot_…`).
func volcarDiario(w io.Writer, ruta string, sinRecibo bool) error {
	f, err := os.Open(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	lector := bufio.NewReaderSize(f, 1<<20)
	for {
		linea, err := lector.ReadBytes('\n')
		if len(linea) > 0 && linea[len(linea)-1] == '\n' {
			var l struct {
				Registro *struct {
					ObjectID string `json:"object_id"`
				} `json:"registro"`
			}
			if json.Unmarshal(linea, &l) != nil {
				return fmt.Errorf("%s: línea ilegible", ruta)
			}
			if !sinRecibo || l.Registro == nil || !strings.HasPrefix(l.Registro.ObjectID, "photos_snapshot_") {
				if _, err := w.Write(linea); err != nil {
					return err
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// comprobarFototeca abre dir con el repositorio y cuenta las fotos y los álbumes.
func comprobarFototeca(dir string, n int) error {
	ctx := context.Background()
	rec, err := almacen.Abrir(ctx, dir, "photos", sujetoFotos)
	if err != nil {
		return err
	}
	defer rec.Cerrar()
	repo, err := photos.New(rec.Config)
	if err != nil {
		return err
	}
	defer repo.Close()
	if err := almacen.Preparar(ctx, repo); err != nil {
		return fmt.Errorf("el almacén juntado no abre: %w", err)
	}
	total, cursor := 0, ""
	for {
		fotos, siguiente, err := repo.Timeline(ctx, cursor, 200, false, "")
		if err != nil {
			return err
		}
		total += len(fotos)
		if siguiente == "" {
			break
		}
		cursor = siguiente
	}
	albumes, err := repo.Albums(ctx)
	if err != nil {
		return err
	}
	if total != n || len(albumes) != n/fotosPorAlbum {
		return fmt.Errorf("el almacén juntado tiene %d fotos y %d álbumes", total, len(albumes))
	}
	var conFotos int
	for _, a := range albumes {
		if a.Count > 0 {
			conFotos++
		}
	}
	if conFotos != len(albumes) {
		return fmt.Errorf("%d álbumes vacíos", len(albumes)-conFotos)
	}
	return nil
}
