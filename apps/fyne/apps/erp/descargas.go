// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// # Lo que en la web es una descarga del navegador
//
// «Exportar CSV», UBL, Facturae, imprimir y abrir un documento son `href` al BFF: el
// navegador los guarda en Descargas o los abre en otra pestaña. Aquí se hace lo mismo
// a mano: se guarda en la carpeta de descargas de quien usa la máquina y se abre con
// la aplicación del sistema (la factura para imprimir, que es HTML, en el navegador).
//
// ponytail: en el móvil no hay «Descargas» que el usuario vea; cae en la carpeta
// temporal y se abre desde ahí. Techo: el fichero no queda a mano en el teléfono. Se
// releva con el diálogo de compartir del sistema, que lleva el agente de las funciones
// del SO.

// carpetaDescargas es la de descargas del sistema (la de XDG en Linux, que puede ser
// «Descargas») si existe, y si no una carpeta temporal NUEVA y solo de quien la crea.
// Antes caía en el /tmp compartido, con lectura para todos, en cuanto «Downloads» no
// existía: una factura o un CSV a la vista de cualquier cuenta del equipo (FYS-06).
func carpetaDescargas() string {
	if d, err := ficheros.Carpeta(); err == nil {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			return d
		}
	}
	if d, err := os.MkdirTemp("", "sytue-erp-"); err == nil {
		return d
	}
	return filepath.Join(os.TempDir(), "sytue-erp-sin-carpeta") // Guardar fallará con su error
}

// nombreLibre es `nombre` en `dir`, o «nombre (1)», «(2)»… si ya existe: diez descargas
// no se pisan, que es lo que hace el navegador.
func nombreLibre(dir, nombre string) string {
	nombre = filepath.Base(nombre)
	ruta := filepath.Join(dir, nombre)
	ext := filepath.Ext(nombre)
	base := strings.TrimSuffix(nombre, ext)
	for i := 1; ; i++ {
		if _, err := os.Stat(ruta); os.IsNotExist(err) {
			return ruta
		}
		ruta = filepath.Join(dir, base+" ("+strconv.Itoa(i)+")"+ext)
	}
}

// guardarDescarga escribe `r` en la carpeta de descargas y devuelve la ruta.
func guardarDescarga(dir, nombre string, r io.Reader) (string, error) {
	ruta := nombreLibre(dir, nombre)
	f, err := os.Create(ruta)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(ruta)
		return "", err
	}
	return ruta, f.Close()
}

// abrirFichero lo abre con la aplicación que el sistema tenga para él.
//
// Con `ficheros.Abrir`, que es quien abre ficheros del equipo, y no como una dirección
// `file:`: las direcciones van por `api.AbrirFuera`, que solo admite http y https.
func abrirFichero(ruta string) {
	go func() { _ = ficheros.Abrir(ruta) }() // aparte: lanza un programa del sistema
}

// abrirURL abre un enlace fuera de la app (el QR de la Agencia Tributaria).
func abrirURL(s string) {
	if a := fyne.CurrentApp(); a != nil {
		_ = api.AbrirFuera(a, s)
	}
}

// desplazable es `overflow-auto`: un desplazador vertical SIN las sombras que Fyne pinta
// en los bordes cuando hay más contenido, que la web no tiene.
//
// ponytail: es del ERP porque el tema lo lleva el agente de los elementos transversales;
// si la suite entera quiere quitarlas, va en `tema.Sytue` (`ColorNameShadow`) y esto
// sobra.
func desplazable(o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewThemeOverride(container.NewVScroll(o), temaSinSombra{})
}

type temaSinSombra struct{ tema.Sytue }

// Color deja la sombra del desplazador transparente y lo demás como el tema.
func (t temaSinSombra) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == temafyne.ColorNameShadow {
		return color.Transparent
	}
	return t.Sytue.Color(n, v)
}
