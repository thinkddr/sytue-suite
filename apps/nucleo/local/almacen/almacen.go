// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package almacen es el puerto LOCAL y EN CLARO del motor de registros
// (`storage/records`) para Sytue Office, la edición sin cuenta ni servidor.
//
// Los repositorios de dominio de la edición conectada (notas, contactos, calendario,
// fotos, bases, hub…) se construyen sobre `records.Config`, que pide un `records.Crypto`:
// hoy lo cumple el cliente de Matrix, que cifra y publica en una sala. Este paquete lo
// cumple en disco: los registros van a un diario JSON por líneas y los binarios a ficheros
// con su huella, SIN CIFRAR. Así los repositorios se reutilizan enteros, sin tocarlos
// (PLAN-EDICION-LOCAL D-06), y la edición conectada no cambia en nada.
//
// # Qué queda sellado y por qué
//
// El motor exige además un diario transitorio sellado (`Journal.Encrypted()`), y esa
// exigencia protege a la edición conectada, así que no se relaja. Aquí se le da su clave
// desde un fichero 0600 del espacio local (D-08). Ese diario solo retiene unos instantes
// lo que se está escribiendo; los DATOS de la persona quedan en claro en este puerto.
//
// # En disco
//
//	<dir>/registros.jsonl   un registro por línea, en orden de escritura
//	<dir>/objetos/ab/<sha>  binarios (fotos, adjuntos, contenidos grandes), tal cual
//	<dir>/origen.json       el estado de la «migración» vacía (SinOrigen)
//	<dir>/diario/           el diario transitorio del motor
//	<dir>/diario.clave/     su clave, 0600
//	<dir>/cerrojo           el cerrojo del sistema entre instancias (compactar.go)
package almacen

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

var (
	// ErrAlmacen: una petición que no es de este almacén (otro alcance, otra sala).
	ErrAlmacen = errors.New("almacen: petición ajena a este almacén")
	// ErrNoEsta: el registro o el fichero no existe.
	ErrNoEsta = errors.New("almacen: no encontrado")
)

const dispositivo = "SYTUE-OFFICE"

// Puerto implementa records.Crypto sobre ficheros en claro.
type Puerto struct {
	dir     string
	scope   storage.Scope
	binding storage.Binding

	mu     sync.Mutex
	diario *os.File
	// soltar suelta el cerrojo COMPARTIDO del almacén (compactar.go): lo tiene cada
	// instancia abierta, y Compactar pide el exclusivo. nil donde el sistema no da
	// cerrojos.
	soltar func()
	// subiendo cuenta las subidas que están copiando fuera de mu: mientras haya alguna,
	// Compactar no barre `objetos/` (podría llevarse un objeto recién renombrado cuya
	// línea aún no está en el diario).
	subiendo int
	records  map[storage.Reference]storage.Record
	orden    []storage.Reference
	txns     map[string]storage.Reference
	ficheros map[storage.Reference]fichero
	trabajos map[string]storage.Reference
	// sello cambia con cada línea que cambia algo y cada vez que se vuelve a leer el
	// diario (CatalogStamp).
	sello uint64
	// leido es hasta dónde, en bytes y por líneas enteras, se ha leído el diario: lo que
	// haya después lo ha añadido otra instancia (seguir).
	leido int64
	// generacion sube cada vez que se vuelve a leer el diario del disco
	// (VerifiedContentGeneration).
	generacion uint64
}

type fichero struct {
	Peticion storage.FileRequest `json:"peticion"`
	SHA256   string              `json:"sha256"`
}

// linea es una entrada del diario en disco.
type linea struct {
	Tipo     string            `json:"t"` // registro | fichero | acuse
	Ref      storage.Reference `json:"ref,omitempty"`
	Txn      string            `json:"txn,omitempty"`
	Registro *storage.Record   `json:"registro,omitempty"`
	Fichero  *fichero          `json:"fichero,omitempty"`
	Trabajo  string            `json:"trabajo,omitempty"`
	Fecha    time.Time         `json:"fecha"`
}

var _ records.Crypto = (*Puerto)(nil)

// AbrirPuerto abre (y crea si falta) el puerto de scope en dir.
func AbrirPuerto(dir string, scope storage.Scope) (*Puerto, error) {
	if !scope.Valid() {
		return nil, ErrAlmacen
	}
	if err := os.MkdirAll(filepath.Join(dir, "objetos"), 0o700); err != nil {
		return nil, err
	}
	// El cerrojo compartido: varias instancias pueden tener el almacén abierto a la vez,
	// pero ninguna mientras otra lo compacta (que lo pide exclusivo).
	soltar, err := tomarCerrojo(dir, true, EsperaCerrojo)
	if err != nil {
		return nil, err
	}
	sala := "!local:" + scope.Product
	p := &Puerto{
		soltar: soltar,
		dir:    dir, scope: scope,
		binding:  storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: sala, DeviceID: dispositivo}, Rooms: []string{sala}},
		records:  map[storage.Reference]storage.Record{},
		txns:     map[string]storage.Reference{},
		ficheros: map[storage.Reference]fichero{},
		trabajos: map[string]storage.Reference{},
	}
	// Con el cerrojo compartido nadie está compactando: un temporal de una compactación
	// es de una que se cortó a medias, y se tira (el diario bueno es el de siempre).
	limpiarTemporales(dir)
	ruta := filepath.Join(dir, "registros.jsonl")
	if err := p.cargar(ruta); err != nil {
		p.soltarCerrojo()
		return nil, err
	}
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		p.soltarCerrojo()
		return nil, err
	}
	p.diario = f
	return p, nil
}

func (p *Puerto) soltarCerrojo() {
	if p.soltar != nil {
		p.soltar()
		p.soltar = nil
	}
}

func (p *Puerto) cargar(ruta string) error {
	f, err := os.Open(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	return p.leerDesde(f, ruta)
}

// leerDesde aplica las líneas enteras de f desde donde esté y deja leido al final de la
// última. Una última línea sin salto es una escritura cortada a medias (o la de otra
// instancia que aún está escribiendo): no se aceptó nunca (no se devolvió su
// referencia), así que no se aplica y leido se queda antes de ella.
func (p *Puerto) leerDesde(f io.Reader, ruta string) error {
	lector := bufio.NewReader(f)
	for {
		raw, err := lector.ReadBytes('\n')
		if len(raw) > 0 && raw[len(raw)-1] == '\n' {
			l, ok := leerLinea(raw)
			if !ok {
				return fmt.Errorf("almacen: %s está dañado", ruta)
			}
			p.aplicar(l)
			p.leido += int64(len(raw))
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// marcaDeLinea es como empieza cada línea del diario (`json.Marshal` de una linea, cuyo
// primer campo es «t»). No sale en ningún otro sitio de una línea: ningún otro campo se
// llama «t», y dentro de una cadena las comillas van escapadas.
var marcaDeLinea = []byte(`{"t":"`)

// leerLinea descodifica una línea entera del diario. Una que no se entiende pero acaba
// en una que sí es una escritura cortada a medias (la luz se fue antes de su salto de
// línea) a la que se pegó la siguiente al añadirla: el trozo cortado nunca se aceptó y
// se ignora. Si no, el diario está dañado.
func leerLinea(raw []byte) (linea, bool) {
	var l linea
	if json.Unmarshal(raw, &l) == nil {
		return l, true
	}
	if i := bytes.LastIndex(raw, marcaDeLinea); i > 0 && json.Unmarshal(raw[i:], &l) == nil {
		return l, true
	}
	return linea{}, false
}

// aplicar aplica una línea del diario. Una que ya se aplicó (lo propio que vuelve a
// leer seguir) no cambia nada, y el sello solo cambia con lo nuevo. La última línea de
// una referencia es la que cuenta.
func (p *Puerto) aplicar(l linea) {
	cambia := false
	switch l.Tipo {
	case "registro":
		if l.Registro != nil {
			if _, ok := p.records[l.Ref]; !ok {
				p.orden = append(p.orden, l.Ref)
				cambia = true
			}
			p.records[l.Ref] = *l.Registro
			if l.Txn != "" {
				p.txns[l.Txn] = l.Ref
			}
		}
	case "fichero":
		if l.Fichero != nil {
			_, ok := p.ficheros[l.Ref]
			cambia = !ok
			p.ficheros[l.Ref] = *l.Fichero
			if l.Trabajo != "" {
				p.trabajos[l.Trabajo] = l.Ref
			}
		}
	case "acuse":
		_, cambia = p.trabajos[l.Trabajo]
		delete(p.trabajos, l.Trabajo)
	}
	if cambia {
		p.sello++
	}
}

// seguir aplica lo que otra instancia (otro proceso: el Hub al traer, otra app) ha
// añadido al diario desde la última vez que se leyó. Así este puerto, como la sala de la
// conectada, ve lo que escribe otro en cuanto el motor vuelve a leer (CatalogStamp,
// StorageObjectHistory), y una escritura suya no va sobre lo de antes: sin esto, el
// segundo que cambiaba un objeto lo hacía sobre la versión que tenía en memoria, el
// objeto quedaba con dos cabezas y el almacén ya no abría (records.ErrConflict). Sin
// nada nuevo es un fstat. Con p.mu.
func (p *Puerto) seguir() error {
	if p.diario == nil {
		return nil
	}
	info, err := p.diario.Stat()
	if err != nil || info.Size() <= p.leido {
		return err
	}
	ruta := filepath.Join(p.dir, ficheroDiario)
	f, err := os.Open(ruta) //nolint:gosec // G304: el diario del almacén
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(p.leido, io.SeekStart); err != nil {
		return err
	}
	return p.leerDesde(f, ruta)
}

// anotar escribe y sincroniza una línea antes de darla por aceptada. Con p.mu.
func (p *Puerto) anotar(l linea) error {
	// Lo que haya añadido otra instancia, antes: así se sabe dónde acaba lo leído.
	if err := p.seguir(); err != nil {
		return err
	}
	l.Fecha = time.Now().UTC()
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	antes, err := p.diario.Stat()
	if err != nil {
		return err
	}
	if _, err := p.diario.Write(raw); err != nil {
		return err
	}
	if err := p.diario.Sync(); err != nil {
		return err
	}
	// Si nadie más ha escrito entre medias, lo leído llega hasta esta línea; si no, la
	// próxima vez se lee desde antes y lo propio no cambia nada (aplicar).
	if despues, err := p.diario.Stat(); err == nil && antes.Size() == p.leido && despues.Size() == antes.Size()+int64(len(raw)) {
		p.leido = despues.Size()
	}
	p.aplicar(l)
	return nil
}

// Cerrar cierra el diario del puerto.
func (p *Puerto) Cerrar() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.diario == nil {
		return nil
	}
	err := p.diario.Close()
	p.diario = nil
	p.soltarCerrojo()
	return err
}

func nuevoEvento() string { return "$" + uuid.NewString() }

// CatalogStamp es records.CatalogStamper: lo que lee el motor de este puerto solo cambia
// a través de él (una línea nueva aplicada, la de otro proceso al seguir el diario, o el
// diario leído otra vez al compactar), así que, mientras el sello no cambie, el motor no
// vuelve a cargar el catálogo entero en cada escritura. Antes de darlo se sigue el
// diario: lo que haya escrito otra instancia lo cambia, y el motor lo carga.
func (p *Puerto) CatalogStamp() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.seguir(); err != nil {
		// Lo dirá StorageObjectHistory, que sigue el diario otra vez.
		slog.Debug("almacen: no se pudo seguir el diario", "dir", p.dir, "error", err)
		p.sello++
	}
	return p.sello
}

var _ records.CatalogStamper = (*Puerto)(nil)

// VerifiedContentGeneration es records.VerifiedContentCacher: un registro de este
// puerto no cambia bajo su referencia (cada línea del diario lleva una nueva) mientras no
// se vuelva a leer el diario del disco, y ya está en claro en el disco y en memoria, así
// que el motor puede recordar lo que ya comprobó en vez de repetirlo en cada lista. Al
// releer el diario (compactar) lo leído puede haber cambiado por fuera: la generación
// sube y el motor lo vuelve a comprobar todo.
func (p *Puerto) VerifiedContentGeneration() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generacion
}

var _ records.VerifiedContentCacher = (*Puerto)(nil)

// StorageIdentityVersion: identidades aisladas, las de la edición actual.
func (p *Puerto) StorageIdentityVersion() storage.IdentityVersion { return storage.IsolatedIdentity }

// StorageBindings devuelve la sala sintética del producto.
func (p *Puerto) StorageBindings(context.Context) (storage.Binding, error) { return p.binding, nil }

// PrepareStorage acepta solo el alcance de este puerto.
func (p *Puerto) PrepareStorage(_ context.Context, scope storage.Scope) (storage.Binding, error) {
	if scope != p.scope {
		return storage.Binding{}, ErrAlmacen
	}
	return p.binding, nil
}

// CheckStoragePublicationTarget acepta solo el destino de este puerto.
func (p *Puerto) CheckStoragePublicationTarget(t storage.PublicationTarget) error {
	if t != p.binding.Target {
		return ErrAlmacen
	}
	return nil
}

// StorageObjectHistory devuelve los índices de la sala, en orden de escritura, con lo
// que haya añadido otra instancia (seguir).
func (p *Puerto) StorageObjectHistory(_ context.Context, sala, cursor string, _ int) (storage.ObjectPage, error) {
	if cursor != "" || sala != p.binding.Target.RoomID {
		return storage.ObjectPage{}, ErrAlmacen
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.seguir(); err != nil {
		return storage.ObjectPage{}, err
	}
	var out []storage.RecordEntry
	for _, ref := range p.orden {
		if r := p.records[ref]; r.Role == "index" {
			out = append(out, storage.RecordEntry{Reference: ref, Record: clonar(r)})
		}
	}
	return storage.ObjectPage{Entries: out}, nil
}

func clonar(r storage.Record) storage.Record {
	r.Payload = slices.Clone(r.Payload)
	r.Previous = slices.Clone(r.Previous)
	return r
}

// ReadStorageRecord lee un registro por su referencia.
func (p *Puerto) ReadStorageRecord(_ context.Context, ref storage.Reference, objeto, rol string) (storage.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.records[ref]
	if !ok || r.ObjectID != objeto || r.Role != rol {
		return storage.Record{}, ErrNoEsta
	}
	return clonar(r), nil
}

// PutStorageRecordAt guarda un registro; la misma transacción devuelve la misma referencia.
func (p *Puerto) PutStorageRecordAt(_ context.Context, t storage.PublicationTarget, txn string, r storage.Record) (storage.Reference, error) {
	if err := p.CheckStoragePublicationTarget(t); err != nil {
		return storage.Reference{}, err
	}
	if r.Validate(p.scope) != nil {
		return storage.Reference{}, ErrAlmacen
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if ref, ok := p.txns[txn]; ok {
		return ref, nil
	}
	ref := storage.Reference{RoomID: t.RoomID, EventID: nuevoEvento()}
	r = clonar(r)
	if err := p.anotar(linea{Tipo: "registro", Ref: ref, Txn: txn, Registro: &r}); err != nil {
		return storage.Reference{}, err
	}
	return ref, nil
}

// PutStorageObjectAt guarda el índice de una revisión de objeto.
func (p *Puerto) PutStorageObjectAt(ctx context.Context, t storage.PublicationTarget, txn, objeto, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	payload, err := json.Marshal(commit)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	previos := make([]string, len(commit.Parents))
	for i, padre := range commit.Parents {
		previos[i] = padre.EventID
	}
	ref, err := p.PutStorageRecordAt(ctx, t, txn, storage.Record{Version: 1, Scope: p.scope, ObjectID: objeto, RevisionID: revision, Role: "index", Payload: payload, Previous: previos})
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return p.StorageObject(ctx, ref, objeto)
}

// StorageObject decodifica el índice ref.
func (p *Puerto) StorageObject(ctx context.Context, ref storage.Reference, objeto string) (storage.ObjectVersion, error) {
	r, err := p.ReadStorageRecord(ctx, ref, objeto, "index")
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: r}, p.scope)
}

func (p *Puerto) rutaObjeto(sha string) string {
	return filepath.Join(p.dir, "objetos", sha[:2], sha)
}

// RutaEnClaro es dónde está en el disco el binario con esa huella (SHA-256 en
// hexadecimal), en claro y tal cual entró, si está. Es para dárselo por su ruta a quien
// solo sabe leer ficheros (en Photos, el descodificador de vídeo del sistema) sin copiar
// un vídeo de varios GB. SOLO para leer: escribir ahí estropea el almacén, y compactar
// puede quitarlo en cuanto nadie lo cite.
func (p *Puerto) RutaEnClaro(sha string) (string, bool) {
	sha = strings.ToLower(sha)
	if b, err := hex.DecodeString(sha); err != nil || len(b) != sha256.Size {
		return "", false
	}
	ruta := p.rutaObjeto(sha)
	if info, err := os.Stat(ruta); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return ruta, true
}

// UploadStorageFileAt guarda un binario en claro, con su huella como nombre.
func (p *Puerto) UploadStorageFileAt(_ context.Context, _ *api.Drive, _ *offline.Store, trabajo string, t storage.PublicationTarget, req storage.FileRequest, src io.ReaderAt) (storage.Reference, error) {
	if err := p.CheckStoragePublicationTarget(t); err != nil {
		return storage.Reference{}, err
	}
	if req.Validate(p.scope) != nil {
		return storage.Reference{}, ErrAlmacen
	}
	p.mu.Lock()
	if ref, ok := p.trabajos[trabajo]; ok {
		p.mu.Unlock()
		return ref, nil
	}
	p.subiendo++
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.subiendo--
		p.mu.Unlock()
	}()
	// Se copia fuera del cerrojo: puede ser una foto de varios MB.
	tmp, err := os.CreateTemp(filepath.Join(p.dir, "objetos"), ".subida-*")
	if err != nil {
		return storage.Reference{}, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.NewSectionReader(src, 0, req.SizeBytes))
	if err == nil && n != req.SizeBytes {
		err = io.ErrUnexpectedEOF
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return storage.Reference{}, err
	}
	sha := hex.EncodeToString(h.Sum(nil))
	destino := p.rutaObjeto(sha)
	if err := os.MkdirAll(filepath.Dir(destino), 0o700); err != nil {
		return storage.Reference{}, err
	}
	if err := os.Rename(tmp.Name(), destino); err != nil {
		return storage.Reference{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if ref, ok := p.trabajos[trabajo]; ok {
		return ref, nil
	}
	ref := storage.Reference{RoomID: t.RoomID, EventID: nuevoEvento()}
	req.Metadata = slices.Clone(req.Metadata)
	if err := p.anotar(linea{Tipo: "fichero", Ref: ref, Trabajo: trabajo, Fichero: &fichero{Peticion: req, SHA256: sha}}); err != nil {
		return storage.Reference{}, err
	}
	return ref, nil
}

// StorageFileInfo describe un binario guardado.
func (p *Puerto) StorageFileInfo(_ context.Context, ref storage.Reference, objeto, revision string) (storage.FileRequest, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.ficheros[ref]
	if !ok || f.Peticion.ObjectID != objeto || f.Peticion.RevisionID != revision {
		return storage.FileRequest{}, ErrNoEsta
	}
	return f.Peticion, nil
}

// DownloadStorageFile copia un binario a out, comprobando su huella.
func (p *Puerto) DownloadStorageFile(_ context.Context, _ *api.Drive, ref storage.Reference, objeto, revision string, out io.Writer) (storage.FileRequest, error) {
	p.mu.Lock()
	f, ok := p.ficheros[ref]
	p.mu.Unlock()
	if !ok || f.Peticion.ObjectID != objeto || f.Peticion.RevisionID != revision {
		return storage.FileRequest{}, ErrNoEsta
	}
	datos, err := os.ReadFile(p.rutaObjeto(f.SHA256))
	if err != nil {
		return f.Peticion, err
	}
	if sum := sha256.Sum256(datos); hex.EncodeToString(sum[:]) != f.SHA256 {
		return f.Peticion, fmt.Errorf("almacen: el objeto %s no coincide con su huella", f.SHA256)
	}
	_, err = out.Write(datos)
	return f.Peticion, err
}

// AcknowledgeStorageFile cierra el trabajo de subida cuando su índice ya lo cita.
func (p *Puerto) AcknowledgeStorageFile(ctx context.Context, _ *offline.Store, trabajo string, indice storage.Reference) error {
	p.mu.Lock()
	ref, ok := p.trabajos[trabajo]
	r := p.records[indice]
	p.mu.Unlock()
	if !ok {
		return storage.ErrUploadMissing
	}
	v, err := storage.DecodeObject(storage.RecordEntry{Reference: indice, Record: r}, p.scope)
	if err != nil {
		return err
	}
	if v.Commit.Content == nil || v.Commit.Content.Reference != ref {
		return ErrAlmacen
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.anotar(linea{Tipo: "acuse", Trabajo: trabajo}); err != nil {
		return err
	}
	return ctx.Err()
}

// ----- Ficheros retenidos -----
//
// Photos guarda junto a cada registro su original y sus variantes como ficheros
// RETENIDOS (`records.StageRetainedFile`, `ReadRetainedFile`, `OpenBinary`). El motor
// descubre estas tres capacidades por aserción de tipo sobre `records.Config.Crypto`.

// ErrRango es una lectura parcial fuera del fichero.
var ErrRango = errors.New("almacen: rango fuera del fichero")

// ficheroDe busca el binario ref de objeto y revisión.
func (p *Puerto) ficheroDe(ref storage.Reference, objeto, revision string) (fichero, error) {
	p.mu.Lock()
	f, ok := p.ficheros[ref]
	p.mu.Unlock()
	if !ok || f.Peticion.ObjectID != objeto || f.Peticion.RevisionID != revision {
		return fichero{}, ErrNoEsta
	}
	return f, nil
}

// StorageFileCommitment describe el fichero guardado y da la huella SHA-256 de sus
// bytes en claro, recalculada sobre el disco (no la anotada): así el motor detecta un
// objeto cambiado por fuera.
func (p *Puerto) StorageFileCommitment(ctx context.Context, ref storage.Reference, objeto, revision string) (storage.FileRequest, string, error) {
	f, err := p.ficheroDe(ref, objeto, revision)
	if err != nil {
		return storage.FileRequest{}, "", err
	}
	o, err := os.Open(p.rutaObjeto(f.SHA256))
	if err != nil {
		return storage.FileRequest{}, "", err
	}
	defer o.Close()
	h := sha256.New()
	if _, err := io.Copy(h, o); err != nil {
		return storage.FileRequest{}, "", err
	}
	req := f.Peticion
	req.Metadata = slices.Clone(req.Metadata)
	return req, hex.EncodeToString(h.Sum(nil)), ctx.Err()
}

// ReadStorageFileRange lee length bytes desde offset, sin cargar el fichero entero (el
// lector del motor pide trozos de 1 MiB para buscar en un vídeo o una foto grande). La
// huella del conjunto la comprueba el motor con StorageFileCommitment.
func (p *Puerto) ReadStorageFileRange(ctx context.Context, _ *api.Drive, ref storage.Reference, objeto, revision string, offset, length int64) ([]byte, error) {
	if offset < 0 || length < 0 {
		return nil, ErrRango
	}
	f, err := p.ficheroDe(ref, objeto, revision)
	if err != nil {
		return nil, err
	}
	o, err := os.Open(p.rutaObjeto(f.SHA256))
	if err != nil {
		return nil, err
	}
	defer o.Close()
	info, err := o.Stat()
	if err != nil {
		return nil, err
	}
	if offset > info.Size() {
		return nil, ErrRango
	}
	datos := make([]byte, min(info.Size()-offset, length))
	if _, err := o.ReadAt(datos, offset); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return datos, ctx.Err()
}

// AcknowledgeStorageFileLink cierra la subida de un fichero retenido. En este puerto la
// subida ya es definitiva al escribirse (renombrado atómico y línea en el diario), así
// que no queda nada pendiente que soltar.
func (p *Puerto) AcknowledgeStorageFileLink(ctx context.Context, _ *offline.Store, _ string, _ storage.Reference) error {
	return ctx.Err()
}

// ----- SinOrigen -----

// SinOrigen es la «fuente de migración» de una edición que nunca tuvo servidor: dice
// que no hay nada que traer. La primera apertura la congela con cero filas y el motor
// publica su recibo; después siempre contesta lo mismo, guardado en disco.
type SinOrigen struct {
	ruta string
	mu   sync.Mutex
}

var _ records.Source = (*SinOrigen)(nil)

// NuevoSinOrigen guarda su estado en ruta.
func NuevoSinOrigen(ruta string) *SinOrigen { return &SinOrigen{ruta: ruta} }

// State devuelve «legacy» hasta la primera importación y «frozen» después.
func (s *SinOrigen) State(_ context.Context, scope storage.Scope) (records.SnapshotState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.ruta)
	if errors.Is(err, os.ErrNotExist) {
		return records.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, nil
	}
	if err != nil {
		return records.SnapshotState{}, err
	}
	var st records.SnapshotState
	if err := json.Unmarshal(raw, &st); err != nil {
		return records.SnapshotState{}, err
	}
	if st.Scope != scope {
		return records.SnapshotState{}, ErrAlmacen
	}
	return st, nil
}

// Begin congela la fuente, vacía, con la época que pide el motor.
func (s *SinOrigen) Begin(_ context.Context, scope storage.Scope, epoca string) (records.SnapshotState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if raw, err := os.ReadFile(s.ruta); err == nil {
		var st records.SnapshotState
		if json.Unmarshal(raw, &st) == nil && st.Mode == "frozen" {
			return st, nil
		}
	}
	ahora := time.Now().UTC()
	st := records.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoca, StartedAt: &ahora}}
	raw, err := json.MarshalIndent(st, "", " ")
	if err != nil {
		return st, err
	}
	return st, escribirAtomico(s.ruta, raw)
}

// Page: no hay filas.
func (s *SinOrigen) Page(_ context.Context, scope storage.Scope, epoca string, _ storage.LegacySourceCursor) (records.SnapshotPage, error) {
	return records.SnapshotPage{Scope: scope, Epoch: epoca}, nil
}

// Open: no hay nada que abrir.
func (s *SinOrigen) Open(context.Context, storage.Scope, string, records.SnapshotRow) (io.ReadCloser, error) {
	return nil, ErrNoEsta
}

// ----- Clave del diario -----

// ClaveEnFichero cumple offline.CacheSecrets con ficheros 0600 en una carpeta 0700.
type ClaveEnFichero struct{ Dir string }

var _ offline.CacheSecrets = ClaveEnFichero{}

func (c ClaveEnFichero) ruta(clave string) string {
	sum := sha256.Sum256([]byte(clave))
	return filepath.Join(c.Dir, hex.EncodeToString(sum[:16]))
}

// Read lee el secreto clave.
func (c ClaveEnFichero) Read(clave string) ([]byte, error) {
	raw, err := os.ReadFile(c.ruta(clave))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // como el llavero de pruebas: sin secreto, el diario crea uno
	}
	return raw, err
}

// Write guarda el secreto clave con permisos 0600.
func (c ClaveEnFichero) Write(clave string, valor []byte) error {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return err
	}
	return escribirAtomico(c.ruta(clave), valor)
}

// Delete borra el secreto clave.
func (c ClaveEnFichero) Delete(clave string) error {
	err := os.Remove(c.ruta(clave))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func escribirAtomico(ruta string, datos []byte) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(ruta), "."+filepath.Base(ruta)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, bytes.NewReader(datos)); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), ruta)
}

// ----- Recursos -----

// Recursos es todo lo que un repositorio de dominio necesita para abrirse en local.
type Recursos struct {
	Config records.Config
	Puerto *Puerto
	// CompactarAlCerrar compacta el almacén al cerrarlo (compactar.go), cuando el
	// repositorio ya está cerrado y nada lo tiene en memoria: es el momento seguro. Si
	// otra instancia lo tiene abierto o queda trabajo a medias, no se compacta y no pasa
	// nada. Falso, como siempre.
	CompactarAlCerrar bool
	// ya son las compactaciones en el acto (CompactarYa).
	ya compactador
}

// Cerrar suelta el puerto y el diario; antes compacta si se pidió (CompactarAlCerrar).
func (r *Recursos) Cerrar() {
	if r == nil {
		return
	}
	r.PararCompactar()
	if r.CompactarAlCerrar {
		ctx, fin := context.WithTimeout(context.Background(), 2*time.Minute)
		c, err := r.Compactar(ctx)
		fin()
		switch {
		case err != nil:
			slog.Debug("almacen: no se compacta al cerrar", "producto", r.Config.Scope.Product, "error", err)
		case c.Algo():
			slog.Info("almacen: compactado al cerrar", "producto", r.Config.Scope.Product,
				"lineas", c.LineasAntes, "quedan", c.LineasDespues, "objetos", c.Objetos, "liberados", c.Liberados)
		}
	}
	_ = r.Puerto.Cerrar()
}

// Abrir prepara la configuración del motor para producto en dir, con la persona sujeto.
// El contexto vive lo que viva la app.
func Abrir(ctx context.Context, dir, producto, sujeto string) (*Recursos, error) {
	scope := storage.Scope{Tenant: "local", Subject: sujeto, Product: producto}
	puerto, err := AbrirPuerto(dir, scope)
	if err != nil {
		return nil, err
	}
	diario, err := offline.OpenWithSecretsContext(ctx, filepath.Join(dir, "diario"), ClaveEnFichero{Dir: filepath.Join(dir, "diario.clave")})
	if err != nil {
		_ = puerto.Cerrar()
		return nil, err
	}
	return &Recursos{Puerto: puerto, Config: records.Config{
		Context: ctx,
		Scope:   scope,
		Crypto:  puerto,
		// El motor solo se lo entrega a Crypto, que no lo usa: nunca hace una petición.
		Gateway: api.NewDrive("https://sin-red.invalid", nil, nil),
		Journal: diario,
		Source:  NuevoSinOrigen(filepath.Join(dir, "origen.json")),
	}}, nil
}

// Inicializable es lo que tienen en común los repositorios de dominio del motor.
type Inicializable interface {
	Initialize(context.Context) error
	ImportLegacySnapshot(context.Context) error
}

// Preparar abre el catálogo de repo; la primera vez, «importa» la fuente vacía.
func Preparar(ctx context.Context, repo Inicializable) error {
	err := repo.Initialize(ctx)
	if errors.Is(err, records.ErrNotReady) {
		if err = repo.ImportLegacySnapshot(ctx); err == nil {
			err = repo.Initialize(ctx)
		}
	}
	return err
}

// IDInstalacion devuelve un identificador estable y aleatorio para esta instalación,
// guardado en ruta. Es el «sujeto» de los alcances: no identifica a nadie fuera.
func IDInstalacion(ruta string) (string, error) {
	if raw, err := os.ReadFile(ruta); err == nil {
		if id := strings.TrimSpace(string(raw)); id != "" {
			return id, nil
		}
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b[:])
	return id, escribirAtomico(ruta, []byte(id+"\n"))
}
