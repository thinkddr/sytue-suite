// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package llamada

import (
	"image"
	"strings"
	"time"

	e2eetypes "github.com/livekit/server-sdk-go/v2/e2ee/types"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
	"github.com/thesyncim/govpx"
)

// leerVideo junta los paquetes RTP de una pista VP8 en fotogramas, los descodifica y
// entrega cada uno como imagen. Otro códec no se pinta: la web publica VP8 por defecto.
//
// Hasta el primer fotograma clave no hay nada que pintar, y el SFU no siempre manda uno
// al suscribirse: se pide con PLI, como mucho una vez por segundo mientras falte.
func leerVideo(t *webrtc.TrackRemote, decrypt e2eetypes.FrameDecryptor, pedirClave func(), entregar func(image.Image)) {
	if !strings.EqualFold(t.Codec().MimeType, webrtc.MimeTypeVP8) {
		return
	}
	dec, err := govpx.NewVP8Decoder(govpx.DecoderOptions{ErrorConcealment: true, MaxWidth: 3840, MaxHeight: 2160})
	if err != nil {
		return
	}
	defer func() { _ = dec.Close() }()
	sb := samplebuilder.New(128, &codecs.VP8Packet{}, t.Codec().ClockRate)
	var ultimaPeticion time.Time
	pedir := func() {
		if time.Since(ultimaPeticion) > time.Second {
			ultimaPeticion = time.Now()
			pedirClave()
		}
	}
	pedir()
	for {
		pkt, _, err := t.ReadRTP()
		if err != nil {
			return
		}
		sb.Push(pkt)
		for s := sb.Pop(); s != nil; s = sb.Pop() {
			data := s.Data
			if decrypt != nil {
				data, err = decrypt.DecryptFrame(data)
				if err != nil || len(data) == 0 {
					pedir()
					continue
				}
			}
			if err := dec.Decode(data); err != nil {
				pedir()
				continue
			}
			if f, ok := dec.NextFrame(); ok {
				entregar(ImagenI420(f))
			}
		}
	}
}

// ImagenI420 copia un fotograma del descodificador a una imagen de Go. Copia porque el
// descodificador reutiliza sus planos en el siguiente.
//
// ponytail: una imagen nueva por fotograma (1,4 MB a 720p, ~40 MB/s a 30 fps). El
// recolector lo lleva sin despeinarse; si una rejilla grande se nota, dos búferes por
// pista y turnarlos.
func ImagenI420(f govpx.Image) *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, f.Width, f.Height), image.YCbCrSubsampleRatio420)
	copiarPlano(img.Y, img.YStride, f.Y, f.YStride, f.Width, f.Height)
	cw, ch := (f.Width+1)/2, (f.Height+1)/2
	copiarPlano(img.Cb, img.CStride, f.U, f.UStride, cw, ch)
	copiarPlano(img.Cr, img.CStride, f.V, f.VStride, cw, ch)
	return img
}

func copiarPlano(dst []byte, dstStride int, src []byte, srcStride, ancho, alto int) {
	for y := range alto {
		copy(dst[y*dstStride:y*dstStride+ancho], src[y*srcStride:y*srcStride+ancho])
	}
}
