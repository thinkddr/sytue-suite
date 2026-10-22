// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

// # Darle a un adjunto un id de Drive
//
// `docs/CONVENTIONS.md` §13.6 quiere que un documento se abra en SU app, y `abrir.go`
// ya sabe hacerlo: se le pasa una `Referencia` —el id del fichero en Drive— y llama a
// la app dueña del formato. Un adjunto de Sytue Mail no tiene ese id, así que para
// abrirlo hay que dárselo: materializarlo como fichero de Drive. Esto es esa pieza.
//
// # Bajo demanda y por un acto EXPLÍCITO, no al recibir. Medido
//
// Lo automático al recibir se descartó con tres medidas del propio repositorio:
//
//   - **Son bytes de verdad, no una fila.** Mail y Drive comparten el paquete
//     `platform/pkg/objectstore` y el mismo S3, pero NO el bucket:
//     `<prefijo>mail-blobs` contra `<prefijo>drive-blobs`
//     (`deploy/helm/bin/common.sh`). Un fichero de Drive es una fila de
//     `files` con su `storage_key` dentro del bucket de Drive, así que «darle un id»
//     es COPIAR el contenido, no apuntar al que ya hay.
//   - **Se pierde la deduplicación.** Mail guarda el `.eml` crudo UNA vez por
//     SHA-256 y cuenta referencias (`services/mail/db/migrations/0017_blob_refs`): el
//     mismo boletín en cien buzones es un objeto. Drive no deduplica —el `checksum`
//     de su migración 0001 dice literalmente «dedupe futuro»—, así que copiarlo al
//     recibir son cien copias, y contra una cuota que se paga (Free 5 GB, Pro 200 GB).
//   - **Y ensancha el permiso.** En Drive, «Mi unidad» es HOY un espacio de toda la
//     organización, no de la persona: lo dicen `store.ListRecent`
//     («alcance por TENANT … añadir un filtro por owner_sub si el Drive pasa a ser
//     por-usuario»), la verja de `service/channelfiles.go` y `docs/architecture.md`
//     §«Una carpeta Sytue Docs/, no una por persona». Un adjunto que solo veía el
//     buzón pasa a verlo la organización entera. Eso no puede ocurrir solo porque
//     llegue un correo, ni en silencio al pulsar «abrir».
//
// Por eso esto se llama GUARDAR y no «abrir»: quien llama tiene que estar ejecutando
// una acción que la persona ha pedido —el «Guardar en Drive» que la web ya tiene en
// `apps/frontend/apps/mail/src/pages/api/attachment-to-drive.ts`— y decir dónde ha
// quedado. Abrir viene DESPUÉS y gratis: con la `Referencia` que esto devuelve,
// `AbrirFichero` funciona sin tocar nada.
//
// Sytue Rooms no necesita esta pieza: sus adjuntos YA SON ficheros de Drive con id
// (`services/drive/internal/service/channelfiles.go`), pero con ámbito de CANAL
// (`channel_id`, mig 0014) e invisibles en «Mi unidad» a propósito —`store.GetFile`
// lleva un `channel_id IS NULL` explícito para que no se sirvan nunca por la ruta
// personal—. Pasarle ese id a otra app por la vía normal da 404, que es lo correcto;
// convertirlo en un fichero personal para que abra es exactamente la fuga que la
// verja evita. Lo que le falta a Rooms es una ruta de apertura que lleve el canal,
// no un materializador.
//
// # Qué pasa al borrar
//
// Las dos copias son INDEPENDIENTES, y a propósito: borrar el correo no toca el
// fichero de Drive, y borrar el fichero de Drive no toca el correo. Es lo que ya hace
// la web y lo que hacen Gmail y Outlook al guardar un adjunto en Drive o OneDrive.
// Enlazar los ciclos de vida exigiría una FK entre dos servicios con bases distintas
// y convertiría vaciar la papelera del correo en un borrado de ficheros de Drive
// —los que la persona archivó justamente para conservarlos—.
//
// # Abrir dos veces no guarda dos veces
//
// Antes de subir se busca en la carpeta un fichero con el MISMO `checksum` (el
// SHA-256 que Drive calcula al subir, `service.putBlob`). Si está, se devuelve su id
// y no se sube nada. Es lo que le falta a la web, que con su `freeName` deja
// «informe (2).pdf» al segundo intento.

// CarpetaAdjuntos es el nombre de la carpeta destino, el mismo `FOLDER` de
// `attachment-to-drive.ts` y por la misma razón: en la raíz, Drive SUSTITUYE por
// nombre —el adjunto de un tercero acabaría de versión del fichero propio que se
// llame igual— y el nombre no se traduce, o la misma persona tendría cinco carpetas
// al cambiar de idioma. `TestLaCarpetaDeAdjuntosEsLaDeLaWeb` lo ata.
const CarpetaAdjuntos = "Attachments"

// maxNombreDrive es el tope de `domain.ValidName` del servicio de Drive. Se recorta
// aquí para que un nombre largo no se convierta en un 400 después de haber leído el
// adjunto entero.
const maxNombreDrive = 255

// GuardarAdjunto materializa un adjunto como fichero de Drive y devuelve la
// `Referencia` que `AbrirFichero` consume tal cual.
//
// Es una COPIA visible para quien pueda ver «Mi unidad» (ver el comentario de
// arriba): llámese solo desde una acción que la persona haya pedido, y dígase dónde
// ha quedado. Si ya había una copia idéntica, se reutiliza y no se sube nada.
func (d *Drive) GuardarAdjunto(ctx context.Context, nombre, tipo string, contenido []byte) (Referencia, error) {
	nombre = nombreDeAdjunto(nombre)
	carpeta, err := d.carpetaDeAdjuntos(ctx)
	if err != nil {
		return Referencia{}, err
	}
	suma := sha256.Sum256(contenido)
	hexSuma := hex.EncodeToString(suma[:])

	ocupados := make(map[string]bool)
	for cursor := ""; ; {
		pagina, err := d.Children(ctx, carpeta, cursor)
		if err != nil {
			return Referencia{}, err
		}
		for _, f := range pagina.Files {
			if f.Checksum != "" && strings.EqualFold(f.Checksum, hexSuma) {
				return referenciaDeAdjunto(f.ID, f.Name), nil
			}
			ocupados[strings.ToLower(f.Name)] = true
		}
		if pagina.NextCursor == "" {
			break
		}
		cursor = pagina.NextCursor
	}

	libre := nombreLibre(ocupados, nombre)
	f, err := d.Upload(ctx, carpeta, libre, tipo, bytes.NewReader(contenido), int64(len(contenido)))
	if err != nil {
		return Referencia{}, err
	}
	// El nombre es el que se MANDÓ, no el que conteste el servicio: la ruta que viaja
	// a la otra app tiene que ser la del fichero que se acaba de subir.
	return referenciaDeAdjunto(f.ID, libre), nil
}

// referenciaDeAdjunto arma la referencia con la que se abre. La ruta la necesita
// CertiPDF, que es la única app que abre por camino y no por id (`RutaDeApertura`).
func referenciaDeAdjunto(id, nombre string) Referencia {
	return Referencia{ID: id, Ruta: "/" + CarpetaAdjuntos + "/" + nombre}
}

// carpetaDeAdjuntos devuelve el id de la carpeta, creándola la primera vez. Las
// subcarpetas vienen COMPLETAS en la primera página del listado (el servicio devuelve
// `ListSubfolders` entero cuando no hay cursor), así que no hay que paginar para
// saber si está.
func (d *Drive) carpetaDeAdjuntos(ctx context.Context) (string, error) {
	raiz, err := d.Root(ctx)
	if err != nil {
		return "", err
	}
	hijos, err := d.Children(ctx, raiz.ID, "")
	if err != nil {
		return "", err
	}
	for _, c := range hijos.Folders {
		if strings.EqualFold(c.Name, CarpetaAdjuntos) {
			return c.ID, nil
		}
	}
	nueva, err := d.CreateFolder(ctx, raiz.ID, CarpetaAdjuntos)
	if err != nil {
		return "", err
	}
	return nueva.ID, nil
}

// nombreDeAdjunto deja el nombre en una HOJA que Drive acepta.
//
// No es cosmética: el nombre sale de una cabecera MIME, que la escribe quien manda el
// correo. Con separadores dentro, quien elige dónde se guarda el fichero —y qué ruta
// viaja luego a la otra aplicación— sería el remitente y no la persona. Se queda con
// lo que hay tras el último `/` o `\`, sin caracteres de control, y recortado a lo que
// `domain.ValidName` admite conservando la extensión, que es lo que decide qué app lo
// abre. `TestUnAdjuntoNoEligeDondeSeGuarda` lo vigila.
func nombreDeAdjunto(nombre string) string {
	if i := strings.LastIndexAny(nombre, `/\`); i >= 0 {
		nombre = nombre[i+1:]
	}
	nombre = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, nombre)
	nombre = strings.TrimSpace(nombre)
	if nombre == "" || nombre == "." || nombre == ".." {
		return "adjunto"
	}
	if len(nombre) > maxNombreDrive {
		ext := extensionDe(nombre)
		nombre = recortarRunas(nombre[:len(nombre)-len(ext)], maxNombreDrive-len(ext)) + ext
	}
	return nombre
}

// extensionDe es el sufijo con el punto, solo si es corto y no se come el nombre: una
// cadena de 300 caracteres sin puntos no tiene extensión, y «.» a secas tampoco.
func extensionDe(nombre string) string {
	i := strings.LastIndexByte(nombre, '.')
	if i <= 0 || len(nombre)-i > 16 {
		return ""
	}
	return nombre[i:]
}

// recortarRunas corta a n bytes sin partir un carácter por la mitad.
func recortarRunas(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// nombreLibre es el `freeName` de la web: `informe.pdf`, `informe (2).pdf`, … Solo
// hace falta cuando hay otro adjunto con el mismo nombre y CONTENIDO distinto; el
// mismo contenido no llega aquí, que lo atrapa el checksum.
func nombreLibre(ocupados map[string]bool, nombre string) string {
	if !ocupados[strings.ToLower(nombre)] {
		return nombre
	}
	ext := extensionDe(nombre)
	base := nombre[:len(nombre)-len(ext)]
	for n := 2; n < 1000; n++ {
		candidato := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if !ocupados[strings.ToLower(candidato)] {
			return candidato
		}
	}
	// ponytail: mil homónimos distintos en la carpeta. Drive versiona por nombre, así
	// que el peor caso es una versión más del milésimo, no una pérdida.
	return nombre
}
