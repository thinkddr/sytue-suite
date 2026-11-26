// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package sistema

import (
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/video"
	"git.sytue.eu/sytue/one/apps/fyne/video/sistema/mediafoundation"
)

// Registrar registra Media Foundation en el reproductor si este Windows lo tiene (en
// Windows N sin el paquete multimedia no está), y dice si lo ha hecho. Con él, Cast y
// Photos ven dentro de la app lo que no es WebM (MP4, MOV, M4V, MKV, AVI, WMV… lo que
// tenga descodificador en el sistema). Un fichero que no sabe abrir es un error de
// AbrirSistema, y la app sigue con lo de siempre (ffmpeg o el reproductor del sistema).
func Registrar() bool {
	if !mediafoundation.Disponible() {
		return false
	}
	video.RegistrarSistema(func(ruta string) (video.Sistema, error) {
		l, err := mediafoundation.Abrir(ruta)
		if err != nil {
			return nil, err
		}
		return lectorMF{l}, nil
	})
	return true
}

// lectorMF es el lector de Media Foundation como video.Sistema.
type lectorMF struct{ l *mediafoundation.Lector }

var _ video.Sistema = lectorMF{}

func (s lectorMF) Info() video.InfoSistema {
	in := s.l.Info()
	return video.InfoSistema{Duracion: in.Duracion, Ancho: in.Ancho, Alto: in.Alto, ConVideo: in.ConVideo, ConAudio: in.ConAudio}
}

func (s lectorMF) Siguiente() (video.Muestra, error) {
	m, err := s.l.Siguiente()
	if err != nil {
		return video.Muestra{}, err
	}
	return video.Muestra{Tiempo: m.Tiempo, Imagen: m.Imagen, PCM: m.PCM}, nil
}

func (s lectorMF) Buscar(t time.Duration) (time.Duration, error) { return s.l.Buscar(t) }

func (s lectorMF) Close() error { return s.l.Close() }
