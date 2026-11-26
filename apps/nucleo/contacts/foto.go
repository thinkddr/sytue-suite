// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contacts

import (
	"bytes"
	"context"
	"sync"
)

// # La instantánea guardada
//
// Leer la libreta es leer el catálogo entero del motor de registros y descodificar y
// validar cada fila (readCatalog). Con 20.000 contactos son segundos, y antes se hacía
// en cada consulta y dos veces en cada alta (la de la escritura y la de devolver el
// contacto), así que dar de alta crecía con la libreta y una importación de CSV, que
// escribe fila a fila, era cuadrática.
//
// Ahora el repositorio guarda la última instantánea con la revisión del catálogo de la
// que salió (records.Repository.CatalogRevision), que sube cada vez que las cabezas que
// lee Catalog cambian: por una escritura propia, por la de otro repositorio sobre el
// mismo almacén o por la de otro dispositivo, en cuanto el motor las ve en un refresco.
//
//   - Mientras la revisión no cambie, Snapshot devuelve la guardada.
//   - Tras una escritura propia que sale bien, la nueva es la candidate que
//     mutateAggregate ya compone y valida para escribir, si el motor confirma que el
//     catálogo es exactamente el de antes con esa escritura y nada más
//     (records.Repository.PublishedSince) y si los índices de favoritos y de papelera
//     no la cambian (model.Snapshot.IndexesKeep): entonces es la misma que daría
//     releer. Si no, la revisión ya no coincide y la próxima lectura relee.
//   - Si la revisión cambió por otra cosa, se relee.
//
// Una instantánea no cambia nunca (model.Snapshot); cada llamada recibe su propio
// envoltorio, así que nadie puede tocar la guardada.

// fotoGuardada es una instantánea con la revisión del catálogo que la dio y los índices
// con que se compuso.
type fotoGuardada struct {
	revision  uint64
	snapshot  *Snapshot
	favorites *FavoriteIndex
	trash     *TrashIndex
}

// fotos es la instantánea guardada de un repositorio.
type fotos struct {
	mu     sync.Mutex
	actual *fotoGuardada
}

// foto es la instantánea del catálogo actual: la guardada si sigue valiendo, o una
// leída ahora (que se guarda).
func (r *Repository) foto(parent context.Context) (*fotoGuardada, error) {
	ctx, done, err := r.core.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	// La revisión se mira ANTES de leer: si el catálogo cambia mientras se lee, lo
	// leído queda con una revisión vieja y la próxima vez se relee, nunca al revés.
	revision, err := r.core.CatalogRevision()
	if err != nil {
		return nil, err
	}
	if foto := r.fotos.de(revision); foto != nil {
		return foto, nil
	}
	catalog, err := r.core.Catalog()
	if err != nil {
		return nil, err
	}
	read, err := readCatalogIndexes(ctx, r.scope, catalog)
	if err != nil {
		return nil, err
	}
	foto := &fotoGuardada{revision: revision, snapshot: read.snapshot, favorites: read.favorites, trash: read.trash}
	r.fotos.poner(foto)
	return foto, nil
}

// de es la guardada si es de esa revisión.
func (f *fotos) de(revision uint64) *fotoGuardada {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.actual != nil && f.actual.revision == revision {
		return f.actual
	}
	return nil
}

// poner guarda foto, salvo que ya haya una de una revisión posterior.
func (f *fotos) poner(foto *fotoGuardada) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.actual == nil || foto.revision >= f.actual.revision {
		f.actual = foto
	}
}

func (f *fotos) olvidar() {
	f.mu.Lock()
	f.actual = nil
	f.mu.Unlock()
}

// escritura es lo que vio y compuso la preparación de una escritura de un agregado.
type escritura struct {
	base      *fotoGuardada
	change    Aggregate
	candidate *Snapshot
	payload   []byte // copia: el motor borra el suyo al acabar
}

// anotarEscritura guarda la candidate como la instantánea de después de la escritura
// raw, si es exactamente la que daría releer (ver arriba). Si no, no guarda nada.
func (r *Repository) anotarEscritura(hecha *escritura, raw []byte) {
	if hecha == nil || !bytes.Equal(raw, hecha.payload) {
		// Sin preparación propia (el motor reanudó una escritura retenida) o con otra.
		return
	}
	revision, ok := r.core.PublishedSince(hecha.base.revision, hecha.change.Kind, hecha.change.ID, raw)
	if !ok || !hecha.candidate.IndexesKeep(hecha.change, hecha.base.favorites, hecha.base.trash) {
		return
	}
	r.fotos.poner(&fotoGuardada{revision: revision, snapshot: hecha.candidate, favorites: hecha.base.favorites, trash: hecha.base.trash})
}
