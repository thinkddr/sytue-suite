// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// instalarPerfilApple deja el perfil en un fichero 600 propio y se lo da al sistema:
// abrir un .mobileconfig lleva a macOS a Ajustes del Sistema con el perfil listo, donde
// solo falta pulsar «Instalar» (ese clic lo exige el sistema, y no debe poder saltarse).
// El fichero lleva la contraseña de aplicación dentro, así que se borra en cuanto macOS
// lo ha leído.
func instalarPerfilApple(perfil []byte) error {
	dir, err := os.MkdirTemp("", "sytue-perfil-")
	if err != nil {
		return err
	}
	ruta := filepath.Join(dir, "Sytue.mobileconfig")
	if err := os.WriteFile(ruta, perfil, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	// ponytail: dos minutos sobran para que macOS lo copie; si la app se cierra antes, el
	// fichero queda en la carpeta temporal del usuario, que solo lee él.
	time.AfterFunc(2*time.Minute, func() { _ = os.RemoveAll(dir) })
	return ficheros.Abrir(ruta)
}

// abrirEnSafari abre el enlace de un solo uso del perfil por la puerta de siempre
// (api.AbrirFuera): iOS se lo da a Safari, que lo descarga y lo deja en Ajustes ›
// «Perfil descargado». Solo https: el enlace lleva la credencial.
func abrirEnSafari(a fyne.App, enlace string) error {
	if !strings.HasPrefix(enlace, "https://") {
		return fmt.Errorf("hub: el enlace del perfil no es https: %q", enlace)
	}
	fallo := make(chan error, 1)
	fyne.Do(func() { fallo <- api.AbrirFuera(a, enlace) })
	return <-fallo
}
