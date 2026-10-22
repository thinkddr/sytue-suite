// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Test de integración del INVARIANTE DE CONFIDENCIALIDAD DE LOS ADJUNTOS DE CANAL
// contra PostgreSQL real (pgtest sobre testcontainers, tras el build-tag
// `integration`). Es el que de verdad prueba la verja: los unitarios corren como
// SUPERUSUARIO y NO ejercitan la RLS de ámbito de la mig 0014.
//
// Hallazgo del comité de paridad de Rooms (docs/roadmap.md, «Ficheros y Sytue
// Drive»): un adjunto de un canal PRIVADO o de un DM se subía a la raíz del tenant y
// «Mi unidad» es un espacio compartido de la organización ⇒ lo leía cualquiera.
//
// Lo que fija este test:
//   - Quien NO es miembro del canal no puede leer el adjunto por NINGUNA vía:
//     metadatos, descarga, miniatura, ni listándolo/buscándolo en su Drive.
//   - Quien SÍ es miembro lo lee entero.
//   - Ser miembro de OTRO canal no sirve (el fichero pertenece a UN canal).
//   - La membresía se evalúa en CADA petición: al salir del canal se pierde el
//     acceso sin tocar dato alguno.
//   - Un fichero adjuntado POR REFERENCIA (ámbito personal) se resuelve por la misma
//     ruta, pero SOLO para miembros del canal.
package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// fakeChannelGate imita la verja de Sytue Rooms: el roster vive en Chat, así que aquí
// se sustituye por un conjunto (canal → miembros) mutable en caliente, que es justo
// lo que permite comprobar que la decisión se toma EN CADA petición.
type fakeChannelGate struct {
	members map[uuid.UUID]map[string]bool
}

func (g *fakeChannelGate) CanAccessChannel(_ context.Context, channelID uuid.UUID, subject string) error {
	if g.members[channelID][subject] {
		return nil
	}
	return domain.ErrNotFound
}

func (g *fakeChannelGate) join(channelID uuid.UUID, subs ...string) {
	if g.members == nil {
		g.members = map[uuid.UUID]map[string]bool{}
	}
	if g.members[channelID] == nil {
		g.members[channelID] = map[string]bool{}
	}
	for _, s := range subs {
		g.members[channelID][s] = true
	}
}

func (g *fakeChannelGate) leave(channelID uuid.UUID, sub string) {
	delete(g.members[channelID], sub)
}

// newChannelService construye un Service REAL sobre PostgreSQL real, con blobs
// falsos (el contenido no es lo que se prueba: lo es quién puede llegar a él).
func newChannelService(t *testing.T) (*Service, *fakeChannelGate, *fakeBlobs, context.Context) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	gate := &fakeChannelGate{}
	blobs := &fakeBlobs{getBody: string(tinyPNG(t))}
	svc := &Service{
		cfg: config.Config{
			DefaultQuotaBytes: 1 << 30,
			MaxUploadBytes:    1 << 20,
			PresignTTL:        time.Minute,
		},
		store: st,
		team:  st,
		blobs: blobs,
		chat:  gate,
	}
	return svc, gate, blobs, tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
}

// tinyPNG genera un PNG mínimo válido (para la miniatura, que decodifica de verdad).
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png de prueba: %v", err)
	}
	return buf.Bytes()
}

// TestChannelAttachmentIsPrivateToChannelMembers es el invariante de la Ola 3A: el
// adjunto de un canal privado NO lo lee quien no es miembro —ni descargándolo, ni por
// la miniatura, ni listándolo— y SÍ lo lee un miembro.
func TestChannelAttachmentIsPrivateToChannelMembers(t *testing.T) {
	svc, gate, _, ctx := newChannelService(t)
	const (
		alice = "alice" // miembro del canal privado
		bob   = "bob"   // otro miembro del tenant, ajeno al canal
	)
	secreto, otro := uuid.New(), uuid.New()
	gate.join(secreto, alice)
	gate.join(otro, bob) // bob es miembro de OTRO canal: no le vale para éste

	// Raíz personal del tenant (para comprobar después que el adjunto no aparece).
	root, err := svc.Root(ctx)
	if err != nil {
		t.Fatalf("Root: %v", err)
	}

	// --- alice adjunta un fichero al canal privado.
	att, err := svc.ChannelUploadFile(ctx, secreto, alice, "presupuesto.png", "image/png",
		strings.NewReader("bytes-confidenciales"))
	if err != nil {
		t.Fatalf("ChannelUploadFile(alice): %v", err)
	}
	if att.ID == uuid.Nil || att.Name != "presupuesto.png" {
		t.Fatalf("adjunto inesperado: %+v", att)
	}

	// --- MIEMBRO: lee metadatos, contenido y miniatura.
	if got, err := svc.ChannelFile(ctx, secreto, att.ID, alice); err != nil || got.Name != att.Name {
		t.Fatalf("miembro ChannelFile => %+v, %v; quiero el adjunto", got, err)
	}
	f, rc, err := svc.ChannelFileContent(ctx, secreto, att.ID, alice)
	if err != nil {
		t.Fatalf("miembro descarga => %v, quiero ok", err)
	}
	body, _ := io.ReadAll(rc)
	_ = rc.Close()
	if len(body) == 0 || f.ID != att.ID {
		t.Fatalf("miembro descarga cuerpo vacío o fichero distinto (%d bytes, %v)", len(body), f.ID)
	}
	tr, ctype, etag, err := svc.ChannelThumbnail(ctx, secreto, att.ID, alice)
	if err != nil {
		t.Fatalf("miembro miniatura => %v, quiero ok", err)
	}
	_ = tr.Close()
	if ctype == "" || etag == "" {
		t.Fatalf("miniatura sin content-type/etag: %q %q", ctype, etag)
	}

	// --- NO MIEMBRO: ni metadatos, ni descarga, ni miniatura. 404, no 403.
	if _, err := svc.ChannelFile(ctx, secreto, att.ID, bob); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-miembro ChannelFile => %v, quiero ErrNotFound", err)
	}
	if _, _, err := svc.ChannelFileContent(ctx, secreto, att.ID, bob); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-miembro descarga => %v, quiero ErrNotFound", err)
	}
	if _, _, _, err := svc.ChannelThumbnail(ctx, secreto, att.ID, bob); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-miembro miniatura => %v, quiero ErrNotFound", err)
	}
	// Ser miembro de otro canal tampoco abre el fichero (pertenece a UN canal).
	if _, _, err := svc.ChannelFileContent(ctx, otro, att.ID, bob); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("miembro de otro canal descarga => %v, quiero ErrNotFound", err)
	}
	// Y bob tampoco puede SUBIR al canal del que no es miembro.
	if _, err := svc.ChannelUploadFile(ctx, secreto, bob, "x.txt", "text/plain",
		strings.NewReader("x")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-miembro sube => %v, quiero ErrNotFound", err)
	}

	// --- NI LISTANDO: el adjunto no existe en el Drive personal de NADIE (RLS de
	// ámbito), tampoco para su autora. Es lo que estaba roto: aparecía en la raíz
	// compartida del tenant y lo bajaba cualquiera con GET /files/{id}/content.
	if _, err := svc.GetFile(ctx, att.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ruta personal GetFile => %v, quiero ErrNotFound", err)
	}
	if _, _, _, err := svc.Thumbnail(ctx, att.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ruta personal Thumbnail => %v, quiero ErrNotFound", err)
	}
	if _, err := svc.GetFolder(ctx, f.FolderID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ruta personal GetFolder(carpeta del canal) => %v, quiero ErrNotFound", err)
	}
	if hits, err := svc.Search(ctx, "presupuesto", 20); err != nil || len(hits) != 0 {
		t.Fatalf("búsqueda personal encuentra %d adjuntos (%v), quiero 0", len(hits), err)
	}
	_, files, err := svc.ListChildren(ctx, root.ID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	for _, x := range files {
		if x.ID == att.ID {
			t.Fatal("el adjunto del canal aparece en la raíz del tenant")
		}
	}
	rec, err := svc.Recent(ctx, "", 50)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	for _, e := range rec.Entries {
		if e.ID == att.ID {
			t.Fatal("el adjunto del canal aparece en «Recientes» del tenant")
		}
	}

	// --- REVOCACIÓN en caliente: la membresía se consulta en CADA petición, así que
	// alice pierde el acceso al salir del canal sin tocar el fichero.
	gate.leave(secreto, alice)
	if _, _, err := svc.ChannelFileContent(ctx, secreto, att.ID, alice); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras salir del canal, alice descarga => %v, quiero ErrNotFound", err)
	}
}

// TestChannelAttachmentByReferenceAndNameCollision cubre los dos casos restantes: el
// fichero PERSONAL adjuntado por referencia (que conserva su ámbito, como en
// Teams/Slack con OneDrive/Drive) y que dos adjuntos con el mismo nombre en el mismo
// canal son ficheros DISTINTOS (un mensaje antiguo nunca cambia de contenido).
func TestChannelAttachmentByReferenceAndNameCollision(t *testing.T) {
	svc, gate, _, ctx := newChannelService(t)
	const (
		alice = "alice"
		bob   = "bob"
	)
	canal := uuid.New()
	gate.join(canal, alice)

	ctxAlice := store.WithOwner(ctx, alice)
	root, err := svc.Root(ctxAlice)
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	// Fichero PERSONAL de alice (el que elegiría el selector «Archivo de Sytue Drive»).
	personal, err := svc.UploadFile(ctxAlice, root.ID, "informe.pdf", "application/pdf", alice,
		strings.NewReader("pdf"))
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	// Un miembro del canal lo resuelve por la ruta del canal (fallback al ámbito
	// personal): es el mismo fichero, no una copia.
	got, err := svc.ChannelFile(ctx, canal, personal.ID, alice)
	if err != nil || got.ID != personal.ID {
		t.Fatalf("adjunto por referencia (miembro) => %+v, %v", got, err)
	}
	// Quien no es miembro del canal no lo obtiene POR ESTA VÍA (la verja de Rooms es
	// la membresía; que «Mi unidad» sea legible por el tenant es un gap propio de
	// Drive, fuera de esta ola).
	if _, err := svc.ChannelFile(ctx, canal, personal.ID, bob); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("adjunto por referencia (no-miembro) => %v, quiero ErrNotFound", err)
	}

	// Mismo nombre dos veces en el mismo canal ⇒ dos ficheros distintos.
	a1, err := svc.ChannelUploadFile(ctx, canal, alice, "captura.png", "image/png", strings.NewReader("uno"))
	if err != nil {
		t.Fatalf("primera subida: %v", err)
	}
	a2, err := svc.ChannelUploadFile(ctx, canal, alice, "captura.png", "image/png", strings.NewReader("dos"))
	if err != nil {
		t.Fatalf("segunda subida: %v", err)
	}
	if a1.ID == a2.ID {
		t.Fatal("la segunda subida SUSTITUYÓ a la primera: el adjunto de un mensaje antiguo cambiaría de contenido")
	}
	if a2.Name == a1.Name {
		t.Fatalf("los dos adjuntos comparten nombre (%q): el índice único los habría rechazado", a2.Name)
	}
	if a1.Version != 1 || a2.Version != 1 {
		t.Fatalf("versiones inesperadas: %d y %d (ninguna es una versión de la otra)", a1.Version, a2.Version)
	}
}
