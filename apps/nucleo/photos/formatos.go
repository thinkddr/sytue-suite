// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

// AdmitirFormatos deja que la ingesta directa (ImportarFoto) acepte, además de JPEG,
// PNG y GIF, los formatos de imagen nombrados como los registra `image.RegisterFormat`
// («webp», «avif»…). El decodificador tiene que estar registrado en el binario: WebP
// viene con `miniatura`; AVIF lo registra quien importe `github.com/gen2brain/avif`.
//
// El límite a los tres de siempre venía de que el servicio de la edición conectada solo
// sabe medirlos; Sytue Office mide y deriva en el dispositivo (FO-03). Se llama antes
// de usar el repositorio. Sin llamarlo, todo sigue como hasta ahora.
func (r *Repository) AdmitirFormatos(formatos ...string) {
	if r.formatosExtra == nil {
		r.formatosExtra = map[string]bool{}
	}
	for _, f := range formatos {
		r.formatosExtra[f] = true
	}
}

// LiberarAlPurgar hace que Purgar, además de quitar la foto de la galería, suelte de su
// fila el original retenido y las derivadas (o las de un original externo, cuyo fichero
// NO se toca): así, al compactar el almacén (`local/almacen`, §5.6), sus bytes se van del
// disco. La huella del original se queda en la fila (`original_sha256`) para que lo
// borrado no se vuelva a importar. Se llama antes de usar el repositorio; sin llamarlo,
// Purgar hace lo de siempre.
func (r *Repository) LiberarAlPurgar() { r.liberarAlPurgar = true }
