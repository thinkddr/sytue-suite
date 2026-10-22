// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// exeRaro tiene lo que rompe una línea de órdenes mal citada: un espacio, un dólar, una
// comilla y un ampersand.
const exeRaro = `/opt/Sytue One/$HOME/"hub"&.app`

// TestLaEntradaDeInicioArrancaLaAppEnLaBandeja: las tres formas de pedirlo arrancan ESE
// ejecutable, entero aunque tenga espacios, y con el argumento de quedarse en la bandeja.
// VISTO EN ROJO sin escapar el dólar en `Exec` y sin escapar el ampersand del plist.
func TestLaEntradaDeInicioArrancaLaAppEnLaBandeja(t *testing.T) {
	d := entradaAutostart("Sytue One", exeRaro)
	if !strings.Contains(d, "\nExec=\"/opt/Sytue One/\\$HOME/\\\"hub\\\"&.app\" --bandeja\n") {
		t.Errorf("Exec mal citado:\n%s", d)
	}

	var args []string
	dec := xml.NewDecoder(strings.NewReader(agenteLaunchd("eu.sytue.one", exeRaro)))
	dec.Strict, dec.Entity = false, xml.HTMLEntity // el DOCTYPE de Apple no se resuelve aquí
	enArray := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("el plist no es XML: %v", err)
		}
		switch x := tok.(type) {
		case xml.StartElement:
			enArray = enArray || x.Name.Local == "array"
		case xml.EndElement:
			enArray = enArray && x.Name.Local != "array"
		case xml.CharData:
			if s := strings.TrimSpace(string(x)); enArray && s != "" {
				args = append(args, s)
			}
		}
	}
	if len(args) != 2 || args[0] != exeRaro || args[1] != ArgBandeja {
		t.Errorf("ProgramArguments = %q", args)
	}
	if got := ordenRun(`C:\Users\Ada\AppData\Local\Programs\Sytue One\hub.exe`); got != `"C:\Users\Ada\AppData\Local\Programs\Sytue One\hub.exe" --bandeja` {
		t.Errorf("Run = %s", got)
	}
}

// TestAbrirAlIniciarSesionSePoneYSeQuita: en Linux y en macOS, contra una casa de mentira.
// En el Mac el plist lo valida además `plutil`, que es quien lo va a leer.
func TestAbrirAlIniciarSesionSePoneYSeQuita(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("aquí la entrada de inicio no es un fichero")
	}
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, ".config"))
	const id = "eu.sytue.pruebas"
	if ArrancaAlIniciarSesion(id) {
		t.Fatal("en una casa vacía ya arranca al iniciar sesión")
	}
	if err := AlIniciarSesion(id, "Sytue One", exeRaro, true); err != nil {
		t.Fatal(err)
	}
	if !ArrancaAlIniciarSesion(id) {
		t.Fatal("puesto y no consta")
	}
	if runtime.GOOS == "darwin" {
		if plutil, err := exec.LookPath("plutil"); err == nil {
			ruta := filepath.Join(casa, "Library", "LaunchAgents", id+".plist")
			if salida, err := exec.Command(plutil, "-lint", ruta).CombinedOutput(); err != nil { //nolint:noctx // una prueba
				t.Errorf("plutil no acepta el LaunchAgent: %v\n%s", err, salida)
			}
		}
	}
	for range 2 { // quitarlo dos veces no es un error
		if err := AlIniciarSesion(id, "Sytue One", exeRaro, false); err != nil {
			t.Fatal(err)
		}
	}
	if ArrancaAlIniciarSesion(id) {
		t.Error("quitado y sigue")
	}
	if _, err := os.Stat(filepath.Join(casa, ".config", "autostart")); err != nil && runtime.GOOS == "linux" {
		t.Errorf("la carpeta de autostart no se ha creado: %v", err)
	}
}
