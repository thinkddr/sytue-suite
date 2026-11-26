// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storagetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// # El puerto en memoria del motor de registros
//
// Memoria es el `records.Crypto` de las pruebas en memoria: el publicador de la edición
// conectada (una sala de Matrix donde cada revisión es un evento) sin red, sin cifrado y
// sin disco. Es el otro lado de los contratos de PLAN-EDICION-LOCAL §8.2: el MISMO juego
// de pruebas de un repositorio de dominio corre sobre él y sobre el almacén en claro de
// Sytue Office (`local/almacen`), y así se ve que los repositorios no dependen de lo que
// solo hace uno de los dos.
//
// Por eso se parece al publicador y NO al almacén donde los dos difieren:
//
//   - el historial de la sala sale de lo más nuevo a lo más viejo y por páginas (como
//     `/messages` hacia atrás), no en una sola página en orden de escritura;
//   - tiene las capacidades opcionales del cliente de Matrix (la huella de un fichero,
//     leerlo por trozos, cerrar la subida de uno retenido) y no las que solo tiene el
//     almacén (`records.CatalogStamper`, `records.VerifiedContentCacher`): el motor
//     vuelve a leer la sala entera en cada lectura, como en la conectada;
//   - dos repositorios sobre la misma Memoria son dos dispositivos de la misma sala: lo
//     que publica uno lo ve el otro en cuanto vuelve a leerla.
//
// Solo lo importan las pruebas.
type Memoria struct {
	scope   storage.Scope
	binding storage.Binding

	mu       sync.Mutex
	records  map[storage.Reference]storage.Record
	orden    []storage.Reference
	txns     map[string]storage.Reference
	ficheros map[storage.Reference]ficheroEnMemoria
	trabajos map[string]storage.Reference
	eventos  int
}

type ficheroEnMemoria struct {
	peticion storage.FileRequest
	datos    []byte
}

var (
	// ErrMemoria es una petición que no es de esta sala (otro alcance, otro destino).
	ErrMemoria = errors.New("storagetest: petición ajena a esta memoria")
	// ErrNoEsta es un registro o un fichero que no existe.
	ErrNoEsta = errors.New("storagetest: no está")
)

var _ records.Crypto = (*Memoria)(nil)

// PaginaDeHistorial es lo más que da StorageObjectHistory de una vez (Matrix pagina).
const PaginaDeHistorial = 7

// NuevaMemoria es una sala vacía para scope.
func NuevaMemoria(scope storage.Scope) *Memoria {
	sala := "!memoria-" + scope.Product + ":prueba.invalid"
	return &Memoria{
		scope:    scope,
		binding:  storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: sala, DeviceID: "PRUEBA"}, Rooms: []string{sala}},
		records:  map[storage.Reference]storage.Record{},
		txns:     map[string]storage.Reference{},
		ficheros: map[storage.Reference]ficheroEnMemoria{},
		trabajos: map[string]storage.Reference{},
	}
}

func (m *Memoria) StorageIdentityVersion() storage.IdentityVersion { return storage.IsolatedIdentity }

func (m *Memoria) StorageBindings(context.Context) (storage.Binding, error) { return m.binding, nil }

func (m *Memoria) PrepareStorage(_ context.Context, scope storage.Scope) (storage.Binding, error) {
	if scope != m.scope {
		return storage.Binding{}, ErrMemoria
	}
	return m.binding, nil
}

func (m *Memoria) CheckStoragePublicationTarget(t storage.PublicationTarget) error {
	if t != m.binding.Target {
		return ErrMemoria
	}
	return nil
}

// StorageObjectHistory da los índices de la sala de lo más nuevo a lo más viejo, de
// PaginaDeHistorial en PaginaDeHistorial; el cursor es la posición siguiente.
func (m *Memoria) StorageObjectHistory(_ context.Context, sala, cursor string, limite int) (storage.ObjectPage, error) {
	if sala != m.binding.Target.RoomID {
		return storage.ObjectPage{}, ErrMemoria
	}
	desde := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n <= 0 {
			return storage.ObjectPage{}, ErrMemoria
		}
		desde = n
	}
	limite = min(max(limite, 1), PaginaDeHistorial)
	m.mu.Lock()
	defer m.mu.Unlock()
	var indices []storage.Reference
	for i := len(m.orden) - 1; i >= 0; i-- {
		if m.records[m.orden[i]].Role == "index" {
			indices = append(indices, m.orden[i])
		}
	}
	if desde > len(indices) {
		return storage.ObjectPage{}, ErrMemoria
	}
	hasta := min(desde+limite, len(indices))
	pagina := storage.ObjectPage{}
	for _, ref := range indices[desde:hasta] {
		pagina.Entries = append(pagina.Entries, storage.RecordEntry{Reference: ref, Record: clonarRegistro(m.records[ref])})
	}
	if hasta < len(indices) {
		pagina.Next = strconv.Itoa(hasta)
	}
	return pagina, nil
}

func clonarRegistro(r storage.Record) storage.Record {
	r.Payload = slices.Clone(r.Payload)
	r.Previous = slices.Clone(r.Previous)
	return r
}

func (m *Memoria) ReadStorageRecord(_ context.Context, ref storage.Reference, objeto, rol string) (storage.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[ref]
	if !ok || r.ObjectID != objeto || r.Role != rol {
		return storage.Record{}, ErrNoEsta
	}
	return clonarRegistro(r), nil
}

// referencia es un evento nuevo de la sala. Con m.mu.
func (m *Memoria) referencia() storage.Reference {
	m.eventos++
	return storage.Reference{RoomID: m.binding.Target.RoomID, EventID: "$evento" + strconv.Itoa(m.eventos)}
}

// PutStorageRecordAt publica un registro; la misma transacción da el mismo evento.
func (m *Memoria) PutStorageRecordAt(_ context.Context, t storage.PublicationTarget, txn string, r storage.Record) (storage.Reference, error) {
	if err := m.CheckStoragePublicationTarget(t); err != nil {
		return storage.Reference{}, err
	}
	if r.Validate(m.scope) != nil {
		return storage.Reference{}, ErrMemoria
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ref, ok := m.txns[txn]; ok {
		return ref, nil
	}
	ref := m.referencia()
	m.records[ref] = clonarRegistro(r)
	m.orden = append(m.orden, ref)
	m.txns[txn] = ref
	return ref, nil
}

func (m *Memoria) PutStorageObjectAt(ctx context.Context, t storage.PublicationTarget, txn, objeto, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	payload, err := json.Marshal(commit)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	previos := make([]string, len(commit.Parents))
	for i, padre := range commit.Parents {
		previos[i] = padre.EventID
	}
	ref, err := m.PutStorageRecordAt(ctx, t, txn, storage.Record{Version: 1, Scope: m.scope, ObjectID: objeto, RevisionID: revision, Role: "index", Payload: payload, Previous: previos})
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return m.StorageObject(ctx, ref, objeto)
}

func (m *Memoria) StorageObject(ctx context.Context, ref storage.Reference, objeto string) (storage.ObjectVersion, error) {
	r, err := m.ReadStorageRecord(ctx, ref, objeto, "index")
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: r}, m.scope)
}

// UploadStorageFileAt guarda un binario; el mismo trabajo da el mismo evento.
func (m *Memoria) UploadStorageFileAt(_ context.Context, _ *api.Drive, _ *offline.Store, trabajo string, t storage.PublicationTarget, req storage.FileRequest, src io.ReaderAt) (storage.Reference, error) {
	if err := m.CheckStoragePublicationTarget(t); err != nil {
		return storage.Reference{}, err
	}
	if req.Validate(m.scope) != nil {
		return storage.Reference{}, ErrMemoria
	}
	datos, err := io.ReadAll(io.NewSectionReader(src, 0, req.SizeBytes))
	if err != nil {
		return storage.Reference{}, err
	}
	if int64(len(datos)) != req.SizeBytes {
		return storage.Reference{}, io.ErrUnexpectedEOF
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ref, ok := m.trabajos[trabajo]; ok {
		return ref, nil
	}
	ref := m.referencia()
	req.Metadata = slices.Clone(req.Metadata)
	req.Previous = slices.Clone(req.Previous)
	m.ficheros[ref] = ficheroEnMemoria{peticion: req, datos: datos}
	m.trabajos[trabajo] = ref
	return ref, nil
}

// fichero es el binario ref de objeto y revisión.
func (m *Memoria) fichero(ref storage.Reference, objeto, revision string) (ficheroEnMemoria, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.ficheros[ref]
	if !ok || f.peticion.ObjectID != objeto || f.peticion.RevisionID != revision {
		return ficheroEnMemoria{}, ErrNoEsta
	}
	return f, nil
}

func (m *Memoria) StorageFileInfo(_ context.Context, ref storage.Reference, objeto, revision string) (storage.FileRequest, error) {
	f, err := m.fichero(ref, objeto, revision)
	return f.peticion, err
}

func (m *Memoria) DownloadStorageFile(ctx context.Context, _ *api.Drive, ref storage.Reference, objeto, revision string, out io.Writer) (storage.FileRequest, error) {
	f, err := m.fichero(ref, objeto, revision)
	if err != nil {
		return storage.FileRequest{}, err
	}
	_, err = io.Copy(out, bytes.NewReader(f.datos))
	if err == nil {
		err = ctx.Err()
	}
	return f.peticion, err
}

// AcknowledgeStorageFile cierra el trabajo de subida cuando el índice ya cita el fichero.
func (m *Memoria) AcknowledgeStorageFile(ctx context.Context, _ *offline.Store, trabajo string, indice storage.Reference) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.trabajos[trabajo]
	if !ok {
		return storage.ErrUploadMissing
	}
	v, err := storage.DecodeObject(storage.RecordEntry{Reference: indice, Record: m.records[indice]}, m.scope)
	if err != nil {
		return err
	}
	if v.Commit.Content == nil || v.Commit.Content.Reference != ref {
		return ErrMemoria
	}
	delete(m.trabajos, trabajo)
	return ctx.Err()
}

// StorageFileCommitment describe el fichero y da la huella de sus bytes.
func (m *Memoria) StorageFileCommitment(ctx context.Context, ref storage.Reference, objeto, revision string) (storage.FileRequest, string, error) {
	f, err := m.fichero(ref, objeto, revision)
	if err != nil {
		return storage.FileRequest{}, "", err
	}
	suma := sha256.Sum256(f.datos)
	req := f.peticion
	req.Metadata = slices.Clone(req.Metadata)
	return req, hex.EncodeToString(suma[:]), ctx.Err()
}

// ReadStorageFileRange lee un trozo del fichero.
func (m *Memoria) ReadStorageFileRange(ctx context.Context, _ *api.Drive, ref storage.Reference, objeto, revision string, offset, length int64) ([]byte, error) {
	f, err := m.fichero(ref, objeto, revision)
	if err != nil {
		return nil, err
	}
	if offset < 0 || length < 0 || offset > int64(len(f.datos)) {
		return nil, ErrMemoria
	}
	return slices.Clone(f.datos[offset:min(int64(len(f.datos)), offset+length)]), ctx.Err()
}

// AcknowledgeStorageFileLink cierra la subida de un fichero retenido: en memoria ya es
// definitiva al publicarse.
func (m *Memoria) AcknowledgeStorageFileLink(ctx context.Context, _ *offline.Store, _ string, _ storage.Reference) error {
	return ctx.Err()
}

// Ficheros cuenta los binarios publicados en la sala (los citados o no).
func (m *Memoria) Ficheros() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ficheros)
}

// OrigenVacio es la fuente de migración de una cuenta sin nada que traer, en memoria: la
// primera importación la congela con cero filas y después contesta siempre lo mismo.
type OrigenVacio struct {
	mu     sync.Mutex
	estado records.SnapshotState
}

var _ records.Source = (*OrigenVacio)(nil)

// NuevoOrigenVacio es la fuente vacía de scope, sin importar todavía.
func NuevoOrigenVacio(scope storage.Scope) *OrigenVacio {
	return &OrigenVacio{estado: records.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}}
}

func (o *OrigenVacio) State(_ context.Context, scope storage.Scope) (records.SnapshotState, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if scope != o.estado.Scope {
		return records.SnapshotState{}, records.ErrSource
	}
	return o.estado, nil
}

func (o *OrigenVacio) Begin(_ context.Context, scope storage.Scope, epoca string) (records.SnapshotState, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if scope != o.estado.Scope {
		return records.SnapshotState{}, records.ErrSource
	}
	// Como `almacen.SinOrigen`: congelada una vez, se queda como está.
	if o.estado.Mode == "legacy" {
		ahora := time.Now().UTC()
		o.estado.Mode, o.estado.Epoch, o.estado.StartedAt = "frozen", epoca, &ahora
	}
	return o.estado, nil
}

func (o *OrigenVacio) Page(_ context.Context, scope storage.Scope, epoca string, _ storage.LegacySourceCursor) (records.SnapshotPage, error) {
	return records.SnapshotPage{Scope: scope, Epoch: epoca, Rows: []storage.StreamSourceRow{}}, nil
}

func (o *OrigenVacio) Open(context.Context, storage.Scope, string, records.SnapshotRow) (io.ReadCloser, error) {
	return nil, ErrNoEsta
}
