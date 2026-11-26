// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package copia es la copia de seguridad de Sytue Office (PLAN-EDICION-LOCAL §5.13): un
// ZIP con la carpeta de datos y los ajustes, SIN secretos, y su restauración.
//
//   - **Crear** deja `<Datos>/Copias/sytue-office-AAAA-MM-DD-HHMMSS.zip`. Dentro van
//     `datos/…` (todo `<Datos>` menos `Copias/`), `ajustes/…` (todo `<Ajustes>` menos el
//     llavero de reserva `secretos/`) y `manifiesto.json`. Las bases SQLite que la app
//     tiene ABIERTAS se copian con su copia en caliente ([Copia.BaseAbierta]); las
//     demás, con su diario (`-wal`) al lado, que SQLite repasa al abrirlas.
//   - **Restaurar** comprueba el ZIP (manifiesto, rutas dentro de su carpeta), hace antes
//     una copia de lo que hay —por si acaso— y sustituye datos y ajustes. Las claves y
//     contraseñas no estaban en la copia y siguen en el llavero.
//
// Todo es un fichero, en claro: la copia se puede abrir con cualquier programa de ZIP.
package copia

import (
	"archive/zip"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
)

// Formato es la versión del ZIP.
const Formato = 1

// Carpeta es la de las copias, dentro de la de datos.
const Carpeta = "Copias"

// Secretos es la carpeta del llavero de reserva dentro de los ajustes (`local/secretos`):
// NUNCA entra en una copia.
const Secretos = "secretos"

// Manifiesto describe una copia.
type Manifiesto struct {
	Formato  int       `json:"formato"`
	Producto string    `json:"producto"`
	Creada   time.Time `json:"creada"`
	Ficheros int       `json:"ficheros"`
	Bytes    int64     `json:"bytes"`
	// SinSecretos lo recuerda a quien abra el ZIP: las claves van aparte.
	SinSecretos bool `json:"sin_secretos"`
}

// Info es una copia de la carpeta de copias.
type Info struct {
	Ruta   string
	Nombre string
	Creada time.Time
	Bytes  int64
}

// ErrNoEsCopia es un ZIP que no es una copia de Sytue Office.
var ErrNoEsCopia = errors.New("copia: el fichero no es una copia de Sytue Office")

// Copia sabe copiar y restaurar una instalación.
type Copia struct {
	Datos, Ajustes string
	// Producto es quien la hace, para el manifiesto.
	Producto string

	mu       sync.Mutex
	abiertas map[string]func(ctx context.Context, destino string) error
	// ahora es el reloj; las pruebas lo fijan.
	ahora func() time.Time
}

// Nueva prepara las copias de datos y ajustes.
func Nueva(datos, ajustes, producto string) *Copia {
	return &Copia{Datos: datos, Ajustes: ajustes, Producto: producto, abiertas: map[string]func(context.Context, string) error{}, ahora: time.Now}
}

// BaseAbierta apunta una base SQLite que esta app tiene abierta y cómo copiarla en
// caliente (con `VACUUM INTO` o la API de copia de SQLite) a destino. La función que
// devuelve la quita, al cerrar la base.
func (c *Copia) BaseAbierta(ruta string, copiar func(ctx context.Context, destino string) error) (quitar func()) {
	ruta = filepath.Clean(ruta)
	c.mu.Lock()
	c.abiertas[ruta] = copiar
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.abiertas, ruta)
		c.mu.Unlock()
	}
}

// CarpetaCopias es `<Datos>/Copias`.
func (c *Copia) CarpetaCopias() string { return filepath.Join(c.Datos, Carpeta) }

// Lista son las copias que hay, de la más nueva a la más vieja.
func (c *Copia) Lista() ([]Info, error) {
	es, err := os.ReadDir(c.CarpetaCopias())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{Ruta: filepath.Join(c.CarpetaCopias(), e.Name()), Nombre: e.Name(), Creada: fi.ModTime(), Bytes: fi.Size()})
	}
	slices.SortFunc(out, func(a, b Info) int { return b.Creada.Compare(a.Creada) })
	return out, nil
}

// Crear hace una copia nueva y devuelve su ruta.
func (c *Copia) Crear(ctx context.Context) (string, error) {
	if err := os.MkdirAll(c.CarpetaCopias(), 0o700); err != nil {
		return "", err
	}
	nombre := "sytue-office-" + c.ahora().Format("2006-01-02-150405") + ".zip"
	destino := filepath.Join(c.CarpetaCopias(), nombre)
	for i := 2; existe(destino); i++ {
		destino = filepath.Join(c.CarpetaCopias(), strings.TrimSuffix(nombre, ".zip")+fmt.Sprintf("-%d.zip", i))
	}
	tmp, err := os.CreateTemp(c.CarpetaCopias(), ".copia-*.zip")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = c.escribir(ctx, tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), destino); err != nil {
		return "", err
	}
	return destino, nil
}

func existe(ruta string) bool {
	_, err := os.Stat(ruta)
	return err == nil
}

func (c *Copia) escribir(ctx context.Context, w io.Writer) (Manifiesto, error) {
	z := zip.NewWriter(w)
	m := Manifiesto{Formato: Formato, Producto: c.Producto, Creada: c.ahora().UTC(), SinSecretos: true}
	tmp, err := os.MkdirTemp("", "sytue-office-copia-*")
	if err != nil {
		return m, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	// Los almacenes de las apps se copian con su cerrojo compartido tomado: así ninguna
	// compactación borra un objeto a mitad de la copia (el diario siempre está entero).
	soltar, err := cerrarAlmacenes(c.Datos)
	if err != nil {
		return m, err
	}
	defer soltar()
	if err := c.añadirCarpeta(ctx, z, "datos", c.Datos, func(rel string) bool {
		return rel == Carpeta || strings.HasPrefix(rel, Carpeta+"/")
	}, tmp, &m); err != nil {
		return m, err
	}
	if err := c.añadirCarpeta(ctx, z, "ajustes", c.Ajustes, func(rel string) bool {
		return rel == Secretos || strings.HasPrefix(rel, Secretos+"/")
	}, tmp, &m); err != nil {
		return m, err
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	fw, err := z.Create("manifiesto.json")
	if err != nil {
		return m, err
	}
	if _, err := fw.Write(raw); err != nil {
		return m, err
	}
	return m, z.Close()
}

// añadirCarpeta mete raiz bajo prefijo. fuera dice qué rutas relativas (con «/») no van.
func (c *Copia) añadirCarpeta(ctx context.Context, z *zip.Writer, prefijo, raiz string, fuera func(string) bool, tmp string, m *Manifiesto) error {
	if raiz == "" {
		return nil
	}
	c.mu.Lock()
	abiertas := make(map[string]func(context.Context, string) error, len(c.abiertas))
	for k, v := range c.abiertas {
		abiertas[k] = v
	}
	c.mu.Unlock()
	return filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && ruta == raiz {
				return filepath.SkipAll
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(raiz, ruta)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if fuera(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		origen := ruta
		if copiar, ok := abiertas[filepath.Clean(ruta)]; ok {
			// Una base abierta: su copia en caliente, nunca el fichero a medio escribir.
			origen = filepath.Join(tmp, fmt.Sprintf("base-%d", m.Ficheros))
			if err := copiar(ctx, origen); err != nil {
				return fmt.Errorf("copia: %s: %w", rel, err)
			}
		} else if esDiarioDeAbierta(ruta, abiertas) {
			return nil // su diario va dentro de la copia en caliente
		}
		return añadirFichero(z, prefijo+"/"+rel, origen, m)
	})
}

// esDiarioDeAbierta: `x.sqlite-wal` o `-shm` de una base que se copia en caliente.
func esDiarioDeAbierta(ruta string, abiertas map[string]func(context.Context, string) error) bool {
	for _, suf := range []string{"-wal", "-shm", "-journal"} {
		if b, ok := strings.CutSuffix(ruta, suf); ok {
			if _, ok := abiertas[filepath.Clean(b)]; ok {
				return true
			}
		}
	}
	return false
}

// Lo que ya va comprimido se guarda tal cual: comprimirlo otra vez no ahorra y tarda.
var comprimidos = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".heic": true, ".webp": true, ".gif": true,
	".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".mp3": true, ".opus": true, ".m4a": true,
	".zip": true, ".docx": true, ".xlsx": true, ".pptx": true, ".odt": true, ".ods": true, ".odp": true, ".pdf": true,
}

func añadirFichero(z *zip.Writer, nombre, origen string, m *Manifiesto) error {
	f, err := os.Open(origen)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	h.Name = nombre
	h.Method = zip.Deflate
	if comprimidos[strings.ToLower(path.Ext(nombre))] {
		h.Method = zip.Store
	}
	w, err := z.CreateHeader(h)
	if err != nil {
		return err
	}
	n, err := io.Copy(w, f)
	m.Ficheros++
	m.Bytes += n
	return err
}

// Comprobar abre un ZIP y dice si es una copia que se puede restaurar.
func Comprobar(ruta string) (Manifiesto, error) {
	z, err := zip.OpenReader(ruta)
	if err != nil {
		return Manifiesto{}, fmt.Errorf("%w: %w", ErrNoEsCopia, err)
	}
	defer func() { _ = z.Close() }()
	return comprobar(&z.Reader)
}

func comprobar(z *zip.Reader) (Manifiesto, error) {
	var m Manifiesto
	hay := false
	for _, f := range z.File {
		if f.Name == "manifiesto.json" {
			r, err := f.Open()
			if err != nil {
				return m, err
			}
			err = json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&m)
			_ = r.Close()
			if err != nil {
				return m, fmt.Errorf("%w: manifiesto ilegible", ErrNoEsCopia)
			}
			hay = true
			continue
		}
		if _, _, err := destinoDe(f.Name); err != nil {
			return m, err
		}
	}
	if !hay || m.Formato < 1 {
		return m, fmt.Errorf("%w: sin manifiesto", ErrNoEsCopia)
	}
	if m.Formato > Formato {
		return m, fmt.Errorf("copia: la hizo una versión más nueva de Sytue Office (formato %d)", m.Formato)
	}
	return m, nil
}

// destinoDe parte «datos/x/y» en ("datos", "x/y") y rechaza lo que se saldría de su
// carpeta (rutas absolutas, «..», el llavero).
func destinoDe(nombre string) (string, string, error) {
	limpio := path.Clean(nombre)
	raiz, rel, ok := strings.Cut(limpio, "/")
	if !ok || (raiz != "datos" && raiz != "ajustes") || rel == "" || strings.HasPrefix(limpio, "/") ||
		limpio != nombre || strings.Contains(nombre, "\\") || slices.Contains(strings.Split(rel, "/"), "..") {
		return "", "", fmt.Errorf("%w: ruta no admitida %q", ErrNoEsCopia, nombre)
	}
	if raiz == "ajustes" && (rel == Secretos || strings.HasPrefix(rel, Secretos+"/")) {
		return "", "", fmt.Errorf("%w: una copia no lleva secretos (%q)", ErrNoEsCopia, nombre)
	}
	if raiz == "datos" && (rel == Carpeta || strings.HasPrefix(rel, Carpeta+"/")) {
		return "", "", fmt.Errorf("%w: ruta no admitida %q", ErrNoEsCopia, nombre)
	}
	return raiz, rel, nil
}

// Restaurar sustituye datos y ajustes por los de la copia de ruta. Antes guarda una
// copia de lo que hay, y devuelve su ruta. Las bases que la app tenga abiertas hay que
// cerrarlas antes, y la app se vuelve a abrir después.
func (c *Copia) Restaurar(ctx context.Context, ruta string) (previa string, err error) {
	z, err := zip.OpenReader(ruta)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoEsCopia, err)
	}
	defer func() { _ = z.Close() }()
	if _, err := comprobar(&z.Reader); err != nil {
		return "", err
	}
	previa, err = c.Crear(ctx)
	if err != nil {
		return "", fmt.Errorf("copia: no se pudo guardar antes lo que hay: %w", err)
	}
	// Se saca todo a una carpeta temporal junto a los datos; solo si sale entero se
	// sustituye lo de ahora.
	tmp, err := os.MkdirTemp(c.CarpetaCopias(), ".restaurando-*")
	if err != nil {
		return previa, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for _, f := range z.File {
		if f.Name == "manifiesto.json" || strings.HasSuffix(f.Name, "/") {
			continue
		}
		raiz, rel, err := destinoDe(f.Name)
		if err != nil {
			return previa, err
		}
		if err := extraer(f, filepath.Join(tmp, raiz, filepath.FromSlash(rel))); err != nil {
			return previa, err
		}
	}
	if err := sustituir(c.Datos, filepath.Join(tmp, "datos"), Carpeta); err != nil {
		return previa, fmt.Errorf("copia: a medias; lo de antes está en %s: %w", previa, err)
	}
	if err := sustituir(c.Ajustes, filepath.Join(tmp, "ajustes"), Secretos); err != nil {
		return previa, fmt.Errorf("copia: a medias; lo de antes está en %s: %w", previa, err)
	}
	return previa, nil
}

func extraer(f *zip.File, destino string) error {
	if err := os.MkdirAll(filepath.Dir(destino), 0o700); err != nil {
		return err
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	w, err := os.OpenFile(destino, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// G110: es una copia que eligió la persona y puede medir gigas; archive/zip ya corta
	// si sale más de lo que declara la cabecera.
	if _, err := io.Copy(w, r); err != nil { //nolint:gosec // G110: ver arriba
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.Chtimes(destino, f.Modified, cmp.Or(f.Modified, time.Now()))
}

// sustituir vacía raiz (menos `conservar`) y pone dentro lo de nuevo.
func sustituir(raiz, nuevo, conservar string) error {
	if err := os.MkdirAll(raiz, 0o700); err != nil {
		return err
	}
	es, err := os.ReadDir(raiz)
	if err != nil {
		return err
	}
	for _, e := range es {
		if e.Name() == conservar {
			continue
		}
		if err := os.RemoveAll(filepath.Join(raiz, e.Name())); err != nil {
			return err
		}
	}
	es, err = os.ReadDir(nuevo)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range es {
		if err := moverOCopiar(filepath.Join(nuevo, e.Name()), filepath.Join(raiz, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// moverOCopiar renombra; si los ajustes están en otro disco que los datos, copia.
func moverOCopiar(origen, destino string) error {
	if err := os.Rename(origen, destino); err == nil {
		return nil
	}
	return filepath.WalkDir(origen, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(origen, ruta)
		dst := filepath.Join(destino, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		in, err := os.Open(ruta) //nolint:gosec // G122: la carpeta temporal de la restauración, que acabamos de llenar con 0700
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}

// cerrarAlmacenes toma el cerrojo compartido de cada almacén bajo datos y devuelve cómo
// soltarlos todos.
func cerrarAlmacenes(datos string) (func(), error) {
	var sueltas []func()
	soltar := func() {
		for _, s := range sueltas {
			s()
		}
	}
	err := filepath.WalkDir(datos, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		if !d.IsDir() || !almacen.EsAlmacen(ruta) {
			return nil
		}
		s, err := almacen.CerrojoCompartido(ruta)
		if err != nil {
			return fmt.Errorf("copia: %s: %w", ruta, err)
		}
		sueltas = append(sueltas, s)
		return filepath.SkipDir
	})
	if err != nil {
		soltar()
		return nil, err
	}
	return soltar, nil
}
