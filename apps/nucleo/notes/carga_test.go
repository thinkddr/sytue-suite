// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen/cargaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
)

// # Prueba de carga del almacén: Notepads (PLAN-EDICION-LOCAL §5.6, R-MEDIR)
//
// 5.000 notas en 50 cuadernos, escritas con el repositorio de la edición conectada
// (NewLocal, como `cmd-local/notes`) sobre el puerto real en disco: apertura en frío en
// un proceso nuevo (preparar el motor y la primera pantalla: los cuadernos y las notas
// del primero), su memoria, y guardar una nota. Las cifras están en SYTUE-OFFICE.md
// («Pendiente → Compactar el almacén»):
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'CargaNotas' -v -timeout 0 ./notes/

const sujetoCarga = "instalacion-carga"

// TestCargaNotas mide 5.000 notas en 50 cuadernos.
func TestCargaNotas(t *testing.T) {
	if dir, hija := cargaprueba.Hija(); hija {
		cargaprueba.Responder(t, func(r *cargaprueba.Reloj) (func(), error) { return abrirNotasMidiendo(dir, r) })
		return
	}
	cargaprueba.Activa(t)
	dir := plantillaNotas(t)
	veces := cargaprueba.Veces()
	frio := cargaprueba.EnFrio(t, dir, veces)
	abrir := cargaprueba.Suma(frio, "preparar", "primera pantalla")
	heap := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.HeapInuse })
	rss := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })

	// Guardar: sobre una copia, con la app ya abierta en un cuaderno, cambiar el texto de
	// una nota distinta cada vez.
	ctx := t.Context()
	repo, cerrar := abrirNotasEn(t, cargaprueba.Copia(t, dir))
	defer cerrar()
	cuadernos, err := repo.ListNotebooks(ctx)
	if err != nil || len(cuadernos) != 50 {
		t.Fatal(len(cuadernos), err)
	}
	lista, err := repo.ListNotes(ctx, cuadernos[0].ID)
	if err != nil || len(lista) != 100 {
		t.Fatal(len(lista), err)
	}
	guardar := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		n := lista[i%len(lista)]
		_, err := repo.UpdateNote(ctx, n.ID, n.NotebookID, api.NoteInput{Kind: n.Kind, Title: n.Title, Content: n.Content + fmt.Sprintf("<p>Cambio %d.</p>", i)})
		return err
	})
	listar := cargaprueba.Repetir(t, 4*veces, nil, func(i int) error {
		_, err := repo.ListNotes(ctx, cuadernos[i%len(cuadernos)].ID)
		return err
	})
	cargaprueba.Informar(t,
		cargaprueba.Fila{Carga: "5.000 notas, 50 cuadernos", Medida: "abrir en frío (motor + primera pantalla)", Valor: abrir.String(), Umbral: "< 1,5 s", Cumple: abrir.Mediana() < 1500},
		cargaprueba.Fila{Carga: "5.000 notas, 50 cuadernos", Medida: "· preparar el motor", Valor: cargaprueba.Tiempo(frio, "preparar").String()},
		cargaprueba.Fila{Carga: "5.000 notas, 50 cuadernos", Medida: "· primera pantalla", Valor: cargaprueba.Tiempo(frio, "primera pantalla").String()},
		cargaprueba.Fila{Carga: "5.000 notas, 50 cuadernos", Medida: "memoria: montón en uso / pico RSS", Valor: cargaprueba.MiB(heap) + " / " + cargaprueba.MiB(rss)},
		cargaprueba.Fila{Carga: "5.000 notas, 50 cuadernos", Medida: "guardar una nota", Valor: guardar.String(), Umbral: "< 50 ms", Cumple: guardar.Mediana() < 50},
		cargaprueba.Fila{Carga: "5.000 notas, 50 cuadernos", Medida: "abrir otro cuaderno (listar sus notas)", Valor: listar.String()},
	)
}

// plantillaNotas es la carga de 5.000 notas en 50 cuadernos.
func plantillaNotas(t *testing.T) string {
	t.Helper()
	return cargaprueba.Plantilla(t, "notas-5000", func(dir string) error {
		return cargaprueba.EnMemoria(dir, func(dir string) error { return generarNotas(dir, 50, 100) })
	})
}

// abrirNotasMidiendo es lo que hace `cmd-local/notes` al arrancar, cronometrado: el
// almacén, el repositorio y su catálogo, y la primera pantalla.
func abrirNotasMidiendo(dir string, r *cargaprueba.Reloj) (func(), error) {
	ctx := context.Background()
	var (
		rec  *almacen.Recursos
		repo *notes.Repository
	)
	err := r.Medir("preparar", func() error {
		var err error
		if rec, err = almacen.Abrir(ctx, dir, "notes", sujetoCarga); err != nil {
			return err
		}
		if repo, err = notes.NewLocal(rec.Config); err != nil {
			return err
		}
		return almacen.Preparar(ctx, repo)
	})
	if err != nil {
		return nil, err
	}
	cerrar := func() { repo.Close(); rec.Cerrar() }
	err = r.Medir("primera pantalla", func() error {
		cuadernos, err := repo.ListNotebooks(ctx)
		if err != nil {
			return err
		}
		if len(cuadernos) == 0 {
			return fmt.Errorf("sin cuadernos")
		}
		_, err = repo.ListNotes(ctx, cuadernos[0].ID)
		return err
	})
	if err != nil {
		cerrar()
		return nil, err
	}
	return cerrar, nil
}

func abrirNotasEn(t *testing.T, dir string) (*notes.Repository, func()) {
	t.Helper()
	rec, err := almacen.Abrir(context.Background(), dir, "notes", sujetoCarga)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := notes.NewLocal(rec.Config)
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

// generarNotas escribe cuadernos × porCuaderno notas con el repositorio, como las
// escribiría la pantalla.
func generarNotas(dir string, cuadernos, porCuaderno int) error {
	ctx := context.Background()
	rec, err := almacen.Abrir(ctx, dir, "notes", sujetoCarga)
	if err != nil {
		return err
	}
	defer rec.Cerrar()
	repo, err := notes.NewLocal(rec.Config)
	if err != nil {
		return err
	}
	defer repo.Close()
	if err := almacen.Preparar(ctx, repo); err != nil {
		return err
	}
	r := rand.New(rand.NewPCG(2026, 5)) //nolint:gosec // G404: datos de prueba repetibles
	for c := range cuadernos {
		pos := c
		nb, err := repo.CreateNotebook(ctx, api.NotebookInput{Title: fmt.Sprintf("%s %d", temas[c%len(temas)], c+1), Position: &pos})
		if err != nil {
			return err
		}
		for i := range porCuaderno {
			if _, err := repo.CreateNote(ctx, nb.ID, notaRealista(r, i)); err != nil {
				return fmt.Errorf("nota %d del cuaderno %d: %w", i, c, err)
			}
		}
	}
	return nil
}

var (
	temas    = []string{"Casa", "Trabajo", "Viajes", "Recetas", "Lecturas", "Proyecto", "Clase", "Ideas", "Salud", "Compras"}
	frases   = []string{"Llamar al fontanero antes del jueves para lo del baño.", "Revisar el presupuesto con la gestoría y pedir la factura rectificada.", "La reunión se mueve al martes a las diez; llevar las cifras del trimestre.", "Comprar harina de fuerza, levadura fresca y aceite de oliva virgen extra.", "El capítulo tres explica bien la diferencia entre índice y catálogo.", "Reservar el hotel cerca de la estación; el tren sale a las 7:42.", "Apuntar las dudas para la tutoría: ejercicios 4, 7 y 12.", "Idea: una plantilla para las actas con los acuerdos arriba.", "Cita con el dentista el día 14 a las 17:30.", "Devolver el libro a la biblioteca antes de fin de mes."}
	listados = []string{"Pan", "Leche", "Huevos", "Tomates", "Café", "Papel", "Pilas", "Fruta"}
)

// notaRealista es una nota como las de un cuaderno de verdad: un título y de uno a ocho
// párrafos, a veces con una lista.
func notaRealista(r *rand.Rand, i int) api.NoteInput {
	var b strings.Builder
	for range 1 + r.IntN(8) {
		fmt.Fprintf(&b, "<p>%s</p>", frases[r.IntN(len(frases))])
	}
	if r.IntN(4) == 0 {
		b.WriteString("<ul>")
		for range 2 + r.IntN(5) {
			fmt.Fprintf(&b, "<li>%s</li>", listados[r.IntN(len(listados))])
		}
		b.WriteString("</ul>")
	}
	return api.NoteInput{Kind: "text", Title: fmt.Sprintf("Nota %d", i+1), Content: b.String()}
}
