// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"errors"
	"slices"
	"testing"
)

// TestDriveSeMontaConElExploradorDeCadaSistema: la orden de cada sistema lleva la
// dirección con el esquema que entiende su explorador, y lo que no es un https de
// verdad no se le pasa a nadie. VISTO EN ROJO dejando el esquema `https` en Linux (GVfs
// lo abriría en el navegador) y sin el `@SSL` de Windows (el WebClient hablaría en claro).
func TestDriveSeMontaConElExploradorDeCadaSistema(t *testing.T) {
	const dav = "https://drive.sytue.eu/dav/Ada%20L/"
	hay := func(quien ...string) func(string) (string, error) {
		return func(p string) (string, error) {
			if slices.Contains(quien, p) {
				return "/usr/bin/" + p, nil
			}
			return "", errors.New("no está")
		}
	}
	for _, c := range []struct {
		goos   string
		buscar func(string) (string, error)
		prog   string
		args   []string
	}{
		{"darwin", hay(), "osascript", []string{"-e", `mount volume "https://drive.sytue.eu/dav/Ada%20L/"`}},
		{"windows", hay(), "explorer.exe", []string{`\\drive.sytue.eu@SSL\dav\Ada L`}},
		{"linux", hay("dolphin", "nautilus"), "nautilus", []string{"davs://drive.sytue.eu/dav/Ada%20L/"}},
		{"linux", hay("dolphin"), "dolphin", []string{"webdavs://drive.sytue.eu/dav/Ada%20L/"}},
		{"linux", hay(), "xdg-open", []string{"davs://drive.sytue.eu/dav/Ada%20L/"}},
	} {
		prog, args, err := ordenMontar(c.goos, dav, c.buscar)
		if err != nil || prog != c.prog || !slices.Equal(args, c.args) {
			t.Errorf("%s: %s %q (%v), quiero %s %q", c.goos, prog, args, err, c.prog, c.args)
		}
	}
	if _, args, _ := ordenMontar("windows", "https://nas.local:8443/dav", hay()); len(args) != 1 || args[0] != `\\nas.local@SSL@8443\dav` {
		t.Errorf("con puerto = %q", args)
	}
	for _, mal := range []string{"http://drive.sytue.eu/dav/", "https://ada:clave@drive.sytue.eu/dav/", "file:///etc", ""} {
		if _, _, err := ordenMontar("linux", mal, hay("nautilus")); err == nil {
			t.Errorf("%q se ha mandado a montar", mal)
		}
	}
}
