// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Revisión del catálogo
//
// Un dominio que lee el catálogo entero para proyectarlo (Contacts lee cada fila, la
// valida y compone la libreta) puede reutilizar esa proyección mientras no cambie lo que
// leería Catalog. La revisión lo dice sin leer nada: es un contador que sube
//
//   - cada vez que un refresco (el de cada escritura, el de RefreshPrivateData, el de
//     Initialize) deja cabezas distintas de las que había, y no cuando trae las mismas;
//     así cuenta también lo que escribió otro repositorio, otro proceso u otro
//     dispositivo en cuanto este motor lo ve, que es cuando cambia lo que lee Catalog;
//   - y, con un Crypto con sello (CatalogStamper, más abajo), cada vez que el sello no
//     es el que ya contaba: algo cambió en el Crypto (una escritura de otro repositorio
//     sobre él, el diario del almacén local releído al compactar) y lo que se lee de
//     unas cabezas iguales podría no serlo. Quien proyectó el catálogo lo relee y lo
//     valida, como siempre.
//
// Una cabeza es la misma si lo es su referencia (un evento, que no cambia), su objeto, su
// estado, sus metadatos (con la huella del contenido, que readJSON comprueba) y su
// puntero de contenido. Los padres no cuentan: la compactación del almacén local los
// reescribe sin cambiar nada de lo que se lee.

// catalogRevision es la revisión y lo que cambió en su último paso. Con r.mu.
type catalogRevision struct {
	value uint64
	// Del valor from a value cambió solo el objeto object; vacío si cambió más de uno
	// o no se sabe qué.
	from   uint64
	object string
	// stamp es el sello del Crypto que la revisión ya cuenta, si lo da.
	stamp loadedStamp
}

// bumpLocked sube la revisión: de la anterior cambió solo object ("" si no se sabe).
func (r *Repository) bumpLocked(object string) {
	r.revision.from = r.revision.value
	r.revision.object = object
	r.revision.value++
}

// noteHeadsLocked anota que las cabezas pasan a ser next, cargadas del Crypto con el
// sello loaded. Con r.mu, antes de ponerlas.
func (r *Repository) noteHeadsLocked(next map[string][]storage.ObjectVersion, loaded loadedStamp) {
	changed, single := headsChange(r.heads, next)
	moved := loaded.valid && r.revision.stamp != loaded
	r.revision.stamp = loaded
	if changed > 0 && next != nil {
		r.verified.keep(next)
	}
	switch {
	case changed == 1:
		// Lo demás que cambiara en el Crypto es lo que llevó a esta cabeza (su contenido,
		// su índice): una escritura no cambia lo que ya estaba.
		r.bumpLocked(single)
	case changed > 1 || moved:
		r.bumpLocked("")
	}
}

// headsChange cuenta los objetos cuyas cabezas cambian de old a next (hasta dos: basta
// para saber si fue uno solo) y, si fue uno, cuál.
func headsChange(old, next map[string][]storage.ObjectVersion) (int, string) {
	changed, single := 0, ""
	for object, versions := range next {
		if !sameHeads(old[object], versions) {
			changed++
			single = object
			if changed > 1 {
				return changed, ""
			}
		}
	}
	for object, versions := range old {
		if _, found := next[object]; !found && len(versions) > 0 {
			changed++
			single = object
			if changed > 1 {
				return changed, ""
			}
		}
	}
	return changed, single
}

func sameHeads(a, b []storage.ObjectVersion) bool {
	if len(a) != len(b) {
		return false
	}
	for _, version := range a {
		if !slices.ContainsFunc(b, func(other storage.ObjectVersion) bool { return sameHead(version, other) }) {
			return false
		}
	}
	return true
}

func sameHead(a, b storage.ObjectVersion) bool {
	if a.Reference != b.Reference || a.ObjectID != b.ObjectID || a.RevisionID != b.RevisionID || a.Commit.Kind != b.Commit.Kind || a.Commit.State != b.Commit.State || !bytes.Equal(a.Commit.Metadata, b.Commit.Metadata) {
		return false
	}
	if (a.Commit.Content == nil) != (b.Commit.Content == nil) {
		return false
	}
	return a.Commit.Content == nil || *a.Commit.Content == *b.Commit.Content
}

// # Cargar solo lo que ha cambiado
//
// Cada escritura refresca el catálogo dos veces (antes de preparar y después de
// publicar), y refrescar es leer y validar el historial entero del almacén
// (storage.LoadAtomicCatalog). Un Crypto cuyo catálogo solo cambia a través de él (el
// almacén local de Sytue Office, que no ve lo que escribe otro proceso hasta volver a
// leer su diario) puede decir con un sello si ha cambiado algo desde la última carga:
// si no, el refresco no vuelve a cargar lo mismo. Sin sello, todo es como siempre.

// CatalogStamper es opcional en un Crypto: CatalogStamp cambia con cualquier cambio de
// lo que el motor lee de él, y solo puede cambiar a través de él.
type CatalogStamper interface {
	CatalogStamp() uint64
}

// loadedStamp es un sello del Crypto, si lo da.
type loadedStamp struct {
	value uint64
	valid bool
}

// cryptoStamp es el sello del Crypto, si lo da.
func (r *Repository) cryptoStamp() (uint64, bool) {
	stamper, ok := r.cfg.Crypto.(CatalogStamper)
	if !ok {
		return 0, false
	}
	return stamper.CatalogStamp(), true
}

// unchangedSince dice si el catálogo cargado es el del sello stamp.
func (r *Repository) unchangedSince(stamp uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loaded.valid && r.loaded.value == stamp && r.heads != nil
}

func (r *Repository) loadedNow() loadedStamp {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loaded
}

// CatalogRevision es la revisión de lo que leería ahora Catalog (ver arriba). Falla
// como Catalog: con ErrClosed o ErrNotReady.
func (r *Repository) CatalogRevision() (uint64, error) {
	stamp, stamped := r.cryptoStamp()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrClosed
	}
	if !r.ready {
		return 0, ErrNotReady
	}
	if current := (loadedStamp{value: stamp, valid: stamped}); stamped && r.revision.stamp != current {
		r.revision.stamp = current
		r.bumpLocked("")
	}
	return r.revision.value, nil
}

// CatalogOwner es lo que da el Catalog que recibe Config.ValidateCatalog: el repositorio
// cuyo catálogo es y su revisión. Al abrir (Initialize), es el mismo repositorio que
// queda listo con esas cabezas, y quien valida puede guardar lo que proyectó con esa
// revisión; al refrescar (RefreshPrivateData) es otro, provisional, cuya revisión no
// cuenta para nadie.
type CatalogOwner interface {
	CatalogOwner() (*Repository, uint64)
}

func (v catalogView) CatalogOwner() (*Repository, uint64) {
	v.r.mu.Lock()
	defer v.r.mu.Unlock()
	return v.r, v.r.revision.value
}

// PublishedSince dice si el catálogo actual es exactamente el de la revisión from con
// una sola diferencia: la cabeza de (kind, id), que es ahora única y tiene por contenido
// raw (lo comprueba por su huella). Es lo que deja una escritura propia que nadie más ha
// cruzado: quien proyectó el catálogo de from puede aplicarle raw en vez de releerlo.
// Devuelve la revisión actual; false si no se sabe (otro cambio por medio, otra revisión
// después, un conflicto, el repositorio cerrado).
func (r *Repository) PublishedSince(from uint64, kind, id string, raw []byte) (uint64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || !r.ready || r.revision.value != from+1 || r.revision.from != from || r.revision.object == "" || r.revision.object != r.object(kind, id) {
		return 0, false
	}
	heads := r.heads[r.revision.object]
	if len(heads) != 1 {
		return 0, false
	}
	// description exige el tipo de entidad del producto: un recibo no pasa.
	summary, err := r.description(heads[0])
	if err != nil || summary.Entity != kind || summary.ID != id || summary.Binary || summary.SHA256 != hashBytes(raw) {
		return 0, false
	}
	return r.revision.value, true
}
