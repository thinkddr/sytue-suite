// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/nucleo/local/videoteca"
)

// codificarJPEG pasa un fotograma a JPEG para el póster.
func codificarJPEG(img image.Image) ([]byte, error) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// fotogramaJPEG es Opciones.Fotograma de la videoteca: el fotograma del instante en
// con el descodificador de la app (WebM) o el del sistema registrado (CA-06, CA-08).
func fotogramaJPEG(_ context.Context, ruta string, en time.Duration) ([]byte, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := video.Fotograma(f, en)
	if err != nil {
		return nil, err
	}
	return codificarJPEG(img)
}

// lectorSistema recuerda, por fichero y fecha, si el descodificador del sistema lo abre
// (Opciones.LeeElSistema): probarlo lanza GStreamer, y la videoteca pregunta a menudo.
type lectorSistema struct {
	mu    sync.Mutex
	visto map[string]vistoSistema
	ahora func() time.Time // nil: time.Now
	// abrir prueba el descodificador del sistema; nil, los registrados en `video`.
	abrir func(ruta string) error
}

// vistoSistema es lo que contestó el descodificador del sistema por un vídeo. Que sí lo lee
// se recuerda; que no, solo un rato (reintentoSistema): en el teléfono, MediaCodec también
// dice que no cuando le faltan recursos un momento, y el vídeo no debe quedarse por
// ilegible hasta reabrir la app.
type vistoSistema struct {
	lee   bool
	hasta time.Time
}

const reintentoSistema = 2 * time.Minute

func (l *lectorSistema) lee(ruta string) bool {
	abrir := l.abrir
	if abrir == nil {
		if !video.HaySistema() {
			return false
		}
		abrir = func(ruta string) error {
			s, err := video.AbrirConSistema(ruta)
			if err == nil {
				_ = s.Close()
			}
			return err
		}
	}
	fi, err := os.Stat(ruta)
	if err != nil {
		return false
	}
	ahora := time.Now
	if l.ahora != nil {
		ahora = l.ahora
	}
	clave := fmt.Sprintf("%s|%d|%d", ruta, fi.Size(), fi.ModTime().UnixNano())
	l.mu.Lock()
	defer l.mu.Unlock()
	if v, ok := l.visto[clave]; ok && (v.lee || ahora().Before(v.hasta)) {
		return v.lee
	}
	err = abrir(ruta)
	if l.visto == nil {
		l.visto = map[string]vistoSistema{}
	}
	l.visto[clave] = vistoSistema{lee: err == nil, hasta: ahora().Add(reintentoSistema)}
	return err == nil
}

// vigiaPosters tira de la caché de pósters el de un vídeo cuyo póster ha cambiado
// (PosterN), sea quien sea quien lo cambió: así la tarjeta pide el nuevo.
type vigiaPosters struct {
	mu sync.Mutex
	n  map[string]int
}

func (v *vigiaPosters) cambio(teca *videoteca.Videoteca, id string) bool {
	e, err := teca.Entrada(id)
	if err != nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.n == nil {
		v.n = map[string]int{}
	}
	antes := v.n[id]
	v.n[id] = e.PosterN
	return antes != e.PosterN
}

// conocer apunta los pósters que ya hay al abrir.
func (v *vigiaPosters) conocer(teca *videoteca.Videoteca) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.n == nil {
		v.n = map[string]int{}
	}
	for _, e := range teca.Lista() {
		v.n[e.ID] = e.PosterN
	}
}

// colecciones es la pantalla de las colecciones: cada una con sus vídeos y cuántos
// están publicados; «Abrir» la enseña en la página de canal de la app (todos sus
// vídeos, publicados o no).
func (c *controlador) colecciones() {
	cs := c.teca.Colecciones()
	lista := container.NewVBox()
	if len(cs) == 0 {
		l := widget.NewLabel(c.t("colecciones.vacio"))
		l.Wrapping = fyne.TextWrapWord
		lista.Add(l)
	}
	var d interface{ Cerrar() }
	for _, k := range cs {
		nombre := k.Nombre
		info := widget.NewLabel(fmt.Sprintf(c.t("colecciones.cuenta"), k.Videos, k.Publicados))
		info.Importance = widget.LowImportance
		titulo := widget.NewLabelWithStyle(nombre, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		abrir := widget.NewButton(c.t("colecciones.abrir"), func() {
			d.Cerrar()
			c.p.AbrirCanal(videoteca.PrefijoColeccion + nombre)
		})
		lista.Add(container.NewBorder(nil, nil, nil, abrir, container.NewVBox(titulo, info)))
		lista.Add(widget.NewSeparator())
	}
	ayuda := widget.NewLabel(c.t("colecciones.ayuda"))
	ayuda.Wrapping = fyne.TextWrapWord
	ayuda.Importance = widget.LowImportance
	desliza := container.NewVScroll(lista)
	desliza.SetMinSize(fyne.NewSize(520, min(lista.MinSize().Height, 420)))
	cuerpo := container.NewBorder(nil, ayuda, nil, nil, desliza)
	d = local.Ventana(c.w, c.t("colecciones"), cuerpo, nil)
}
