// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"

	// Registra AVIF y BMP (los que mide Photos además de JPEG, PNG, GIF y WebP) y convierte
	// las HEIC con el sistema, como Photos: así entran en ella igual que desde Photos.
	"git.sytue.eu/sytue/one/apps/fyne/local/imagenes"
	"git.sytue.eu/sytue/one/apps/fyne/local/videos"
	"git.sytue.eu/sytue/one/apps/fyne/video/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traer"
)

// # «Traer desde Sytue» (HU-07)
//
// Elegir el ZIP de «Tus datos» que se descargó en la edición conectada (Sytue → Tus
// datos → Descargar una copia) y repartirlo en los almacenes de esta (`local/traer`).
// Es un fichero: no se inicia sesión ni se abre ninguna conexión. Antes se dice qué
// trae y adónde va, y se pide cerrar las apps cuyos almacenes se escriben.
//
// Las fotos y los vídeos entran en Photos como entrarían desde la propia Photos: con
// los mismos formatos (AVIF y BMP con `local/imagenes`), los vídeos medidos con lo mismo
// (`local/videos`) y las HEIC pasadas a JPEG con el conversor del sistema, si lo hay (se
// mira al traer: quien lo instala después de un aviso no tiene que reiniciar el Hub).
// Lo que Photos no sabe abrir en este equipo se queda en Drive y el resumen lo dice.
//
// Traer otra vez la misma copia, o una posterior, no repite nada (`local/traer`): lo que
// cambió en Sytue se pone al día si aquí no se ha tocado y, si se ha tocado, lo de aquí
// se queda como está y lo de Sytue entra aparte. El resumen dice, de cada app, cuánto ya
// estaba, cuánto se ha puesto al día, cuánto había cambiado también aquí y cuánto quitó
// la persona (y no vuelve).

// Traedor es el flujo de «Traer desde Sytue» en la ventana del Hub.
type Traedor struct {
	// Idioma es el de la app; Destino, adónde va lo traído (la carpeta de datos y la
	// persona de esta instalación).
	Idioma  i18n.Locale
	Destino traer.Destino
	W       fyne.Window
	P       *Portada
	// Abiertas dice qué apps (por su carpeta del catálogo) están abiertas; sabido es
	// falso si el sistema no deja saberlo. Se inyecta para las pruebas.
	Abiertas func(carpetas []string) (abiertas []string, sabido bool)
	// Elegir pide el fichero; Traer lo trae. Se inyectan para las pruebas.
	Elegir func(func(ruta string))
	Traer  func(ctx context.Context, ruta string, d traer.Destino) (traer.Informe, error)
	// HEIC da, al traer, el conversor de HEIC del sistema, o nil si este equipo no tiene
	// ninguno; nil, el que diga Destino.
	HEIC func() func(ctx context.Context, ruta string) (string, func(), error)
}

// NuevoTraedor es el de la app, con el selector y el lector de procesos del sistema.
func NuevoTraedor(l *local.App, w fyne.Window, p *Portada) *Traedor {
	e := l.Espacio
	return &Traedor{
		W: w, P: p, Idioma: l.Idioma(),
		Destino:  traer.Destino{Datos: e.Rutas.Datos, Sujeto: e.ID, Persona: e.Persona(), Secretos: l.Secretos, Video: medirVideo},
		Abiertas: func(cs []string) ([]string, bool) { return abiertas(cs, runtime.GOOS) },
		Elegir: func(listo func(string)) {
			local.ElegirFichero(func(rutas []string) { listo(rutas[0]) })
		},
		Traer: traer.Traer,
		HEIC:  conversorHEIC,
	}
}

// conversorHEIC es el de Photos (`local/imagenes`: el del sistema, sips, heif-convert o
// el ffmpeg de la persona) si este equipo tiene alguno; nil si no.
func conversorHEIC() func(context.Context, string) (string, func(), error) {
	if !imagenes.HayConversorHEIC() {
		return nil
	}
	return imagenes.Convertir
}

// conSistema registra una vez el descodificador de vídeo del sistema, como Photos al
// abrirse: mide lo que no es WebM ni MP4 donde no hay ffprobe.
var conSistema = sync.OnceFunc(func() { sistema.Registrar() })

// medirVideo mide un vídeo de la copia como Photos (`local/videos`).
func medirVideo(ctx context.Context, ruta string) traer.Video {
	conSistema()
	m := videos.Medir(ctx, ruta)
	return traer.Video{Poster: m.Poster, Duracion: m.Duracion, Ancho: m.Ancho, Alto: m.Alto}
}

func (t *Traedor) tx(clave string, vars ...string) string { return texto(t.Idioma, clave, vars...) }

// nombres son los nombres cortos de las apps de esas carpetas.
func nombres(carpetas []string) string {
	var out []string
	for _, a := range catalogo {
		if slices.Contains(carpetas, a.Carpeta) {
			out = append(out, a.Marca.NombreCorto())
		}
	}
	return strings.Join(out, ", ")
}

// nombreDeProducto es el nombre corto de la app de un producto del ZIP («Calendar»), o
// la carpeta tal cual si no es de una app de la edición (`account`, `pages`…).
func nombreDeProducto(p string) string {
	if p == "docs" {
		return nombres([]string{"docus", "calcus", "diapos"})
	}
	if m, ok := iconos.ProductoDe(p); ok {
		return m.NombreCorto()
	}
	return p
}

// Empezar pide el ZIP y sigue.
func (t *Traedor) Empezar() {
	t.Elegir(t.Mirar)
}

// Mirar enseña lo que trae el ZIP y pide confirmarlo.
func (t *Traedor) Mirar(ruta string) {
	c, err := traer.Mirar(ruta)
	if err != nil {
		msg := err.Error()
		if errors.Is(err, traer.ErrNoEsDeSytue) {
			msg = t.tx("traer.noEs.texto")
		}
		local.Info(t.W, t.tx("traer.noEs"), msg)
		return
	}
	escriben := carpetasDe(traer.Escriben)
	as, sabido := t.Abiertas(escriben)
	if len(as) > 0 {
		local.Info(t.W, t.tx("traer.cerrar"), t.tx("traer.cerrar.texto", "apps", nombres(as)))
		return
	}
	var lista []string
	for _, p := range c.Productos() {
		lista = append(lista, nombreDeProducto(p))
	}
	cuerpo := t.tx("traer.confirmar.texto", "productos", strings.Join(lista, ", "), "carpeta", "Drive/Traído de Sytue")
	if !sabido {
		// Sin poder mirarlo, se le pide a la persona.
		cuerpo += "\n\n" + t.tx("traer.cerrar.texto", "apps", nombres(escriben))
	}
	local.Confirmar(t.W, t.tx("traer.titulo"), cuerpo, t.tx("cancelar"), t.tx("traer.verbo"), false, func() { t.Hacer(ruta) })
}

// Hacer trae el ZIP con una espera delante y, al acabar, dice cómo ha ido.
func (t *Traedor) Hacer(ruta string) {
	espera := local.Espera(t.W, t.tx("traer.titulo"), t.tx("traer.trayendo"))
	d := t.Destino
	if t.HEIC != nil {
		d.HEIC = t.HEIC()
	}
	go func() {
		inf, err := t.Traer(context.Background(), ruta, d)
		fyne.Do(func() {
			espera.Cerrar()
			t.Informar(inf, err)
			if t.P != nil {
				t.P.Recargar()
			}
		})
	}()
}

// porApp es lo que cuenta n en cada producto del informe, como «Calendar: 4 · Contacts:
// 2», y el total.
func porApp(inf traer.Informe, n func(traer.Parte) int) (total int, lista string) {
	var out []string
	for _, p := range inf.Partes {
		if k := n(p); k > 0 {
			total += k
			out = append(out, nombreDeProducto(p.Producto)+": "+strconv.Itoa(k))
		}
	}
	return total, strings.Join(out, " · ")
}

// Resumen es el texto del informe.
func (t *Traedor) Resumen(inf traer.Informe, err error) string {
	var lineas []string
	for _, p := range inf.Partes {
		if p.Importados > 0 {
			lineas = append(lineas, t.tx("traer.parte", "app", nombreDeProducto(p.Producto), "n", strconv.Itoa(p.Importados)))
		}
	}
	importados, copiados, fallidos := inf.Total()
	_, enDrive := inf.YaEstaban()
	sinAbrir, sinConversor := inf.SinAbrir()
	// De lo que ya se trajo otra vez, cuánto de cada app: lo que ya estaba, lo que se ha
	// puesto al día, lo que cambió también aquí (y se ha dejado como estaba) y lo que la
	// persona quitó (y no vuelve). Una línea de cada, con su lista.
	vuelta := []struct {
		clave string
		n     func(traer.Parte) int
	}{
		{"traer.repetidos", func(p traer.Parte) int { return p.Repetidos }},
		{"traer.actualizados", func(p traer.Parte) int { return p.Actualizados }},
		{"traer.conservados", func(p traer.Parte) int { return p.Conservados }},
		{"traer.borrados", func(p traer.Parte) int { return p.Borrados }},
	}
	algo := importados > 0 || enDrive > 0 || sinAbrir > 0
	var otraVez []string
	for _, v := range vuelta {
		if n, lista := porApp(inf, v.n); n > 0 {
			algo = true
			otraVez = append(otraVez, t.tx(v.clave, "n", strconv.Itoa(n), "lista", lista))
		}
	}
	if !algo {
		lineas = append(lineas, t.tx("traer.nada"))
	}
	lineas = append(lineas, otraVez...)
	if enDrive > 0 {
		// Lo que ya se dejó en Drive otra vez y sigue allí: no se copia otra vez.
		lineas = append(lineas, t.tx("traer.yaEnDrive", "n", strconv.Itoa(enDrive)))
	}
	if copiados > 0 && inf.Carpeta != "" {
		lineas = append(lineas, t.tx("traer.copiados", "n", strconv.Itoa(copiados), "carpeta", inf.Carpeta))
	}
	if sinAbrir > 0 {
		// Lo que Photos no sabe abrir en este equipo, en Drive; y, si son HEIC sin
		// conversor, con qué se arregla.
		lineas = append(lineas, t.tx("traer.sinAbrir", "n", strconv.Itoa(sinAbrir)))
		if sinConversor > 0 {
			lineas = append(lineas, t.tx("traer.sinHEIC"))
		}
	}
	if fallidos > 0 {
		lineas = append(lineas, t.tx("traer.fallidos", "n", strconv.Itoa(fallidos)))
	}
	if err != nil {
		lineas = append(lineas, err.Error())
	}
	return strings.Join(lineas, "\n")
}

// Informar dice cómo ha ido.
func (t *Traedor) Informar(inf traer.Informe, err error) {
	titulo := t.tx("traer.hecho")
	if err != nil {
		titulo = t.tx("traer.aMedias")
	}
	local.Info(t.W, titulo, t.Resumen(inf, err))
}
