// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen/cargaprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Prueba de carga del almacén: un historial de 1.000.000 de registros (§5.6)
//
// Las 5.000 notas de TestCargaNotas, cada una guardada 99 veces más: unas 990.000 líneas
// de historial (contenido e índice de cada versión, con su padre) en el diario, que es lo
// que se acumula si el almacén no se compacta nunca (otra instancia siempre abierta, o una
// app que no se cierra). Mide abrirlo en frío y su memoria, cuánto tarda compactarlo y
// abrirlo después: es lo que dice si hace falta compactar. SYTUE_CARGA_VERSIONES cambia
// las versiones por nota (100 por defecto: 1.000.000 de registros con las 5.000 notas).
//
//	SYTUE_CARGA=1 go test -tags goolm -run 'CargaHistorial' -v -timeout 0 ./notes/
//
// Las versiones no se escriben con el repositorio: cada guardado recarga el catálogo
// entero, que con el historial creciendo hasta el millón llevaría días. Se escriben con el
// puerto, en el mismo formato que el motor (records.publishJSON: el contenido en un
// registro y un índice con la huella y el padre), sobre la cabeza de cada nota; y la
// prueba de que es lo mismo es que el repositorio abre el resultado, que valida el
// catálogo y el grafo entero, y lee la última versión de cada nota.

// versionesDeCarga es SYTUE_CARGA_VERSIONES, o 100.
func versionesDeCarga(t *testing.T) int {
	t.Helper()
	n := 100
	if v := os.Getenv("SYTUE_CARGA_VERSIONES"); v != "" {
		var err error
		if n, err = strconv.Atoi(v); err != nil || n < 1 {
			t.Fatalf("SYTUE_CARGA_VERSIONES=%q", v)
		}
	}
	return n
}

// TestCargaHistorial mide el historial grande sin compactar y compactado.
func TestCargaHistorial(t *testing.T) {
	if dir, hija := cargaprueba.Hija(); hija {
		cargaprueba.Responder(t, func(r *cargaprueba.Reloj) (func(), error) { return abrirNotasMidiendo(dir, r) })
		return
	}
	cargaprueba.Activa(t)
	versiones := versionesDeCarga(t)
	notas := plantillaNotas(t)
	dir := cargaprueba.Plantilla(t, fmt.Sprintf("historial-%d", versiones), func(dir string) error {
		return cargaprueba.EnMemoria(dir, func(dir string) error {
			if err := cargaprueba.CopiarArbol(notas, dir); err != nil {
				return err
			}
			return escribirHistorial(dir, versiones-1)
		})
	})
	lineas, err := contarLineas(dir)
	if err != nil {
		t.Fatal(err)
	}
	carga := fmt.Sprintf("historial de %d registros", lineas)
	t.Logf("%s: %s de diario", carga, cargaprueba.Megas(tamano(dir)))
	// El repositorio lo abre y la última versión de cada nota es la que se escribió.
	repo, cerrar := abrirNotasEn(t, cargaprueba.Copia(t, dir))
	cuadernos, err := repo.ListNotebooks(t.Context())
	if err != nil || len(cuadernos) != 50 {
		t.Fatal(len(cuadernos), err)
	}
	lista, err := repo.ListNotes(t.Context(), cuadernos[0].ID)
	if err != nil || len(lista) != 100 || lista[0].Title == "" {
		t.Fatal(len(lista), err)
	}
	cerrar()

	veces := min(cargaprueba.Veces(), 3)
	frio := cargaprueba.EnFrio(t, dir, veces)
	abrir := cargaprueba.Suma(frio, "preparar", "primera pantalla")
	heap := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.HeapInuse })
	rss := cargaprueba.Memoria(frio, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })

	// Compactar una copia (lo que hace la app al cerrar) y abrirla después.
	copia := cargaprueba.Copia(t, dir)
	var hecho almacen.Compactado
	compactar := cargaprueba.Repetir(t, 1, nil, func(int) error {
		rec, err := almacen.Abrir(context.Background(), copia, "notes", sujetoCarga)
		if err != nil {
			return err
		}
		defer rec.Cerrar()
		hecho, err = rec.Compactar(context.Background())
		return err
	})
	despues := cargaprueba.EnFrio(t, copia, veces)
	abrirDespues := cargaprueba.Suma(despues, "preparar", "primera pantalla")
	rssDespues := cargaprueba.Memoria(despues, func(m cargaprueba.Medidas) uint64 { return m.PicoRSS })
	cargaprueba.Informar(t,
		cargaprueba.Fila{Carga: carga, Medida: "abrir en frío sin compactar (motor + primera pantalla)", Valor: abrir.String()},
		cargaprueba.Fila{Carga: carga, Medida: "· preparar el motor", Valor: cargaprueba.Tiempo(frio, "preparar").String()},
		cargaprueba.Fila{Carga: carga, Medida: "memoria sin compactar: montón en uso / pico RSS", Valor: cargaprueba.MiB(heap) + " / " + cargaprueba.MiB(rss)},
		cargaprueba.Fila{Carga: carga, Medida: fmt.Sprintf("compactar (%d → %d líneas)", hecho.LineasAntes, hecho.LineasDespues), Valor: compactar.String()},
		cargaprueba.Fila{Carga: carga, Medida: "abrir en frío ya compactado", Valor: abrirDespues.String()},
		cargaprueba.Fila{Carga: carga, Medida: "pico RSS ya compactado", Valor: cargaprueba.MiB(rssDespues)},
	)
}

// entidadNota son los metadatos del índice de una nota, como los escribe el motor
// (records.entity, con el mismo orden de campos).
type entidadNota struct {
	Kind   string `json:"kind"`
	Entity string `json:"entity"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

// contenidoNota es el registro de contenido de una versión (records.content).
type contenidoNota struct {
	Kind string `json:"kind"`
	JSON []byte `json:"json"`
}

// escribirHistorial guarda cada nota de dir otras extra veces, con el puerto y en el
// formato del motor, cada versión hija de la anterior.
func escribirHistorial(dir string, extra int) error {
	ctx := context.Background()
	scope := storage.Scope{Tenant: "local", Subject: sujetoCarga, Product: "notes"}
	p, err := almacen.AbrirPuerto(dir, scope)
	if err != nil {
		return err
	}
	defer func() { _ = p.Cerrar() }()
	enlace, err := p.StorageBindings(ctx)
	if err != nil {
		return err
	}
	pagina, err := p.StorageObjectHistory(ctx, enlace.Target.RoomID, "", 0)
	if err != nil {
		return err
	}
	// Las cabezas de las notas: el índice que no es padre de ningún otro.
	padres := map[storage.Reference]bool{}
	var versiones []storage.ObjectVersion
	for _, e := range pagina.Entries {
		v, err := storage.DecodeObject(e, scope)
		if err != nil {
			continue // el recibo y lo que no es un objeto suelto
		}
		for _, padre := range v.Commit.Parents {
			padres[padre] = true
		}
		versiones = append(versiones, v)
	}
	escritas := 0
	for _, v := range versiones {
		var meta entidadNota
		if padres[v.Reference] || json.Unmarshal(v.Commit.Metadata, &meta) != nil || meta.Entity != "note" || v.Commit.Content == nil || v.Commit.Content.Kind != "record" {
			continue
		}
		reg, err := p.ReadStorageRecord(ctx, v.Commit.Content.Reference, v.ObjectID, "content")
		if err != nil {
			return err
		}
		var c contenidoNota
		if err := json.Unmarshal(reg.Payload, &c); err != nil {
			return err
		}
		var fila map[string]json.RawMessage
		if err := json.Unmarshal(c.JSON, &fila); err != nil {
			return err
		}
		cuerpo := ""
		_ = json.Unmarshal(fila["content"], &cuerpo)
		anterior := v.Reference
		for i := range extra {
			fila["content"], _ = json.Marshal(cuerpo + fmt.Sprintf("<p>Cambio %d.</p>", i+1))
			raw, err := json.Marshal(fila)
			if err != nil {
				return err
			}
			rev := uuid.NewString()
			carga, _ := json.Marshal(contenidoNota{Kind: meta.Kind, JSON: raw})
			ref, err := p.PutStorageRecordAt(ctx, enlace.Target, rev+"_content", storage.Record{Version: 1, Scope: scope, ObjectID: v.ObjectID, RevisionID: rev, Role: "content", Payload: carga})
			if err != nil {
				return err
			}
			suma := sha256.Sum256(raw)
			metadatos, _ := json.Marshal(entidadNota{Kind: meta.Kind, Entity: meta.Entity, ID: meta.ID, SHA256: hex.EncodeToString(suma[:])})
			nueva, err := p.PutStorageObjectAt(ctx, enlace.Target, rev+"_index", v.ObjectID, rev+"_index", storage.ObjectCommit{
				Kind: storage.ObjectKind, State: v.Commit.State, Metadata: metadatos, Parents: []storage.Reference{anterior},
				Content: &storage.ContentPointer{Kind: "record", RevisionID: rev, Reference: ref},
			})
			if err != nil {
				return err
			}
			anterior = nueva.Reference
		}
		escritas++
	}
	if escritas == 0 {
		return errors.New("no hay notas a las que añadir historial")
	}
	return nil
}

func contarLineas(dir string) (int, error) {
	f, err := os.Open(dir + "/registros.jsonl")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, trozo := 0, make([]byte, 1<<20)
	for {
		leidos, err := f.Read(trozo)
		n += bytes.Count(trozo[:leidos], []byte{'\n'})
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
	}
}

func tamano(dir string) int64 {
	info, err := os.Stat(dir + "/registros.jsonl")
	if err != nil {
		return 0
	}
	return info.Size()
}
