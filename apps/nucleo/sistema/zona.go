// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ZonaIANA es el nombre IANA de la zona `z` («Europe/Madrid»), que es lo que piden los
// servicios para calcular en ella una repetición (Calendar, Notepads). Es lo que la web
// saca de `Intl.DateTimeFormat().resolvedOptions().timeZone`.
//
// `time.Local` se llama «Local», que no es un nombre IANA: entonces se pregunta al
// sistema —TZ; si no, el enlace de /etc/localtime (Linux y macOS); en Windows, `tzutil`—
// y, si nada contesta, la zona fija del desfase de ahora («Etc/GMT-1»): sin el cambio de
// hora, pero con el día correcto, que es lo que decide un «cada día laborable». UTC solo
// si ni eso.
func ZonaIANA(z *time.Location) string {
	if n := z.String(); n != "Local" && n != "" {
		return n
	}
	if n := zonaDelSistema(); n != "" {
		return n
	}
	return zonaDelDesfase(time.Now().In(z))
}

func zonaDelSistema() string {
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); valida(tz) {
		return tz
	}
	if runtime.GOOS == "windows" {
		ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelar()
		out, err := exec.CommandContext(ctx, "tzutil", "/g").Output()
		if err != nil {
			return ""
		}
		return zonaDeWindows[strings.TrimSpace(string(out))]
	}
	if destino, err := os.Readlink("/etc/localtime"); err == nil {
		if n := zonaDeEnlace(destino); valida(n) {
			return n
		}
	}
	return ""
}

// zonaDeEnlace es lo que va tras «zoneinfo/» en el destino de /etc/localtime:
// «/usr/share/zoneinfo/Europe/Madrid» en Linux, «/var/db/timezone/zoneinfo/Europe/Madrid»
// en macOS.
func zonaDeEnlace(destino string) string {
	_, n, ok := strings.Cut(destino, "zoneinfo/")
	if !ok {
		return ""
	}
	return n
}

// zonaDelDesfase es la zona fija «Etc/GMT±N» del desfase de `t`; el signo de esas zonas va
// al revés (Etc/GMT-1 es UTC+1). UTC si no es de horas enteras, que no tienen nombre.
func zonaDelDesfase(t time.Time) string {
	_, s := t.Zone()
	if s == 0 || s%3600 != 0 {
		return "UTC"
	}
	h := -s / 3600
	signo := "+"
	if h < 0 {
		signo, h = "-", -h
	}
	return "Etc/GMT" + signo + strconv.Itoa(h)
}

func valida(n string) bool {
	if n == "" || n == "Local" {
		return false
	}
	_, err := time.LoadLocation(n)
	return err == nil
}

// zonaDeWindows son los nombres de `tzutil /g` de las zonas de la UE, el EEE y Suiza, que
// es donde se vende la suite, con su zona IANA (la «001» del mapa de CLDR). Una que no
// esté cae al desfase.
var zonaDeWindows = map[string]string{
	"GMT Standard Time":              "Europe/London",
	"Greenwich Standard Time":        "Atlantic/Reykjavik",
	"W. Europe Standard Time":        "Europe/Berlin",
	"Central Europe Standard Time":   "Europe/Budapest",
	"Central European Standard Time": "Europe/Warsaw",
	"Romance Standard Time":          "Europe/Paris",
	"GTB Standard Time":              "Europe/Bucharest",
	"E. Europe Standard Time":        "Europe/Chisinau",
	"FLE Standard Time":              "Europe/Helsinki",
	"Azores Standard Time":           "Atlantic/Azores",
	"UTC":                            "UTC",
}
