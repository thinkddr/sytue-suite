// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package objectstore contiene los clientes de almacenamiento de Sytue One,
// S3 (MinIO/Scaleway), Google Drive, OneDrive, Dropbox y WebDAV. Aloja blobs
// grandes (ficheros de Drive, adjuntos de Mail, documentos de Docs); el estado
// vive en PostgreSQL.
//
// El aislamiento por tenant se aplica con prefijos de clave dedicados
// (tenancy.Tenant.StoragePrefix), coherente con docs/CONVENTIONS.md §9.
package objectstore

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Config configura el cliente S3.
type Config struct {
	// Endpoint es el host del servicio S3 (p. ej. "minio:9000" en dev,
	// "s3.fr-par.scw.cloud" en prod). Sin esquema.
	Endpoint string
	// Region es la región S3 (Scaleway: fr-par). Solo UE.
	Region string
	// AccessKey / SecretKey son las credenciales (nunca en git; vienen de env).
	AccessKey string
	SecretKey string
	// UseSSL activa TLS (true en prod, false con MinIO local).
	UseSSL bool
	// Bucket por defecto del servicio (drive-blobs, mail-blobs, docs-blobs...).
	Bucket string
	// StorageClass es la clase con la que se suben los objetos (vacía = la del
	// proveedor). En Scaleway, "ONEZONE_IA" es One Zone: la clase más barata que
	// sirve ficheros en vivo (ADR 0038).
	StorageClass string
	// Remote es el almacén REMOTO (Scaleway One Zone, ADR 0038): recibe los blobs
	// de todos los tenants salvo los de LocalTenants, y siempre los de las
	// cuentas personales (tenancy.PersonalSlugPrefix). Es la regla «todo lo nuevo
	// va a Scaleway, lo que ya estaba se queda»: un bucket crece con el uso y
	// cuesta cero vacío. nil = todo al almacén del servicio. Se rellena con
	// Remote().
	Remote *Config
	// Transport es el transporte HTTP del cliente; nil = el de minio. Lo pasa quien
	// abre un almacén cuya dirección NO es de la casa (el bucket de terceros de una
	// organización, en Drive) para que la conexión pase por la barrera SSRF.
	Transport http.RoundTripper
	// LocalTenants son los tenants que SIGUEN en el almacén del servicio aunque
	// haya Remote: los que ya tenían sus bytes aquí antes de la decisión. Una
	// cuenta personal nunca está en esta lista.
	LocalTenants []string
}

// Client envuelve minio-go con helpers de Sytue One.
type Client struct {
	mc     *minio.Client
	bucket string
	class  string
	// remote es el backend remoto; nil = el mismo que mc.
	remote  *minio.Client
	rbucket string
	rclass  string
	local   map[string]bool
}

// New crea el cliente S3 y valida la configuración mínima.
func New(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("objectstore: endpoint vacío")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("objectstore: credenciales vacías")
	}
	mc, err := cliente(cfg)
	if err != nil {
		return nil, err
	}
	c := &Client{mc: mc, bucket: cfg.Bucket, class: cfg.StorageClass}
	if cfg.Remote != nil {
		if cfg.Remote.Bucket == "" {
			return nil, fmt.Errorf("objectstore: bucket vacío en el almacén remoto")
		}
		if c.remote, err = cliente(*cfg.Remote); err != nil {
			return nil, fmt.Errorf("almacén remoto: %w", err)
		}
		c.rbucket, c.rclass = cfg.Remote.Bucket, cfg.Remote.StorageClass
		c.local = make(map[string]bool, len(cfg.LocalTenants))
		for _, slug := range cfg.LocalTenants {
			c.local[slug] = true
		}
	}
	return c, nil
}

func cliente(cfg Config) (*minio.Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("objectstore: endpoint vacío")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("objectstore: credenciales vacías")
	}
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:    cfg.UseSSL,
		Region:    cfg.Region,
		Transport: cfg.Transport,
	})
	if err != nil {
		return nil, fmt.Errorf("objectstore: crear cliente: %w", err)
	}
	return mc, nil
}

// Remote normaliza la configuración del almacén remoto y devuelve nil si no
// está configurado (endpoint vacío): entonces todo va al almacén del servicio.
// Los nombres de las variables (S3_REMOTE_*) los lee el config.go de cada
// servicio, que es donde el gate de Helm los busca.
func Remote(c Config) *Config {
	if c.Endpoint == "" {
		return nil
	}
	c.Endpoint, c.UseSSL = pcfg.NormalizeS3Endpoint(c.Endpoint, c.UseSSL)
	return &c
}

// backend elige el almacén de una clave. El remoto si existe y el tenant no es
// de los que se quedaron en local (una cuenta personal nunca lo es); el del
// servicio en cualquier otro caso. Las claves del espacio compartido (`shared/`)
// viven SIEMPRE en el del servicio, sea quien sea el tenant que las toca: un
// blob deduplicado entre dos organizaciones tiene que estar en un solo sitio
// para que las dos lo vean, y ese sitio es el que ya existía antes del remoto.
func (c *Client) backend(t tenancy.Tenant, key string) (mc *minio.Client, bucket, class string) {
	if c.remote != nil && !strings.HasPrefix(key, PrefijoCompartido) && (t.Personal() || !c.local[t.Slug]) {
		return c.remote, c.rbucket, c.rclass
	}
	return c.mc, c.bucket, c.class
}

// EnsureBucket crea el bucket por defecto si no existe (idempotente, dev/seed).
func (c *Client) EnsureBucket(ctx context.Context, region string) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("objectstore: comprobar bucket: %w", err)
	}
	if exists {
		return nil
	}
	if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
		return fmt.Errorf("objectstore: crear bucket %q: %w", c.bucket, err)
	}
	return nil
}

// PutOptions controla la subida de un objeto.
type PutOptions struct {
	ContentType string
	// Metadata son cabeceras de usuario (x-amz-meta-*).
	Metadata map[string]string
	// Comprimir guarda el objeto en gzip y lo marca en sus metadatos; Get lo
	// devuelve descomprimido sin que quien lee sepa nada. Es para lo que comprime
	// de verdad y cabe en memoria —un mensaje de correo, un documento—: medido en
	// el piloto, el correo baja un 36-71 % y Docs un 79 %, mientras que Drive y
	// Fotos (ya comprimidos por formato) no llegan al 5 %. Si el resultado no es
	// más pequeño que el original, se guarda sin comprimir.
	//
	// NO para objetos que se sirvan con PresignGet: la URL prefirmada entrega los
	// bytes tal cual están, sin descomprimir.
	Comprimir bool
}

// PrefijoCompartido abre un espacio de claves COMÚN a todas las organizaciones,
// fuera del prefijo por tenant: `shared/<lo que sea>`. Es lo que permite
// deduplicar entre organizaciones —el mismo boletín recibido por cien buzones
// de cincuenta empresas se guarda una vez— con claves direccionadas por el
// contenido (hash SHA-256).
//
// Seguridad: la clave no la elige nunca quien navega. Solo llega a un objeto
// compartido quien tiene una fila (bajo RLS) que apunta a él, y el nombre de
// la clave es el hash del contenido: saber el hash de un documento equivale a
// tener el documento. La contabilidad de referencias vive en el servicio que
// la usa (blob_refs en Mail, candidatos a huérfano en Drive), no aquí.
//
// Vive siempre en el almacén del servicio, también para las cuentas personales
// que tengan almacén propio (ver backend).
const PrefijoCompartido = "shared/"

// metaCodificacion es el metadato de usuario que marca un objeto comprimido.
// S3 lo devuelve en Stat como `X-Amz-Meta-Sytue-Codificacion`.
const metaCodificacion = "sytue-codificacion"

// tamanoDeParte acota el buffer de cada parte en las subidas por streaming.
//
// SIN ESTO EL POD SE MUERE. Cuando el tamaño llega como -1 —que es el caso de
// TODA subida de fichero: el cuerpo es un stream sin Content-Length fiable, ver
// service.putBlob— minio dimensiona las partes para el peor caso posible, un
// objeto de 5 TiB en 10.000 partes, y reserva un buffer de ~524 MiB para copiar
// cada una. El contenedor de Drive tiene 512 Mi de límite: reservar ese buffer
// lo mata con OOMKilled (exit 137) ANTES de escribir un solo byte. Medido el
// 2026-09-07 en el piloto: subir un .odt vacío de unos kilobytes tumbaba el pod,
// y Sytue Docs lo veía como «Drive is unavailable» porque, en efecto, no estaba.
//
// 16 MiB es el mínimo cómodo por encima del que exige S3 (5 MiB) y deja techo de
// sobra: 16 MiB × 10.000 partes = 160 GiB, contra un MAX_UPLOAD_MB de 5 GiB.
// Con el tamaño conocido minio sigue eligiendo subida simple para lo pequeño,
// así que esto solo actúa donde hacía daño.
//
// ponytail: constante y no opción. Nadie necesita variarlo por llamada todavía;
// si algún día hace falta, sube a PutOptions.
const tamanoDeParte = 16 * 1024 * 1024

// Put sube un objeto con prefijo de tenant aplicado a la clave.
func (c *Client) Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts PutOptions) error {
	full := tenantKey(t, key)
	meta := opts.Metadata
	if opts.Comprimir {
		original, comprimido, err := comprimir(r)
		if err != nil {
			return fmt.Errorf("objectstore: comprimir %q: %w", full, err)
		}
		if comprimido != nil {
			meta = make(map[string]string, len(opts.Metadata)+1)
			for k, v := range opts.Metadata {
				meta[k] = v
			}
			meta[metaCodificacion] = "gzip"
			r, size = bytes.NewReader(comprimido), int64(len(comprimido))
		} else {
			r, size = bytes.NewReader(original), int64(len(original))
		}
	}
	mc, bucket, class := c.backend(t, key)
	_, err := mc.PutObject(ctx, bucket, full, r, size, minio.PutObjectOptions{
		ContentType:  opts.ContentType,
		UserMetadata: meta,
		PartSize:     tamanoDeParte,
		StorageClass: class,
	})
	if err != nil {
		return fmt.Errorf("objectstore: put %q: %w", full, err)
	}
	return nil
}

// Get descarga un objeto. El llamador debe cerrar el io.ReadCloser.
func (c *Client) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	full := tenantKey(t, key)
	mc, bucket, _ := c.backend(t, key)
	obj, err := mc.GetObject(ctx, bucket, full, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("objectstore: get %q: %w", full, err)
	}
	// Forzar comprobación temprana de existencia para devolver ErrNotFound.
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("objectstore: stat %q: %w", full, err)
	}
	if codificacionDe(info.UserMetadata) == "gzip" {
		gz, err := gzip.NewReader(obj)
		if err != nil {
			_ = obj.Close()
			return nil, fmt.Errorf("objectstore: descomprimir %q: %w", full, err)
		}
		return &lectorComprimido{Reader: gz, cierra: obj}, nil
	}
	return obj, nil
}

// Existe dice si hay un objeto bajo la clave. Es lo que necesita la
// deduplicación para no volver a subir lo que ya está.
func (c *Client) Existe(ctx context.Context, t tenancy.Tenant, key string) (bool, error) {
	full := tenantKey(t, key)
	mc, bucket, _ := c.backend(t, key)
	_, err := mc.StatObject(ctx, bucket, full, minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("objectstore: existe %q: %w", full, err)
	}
	return true, nil
}

// Copiar duplica un objeto SIN pasar los bytes por el servicio (copia del lado
// del servidor) cuando origen y destino están en el mismo almacén. Es lo que
// mueve una subida directa de Drive a su clave compartida por contenido.
//
// Si están en almacenes distintos —una cuenta personal con almacén propio que
// deduplica hacia el espacio compartido del servicio— S3 no puede copiar entre
// endpoints y los bytes pasan por aquí, tal cual están (comprimidos o no: los
// metadatos viajan con ellos).
func (c *Client) Copiar(ctx context.Context, t tenancy.Tenant, origen, destino string) error {
	mcO, bucketO, _ := c.backend(t, origen)
	mcD, bucketD, class := c.backend(t, destino)
	src := minio.CopySrcOptions{Bucket: bucketO, Object: tenantKey(t, origen)}
	dst := minio.CopyDestOptions{Bucket: bucketD, Object: tenantKey(t, destino)}
	if mcO == mcD {
		if _, err := mcD.CopyObject(ctx, dst, src); err != nil {
			return fmt.Errorf("objectstore: copiar %q → %q: %w", src.Object, dst.Object, err)
		}
		return nil
	}
	obj, err := mcO.GetObject(ctx, bucketO, src.Object, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("objectstore: copiar %q: %w", src.Object, err)
	}
	defer func() { _ = obj.Close() }()
	info, err := obj.Stat()
	if err != nil {
		if isNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("objectstore: copiar %q: %w", src.Object, err)
	}
	if _, err := mcD.PutObject(ctx, bucketD, dst.Object, obj, info.Size, minio.PutObjectOptions{
		ContentType:  info.ContentType,
		UserMetadata: info.UserMetadata,
		PartSize:     tamanoDeParte,
		StorageClass: class,
	}); err != nil {
		return fmt.Errorf("objectstore: copiar %q → %q: %w", src.Object, dst.Object, err)
	}
	return nil
}

// Delete elimina un objeto.
func (c *Client) Delete(ctx context.Context, t tenancy.Tenant, key string) error {
	full := tenantKey(t, key)
	mc, bucket, _ := c.backend(t, key)
	if err := mc.RemoveObject(ctx, bucket, full, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("objectstore: delete %q: %w", full, err)
	}
	return nil
}

// PresignGet genera una URL prefirmada de descarga válida durante expiry.
func (c *Client) PresignGet(ctx context.Context, t tenancy.Tenant, key string, expiry time.Duration) (*url.URL, error) {
	full := tenantKey(t, key)
	mc, bucket, _ := c.backend(t, key)
	u, err := mc.PresignedGetObject(ctx, bucket, full, expiry, url.Values{})
	if err != nil {
		return nil, fmt.Errorf("objectstore: presign get %q: %w", full, err)
	}
	return u, nil
}

// PresignPut genera una URL prefirmada de subida válida durante expiry.
func (c *Client) PresignPut(ctx context.Context, t tenancy.Tenant, key string, expiry time.Duration) (*url.URL, error) {
	full := tenantKey(t, key)
	mc, bucket, _ := c.backend(t, key)
	u, err := mc.PresignedPutObject(ctx, bucket, full, expiry)
	if err != nil {
		return nil, fmt.Errorf("objectstore: presign put %q: %w", full, err)
	}
	return u, nil
}

// Healthz comprueba la conectividad con el backend S3 (para sondas /readyz).
func (c *Client) Healthz(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := c.mc.BucketExists(ctx, c.bucket); err != nil {
		return fmt.Errorf("objectstore: healthz: %w", err)
	}
	if c.remote != nil {
		if _, err := c.remote.BucketExists(ctx, c.rbucket); err != nil {
			return fmt.Errorf("objectstore: healthz almacén remoto: %w", err)
		}
	}
	return nil
}

// ErrNotFound se devuelve cuando un objeto no existe.
var ErrNotFound = errors.New("objectstore: objeto no encontrado")

// tenantKey antepone el prefijo S3 del tenant a la clave, normalizando barras.
// Las claves compartidas (PrefijoCompartido) van tal cual: no son de nadie.
func tenantKey(t tenancy.Tenant, key string) string {
	for key != "" && key[0] == '/' {
		key = key[1:]
	}
	if strings.HasPrefix(key, PrefijoCompartido) {
		return key
	}
	return t.StoragePrefix() + key
}

// comprimir lee el cuerpo entero y devuelve el original y, si sale más pequeño,
// su gzip; `comprimido` nulo significa «guárdalo tal cual». Se lee entero a
// propósito: la opción es para objetos acotados, y un stream no se puede
// comparar con su versión comprimida sin haberlo leído.
func comprimir(r io.Reader) (original, comprimido []byte, err error) {
	original, err = io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(original); err != nil {
		return nil, nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, nil, err
	}
	if buf.Len() >= len(original) {
		return original, nil, nil
	}
	return original, buf.Bytes(), nil
}

// codificacionDe lee el metadato de compresión tal como lo devuelva el
// proveedor: minio quita el `x-amz-meta-` y canoniza las mayúsculas, y otros
// S3 lo dejan entero, así que se compara sin distinguirlas y con o sin prefijo.
func codificacionDe(meta map[string]string) string {
	for k, v := range meta {
		k = strings.ToLower(strings.TrimPrefix(strings.ToLower(k), "x-amz-meta-"))
		if k == metaCodificacion {
			return strings.ToLower(v)
		}
	}
	return ""
}

// lectorComprimido descomprime al vuelo y, al cerrarse, cierra también el
// objeto de debajo: quien lee no sabe que hubo gzip por medio.
type lectorComprimido struct {
	*gzip.Reader
	cierra io.Closer
}

func (l *lectorComprimido) Close() error {
	errGz := l.Reader.Close()
	if err := l.cierra.Close(); err != nil {
		return err
	}
	return errGz
}

func isNotFound(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound
}
