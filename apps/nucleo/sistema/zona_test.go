// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"runtime"
	"testing"
	"time"
)

// TestZonaIANA: una zona con nombre se queda; «Local» pregunta a TZ; el enlace de
// /etc/localtime se lee en Linux y en macOS; y sin nada, el desfase con su signo al revés.
func TestZonaIANA(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	if z := ZonaIANA(madrid); z != "Europe/Madrid" {
		t.Errorf("una zona con nombre: %q", z)
	}
	if runtime.GOOS != "windows" {
		t.Setenv("TZ", "Europe/Lisbon")
		if z := ZonaIANA(time.FixedZone("Local", 0)); z != "Europe/Lisbon" {
			t.Errorf("con TZ: %q", z)
		}
		t.Setenv("TZ", "Marte/Olympus")
		if z := ZonaIANA(time.FixedZone("Local", 0)); z == "Marte/Olympus" {
			t.Error("una TZ que no existe no vale")
		}
	}
	for destino, quiero := range map[string]string{
		"/usr/share/zoneinfo/Europe/Madrid":       "Europe/Madrid",
		"/var/db/timezone/zoneinfo/Europe/Athens": "Europe/Athens",
		"../usr/share/zoneinfo/UTC":               "UTC",
		"/etc/otra":                               "",
	} {
		if z := zonaDeEnlace(destino); z != quiero {
			t.Errorf("%s: %q, quiero %q", destino, z, quiero)
		}
	}
	for s, quiero := range map[int]string{3600: "Etc/GMT-1", -5 * 3600: "Etc/GMT+5", 0: "UTC", 19800: "UTC"} {
		if z := zonaDelDesfase(time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("", s))); z != quiero {
			t.Errorf("desfase %d: %q, quiero %q", s, z, quiero)
		}
	}
}
