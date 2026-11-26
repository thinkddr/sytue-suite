// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package almacen

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// # Leer el almacén de OTRA app, sin abrirlo
//
// El Hub de Sytue Office enseña lo reciente y busca en las notas, los contactos y los
// eventos de las demás apps (PLAN-EDICION-LOCAL §6.15, HU-05). No puede abrirlas con
// [Abrir]: eso abre el diario para añadir, crea el diario transitorio del motor y su
// clave, y «congela» el origen; es decir, ESCRIBE en la carpeta de otra app, que puede
// estar abierta a la vez.
//
// [Leer] solo abre `registros.jsonl` para leer, sin cerrojos, y no crea nada. La app
// dueña sigue escribiendo mientras tanto: una última línea a medias (la está
// escribiendo ahora) se ignora igual que al cargar el puerto, y lo que entre después se
// verá en la siguiente lectura.

// Fila es una entidad viva del almacén en su última revisión: el JSON de dominio tal
// cual lo guardó el repositorio de su app (una nota, un contacto, un evento…).
type Fila struct {
	// Tipo es el del motor (`org.sytue.notes.entity.v1`); Entidad, la del dominio
	// (`note`, `contact`, `event`…), e ID su identificador.
	Tipo, Entidad, ID string
	JSON              []byte
	// Fecha es cuándo se escribió esta revisión en el diario.
	Fecha time.Time
}

// cabecera son los metadatos que el motor pone en el índice de cada entidad
// (`records.entity`). Se leen solo los campos que hacen falta aquí.
type cabecera struct {
	Kind   string `json:"kind"`
	Entity string `json:"entity"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Binary bool   `json:"binary,omitempty"`
}

// contenido es la carga de un registro `content` (`records.content`).
type contenido struct {
	Kind string `json:"kind"`
	JSON []byte `json:"json"`
}

// Leer devuelve las entidades vivas del almacén de dir, en el orden en que se escribió
// su última revisión. Una carpeta sin almacén no es un error: no hay nada. No escribe
// nada ni toma ningún cerrojo. Lo que no se entiende (una entidad binaria, un índice
// dañado, un contenido que no coincide con su huella) se salta: quien lee es un
// buscador, no el dueño de los datos.
//
//nolint:gocognit,gocyclo // leer el diario, quedarse con la última revisión y ordenar: tres pasos de corrido sobre los mismos mapas
func Leer(dir string) ([]Fila, error) {
	f, err := os.Open(filepath.Join(dir, "registros.jsonl")) //nolint:gosec // G304: la carpeta de datos de la edición
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	registros := map[storage.Reference]storage.Record{}
	fechas := map[storage.Reference]time.Time{}
	ficheros := map[storage.Reference]fichero{}
	var indices []storage.Reference
	lector := bufio.NewReader(f)
	for {
		raw, err := lector.ReadBytes('\n')
		if len(raw) > 0 && raw[len(raw)-1] == '\n' {
			l, ok := leerLinea(raw)
			if !ok {
				return nil, fmt.Errorf("almacen: %s está dañado", dir)
			}
			switch {
			case l.Tipo == "registro" && l.Registro != nil:
				if _, ya := registros[l.Ref]; !ya && l.Registro.Role == "index" {
					indices = append(indices, l.Ref)
				}
				registros[l.Ref], fechas[l.Ref] = *l.Registro, l.Fecha
			case l.Tipo == "fichero" && l.Fichero != nil:
				ficheros[l.Ref] = *l.Fichero
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	// La última revisión de cada objeto es la última que se escribió: el diario solo
	// crece y el puerto no reordena.
	ultima := map[string]int{}
	var objetos []string
	commits := make([]storage.ObjectCommit, len(indices))
	for i, ref := range indices {
		r := registros[ref]
		if json.Unmarshal(r.Payload, &commits[i]) != nil {
			continue
		}
		if _, ya := ultima[r.ObjectID]; !ya {
			objetos = append(objetos, r.ObjectID)
		}
		ultima[r.ObjectID] = i
	}
	type viva struct {
		pos  int
		fila Fila
	}
	var vivas []viva
	for _, obj := range objetos {
		i := ultima[obj]
		c := commits[i]
		var cab cabecera
		if c.State != "live" || c.Content == nil || json.Unmarshal(c.Metadata, &cab) != nil ||
			cab.Binary || cab.Entity == "" || cab.ID == "" || len(cab.SHA256) != 64 {
			continue
		}
		datos, ok := leerContenido(dir, registros, ficheros, obj, *c.Content)
		if !ok {
			continue
		}
		if sum := sha256.Sum256(datos); hex.EncodeToString(sum[:]) != cab.SHA256 || !json.Valid(datos) {
			continue
		}
		vivas = append(vivas, viva{i, Fila{Tipo: cab.Kind, Entidad: cab.Entity, ID: cab.ID, JSON: datos, Fecha: fechas[indices[i]]}})
	}
	// En el orden de su última escritura, no en el de su primera aparición.
	for a := 1; a < len(vivas); a++ {
		for b := a; b > 0 && vivas[b].pos < vivas[b-1].pos; b-- {
			vivas[b], vivas[b-1] = vivas[b-1], vivas[b]
		}
	}
	out := make([]Fila, len(vivas))
	for i, v := range vivas {
		out[i] = v.fila
	}
	return out, nil
}

// leerContenido es el JSON de dominio al que apunta un índice: en un registro
// `content` o, si es grande, en un objeto de `objetos/`.
func leerContenido(dir string, registros map[storage.Reference]storage.Record, ficheros map[storage.Reference]fichero, obj string, p storage.ContentPointer) ([]byte, bool) {
	switch p.Kind {
	case "record":
		r, ok := registros[p.Reference]
		var c contenido
		if !ok || r.Role != "content" || r.ObjectID != obj || r.RevisionID != p.RevisionID || json.Unmarshal(r.Payload, &c) != nil {
			return nil, false
		}
		return c.JSON, true
	case "file":
		f, ok := ficheros[p.Reference]
		if !ok || f.Peticion.ObjectID != obj || f.Peticion.RevisionID != p.RevisionID || len(f.SHA256) < 2 {
			return nil, false
		}
		datos, err := os.ReadFile(filepath.Join(dir, "objetos", f.SHA256[:2], f.SHA256)) //nolint:gosec // G304: objeto por su huella
		return datos, err == nil
	}
	return nil, false
}
