// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
)

// # Lo que otra app deja para que Mail lo adjunte («para-enviar»)
//
// «Abrir en el correo» del ERP abre el redactor de Sytue Mail de este equipo con el PDF
// de la factura ya adjunto. `mailto:` no lleva adjuntos, y el enlace de Sytue Office
// (`sytueoffice-mail:/redactar?…`) puede llegar de CUALQUIER sitio —una web, un correo,
// otro programa—: si llevara una ruta, serviría para adjuntar un fichero cualquiera del
// disco y sacarlo de casa con un clic en «Enviar».
//
// Por eso el enlace no lleva rutas sino IDENTIFICADORES OPACOS (`adjunto=<id>`): quien
// adjunta deja el fichero con Dejar en la carpeta de la edición para eso
// (`espacio.Rutas.ParaEnviar`), bajo un nombre aleatorio de 128 bits que solo conoce él,
// y Mail lo recoge con Recoger, que lo copia a su borrador y lo BORRA: es de un solo uso.
// Una web no puede adivinar el identificador, y uno ya usado no vuelve a servir.
//
// Recoger no acepta nada más: un identificador con otra forma (una ruta, `..`), una
// carpeta que no es la de la edición, un enlace simbólico (la carpeta, la entrega o el
// fichero), algo que no sea un fichero normal, un fichero con más de un nombre (enlace
// duro) o más grande que TopeParaEnviar. Lo que nadie recoge en CaducidadParaEnviar se
// barre (Barrer), lo llame quien deja o quien recoge.
//
// Cada entrega es una carpeta `<id>/` con un único fichero dentro, que lleva el nombre
// con el que se adjunta. Se escribe entera en una carpeta oculta y se renombra al
// final: nunca se ve a medias.

// ParamAdjunto es el parámetro del enlace de redactar que nombra una entrega.
const ParamAdjunto = "adjunto"

// MaxAdjuntosEnlace es cuántas entregas se recogen, como mucho, de un enlace.
const MaxAdjuntosEnlace = 10

// TopeParaEnviar es lo que puede ocupar una entrega, y todas las de un enlace juntas.
const TopeParaEnviar = 25 << 20

// CaducidadParaEnviar es cuánto espera una entrega que nadie recoge antes de barrerse.
const CaducidadParaEnviar = 24 * time.Hour

// largoID es el de `rand.Text`: 26 letras del alfabeto base32 (128 bits).
const largoID = 26

// Sufijos de las carpetas ocultas: la que se está escribiendo y la que se está leyendo.
const (
	sufijoEscribiendo = ".escribiendo"
	sufijoRecogiendo  = ".recogiendo"
)

var (
	// ErrSinParaEnviar: en este equipo las apps no comparten ninguna carpeta (Android).
	ErrSinParaEnviar = errors.New("correo: las apps de este equipo no comparten una carpeta para enviar")
	// ErrEntregaNoValida es todo lo que Recoger rechaza, sin decir por qué a quien manda
	// el enlace.
	ErrEntregaNoValida = errors.New("correo: el adjunto no existe, ya se recogió o no es válido")
	// ErrEntregaGrande: lo que se quiere dejar pasa de TopeParaEnviar.
	ErrEntregaGrande = errors.New("correo: el adjunto es demasiado grande")
)

// ParaEnviar es la carpeta de las entregas (`espacio.Rutas.ParaEnviar`). Vacía, no hay:
// Dejar falla y Recoger lo rechaza todo.
type ParaEnviar struct{ Dir string }

// EnlaceRedactar es el enlace que abre el redactor de Sytue Mail de este equipo con el
// destinatario, el asunto, el texto y las entregas ya dejadas (sus identificadores).
// Lo lee `apps/fyne/local/correo.LeerEnlace`.
func EnlaceRedactar(para, asunto, cuerpo string, adjuntos ...string) string {
	q := url.Values{}
	for _, c := range [][2]string{{"para", para}, {"asunto", asunto}, {"cuerpo", cuerpo}} {
		if c[1] != "" {
			q.Set(c[0], c[1])
		}
	}
	for _, id := range adjuntos {
		q.Add(ParamAdjunto, id)
	}
	return espacio.Esquema("mail") + ":/redactar?" + q.Encode()
}

// Dejar deja datos para adjuntarlos con el nombre dado y devuelve su identificador, que
// es lo que viaja en el enlace. Antes barre lo caducado.
func (s ParaEnviar) Dejar(nombre string, datos []byte) (string, error) {
	if s.Dir == "" {
		return "", ErrSinParaEnviar
	}
	if len(datos) > TopeParaEnviar {
		return "", ErrEntregaGrande
	}
	nombre = NombreParaEnviar(nombre)
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	if !s.propia() {
		return "", ErrEntregaNoValida
	}
	s.barrer(time.Now())
	id := rand.Text()
	oculta := filepath.Join(s.Dir, "."+id+sufijoEscribiendo)
	if err := os.Mkdir(oculta, 0o700); err != nil {
		return "", err
	}
	err := os.WriteFile(filepath.Join(oculta, nombre), datos, 0o600)
	if err == nil {
		err = os.Rename(oculta, filepath.Join(s.Dir, id))
	}
	if err != nil {
		_ = os.RemoveAll(oculta)
		return "", err
	}
	return id, nil
}

// Recoger lee la entrega id y la borra (de un solo uso), o la rechaza con
// ErrEntregaNoValida. Lo rechazado que se llegó a tomar también se borra.
func (s ParaEnviar) Recoger(id string) (api.AdjuntoRedactar, error) {
	if s.Dir == "" || !IDParaEnviar(id) || !s.propia() {
		return api.AdjuntoRedactar{}, ErrEntregaNoValida
	}
	// Tomarla es renombrarla: de dos que la piden a la vez, solo una la encuentra, y un
	// enlace simbólico se renombra a sí mismo, sin seguirlo.
	tomada := filepath.Join(s.Dir, "."+id+sufijoRecogiendo)
	if err := os.Rename(filepath.Join(s.Dir, id), tomada); err != nil {
		return api.AdjuntoRedactar{}, ErrEntregaNoValida
	}
	defer func() { _ = os.RemoveAll(tomada) }()
	return leerEntrega(tomada)
}

// Quitar borra una entrega que ya no se va a recoger (Mail no se abrió).
func (s ParaEnviar) Quitar(id string) {
	if s.Dir != "" && IDParaEnviar(id) && s.propia() {
		_ = os.RemoveAll(filepath.Join(s.Dir, id))
	}
}

// Barrer borra lo que lleva en la carpeta más de CaducidadParaEnviar: lo que nadie
// recogió y lo que quedó a medias. Devuelve cuántas entradas ha quitado.
func (s ParaEnviar) Barrer() int { return s.barrer(time.Now()) }

func (s ParaEnviar) barrer(ahora time.Time) int {
	// Una carpeta que no es la de la edición (un enlace simbólico a otra) no se toca: se
	// borraría lo viejo de una carpeta de la persona.
	if s.Dir == "" || !s.propia() {
		return 0
	}
	es, err := os.ReadDir(s.Dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range es {
		fi, err := e.Info() // sin seguir enlaces
		if err != nil || ahora.Sub(fi.ModTime()) < CaducidadParaEnviar {
			continue
		}
		// RemoveAll quita un enlace simbólico, no lo que hay al otro lado.
		if os.RemoveAll(filepath.Join(s.Dir, e.Name())) == nil {
			n++
		}
	}
	return n
}

// propia dice si Dir es una carpeta de verdad, no un enlace simbólico a otra.
func (s ParaEnviar) propia() bool {
	fi, err := os.Lstat(s.Dir)
	return err == nil && fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0
}

// IDParaEnviar dice si id tiene la forma de un identificador de entrega: 26 letras
// mayúsculas o cifras del 2 al 7, sin nada más (ni rutas, ni puntos, ni barras).
func IDParaEnviar(id string) bool {
	if len(id) != largoID {
		return false
	}
	for i := range len(id) {
		if c := id[i]; (c < 'A' || c > 'Z') && (c < '2' || c > '7') {
			return false
		}
	}
	return true
}

// NombreParaEnviar es el nombre con el que se deja y se adjunta un fichero: sin carpetas,
// sin caracteres de control, ni vacío, ni oculto, y de 200 bytes como mucho.
func NombreParaEnviar(nombre string) string {
	if i := strings.LastIndexAny(nombre, `/\`); i >= 0 {
		nombre = nombre[i+1:]
	}
	nombre = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, nombre)
	nombre = strings.TrimLeft(strings.TrimSpace(nombre), ".")
	for len(nombre) > 200 {
		_, n := utf8.DecodeLastRuneInString(nombre)
		nombre = nombre[:len(nombre)-n]
	}
	if nombre == "" {
		return "adjunto"
	}
	return nombre
}

// leerEntrega lee la única entrada de la carpeta tomada si es un fichero normal, con un
// solo nombre, sin enlaces y dentro del tope.
func leerEntrega(tomada string) (api.AdjuntoRedactar, error) {
	no := api.AdjuntoRedactar{}
	if fi, err := os.Lstat(tomada); err != nil || !fi.IsDir() {
		return no, ErrEntregaNoValida
	}
	es, err := os.ReadDir(tomada)
	if err != nil || len(es) != 1 {
		return no, ErrEntregaNoValida
	}
	nombre := es[0].Name()
	if NombreParaEnviar(nombre) != nombre {
		return no, ErrEntregaNoValida
	}
	ruta := filepath.Join(tomada, nombre)
	visto, err := os.Lstat(ruta)
	if err != nil || !visto.Mode().IsRegular() || visto.Size() > TopeParaEnviar || !unSoloNombre(visto) {
		return no, ErrEntregaNoValida
	}
	f, err := os.OpenFile(ruta, os.O_RDONLY|sinSeguirEnlaces, 0)
	if err != nil {
		return no, ErrEntregaNoValida
	}
	defer func() { _ = f.Close() }()
	// Lo abierto es lo que se miró: nadie lo ha cambiado por otra cosa entre medias.
	if abierto, err := f.Stat(); err != nil || !os.SameFile(visto, abierto) {
		return no, ErrEntregaNoValida
	}
	datos, err := io.ReadAll(io.LimitReader(f, TopeParaEnviar+1))
	if err != nil || len(datos) > TopeParaEnviar {
		return no, ErrEntregaNoValida
	}
	tipo := mime.TypeByExtension(filepath.Ext(nombre))
	if tipo == "" {
		tipo = "application/octet-stream"
	}
	return api.AdjuntoRedactar{Nombre: nombre, Tipo: tipo, Datos: datos}, nil
}
