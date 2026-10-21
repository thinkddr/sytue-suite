// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"maps"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// AbrirMedioDelSistema reproduce el medio de la caja con el reproductor del sistema: uno
// incrustado (`data:`) se escribe en un fichero temporal y se abre; uno enlazado
// (http/https) se abre en el navegador. Lo que no es ni lo uno ni lo otro no se abre.
//
// Del documento solo se usan los BYTES del medio. Ni el nombre del fichero ni su extensión
// salen de él (ver [destinoMedio]): el documento lo escribe quien lo edita, y el sistema
// decide qué programa abre un fichero por su extensión.
func AbrirMedioDelSistema(c Caja) {
	dir, err := carpetaMedios()
	if err != nil {
		return
	}
	ruta, web, ok := destinoMedio(c, dir)
	switch {
	case !ok:
	case web != nil:
		_ = api.AbrirFuera(fyne.CurrentApp(), web.String())
	default:
		go func() { _ = ficheros.Abrir(ruta) }()
	}
}

var (
	mediosMu  sync.Mutex
	mediosDir string
)

// carpetaMedios es la carpeta temporal de ESTE proceso para los medios incrustados. La
// crea `os.MkdirTemp` —solo de quien ejecuta la app y con nombre al azar—, así que en un
// `/tmp` compartido nadie la conoce ni la puede preparar antes. Solo se recuerda si se
// pudo crear: un fallo (el disco lleno) no deja sin reproducir hasta reiniciar.
func carpetaMedios() (string, error) {
	mediosMu.Lock()
	defer mediosMu.Unlock()
	if mediosDir == "" {
		dir, err := os.MkdirTemp("", "sytue-diapos-")
		if err != nil {
			return "", err
		}
		mediosDir = dir
	}
	return mediosDir, nil
}

// extPorMIME es la extensión con la que se deja en disco un medio incrustado, por su
// MIME: las de `extVideo` y `extAudio` y nada más. Es una lista CERRADA a propósito. Con
// la tabla del sistema (`mime.ExtensionsByType`) el MIME que declara el documento elegía
// la extensión, y con ella el programa que el sistema lanzaba al abrirlo.
//
// ponytail: un MIME de audio o vídeo que no esté aquí no se reproduce en nativo. Si
// aparece uno legítimo, se añade su fila; no se vuelve a preguntar al sistema.
var extPorMIME = func() map[string]string {
	out := map[string]string{
		// Lo que ponen algunos navegadores a esos mismos ficheros.
		"audio/x-m4a": ".m4a", "audio/x-wav": ".wav", "audio/mp3": ".mp3", "audio/x-flac": ".flac",
	}
	for _, tabla := range []map[string]string{extVideo, extAudio} {
		// En orden, para que salga siempre la misma: si dos extensiones comparten MIME
		// queda la última, que es la corriente (`.ogg` y no `.oga`, `.ogv` y no `.ogm`).
		for _, ext := range slices.Sorted(maps.Keys(tabla)) {
			out[tabla[ext]] = "." + ext
		}
	}
	return out
}()

// destinoMedio es dónde se reproduce la caja: el fichero donde se deja un medio
// incrustado, dentro de `dir`, o la dirección de uno enlazado.
//
// El fichero se llama por la huella de su contenido y lleva la extensión de [extPorMIME]:
// el `id` de la caja no entra en la ruta (con barras y `..` la sacaba de la carpeta) y un
// `data:` que no sea audio o vídeo de esa lista ni se escribe ni se abre.
func destinoMedio(c Caja, dir string) (ruta string, web *url.URL, ok bool) {
	if u, err := url.Parse(c.Src); err == nil && (u.Scheme == "https" || u.Scheme == "http") {
		return "", u, true
	}
	cab, datos, hay := strings.Cut(c.Src, ",")
	tipo, esB64 := strings.CutSuffix(strings.TrimPrefix(cab, "data:"), ";base64")
	if !hay || !strings.HasPrefix(cab, "data:") || !esB64 {
		return "", nil, false
	}
	// Sin parámetros (`; codecs=…`) y en minúsculas, que es como está la tabla.
	if t, _, err := mime.ParseMediaType(tipo); err == nil {
		tipo = t
	}
	ext := extPorMIME[tipo]
	if ext == "" {
		return "", nil, false
	}
	crudo, err := base64.StdEncoding.DecodeString(datos)
	if err != nil {
		return "", nil, false
	}
	suma := sha256.Sum256(crudo)
	ruta = filepath.Join(dir, "medio-"+hex.EncodeToString(suma[:8])+ext)
	if os.WriteFile(ruta, crudo, 0o600) != nil {
		return "", nil, false
	}
	return ruta, nil, true
}
