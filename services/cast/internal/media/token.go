// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package media firma y verifica los enlaces de `/media/{video_id}/{token}/…`
// (CAST-ENCARGO §5.1): el token es HMAC-SHA256(clave, video_id|alcance|exp_unix)
// en base64url, un punto y el exp_unix.
//
// El ALCANCE se desvía del contrato a propósito (que firmaba solo video_id|exp):
// sin él, el token de reproducción que recibe cualquier espectador servía también
// el original, aunque el dueño tuviera la descarga cerrada. Hay dos: `ver` (HLS,
// WebM, póster) y `original`, y el alcance sale de la ruta pedida, así que un
// token de `ver` no abre `/original` y al revés. Lo emite `play` (y la ficha, para el póster)
// y lo verifica el servidor de ficheros. Caduca a las 6 horas: quien sigue viendo
// pide otro `play`.
//
// El token no lleva quién eres: dice «este vídeo, hasta esta hora». Quien lo tiene
// lo puede pasar, igual que el enlace de un vídeo `unlisted`; lo que protege es
// que nadie compone un enlace a un vídeo que no ha podido abrir por la API.
package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Caducidad es cuánto vale un token.
const Caducidad = 6 * time.Hour

// ErrTokenInvalido: firma que no casa, caducado o mal formado. Un solo error a
// propósito: al de fuera no le decimos cuál.
var ErrTokenInvalido = errors.New("media: token inválido")

// Firmante emite y verifica tokens con una clave.
type Firmante struct {
	clave []byte
	ahora func() time.Time
}

// New construye el firmante. La clave viene de MEDIA_SIGNING_KEY; vacía = nil, y
// el servicio no arranca sin ella fuera de desarrollo.
func New(clave string) *Firmante {
	if clave == "" {
		return nil
	}
	return &Firmante{clave: []byte(clave), ahora: time.Now}
}

// Alcance es qué ficheros abre un token.
type Alcance string

// Alcances.
const (
	AlcanceVer      Alcance = "ver"
	AlcanceOriginal Alcance = "original"
)

// AlcanceDe es el alcance que exige una ruta de /media.
func AlcanceDe(ruta string) Alcance {
	if strings.TrimPrefix(ruta, "/") == RutaOriginal {
		return AlcanceOriginal
	}
	return AlcanceVer
}

func (f *Firmante) firma(id uuid.UUID, alcance Alcance, exp int64) []byte {
	m := hmac.New(sha256.New, f.clave)
	m.Write([]byte(id.String()))
	m.Write([]byte("|"))
	m.Write([]byte(alcance))
	m.Write([]byte("|"))
	m.Write([]byte(strconv.FormatInt(exp, 10)))
	return m.Sum(nil)
}

// Emitir devuelve el token del vídeo para ese alcance, válido hasta dentro de Caducidad.
func (f *Firmante) Emitir(id uuid.UUID, alcance Alcance) string {
	exp := f.ahora().Add(Caducidad).Unix()
	return base64.RawURLEncoding.EncodeToString(f.firma(id, alcance, exp)) + "." + strconv.FormatInt(exp, 10)
}

// Verificar comprueba que el token es de ese vídeo, abre esa ruta y no ha caducado.
func (f *Firmante) Verificar(id uuid.UUID, token, ruta string) error {
	firmaB64, expS, ok := strings.Cut(token, ".")
	if !ok {
		return ErrTokenInvalido
	}
	exp, err := strconv.ParseInt(expS, 10, 64)
	if err != nil || exp < f.ahora().Unix() {
		return ErrTokenInvalido
	}
	dada, err := base64.RawURLEncoding.DecodeString(firmaB64)
	if err != nil {
		return ErrTokenInvalido
	}
	if subtle.ConstantTimeCompare(dada, f.firma(id, AlcanceDe(ruta), exp)) != 1 {
		return ErrTokenInvalido
	}
	return nil
}

// URL compone la ruta relativa de un fichero del vídeo: `/media/<id>/<token>/<ruta>`.
func (f *Firmante) URL(id uuid.UUID, ruta string) string {
	return "/media/" + id.String() + "/" + f.Emitir(id, AlcanceDe(ruta)) + "/" + strings.TrimPrefix(ruta, "/")
}

// Rutas de los ficheros de un vídeo dentro de `/media/<id>/<token>/`, y cómo se
// llaman sus claves en el almacén. La clave real la guarda la fila del vídeo
// (`hls_key`, `webm_key`, `poster_key`, `original_key`); estas son las rutas que ve
// el cliente, estables aunque cambie dónde se guardan.
//
// Los WebM van uno por calidad, `webm/<q>p.webm` (RutaWebMDe); RutaWebM es el de
// 720p, el único que tenían los vídeos de antes de la escalera, y sigue valiendo.
const (
	RutaHLS      = "hls/master.m3u8"
	RutaWebM     = "webm/720p.webm"
	RutaPoster   = "poster.jpg"
	RutaOriginal = "original"
)

// RutaWebMDe es la ruta del WebM de una calidad: `webm/<q>p.webm`.
func RutaWebMDe(q int) string { return "webm/" + strconv.Itoa(q) + "p.webm" }

// CalidadDeRutaWebM es la calidad de una ruta `webm/<q>p.webm`, o false si la
// ruta no es eso exactamente (nada de `webm/../`, ceros delante ni signos).
func CalidadDeRutaWebM(ruta string) (int, bool) {
	resto, ok := strings.CutPrefix(ruta, "webm/")
	if !ok {
		return 0, false
	}
	num, ok := strings.CutSuffix(resto, "p.webm")
	if !ok || num == "" || len(num) > 4 || num[0] == '0' {
		return 0, false
	}
	for _, c := range num {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	q, err := strconv.Atoi(num)
	return q, err == nil
}
