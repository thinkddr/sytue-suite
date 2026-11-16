// SPDX-License-Identifier: Apache-2.0
// Package exif preserves the service's historical surface over the shared
// parser used by authorized Photos devices.
package exif

import shared "git.sytue.eu/sytue/one/apps/nucleo/photos/exif"

type Metadatos = shared.Metadatos

const BytesDeCabecera = shared.BytesDeCabecera

var ErrSinExif = shared.ErrSinExif

func Leer(datos []byte) (Metadatos, error) { return shared.Leer(datos) }
