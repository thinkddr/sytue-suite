// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package service junta las piezas: Drive, EXIF, el índice y las derivadas.
package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"strings"

	"github.com/gen2brain/h265/heic"

	"git.sytue.eu/sytue/one/apps/nucleo/miniatura"
	photorender "git.sytue.eu/sytue/one/apps/nucleo/photos/render"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
	"git.sytue.eu/sytue/one/services/syphotos/internal/exif"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// LA INGESTA VA EN DOS FASES, y esa es la decisión de esta capa.
//
//	INDEXAR   lee 128 KiB de cabecera, saca EXIF y dimensiones, y escribe la
//	          fila. Milisegundos. La foto YA aparece en la línea de tiempo.
//	DERIVAR   lee el original entero, decodifica una vez y genera las tres
//	          variantes en cascada. Cientos de milisegundos y decenas de MB.
//
// Hacerlo en una sola fase obligaría a esperar la descarga completa de cada foto
// antes de enseñar nada: subir un carrete de doscientas dejaría la galería vacía
// varios minutos. Separadas, la cuadrícula se llena de huecos con su tamaño
// correcto —que se sabe del EXIF— y las miniaturas entran cuando están.
//
// Y las dimensiones salen de `DecodeConfig` sobre esos mismos 128 KiB, sin
// decodificar un píxel: la cabecera SOF de un JPEG va con las demás, y la caja
// `meta` de un HEIC también (miniatura.Cabecera, que la da ya girada).

// Ingesta orquesta el alta de una foto.
type Ingesta struct {
	store    *store.Store
	drive    *drive.Cliente
	objetos  *objectstore.Client
	cabecera int64
	lados    []int
}

// NuevaIngesta construye el orquestador.
func NuevaIngesta(st *store.Store, dr *drive.Cliente, obj *objectstore.Client, bytesCabecera int64, lados []int) *Ingesta {
	if bytesCabecera <= 0 {
		bytesCabecera = exif.BytesDeCabecera
	}
	return &Ingesta{store: st, drive: dr, objetos: obj, cabecera: bytesCabecera, lados: lados}
}

// Indexar da de alta una foto de Drive en la galería leyendo SOLO su cabecera.
//
// Es idempotente por el fichero de Drive (lo garantiza el store), así que
// reindexar una carpeta entera no duplica nada y sí mejora lo que la primera
// pasada no supo leer.
//
// `idioma` es el del perfil de quien la da de alta (claim `locale`), para que el
// lote de IA la describa en él. Vacío = no se sabe, y no borra el que hubiera.
func (i *Ingesta) Indexar(ctx context.Context, token, driveID, duenyo, idioma string) (domain.Activo, error) {
	f, err := i.drive.Metadatos(ctx, token, driveID)
	if err != nil {
		return domain.Activo{}, err
	}
	// Solo lo PROPIO entra en la galería de cada cual: poder ver una foto de otra
	// persona (compartida, o en una unidad de equipo) no la copia, con su EXIF y
	// sus derivadas, a la galería de quien mira, donde seguiría aunque le quitaran
	// el acceso (FILE-11).
	if f.Duenyo != duenyo {
		return domain.Activo{}, fmt.Errorf("%w: solo se añaden a la galería tus propias fotos", domain.ErrInvalidInput)
	}
	cab, err := i.drive.Cabecera(ctx, token, driveID, i.cabecera)
	if err != nil {
		return domain.Activo{}, err
	}

	// Las dimensiones son OBLIGATORIAS: sin ellas no hay dónde colocar el hueco
	// en la cuadrícula, y el store lo rechaza. Salen de la cabecera, sin
	// decodificar.
	cfg, formato, err := miniatura.Cabecera(cab)
	if err != nil {
		return domain.Activo{}, fmt.Errorf("%w: %s no es una imagen que se pueda medir",
			domain.ErrInvalidInput, f.Nombre)
	}

	a := domain.Activo{
		Duenyo: duenyo, DriveID: driveID, Nombre: f.Nombre,
		Tipo: f.ContentType, Bytes: f.Bytes,
		Ancho: cfg.Width, Alto: cfg.Height,
		IdiomaDuenyo: idioma,
	}
	// El EXIF es OPCIONAL: hay millones de imágenes sin él —capturas, PNG— y
	// tratarlo como error impediría subirlas. Lo que no trae, se queda vacío.
	if m, err := leerExif(cab, formato); err == nil {
		a.Orientacion = domain.OrientacionDe(int(m.Orientacion))
		a.TomadaEn = m.TomadaEn
		a.Lat, a.Lon = m.Lat, m.Lon
		a.Camara = m.Camara
		a.Objetivo = m.Objetivo
	}
	return i.store.Ingerir(ctx, a)
}

// leerExif es el EXIF de la cabecera. El de un HEIC no va en un segmento APP1 sino en
// su elemento `Exif` (heic.RawExif), y su Orientation NO cuenta: en HEIF manda el `irot`
// del contenedor, que miniatura ya aplica al decodificar, y girar además con el EXIF
// pondría la foto de lado. La fecha, la cámara y el lugar sí valen. Si el elemento cae
// fuera de los bytes de cabecera, la foto entra sin ellos, como una sin EXIF.
func leerExif(cab []byte, formato string) (exif.Metadatos, error) {
	if formato != "heic" {
		return exif.Leer(cab)
	}
	tiff, err := heic.RawExif(bytes.NewReader(cab))
	if err != nil {
		return exif.Metadatos{}, exif.ErrSinExif
	}
	m, err := exif.LeerTIFF(tiff)
	m.Orientacion = 0
	return m, err
}

// Derivar genera y guarda las variantes de una foto para una receta dada.
//
// LEE EL ORIGINAL UNA VEZ y decodifica UNA vez; las tres variantes salen en
// cascada de esa única decodificación (ver `miniatura.Cascada`, −42% medido).
// Descargar tres veces o decodificar tres veces es el desperdicio evidente que
// hay que no cometer.
//
// La orientación y la receta se aplican en el motor compartido antes de reducir.
// EsPNG determina si el activo corresponde a una imagen PNG que puede contener transparencia.
func EsPNG(a domain.Activo) bool {
	return strings.EqualFold(a.Tipo, "image/png") || strings.HasSuffix(strings.ToLower(a.Nombre), ".png")
}

// aplanarSobreBlanco compone una imagen sobre fondo blanco sólido para codificarla
// en formatos sin soporte de transparencia (como JPEG), evitando que píxeles transparentes
// se vuelvan negros.
func aplanarSobreBlanco(src image.Image) image.Image { return photorender.AplanarSobreBlanco(src) }

func (i *Ingesta) Derivar(ctx context.Context, token string, a domain.Activo, rec domain.Receta) error {
	return i.store.WithLegacyOwnerWrite(ctx, a.Duenyo, func() error { return i.derivar(ctx, token, a, rec) })
}

func (i *Ingesta) derivar(ctx context.Context, token string, a domain.Activo, rec domain.Receta) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	huella, err := rec.Huella()
	if err != nil {
		return err
	}
	return photorender.Derivar(ctx, func(ctx context.Context) ([]byte, error) { return i.originalBytes(ctx, token, a) }, a.Orientacion, rec, EsPNG(a), i.lados, func(lado int, formato, contentType string, encoded []byte) error {
		d := domain.Derivada{ActivoID: a.ID, Huella: huella, Variante: varianteDeLado(lado), Formato: formato}
		clave, err := d.Clave()
		if err != nil {
			return err
		}
		if err := i.objetos.Put(ctx, t, clave, bytes.NewReader(encoded), int64(len(encoded)), objectstore.PutOptions{ContentType: contentType}); err != nil {
			return fmt.Errorf("syphotos: guardar %s: %w", clave, err)
		}
		return nil
	})
}

// maxOriginal acota lo que se lee de Drive. Solo se decodifica JPEG, PNG, GIF, WebP y
// HEIC, y lo que cabe en miniatura.MaxBytes ya decodificado no ocupa más que esto
// comprimido. Con 512 MiB, leer un fichero grande ya llenaba el pod (FILE-07).
const maxOriginal = photorender.MaxOriginal

func (i *Ingesta) originalBytes(ctx context.Context, token string, a domain.Activo) ([]byte, error) {
	// El tamaño se mira ANTES de leer: el activo ya lo sabe.
	if a.Bytes > maxOriginal {
		return nil, fmt.Errorf("%w: la foto pesa más de lo que se puede procesar", domain.ErrInvalidInput)
	}
	rc, err := i.drive.Abrir(ctx, token, a.DriveID)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	crudo, err := io.ReadAll(io.LimitReader(rc, maxOriginal+1))
	if err != nil {
		return crudo, fmt.Errorf("syphotos: leer el original: %w", err)
	}
	if len(crudo) > maxOriginal {
		return crudo, fmt.Errorf("%w: la foto pesa más de lo que se puede procesar", domain.ErrInvalidInput)
	}
	return crudo, nil
}

// CalidadExportacion: la exportación es lo que la persona se lleva; se codifica
// con más calidad que las derivadas de pantalla.
const CalidadExportacion = photorender.CalidadExportacion

// Exportar escribe en w la foto a resolución completa con la receta aplicada:
// PNG sin pérdida si formato es "png", JPEG con CalidadExportacion en cualquier
// otro caso. No se guarda: exportar es raro y el resultado pesa lo que el
// original.
func (i *Ingesta) Exportar(ctx context.Context, w io.Writer, token string, a domain.Activo, rec domain.Receta, formato string) error {
	return photorender.Exportar(ctx, w, func(ctx context.Context) ([]byte, error) { return i.originalBytes(ctx, token, a) }, a.Orientacion, rec, formato)
}

// varianteDeLado traduce el tamaño al nombre que va en la clave.
func varianteDeLado(lado int) domain.Variante {
	for _, v := range domain.Variantes() {
		if n, ok := v.LadoMayor(); ok && n == lado {
			return v
		}
	}
	return ""
}
