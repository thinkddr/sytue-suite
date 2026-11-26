// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
)

// # «Exportar todo» (PLAN §5.13)
//
// Lo de TODAS las apps de la edición en formatos abiertos, de una vez, en una carpeta
// que elige la persona: `<carpeta>/Sytue Office <fecha>/<app>/`, con lo mismo que deja
// cada app en su Ajustes → Copia → «Exportar en formatos abiertos». Son las mismas
// funciones (`nucleo/local/exportar`), con el almacén o la biblioteca de cada app abierto
// por su mismo repositorio. Los almacenes del motor no admiten dos escritores, así que,
// como en «Traer desde Sytue», esas apps tienen que estar cerradas mientras tanto.

// Exportador es el flujo de «Exportar todo» en la ventana del Hub.
type Exportador struct {
	Idioma i18n.Locale
	// Origen es la instalación (su carpeta de datos y su persona).
	Origen exportar.Origen
	W      fyne.Window
	// Abiertas, Elegir, Exportar, Ahora y Mostrar se inyectan para las pruebas: qué apps
	// están abiertas (sabido es falso si el sistema no deja saberlo), el selector de
	// carpetas, la exportación, la hora del nombre de la carpeta y enseñarla al acabar.
	Abiertas func(carpetas []string) (abiertas []string, sabido bool)
	Elegir   func(titulo string, listo func(carpeta string))
	Exportar func(ctx context.Context, o exportar.Origen, destino string, carpeta func(string) string) ([]exportar.Parte, error)
	Ahora    func() time.Time
	Mostrar  func(carpeta string) error
}

// NuevoExportador es el de la app, con el selector y el lector de procesos del sistema.
func NuevoExportador(l *local.App, w fyne.Window) *Exportador {
	e := l.Espacio
	return &Exportador{
		W: w, Idioma: l.Idioma(),
		Origen:   exportar.Origen{Datos: e.Rutas.Datos, Sujeto: e.ID, Persona: e.Persona(), Idioma: string(l.Idioma())},
		Abiertas: func(cs []string) ([]string, bool) { return abiertas(cs, runtime.GOOS) },
		Elegir:   local.ElegirCarpeta,
		Exportar: exportar.Todo,
		Ahora:    time.Now,
		Mostrar:  ficheros.Abrir,
	}
}

func (x *Exportador) tx(clave string, vars ...string) string { return texto(x.Idioma, clave, vars...) }

// nombreApp es el nombre corto de la app de un producto («Calcus»), o la clave si no
// tiene marca.
func nombreApp(producto string) string {
	if m, ok := iconos.ProductoDe(producto); ok {
		return m.NombreCorto()
	}
	return producto
}

// Empezar comprueba que las apps están cerradas y pide la carpeta.
func (x *Exportador) Empezar() {
	cerrar := carpetasDe(exportar.Productos)
	as, sabido := x.Abiertas(cerrar)
	if len(as) > 0 {
		local.Info(x.W, x.tx("traer.cerrar"), x.tx("exportar.cerrar.texto", "apps", nombres(as)))
		return
	}
	elegir := func() { x.Elegir(x.tx("exportar.elegir"), x.Hacer) }
	if !sabido {
		// Sin poder mirarlo, se le pide a la persona.
		local.Confirmar(x.W, x.tx("exportar.titulo"), x.tx("exportar.cerrar.texto", "apps", nombres(cerrar)),
			x.tx("cancelar"), x.tx("exportar.verbo"), false, elegir)
		return
	}
	elegir()
}

// Hacer exporta dentro de base, en una carpeta nueva con la fecha, con una espera
// delante y, al acabar, dice cómo ha ido.
func (x *Exportador) Hacer(base string) {
	destino := filepath.Join(base, espacio.Nombre+" "+x.Ahora().Format("2006-01-02 15.04"))
	espera := local.Espera(x.W, x.tx("exportar.titulo"), x.tx("exportar.exportando"))
	o := x.Origen
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), time.Hour)
		defer parar()
		var partes []exportar.Parte
		err := os.MkdirAll(destino, 0o700)
		if err == nil {
			partes, err = x.Exportar(ctx, o, destino, nombreApp)
		}
		fyne.Do(func() {
			espera.Cerrar()
			x.Informar(destino, partes, err)
		})
	}()
}

// Resumen es el texto del informe: qué apps se han exportado y dónde, cuántas fotos no
// se han podido copiar y qué ha fallado. hecho es falso si no ha quedado nada.
func (x *Exportador) Resumen(destino string, partes []exportar.Parte, err error) (texto string, hecho bool) {
	var apps, lineas []string
	for _, p := range partes {
		if p.Ficheros > 0 {
			apps = append(apps, nombreApp(p.Producto))
		}
		if p.Fallidos > 0 {
			lineas = append(lineas, x.tx("exportar.fallidos", "app", nombreApp(p.Producto), "n", strconv.Itoa(p.Fallidos)))
		}
	}
	if len(apps) == 0 {
		lineas = append([]string{x.tx("exportar.nada")}, lineas...)
	} else {
		lineas = append([]string{x.tx("exportar.hecho.texto", "apps", strings.Join(apps, ", "), "carpeta", destino)}, lineas...)
	}
	if err != nil {
		lineas = append(lineas, err.Error())
	}
	return strings.Join(lineas, "\n\n"), len(apps) > 0
}

// Informar dice cómo ha ido y, en el escritorio, abre la carpeta.
func (x *Exportador) Informar(destino string, partes []exportar.Parte, err error) {
	cuerpo, hecho := x.Resumen(destino, partes, err)
	titulo := x.tx("exportar.hecho")
	if err != nil {
		titulo = x.tx("exportar.aMedias")
	}
	if !hecho {
		// Una carpeta vacía con la fecha no le sirve a nadie.
		_ = os.Remove(destino)
	}
	local.Info(x.W, titulo, cuerpo)
	if hecho && x.Mostrar != nil && !local.Tactil() {
		_ = x.Mostrar(destino)
	}
}
