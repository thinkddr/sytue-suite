// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

// El portal GlobalShortcuts (xdg-desktop-portal 1.17+: Plasma 6, GNOME 48, Hyprland).
// Tres pasos, cada uno con su respuesta por org.freedesktop.portal.Request.Response:
// registrar la app (Registry, para que el portal sepa de quién es el atajo), abrir una
// sesión y atar el atajo. Después, `Activated` cada vez que se pulsa.
//
// La sesión vive lo que viva LA CONEXIÓN: por eso va en una propia, que se cierra al
// soltar el atajo, y no en la compartida del proceso.

const (
	portalNombre = "org.freedesktop.portal.Desktop"
	portalRuta   = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	atajosIface  = "org.freedesktop.portal.GlobalShortcuts"
	atajoID      = "buscar"
)

var fichas atomic.Uint64

func atajo(id, descripcion string, c Combinacion, alPulsar func()) (func(), error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, ErrNoDisponible
	}
	soltar := func() { _ = conn.Close() }
	if err := atarAtajo(conn, id, descripcion, c, alPulsar); err != nil {
		soltar()
		return nil, err
	}
	return soltar, nil
}

func atarAtajo(conn *dbus.Conn, id, descripcion string, c Combinacion, alPulsar func()) error {
	portal := conn.Object(portalNombre, portalRuta)
	// Sin Registry (portales anteriores al 1.19.4) el portal adivina la app por el
	// proceso: no es un error.
	registrarEnPortal(conn, id)

	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		return err
	}
	if err := conn.AddMatchSignal(dbus.WithMatchInterface(atajosIface), dbus.WithMatchMember("Activated")); err != nil {
		return err
	}
	senales := make(chan *dbus.Signal, 16)
	conn.Signal(senales)

	var pedido dbus.ObjectPath
	err := portal.Call(atajosIface+".CreateSession", 0, map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(ficha()),
		"session_handle_token": dbus.MakeVariant(ficha()),
	}).Store(&pedido)
	if err != nil {
		return sinPortal(err)
	}
	resultado, err := esperarRespuesta(senales, pedido)
	if err != nil {
		return err
	}
	sesion := sesionDe(resultado["session_handle"])
	if sesion == "" {
		return errors.New("escritorio: el portal no dio sesión para el atajo")
	}

	atajos := []struct {
		ID       string
		Opciones map[string]dbus.Variant
	}{{atajoID, map[string]dbus.Variant{
		"description":       dbus.MakeVariant(descripcion),
		"preferred_trigger": dbus.MakeVariant(c.Portal()),
	}}}
	err = portal.Call(atajosIface+".BindShortcuts", 0, sesion, atajos, "",
		map[string]dbus.Variant{"handle_token": dbus.MakeVariant(ficha())}).Store(&pedido)
	if err != nil {
		return sinPortal(err)
	}
	if _, err := esperarRespuesta(senales, pedido); err != nil {
		return err
	}

	go func() {
		for s := range senales {
			if s != nil && s.Name == atajosIface+".Activated" && len(s.Body) >= 2 {
				if quien, _ := s.Body[1].(string); quien == atajoID {
					alPulsar()
				}
			}
		}
	}()
	return nil
}

// sesionDe acepta la sesión como la mandan los portales: `o` en los nuevos, `s` en los
// que siguieron la primera versión de la especificación.
func sesionDe(v dbus.Variant) dbus.ObjectPath {
	switch x := v.Value().(type) {
	case dbus.ObjectPath:
		return x
	case string:
		return dbus.ObjectPath(x)
	}
	return ""
}

// esperarRespuesta espera la Response de un pedido. Un pedido que el portal no contesta
// en un minuto no va a contestarse.
func esperarRespuesta(senales <-chan *dbus.Signal, pedido dbus.ObjectPath) (map[string]dbus.Variant, error) {
	plazo := time.After(time.Minute)
	for {
		select {
		case s := <-senales:
			if s == nil || s.Path != pedido || len(s.Body) < 2 {
				continue
			}
			codigo, _ := s.Body[0].(uint32)
			if codigo != 0 {
				return nil, errors.New("escritorio: el portal no ató el atajo (código " + strconv.Itoa(int(codigo)) + ")")
			}
			resultado, _ := s.Body[1].(map[string]dbus.Variant)
			return resultado, nil
		case <-plazo:
			return nil, errors.New("escritorio: el portal no contestó")
		}
	}
}

// sinPortal: sin portal, o sin GlobalShortcuts en él, es «este escritorio no lo tiene».
func sinPortal(err error) error {
	var e dbus.Error
	if errors.As(err, &e) && strings.HasPrefix(e.Name, "org.freedesktop.DBus.Error.") {
		return ErrNoDisponible
	}
	return err
}

func ficha() string { return "sytue" + strconv.FormatUint(fichas.Add(1), 10) }
