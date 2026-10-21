// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package voz

import (
	"os"
	"strings"
)

// guion es lo que corre PowerShell: la voz de la cultura pedida si está instalada, y el
// texto leído de un fichero en UTF-8 (por la entrada llegaría en la página de códigos de
// la consola, y un documento no cabe en una variable de entorno).
const guion = `Add-Type -AssemblyName System.Speech
$s = New-Object System.Speech.Synthesis.SpeechSynthesizer
$v = $s.GetInstalledVoices() | Where-Object { $_.Enabled -and $_.VoiceInfo.Culture.Name -like ($env:SYTUE_VOZ_IDIOMA + '*') } | Select-Object -First 1
if ($v) { $s.SelectVoice($v.VoiceInfo.Name) }
$s.Speak([IO.File]::ReadAllText($env:SYTUE_VOZ_TEXTO, [Text.Encoding]::UTF8))`

// leer es SAPI desde PowerShell. Callar mata el proceso, y con él la voz, que habla
// desde dentro de él.
func leer(texto, idioma string, alAcabar func()) (func(), error) {
	f, err := os.CreateTemp("", "sytue-voz-*.txt")
	if err != nil {
		return nil, err
	}
	nombre := f.Name()
	_, err = f.WriteString(texto)
	if cerrar := f.Close(); err == nil {
		err = cerrar
	}
	if err != nil {
		_ = os.Remove(nombre)
		return nil, err
	}
	o := orden("powershell", "-NoProfile", "-NonInteractive", "-Command", guion)
	o.Env = append(os.Environ(), "SYTUE_VOZ_TEXTO="+nombre, "SYTUE_VOZ_IDIOMA="+strings.ReplaceAll(lengua(idioma), "'", ""))
	callar, err := leerCon(o, func() {
		_ = os.Remove(nombre)
		if alAcabar != nil {
			alAcabar()
		}
	}, nil)
	if err != nil {
		_ = os.Remove(nombre)
	}
	return callar, err
}
