// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// mapBlobs es un blobStore en memoria (keyed por clave) que cuenta Get/Put por
// clave y registra el tenant de cada operación. Sirve para probar la caché de la
// derivada (una segunda petición NO debe regenerar) y el aislamiento por tenant.
type mapBlobs struct {
	data    map[string][]byte
	gets    map[string]int
	puts    map[string]int
	tenants []string
}

func newMapBlobs() *mapBlobs {
	return &mapBlobs{data: map[string][]byte{}, gets: map[string]int{}, puts: map[string]int{}}
}

func (b *mapBlobs) Put(_ context.Context, t tenancy.Tenant, key string, r io.Reader, _ int64, _ objectstore.PutOptions) error {
	body, _ := io.ReadAll(r)
	b.data[key] = body
	b.puts[key]++
	b.tenants = append(b.tenants, t.Slug)
	return nil
}

func (b *mapBlobs) Get(_ context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	b.gets[key]++
	b.tenants = append(b.tenants, t.Slug)
	body, ok := b.data[key]
	if !ok {
		return nil, objectstore.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

func (b *mapBlobs) Delete(_ context.Context, _ tenancy.Tenant, key string) error {
	delete(b.data, key)
	return nil
}

//nolint:nilnil // mock: la firma exige (*url.URL, error) y estas rutas no se ejercitan en este test.
func (b *mapBlobs) PresignGet(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, nil
}

//nolint:nilnil // ídem.
func (b *mapBlobs) PresignPut(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, nil
}

func (b *mapBlobs) Existe(_ context.Context, _ tenancy.Tenant, key string) (bool, error) {
	_, ok := b.data[key]
	return ok, nil
}

func (b *mapBlobs) Copiar(_ context.Context, _ tenancy.Tenant, origen, destino string) error {
	b.data[destino] = b.data[origen]
	return nil
}

func (b *mapBlobs) Stat(_ context.Context, _ tenancy.Tenant, key string) (int64, error) {
	body, ok := b.data[key]
	if !ok {
		return 0, objectstore.ErrNotFound
	}
	return int64(len(body)), nil
}

var _ blobStore = (*mapBlobs)(nil)

// pngBytes codifica una imagen PNG real de w×h (degradado) para usarla de origen.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// pngHeader construye un PNG VÁLIDO (firma + IHDR con CRC correcto + IEND) que
// DECLARA dimensiones w×h pero SIN datos de imagen: simula una bomba de
// descompresión (fichero diminuto, dimensiones enormes). image.DecodeConfig lee sus
// dimensiones sin decodificar píxeles.
func pngHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8 // profundidad de bits
	ihdr[9] = 6 // color type: truecolor + alfa (no paletado)
	writePNGChunk(&b, "IHDR", ihdr)
	writePNGChunk(&b, "IEND", nil)
	return b.Bytes()
}

func writePNGChunk(b *bytes.Buffer, typ string, data []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	b.Write(length[:])
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(typ))
	_, _ = crc.Write(data)
	b.WriteString(typ)
	b.Write(data)
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], crc.Sum32())
	b.Write(c[:])
}

// La miniatura se GENERA en la primera petición (decodifica, redimensiona a ≤256 px,
// cachea la derivada) y en la SEGUNDA se sirve de la caché SIN regenerar (ni releer
// el origen ni volver a hacer Put). El aislamiento por tenant se comprueba: toda
// operación de blob usa el tenant del contexto.
func TestThumbnailGeneratesAndCaches(t *testing.T) {
	id := uuid.New()
	fs := &fakeStore{file: domain.File{
		ID: id, StorageKey: "src-key", ContentType: "image/png", Checksum: "sum1",
	}}
	blobs := newMapBlobs()
	blobs.data["src-key"] = pngBytes(t, 1024, 256) // ancho>alto y >512: fuerza reducción
	s := newSvc(fs, blobs, nil)
	ctx := tenantCtx(t)

	// 1ª llamada: miss → genera y cachea.
	rc, ct, etag, err := s.Thumbnail(ctx, id)
	if err != nil {
		t.Fatalf("Thumbnail (1ª): %v", err)
	}
	out1, _ := io.ReadAll(rc)
	_ = rc.Close()
	if ct != "image/png" {
		t.Fatalf("content-type = %q, quiero image/png", ct)
	}
	// El ETag y la clave llevan el lado máximo: cambiarlo INVALIDA lo cacheado, que es
	// lo que hace que subir la dimensión surta efecto en vez de servirse la vieja.
	if etag != `"sum1-512-thumb"` {
		t.Fatalf("etag = %q, quiero \"sum1-512-thumb\"", etag)
	}
	key := "thumbs/" + id.String() + "/sum1-512"
	if blobs.puts[key] != 1 {
		t.Fatalf("la 1ª llamada debe cachear la derivada 1 vez, puts=%d", blobs.puts[key])
	}
	// La derivada es una imagen válida con lado máx 256 (aspecto preservado: 256×64).
	tm, err := png.Decode(bytes.NewReader(out1))
	if err != nil {
		t.Fatalf("la miniatura no decodifica como PNG: %v", err)
	}
	// 512 de lado mayor, aspecto preservado. Se sirve a 256 px CSS, así que en pantallas
	// 2x la fuente tiene el doble de píxeles que el hueco y se ve nítida.
	if tm.Bounds().Dx() != 512 || tm.Bounds().Dy() != 128 {
		t.Fatalf("dimensiones miniatura = %dx%d, quiero 512x128", tm.Bounds().Dx(), tm.Bounds().Dy())
	}

	// 2ª llamada: HIT → sirve de caché sin regenerar (ni Put nuevo ni releer origen).
	rc2, _, _, err := s.Thumbnail(ctx, id)
	if err != nil {
		t.Fatalf("Thumbnail (2ª): %v", err)
	}
	out2, _ := io.ReadAll(rc2)
	_ = rc2.Close()
	if blobs.puts[key] != 1 {
		t.Fatalf("la 2ª llamada NO debe regenerar (Put), puts=%d", blobs.puts[key])
	}
	if blobs.gets["src-key"] != 1 {
		t.Fatalf("la 2ª llamada NO debe releer el origen, gets(src)=%d", blobs.gets["src-key"])
	}
	if !bytes.Equal(out1, out2) {
		t.Fatal("la miniatura cacheada debe ser idéntica a la generada")
	}
	// Aislamiento por tenant: todo blob se lee/escribe bajo el tenant del contexto.
	for _, tn := range blobs.tenants {
		if tn != "acme" {
			t.Fatalf("operación de blob con tenant ajeno: %q", tn)
		}
	}
}

// GUARDA anti bomba de descompresión: un fichero con dimensiones DECLARADAS enormes
// se rechaza (ErrUnsupportedMedia) y NO se cachea nada (no se decodificó).
func TestThumbnailRejectsDecompressionBomb(t *testing.T) {
	id := uuid.New()
	fs := &fakeStore{file: domain.File{
		ID: id, StorageKey: "bomb-key", ContentType: "image/png", Checksum: "b1",
	}}
	blobs := newMapBlobs()
	blobs.data["bomb-key"] = pngHeader(100000, 100000) // 10^10 px declarados
	s := newSvc(fs, blobs, nil)

	if _, _, _, err := s.Thumbnail(tenantCtx(t), id); !errors.Is(err, domain.ErrUnsupportedMedia) {
		t.Fatalf("bomba => %v, quiero ErrUnsupportedMedia", err)
	}
	if len(blobs.puts) != 0 {
		t.Fatalf("una bomba rechazada NO debe cachear nada, puts=%v", blobs.puts)
	}
}

// La guarda de la cabecera —rechazar por dimensiones ANTES de decodificar— ya no se
// prueba aquí: vive en platform/pkg/miniatura y la cubre
// TestRenderRechazaLaBombaSinDecodificarla. Lo que sí sigue probándose en este
// fichero es que Drive TRADUCE ese rechazo a ErrUnsupportedMedia, que es lo que su
// capa HTTP convierte en 415: ver TestThumbnailRejectsDecompressionBomb, arriba.

// Un fichero que NO es imagen ráster soportada (p.ej. PDF) → ErrUnsupportedMedia,
// sin tocar el object store (ni siquiera se abre el blob).
func TestThumbnailUnsupportedType(t *testing.T) {
	id := uuid.New()
	fs := &fakeStore{file: domain.File{ID: id, StorageKey: "k", ContentType: "application/pdf"}}
	blobs := newMapBlobs()
	s := newSvc(fs, blobs, nil)
	if _, _, _, err := s.Thumbnail(tenantCtx(t), id); !errors.Is(err, domain.ErrUnsupportedMedia) {
		t.Fatalf("no-imagen => %v, quiero ErrUnsupportedMedia", err)
	}
	if len(blobs.gets) != 0 {
		t.Fatalf("un tipo no soportado NO debe abrir ningún blob, gets=%v", blobs.gets)
	}
}

// Gating por permiso / aislamiento por tenant: si el fichero no es accesible (RLS:
// otro tenant o inexistente) GetFile devuelve ErrNotFound y la miniatura NUNCA se
// genera (deny-by-default). El object store no se toca.
func TestThumbnailDeniedByRLS(t *testing.T) {
	id := uuid.New()
	fs := &fakeStore{fileErr: domain.ErrNotFound}
	blobs := newMapBlobs()
	s := newSvc(fs, blobs, nil)
	if _, _, _, err := s.Thumbnail(tenantCtx(t), id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("fichero inaccesible => %v, quiero ErrNotFound", err)
	}
	if len(blobs.gets) != 0 || len(blobs.puts) != 0 {
		t.Fatal("sin acceso al fichero, no debe tocarse el object store")
	}
}

// Blob de origen ausente (metadato sin blob) → ErrNotFound (no un 500).
func TestThumbnailMissingSourceBlob(t *testing.T) {
	id := uuid.New()
	fs := &fakeStore{file: domain.File{ID: id, StorageKey: "gone", ContentType: "image/jpeg", Checksum: "c"}}
	blobs := newMapBlobs() // sin "gone" sembrado
	s := newSvc(fs, blobs, nil)
	if _, _, _, err := s.Thumbnail(tenantCtx(t), id); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("blob de origen ausente => %v, quiero ErrNotFound", err)
	}
}

func TestThumbOutputType(t *testing.T) {
	cases := map[string]struct {
		out string
		ok  bool
	}{
		"image/jpeg":                {"image/jpeg", true},
		"image/jpg":                 {"image/jpeg", true},
		"image/png":                 {"image/png", true},
		"image/gif":                 {"image/png", true},
		"IMAGE/PNG":                 {"image/png", true},  // case-insensitive
		"image/jpeg; charset=utf-8": {"image/jpeg", true}, // ignora parámetros
		"image/webp":                {"", false},          // stdlib no decodifica webp
		"image/svg+xml":             {"", false},
		"application/pdf":           {"", false},
		"":                          {"", false},
	}
	for in, want := range cases {
		got, ok := thumbOutputType(in)
		if got != want.out || ok != want.ok {
			t.Errorf("thumbOutputType(%q) = (%q,%v), quiero (%q,%v)", in, got, ok, want.out, want.ok)
		}
	}
}

// TestThumbVersionIncluyeLaDimension fija lo que hace que subir `thumbMaxDim` sirva de
// algo. Sin la dimensión en la versión, la miniatura cacheada con el tamaño anterior se
// seguiría sirviendo —misma clave de object store— y el navegador que ya la tuviera
// recibiría un 304 con el mismo ETag: el cambio no se notaría en ninguna parte y tampoco
// fallaría en ninguna, que es la peor combinación.
func TestThumbVersionIncluyeLaDimension(t *testing.T) {
	f := domain.File{ID: uuid.New(), Checksum: "abc123"}
	v := thumbVersion(f)
	if !strings.Contains(v, strconv.Itoa(thumbMaxDim)) {
		t.Fatalf("thumbVersion = %q, debe incluir el lado máximo (%d)", v, thumbMaxDim)
	}
	if !strings.Contains(v, "abc123") {
		t.Fatalf("thumbVersion = %q, debe seguir invalidando por checksum", v)
	}
	// Y la clave y el ETag heredan esa invalidación.
	if !strings.Contains(thumbKey(f), v) {
		t.Fatalf("thumbKey = %q, debe llevar la versión %q", thumbKey(f), v)
	}
	if !strings.Contains(thumbETag(f), v) {
		t.Fatalf("thumbETag = %q, debe llevar la versión %q", thumbETag(f), v)
	}

	// Sin checksum (datos heredados) sigue habiendo versión: número de versión + dimensión.
	sinSuma := domain.File{ID: uuid.New(), Version: 3}
	if v2 := thumbVersion(sinSuma); !strings.Contains(v2, "v3") || !strings.Contains(v2, strconv.Itoa(thumbMaxDim)) {
		t.Fatalf("thumbVersion sin checksum = %q, quiero v3 + dimensión", v2)
	}
}
