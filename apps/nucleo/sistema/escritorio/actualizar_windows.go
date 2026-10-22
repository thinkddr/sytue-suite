// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// detachedProcess es DETACHED_PROCESS: el instalador sigue vivo cuando esta app sale.
const detachedProcess = 0x00000008

// instalar lanza el instalador de NSIS y no lo espera: reemplaza la app, así que esta
// tiene que salir para que su ejecutable no esté en uso. Lo enseña con su asistente, que
// es donde Windows pregunta lo que tenga que preguntar.
//
// ponytail: sin verlo en un Windows. El instalador lo baja la propia app, sin la marca
// «de Internet» que pone un navegador, así que SmartScreen no debería asomar.
func instalar(_ context.Context, ruta string) error {
	// Sin el contexto de quien llama, a propósito: el instalador tiene que seguir cuando
	// esta app ya ha salido.
	cmd := exec.CommandContext(context.Background(), ruta) //nolint:gosec,contextcheck // el instalador ya comprobado por su SHA-256
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

const claveDesinstalar = `Software\Microsoft\Windows\CurrentVersion\Uninstall`

// hermanasDe lee del registro las entradas de desinstalación de la suite
// (actualizar_nsis.go).
func hermanasDe(exe string) []Hermana {
	k, err := registry.OpenKey(registry.CURRENT_USER, claveDesinstalar, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer func() { _ = k.Close() }()
	nombres, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	var entradas []entradaNSIS
	for _, n := range nombres {
		if !strings.HasPrefix(n, "eu.sytue.") {
			continue
		}
		sub, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		carpeta, _, _ := sub.GetStringValue("InstallLocation")
		version, _, _ := sub.GetStringValue("DisplayVersion")
		_ = sub.Close()
		entradas = append(entradas, entradaNSIS{Clave: n, Carpeta: carpeta, Version: version})
	}
	return hermanasNSIS(exe, entradas, func(carpeta string) []string {
		exes, _ := filepath.Glob(filepath.Join(carpeta, "sytue-*.exe"))
		return exes
	})
}

// instalarHermana lanza el instalador en silencio y lo ESPERA: las hermanas se ponen de
// una en una, y la última es esta app, con su asistente.
//
// ponytail: sin verlo en un Windows. Si la hermana está abierta, el instalador aparta su
// ejecutable y pone el nuevo al lado; sigue con el viejo hasta que se cierre.
func instalarHermana(ctx context.Context, ruta, _ string) error {
	cmd := exec.CommandContext(ctx, ruta, argsSilencio()...) //nolint:gosec // el instalador ya comprobado por su SHA-256
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
