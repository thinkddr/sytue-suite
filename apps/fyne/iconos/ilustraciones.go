// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"embed"
	"fmt"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// ilustraciones/ es la copia que deja `scripts/sync-branding.sh` de
// `branding/illustrations`, que es la fuente de verdad y la misma que usa la web. La
// copia existe porque `go:embed` no sale de su módulo;
// `TestLasIlustracionesSonLasDeBranding` compara byte a byte.
//
//go:embed ilustraciones/*.svg
var ilustracionesFS embed.FS

// Ilustracion es un spot de `branding/illustrations`. Conjunto CERRADO, como Icono y
// Producto: `iconos.VacioMail` existe o no compila.
type Ilustracion uint8

// Los doce spots que la web TIÑE con el tema, en el orden de `SPOTS` en
// Illustration.tsx.
//
// `hero-suite` no está y no es un olvido: es superficie de marca con el color
// horneado, no line-art que herede nada, y pesa lo que tres spots juntos. La web la
// sirve con un `<img src>` corriente y aquí se lee del propio `ilustracionesFS` quien
// la necesite.
const (
	VacioMail Ilustracion = iota
	VacioDrive
	VacioCalendario
	VacioChat
	VacioContactos
	VacioNotas
	VacioBusqueda
	VacioFotos
	ErrorReintentar
	TodoHecho
	SpotSoberania
	SpotSubida

	numIlustraciones
)

var ficheros = [...]string{
	VacioMail: "empty-mail", VacioDrive: "empty-drive", VacioCalendario: "empty-calendar",
	VacioChat: "empty-chat", VacioContactos: "empty-contacts", VacioNotas: "empty-notes",
	VacioBusqueda: "empty-search", VacioFotos: "empty-photos",
	ErrorReintentar: "error-retry", TodoHecho: "all-done",
	SpotSoberania: "spot-sovereignty", SpotSubida: "spot-upload",
}

// Fichero es el nombre del SVG en `branding/illustrations`, sin extensión.
func (i Ilustracion) Fichero() string {
	if int(i) >= len(ficheros) {
		return fmt.Sprintf("Ilustracion(%d)", uint8(i))
	}
	return ficheros[i]
}

// String hace legible la ilustración en los errores.
func (i Ilustracion) String() string { return i.Fichero() }

// MedidaNatural es el `width`/`height` que el SVG declara. Las trece son 200×160 salvo
// las que no, así que se lee del fichero y no se escribe aquí.
func (i Ilustracion) MedidaNatural() fyne.Size {
	w, h := medidaSVG(i.leer())
	return fyne.NewSize(w, h)
}

func (i Ilustracion) leer() []byte {
	b, err := ilustracionesFS.ReadFile("ilustraciones/" + i.Fichero() + ".svg")
	if err != nil {
		panic("iconos: falta la ilustración " + i.Fichero() + ": pásale scripts/sync-branding.sh")
	}
	return b
}

// Pintar devuelve la ilustración lista para colgar de un renderer, a su tamaño natural
// y con los tokens de la hoja YA resueltos en la variante pedida.
//
// La variante se resuelve aquí y no al pintar, igual que en `Trazo`: quien cambie de
// tema reconstruye el dibujo, que es lo que hace su `Refresh`.
func Pintar(i Ilustracion, v tema.Variante) *Dibujo {
	if int(i) >= int(numIlustraciones) {
		panic(fmt.Sprintf("iconos: ilustración desconocida (%d)", uint8(i)))
	}
	svg := conTokens(i.leer(), v)
	m := i.MedidaNatural()
	d := &Dibujo{svg: func(pintura) []byte { return svg }}
	d.raster = nuevoRaster(d)
	d.raster.Resize(m)
	return d
}

// A es la ilustración escalada a un ancho, conservando la proporción. Es el
// `max-w-[200px]` con `h-auto` que la web le pone en el estado vacío.
func (d *Dibujo) A(ancho float32) *Dibujo {
	m := d.raster.Size()
	if m.Width > 0 {
		d.raster.Resize(fyne.NewSize(ancho, ancho*m.Height/m.Width))
	}
	return d
}
