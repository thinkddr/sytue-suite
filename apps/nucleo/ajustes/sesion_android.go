// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package ajustes

import (
	"encoding/json"
	"errors"
	"os"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func estadoPlataforma(e Estado) Estado {
	habilitada, err := sistema.SesionAndroidHabilitada()
	if err == nil && !habilitada {
		return e
	}
	var estado struct {
		Generacion       string `json:"generacion"`
		Bloqueo          bool   `json:"bloqueo"`
		DriveDesactivado bool   `json:"drive_sistema_desactivado"`
	}
	texto, errEstado := sistema.EstadoSesionAndroid()
	if err != nil || errEstado != nil || json.Unmarshal([]byte(texto), &estado) != nil {
		e.Sesion, e.Bloqueo, e.DriveSistemaDesactivado = "", true, true
		return e
	}
	e.Sesion, e.Bloqueo, e.DriveSistemaDesactivado = estado.Generacion, estado.Bloqueo, estado.DriveDesactivado
	return e
}

func abrirCerrojoSesion() (*os.File, bool, error) {
	habilitada, err := sistema.SesionAndroidHabilitada()
	if err != nil || !habilitada {
		return nil, habilitada, err
	}
	fd, err := sistema.CerrojoSesionAndroid()
	if err != nil {
		return nil, true, err
	}
	if fd < 0 {
		return nil, true, errors.New("ajustes: Android no ha dado el cerrojo de sesión")
	}
	return os.NewFile(uintptr(fd), "sytue-session-lock"), true, nil
}
