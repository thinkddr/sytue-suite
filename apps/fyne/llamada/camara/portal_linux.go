// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package camara

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

// El portal de escritorio (xdg-desktop-portal, ScreenCast): en Wayland ninguna app ve la
// pantalla de las demás, y la única puerta es pedírsela al compositor por D-Bus. El
// compositor enseña SU diálogo (qué monitor o ventana), y a cambio da un nodo de
// PipeWire y el descriptor para conectarse a él. Cada paso contesta por una señal
// Response en un objeto Request propio, no por el valor de retorno.
//
// https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.ScreenCast.html
const (
	portalNombre  = "org.freedesktop.portal.Desktop"
	portalRuta    = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalCaptura = "org.freedesktop.portal.ScreenCast"
)

// EsperaPortal es lo que se espera a que la persona conteste al diálogo del compositor.
var EsperaPortal = 2 * time.Minute

var fichas atomic.Uint64

// sesionPortal es la sesión de ScreenCast ya concedida: el nodo que mirar y el
// descriptor de PipeWire por el que se ve. Cerrarla es lo que apaga el aviso del
// compositor de «se está compartiendo».
type sesionPortal struct {
	con    *dbus.Conn
	sesion dbus.ObjectPath
	nodo   uint32
	fd     int
}

// pedirPortal recorre CreateSession → SelectSources → Start → OpenPipeWireRemote. Si la
// persona cancela el diálogo, ErrSinPermiso; si no hay portal, ErrSinPantalla.
func pedirPortal(con *dbus.Conn) (*sesionPortal, error) {
	p := con.Object(portalNombre, portalRuta)
	// La versión 4 trae persist_mode; aquí basta la 1, que es de 2016: todo compositor
	// con portal la tiene. Solo se mira que el interfaz exista.
	if _, err := p.GetProperty(portalCaptura + ".AvailableSourceTypes"); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSinPantalla, err)
	}
	r, err := pedirAlPortal(con, p, "CreateSession", map[string]dbus.Variant{
		"session_handle_token": dbus.MakeVariant(ficha()),
	})
	if err != nil {
		return nil, err
	}
	var sesion string
	if v, ok := r["session_handle"]; !ok || v.Store(&sesion) != nil {
		return nil, fmt.Errorf("camara: el portal no dio sesión: %v", r)
	}
	s := &sesionPortal{con: con, sesion: dbus.ObjectPath(sesion), fd: -1}
	if _, err := pedirAlPortal(con, p, "SelectSources", map[string]dbus.Variant{
		"types":       dbus.MakeVariant(uint32(1)), // un monitor, como las demás plataformas
		"multiple":    dbus.MakeVariant(false),
		"cursor_mode": dbus.MakeVariant(uint32(2)), // el puntero dentro de la imagen
	}, s.sesion); err != nil {
		s.Cerrar()
		return nil, err
	}
	r, err = pedirAlPortal(con, p, "Start", nil, s.sesion, "")
	if err != nil {
		s.Cerrar()
		return nil, err
	}
	var flujos []struct {
		Nodo  uint32
		Props map[string]dbus.Variant
	}
	if v, ok := r["streams"]; !ok || v.Store(&flujos) != nil || len(flujos) == 0 {
		s.Cerrar()
		return nil, fmt.Errorf("camara: el portal no dio ningún flujo: %v", r)
	}
	s.nodo = flujos[0].Nodo
	var fd dbus.UnixFD
	if err := p.Call(portalCaptura+".OpenPipeWireRemote", 0, s.sesion, map[string]dbus.Variant{}).Store(&fd); err != nil {
		s.Cerrar()
		return nil, err
	}
	s.fd = int(fd)
	return s, nil
}

// pedirAlPortal llama a un método que contesta por Request.Response y espera esa señal.
// Los argumentos van delante de las opciones, que es como los declaran todos.
func pedirAlPortal(con *dbus.Conn, p dbus.BusObject, metodo string, opts map[string]dbus.Variant, args ...any) (map[string]dbus.Variant, error) {
	if opts == nil {
		opts = map[string]dbus.Variant{}
	}
	tok := ficha()
	opts["handle_token"] = dbus.MakeVariant(tok)
	// La ruta del Request se sabe ANTES de llamar: suscribirse después perdería una
	// respuesta rápida. Es /…/request/<remitente sin «:» y con «.»→«_»>/<ficha>.
	remitente := strings.ReplaceAll(strings.TrimPrefix(con.Names()[0], ":"), ".", "_")
	ruta := dbus.ObjectPath(fmt.Sprintf("%s/request/%s/%s", portalRuta, remitente, tok))
	regla := []dbus.MatchOption{
		dbus.WithMatchObjectPath(ruta),
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	}
	if err := con.AddMatchSignal(regla...); err != nil {
		return nil, err
	}
	defer func() { _ = con.RemoveMatchSignal(regla...) }()
	ch := make(chan *dbus.Signal, 4)
	con.Signal(ch)
	defer con.RemoveSignal(ch)

	var devuelta dbus.ObjectPath
	if err := p.Call(portalCaptura+"."+metodo, 0, append(args, opts)...).Store(&devuelta); err != nil {
		return nil, err
	}
	fin := time.After(EsperaPortal)
	for {
		select {
		case s := <-ch:
			// Un portal anterior a 0.9 devuelve otra ruta: vale la que diga él.
			if s.Path != ruta && s.Path != devuelta || len(s.Body) < 2 {
				continue
			}
			codigo, _ := s.Body[0].(uint32)
			res, _ := s.Body[1].(map[string]dbus.Variant)
			switch codigo {
			case 0:
				return res, nil
			case 1:
				return nil, ErrSinPermiso // la persona canceló el diálogo
			default:
				return nil, fmt.Errorf("camara: el portal contestó %d a %s", codigo, metodo)
			}
		case <-fin:
			return nil, ErrSinPermiso
		}
	}
}

// Cerrar suelta la sesión del compositor y el descriptor. Se puede llamar más de una vez.
func (s *sesionPortal) Cerrar() {
	if s.sesion != "" {
		_ = s.con.Object(portalNombre, s.sesion).Call("org.freedesktop.portal.Session.Close", 0).Err
		s.sesion = ""
	}
	if s.fd >= 0 {
		_ = os.NewFile(uintptr(s.fd), "pipewire").Close()
		s.fd = -1
	}
	_ = s.con.Close()
}

func ficha() string { return fmt.Sprintf("sytue%d_%d", os.Getpid(), fichas.Add(1)) }

// esWayland: con WAYLAND_DISPLAY la pantalla de X11 que se ve es la de XWayland, que solo
// tiene las ventanas X11: compartirla enseñaría un escritorio negro.
func esWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
}

var errSinPipeWire = errors.New("camara: no está libpipewire-0.3, que el portal necesita")

// abrirPantallaPortal pide la pantalla al compositor y la lee por PipeWire. Sin bus de
// sesión o sin portal, ErrSinPantalla: el botón se queda y dice por qué.
func abrirPantallaPortal() (Fuente, error) {
	con, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSinPantalla, err)
	}
	s, err := pedirPortal(con)
	if err != nil {
		_ = con.Close()
		return nil, err
	}
	f, err := abrirPipeWire(s.fd, s.nodo)
	if err != nil {
		s.Cerrar()
		return nil, fmt.Errorf("%w: %w", ErrSinPantalla, err)
	}
	f.sesion = s
	return f, nil
}
