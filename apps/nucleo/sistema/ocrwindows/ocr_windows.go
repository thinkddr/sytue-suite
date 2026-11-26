// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ocrwindows

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema/com"
)

// Las funciones de WinRT (combase.dll, Windows 8 y posteriores).
var (
	combase = windows.NewLazySystemDLL("combase.dll")

	fRoGetActivationFactory = com.NuevaFuncion(combase, "RoGetActivationFactory")
	fWindowsCreateString    = com.NuevaFuncion(combase, "WindowsCreateString")
	fWindowsDeleteString    = com.NuevaFuncion(combase, "WindowsDeleteString")
	fWindowsGetStringRaw    = com.NuevaFuncion(combase, "WindowsGetStringRawBuffer")
)

// Las clases que se activan.
const (
	claseOcrEngine      = "Windows.Media.Ocr.OcrEngine"
	claseSoftwareBitmap = "Windows.Graphics.Imaging.SoftwareBitmap"
	claseBufferCripto   = "Windows.Security.Cryptography.CryptographicBuffer"
)

func guid(d1 uint32, d2, d3 uint16, d4 ...byte) windows.GUID {
	g := windows.GUID{Data1: d1, Data2: d2, Data3: d3}
	copy(g.Data4[:], d4)
	return g
}

// Los IID de las interfaces que se piden (de las cabeceras del SDK).
var (
	iidOcrEngineStatics      = guid(0x5bffa85a, 0x3384, 0x3540, 0x99, 0x40, 0x69, 0x91, 0x20, 0xd4, 0x28, 0xa8) // IOcrEngineStatics
	iidSoftwareBitmapStatics = guid(0xdf0385db, 0x672f, 0x4a9d, 0x80, 0x6e, 0xc2, 0x44, 0x2f, 0x34, 0x3e, 0x86) // ISoftwareBitmapStatics
	iidBufferCriptoStatics   = guid(0x320b7e22, 0x3cb0, 0x4cdf, 0x86, 0x63, 0x1d, 0x28, 0x91, 0x00, 0x65, 0xeb) // ICryptographicBufferStatics
	iidAsyncInfo             = guid(0x00000036, 0x0000, 0x0000, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46) // IAsyncInfo
)

// Los métodos, por su número en la tabla: los seis primeros son de IInspectable
// (QueryInterface, AddRef, Release, GetIids, GetRuntimeClassName, GetTrustLevel).
const (
	metodoQueryInterface = 0

	// IOcrEngineStatics
	estMaxImageDimension             = 6
	estAvailableRecognizerLanguages  = 7
	estTryCreateFromLanguage         = 9
	estTryCreateFromUserProfileLangs = 10

	// IOcrEngine
	motorRecognizeAsync = 6

	// IOcrResult, IOcrLine, IOcrWord
	resultadoLines = 6
	lineaWords     = 6
	palabraRect    = 6
	palabraText    = 7

	// IVectorView<T>
	vistaGetAt   = 6
	vistaGetSize = 7

	// ILanguage
	idiomaLanguageTag = 6

	// IAsyncOperation<T> e IAsyncInfo
	opGetResults    = 8
	infoGetStatus   = 7
	infoGetError    = 8
	infoCancel      = 9
	infoClose       = 10
	bufCreateFromBA = 9 // ICryptographicBufferStatics.CreateFromByteArray
	sbCreateCopy    = 9 // ISoftwareBitmapStatics.CreateCopyFromBuffer
)

// bitmapBgra8 es BitmapPixelFormat.Bgra8.
const bitmapBgra8 = 87

// AsyncStatus.
const (
	asyncEmpezada   = 0
	asyncHecha      = 1
	asyncCancelada  = 2
	asyncConFallo   = 3
	esperaSondeo    = 10 * time.Millisecond
	plazoReconocer  = 2 * time.Minute
	errNoEstaMotor  = "ocrwindows: no hay motor para ningún idioma de la persona"
	errSinResultado = "ocrwindows: el reconocimiento no devolvió nada"
)

// rect es Windows.Foundation.Rect.
type rect struct{ x, y, w, h float32 }

// ----- HSTRING -----

func crearHString(s string) (uintptr, error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var h uintptr
	r, err := fWindowsCreateString.Llamar(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	if err != nil {
		return 0, err
	}
	return h, com.HR(r, "WindowsCreateString")
}

func soltarHString(h uintptr) {
	if h != 0 {
		_, _ = fWindowsDeleteString.Llamar(h)
	}
}

// leerHString copia el texto de un HSTRING (que sigue siendo de quien llama).
func leerHString(h uintptr) string {
	if h == 0 {
		return ""
	}
	var n uint32
	p, err := fWindowsGetStringRaw.Llamar(h, uintptr(unsafe.Pointer(&n)))
	if err != nil || p == 0 || n == 0 {
		return ""
	}
	// El puntero que devuelve la función, leído como *uint16 sin convertir un uintptr
	// suelto (que `go vet` no admite): el HSTRING lo mantiene vivo mientras no se suelte.
	ptr := *(**uint16)(unsafe.Pointer(&p))
	return windows.UTF16ToString(unsafe.Slice(ptr, n))
}

// ----- Activación -----

// fabrica activa la clase y pide su interfaz de estáticos.
func fabrica(clase string, iid *windows.GUID) (*com.Objeto, error) {
	h, err := crearHString(clase)
	if err != nil {
		return nil, err
	}
	defer soltarHString(h)
	var f *com.Objeto
	r, err := fRoGetActivationFactory.Llamar(h, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f)))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoDisponible, err)
	}
	if err := com.HR(r, "RoGetActivationFactory "+clase); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoDisponible, err)
	}
	if f == nil {
		return nil, ErrNoDisponible
	}
	return f, nil
}

// enHilo hace f en un hilo propio con COM (y con él WinRT) iniciado en multihilo, y lo
// cierra después.
func enHilo(f func() error) error {
	if !fRoGetActivationFactory.Esta() {
		return ErrNoDisponible
	}
	h, err := com.NuevoHilo(com.Multihilo)
	if err != nil {
		return err
	}
	defer h.Cerrar()
	return h.Hacer(f)
}

// ----- Vistas (IVectorView) -----

func tamano(v *com.Objeto) (uint32, error) {
	var n uint32
	return n, com.HR(v.Llamar(vistaGetSize, uintptr(unsafe.Pointer(&n))), "IVectorView.Size")
}

func elemento(v *com.Objeto, i uint32) (*com.Objeto, error) {
	var o *com.Objeto
	if err := com.HR(v.Llamar(vistaGetAt, uintptr(i), uintptr(unsafe.Pointer(&o))), "IVectorView.GetAt"); err != nil {
		return nil, err
	}
	if o == nil {
		return nil, errors.New("ocrwindows: elemento vacío")
	}
	return o, nil
}

// idiomasDe recorre los idiomas instalados (IVectorView<Language>) con su etiqueta; ve
// puede quedarse el idioma (devuelve true) y entonces no se suelta: es de quien lo pidió.
func idiomasDe(est *com.Objeto, ve func(etiqueta string, idioma *com.Objeto) bool) error {
	var vista *com.Objeto
	if err := com.HR(est.Llamar(estAvailableRecognizerLanguages, uintptr(unsafe.Pointer(&vista))), "AvailableRecognizerLanguages"); err != nil {
		return err
	}
	if vista == nil {
		return nil
	}
	defer vista.Soltar()
	n, err := tamano(vista)
	if err != nil {
		return err
	}
	for i := range n {
		idioma, err := elemento(vista, i)
		if err != nil {
			return err
		}
		var h uintptr
		if err := com.HR(idioma.Llamar(idiomaLanguageTag, uintptr(unsafe.Pointer(&h))), "LanguageTag"); err != nil {
			idioma.Soltar()
			return err
		}
		etiqueta := leerHString(h)
		soltarHString(h)
		if !ve(etiqueta, idioma) {
			idioma.Soltar()
		}
	}
	return nil
}

// ----- Consultar -----

// consultar pregunta al sistema qué idiomas sabe reconocer y su lado máximo.
func consultar() (consulta, error) {
	var c consulta
	err := enHilo(func() error {
		est, err := fabrica(claseOcrEngine, &iidOcrEngineStatics)
		if err != nil {
			return err
		}
		defer est.Soltar()
		var lado uint32
		if com.HR(est.Llamar(estMaxImageDimension, uintptr(unsafe.Pointer(&lado))), "") == nil && lado > 0 {
			c.maxLado = int(lado)
		}
		return idiomasDe(est, func(e string, _ *com.Objeto) bool {
			if e != "" {
				c.etiquetas = append(c.etiquetas, e)
			}
			return false
		})
	})
	return c, err
}

// ----- Reconocer -----

// reconocer pasa la imagen BGRA8 al motor del idioma etiqueta ("" o uno que no esté: el
// de los idiomas de la persona) y espera sus palabras.
func reconocer(ctx context.Context, bgra []byte, ancho, alto int, etiqueta string) ([]palabraCruda, error) {
	if len(bgra) != ancho*alto*4 || ancho <= 0 || alto <= 0 {
		return nil, errors.New("ocrwindows: imagen mal formada")
	}
	ctx, cancelar := context.WithTimeout(ctx, plazoReconocer)
	defer cancelar()
	var out []palabraCruda
	err := enHilo(func() error {
		motor, err := crearMotor(etiqueta)
		if err != nil {
			return err
		}
		defer motor.Soltar()
		bitmap, err := crearBitmap(bgra, ancho, alto)
		if err != nil {
			return err
		}
		defer bitmap.Soltar()
		var op *com.Objeto
		if err := com.HR(motor.Llamar(motorRecognizeAsync, uintptr(unsafe.Pointer(bitmap)), uintptr(unsafe.Pointer(&op))), "RecognizeAsync"); err != nil {
			return err
		}
		if op == nil {
			return errors.New(errSinResultado)
		}
		defer op.Soltar()
		if err := esperar(ctx, op); err != nil {
			return err
		}
		var resultado *com.Objeto
		if err := com.HR(op.Llamar(opGetResults, uintptr(unsafe.Pointer(&resultado))), "GetResults"); err != nil {
			return err
		}
		if resultado == nil {
			return errors.New(errSinResultado)
		}
		defer resultado.Soltar()
		out, err = palabras(resultado)
		return err
	})
	return out, err
}

// crearMotor es el OcrEngine del idioma de etiqueta o, sin él, el de los idiomas de la
// persona (TryCreate da nulo, sin error, si no puede).
func crearMotor(etiqueta string) (*com.Objeto, error) {
	est, err := fabrica(claseOcrEngine, &iidOcrEngineStatics)
	if err != nil {
		return nil, err
	}
	defer est.Soltar()
	var motor *com.Objeto
	if etiqueta != "" {
		var idioma *com.Objeto
		_ = idiomasDe(est, func(e string, i *com.Objeto) bool {
			if idioma == nil && e == etiqueta {
				idioma = i
				return true
			}
			return false
		})
		if idioma != nil {
			_ = com.HR(est.Llamar(estTryCreateFromLanguage, uintptr(unsafe.Pointer(idioma)), uintptr(unsafe.Pointer(&motor))), "TryCreateFromLanguage")
			idioma.Soltar()
		}
	}
	if motor == nil {
		if err := com.HR(est.Llamar(estTryCreateFromUserProfileLangs, uintptr(unsafe.Pointer(&motor))), "TryCreateFromUserProfileLanguages"); err != nil {
			return nil, err
		}
	}
	if motor == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoDisponible, errNoEstaMotor)
	}
	return motor, nil
}

// crearBitmap copia la imagen a un SoftwareBitmap BGRA8 por un IBuffer.
func crearBitmap(bgra []byte, ancho, alto int) (*com.Objeto, error) {
	cripto, err := fabrica(claseBufferCripto, &iidBufferCriptoStatics)
	if err != nil {
		return nil, err
	}
	defer cripto.Soltar()
	var buf *com.Objeto
	if err := com.HR(cripto.Llamar(bufCreateFromBA, uintptr(len(bgra)), uintptr(unsafe.Pointer(&bgra[0])), uintptr(unsafe.Pointer(&buf))), "CreateFromByteArray"); err != nil {
		return nil, err
	}
	if buf == nil {
		return nil, errors.New("ocrwindows: sin búfer")
	}
	defer buf.Soltar()
	est, err := fabrica(claseSoftwareBitmap, &iidSoftwareBitmapStatics)
	if err != nil {
		return nil, err
	}
	defer est.Soltar()
	var bitmap *com.Objeto
	if err := com.HR(est.Llamar(sbCreateCopy, uintptr(unsafe.Pointer(buf)), bitmapBgra8, uintptr(ancho), uintptr(alto), uintptr(unsafe.Pointer(&bitmap))), "CreateCopyFromBuffer"); err != nil {
		return nil, err
	}
	if bitmap == nil {
		return nil, errors.New("ocrwindows: sin imagen")
	}
	return bitmap, nil
}

// esperar pregunta por el estado de la operación hasta que acaba; si ctx se acaba antes,
// la cancela. Siempre la cierra.
func esperar(ctx context.Context, op *com.Objeto) error {
	var info *com.Objeto
	if err := com.HR(op.Llamar(metodoQueryInterface, uintptr(unsafe.Pointer(&iidAsyncInfo)), uintptr(unsafe.Pointer(&info))), "IAsyncInfo"); err != nil {
		return err
	}
	if info == nil {
		return errors.New("ocrwindows: la operación no dice su estado")
	}
	defer info.Soltar()
	defer info.Llamar(infoClose)
	for {
		var estado int32
		if err := com.HR(info.Llamar(infoGetStatus, uintptr(unsafe.Pointer(&estado))), "AsyncInfo.Status"); err != nil {
			return err
		}
		switch estado {
		case asyncHecha:
			return nil
		case asyncCancelada:
			if err := ctx.Err(); err != nil {
				return err
			}
			return context.Canceled
		case asyncConFallo:
			var hr int32
			_ = info.Llamar(infoGetError, uintptr(unsafe.Pointer(&hr)))
			return &com.Error{Que: "RecognizeAsync", HR: uint32(hr)} //nolint:gosec // G115: un HRESULT son 32 bits
		case asyncEmpezada:
		default:
			return fmt.Errorf("ocrwindows: estado desconocido %d", estado)
		}
		select {
		case <-ctx.Done():
			info.Llamar(infoCancel)
			return ctx.Err()
		case <-time.After(esperaSondeo):
		}
	}
}

// palabras recorre las líneas del resultado y sus palabras.
func palabras(resultado *com.Objeto) ([]palabraCruda, error) {
	var lineas *com.Objeto
	if err := com.HR(resultado.Llamar(resultadoLines, uintptr(unsafe.Pointer(&lineas))), "OcrResult.Lines"); err != nil {
		return nil, err
	}
	if lineas == nil {
		return nil, nil
	}
	defer lineas.Soltar()
	nl, err := tamano(lineas)
	if err != nil {
		return nil, err
	}
	var out []palabraCruda
	for i := range nl {
		linea, err := elemento(lineas, i)
		if err != nil {
			return nil, err
		}
		ps, err := palabrasDeLinea(linea)
		linea.Soltar()
		if err != nil {
			return nil, err
		}
		out = append(out, ps...)
	}
	return out, nil
}

func palabrasDeLinea(linea *com.Objeto) ([]palabraCruda, error) {
	var vista *com.Objeto
	if err := com.HR(linea.Llamar(lineaWords, uintptr(unsafe.Pointer(&vista))), "OcrLine.Words"); err != nil {
		return nil, err
	}
	if vista == nil {
		return nil, nil
	}
	defer vista.Soltar()
	n, err := tamano(vista)
	if err != nil {
		return nil, err
	}
	out := make([]palabraCruda, 0, n)
	for i := range n {
		w, err := elemento(vista, i)
		if err != nil {
			return nil, err
		}
		var r rect
		var h uintptr
		err = com.HR(w.Llamar(palabraRect, uintptr(unsafe.Pointer(&r))), "OcrWord.BoundingRect")
		if err == nil {
			err = com.HR(w.Llamar(palabraText, uintptr(unsafe.Pointer(&h))), "OcrWord.Text")
		}
		w.Soltar()
		if err != nil {
			return nil, err
		}
		out = append(out, palabraCruda{texto: leerHString(h), x: r.x, y: r.y, w: r.w, h: r.h})
		soltarHString(h)
	}
	return out, nil
}
