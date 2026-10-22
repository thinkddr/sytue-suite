// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package llamada

import (
	"errors"
	"image"
	"sync"
	"time"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/thesyncim/govpx"

	"git.sytue.eu/sytue/one/apps/fyne/llamada/camara"
)

// Lo que se PUBLICA de uno mismo: la cámara y la pantalla. Cada una es una pista VP8 que
// se publica la primera vez que se enciende y luego se silencia y se reanuda, como hace
// la web con `setCameraEnabled`. Apagar suelta el dispositivo (se apaga la luz de la
// cámara); encender lo vuelve a abrir y fuerza un fotograma clave.

// Tallas de lo que se manda: la cámara a la capa media de la web (640×360, 15 fps, 600
// kbps) y la pantalla a 1280×720 a 5 fps, que es lo que un texto necesita para leerse.
var (
	TallaCamara   = talla{640, 360, 15, 600}
	TallaPantalla = talla{1280, 720, 5, 1200}
)

type talla struct{ ancho, alto, fps, kbps int }

// ErrSinFuente es encender la cámara o la pantalla sin quién la abra.
var ErrSinFuente = errors.New("llamada: sin cámara ni pantalla que abrir")

// videoPropio es una de las dos pistas propias.
type videoPropio struct {
	fuenteLK livekit.TrackSource
	talla    talla

	mu     sync.Mutex
	pista  *lksdk.LocalTrack
	pub    *lksdk.LocalTrackPublication
	parar  chan struct{}
	ultimo image.Image
	clave  bool // pedido un fotograma clave (PLI del SFU o reanudación)
}

// encender abre la fuente y empieza a mandar. `abrir` es la cámara o la pantalla.
func (l *Llamada) encender(v *videoPropio, abrir func() (camara.Fuente, error)) error {
	if abrir == nil {
		return ErrSinFuente
	}
	f, err := abrir()
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.parar != nil { // ya encendida
		f.Cerrar()
		return nil
	}
	if v.pista == nil {
		options, err := l.opcionesCifrado(lksdk.CodecVP8)
		if err != nil {
			f.Cerrar()
			return err
		}
		options = append(options, lksdk.WithRTCPHandler(func(p rtcp.Packet) {
			switch p.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				v.mu.Lock()
				v.clave = true
				v.mu.Unlock()
			}
		}))
		pista, err := lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}, options...)
		if err != nil {
			f.Cerrar()
			return err
		}
		nombre := "camera"
		if v.fuenteLK == livekit.TrackSource_SCREEN_SHARE {
			nombre = "screen"
		}
		pub, err := l.room.LocalParticipant.PublishTrack(pista, &lksdk.TrackPublicationOptions{
			Name: nombre, Source: v.fuenteLK, VideoWidth: v.talla.ancho, VideoHeight: v.talla.alto,
		})
		if err != nil {
			f.Cerrar()
			return err
		}
		v.pista, v.pub = pista, pub
	}
	v.pub.SetMuted(false)
	v.clave = true
	v.parar = make(chan struct{})
	go l.mandar(v, f, v.pista, v.parar)
	return nil
}

// apagar suelta la fuente y silencia la pista.
func (v *videoPropio) apagar() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.parar != nil {
		close(v.parar)
		v.parar = nil
	}
	if v.pub != nil {
		v.pub.SetMuted(true)
	}
	v.ultimo = nil
}

func (v *videoPropio) fotograma() image.Image {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.ultimo
}

// mandar lee la fuente a su ritmo, escala, codifica y manda. Un fotograma clave cada dos
// segundos además de los que pida el SFU: quien entra tarde ve imagen enseguida.
func (l *Llamada) mandar(v *videoPropio, f camara.Fuente, pista *lksdk.LocalTrack, parar <-chan struct{}) {
	defer f.Cerrar()
	tic := time.NewTicker(time.Second / time.Duration(v.talla.fps))
	defer tic.Stop()
	var (
		enc          *govpx.VP8Encoder
		w, h         int
		buf          = make([]byte, 1<<20)
		pts          uint64
		ultimaClave  time.Time
		intervaloDur = time.Second / time.Duration(v.talla.fps)
	)
	defer func() {
		if enc != nil {
			_ = enc.Close()
		}
	}()
	for {
		select {
		case <-parar:
			return
		case <-tic.C:
		}
		img, ok := f.Leer()
		if !ok {
			continue
		}
		img = camara.Escalar(img, v.talla.ancho, v.talla.alto)
		if enc == nil || img.Rect.Dx() != w || img.Rect.Dy() != h {
			if enc != nil {
				_ = enc.Close()
			}
			w, h = img.Rect.Dx(), img.Rect.Dy()
			var err error
			enc, err = govpx.NewVP8Encoder(govpx.EncoderOptions{
				Width: w, Height: h, FPS: v.talla.fps, TargetBitrateKbps: v.talla.kbps,
				RateControlMode: govpx.RateControlCBR, Threads: 2,
				// Tiempo real: la llamada no espera a fotogramas futuros y no puede tardar
				// más de lo que dura uno (cpu-used 8 es lo que usa WebRTC en libvpx).
				Deadline: govpx.DeadlineRealtime, CpuUsed: 8,
			})
			if err != nil {
				return
			}
			ultimaClave = time.Time{}
		}
		v.mu.Lock()
		clave := v.clave || time.Since(ultimaClave) > 2*time.Second
		v.clave = false
		v.ultimo = img
		v.mu.Unlock()
		banderas := govpx.EncodeFlags(0)
		if clave {
			banderas, ultimaClave = govpx.EncodeForceKeyFrame, time.Now()
		}
		r, err := enc.EncodeInto(buf, aGovpx(img), pts, 1, banderas)
		pts++
		if err == nil && len(r.Data) > 0 {
			_ = pista.WriteSample(media.Sample{Data: append([]byte(nil), r.Data...), Duration: intervaloDur}, nil)
		}
		l.fotograma()
	}
}

// aGovpx presenta una imagen 4:2:0 de Go al codificador sin copiarla.
func aGovpx(img *image.YCbCr) govpx.Image {
	return govpx.Image{
		Width: img.Rect.Dx(), Height: img.Rect.Dy(),
		Y: img.Y, U: img.Cb, V: img.Cr,
		YStride: img.YStride, UStride: img.CStride, VStride: img.CStride,
	}
}
