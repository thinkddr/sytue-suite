// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package ficheros

import (
	"encoding/base64"
	"strings"
)

// ordenElegir es el OpenFileDialog de Windows Forms por PowerShell: el cuadro «Abrir»
// del sistema, con varios a la vez. `-STA` porque el diálogo lo exige.
func ordenElegir(titulo string, extensiones []string) (string, []string, error) {
	patrones := make([]string, len(extensiones))
	for i, e := range extensiones {
		patrones[i] = "*." + e
	}
	// El título viene de una traducción, y PowerShell cierra una cadena entre comillas
	// simples también con las tipográficas (’): «'L’app'» rompía el guion y dejaba meter
	// código (FYS-12). Va en base64, que no tiene ninguna comilla, y se decodifica dentro.
	comilla := func(s string) string {
		return "[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + base64.StdEncoding.EncodeToString([]byte(s)) + "'))"
	}
	script := "Add-Type -AssemblyName System.Windows.Forms;" +
		"$d = New-Object System.Windows.Forms.OpenFileDialog;" +
		"$d.Multiselect = $true;" +
		"$d.Title = " + comilla(titulo) + ";" +
		"$d.Filter = " + comilla(titulo+"|"+strings.Join(patrones, ";")) + ";" +
		"if ($d.ShowDialog() -eq 'OK') { $d.FileNames -join \"`n\" } else { exit 1 }"
	return "powershell", []string{"-NoProfile", "-STA", "-Command", script}, nil
}
