// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32Graf = windows.NewLazySystemDLL("user32.dll")
	gdi32Graf  = windows.NewLazySystemDLL("gdi32.dll")
	opengl32   = syscall.NewLazyDLL("opengl32.dll") // la que cargue la app, como hará GLFW
	pRegClase  = user32Graf.NewProc("RegisterClassExW")
	pCrearVent = user32Graf.NewProc("CreateWindowExW")
	pDestVent  = user32Graf.NewProc("DestroyWindow")
	pDefVent   = user32Graf.NewProc("DefWindowProcW")
	pGetDC     = user32Graf.NewProc("GetDC")
	pReleaseDC = user32Graf.NewProc("ReleaseDC")
	pMensaje   = user32Graf.NewProc("MessageBoxW")
	pElegirPF  = gdi32Graf.NewProc("ChoosePixelFormat")
	pPonerPF   = gdi32Graf.NewProc("SetPixelFormat")
	pCrearCtx  = opengl32.NewProc("wglCreateContext")
	pActualCtx = opengl32.NewProc("wglMakeCurrent")
	pBorrarCtx = opengl32.NewProc("wglDeleteContext")
	pGetString = opengl32.NewProc("glGetString")
)

// ponerDirectorioDLL: lo que se cargue por nombre sale de ahí antes que de System32.
var ponerDirectorioDLL = windows.SetDllDirectory

// WNDCLASSEXW.
type claseVentana struct {
	tamano, estilo       uint32
	proc                 uintptr
	extraClase, extraVen int32
	instancia            uintptr
	icono, cursor, fondo uintptr
	menu, nombre         *uint16
	iconoPeq             uintptr
}

// PIXELFORMATDESCRIPTOR.
type formatoPixel struct {
	tamano, version                                    uint16
	banderas                                           uint32
	tipo, bitsColor                                    byte
	_                                                  [8]byte
	bitsAcum, rAcum, gAcum, bAcum, aAcum, bitsZ, bitsS byte
	auxiliares, capa, reservado                        byte
	_                                                  [12]byte
}

func graficos() error {
	if opengl32.Load() != nil {
		return &SinGraficos{"opengl32.dll"}
	}
	nombre, _ := windows.UTF16PtrFromString("SytueGraficos")
	c := claseVentana{proc: pDefVent.Addr(), nombre: nombre}
	c.tamano = uint32(unsafe.Sizeof(c))
	_, _, _ = pRegClase.Call(uintptr(unsafe.Pointer(&c)))
	v, _, _ := pCrearVent.Call(0, uintptr(unsafe.Pointer(nombre)), 0, 0, 0, 0, 1, 1, 0, 0, 0, 0)
	if v == 0 {
		return nil // sin ventana no se puede preguntar; que lo intente Fyne
	}
	defer func() { _, _, _ = pDestVent.Call(v) }()
	dc, _, _ := pGetDC.Call(v)
	defer func() { _, _, _ = pReleaseDC.Call(v, dc) }()
	pf := formatoPixel{version: 1, banderas: 0x4 | 0x20 | 0x1, bitsColor: 32, bitsZ: 24} // DRAW_TO_WINDOW | SUPPORT_OPENGL | DOUBLEBUFFER
	pf.tamano = uint16(unsafe.Sizeof(pf))
	n, _, _ := pElegirPF.Call(dc, uintptr(unsafe.Pointer(&pf)))
	if n == 0 {
		return &SinGraficos{"ChoosePixelFormat"}
	}
	_, _, _ = pPonerPF.Call(dc, n, uintptr(unsafe.Pointer(&pf)))
	ctx, _, _ := pCrearCtx.Call(dc)
	if ctx == 0 {
		return &SinGraficos{"wglCreateContext"}
	}
	defer func() {
		_, _, _ = pActualCtx.Call(0, 0)
		_, _, _ = pBorrarCtx.Call(ctx)
	}()
	_, _, _ = pActualCtx.Call(dc, ctx)
	version := cadenaGL(0x1F02)  // GL_VERSION
	renderer := cadenaGL(0x1F01) // GL_RENDERER
	if !versionSuficiente(version) {
		return &SinGraficos{"OpenGL " + version + ", " + renderer}
	}
	return nil
}

// cadenaGL lee un glGetString.
func cadenaGL(nombre uintptr) string {
	p, _, _ := pGetString.Call(nombre)
	if p == 0 {
		return ""
	}
	// Así y no con (*byte)(unsafe.Pointer(p)), que vet toma por un uintptr reciclado:
	// p es un puntero de C que OpenGL devuelve y no mueve.
	return windows.BytePtrToString(*(**byte)(unsafe.Pointer(&p)))
}

// AvisarSinGraficos enseña el aviso del sistema, que no necesita OpenGL.
func AvisarSinGraficos(titulo, texto string) {
	t, _ := windows.UTF16PtrFromString(titulo)
	m, _ := windows.UTF16PtrFromString(texto)
	_, _, _ = pMensaje.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10) // MB_ICONERROR
}
