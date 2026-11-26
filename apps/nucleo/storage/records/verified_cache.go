// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Lo ya comprobado
//
// readJSON lee del Crypto el contenido de una revisión, comprueba que es el que dice su
// índice (el registro entero, la huella, el tipo, el objeto) y lo valida con el dominio
// (ValidateRow) CADA vez: una lista que recorre 5.000 notas o 50.000 fotos repite todo
// eso en cada página aunque nada haya cambiado (medido en la prueba de carga del almacén
// local, §5.6 del plan de Sytue Office). Un Crypto cuyo contenido no cambia bajo una
// referencia, y que lo guarda en claro de todos modos, puede pedir que no se repita
// (VerifiedContentCacher): el motor recuerda entonces QUÉ revisiones ya comprobó, por la
// referencia de su índice, y al volver a leer una sigue leyendo el registro del Crypto,
// pero sin repetir lo que solo depende de él. No guarda el JSON: el Crypto ya lo tiene,
// y una segunda copia de toda la galería en memoria no compensa.
//
// El Crypto lo promete por GENERACIONES: mientras no cambie la suya, el registro de una
// referencia es el mismo. El almacén local la sube cuando vuelve a leer su diario del
// disco (al compactar), porque lo leído puede haber cambiado por fuera; entonces el
// motor olvida lo comprobado y todo se vuelve a comprobar entero, como siempre.
//
// Es el mismo resultado: el índice de una referencia es el mismo (se compara entero:
// objeto, metadatos con la huella y puntero de contenido), el registro de esa referencia
// no ha cambiado (lo promete el Crypto) y ValidateRow es una función de la fila (lo son
// las de todos los dominios), así que comprobarlo otra vez daría lo mismo. Solo vale para
// el contenido que va en un registro (`record`, hasta 16 KiB); el que va en un fichero se
// sigue leyendo y comprobando entero cada vez.
//
// Sin esa interfaz (el cliente de Matrix de la edición conectada) todo es como siempre.

// VerifiedContentCacher es opcional en un Crypto: VerifiedContentGeneration es la
// generación de lo que se lee de él; mientras no cambie, el registro de una referencia es
// el mismo y el motor puede recordar lo que ya comprobó de él.
type VerifiedContentCacher interface {
	VerifiedContentGeneration() uint64
}

// verifiedCache es lo ya comprobado, por la referencia del índice. nil si el Crypto no
// lo pide.
type verifiedCache struct {
	crypto VerifiedContentCacher
	mu     sync.Mutex
	// generation es la del Crypto para la que vale entries.
	generation uint64
	entries    map[storage.Reference]verifiedEntry
	// headers es la cabecera (tipo y entidad) de los metadatos de cada índice, como la
	// lee Selected, por su referencia: Selected la descodificaba de cada cabeza en cada
	// llamada. Depende solo de los metadatos, que se comparan, así que vale en cualquier
	// generación.
	headers map[storage.Reference]verifiedHeader
}

// verifiedEntry es lo que readJSON miró del índice de una revisión que comprobó.
type verifiedEntry struct {
	object   string
	metadata []byte
	content  storage.ContentPointer
}

// verifiedHeader es la cabecera descodificada de los metadatos de un índice.
type verifiedHeader struct {
	object       string
	metadata     []byte
	kind, entity string
}

// VerifiedContentGeneration dice si el repositorio recuerda lo comprobado y con qué
// generación del Crypto. Mientras la generación sea la misma, lo que lee una revisión
// por la referencia de su índice no cambia, y un dominio puede guardar también lo que
// descodificó de ella por esa referencia (Notepads lo hace con las relaciones de cada
// nota) y olvidarlo cuando cambie. false, como siempre, con un Crypto que no lo pide.
func (r *Repository) VerifiedContentGeneration() (uint64, bool) {
	if r.verified == nil {
		return 0, false
	}
	return r.verified.current(), true
}

func newVerifiedCache(crypto Crypto) *verifiedCache {
	if c, ok := crypto.(VerifiedContentCacher); ok {
		return &verifiedCache{crypto: c, generation: c.VerifiedContentGeneration(), entries: map[storage.Reference]verifiedEntry{}, headers: map[storage.Reference]verifiedHeader{}}
	}
	return nil
}

// current es la generación del Crypto ahora (0 sin caché).
func (c *verifiedCache) current() uint64 {
	if c == nil {
		return 0
	}
	return c.crypto.VerifiedContentGeneration()
}

// cacheable dice si lo comprobado de version se puede recordar: un índice con
// referencia y contenido en un registro.
func cacheable(version storage.ObjectVersion) bool {
	return version.Reference.Valid() && version.Commit.Content != nil && version.Commit.Content.Kind == "record"
}

// renewLocked olvida lo comprobado si la generación ya no es generation. Con c.mu.
func (c *verifiedCache) renewLocked(generation uint64) {
	if c.entries != nil && generation != c.generation {
		c.generation, c.entries = generation, map[storage.Reference]verifiedEntry{}
	}
}

// has dice si version ya se comprobó con este mismo índice en la generación actual del
// Crypto.
func (c *verifiedCache) has(version storage.ObjectVersion) bool {
	if c == nil || !cacheable(version) {
		return false
	}
	generation := c.current()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.renewLocked(generation)
	e, ok := c.entries[version.Reference]
	return ok && e.object == version.ObjectID && e.content == *version.Commit.Content && bytes.Equal(e.metadata, version.Commit.Metadata)
}

// mark anota que version se ha comprobado en una lectura que empezó en la generación
// generation; no lo anota si la generación ha cambiado desde entonces.
func (c *verifiedCache) mark(version storage.ObjectVersion, generation uint64) {
	if c == nil || !cacheable(version) || generation != c.current() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		return
	}
	c.renewLocked(generation)
	c.entries[version.Reference] = verifiedEntry{object: version.ObjectID, metadata: slices.Clone(version.Commit.Metadata), content: *version.Commit.Content}
}

// readVerifiedJSON lee el contenido de una revisión ya comprobada: el mismo registro del
// Crypto, sin repetir lo que solo depende de él. false si algo no es lo que era (lo dice
// entonces la lectura completa).
func (r *Repository) readVerifiedJSON(ctx context.Context, version storage.ObjectVersion) ([]byte, bool) {
	pointer := version.Commit.Content
	record, err := r.cfg.Crypto.ReadStorageRecord(ctx, pointer.Reference, version.ObjectID, "content")
	if err != nil {
		return nil, false
	}
	defer clear(record.Payload)
	var value content
	if record.ObjectID != version.ObjectID || record.RevisionID != pointer.RevisionID || record.Role != "content" || json.Unmarshal(record.Payload, &value) != nil || value.Kind != r.cfg.EntityKind {
		clear(value.JSON)
		return nil, false
	}
	return value.JSON, true
}

// header devuelve la cabecera ya descodificada de los metadatos de version, si la hay y
// son los mismos.
func (c *verifiedCache) header(version storage.ObjectVersion) (kind, entity string, ok bool) {
	if c == nil || !version.Reference.Valid() {
		return "", "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.headers[version.Reference]
	if !ok || h.object != version.ObjectID || !bytes.Equal(h.metadata, version.Commit.Metadata) {
		return "", "", false
	}
	return h.kind, h.entity, true
}

// putHeader guarda la cabecera descodificada de los metadatos de version.
func (c *verifiedCache) putHeader(version storage.ObjectVersion, kind, entity string) {
	if c == nil || !version.Reference.Valid() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.headers == nil {
		return
	}
	c.headers[version.Reference] = verifiedHeader{object: version.ObjectID, metadata: slices.Clone(version.Commit.Metadata), kind: kind, entity: entity}
}

// keep suelta lo que ya no es de ninguna cabeza de heads.
func (c *verifiedCache) keep(heads map[string][]storage.ObjectVersion) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for ref, e := range c.entries {
		if !slices.ContainsFunc(heads[e.object], func(v storage.ObjectVersion) bool { return v.Reference == ref }) {
			delete(c.entries, ref)
		}
	}
	for ref, h := range c.headers {
		if !slices.ContainsFunc(heads[h.object], func(v storage.ObjectVersion) bool { return v.Reference == ref }) {
			delete(c.headers, ref)
		}
	}
}

// close olvida lo comprobado y no anota más.
func (c *verifiedCache) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries, c.headers = nil, nil
}
