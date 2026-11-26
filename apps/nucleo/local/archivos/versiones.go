// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

/*
VERSIONES (P-02, DR-02). Cada fichero que se sobrescribe DESDE LA SUITE tiene una carpeta
`<raíz>/.sytue/versiones/<clave>/` con:

  - `historial.json`: la ruta del fichero, el número de la versión actual y las archivadas;
  - `<n><ext>`: el contenido de cada versión archivada, en claro y con su extensión, así
    que se puede abrir a mano.

La versión actual es el propio fichero. Al sobrescribir, su contenido se copia como
versión `Actual` y el fichero nuevo pasa a ser `Actual+1`. Se guardan como mucho
`MaxVersiones`; la más vieja se borra.
*/

type historial struct {
	Ruta      string    `json:"ruta"`
	Actual    int       `json:"actual"`
	Versiones []version `json:"versiones,omitempty"`
}

type version struct {
	N      int    `json:"n"`
	Tamano int64  `json:"tamano"`
	Creada string `json:"creada"` // RFC 3339: la fecha del contenido archivado
	Autor  string `json:"autor,omitempty"`
}

func (a *Archivos) dirVersiones(r string) string { return dirVersionesEn(a.meta(), r) }

// dirVersionesEn es la carpeta de versiones de r en el `.sytue` meta: va por la huella
// de su ruta absoluta.
func dirVersionesEn(meta, r string) string {
	h := sha256.Sum256([]byte(filepath.ToSlash(r)))
	return filepath.Join(meta, "versiones", hex.EncodeToString(h[:16]))
}

func (a *Archivos) historial(r string) historial {
	h := historial{Ruta: r, Actual: 1}
	raw, err := a.fs().ReadFile(filepath.Join(a.dirVersiones(r), "historial.json"))
	if err == nil && json.Unmarshal(raw, &h) == nil && h.Actual < 1 {
		h.Actual = 1
	}
	return h
}

func (a *Archivos) guardarHistorial(r string, h historial) error {
	raw, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return a.fs().EscribirAtomico(filepath.Join(a.dirVersiones(r), "historial.json"), append(raw, '\n'))
}

func (a *Archivos) rutaVersion(r string, n int) string {
	return filepath.Join(a.dirVersiones(r), strconv.Itoa(n)+filepath.Ext(r))
}

// archivar copia el contenido actual de r como versión archivada, antes de sobrescribirlo.
func (a *Archivos) archivar(r string, fi fs.FileInfo) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	h := a.historial(r)
	h.Ruta = r
	dir := a.dirVersiones(r)
	if err := a.fs().MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := copiarAtomico(a.fs(), r, a.fs(), a.rutaVersion(r, h.Actual)); err != nil {
		return err
	}
	h.Versiones = append(h.Versiones, version{N: h.Actual, Tamano: fi.Size(), Creada: fi.ModTime().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), Autor: a.scope.Subject})
	h.Actual++
	for len(h.Versiones) > a.maxV {
		_ = a.fs().Remove(a.rutaVersion(r, h.Versiones[0].N))
		h.Versiones = h.Versiones[1:]
	}
	return a.guardarHistorial(r, h)
}

// Versions es el historial: la actual primero y después las archivadas, de la más nueva
// a la más vieja. Sin páginas.
func (a *Archivos) Versions(_ context.Context, id, _ string) ([]api.DriveVersion, string, error) {
	r, err := a.ruta(id)
	if err != nil {
		return nil, "", err
	}
	fi, err := a.fs().Stat(r)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, "", noEsta(id)
	}
	h := a.historial(r)
	out := []api.DriveVersion{{Version: h.Actual, SizeBytes: fi.Size(), AuthorSub: a.scope.Subject, IsCurrent: true, CreatedAt: fi.ModTime()}}
	for _, v := range slices.Backward(h.Versiones) {
		dv := api.DriveVersion{Version: v.N, SizeBytes: v.Tamano, AuthorSub: v.Autor}
		if vi, err := a.fs().Stat(a.rutaVersion(r, v.N)); err == nil {
			dv.CreatedAt = vi.ModTime()
		}
		_ = dv.CreatedAt.UnmarshalText([]byte(v.Creada))
		out = append(out, dv)
	}
	return out, "", nil
}

// DownloadVersion abre una versión: la actual es el propio fichero.
func (a *Archivos) DownloadVersion(_ context.Context, id string, n int) (io.ReadCloser, error) {
	r, err := a.ruta(id)
	if err != nil {
		return nil, err
	}
	h := a.historial(r)
	if n == h.Actual {
		return a.fs().Open(r)
	}
	if !slices.ContainsFunc(h.Versiones, func(v version) bool { return v.N == n }) {
		return nil, noEsta(id + "@" + strconv.Itoa(n))
	}
	f, err := a.fs().Open(a.rutaVersion(r, n))
	if errors.Is(err, os.ErrNotExist) {
		return nil, noEsta(id + "@" + strconv.Itoa(n))
	}
	return f, err
}

// RestoreVersion hace actual una versión archivada. No se pierde nada: lo que había pasa
// a ser una versión más.
func (a *Archivos) RestoreVersion(ctx context.Context, id string, n int) error {
	r, err := a.ruta(id)
	if err != nil {
		return err
	}
	if n == a.historial(r).Actual {
		return nil
	}
	in, err := a.DownloadVersion(ctx, id, n)
	if err != nil {
		return err
	}
	datos, err := io.ReadAll(in)
	_ = in.Close()
	if err != nil {
		return err
	}
	return a.escribir(ctx, r, bytes.NewReader(datos), int64(len(datos)))
}
