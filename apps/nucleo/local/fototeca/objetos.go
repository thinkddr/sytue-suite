// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fototeca

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// # Los ficheros
//
// Los originales copiados, las derivadas y los pósters van a `objetos/<ab>/<sha>`, en
// claro y por la huella SHA-256 de sus bytes: el mismo sitio y la misma forma que en el
// almacén de registros, así que migrar no copia ni un byte. Dos fotos con el mismo
// original (una repetida) comparten el fichero.
//
// Poner y quitar van con el cerrojo de escritura de SQLite, para que otro proceso no
// borre un fichero que esta escritura está a punto de citar ni cite uno que otro está
// borrando:
//
//   - un fichero nuevo se escribe a un temporal (`objetos/.subida-*`), se sincroniza y,
//     DENTRO de la transacción que lo cita, se renombra a su sitio (colocar);
//   - un fichero que deja de citarse (al borrar para siempre) se apunta en `por_borrar`
//     en la misma transacción, y después se borra, de uno en uno y con el cerrojo, si
//     sigue sin citarlo nadie (borrarPendientes). Un corte entre medias deja la
//     anotación, que se repasa la próxima vez;
//   - lo que un corte deja sin citar —un temporal, o un fichero ya en su sitio cuya
//     transacción no llegó a confirmarse— lo barre vaciar la papelera (barrer).

const prefijoSubida = ".subida-"

// ruta es dónde va el fichero de esa huella.
func (f *Fototeca) ruta(sha string) string {
	return filepath.Join(f.dir, "objetos", sha[:2], sha)
}

// esHuella dice si s es una huella SHA-256 en hexadecimal (minúsculas).
func esHuella(s string) bool {
	if len(s) != 2*sha256.Size {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}

// RutaEnClaro es dónde está en el disco el fichero con esa huella, si está: para dárselo
// por su ruta a quien solo sabe leer ficheros (el reproductor de vídeo del sistema) sin
// copiar un vídeo de varios GB. SOLO para leer: escribir ahí estropea la fototeca, y
// borrar la foto para siempre lo quita.
func (f *Fototeca) RutaEnClaro(sha string) (string, bool) {
	sha = strings.ToLower(sha)
	if !esHuella(sha) {
		return "", false
	}
	ruta := f.ruta(sha)
	if info, err := os.Stat(ruta); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return ruta, true
}

// subida es un fichero escrito a su temporal, que aún no está en su sitio.
type subida struct {
	tmp   string
	sha   string
	bytes int64
}

// tirar quita el temporal, si sigue ahí (no llegó a colocarse).
func (s *subida) tirar() {
	if s != nil && s.tmp != "" {
		_ = os.Remove(s.tmp)
		s.tmp = ""
	}
}

// subir escribe lo que lee de r (exactamente tamano bytes si tamano >= 0) a un temporal
// de objetos/, sincronizado, con su huella.
func (f *Fototeca) subir(ctx context.Context, r io.Reader, tamano int64) (*subida, error) {
	tmp, err := os.CreateTemp(filepath.Join(f.dir, "objetos"), prefijoSubida+"*")
	if err != nil {
		return nil, err
	}
	s := &subida{tmp: tmp.Name()}
	h := sha256.New()
	var fuente io.Reader = lectorConContexto{ctx, r}
	if tamano >= 0 {
		fuente = io.LimitReader(fuente, tamano+1)
	}
	n, err := io.Copy(io.MultiWriter(tmp, h), fuente)
	if err == nil && tamano >= 0 && n != tamano {
		err = fmt.Errorf("%w: se esperaban %d bytes y llegaron %d", io.ErrUnexpectedEOF, tamano, n)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		s.tirar()
		return nil, err
	}
	s.sha, s.bytes = hex.EncodeToString(h.Sum(nil)), n
	return s, nil
}

// subirBytes es subir con unos bytes en memoria (una derivada).
func (f *Fototeca) subirBytes(ctx context.Context, datos []byte) (*subida, error) {
	return f.subir(ctx, bytes.NewReader(datos), int64(len(datos)))
}

// colocar pone en su sitio el temporal de s, dentro de la transacción tx que lo cita: si
// ya había un fichero con esa huella, se queda el que había (son los mismos bytes) y se
// tira el temporal. Lo quita de lo que queda por borrar.
func (f *Fototeca) colocar(ctx context.Context, tx *sql.Tx, s *subida) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM por_borrar WHERE sha = ?`, s.sha); err != nil {
		return err
	}
	destino := f.ruta(s.sha)
	if info, err := os.Stat(destino); err == nil && info.Mode().IsRegular() && info.Size() == s.bytes {
		s.tirar()
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(destino), 0o700); err != nil {
		return err
	}
	if err := os.Rename(s.tmp, destino); err != nil {
		return err
	}
	s.tmp = ""
	return nil
}

// apuntarParaBorrar anota en tx que esos ficheros pueden dejar de citarse.
func apuntarParaBorrar(ctx context.Context, tx *sql.Tx, shas ...string) error {
	for _, sha := range shas {
		if !esHuella(sha) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO por_borrar (sha) VALUES (?)`, sha); err != nil {
			return err
		}
	}
	return nil
}

// citado dice si alguna fila cita el fichero sha: el original copiado de una foto no
// purgada o una derivada.
func citado(ctx context.Context, tx *sql.Tx, sha string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM fotos WHERE huella = ? AND retenido = 1 AND purgada IS NULL)
		OR EXISTS (SELECT 1 FROM derivadas WHERE sha = ?)`, sha, sha).Scan(&n)
	return n != 0, err
}

// borrarPendientes borra los ficheros apuntados en por_borrar que ya no cita nadie, cada
// uno con el cerrojo de escritura. Lo que no puede, lo deja para la próxima vez.
func (f *Fototeca) borrarPendientes(ctx context.Context) {
	shas, err := columna(ctx, f.db, `SELECT sha FROM por_borrar`)
	if err != nil {
		return
	}
	for _, sha := range shas {
		err := f.tx(ctx, func(tx *sql.Tx) error {
			en, err := citado(ctx, tx, sha)
			if err != nil {
				return err
			}
			if !en {
				if err := os.Remove(f.ruta(sha)); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			_, err = tx.ExecContext(ctx, `DELETE FROM por_borrar WHERE sha = ?`, sha)
			return err
		})
		if err != nil {
			slog.Debug("fototeca: no se pudo borrar un fichero; otra vez será", "sha", sha, "error", err)
			return
		}
	}
}

// edadParaBarrer es lo que tiene que tener un fichero sin citar para que barrer lo quite:
// uno más nuevo puede ser de una importación en curso en otro proceso.
const edadParaBarrer = 10 * time.Minute

// barrer quita de objetos/ los ficheros que no cita nadie y los temporales viejos: lo que
// dejó un corte (ver arriba). Lo que es de antes de la migración (lo que solo cita el
// diario del almacén de registros, que se conserva) no lo toca. Va con el cerrojo de
// escritura. Devuelve cuántos y cuánto.
func (f *Fototeca) barrer(ctx context.Context) (int, int64, error) {
	desde := time.Time{}
	if v, err := meta(ctx, f.db, claveMigradaEn); err == nil && v != "" {
		if t, err := deMarca(v); err == nil {
			desde = t
		}
	}
	var n int
	var liberados int64
	err := f.tx(ctx, func(tx *sql.Tx) error {
		vivos, err := vivosEn(ctx, tx)
		if err != nil {
			return err
		}
		n, liberados, err = f.barrerCarpeta(ctx, vivos, desde, true)
		return err
	})
	return n, liberados, err
}

// vivosEn son los ficheros de objetos/ que cita q: los originales copiados de las fotos
// no purgadas y las derivadas.
func vivosEn(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
},
) (map[string]bool, error) {
	vivos := map[string]bool{}
	for _, consulta := range []string{`SELECT huella FROM fotos WHERE retenido = 1 AND purgada IS NULL`, `SELECT sha FROM derivadas`} {
		shas, err := columna(ctx, q, consulta)
		if err != nil {
			return nil, err
		}
		for _, sha := range shas {
			vivos[sha] = true
		}
	}
	return vivos, nil
}

// barrerCarpeta quita de objetos/ lo que sobra (sobra) de entre lo que es de después de
// desde y no es de los últimos edadParaBarrer; sin quitar, solo lo cuenta.
func (f *Fototeca) barrerCarpeta(ctx context.Context, vivos map[string]bool, desde time.Time, quitar bool) (int, int64, error) {
	var n int
	var liberados int64
	limite := f.ahora().Add(-edadParaBarrer)
	err := filepath.WalkDir(filepath.Join(f.dir, "objetos"), func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() || !sobra(d.Name(), vivos) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().After(limite) || !info.ModTime().After(desde) {
			return nil //nolint:nilerr // un fichero que no se deja mirar, nuevo o de antes de migrar se deja estar
		}
		// G122: objetos/ es de la fototeca; si entretanto algo pone ahí un enlace, Remove
		// quita el enlace, nunca lo que apunta.
		if !quitar || os.Remove(ruta) == nil { //nolint:gosec // G122: ver arriba
			n++
			liberados += info.Size()
		}
		return nil
	})
	return n, liberados, err
}

// sobra dice si un fichero de objetos/ con ese nombre no lo cita nadie: una huella que no
// está entre las vivas o un temporal.
func sobra(nombre string, vivos map[string]bool) bool {
	if esHuella(nombre) {
		return !vivos[nombre]
	}
	return strings.HasPrefix(nombre, prefijoSubida) || strings.HasPrefix(nombre, ".compactando-")
}

// abrirObjeto abre el fichero de esa huella para leerlo.
func (f *Fototeca) abrirObjeto(sha string) (*os.File, error) {
	if !esHuella(sha) {
		return nil, ErrSource
	}
	o, err := os.Open(f.ruta(sha))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: falta el fichero %s", ErrSource, sha)
	}
	return o, err
}

// copiarObjeto copia a out el fichero de esa huella comprobando que sigue siendo él: si
// sus bytes ya no son los de su huella, falla (lo copiado ya está en out).
func (f *Fototeca) copiarObjeto(ctx context.Context, sha string, out io.Writer) error {
	o, err := f.abrirObjeto(sha)
	if err != nil {
		return err
	}
	defer func() { _ = o.Close() }()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), lectorConContexto{ctx, o}); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != sha {
		return fmt.Errorf("%w: el fichero %s no coincide con su huella", ErrSource, sha)
	}
	return nil
}

// leerObjeto lee entero el fichero de esa huella, de hasta tope bytes.
func (f *Fototeca) leerObjeto(ctx context.Context, sha string, tope int64) ([]byte, error) {
	var b bytes.Buffer
	if err := f.copiarObjeto(ctx, sha, &limitado{w: &b, queda: tope}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// limitado escribe en w hasta queda bytes; más, es un error.
type limitado struct {
	w     io.Writer
	queda int64
}

var errDemasiado = errors.New("fototeca: el fichero pasa del tope")

func (l *limitado) Write(p []byte) (int, error) {
	if int64(len(p)) > l.queda {
		return 0, errDemasiado
	}
	l.queda -= int64(len(p))
	return l.w.Write(p)
}

// lectorConContexto corta una lectura larga (un vídeo de varios GB) si se cancela.
type lectorConContexto struct {
	ctx context.Context
	r   io.Reader
}

func (c lectorConContexto) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
