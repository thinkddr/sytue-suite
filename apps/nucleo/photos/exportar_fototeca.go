// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ResumenExportacion es lo que hizo ExportarFototeca.
type ResumenExportacion struct {
	// Copiadas son las fotos que se escribieron; YaEstaban, las que ya estaban en su
	// carpeta con los mismos bytes (exportar dos veces no duplica nada).
	Copiadas, YaEstaban int
	// Fallidas son las que no se pudieron leer o escribir, con su motivo.
	Fallidas []error
}

// ExportarFototeca saca los ORIGINALES de la galería (fuera de la papelera) a
// dir/AAAA/MM/, con su nombre, tal y como entraron: sin receta ni recompresión
// (PLAN-EDICION-LOCAL.md §6.8, FO-05). Compensa que el almacén los guarde por su huella.
//
// La carpeta del mes es la de la fecha de la galería (la de captura o, sin EXIF, la de
// importación), en UTC como la línea de tiempo. Si ya hay un fichero con ese nombre y
// otros bytes, se escribe «nombre (2).ext»; nunca se pisa nada. Cada fichero se escribe
// a un temporal y se renombra. avance, si no es nil, se llama tras cada foto.
func (r *Repository) ExportarFototeca(ctx context.Context, dir string, avance func(hechas, total int)) (ResumenExportacion, error) {
	if dir == "" {
		return ResumenExportacion{}, os.ErrInvalid
	}
	fotos, err := r.vivas(ctx, false)
	if err != nil {
		return ResumenExportacion{}, err
	}
	conHuella := make([]FotoConHuella, len(fotos))
	for i, f := range fotos {
		conHuella[i] = FotoConHuella{Vista: f.vista, Subida: f.asset.UploadedAt, Huella: f.huella}
	}
	return ExportarOriginales(ctx, dir, conHuella, r.Original, avance)
}

// ExportarOriginales es ExportarFototeca sobre las fotos dadas (de la galería, fuera de
// la papelera, en el orden de la línea de tiempo), leyendo el original de una foto de la
// fototeca con original. La comparten las dos fototecas de Sytue Office.
func ExportarOriginales(ctx context.Context, dir string, fotos []FotoConHuella, original func(context.Context, string, io.Writer) error, avance func(hechas, total int)) (ResumenExportacion, error) {
	var resumen ResumenExportacion
	if dir == "" {
		return resumen, os.ErrInvalid
	}
	for i, f := range fotos {
		if err := ctx.Err(); err != nil {
			return resumen, err
		}
		var leido bytes.Buffer
		cuando := f.Vista.TakenAt.UTC()
		carpeta := filepath.Join(dir, fmt.Sprintf("%04d", cuando.Year()), fmt.Sprintf("%02d", int(cuando.Month())))
		var err error
		// Un vídeo copiado a la fototeca (de varios GB) se copia sin pasar por memoria.
		videoCopiado := f.Vista.Externo == "" && f.Vista.EsVideo() && f.Huella != ""
		if f.Vista.Externo == "" && !videoCopiado {
			err = original(ctx, f.Vista.ID, &leido)
		}
		if err == nil {
			var nueva bool
			nombre := nombreDeFichero(f.Vista.Name, f.Vista.ID)
			switch {
			case f.Vista.Externo != "":
				// Un original externo (un vídeo de varios GB) se copia sin pasar por memoria.
				nueva, err = copiarSinPisar(ctx, carpeta, nombre, f.Vista.Externo, f.Vista.SizeBytes, f.Huella)
			case videoCopiado:
				id := f.Vista.ID
				nueva, err = conHuellaSinPisar(carpeta, nombre, f.Vista.SizeBytes, f.Huella, func(w io.Writer) error { return original(ctx, id, w) })
			default:
				nueva, err = escribirSinPisar(carpeta, nombre, leido.Bytes())
			}
			if err == nil && nueva {
				resumen.Copiadas++
			} else if err == nil {
				resumen.YaEstaban++
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return resumen, ctx.Err()
			}
			resumen.Fallidas = append(resumen.Fallidas, fmt.Errorf("%s: %w", f.Vista.Name, err))
		}
		clear(leido.Bytes())
		if avance != nil {
			avance(i+1, len(fotos))
		}
	}
	return resumen, nil
}

// nombreDeFichero deja solo el último tramo del nombre y quita lo que ningún sistema de
// ficheros admite; sin nombre usable, el id.
func nombreDeFichero(nombre, id string) string {
	nombre = filepath.Base(strings.ReplaceAll(nombre, `\`, "/"))
	nombre = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, nombre)
	nombre = strings.Trim(nombre, " .")
	if nombre == "" || nombre == "_" {
		return id
	}
	return nombre
}

// copiarSinPisar es escribirSinPisar con un original externo, comprobado al copiarlo.
func copiarSinPisar(ctx context.Context, carpeta, nombre, origen string, tamano int64, huella string) (nueva bool, err error) {
	return conHuellaSinPisar(carpeta, nombre, tamano, huella, func(w io.Writer) error { return copiarExterno(ctx, w, origen, tamano, huella) })
}

// conHuellaSinPisar es sinPisar con la huella en hexadecimal y lo que escriba escribir.
func conHuellaSinPisar(carpeta, nombre string, tamano int64, huella string, escribir func(io.Writer) error) (bool, error) {
	raw, err := hex.DecodeString(huella)
	if err != nil || len(raw) != sha256.Size {
		return false, ErrSource
	}
	var suma [32]byte
	copy(suma[:], raw)
	return sinPisar(carpeta, nombre, tamano, suma, func(ruta string) error { return escribirAtomicoCon(ruta, escribir) })
}

// escribirSinPisar deja datos en carpeta/nombre. Si ya existe con los mismos bytes no
// hace nada (nueva = false); si existe con otros, prueba «nombre (2).ext», «(3)», etc.
func escribirSinPisar(carpeta, nombre string, datos []byte) (nueva bool, err error) {
	return sinPisar(carpeta, nombre, int64(len(datos)), sha256.Sum256(datos), func(ruta string) error { return escribirAtomico(ruta, datos) })
}

// sinPisar busca en carpeta el primer «nombre», «nombre (2)»… libre o con ese mismo
// contenido, y en el libre escribe con escribir.
func sinPisar(carpeta, nombre string, tamano int64, suma [32]byte, escribir func(ruta string) error) (nueva bool, err error) {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return false, err
	}
	ext := filepath.Ext(nombre)
	base := strings.TrimSuffix(nombre, ext)
	for n := 1; n < 10000; n++ {
		candidato := nombre
		if n > 1 {
			candidato = base + " (" + strconv.Itoa(n) + ")" + ext
		}
		ruta := filepath.Join(carpeta, candidato)
		igual, err := mismoContenido(ruta, tamano, suma)
		if errors.Is(err, os.ErrNotExist) {
			return true, escribir(ruta)
		}
		if err != nil {
			return false, err
		}
		if igual {
			return false, nil
		}
	}
	return false, os.ErrExist
}

func mismoContenido(ruta string, tamano int64, suma [32]byte) (bool, error) {
	f, err := os.Open(ruta) //nolint:gosec // la carpeta la eligió la persona
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.Size() != tamano {
		return false, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return bytes.Equal(h.Sum(nil), suma[:]), nil
}

func escribirAtomico(ruta string, datos []byte) error {
	return escribirAtomicoCon(ruta, func(w io.Writer) error {
		_, err := w.Write(datos)
		return err
	})
}

// escribirAtomicoCon es escribirAtomico con lo que escriba escribir.
func escribirAtomicoCon(ruta string, escribir func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(ruta), "."+filepath.Base(ruta)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := escribir(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Un fichero que apareció entre la comprobación y aquí no se pisa: Link falla si
	// el destino existe (donde el sistema no tiene enlaces duros, se renombra).
	if err := os.Link(tmp.Name(), ruta); err == nil {
		return nil
	} else if errors.Is(err, os.ErrExist) {
		return err
	}
	return os.Rename(tmp.Name(), ruta)
}
