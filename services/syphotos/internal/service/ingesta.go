// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package service junta las piezas: Drive, EXIF, el índice y las derivadas.
package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/miniatura"
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
// decodificar un píxel: la cabecera SOF de un JPEG va con las demás.

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
func (i *Ingesta) Indexar(ctx context.Context, token, driveID, duenyo string) (domain.Activo, error) {
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
	cfg, _, err := image.DecodeConfig(bytes.NewReader(cab))
	if err != nil {
		return domain.Activo{}, fmt.Errorf("%w: %s no es una imagen que se pueda medir",
			domain.ErrInvalidInput, f.Nombre)
	}

	a := domain.Activo{
		Duenyo: duenyo, DriveID: driveID, Nombre: f.Nombre,
		Tipo: f.ContentType, Bytes: f.Bytes,
		Ancho: cfg.Width, Alto: cfg.Height,
	}
	// El EXIF es OPCIONAL: hay millones de imágenes sin él —capturas, PNG— y
	// tratarlo como error impediría subirlas. Lo que no trae, se queda vacío.
	if m, err := exif.Leer(cab); err == nil {
		a.Orientacion = domain.OrientacionDe(int(m.Orientacion))
		a.TomadaEn = m.TomadaEn
		a.Lat, a.Lon = m.Lat, m.Lon
		a.Camara = m.Camara
		a.Objetivo = m.Objetivo
	}
	return i.store.Ingerir(ctx, a)
}

// Derivar genera y guarda las variantes de una foto para una receta dada.
//
// LEE EL ORIGINAL UNA VEZ y decodifica UNA vez; las tres variantes salen en
// cascada de esa única decodificación (ver `miniatura.Cascada`, −42% medido).
// Descargar tres veces o decodificar tres veces es el desperdicio evidente que
// hay que no cometer.
//
// De momento aplica la ORIENTACIÓN y nada más: los pasos de la receta se
// aplicarán cuando el motor de píxeles esté, y hasta entonces la huella de la
// receta sigue nombrando el fichero — así que cuando lleguen, las claves ya
// están bien y no hay que renombrar nada.
// EsPNG determina si el activo corresponde a una imagen PNG que puede contener transparencia.
func EsPNG(a domain.Activo) bool {
	return strings.EqualFold(a.Tipo, "image/png") || strings.HasSuffix(strings.ToLower(a.Nombre), ".png")
}

// aplanarSobreBlanco compone una imagen sobre fondo blanco sólido para codificarla
// en formatos sin soporte de transparencia (como JPEG), evitando que píxeles transparentes
// se vuelvan negros.
func aplanarSobreBlanco(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, b, src, b.Min, draw.Over)
	return dst
}

func (i *Ingesta) Derivar(ctx context.Context, token string, a domain.Activo, rec domain.Receta) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	huella, err := rec.Huella()
	if err != nil {
		return err
	}
	// Una foto a la vez en el proceso: el coste que mide miniatura.Cabe es el de
	// UNA, y derivar y exportar a la vez lo multiplicaban (FILE-07).
	liberar := miniatura.Turno()
	defer liberar()
	img, err := i.imagenEditada(ctx, token, a, rec)
	if err != nil {
		return err
	}

	esPng := EsPNG(a)
	formato := "jpg"
	contentType := "image/jpeg"
	if esPng {
		formato = "png"
		contentType = "image/png"
	}

	for lado, variante := range i.cascadaPorVariante(img) {
		var buf bytes.Buffer
		if esPng {
			if err := png.Encode(&buf, variante); err != nil {
				return fmt.Errorf("syphotos: codificar la variante PNG de %d px: %w", lado, err)
			}
		} else {
			if err := jpeg.Encode(&buf, aplanarSobreBlanco(variante), &jpeg.Options{Quality: miniatura.CalidadJPEG}); err != nil {
				return fmt.Errorf("syphotos: codificar la variante de %d px: %w", lado, err)
			}
		}
		d := domain.Derivada{
			ActivoID: a.ID, Huella: huella,
			Variante: varianteDeLado(lado), Formato: formato,
		}
		clave, err := d.Clave()
		if err != nil {
			return err
		}
		if err := i.objetos.Put(ctx, t, clave, bytes.NewReader(buf.Bytes()),
			int64(buf.Len()), objectstore.PutOptions{ContentType: contentType}); err != nil {
			return fmt.Errorf("syphotos: guardar %s: %w", clave, err)
		}
	}
	return nil
}

// maxOriginal acota lo que se lee de Drive. Solo se decodifica JPEG, PNG y GIF,
// y lo que cabe en miniatura.MaxBytes ya decodificado no ocupa más que esto
// comprimido. Con 512 MiB, leer un fichero grande ya llenaba el pod (FILE-07).
const maxOriginal = 96 << 20

// imagenEditada baja el original de Drive, lo decodifica con la barrera anti
// bomba —`DecodeConfig` ANTES de `Decode`—, lo endereza según el EXIF y le
// aplica la receta. Es lo que comparten las derivadas y la exportación: la
// misma imagen, a distinto tamaño.
func (i *Ingesta) imagenEditada(ctx context.Context, token string, a domain.Activo, rec domain.Receta) (image.Image, error) {
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
		return nil, fmt.Errorf("syphotos: leer el original: %w", err)
	}
	if len(crudo) > maxOriginal {
		return nil, fmt.Errorf("%w: la foto pesa más de lo que se puede procesar", domain.ErrInvalidInput)
	}
	img, err := miniatura.Decodificar(crudo)
	if err != nil {
		return nil, fmt.Errorf("%w: imagen no soportada o demasiado grande para procesarla", domain.ErrInvalidInput)
	}
	return aplicarReceta(orientar(img, a.Orientacion), rec), nil
}

// CalidadExportacion: la exportación es lo que la persona se lleva; se codifica
// con más calidad que las derivadas de pantalla.
const CalidadExportacion = 92

// Exportar escribe en w la foto a resolución completa con la receta aplicada:
// PNG sin pérdida si formato es "png", JPEG con CalidadExportacion en cualquier
// otro caso. No se guarda: exportar es raro y el resultado pesa lo que el
// original.
func (i *Ingesta) Exportar(ctx context.Context, w io.Writer, token string, a domain.Activo, rec domain.Receta, formato string) error {
	// Una foto a la vez en el proceso: el coste que mide miniatura.Cabe es el de
	// UNA, y derivar y exportar a la vez lo multiplicaban (FILE-07).
	liberar := miniatura.Turno()
	defer liberar()
	img, err := i.imagenEditada(ctx, token, a, rec)
	if err != nil {
		return err
	}
	if formato == "png" {
		if err := png.Encode(w, img); err != nil {
			return fmt.Errorf("syphotos: codificar la exportación PNG: %w", err)
		}
		return nil
	}
	if err := jpeg.Encode(w, aplanarSobreBlanco(img), &jpeg.Options{Quality: CalidadExportacion}); err != nil {
		return fmt.Errorf("syphotos: codificar la exportación: %w", err)
	}
	return nil
}

func (i *Ingesta) cascadaPorVariante(img image.Image) map[int]*image.NRGBA {
	return miniatura.Cascada(img, i.lados)
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
