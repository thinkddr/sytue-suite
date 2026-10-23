// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package offline es @sytue/offline en Go: un almacén clave-valor por dueño y espacio
// (storage.ts), el estado de conectividad (native.ts) y la caché «red primero» con sus
// reglas de privacidad (worker.ts).
//
// NO hay cola de mutaciones, aunque el encargo de la migración la diera por hecha: el
// paquete original no la tiene. Reintentar escrituras al volver la red obliga a decidir
// qué pasa cuando el servidor cambió mientras tanto, y eso es una decisión de producto
// por servicio, no fontanería que se pueda portar.
package offline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
	"sync"
	"time"
)

// Record es un valor guardado, con su fecha: lo que devuelve List.
type Record struct {
	ID        string          `json:"id"`
	Value     json.RawMessage `json:"value"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// backend es dónde caen los bytes: ficheros en nativo, localStorage en la web. get
// devuelve fs.ErrNotExist cuando la clave no está.
type backend interface {
	get(clave string) ([]byte, error)
	put(clave string, datos []byte) error
	del(clave string) error
	list(prefijo string) ([][]byte, error)
	clear() error
	// contar dice cuántos valores hay en un espacio (ya como clave) de TODOS los dueños.
	contar(espacio string) (int, error)
}

// Store guarda valores por (dueño, espacio, id). El dueño es el `sub` de la persona:
// dos cuentas en el mismo equipo no ven lo de la otra.
type Store struct {
	mu sync.Mutex
	b  backend
}

// clave nunca lleva texto de fuera: cada parte es un hash, así que un id como
// «../../etc/passwd» no puede salirse del directorio.
func clave(partes ...string) string {
	out := ""
	for i, p := range partes {
		suma := sha256.Sum256([]byte(p))
		if i > 0 {
			out += "/"
		}
		out += hex.EncodeToString(suma[:16])
	}
	return out
}

// Read carga el valor en v y dice si estaba.
func (s *Store) Read(owner, namespace, id string, v any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.leer(clave(owner, namespace, id))
	if r == nil || err != nil {
		return false, err
	}
	return true, json.Unmarshal(r.Value, v)
}

func (s *Store) leer(k string) (*Record, error) {
	crudo, err := s.b.get(k)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil //nolint:nilnil // «no está» no es un error: es la mitad de las lecturas
	}
	if err != nil {
		return nil, err
	}
	var r Record
	if err := json.Unmarshal(crudo, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Write guarda v.
func (s *Store) Write(owner, namespace, id string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := encodeRecord(id, v)
	if err != nil {
		return err
	}
	return s.b.put(clave(owner, namespace, id), record)
}

// Delete borra el valor; que no estuviera no es un error.
func (s *Store) Delete(owner, namespace, id string) error {
	return s.Mutate(owner, namespace, id, func(json.RawMessage) (any, error) { return nil, nil })
}

// Mutate lee, transforma y escribe sin que nadie se cuele en medio. `cambiar` recibe el
// valor anterior (nil si no había) y devuelve el nuevo, o nil para borrarlo. Si devuelve
// un error no se toca nada.
func (s *Store) Mutate(owner, namespace, id string, cambiar func(anterior json.RawMessage) (any, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := clave(owner, namespace, id)
	previo, err := s.leer(k)
	if err != nil {
		return err
	}
	var anterior json.RawMessage
	if previo != nil {
		anterior = previo.Value
	}
	nuevo, err := cambiar(anterior)
	if err != nil {
		return err
	}
	if nuevo == nil {
		return s.b.del(k)
	}
	crudo, err := encodeRecord(id, nuevo)
	if err != nil {
		return err
	}
	return s.b.put(k, crudo)
}

func encodeRecord(id string, v any) ([]byte, error) {
	value, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Record{ID: id, Value: value, UpdatedAt: time.Now()})
}

// List devuelve lo guardado en un espacio, lo más reciente primero.
func (s *Store) List(owner, namespace string) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	crudos, err := s.b.list(clave(owner, namespace))
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(crudos))
	for _, c := range crudos {
		var r Record
		if err := json.Unmarshal(c, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// espacioHTTP es el espacio donde la caché «red primero» guarda las respuestas de un
// producto. Tiene nombre para que quien guarda (transport.go) y quien vacía hablen del mismo.
func espacioHTTP(producto string) string { return "http:" + producto }

// Count cuenta lo guardado en esos espacios, de TODOS los dueños: lo que se perdería
// con [Store.Clear]. Al cerrar sesión, es lo que dice si hay algo de lo que avisar.
func (s *Store) Count(espacios ...string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, e := range espacios {
		n, err := s.b.contar(clave(e))
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// Clear lo borra TODO, de todos los dueños. Es lo que hace la web al cerrar sesión
// (`indexedDB.deleteDatabase`), y se lleva también la caché de respuestas.
func (s *Store) Clear() error {
	// The file backend shares the HTTP-write lock across app processes. Acquire it
	// before mu, in the same order as writeHTTP, so logout cannot remove its inode
	// or race a cache commit that was already in progress.
	if locker, ok := s.b.(interface{ lockClear() (func(), error) }); ok {
		unlock, err := locker.lockClear()
		if err != nil {
			return err
		}
		defer unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.clear()
}
