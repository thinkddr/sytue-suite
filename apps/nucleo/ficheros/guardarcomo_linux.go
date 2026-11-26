// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"errors"
	"os/exec"
	"strings"

	"github.com/godbus/dbus/v5"
)

// ordenGuardarComo es el selector de guardado de zenity o, si no, de kdialog. Los dos
// preguntan si se pisa un fichero que ya existe (la vieja `--confirm-overwrite` de
// zenity no se pasa: la 4 la quitó porque ya pregunta siempre).
func ordenGuardarComo(titulo, nombre string, extensiones []string) (string, []string, error) {
	patrones := make([]string, len(extensiones))
	for i, e := range extensiones {
		patrones[i] = "*." + e
	}
	if ruta, err := exec.LookPath("zenity"); err == nil {
		args := make([]string, 0, 4+len(patrones))
		args = append(args, "--file-selection", "--save", "--title="+titulo, "--filename="+nombre)
		for _, p := range patrones {
			args = append(args, "--file-filter="+titulo+" ("+p+") | "+p)
		}
		return ruta, args, nil
	}
	if ruta, err := exec.LookPath("kdialog"); err == nil {
		return ruta, []string{"--title", titulo, "--getsavefilename", nombre, strings.Join(patrones, " ")}, nil
	}
	return "", nil, ErrNoDisponible
}

func init() { porPortalGuardar = guardarPorPortal }

// guardarPorPortal es `FileChooser.SaveFile` del portal de XDG: el diálogo de guardar de
// GTK o de KDE sin zenity ni kdialog, y el único dentro de un Flatpak. Sin portal,
// ErrNoDisponible.
func guardarPorPortal(titulo, nombre string, extensiones []string) (string, error) {
	conn, senales, err := conectarPortal()
	if err != nil {
		return "", ErrNoDisponible
	}
	defer func() { _ = conn.Close() }()
	opciones := map[string]dbus.Variant{
		"modal":        dbus.MakeVariant(true),
		"current_name": dbus.MakeVariant(nombre),
	}
	if f := filtrosPortal(titulo, extensiones); f != nil {
		opciones["filters"] = dbus.MakeVariant(f)
		opciones["current_filter"] = dbus.MakeVariant(f[0])
	}
	portal := conn.Object(nombrePortal, "/org/freedesktop/portal/desktop")
	var pedido dbus.ObjectPath
	if err := portal.Call("org.freedesktop.portal.FileChooser.SaveFile", 0, "", titulo, opciones).Store(&pedido); err != nil {
		if errors.Is(sinPortalSi(err), errSinPortal) {
			return "", ErrNoDisponible
		}
		return "", err
	}
	codigo, resultado, err := respuesta(senales, pedido, 0)
	if err != nil || codigo != 0 {
		return "", err // 1 es «cancelado»: nada elegido y sin error
	}
	uris, _ := resultado["uris"].Value().([]string)
	if rutas := rutasDeURIs(uris); len(rutas) > 0 {
		return rutas[0], nil
	}
	return "", nil
}
