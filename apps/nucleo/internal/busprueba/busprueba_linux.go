// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

// Package busprueba arranca un bus de sesión de D-Bus solo para un test: el de los tests
// de apps/nucleo que hacen del escritorio al otro lado (lanzador, portal, llavero).
package busprueba

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

// Nuevo arranca un dbus-daemon de sesión, apunta el proceso a él y devuelve una conexión
// nueva: la del «otro lado» (el lanzador, el portal, GNOME Shell). Sin dbus-daemon, el
// test se salta.
//
// Con su propia configuración y SIN carpetas de servicios: con las del sistema, pedir un
// nombre que nadie tiene arrancaría lo que haya instalado (el gnome-keyring de la
// máquina de pruebas contestaba por org.freedesktop.secrets).
func Nuevo(t *testing.T) *dbus.Conn {
	t.Helper()
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("sin dbus-daemon no hay bus")
	}
	ctx, parar := context.WithCancel(context.Background())
	// No t.TempDir(): con el nombre del test la ruta del enchufe pasa de los 108 bytes.
	dir, err := os.MkdirTemp("", "bus")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	config := filepath.Join(dir, "bus.conf")
	if err := os.WriteFile(config, []byte(`<busconfig><type>session</type>
<listen>unix:dir=`+dir+`</listen>
<policy context="default"><allow send_destination="*"/><allow receive_sender="*"/><allow own="*"/></policy>
</busconfig>`), 0o600); err != nil {
		t.Fatal(err)
	}
	daemon := exec.CommandContext(ctx, "dbus-daemon", "--config-file="+config, "--nofork", "--nopidfile", "--print-address=1") //nolint:gosec // G204: la configuración la acaba de escribir el test
	salida, _ := daemon.StdoutPipe()
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { parar(); _ = daemon.Wait() })
	direccion, err := bufio.NewReader(salida).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	direccion = strings.TrimSpace(direccion)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", direccion)
	conn, err := dbus.Connect(direccion)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
