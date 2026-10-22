// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package camara

import (
	"errors"
	"testing"
	"unsafe"
)

// Los tamaños de las estructuras de V4L2 son los que llevan codificados sus números de
// ioctl (bits 16-29): si una estructura de aquí se descuadra respecto a la del núcleo,
// el ioctl devuelve EINVAL o, peor, escribe fuera. No hay cámara en el frontal; esto es
// lo que se puede comprobar sin ella.
//
// VISTO EN ROJO (2026-09-26, en el frontal): quitando el relleno de alineación de
// v4l2Format, S_FMT no casa (200 en vez de 208).
func TestLasEstructurasDeV4L2(t *testing.T) {
	tam := func(req uintptr) uintptr { return (req >> 16) & 0x3fff }
	for _, c := range []struct {
		nombre string
		req    uintptr
		tam    uintptr
	}{
		{"QUERYCAP", vidiocQuerycap, unsafe.Sizeof(v4l2Capability{})},
		{"S_FMT", vidiocSFmt, unsafe.Sizeof(v4l2Format{})},
		{"REQBUFS", vidiocReqbufs, unsafe.Sizeof(v4l2RequestBuffers{})},
		{"QUERYBUF", vidiocQuerybuf, unsafe.Sizeof(v4l2Buffer{})},
		{"QBUF", vidiocQbuf, unsafe.Sizeof(v4l2Buffer{})},
		{"DQBUF", vidiocDqbuf, unsafe.Sizeof(v4l2Buffer{})},
		{"S_PARM", vidiocSParm, unsafe.Sizeof(v4l2StreamParm{})},
	} {
		if tam(c.req) != c.tam {
			t.Errorf("%s: el ioctl dice %d bytes y la estructura mide %d", c.nombre, tam(c.req), c.tam)
		}
	}
	if o := unsafe.Offsetof(v4l2Buffer{}.Offset); o != 64 {
		t.Errorf("v4l2_buffer.m está en %d y en el núcleo en 64", o)
	}
	if o := unsafe.Offsetof(v4l2Format{}.Pix); o != 8 {
		t.Errorf("v4l2_format.fmt está en %d y en el núcleo en 8", o)
	}
}

// Sin cámaras, Abrir dice ErrSinCamara y no se cuelga.
func TestSinCamaraEnLinux(t *testing.T) {
	if len(Dispositivos()) > 0 {
		t.Skip("este equipo tiene cámara")
	}
	if _, err := Abrir(PedidoPorOmision); !errors.Is(err, ErrSinCamara) {
		t.Errorf("sin cámara, Abrir da %v", err)
	}
}
