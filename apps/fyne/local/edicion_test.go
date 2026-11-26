// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema/escritorio"
)

// casaDePrueba deja una carpeta personal vacía (recién instalada) y devuelve la casa y
// las carpetas de ajustes y de caché.
func casaDePrueba(t *testing.T) (casa, config, cache string) {
	t.Helper()
	casa = t.TempDir()
	config, cache = filepath.Join(casa, ".config"), filepath.Join(casa, ".cache")
	for clave, valor := range map[string]string{
		"HOME": casa, "USERPROFILE": casa, "XDG_CONFIG_HOME": config, "XDG_CACHE_HOME": cache,
		"APPDATA": config, "LOCALAPPDATA": cache, "SYTUE_OFFICE_DATOS": "",
	} {
		t.Setenv(clave, valor)
	}
	return casa, config, cache
}

// abrirDePrueba es Abrir con la app del driver de pruebas, dejando al acabar la red del
// proceso como estaba (Abrir pone el portero a cargo de toda ella).
func abrirDePrueba(t *testing.T, producto string) (*App, error) {
	t.Helper()
	transporte, cliente, transporteAPI, office := http.DefaultTransport, http.DefaultClient, api.Transporte, api.FormatosOffice
	t.Cleanup(func() {
		http.DefaultTransport, http.DefaultClient, api.Transporte, api.FormatosOffice = transporte, cliente, transporteAPI, office
	})
	a := test.NewApp()
	t.Cleanup(a.Quit)
	l, err := Abrir(a, producto)
	if l != nil {
		t.Cleanup(l.Cerrar)
	}
	return l, err
}

// TestSinEtiquetaNoArranca (D-02, §8.1): la cáscara solo arranca en un binario compilado
// con la etiqueta `edicionlocal`; sin ella `ui` enlazaría la sesión de la suite y su
// login. Corre en las dos compilaciones: con la etiqueta abre, y sin ella dice por qué no
// y no deja nada en la carpeta personal.
//
// VISTO EN ROJO (2026-11-25): quitando la comprobación de Abrir, sin la etiqueta abrió y
// dejó `~/Sytue Office`.
func TestSinEtiquetaNoArranca(t *testing.T) {
	casa, config, _ := casaDePrueba(t)
	l, err := abrirDePrueba(t, "notes")
	if escritorio.EdicionLocal {
		if err != nil || l == nil {
			t.Fatalf("con la etiqueta no abre: %v", err)
		}
		return
	}
	if !errors.Is(err, ErrSinEtiqueta) || l != nil {
		t.Fatalf("sin la etiqueta: abre=%v, %v; quiero ErrSinEtiqueta", l != nil, err)
	}
	for _, d := range []string{filepath.Join(config, espacio.Carpeta), filepath.Join(casa, espacio.CarpetaDatos)} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("sin la etiqueta ha dejado %s", d)
		}
	}
}

var reServicioDeLaSuite = regexp.MustCompile(`llaveroServicio\s*=\s*"([^"]+)"`)

// TestElEspacioLocalNoEsElDeLaSuite (R-CONVIVE, §8.1): los ajustes de la edición van a
// `<config>/sytue-office`, la caché a `<caché>/sytue-office`, los datos a
// `~/Sytue Office` y los secretos al servicio de llavero `eu.sytue.office` (o a
// `<ajustes>/secretos` donde no hay llavero); los de la conectada siguen donde siempre
// (`<config>/sytue`, servicio `eu.sytue.one`) y no se tocan. El diseño cambió respecto
// al plan (§5.3): los paquetes compartidos no leen la etiqueta; la edición tiene SUS
// rutas (`nucleo/local/espacio`) y la prueba de arranque de cada app comprueba que no
// escribe en las de la conectada.
//
// VISTO EN ROJO (2026-11-25): con espacio.Carpeta en «sytue» y con la cáscara abriendo
// los secretos con el servicio de la suite, falló diciendo cuál chocaba.
func TestElEspacioLocalNoEsElDeLaSuite(t *testing.T) {
	casa, config, cache := casaDePrueba(t)
	e, err := espacio.Abrir("")
	if err != nil {
		t.Fatal(err)
	}
	for que, par := range map[string][2]string{
		"ajustes": {e.Rutas.Ajustes, filepath.Join(config, "sytue-office")},
		"caché":   {e.Rutas.Cache, filepath.Join(cache, "sytue-office")},
		"datos":   {e.Rutas.Datos, filepath.Join(casa, "Sytue Office")},
	} {
		if par[0] != par[1] {
			t.Errorf("la carpeta de %s de la edición es %s; quiero %s", que, par[0], par[1])
		}
	}
	// La de la conectada (en esta casa de prueba): otra, y ninguna dentro de la otra.
	suya, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	if suya == e.Rutas.Ajustes || strings.HasPrefix(e.Rutas.Ajustes, suya+string(filepath.Separator)) || strings.HasPrefix(suya, e.Rutas.Ajustes+string(filepath.Separator)) {
		t.Errorf("los ajustes de la edición (%s) y los de la conectada (%s) se pisan", e.Rutas.Ajustes, suya)
	}
	fuente, err := os.ReadFile(filepath.Join("..", "..", "nucleo", "auth", "store.go"))
	if err != nil {
		t.Fatal(err)
	}
	m := reServicioDeLaSuite.FindSubmatch(fuente)
	if m == nil {
		t.Fatal("nucleo/auth/store.go ya no dice su servicio de llavero")
	}
	if suite, propio := string(m[1]), strings.TrimSuffix(espacio.PrefijoID, "."); espacio.ServicioLlavero == suite || espacio.ServicioLlavero != propio {
		t.Errorf("el servicio de llavero de la edición es %q y el de la suite %q", espacio.ServicioLlavero, suite)
	}
	if !escritorio.EdicionLocal {
		return
	}
	// La cáscara usa esas rutas y ese llavero.
	l, err := abrirDePrueba(t, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if l.Espacio.Rutas != e.Rutas {
		t.Errorf("la cáscara abre %+v; quiero %+v", l.Espacio.Rutas, e.Rutas)
	}
	if l.Secretos.Servicio() != espacio.ServicioLlavero || l.Secretos.Reserva() != filepath.Join(e.Rutas.Ajustes, "secretos") {
		t.Errorf("los secretos van a %q (reserva %s); quiero %q y %s", l.Secretos.Servicio(), l.Secretos.Reserva(), espacio.ServicioLlavero, filepath.Join(e.Rutas.Ajustes, "secretos"))
	}
}
