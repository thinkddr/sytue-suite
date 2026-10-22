// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import "testing"

// TestSoloSeActualizaAUnaVersionPosteriorQueSeEntiende: la comparación es numérica
// (3.10.0 va después de 3.9.9, que como texto sería al revés) y una versión que no se
// entiende no es posterior a nada. VISTO EN ROJO comparando como texto.
func TestSoloSeActualizaAUnaVersionPosteriorQueSeEntiende(t *testing.T) {
	for _, c := range []struct {
		actual, publicada string
		nueva             bool
	}{
		{"3.9.9", "3.10.0", true},
		{"3.6.0", "v3.6.1", true},
		{"0.0.0", "3.6.0", true},
		{"3.6.0", "3.6.0", false},
		{"3.10.0", "3.9.9", false},
		{"3.6.0", "", false},
		{"", "3.6.0", false},
		{"3.6.0", "3.7.0-rc1", false},
		{"3.6", "3.7.0", false},
	} {
		if got := MasNueva(c.actual, c.publicada); got != c.nueva {
			t.Errorf("MasNueva(%q, %q) = %v", c.actual, c.publicada, got)
		}
	}
	for exe, app := range map[string]string{
		"/Applications/Sytue Hub.app/Contents/MacOS/sytue-hub": "hub",
		`C:\Program Files\Sytue Mail\sytue-mail.exe`:           "mail",
		"/home/ada/.local/bin/sytue-docus":                     "docus",
		"/tmp/go-build123/exe/hub":                             "",
		"/usr/bin/sytue-":                                      "",
		"/usr/bin/sytue-../x":                                  "",
	} {
		if got := AppDeEjecutable(exe); got != app {
			t.Errorf("AppDeEjecutable(%q) = %q, quiero %q", exe, got, app)
		}
	}
	if plataformaDe("darwin") != "macos" || plataformaDe("android") != "" || plataformaDe("ios") != "" {
		t.Error("las plataformas del catálogo no son las de Go")
	}
}

// TestLasHermanasDeWindowsSalenDeSusEntradasDeNSIS: de las entradas de desinstalación
// solo cuentan las de la suite con carpeta, cada una con la versión que dejó su
// instalador, y ni esta app ni las de su bundle —las de su carpeta, que pone su mismo
// instalador— son hermanas, aunque la ruta venga con otras mayúsculas. VISTO EN ROJO sin
// el filtro de `eu.sytue.`, comparando la ruta con `==` y sin saltarse la carpeta propia
// (salía Calendar, y el instalador de Sytue One se habría lanzado dos veces).
func TestLasHermanasDeWindowsSalenDeSusEntradasDeNSIS(t *testing.T) {
	const hub = `C:\Users\Ada\AppData\Local\Programs\Sytue One\sytue-hub.exe`
	carpetas := map[string][]string{
		// La carpeta de `hub` con otras mayúsculas y la barra final: es la suya, y la de su
		// bundle. Los ejecutables en minúsculas, o ya los descartaría AppDeEjecutable y el
		// guardián no vería la comparación.
		`C:\Users\Ada\AppData\Local\Programs\sytue one\`: {
			`C:\Users\Ada\AppData\Local\Programs\sytue one\sytue-hub.exe`,
			`C:\Users\Ada\AppData\Local\Programs\Sytue One\sytue-calendar.exe`,
		},
		`C:\Users\Ada\AppData\Local\Programs\Sytue Mail`: {`C:\Users\Ada\AppData\Local\Programs\Sytue Mail\sytue-mail.exe`},
		`C:\Otra\Cosa`: {`C:\Otra\Cosa\sytue-falsa.exe`},
	}
	got := hermanasNSIS(hub, []entradaNSIS{
		{Clave: "eu.sytue.bundle.one", Carpeta: `C:\Users\Ada\AppData\Local\Programs\sytue one\`, Version: "3.6.0"},
		{Clave: "eu.sytue.mail", Carpeta: `C:\Users\Ada\AppData\Local\Programs\Sytue Mail`, Version: "3.5.9"},
		{Clave: "eu.sytue.drive"}, // sin carpeta: un instalador anterior a InstallLocation
		{Clave: "com.otro.programa", Carpeta: `C:\Otra\Cosa`},
	}, func(c string) []string { return carpetas[c] })
	if len(got) != 1 || got[0].App != "mail" || got[0].Version != "3.5.9" ||
		got[0].Destino != `C:\Users\Ada\AppData\Local\Programs\Sytue Mail\sytue-mail.exe` {
		t.Errorf("hermanas = %+v", got)
	}
	if a := argsSilencio(); len(a) != 1 || a[0] != "/S" {
		t.Errorf("el instalador de una hermana no va en silencio: %q", a)
	}
}
