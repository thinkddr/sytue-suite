// SPDX-License-Identifier: Apache-2.0
// Package exif preserves the service's historical surface over the shared
// parser used by authorized Photos devices.
package exif

import shared "git.sytue.eu/sytue/one/apps/nucleo/photos/exif"

type Metadatos = shared.Metadatos

const BytesDeCabecera = shared.BytesDeCabecera

var ErrSinExif = shared.ErrSinExif

func Leer(datos []byte) (Metadatos, error) { return shared.Leer(datos) }

// LeerTIFF es el lector compartido sobre un bloque TIFF ya extraído (el EXIF de un HEIC).
func LeerTIFF(tiff []byte) (Metadatos, error) { return shared.LeerTIFF(tiff) }
