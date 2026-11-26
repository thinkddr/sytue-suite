// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// Las interfaces opcionales de la fachada (DR-03, Anexo A).
var (
	_ api.DrivePurger       = (*Archivos)(nil)
	_ api.DriveFolderCopier = (*Archivos)(nil)
	_ api.DriveBatcher      = (*Archivos)(nil)
	_ api.DriveQuotaReader  = (*Archivos)(nil)
)

// ----- Papelera: borrar del todo -----

// Purge borra de verdad algo de la papelera, con su historial de versiones. El
// identificador es el que tenía antes de tirarlo. No tiene vuelta atrás.
func (a *Archivos) Purge(_ context.Context, _ api.DriveKind, id string) error {
	raw, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return noEsta(id)
	}
	ruta := filepath.FromSlash(string(raw))
	a.mu.Lock()
	defer a.mu.Unlock()
	i := slices.IndexFunc(a.ind.Papelera, func(x enPapelera) bool { return x.Ruta == ruta })
	if i < 0 {
		return noEsta(id)
	}
	x := a.ind.Papelera[i]
	if err := a.fs().RemoveAll(filepath.Join(a.meta(), "papelera", x.Clave)); err != nil {
		return err
	}
	if !x.Carpeta {
		_ = a.fs().RemoveAll(a.dirVersiones(x.Ruta))
	}
	a.ind.Papelera = slices.Delete(a.ind.Papelera, i, i+1)
	return a.guardarIndice()
}

// ----- Copias -----

// CopyFolder copia una carpeta con todo lo que lleva dentro a su lado: «nombre (2)».
// Cada fichero se escribe a temporal y renombrado; si algo falla a medias, la copia
// incompleta se retira y no queda nada.
func (a *Archivos) CopyFolder(ctx context.Context, id string) (api.Folder, error) {
	r, err := a.ruta(id)
	if err != nil {
		return api.Folder{}, err
	}
	fi, err := a.fs().Stat(r)
	if err != nil || !fi.IsDir() || r == a.raiz() {
		return api.Folder{}, noEsta(id)
	}
	destino := nombreLibreEn(a.fs(), filepath.Dir(r), fi.Name())
	if err := copiarArbol(ctx, a.fs(), r, a.fs(), destino); err != nil {
		_ = a.fs().RemoveAll(destino)
		return api.Folder{}, err
	}
	di, err := a.fs().Stat(destino)
	if err != nil {
		return api.Folder{}, err
	}
	return a.carpeta(destino, di), nil
}

// CopiarFuera deja una copia del elemento (fichero o carpeta con su árbol) en otra
// carpeta del disco, que no tiene por qué ser accesible: es «Copiar a…», el «Descargar»
// de la edición local (DR-08). Si el nombre ya está, la copia se llama «nombre (2)».
// Devuelve la ruta de la copia.
func (a *Archivos) CopiarFuera(ctx context.Context, id, carpeta string) (string, error) {
	return a.CopiarFueraEn(ctx, id, DelDisco, carpeta)
}

// CopiarFueraEn es CopiarFuera hacia una carpeta de otro sistema de ficheros: en Android,
// una carpeta del teléfono elegida con el selector del sistema (un [Arbol]), que no es una
// ruta del disco. Con [DelDisco] es CopiarFuera.
func (a *Archivos) CopiarFueraEn(ctx context.Context, id string, destino Sistema, carpeta string) (string, error) {
	r, err := a.ruta(id)
	if err != nil {
		return "", err
	}
	fi, err := a.fs().Stat(r)
	if err != nil {
		return "", noEsta(id)
	}
	// El destino es una carpeta que dio el selector, no de la raíz.
	if di, err := destino.Stat(carpeta); err != nil || !di.IsDir() {
		return "", noEsta(carpeta)
	}
	carpeta = filepath.Clean(carpeta)
	if fi.IsDir() && (carpeta == r || dentro(r, carpeta)) {
		return "", ErrNombre // una carpeta no se copia dentro de sí misma
	}
	dst := filepath.Join(carpeta, fi.Name())
	if _, err := destino.Lstat(dst); err == nil {
		dst = nombreLibreEn(destino, carpeta, fi.Name())
	}
	if fi.IsDir() {
		if err := copiarArbol(ctx, a.fs(), r, destino, dst); err != nil {
			_ = destino.RemoveAll(dst)
			return "", err
		}
		return dst, nil
	}
	if err := copiarAtomico(a.fs(), r, destino, dst); err != nil {
		return "", err
	}
	_ = destino.Chmod(dst, fi.Mode().Perm())
	return dst, nil
}

// copiarArbol copia de (en el sistema origen) en a2 (en el destino, que no existe), sin lo
// oculto ni los enlaces simbólicos. Primero recorre el origen y después copia: un destino
// que esté dentro del origen (en otro sistema de ficheros no se puede saber por la ruta)
// no se copia otra vez dentro de sí mismo.
func copiarArbol(ctx context.Context, origen Sistema, de string, destino Sistema, a2 string) error {
	type paso struct {
		r       string
		carpeta bool
	}
	var pasos []paso
	err := origen.WalkDir(de, func(r string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if r != de && oculto(e.Name()) {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case e.IsDir():
			pasos = append(pasos, paso{r, true})
		case e.Type().IsRegular():
			pasos = append(pasos, paso{r, false})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, p := range pasos {
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(de, p.r)
		if err != nil {
			return err
		}
		dst := filepath.Join(a2, rel)
		if p.carpeta {
			err = destino.Mkdir(dst, 0o755)
		} else {
			err = copiarAtomico(origen, p.r, destino, dst)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ----- Lotes -----

// BatchMove mueve ficheros y carpetas bajo destino (vacío: la raíz). Sigue con los
// demás si uno falla y cuenta lo que movió; el primer error se devuelve si no se movió
// nada, como el servicio, que se salta lo que no puede.
func (a *Archivos) BatchMove(ctx context.Context, ficheros, carpetas []string, destino string) (api.DriveLote, error) {
	if destino == "" {
		destino = idDe(a.raiz())
	}
	var lote api.DriveLote
	var primero error
	for _, id := range ficheros {
		if err := a.Move(ctx, api.DriveFile, id, destino); err != nil {
			primero = cmpErr(primero, err)
			continue
		}
		lote.Ficheros++
	}
	for _, id := range carpetas {
		if err := a.Move(ctx, api.DriveFolder, id, destino); err != nil {
			primero = cmpErr(primero, err)
			continue
		}
		lote.Carpetas++
	}
	if lote.Total() == 0 && primero != nil {
		return lote, primero
	}
	return lote, nil
}

// BatchTrash manda a la papelera ficheros y carpetas, con las mismas reglas que BatchMove.
func (a *Archivos) BatchTrash(ctx context.Context, ficheros, carpetas []string) (api.DriveLote, error) {
	var lote api.DriveLote
	var primero error
	for _, id := range ficheros {
		if err := a.Delete(ctx, api.DriveFile, id); err != nil {
			primero = cmpErr(primero, err)
			continue
		}
		lote.Ficheros++
	}
	for _, id := range carpetas {
		if err := a.Delete(ctx, api.DriveFolder, id); err != nil {
			primero = cmpErr(primero, err)
			continue
		}
		lote.Carpetas++
	}
	if lote.Total() == 0 && primero != nil {
		return lote, primero
	}
	return lote, nil
}

func cmpErr(primero, err error) error {
	if primero != nil {
		return primero
	}
	return err
}

// ----- Espacio -----

// Quota es el espacio del disco donde está la raíz: el total, lo ocupado (por todo, no
// solo por la raíz) y lo libre para quien usa la app.
func (a *Archivos) Quota(ctx context.Context) (api.DriveQuota, error) {
	if err := ctx.Err(); err != nil {
		return api.DriveQuota{}, err
	}
	total, libre, err := a.fs().Espacio(a.raiz())
	if err != nil {
		return api.DriveQuota{}, err
	}
	usado := max(total-libre, 0)
	return api.DriveQuota{LimitBytes: total, UsedBytes: usado, AvailableBytes: libre}, nil
}

// ----- Miniaturas -----

const (
	// LadoMiniatura es el lado mayor de una miniatura, en píxeles.
	LadoMiniatura = 320
	// topeMiniatura es lo más grande que se lee para hacer una.
	topeMiniatura = 64 << 20
)

// Thumbnail hace en el dispositivo la miniatura de una imagen (JPEG, PNG, GIF, WebP):
// lado mayor LadoMiniatura, PNG si puede llevar transparencia. Lo demás no tiene.
func (a *Archivos) Thumbnail(ctx context.Context, id string) (io.ReadCloser, error) {
	r, err := a.ruta(id)
	if err != nil {
		return nil, err
	}
	salida, ok := miniatura.TipoSalida(tipoDe(r))
	if !ok {
		return nil, ErrNoDisponible
	}
	fi, err := a.fs().Stat(r)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, noEsta(id)
	}
	if fi.Size() > topeMiniatura {
		return nil, ErrNoDisponible
	}
	cache := a.rutaMiniatura(r, fi)
	if cache != "" {
		if datos, err := os.ReadFile(cache); err == nil && len(datos) > 0 {
			return io.NopCloser(bytes.NewReader(datos)), nil
		}
	}
	datos, err := miniatura.RenderConContexto(ctx, func(context.Context) ([]byte, error) { return a.fs().ReadFile(r) }, LadoMiniatura, salida)
	if err != nil {
		return nil, err
	}
	if cache != "" {
		if os.MkdirAll(filepath.Dir(cache), 0o700) == nil {
			_ = biblioteca.EscribirAtomico(cache, datos)
		}
	}
	return io.NopCloser(bytes.NewReader(datos)), nil
}

// rutaMiniatura es dónde va en la caché la miniatura de r tal y como está ahora: la
// clave lleva la ruta, el tamaño y la fecha de modificación, así que un fichero que
// cambia (aquí o en otro programa) estrena miniatura sin tener que invalidar nada. "" sin
// caché.
func (a *Archivos) rutaMiniatura(r string, fi fs.FileInfo) string {
	if a.minis == "" {
		return ""
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d\x00%d", r, fi.Size(), fi.ModTime().UnixNano(), LadoMiniatura)))
	clave := hex.EncodeToString(h[:])
	return filepath.Join(a.minis, clave[:2], clave)
}

// ----- ZIP -----

// Zip empaqueta una selección en el dispositivo, en flujo: los ficheros sueltos en la
// raíz del ZIP y cada carpeta con su árbol (sin lo oculto). `contenido` es lo que suman
// sin comprimir.
func (a *Archivos) Zip(ctx context.Context, ficheros, carpetas []string, _ string) (io.ReadCloser, int64, error) {
	sel := &seleccionZip{usados: map[string]bool{}}
	for _, id := range ficheros {
		if err := a.zipFichero(sel, id); err != nil {
			return nil, 0, err
		}
	}
	for _, id := range carpetas {
		if err := a.zipCarpeta(sel, id); err != nil {
			return nil, 0, err
		}
	}
	entradas := sel.entradas
	if len(entradas) == 0 {
		return nil, 0, noEsta("zip")
	}
	pr, pw := io.Pipe()
	go func() {
		zw := zip.NewWriter(pw)
		var err error
		for _, e := range entradas {
			if err = ctx.Err(); err != nil {
				break
			}
			if err = anadirAlZip(a.fs(), zw, e.ruta, e.nombre); err != nil {
				break
			}
		}
		if cerr := zw.Close(); err == nil {
			err = cerr
		}
		_ = pw.CloseWithError(err)
	}()
	return pr, sel.contenido, nil
}

// entradaZip es un fichero del dispositivo y su nombre dentro del ZIP.
type entradaZip struct{ ruta, nombre string }

// seleccionZip es lo que va en el ZIP: sus entradas, lo que suman sin comprimir y los
// nombres ya usados en la raíz.
type seleccionZip struct {
	entradas  []entradaZip
	contenido int64
	usados    map[string]bool
}

// libre es n, o «n (2)», «n (3)»… si ya hay algo con ese nombre en la raíz.
func (s *seleccionZip) libre(n string) string {
	ext := filepath.Ext(n)
	base := strings.TrimSuffix(n, ext)
	for i := 2; s.usados[n]; i++ {
		n = fmt.Sprintf("%s (%d)%s", base, i, ext)
	}
	s.usados[n] = true
	return n
}

// zipFichero añade un fichero suelto a la raíz del ZIP.
func (a *Archivos) zipFichero(sel *seleccionZip, id string) error {
	r, err := a.ruta(id)
	if err != nil {
		return err
	}
	fi, err := a.fs().Stat(r)
	if err != nil || !fi.Mode().IsRegular() {
		return noEsta(id)
	}
	sel.contenido += fi.Size()
	sel.entradas = append(sel.entradas, entradaZip{r, sel.libre(fi.Name())})
	return nil
}

// zipCarpeta añade una carpeta con su árbol, sin lo oculto.
func (a *Archivos) zipCarpeta(sel *seleccionZip, id string) error {
	r, err := a.ruta(id)
	if err != nil {
		return err
	}
	fi, err := a.fs().Stat(r)
	if err != nil || !fi.IsDir() {
		return noEsta(id)
	}
	base := sel.libre(fi.Name())
	return a.fs().WalkDir(r, func(x string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if x != r && oculto(e.Name()) {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !e.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(r, x)
		if info, err := e.Info(); err == nil {
			sel.contenido += info.Size()
		}
		sel.entradas = append(sel.entradas, entradaZip{x, base + "/" + filepath.ToSlash(rel)})
		return nil
	})
}

func anadirAlZip(s Sistema, zw *zip.Writer, ruta, nombre string) error {
	f, err := s.Open(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	h.Name, h.Method = nombre, zip.Deflate
	w, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

// ----- Documentos nuevos -----

// ExtensionDe es la extensión con la que nace un documento nuevo de la ofimática desde
// Drive: el formato abierto de su familia, que Docus, Calcus y Diapos abren y guardan.
var ExtensionDe = map[api.DocKind]string{api.DocText: ".docx", api.DocSheet: ".xlsx", api.DocSlides: ".pptx"}

// CrearDocumento deja en la carpeta (vacía: la raíz) un documento vacío del tipo, con
// el primer nombre libre de «titulo.ext» (DR-06). Lo crea el mismo exportador que usan
// los editores, así que la app que lo abra lo reconoce como suyo.
func (a *Archivos) CrearDocumento(ctx context.Context, carpeta string, k api.DocKind, titulo string) (api.File, error) {
	ext, ok := ExtensionDe[k]
	if !ok {
		return api.File{}, errors.New("archivos: tipo de documento desconocido")
	}
	if carpeta == "" {
		carpeta = idDe(a.raiz())
	}
	d, err := a.ruta(carpeta)
	if err != nil {
		return api.File{}, err
	}
	nombre, err := nombreValido(titulo + ext)
	if err != nil {
		return api.File{}, err
	}
	datos, _, _, err := model.Export(model.Kind(k), titulo, model.NewState(), model.FormatOOXML)
	if err != nil {
		return api.File{}, err
	}
	r := filepath.Join(d, nombre)
	if _, err := a.fs().Lstat(r); err == nil {
		r = nombreLibreEn(a.fs(), d, nombre)
	}
	if err := a.escribir(ctx, r, bytes.NewReader(datos), int64(len(datos))); err != nil {
		return api.File{}, err
	}
	return a.ficheroConSuma(r)
}
