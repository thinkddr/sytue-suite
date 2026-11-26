// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package videoteca

import (
	"context"
	"errors"
	"fmt"
	"image"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema/com"
)

// Lo de Windows para la miniatura del Explorador (ver miniatura_explorador.go). Todo de
// System32 y buscado al usarlo: si algo falta, no hay miniatura (nunca un pánico).
var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	gdi32   = windows.NewLazySystemDLL("gdi32.dll")
	user32  = windows.NewLazySystemDLL("user32.dll")

	fSHCreateItemFromParsingName = com.NuevaFuncion(shell32, "SHCreateItemFromParsingName")
	fGetObject                   = com.NuevaFuncion(gdi32, "GetObjectW")
	fGetDIBits                   = com.NuevaFuncion(gdi32, "GetDIBits")
	fDeleteObject                = com.NuevaFuncion(gdi32, "DeleteObject")
	fGetDC                       = com.NuevaFuncion(user32, "GetDC")
	fReleaseDC                   = com.NuevaFuncion(user32, "ReleaseDC")
)

// IID_IShellItemImageFactory (shobjidl_core.h) y su único método, GetImage, detrás de
// los tres de IUnknown.
var iidImageFactory = windows.GUID{
	Data1: 0xbcc18b79, Data2: 0xba16, Data3: 0x442f,
	Data4: [8]byte{0x80, 0xc4, 0x8a, 0x59, 0xc3, 0x0c, 0x46, 0x3b},
}

const (
	metodoGetImage      = 3
	siigbfThumbnailOnly = 0x8 // la miniatura o nada (no el icono del tipo)
	biRGB               = 0
	dibRGBColors        = 0
)

// mapaBits es BITMAP (wingdi.h).
type mapaBits struct {
	tipo, ancho, alto, bytesFila int32
	planos, bitsPixel            uint16
	bits                         uintptr
}

// cabeceraMapa es BITMAPINFOHEADER con sitio de sobra detrás para la tabla de colores
// de BITMAPINFO (con 32 bits por píxel y BI_RGB no se escribe).
type cabeceraMapa struct {
	tamano                uint32
	ancho, alto           int32
	planos, bits          uint16
	compresion, tamImagen uint32
	xPorMetro, yPorMetro  int32
	usados, importantes   uint32
	_                     [4]uint32
}

// miniaturaExplorador pide al Explorador la miniatura del vídeo, en PNG. Va en un hilo
// propio (COM en STA, el que prefiere el Shell) y con tiempo tasado: si el
// miniaturizador del sistema se cuelga, aquí no se espera más (el hilo acaba solo cuando
// vuelva).
func miniaturaExplorador(ctx context.Context, abs string) ([]byte, error) {
	if !fSHCreateItemFromParsingName.Esta() {
		return nil, ErrSinMiniatura
	}
	ruta, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return nil, ErrSinMiniatura
	}
	hilo, err := com.NuevoHilo(com.Unhilo)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSinMiniatura, err)
	}
	type resultado struct {
		img *image.RGBA
		err error
	}
	hecho := make(chan resultado, 1)
	go func() {
		defer hilo.Cerrar()
		var img *image.RGBA
		err := hilo.Hacer(func() error {
			var err error
			img, err = imagenDelExplorador(ruta)
			return err
		})
		hecho <- resultado{img, err}
	}()
	c, fin := context.WithTimeout(ctx, 30*time.Second)
	defer fin()
	select {
	case <-c.Done():
		return nil, ErrSinMiniatura
	case r := <-hecho:
		if r.err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSinMiniatura, r.err)
		}
		return aPNG(r.img)
	}
}

// imagenDelExplorador, en el hilo COM: el IShellItemImageFactory del fichero, su
// GetImage y los píxeles del HBITMAP que da.
func imagenDelExplorador(ruta *uint16) (*image.RGBA, error) {
	var f *com.Objeto
	r, err := fSHCreateItemFromParsingName.Llamar(uintptr(unsafe.Pointer(ruta)), 0,
		uintptr(unsafe.Pointer(&iidImageFactory)), uintptr(unsafe.Pointer(&f)))
	if err == nil {
		err = com.HR(r, "SHCreateItemFromParsingName")
	}
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, errors.New("videoteca: el Explorador no dio el objeto de la miniatura")
	}
	defer f.Soltar()
	// GetImage recibe un SIZE por valor: en 64 bits (amd64 y arm64) van los dos LONG en
	// un registro; en 32, uno detrás de otro en la pila.
	lado := uint64(TamanoMiniatura)
	var mapa uintptr // HBITMAP
	if unsafe.Sizeof(uintptr(0)) == 8 {
		r = f.Llamar(metodoGetImage, uintptr(lado|lado<<32), siigbfThumbnailOnly, uintptr(unsafe.Pointer(&mapa)))
	} else {
		r = f.Llamar(metodoGetImage, uintptr(lado), uintptr(lado), siigbfThumbnailOnly, uintptr(unsafe.Pointer(&mapa)))
	}
	if err := com.HR(r, "GetImage"); err != nil {
		return nil, err
	}
	if mapa == 0 {
		return nil, errors.New("videoteca: el Explorador no dio miniatura")
	}
	defer func() { _, _ = fDeleteObject.Llamar(mapa) }()
	return pixelesDe(mapa)
}

// pixelesDe saca los píxeles de un HBITMAP a 32 bits por píxel, de arriba abajo.
func pixelesDe(mapa uintptr) (*image.RGBA, error) {
	var bm mapaBits
	if n, err := fGetObject.Llamar(mapa, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); err != nil || n == 0 {
		return nil, errors.New("videoteca: GetObject no describe la miniatura")
	}
	ancho, alto := int(bm.ancho), int(bm.alto)
	if alto < 0 {
		alto = -alto
	}
	if ancho <= 0 || alto <= 0 || ancho > maxLadoExplorador || alto > maxLadoExplorador {
		return nil, errPixeles
	}
	cab := cabeceraMapa{tamano: 40, ancho: int32(ancho), alto: -int32(alto), planos: 1, bits: 32, compresion: biRGB} //nolint:gosec // G115: lados acotados arriba
	px := make([]byte, ancho*alto*4)
	dc, err := fGetDC.Llamar(0)
	if err != nil || dc == 0 {
		return nil, errors.New("videoteca: sin contexto de pantalla para leer la miniatura")
	}
	lineas, err := fGetDIBits.Llamar(dc, mapa, 0, uintptr(alto), uintptr(unsafe.Pointer(&px[0])), uintptr(unsafe.Pointer(&cab)), dibRGBColors)
	_, _ = fReleaseDC.Llamar(0, dc)
	if err != nil || lineas == 0 {
		return nil, errors.New("videoteca: GetDIBits no dio los píxeles de la miniatura")
	}
	return deBGRA(px, ancho, alto)
}
