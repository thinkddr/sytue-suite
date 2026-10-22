// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package ficheros

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestDescargasDe es el guardián de la carpeta de descargas en Linux: lo que se bajaba
// acababa en un «~/Downloads» recién creado al lado del «~/Descargas» de verdad, porque
// la ruta se componía a mano. Se ha visto en rojo devolviendo `filepath.Join(casa,
// "Downloads")` sin mirar la configuración: entonces fallan los cuatro primeros casos.
func TestDescargasDe(t *testing.T) {
	const casa = "/home/ada"
	for _, c := range []struct{ nombre, contenido, quiero string }{
		{
			"lo que escribe xdg-user-dirs en español",
			"# generado por xdg-user-dirs\nXDG_DESKTOP_DIR=\"$HOME/Escritorio\"\nXDG_DOWNLOAD_DIR=\"$HOME/Descargas\"\n",
			"/home/ada/Descargas",
		},
		{
			"en otro disco, ruta absoluta",
			"XDG_DOWNLOAD_DIR=\"/mnt/datos/bajadas\"\n",
			"/mnt/datos/bajadas",
		},
		{
			"sin comillas",
			"XDG_DOWNLOAD_DIR=$HOME/Descargas\n",
			"/home/ada/Descargas",
		},
		{
			"con espacios alrededor",
			"  XDG_DOWNLOAD_DIR=\"$HOME/Mis descargas\"  \n",
			"/home/ada/Mis descargas",
		},
		{
			"comentada: no cuenta",
			"#XDG_DOWNLOAD_DIR=\"$HOME/Descargas\"\n",
			"",
		},
		{
			"la casa a secas está desactivada, no es «descarga en ~»",
			"XDG_DOWNLOAD_DIR=\"$HOME\"\n",
			"",
		},
		{
			"un valor relativo no vale",
			"XDG_DOWNLOAD_DIR=\"bajadas\"\n",
			"",
		},
		{
			"otra clave que empieza igual no confunde",
			"XDG_DOWNLOAD_DIRS=\"$HOME/otra\"\nXDG_DOWNLOAD_DIR=\"$HOME/Descargas\"\n",
			"/home/ada/Descargas",
		},
		{"fichero vacío", "", ""},
	} {
		if hay := descargasDe(c.contenido, casa); hay != c.quiero {
			t.Errorf("%s: %q, quiero %q", c.nombre, hay, c.quiero)
		}
	}
}

// TestCarpetaMiraLaConfiguracion comprueba el camino entero, no solo el analizador: la
// variable de entorno manda sobre el fichero, y el fichero sobre el valor por defecto.
func TestCarpetaMiraLaConfiguracion(t *testing.T) {
	casa := t.TempDir()
	t.Setenv("HOME", casa)
	config := filepath.Join(casa, ".config")
	if err := os.MkdirAll(config, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "user-dirs.dirs"),
		[]byte("XDG_DOWNLOAD_DIR=\"$HOME/Descargas\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)

	t.Setenv("XDG_DOWNLOAD_DIR", "")
	if hay, err := Carpeta(); err != nil || hay != filepath.Join(casa, "Descargas") {
		t.Errorf("con user-dirs.dirs: %q (%v), quiero %q", hay, err, filepath.Join(casa, "Descargas"))
	}

	t.Setenv("XDG_DOWNLOAD_DIR", filepath.Join(casa, "Bajadas"))
	if hay, _ := Carpeta(); hay != filepath.Join(casa, "Bajadas") {
		t.Errorf("la variable de entorno no manda: %q", hay)
	}

	t.Setenv("XDG_DOWNLOAD_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, "sin-configurar"))
	if hay, _ := Carpeta(); hay != filepath.Join(casa, "Downloads") {
		t.Errorf("sin configuración quiero ~/Downloads, no %q", hay)
	}
}

// TestNoDisponibleSeDistingueDeUnFallo: sin xdg-open, Abrir e Imprimir tienen que decir
// «esta plataforma no lo tiene» y no un error cualquiera, porque quien llama usa eso para
// NO ofrecer el botón en vez de enseñar un fallo encima de una descarga que ha ido bien.
// Se prueba vaciando el PATH, que es lo que pasa en un contenedor pelado.
func TestNoDisponibleSeDistingueDeUnFallo(t *testing.T) {
	fichero := hacerFichero(t)
	t.Setenv("PATH", filepath.Join(t.TempDir(), "vacío"))
	for nombre, f := range map[string]func(string) error{"Abrir": Abrir, "Imprimir": Imprimir} {
		if err := f(fichero); !errors.Is(err, ErrNoDisponible) {
			t.Errorf("%s sin xdg-open devuelve %v, quiero ErrNoDisponible", nombre, err)
		}
	}
	// Revelar tiene un escalón MÁS —org.freedesktop.FileManager1 por D-Bus—, y en esta
	// máquina sí contesta (el servicio está activable), así que lo que se exige es el
	// contrato y no un resultado: o lo ha enseñado, o dice que no puede. Lo que NO vale
	// es un error crudo del bus o de exec, que es lo que la vista enseñaría como fallo.
	if err := Revelar(fichero); err != nil && !errors.Is(err, ErrNoDisponible) {
		t.Errorf("Revelar devuelve %v: ni lo ha hecho ni dice ErrNoDisponible", err)
	}
}

// TestNoSeLePasaAlSistemaLoQueNoEsUnFichero comprueba que las tres puertas al sistema
// llaman a `local` ANTES de lanzar nada: una carpeta, o algo que no existe, no llega a
// xdg-open ni al bus. Se ha visto en rojo quitando la llamada a `local` de Revelar.
func TestNoSeLePasaAlSistemaLoQueNoEsUnFichero(t *testing.T) {
	dir := t.TempDir()
	for nombre, f := range map[string]func(string) error{"Abrir": Abrir, "Imprimir": Imprimir, "Revelar": Revelar} {
		for _, mala := range []string{"", dir, filepath.Join(dir, "no-está.pdf")} {
			if err := f(mala); err == nil {
				t.Errorf("%s ha dado por buena una ruta que no es un fichero (%q)", nombre, mala)
			}
		}
	}
}

func hacerFichero(t *testing.T) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "informe.pdf")
	if err := os.WriteFile(ruta, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}
