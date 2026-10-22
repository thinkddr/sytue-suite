// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package instaladores es el catálogo de los instaladores nativos visto por las apps de
// escritorio: qué versión hay publicada de cada una, cuánto pesa, su suma SHA-256 y de
// dónde bajarla. Es lo que necesita el actualizador de las apps de Fyne, que hablan con
// el Hub con su token y no tienen la cookie con la que el portal sirve la página de
// descargas (`apps/frontend/apps/portal/src/lib/descargas.ts`).
//
// Mismo bucket y mismo criterio que el portal: tener sesión ya es tener derecho, porque
// Sytue ID no emite ni refresca un token si la organización no tiene la suite. Aquí no
// se repite la comprobación con otra consulta, que sería un segundo criterio que puede
// discrepar del que manda.
//
// La versión y la suma no se deducen: las deja en los metadatos de cada objeto quien lo
// sube (`deploy/helm/bin/traer-instaladores.sh`, `x-amz-meta-sytue-tag` y
// `x-amz-meta-sytue-sha256`). Un instalador sin suma se lista igual, con la suma vacía,
// y la app no lo instala: bajar un ejecutable sin poder comprobarlo no es actualizar.
//
// # Bundles
//
// En escritorio se publica un instalador por BUNDLE («Sytue One», «Sytue Drive»:
// `apps/fyne/scripts/empaquetar.sh`) con varias apps dentro, y quien lo sube deja qué
// apps lleva en `x-amz-meta-sytue-apps`. El catálogo lo cuenta POR APP: una entrada por
// cada una, con el fichero de su bundle. Así cada app encuentra el suyo por su clave
// —el Hub, `sytue-one-…`; Drive, `sytue-drive-…`—, también las versiones ya instaladas,
// que buscan `hub` y no saben de bundles. Sin ese metadato, el nombre del fichero es el
// de la app, que es lo que se publicaba antes (`sytue-hub-…`).
package instaladores

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// Instalador es un instalador publicado de una app para una plataforma.
type Instalador struct {
	// App es la clave del binario (`hub`, `mail`): el directorio de `apps/fyne/cmd`.
	App string `json:"app"`
	// Bundle es la clave del bundle cuyo instalador es `Fichero` (`one`); vacía si el
	// fichero es de la app sola.
	Bundle     string `json:"bundle,omitempty"`
	Plataforma string `json:"plataforma"`
	// Arquitectura es el procesador (`amd64`, `arm64`, `universal`), leído del nombre
	// como `arquitecturaDe` del portal; vacía si el nombre no lo dice.
	Arquitectura string `json:"arquitectura,omitempty"`
	Fichero      string `json:"fichero"`
	// Version va sin la «v» (`3.6.0`), como la graba `fyne package` en la app.
	Version string `json:"version"`
	Bytes   int64  `json:"bytes"`
	// SHA256 en hexadecimal; vacía si quien lo subió no la dejó.
	SHA256 string `json:"sha256"`
	// URL firmada de corta vida, cuando el almacén se alcanza desde fuera. Vacía: se
	// baja por `GET /api/v1/instaladores/{fichero}`, con el token.
	URL string `json:"url,omitempty"`
}

// nombreInstalador es lo que publica `apps/fyne/scripts/empaquetar.sh`, y nada más: el
// bundle (o la app) y un sufijo por plataforma y procesador. Lo demás que haya en el
// bucket (SHA256SUMS, los lotes viejos de Wails, un APK: Android va por Play) no es de
// esto. Sin «/» ni «..» por construcción.
//
// Es LA MISMA que `NOMBRE_INSTALADOR` de apps/portal/src/lib/descargas.ts: la web y las
// apps nativas ofrecen los mismos ficheros. Lo vigila `TestLaWebYElHubAceptanLoMismo`,
// con los vectores de `nombres-instaladores.json`.
var nombreInstalador = regexp.MustCompile(`^sytue-([a-z0-9]+)-(linux-(?:amd64|arm64)\.tar\.xz|windows-(?:amd64|arm64)-installer\.exe|macos-(?:universal|amd64|arm64)\.dmg)$`)

// arquitecturaDe es `arquitecturaDe` de apps/portal/src/lib/descargas.ts: el procesador
// que dice el nombre que da empaquetar.sh, o "" si no lo dice.
var arquitecturaDe = regexp.MustCompile(`-(amd64|arm64|universal)(?:[-.]|$)`)

// ArquitecturaDe lee el procesador del nombre de un instalador.
func ArquitecturaDe(nombre string) string {
	if m := arquitecturaDe.FindStringSubmatch(nombre); m != nil {
		return m[1]
	}
	return ""
}

// plataformaDe es la plataforma de un sufijo: lo que va antes del primer guion.
func plataformaDe(sufijo string) string {
	p, _, _ := strings.Cut(sufijo, "-")
	return p
}

// EsPlataforma dice si `p` es una de las que se publican.
func EsPlataforma(p string) bool { return p == "linux" || p == "windows" || p == "macos" }

// Clasificar es la plataforma y el procesador de un nombre publicado, o vacías si el
// nombre no es de un instalador. Es `clasificar` y `arquitecturaDe` del portal.
func Clasificar(nombre string) (plataforma, arquitectura string) {
	m := nombreInstalador.FindStringSubmatch(nombre)
	if m == nil {
		return "", ""
	}
	return plataformaDe(m[2]), ArquitecturaDe(nombre)
}

// ErrNoEsta es que ese fichero no es un instalador publicado.
var ErrNoEsta = errors.New("instaladores: no está publicado")

// firma es cuánto vive la URL firmada: lo que tarda en empezar la descarga, como en el
// portal.
const firma = 5 * time.Minute

// Catalogo lee el bucket de descargas.
type Catalogo struct {
	lector          *minio.Client
	firmante        *minio.Client // nil: no hay URL que el equipo de la persona alcance
	bucket, prefijo string
}

// Config es lo que hace falta para leerlo.
type Config struct {
	Endpoint, EndpointPublico, Region, Bucket, Prefijo, AccessKey, SecretKey string
}

// Nuevo monta el catálogo. El endpoint admite esquema: `http://` es un almacén en
// claro (el SeaweedFS del piloto), `https://` o nada, con TLS.
func Nuevo(c Config) (*Catalogo, error) {
	lector, seguro, err := cliente(c.Endpoint, c)
	if err != nil {
		return nil, err
	}
	cat := &Catalogo{lector: lector, bucket: c.Bucket, prefijo: c.Prefijo}
	switch {
	case c.EndpointPublico != "":
		if cat.firmante, _, err = cliente(c.EndpointPublico, c); err != nil {
			return nil, err
		}
	case seguro:
		// Un almacén con TLS es uno de fuera (Scaleway): la URL firmada contra él la
		// abre cualquiera. Uno en claro vive dentro del clúster, y su nombre no lo
		// resuelve el equipo de nadie: ahí se baja por el Hub.
		cat.firmante = lector
	}
	return cat, nil
}

func cliente(endpoint string, c Config) (*minio.Client, bool, error) {
	host, seguro := pcfg.NormalizeS3Endpoint(endpoint, true)
	mc, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: seguro,
		// Con la región puesta, minio no pregunta por ella antes de cada operación.
		Region: c.Region,
	})
	if err != nil {
		return nil, false, fmt.Errorf("instaladores: %w", err)
	}
	return mc, seguro, nil
}

// Listar devuelve los instaladores publicados para `plataforma` —o de todas, si va
// vacía: la página de descargas de la app nativa del Hub—, con su versión y su suma. Un
// objeto que no se deja leer se salta: el resto sigue valiendo.
func (c *Catalogo) Listar(ctx context.Context, plataforma string) ([]Instalador, error) {
	var out []Instalador
	for obj := range c.lector.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: c.prefijo}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("instaladores: listar: %w", obj.Err)
		}
		nombre := strings.TrimPrefix(obj.Key, c.prefijo)
		m := nombreInstalador.FindStringSubmatch(nombre)
		if len(m) < 3 || (plataforma != "" && plataformaDe(m[2]) != plataforma) {
			continue
		}
		inst, apps, err := c.leer(ctx, nombre)
		if err != nil {
			continue
		}
		if len(apps) == 0 {
			out = append(out, inst)
			continue
		}
		// Un bundle: una entrada por cada app que lleva, todas con su fichero.
		for _, a := range apps {
			i := inst
			i.App, i.Bundle = a, m[1]
			out = append(out, i)
		}
	}
	return out, nil
}

// clave es la de una app o un bundle: lo que va entre `sytue-` y la plataforma.
var clave = regexp.MustCompile(`^[a-z0-9]+$`)

// leer trae los metadatos de UN instalador, las apps que lleva si es un bundle y, si se
// puede, su URL firmada.
func (c *Catalogo) leer(ctx context.Context, nombre string) (Instalador, []string, error) {
	m := nombreInstalador.FindStringSubmatch(nombre)
	if len(m) < 3 {
		return Instalador{}, nil, ErrNoEsta
	}
	info, err := c.lector.StatObject(ctx, c.bucket, c.prefijo+nombre, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return Instalador{}, nil, ErrNoEsta
		}
		return Instalador{}, nil, err
	}
	var apps []string
	for _, a := range strings.Split(meta(info, "Sytue-Apps"), ",") {
		if a = strings.TrimSpace(a); clave.MatchString(a) {
			apps = append(apps, a)
		}
	}
	inst := Instalador{
		App: m[1], Plataforma: plataformaDe(m[2]), Arquitectura: ArquitecturaDe(nombre),
		Fichero: nombre, Bytes: info.Size,
		Version: strings.TrimPrefix(meta(info, "Sytue-Tag"), "v"),
		SHA256:  strings.ToLower(meta(info, "Sytue-Sha256")),
	}
	if c.firmante != nil {
		q := url.Values{"response-content-disposition": {`attachment; filename="` + nombre + `"`}}
		if u, err := c.firmante.PresignedGetObject(ctx, c.bucket, c.prefijo+nombre, firma, q); err == nil {
			inst.URL = u.String()
		}
	}
	return inst, apps, nil
}

// meta lee un metadato de usuario sin fiarse de cómo lo ha capitalizado el almacén.
func meta(info minio.ObjectInfo, clave string) string {
	for k, v := range info.UserMetadata {
		if strings.EqualFold(k, clave) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Abrir es el instalador entero, para servirlo por el Hub cuando el almacén no se
// alcanza desde fuera. Solo abre nombres de instalador: nada más del bucket.
func (c *Catalogo) Abrir(ctx context.Context, nombre string) (io.ReadCloser, Instalador, error) {
	inst, _, err := c.leer(ctx, nombre)
	if err != nil {
		return nil, Instalador{}, err
	}
	obj, err := c.lector.GetObject(ctx, c.bucket, c.prefijo+nombre, minio.GetObjectOptions{})
	if err != nil {
		return nil, Instalador{}, err
	}
	return obj, inst, nil
}

// topeSumas es lo más que se lee de SHA256SUMS o de su firma: unas líneas de texto.
const topeSumas = 64 << 10

// Sumas es el SHA256SUMS de los instaladores publicados y su firma (base64), tal como
// los dejó quien los sube (`traer-instaladores.sh`). Sin SHA256SUMS, ErrNoEsta; sin
// firma, la firma va vacía. El Hub no verifica nada: lo hace la app con la clave que
// trae embebida (apps/nucleo/firma), que es lo que hace que la firma valga algo.
func (c *Catalogo) Sumas(ctx context.Context) ([]byte, string, error) {
	sumas, err := c.texto(ctx, "SHA256SUMS")
	if err != nil {
		return nil, "", err
	}
	firma, err := c.texto(ctx, "SHA256SUMS.sig")
	if err != nil && !errors.Is(err, ErrNoEsta) {
		return nil, "", err
	}
	return sumas, strings.TrimSpace(string(firma)), nil
}

func (c *Catalogo) texto(ctx context.Context, nombre string) ([]byte, error) {
	obj, err := c.lector.GetObject(ctx, c.bucket, c.prefijo+nombre, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = obj.Close() }()
	b, err := io.ReadAll(io.LimitReader(obj, topeSumas+1))
	if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
		return nil, ErrNoEsta
	}
	if err != nil {
		return nil, err
	}
	if len(b) > topeSumas {
		return nil, fmt.Errorf("instaladores: %s pasa de %d bytes", nombre, topeSumas)
	}
	return b, nil
}
