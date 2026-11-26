// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package traslado mueve la carpeta de datos de Sytue Office a otra (Ajustes → Datos):
// se lleva TODO lo que había, lo comprueba y solo entonces la apunta como la nueva.
//
//   - **Antes**: la nueva no puede ser la de ahora ni estar dentro de ella, y tiene que
//     estar vacía (o no existir): no se mezcla nada con lo que ya hubiera ([Comprobar]).
//     Los almacenes de las apps se toman con su cerrojo EXCLUSIVO (`almacen`): si una app
//     tiene los suyos abiertos, no se empieza ([ErrEnUso]); y, mientras dura, ninguna los
//     abre. Si no cabe en el disco de la nueva, tampoco ([ErrSinSitio]).
//   - **Copia**: fichero a fichero, con su fecha y sus permisos, los enlaces como enlaces;
//     de cada uno se apunta el tamaño y la huella (SHA-256) de lo que se leyó.
//   - **Comprobación**: se vuelve a leer cada copia del disco nuevo y su tamaño y su
//     huella tienen que ser los de lo leído; y el original no puede haber cambiado
//     mientras tanto.
//   - **Ajustes**: lo que guarda rutas absolutas dentro de la carpeta (el índice y las
//     versiones de Drive, los vídeos copiados de Cast, los documentos anclados a un
//     fichero de Drive) pasa a la ruta nueva, en la copia.
//   - **Apuntar**: con todo eso hecho, quien llama guarda la carpeta nueva (el perfil).
//
// Si algo falla o se cancela a medias, se quita lo copiado y la nueva queda como estaba
// (vacía o sin crear): nada cambia. La carpeta vieja NO se borra nunca sola; quien llama
// la ofrece y, si la persona quiere, [BorrarOrigen] quita exactamente lo copiado.
package traslado

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/archivos"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/videoteca"
)

var (
	// ErrMisma: la carpeta nueva es la de ahora.
	ErrMisma = errors.New("traslado: la carpeta nueva es la de ahora")
	// ErrDentro: la carpeta nueva está dentro de la de ahora (se copiaría a sí misma).
	ErrDentro = errors.New("traslado: la carpeta nueva está dentro de la de ahora")
	// ErrNoVacia: la carpeta nueva tiene algo: no se mezcla.
	ErrNoVacia = errors.New("traslado: la carpeta nueva no está vacía")
	// ErrEnUso: una app de Sytue Office tiene sus datos abiertos.
	ErrEnUso = errors.New("traslado: una app de Sytue Office tiene sus datos abiertos")
	// ErrSinSitio: no cabe en el disco de la carpeta nueva.
	ErrSinSitio = errors.New("traslado: no cabe en el disco de la carpeta nueva")
	// ErrCopiaMala: una copia no es igual que su original, o el original cambió mientras.
	ErrCopiaMala = errors.New("traslado: la copia no es igual que el original")
)

// Marca es el fichero que se deja en la carpeta nueva mientras se copia: si el equipo
// se apaga a medias, la próxima vez se sabe que lo que hay es una copia cortada de un
// traslado (y se puede vaciar) y no datos de nadie.
const Marca = ".sytue-office-moviendo"

// Fase es en qué va el traslado.
type Fase int

const (
	// Midiendo: se cuenta lo que hay que copiar.
	Midiendo Fase = iota
	// Copiando: se copia.
	Copiando
	// Comprobando: se vuelve a leer cada copia.
	Comprobando
	// Ajustando: se ponen al día las rutas y se apunta la carpeta nueva.
	Ajustando
)

// Progreso es por dónde va: bytes hechos de los Total de la fase.
type Progreso struct {
	Fase         Fase
	Hecho, Total int64
}

// Opciones son el traslado que se pide.
type Opciones struct {
	Origen, Destino string
	// Apuntar se llama con todo copiado, comprobado y ajustado, y los almacenes aún
	// tomados: guarda la carpeta nueva. Si falla, se quita la copia.
	Apuntar func(destino string) error
	// Progreso, si está, recibe por dónde va (desde la goroutine del traslado, como
	// mucho cada décima de segundo y al cambiar de fase).
	Progreso func(Progreso)
	// Libre, si está, dice cuánto cabe en el disco de una carpeta.
	Libre func(ruta string) (int64, error)
}

// Informe es lo que se ha llevado.
type Informe struct {
	Origen, Destino string
	Ficheros        int
	Bytes           int64
	// Omitidos son las rutas (relativas) de lo que no es fichero, carpeta ni enlace (un
	// enchufe, una tubería): no se copian y se quedan en la vieja.
	Omitidos []string
	// copiados son las rutas relativas de los ficheros y enlaces copiados y carpetas las
	// de las carpetas, para que BorrarOrigen quite exactamente eso.
	copiados, carpetas []string
}

// Destino es la carpeta en que acaban los datos si la persona elige elegida: ella misma
// si está vacía (o no existe); si no, una carpeta `nombre` dentro (`Sytue Office`), como
// cuando se ponen en una carpeta que ya sincroniza el sistema.
func Destino(elegida, nombre string) string {
	if vacia(elegida) {
		return filepath.Clean(elegida)
	}
	return filepath.Join(elegida, nombre)
}

// vacia dice si dir no existe o no tiene nada (o solo la marca de un traslado cortado).
func vacia(dir string) bool {
	es, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	for _, e := range es {
		if e.Name() != Marca {
			return cortada(dir)
		}
	}
	return true
}

// cortada dice si dir es lo que dejó un traslado cortado a medias: lleva la marca.
func cortada(dir string) bool {
	fi, err := os.Lstat(filepath.Join(dir, Marca))
	return err == nil && fi.Mode().IsRegular()
}

// limpias son las dos rutas absolutas y limpias, con los enlaces de lo que exista
// resueltos (dos nombres de la misma carpeta son la misma).
func limpias(origen, destino string) (string, string, error) {
	o, err := resuelta(origen)
	if err != nil {
		return "", "", err
	}
	d, err := resuelta(destino)
	return o, d, err
}

// resuelta es la ruta absoluta con los enlaces resueltos hasta donde existe.
func resuelta(ruta string) (string, error) {
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return "", err
	}
	resto := ""
	for {
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			return filepath.Join(r, resto), nil
		}
		arriba := filepath.Dir(abs)
		if arriba == abs {
			return filepath.Join(abs, resto), nil
		}
		resto = filepath.Join(filepath.Base(abs), resto)
		abs = arriba
	}
}

// dentro dice si r está dentro de base (o es ella).
func dentro(base, r string) bool {
	rel, err := filepath.Rel(base, r)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Comprobar dice si se puede mover origen a destino sin tocar nada: que no es la misma,
// que no está dentro y que la nueva está vacía.
func Comprobar(origen, destino string) error {
	o, d, err := limpias(origen, destino)
	if err != nil {
		return err
	}
	switch {
	case o == d || strings.EqualFold(o, d) && mismaCarpeta(o, d):
		return ErrMisma
	case dentro(o, d):
		return ErrDentro
	case !vacia(d):
		return ErrNoVacia
	}
	return nil
}

// mismaCarpeta dice si dos rutas son la misma carpeta (en un disco que no distingue
// mayúsculas, `Datos` y `datos` lo son).
func mismaCarpeta(a, b string) bool {
	fa, e1 := os.Stat(a)
	fb, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(fa, fb)
}

// Medir cuenta los ficheros y los bytes de dir.
func Medir(ctx context.Context, dir string) (ficheros int, bytes int64, err error) {
	err = filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type().IsRegular() {
			fi, err := d.Info()
			if err != nil {
				return err
			}
			ficheros++
			bytes += fi.Size()
		}
		return nil
	})
	return ficheros, bytes, err
}

// copiado es lo que se apuntó de un fichero al copiarlo.
type copiado struct {
	rel    string
	enlace string // destino del enlace, si es un enlace
	tamano int64
	fecha  time.Time
	huella [sha256.Size]byte
}

// trasladador es un traslado en marcha.
type trasladador struct {
	ctx      context.Context
	o        Opciones
	inf      Informe
	hechos   []copiado
	almacens map[string]bool
	hecho    int64
	total    int64
	fase     Fase
	ultimo   time.Time
}

func (t *trasladador) avisar(forzar bool) {
	if t.o.Progreso == nil {
		return
	}
	if !forzar && time.Since(t.ultimo) < 100*time.Millisecond {
		return
	}
	t.ultimo = time.Now()
	t.o.Progreso(Progreso{Fase: t.fase, Hecho: t.hecho, Total: t.total})
}

func (t *trasladador) empezar(f Fase) {
	t.fase, t.hecho = f, 0
	t.avisar(true)
}

// Mover lleva todo lo de o.Origen a o.Destino, lo comprueba, ajusta las rutas y llama a
// o.Apuntar. Si algo falla o ctx se cancela, la nueva queda como estaba. La vieja no se
// toca: para eso está BorrarOrigen.
func Mover(ctx context.Context, o Opciones) (_ Informe, errFinal error) {
	origen, destino, err := limpias(o.Origen, o.Destino)
	if err != nil {
		return Informe{}, err
	}
	if err := Comprobar(origen, destino); err != nil {
		return Informe{}, err
	}
	t := &trasladador{ctx: ctx, o: o, inf: Informe{Origen: origen, Destino: destino}, almacens: map[string]bool{}}

	// Lo que hay, y dónde están los almacenes.
	t.empezar(Midiendo)
	if err := t.medir(origen); err != nil {
		return t.inf, err
	}
	// Los almacenes, cerrados para todos mientras dura.
	soltar, err := tomarAlmacenes(origen, t.almacens)
	if err != nil {
		return t.inf, err
	}
	defer soltar()
	if o.Libre != nil {
		if libre, err := o.Libre(existente(destino)); err == nil && libre < t.total {
			return t.inf, fmt.Errorf("%w: hacen falta %d bytes y hay %d", ErrSinSitio, t.total, libre)
		}
	}

	deshacer, err := prepararDestino(destino)
	if err != nil {
		return t.inf, err
	}
	defer func() {
		if errFinal != nil {
			if err := deshacer(); err != nil {
				errFinal = fmt.Errorf("%w (y no se pudo quitar lo copiado de %s: %w)", errFinal, destino, err)
			}
		}
	}()

	if err := t.llevar(origen, destino); err != nil {
		return t.inf, err
	}
	t.hecho = t.total
	t.avisar(true)
	return t.inf, nil
}

// medir cuenta los bytes de origen y apunta dónde están los almacenes.
func (t *trasladador) medir(origen string) error {
	return filepath.WalkDir(origen, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := t.ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() && almacen.EsAlmacen(ruta) {
			t.almacens[ruta] = true
		}
		if d.Type().IsRegular() {
			fi, err := d.Info()
			if err != nil {
				return err
			}
			t.total += fi.Size()
		}
		return nil
	})
}

// llevar copia, comprueba, ajusta y apunta, con los almacenes ya tomados y la carpeta
// nueva preparada.
func (t *trasladador) llevar(origen, destino string) error {
	t.empezar(Copiando)
	if err := t.copiar(origen, destino); err != nil {
		return err
	}
	t.empezar(Comprobando)
	if err := t.comprobar(origen, destino); err != nil {
		return err
	}
	t.empezar(Ajustando)
	for _, a := range ajustes {
		if err := a(destino, origen, destino); err != nil {
			return err
		}
	}
	if err := t.ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(destino, Marca)); err != nil {
		return err
	}
	if t.o.Apuntar != nil {
		return t.o.Apuntar(destino)
	}
	return nil
}

// ajustes ponen al día, en la copia, lo que guarda rutas absolutas de dentro de la
// carpeta de datos: (raíz nueva, de, a).
var ajustes = []func(nueva, de, a string) error{
	func(nueva, de, a string) error { return archivos.Trasladar(filepath.Join(nueva, "Drive"), de, a) },
	func(nueva, de, a string) error {
		return videoteca.Trasladar(filepath.Join(nueva, "Datos", "cast"), de, a)
	},
	func(nueva, de, a string) error {
		return biblioteca.Trasladar(filepath.Join(nueva, "Documentos"), de, a)
	},
}

// tomarAlmacenes toma el cerrojo exclusivo de cada almacén: si alguno está abierto,
// suelta los que tenía y falla con ErrEnUso.
func tomarAlmacenes(origen string, dirs map[string]bool) (func(), error) {
	var sueltas []func()
	soltar := func() {
		for _, s := range sueltas {
			s()
		}
	}
	for d := range dirs {
		s, err := almacen.CerrojoExclusivo(d)
		if err != nil {
			soltar()
			rel, _ := filepath.Rel(origen, d)
			if errors.Is(err, almacen.ErrEnUso) {
				return nil, fmt.Errorf("%w (%s)", ErrEnUso, rel)
			}
			return nil, err
		}
		sueltas = append(sueltas, s)
	}
	return soltar, nil
}

// LibreEn es lo que cabe, para quien usa la app, en el disco de ruta (o, si aún no
// existe, de la carpeta más cercana por encima que sí).
func LibreEn(ruta string) (int64, error) {
	_, libre, err := archivos.DelDisco.Espacio(existente(ruta))
	return libre, err
}

// existente es ruta o la carpeta más cercana por encima que existe.
func existente(ruta string) string {
	for {
		if _, err := os.Stat(ruta); err == nil {
			return ruta
		}
		arriba := filepath.Dir(ruta)
		if arriba == ruta {
			return ruta
		}
		ruta = arriba
	}
}

// prepararDestino deja la carpeta nueva lista y con la marca, y devuelve cómo dejarla
// como estaba: si no existía, se quita (con las carpetas de encima que se crearon); si
// estaba vacía, se vacía. Lo que dejó un traslado cortado se tira antes.
func prepararDestino(destino string) (deshacer func() error, err error) {
	if cortada(destino) {
		es, err := os.ReadDir(destino)
		if err != nil {
			return nil, err
		}
		for _, e := range es {
			if err := os.RemoveAll(filepath.Join(destino, e.Name())); err != nil {
				return nil, err
			}
		}
	}
	creada := ""
	if _, err := os.Stat(destino); errors.Is(err, fs.ErrNotExist) {
		// La más alta de las que se van a crear.
		creada = destino
		for arriba := filepath.Dir(creada); arriba != creada; arriba = filepath.Dir(creada) {
			if _, err := os.Stat(arriba); err == nil {
				break
			}
			creada = arriba
		}
	}
	if err := os.MkdirAll(destino, 0o700); err != nil {
		return nil, err
	}
	deshacer = func() error {
		if creada != "" {
			return os.RemoveAll(creada)
		}
		es, err := os.ReadDir(destino)
		if err != nil {
			return err
		}
		var errs []error
		for _, e := range es {
			errs = append(errs, os.RemoveAll(filepath.Join(destino, e.Name())))
		}
		return errors.Join(errs...)
	}
	if err := os.WriteFile(filepath.Join(destino, Marca), []byte("Sytue Office: copia a medias de la carpeta de datos.\n"), 0o600); err != nil {
		return nil, errors.Join(err, deshacer())
	}
	return deshacer, nil
}

// copiar copia el árbol de origen en destino.
func (t *trasladador) copiar(origen, destino string) error {
	return filepath.WalkDir(origen, func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := t.ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(origen, ruta)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if rel == Marca {
			return nil
		}
		dst := filepath.Join(destino, rel)
		fi, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			t.inf.carpetas = append(t.inf.carpetas, rel)
			return os.Mkdir(dst, fi.Mode().Perm()|0o700)
		case d.Type()&fs.ModeSymlink != 0:
			destinoEnlace, err := os.Readlink(ruta)
			if err != nil {
				return err
			}
			// G122: dst está en la carpeta nueva, que estaba vacía y llena este traslado; el
			// enlace se copia tal cual, sin seguirlo.
			if err := os.Symlink(destinoEnlace, dst); err != nil { //nolint:gosec // G122: ver arriba
				return err
			}
			t.hechos = append(t.hechos, copiado{rel: rel, enlace: destinoEnlace})
			t.inf.copiados = append(t.inf.copiados, rel)
			return nil
		case !d.Type().IsRegular():
			t.inf.Omitidos = append(t.inf.Omitidos, rel)
			return nil
		}
		c := copiado{rel: rel, tamano: fi.Size(), fecha: fi.ModTime()}
		if filepath.Base(ruta) == almacen.FicheroCerrojo && t.almacens[filepath.Dir(ruta)] {
			// El cerrojo del almacén está tomado (en Windows no se deja leer): se crea
			// vacío, que es lo que es.
			c.tamano = 0
			c.huella = sha256.Sum256(nil)
			if err := os.WriteFile(dst, nil, fi.Mode().Perm()); err != nil {
				return err
			}
		} else if c.huella, err = t.copiarFichero(ruta, dst, fi); err != nil {
			return err
		}
		t.hechos = append(t.hechos, c)
		t.inf.copiados = append(t.inf.copiados, rel)
		t.inf.Ficheros++
		t.inf.Bytes += c.tamano
		return nil
	})
}

// lectorVigilado cuenta lo leído para el progreso y para en seco si se cancela.
type lectorVigilado struct {
	r io.Reader
	t *trasladador
}

func (l lectorVigilado) Read(p []byte) (int, error) {
	if err := l.t.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := l.r.Read(p)
	l.t.hecho += int64(n)
	l.t.avisar(false)
	return n, err
}

// copiarFichero copia un fichero (sin pisar nada: O_EXCL) con sus permisos y su fecha, y
// devuelve la huella de lo leído.
func (t *trasladador) copiarFichero(de, a string, fi fs.FileInfo) (huella [sha256.Size]byte, err error) {
	in, err := os.Open(de) //nolint:gosec // G304: un fichero de la carpeta de datos de la persona
	if err != nil {
		return huella, err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(a, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fi.Mode().Perm()|0o600) //nolint:gosec // G304: dentro de la carpeta nueva que eligió la persona
	if err != nil {
		return huella, err
	}
	h := sha256.New()
	n, err := io.CopyBuffer(out, io.TeeReader(lectorVigilado{r: in, t: t}, h), make([]byte, 1<<20))
	if err == nil && n != fi.Size() {
		err = fmt.Errorf("%w: %s cambió de tamaño mientras se copiaba", ErrCopiaMala, de)
	}
	if err == nil {
		err = out.Sync()
	}
	if e := out.Close(); err == nil {
		err = e
	}
	if err != nil {
		return huella, err
	}
	copy(huella[:], h.Sum(nil))
	return huella, os.Chtimes(a, fi.ModTime(), fi.ModTime())
}

// comprobar vuelve a leer cada copia del disco nuevo: tamaño y huella como los leídos, y
// el original sin cambiar desde entonces.
func (t *trasladador) comprobar(origen, destino string) error {
	for _, c := range t.hechos {
		if err := t.ctx.Err(); err != nil {
			return err
		}
		dst := filepath.Join(destino, c.rel)
		if c.enlace != "" {
			if e, err := os.Readlink(dst); err != nil || e != c.enlace {
				return fmt.Errorf("%w: el enlace %s", ErrCopiaMala, c.rel)
			}
			continue
		}
		fi, err := os.Lstat(dst)
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() || fi.Size() != c.tamano {
			return fmt.Errorf("%w: %s", ErrCopiaMala, c.rel)
		}
		huella, err := t.huella(dst)
		if err != nil {
			return err
		}
		if huella != c.huella {
			return fmt.Errorf("%w: %s", ErrCopiaMala, c.rel)
		}
		if c.tamano == 0 && filepath.Base(c.rel) == almacen.FicheroCerrojo {
			continue
		}
		if fo, err := os.Lstat(filepath.Join(origen, c.rel)); err != nil || fo.Size() != c.tamano || !fo.ModTime().Equal(c.fecha) {
			return fmt.Errorf("%w: %s cambió mientras se copiaba; ¿hay alguna app de Sytue Office abierta?", ErrCopiaMala, c.rel)
		}
	}
	return nil
}

func (t *trasladador) huella(ruta string) (huella [sha256.Size]byte, err error) {
	f, err := os.Open(ruta) //nolint:gosec // G304: la copia recién hecha en la carpeta nueva
	if err != nil {
		return huella, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.CopyBuffer(h, lectorVigilado{r: f, t: t}, make([]byte, 1<<20)); err != nil {
		return huella, err
	}
	copy(huella[:], h.Sum(nil))
	return huella, nil
}

// BorrarOrigen quita de la carpeta vieja exactamente lo que se copió (los ficheros y
// enlaces y, después, las carpetas que se quedan vacías), y la carpeta vieja si se queda
// vacía. Lo que no se copió —lo omitido, o lo que alguien haya puesto después— se queda.
// Devuelve cuántas cosas no se han podido quitar.
func BorrarOrigen(inf Informe) (quedan int, err error) {
	if inf.Origen == "" || inf.Destino == "" || dentro(inf.Origen, inf.Destino) {
		return 0, errors.New("traslado: no hay un traslado hecho que borrar")
	}
	var errs []error
	for _, rel := range inf.copiados {
		if err := os.Remove(filepath.Join(inf.Origen, rel)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			quedan++
			errs = append(errs, err)
		}
	}
	// Las carpetas, de las más hondas a las de arriba.
	for i := len(inf.carpetas) - 1; i >= 0; i-- {
		if err := os.Remove(filepath.Join(inf.Origen, inf.carpetas[i])); err != nil && !errors.Is(err, fs.ErrNotExist) {
			quedan++
		}
	}
	if err := os.Remove(inf.Origen); err != nil && !errors.Is(err, fs.ErrNotExist) {
		quedan++
	}
	return quedan, errors.Join(errs...)
}
