// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package camara

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// La cámara de Linux: V4L2 por ioctl, en Go y sin cgo. Captura por mmap con cuatro
// búferes, en YUYV (lo que da toda webcam UVC) o, si el controlador no lo ofrece a ese
// tamaño, en MJPEG, que se descodifica con image/jpeg.
//
// Las estructuras son las de <linux/videodev2.h> en 64 bits (amd64 y arm64 las alinean
// igual); `TestLasEstructurasDeV4L2` comprueba sus tamaños contra los números de ioctl,
// que los llevan codificados.

const (
	capVideoCapture = 0x00000001
	capStreaming    = 0x04000000
	capDeviceCaps   = 0x80000000

	tipoCaptura  = 1 // V4L2_BUF_TYPE_VIDEO_CAPTURE
	memoriaMmap  = 1 // V4L2_MEMORY_MMAP
	campoNinguno = 1 // V4L2_FIELD_NONE

	fourccYUYV  = 'Y' | 'U'<<8 | 'Y'<<16 | 'V'<<24
	fourccMJPEG = 'M' | 'J'<<8 | 'P'<<16 | 'G'<<24

	nBuferes = 4
)

// Los números de ioctl: _IOC(dir, 'V', nr, tamaño).
const (
	vidiocQuerycap  = 0x80685600
	vidiocSFmt      = 0xc0d05605
	vidiocReqbufs   = 0xc0145608
	vidiocQuerybuf  = 0xc0585609
	vidiocQbuf      = 0xc058560f
	vidiocDqbuf     = 0xc0585611
	vidiocStreamon  = 0x40045612
	vidiocStreamoff = 0x40045613
	vidiocSParm     = 0xc0cc5616
)

type v4l2Capability struct {
	Driver     [16]byte
	Card       [32]byte
	BusInfo    [32]byte
	Version    uint32
	Caps       uint32
	DeviceCaps uint32
	_          [3]uint32
}

type v4l2PixFormat struct {
	Width, Height, PixelFormat, Field, BytesPerLine, SizeImage, ColorSpace, Priv uint32
	Flags, YCbCrEnc, Quantization, XferFunc                                      uint32
}

type v4l2Format struct {
	Type uint32
	_    uint32 // la unión va alineada a 8: lleva punteros (v4l2_window)
	Pix  v4l2PixFormat
	_    [200 - 48]byte
}

type v4l2RequestBuffers struct {
	Count, Type, Memory, Capabilities uint32
	Flags                             uint8
	_                                 [3]uint8
}

type v4l2Buffer struct {
	Index, Type, BytesUsed, Flags, Field uint32
	_                                    uint32
	Timestamp                            unix.Timeval
	Timecode                             [16]byte
	Sequence, Memory                     uint32
	Offset                               uint64 // la unión m: offset (mmap), userptr, planes o fd
	Length                               uint32
	_                                    uint32
	_                                    uint32 // request_fd
	_                                    uint32
}

type v4l2StreamParm struct {
	Type         uint32
	Capability   uint32
	CaptureMode  uint32
	Numerator    uint32
	Denominator  uint32
	ExtendedMode uint32
	ReadBuffers  uint32
	_            [4]uint32
	_            [200 - 40]byte
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	for {
		_, _, e := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
		if e == unix.EINTR {
			continue
		}
		if e != 0 {
			return e
		}
		return nil
	}
}

// fuenteV4L2 es una cámara abierta.
type fuenteV4L2 struct {
	mu      sync.Mutex
	fd      int
	mapas   [][]byte
	formato uint32
	ancho   int
	alto    int
	cerrada bool
}

func abrir(p Pedido) (Fuente, error) {
	ruta := p.ID
	if ruta == "" {
		ds := dispositivos()
		if len(ds) == 0 {
			return nil, ErrSinCamara
		}
		ruta = ds[0].ID
	}
	fd, err := unix.Open(ruta, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EACCES) {
			return nil, ErrSinPermiso // sin estar en el grupo `video`
		}
		return nil, ErrSinCamara
	}
	f := &fuenteV4L2{fd: fd}
	if err := f.preparar(p); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return f, nil
}

func (f *fuenteV4L2) preparar(p Pedido) error {
	var fmtv v4l2Format
	for _, pf := range []uint32{fourccYUYV, fourccMJPEG} {
		fmtv = v4l2Format{Type: tipoCaptura, Pix: v4l2PixFormat{
			Width: uint32(max(p.Ancho, 160)), Height: uint32(max(p.Alto, 120)), //nolint:gosec // G115: tamaños pequeños
			PixelFormat: pf, Field: campoNinguno,
		}}
		if ioctl(f.fd, vidiocSFmt, unsafe.Pointer(&fmtv)) == nil && fmtv.Pix.PixelFormat == pf {
			break
		}
	}
	if fmtv.Pix.PixelFormat != fourccYUYV && fmtv.Pix.PixelFormat != fourccMJPEG {
		return fmt.Errorf("%w: ni YUYV ni MJPEG", ErrSinCamara)
	}
	f.formato, f.ancho, f.alto = fmtv.Pix.PixelFormat, int(fmtv.Pix.Width), int(fmtv.Pix.Height)
	if p.Fotogramas > 0 {
		parm := v4l2StreamParm{Type: tipoCaptura, Numerator: 1, Denominator: uint32(p.Fotogramas)} //nolint:gosec // G115
		_ = ioctl(f.fd, vidiocSParm, unsafe.Pointer(&parm))                                        // una petición: si no la admite, la suya
	}
	req := v4l2RequestBuffers{Count: nBuferes, Type: tipoCaptura, Memory: memoriaMmap}
	if err := ioctl(f.fd, vidiocReqbufs, unsafe.Pointer(&req)); err != nil || req.Count == 0 {
		return fmt.Errorf("%w: sin búferes", ErrSinCamara)
	}
	for i := range req.Count {
		b := v4l2Buffer{Index: i, Type: tipoCaptura, Memory: memoriaMmap}
		if err := ioctl(f.fd, vidiocQuerybuf, unsafe.Pointer(&b)); err != nil {
			return err
		}
		m, err := unix.Mmap(f.fd, int64(b.Offset), int(b.Length), unix.PROT_READ, unix.MAP_SHARED) //nolint:gosec // G115
		if err != nil {
			return err
		}
		f.mapas = append(f.mapas, m)
		if err := ioctl(f.fd, vidiocQbuf, unsafe.Pointer(&b)); err != nil {
			return err
		}
	}
	tipo := uint32(tipoCaptura)
	return ioctl(f.fd, vidiocStreamon, unsafe.Pointer(&tipo))
}

// Leer saca todos los búferes llenos, se queda con el último y los devuelve a la cola.
func (f *fuenteV4L2) Leer() (*image.YCbCr, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cerrada {
		return nil, false
	}
	var ultimo []byte
	for {
		b := v4l2Buffer{Type: tipoCaptura, Memory: memoriaMmap}
		if ioctl(f.fd, vidiocDqbuf, unsafe.Pointer(&b)) != nil {
			break // EAGAIN: no hay más
		}
		if int(b.Index) < len(f.mapas) {
			ultimo = append(ultimo[:0], f.mapas[b.Index][:min(int(b.BytesUsed), len(f.mapas[b.Index]))]...)
		}
		_ = ioctl(f.fd, vidiocQbuf, unsafe.Pointer(&b))
	}
	if ultimo == nil {
		return nil, false
	}
	if f.formato == fourccYUYV {
		if len(ultimo) < f.ancho*f.alto*2 {
			return nil, false
		}
		return DeYUYV(ultimo, f.ancho, f.alto), true
	}
	img, err := jpeg.Decode(bytes.NewReader(ultimo))
	if err != nil {
		return nil, false
	}
	y, ok := img.(*image.YCbCr)
	if !ok {
		return nil, false
	}
	return A420(y), true
}

func (f *fuenteV4L2) Cerrar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cerrada {
		return
	}
	f.cerrada = true
	tipo := uint32(tipoCaptura)
	_ = ioctl(f.fd, vidiocStreamoff, unsafe.Pointer(&tipo))
	for _, m := range f.mapas {
		_ = unix.Munmap(m)
	}
	_ = unix.Close(f.fd)
}

// dispositivos son los /dev/video* que CAPTURAN: un UVC publica dos nodos por cámara y
// el segundo es de metadatos.
func dispositivos() []Dispositivo {
	rutas, _ := filepath.Glob("/dev/video*")
	sort.Strings(rutas)
	var out []Dispositivo
	for _, r := range rutas {
		fd, err := unix.Open(r, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		var c v4l2Capability
		err = ioctl(fd, vidiocQuerycap, unsafe.Pointer(&c))
		_ = unix.Close(fd)
		caps := c.Caps
		if caps&capDeviceCaps != 0 {
			caps = c.DeviceCaps
		}
		if err != nil || caps&capVideoCapture == 0 || caps&capStreaming == 0 {
			continue
		}
		out = append(out, Dispositivo{ID: r, Nombre: strings.TrimRight(string(c.Card[:]), "\x00")})
	}
	return out
}
