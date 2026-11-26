// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package traer_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traer"
	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

type secretos struct{}

func (secretos) Leer(string) (string, error)  { return "", nil }
func (secretos) Guardar(string, string) error { return nil }
func (secretos) Borrar(string) error          { return nil }

const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Sytue Calendar//ES\r\nBEGIN:VEVENT\r\nUID:visita-1@sytue\r\n" +
	"DTSTAMP:20261120T100000Z\r\nDTSTART:20261201T090000Z\r\nDTEND:20261201T100000Z\r\nSUMMARY:Visita del arquitecto\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

const vcf = "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Íñigo Fontanero\r\nORG:Reformas Íñigo\r\nEND:VCARD\r\n" +
	"BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ana Pintora\r\nEND:VCARD\r\n"

const eml = "From: Ana <ana@ejemplo.invalid>\r\nTo: yo@ejemplo.invalid\r\nSubject: Presupuesto del tejado\r\n" +
	"Date: Fri, 20 Nov 2026 10:00:00 +0000\r\nMessage-ID: <1@ejemplo.invalid>\r\n\r\nAdjunto el presupuesto.\r\n"

// docx es un .docx de verdad, sacado de la biblioteca como lo sacaría el servicio.
func docx(t *testing.T) []byte {
	t.Helper()
	b, err := biblioteca.Abrir(t.TempDir(), "docs", biblioteca.Persona{ID: "otro"})
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := b.CreateDocumentFromSource(t.Context(), api.DocText, "Carta", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveDocumentContent(t.Context(), d.ID, "doc:html", "<p>Querida comunidad: la derrama del tejado</p>", api.DocDot{}, 1, false, 0); err != nil {
		t.Fatal(err)
	}
	raw, err := b.ExportDocument(t.Context(), d.ID, "ooxml", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// tusDatos escribe un ZIP con la forma del de «Tus datos» del Hub del servicio.
func tusDatos(t *testing.T, ficheros map[string][]byte) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "sytue-one-acme-2026-11-20.zip")
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	nombres := make([]string, 0, len(ficheros))
	for n := range ficheros {
		nombres = append(nombres, n)
	}
	slices.Sort(nombres)
	for _, n := range nombres {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Deflate, Modified: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(ficheros[n]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// Fechas y identificadores de los cuadernos de ejemplo.
var (
	creada  = time.Date(2025, 5, 1, 9, 0, 0, 0, time.UTC)
	cambio  = time.Date(2025, 6, 7, 18, 15, 0, 0, time.UTC)
	hecha   = time.Date(2025, 6, 2, 12, 0, 0, 0, time.UTC)
	idCasa  = uuid.MustParse("0a000000-0000-4000-8000-000000000001")
	idHijo  = uuid.MustParse("0a000000-0000-4000-8000-000000000002")
	idCifra = uuid.MustParse("0a000000-0000-4000-8000-000000000003")
	nota    = func(n byte) uuid.UUID { return uuid.UUID{0x0b, 6: 0x40, 8: 0x80, 15: n} }
)

// pixel es un PNG de 1×1, el adjunto de una nota.
const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// cuadernos son los HTML nativos de los cuadernos, como los entrega Sytue Notepads en
// «Tus datos» (services/notes: domain.Exportar en HTML, que es model.NotebookAHTML).
func cuadernos() map[string][]byte {
	casa := model.Notebook{ID: idCasa, Title: "Casa", Color: "#86198f", Emoji: "🏠", Layout: model.LayoutTree, CreatedAt: creada, UpdatedAt: cambio}
	hijo := model.Notebook{ID: idHijo, ParentID: &idCasa, Title: "Presupuestos", Color: "#0f766e", Layout: model.LayoutTree, CreatedAt: creada, UpdatedAt: creada}
	cifrado := model.Notebook{
		ID: idCifra, Title: "Secreto", Encrypted: true, Layout: model.LayoutTree,
		Crypto: &model.CryptoParams{V: 1, KDF: "PBKDF2-SHA256", Salt: "c2Fs", Iterations: 600000, Check: "encv1:AAAA"},
	}
	adjunto := `{"attachments":[{"id":"1-abc","name":"pixel.png","size":70,"type":"image/png","dataUrl":"data:image/png;base64,` + pixel + `"}]}`
	notasCasa := []model.Note{
		{
			ID: nota(1), NotebookID: idCasa, Kind: model.KindText, Title: "Llamar al fontanero", Content: "<p>Por la <b>reforma</b> #obra</p>",
			Data: json.RawMessage(adjunto), CreatedAt: creada, UpdatedAt: cambio,
			States: []model.NoteState{{TagID: uuid.New(), TagName: "Prioridad", StateID: uuid.New(), Name: "Alta", Emoji: "🔥", Color: "#dc2626"}},
		},
		{ID: nota(2), NotebookID: idCasa, Kind: model.KindTodo, Title: "Comprar tejas", Checked: true, CheckedAt: &hecha, Position: 1, CreatedAt: creada, UpdatedAt: hecha},
		{ID: nota(3), NotebookID: idCasa, Kind: model.KindImage, Title: "Plano", Data: json.RawMessage(`{"url":"https://ejemplo.invalid/plano.png"}`), Position: 2, CreatedAt: creada, UpdatedAt: creada},
		// Un fichero de Drive: aquí no hay a qué apuntar.
		{
			ID: nota(4), NotebookID: idCasa, Kind: model.KindDrive, Title: "Planos.pdf", Position: 3, CreatedAt: creada, UpdatedAt: creada,
			Links: []model.NoteLink{{TargetKind: model.TargetDriveFile, TargetID: uuid.NewString(), Label: "Planos.pdf"}},
		},
	}
	return map[string][]byte{
		"notes/notebooks/Casa.html":         []byte(model.NotebookAHTML(casa, notasCasa)),
		"notes/notebooks/Presupuestos.html": []byte(model.NotebookAHTML(hijo, []model.Note{{ID: nota(5), NotebookID: idHijo, Kind: model.KindText, Title: "Tejado", Content: "<p>2.400 € la derrama</p>", CreatedAt: creada, UpdatedAt: creada}})),
		"notes/notebooks/Secreto.html":      []byte(model.NotebookAHTML(cifrado, []model.Note{{ID: nota(6), NotebookID: idCifra, ContentCipher: "encv1:BBBB"}})),
	}
}

// tabla es una tabla de relleno en el formato f (Traer no la lee: la fuente es otra).
func tabla(f, nombre string) []byte {
	switch f {
	case "csv":
		return []byte("\xef\xbb\xbfid,title\r\nx," + nombre + "\r\n")
	case "xml":
		return []byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<" + nombre + ">\n</" + nombre + ">\n")
	}
	return []byte("[\n]\n")
}

// completo es una copia de «Tus datos» con las tablas en el formato f: lo de cada
// producto como lo entrega su servicio. Las fotos y sus originales son los de
// testdata/tusdatos (las tablas de Fotos, escritas por el exportador del servicio:
// services/syphotos, TestLaEntregaDeFotosLaLeeSytueOffice).
func completo(t *testing.T, f string) map[string][]byte {
	t.Helper()
	const r = "sytue-one-acme-2026-11-20/"
	out := map[string][]byte{
		r + "account/profile." + f:         tabla(f, "profile"),
		r + "calendar/calendars/Casa.ics":  []byte(ics),
		r + "calendar/events." + f:         tabla(f, "events"),
		r + "contacts/contacts.vcf":        []byte(vcf),
		r + "notes/notebooks." + f:         tabla(f, "notebooks"),
		r + "notes/notes." + f:             tabla(f, "notes"),
		r + "docs/documents/Carta.docx":    docx(t),
		r + "docs/documents." + f:          tabla(f, "documents"),
		r + "mail/messages/INBOX/1.eml":    []byte(eml),
		r + "mail/messages." + f:           tabla(f, "messages"),
		r + "drive/files/Obras/planos.pdf": []byte("%PDF-1.7"),
		r + "drive/files." + f:             tabla(f, "files"),
		r + "hub/notifications." + f:       tabla(f, "notifications"),
		r + "summary." + f:                 tabla(f, "summary"),
	}
	for k, v := range cuadernos() {
		out[r+k] = v
	}
	base := filepath.Join("testdata", "tusdatos")
	err := filepath.WalkDir(filepath.Join(base, "drive"), func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(filepath.Join(base, "drive"), ruta)
		datos, err := os.ReadFile(ruta)
		out[r+"drive/files/"+filepath.ToSlash(rel)] = datos
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, nombre := range []string{"photos", "albums"} {
		datos, err := os.ReadFile(filepath.Join(base, "photos", nombre+"."+f))
		if err != nil {
			t.Fatal(err)
		}
		out[r+"photos/"+nombre+"."+f] = datos
	}
	return out
}

// destino es la carpeta de datos de prueba, con un medidor de vídeos y un conversor de
// HEIC de prueba (los de verdad son los de Photos, en el Hub).
func destino(datos string) traer.Destino {
	d := destinoSinConversor(datos)
	d.HEIC = conversorDePrueba
	return d
}

// destinoSinConversor es destino en un equipo sin con qué convertir las HEIC.
func destinoSinConversor(datos string) traer.Destino {
	return traer.Destino{
		Datos: datos, Sujeto: "instalacion", Persona: biblioteca.Persona{ID: "instalacion", Nombre: "Yo"},
		Secretos: secretos{}, Ahora: func() time.Time { return time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC) },
		Video: medidorDePrueba,
	}
}

// jpegDePrueba es un JPEG de lados x alto con un degradado de semilla n: el mismo para
// los mismos números.
func jpegDePrueba(ancho, alto int, n uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	for y := range alto {
		for x := range ancho {
			img.Set(x, y, color.RGBA{uint8(x * 5), uint8(y * 7), n, 255}) //nolint:gosec // lados pequeños
		}
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 90})
	return b.Bytes()
}

// conversorDePrueba hace de conversor de HEIC del sistema: de una HEIC (por su
// cabecera) saca siempre el mismo JPEG, en un temporal.
func conversorDePrueba(_ context.Context, ruta string) (string, func(), error) {
	datos, err := os.ReadFile(ruta) //nolint:gosec // G304: de la prueba
	if err != nil || !bytes.Contains(datos[:min(len(datos), 32)], []byte("ftypheic")) {
		return "", func() {}, errors.New("no es una HEIC")
	}
	dir, err := os.MkdirTemp("", "heic-de-prueba-")
	if err != nil {
		return "", func() {}, err
	}
	salida := filepath.Join(dir, "convertida.jpg")
	return salida, func() { _ = os.RemoveAll(dir) }, os.WriteFile(salida, jpegDePrueba(40, 30, 9), 0o600)
}

// medidorDePrueba hace de medidor de vídeos de Photos: un WebM lo sabe medir, lo demás
// no.
func medidorDePrueba(_ context.Context, ruta string) traer.Video {
	if !strings.HasSuffix(ruta, ".webm") {
		return traer.Video{}
	}
	return traer.Video{Poster: jpegDePrueba(32, 18, 3), Duracion: 2008 * time.Millisecond, Ancho: 320, Alto: 180}
}

func parte(inf traer.Informe, producto string) traer.Parte {
	for _, p := range inf.Partes {
		if p.Producto == producto {
			return p
		}
	}
	return traer.Parte{}
}

// HU-07: cada producto del ZIP entra en su almacén —y el Hub lo encuentra después— y lo
// que no tiene almacén se deja tal cual en una carpeta de Drive. Traerlo dos veces no
// pisa nada.
func TestTraerRepartePorAlmacenes(t *testing.T) {
	zipRuta := tusDatos(t, completo(t, "json"))
	c, err := traer.Mirar(zipRuta)
	if err != nil {
		t.Fatal(err)
	}
	if c.Raiz != "sytue-one-acme-2026-11-20" || !slices.Equal(c.Productos(), []string{"account", "calendar", "contacts", "docs", "drive", "hub", "mail", "notes", "photos"}) {
		t.Fatalf("contenido = %+v", c)
	}

	datos := t.TempDir()
	inf, err := traer.Traer(t.Context(), zipRuta, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	// Drive: planos.pdf, presupuesto.pdf y la copia de la playa de Copias/ (los demás
	// originales los deja Photos).
	for producto, quiero := range map[string]int{"calendar": 1, "contacts": 2, "notes": 4, "docs": 1, "mail": 1, "drive": 3, "photos": 6} {
		if got := parte(inf, producto).Importados; got != quiero {
			t.Errorf("%s: %d importados, quiero %d (%+v)", producto, got, quiero, inf.Partes)
		}
	}
	if p := parte(inf, "notes"); p.Fallidos != 2 {
		t.Errorf("la nota cifrada y la de Drive no cuentan como fallidas: %+v", p)
	}
	// La foto sin original falla; la AVIF (sin su decodificador en este proceso) se
	// queda en Drive sin abrir.
	if p := parte(inf, "photos"); p.Fallidos != 1 || p.SinAbrir != 1 || p.SinConversor != 0 {
		t.Errorf("la foto sin original y la AVIF: %+v", p)
	}
	carpeta := filepath.Join(datos, "Drive", "Traído de Sytue 2026-11-24")
	if inf.Carpeta != carpeta {
		t.Fatalf("carpeta = %q", inf.Carpeta)
	}
	for _, f := range []string{
		"Drive/Obras/planos.pdf", "Drive/Fotos/playa.jpg", "Drive/Copias/playa.jpg", "Drive/Fotos/clip.mp4",
		"Drive/Fotos/paseo.webm", "Drive/Fotos/flor.avif", "Drive/Fotos/IMG_0001.heic",
		"account/profile.json", "photos/photos.json", "summary.json", "calendar/events.json", "mail/messages.json",
		"notes/notebooks/Secreto.html", "notes/notebooks/Casa.html", "notes/notes.json",
	} {
		if _, err := os.Stat(filepath.Join(carpeta, filepath.FromSlash(f))); err != nil {
			t.Errorf("no se ha dejado %s: %v", f, err)
		}
	}
	// Lo que entró en su almacén no se copia además.
	for _, f := range []string{
		"calendar/calendars/Casa.ics", "contacts/contacts.vcf", "docs/documents/Carta.docx", "mail/messages/INBOX/1.eml",
		"notes/notebooks/Presupuestos.html",
	} {
		if _, err := os.Stat(filepath.Join(carpeta, filepath.FromSlash(f))); err == nil {
			t.Errorf("%s ya entró en su almacén y se ha copiado además", f)
		}
	}

	// El Hub lo encuentra en cada app.
	s := indice.Leer(datos)
	if len(s.Fallos) != 0 {
		t.Fatal(s.Fallos)
	}
	for consulta, producto := range map[string]string{
		"arquitecto": "calendar", "fontanero reformas": "contacts", "reforma": "notes",
		"derrama": "docs", "presupuesto tejado": "mail", "planos": "pdf", "atardecer": "photos", "verano 2023": "photos",
	} {
		got := s.Buscar(consulta, 10)
		if !slices.ContainsFunc(got, func(e indice.Elemento) bool { return e.Producto == producto }) {
			t.Errorf("«%s» no encuentra nada de %s: %+v", consulta, producto, got)
		}
	}

	// Otra vez: no se repite nada —ni los eventos, los contactos, los documentos y los
	// correos (repetir.go), ni las fotos, las notas y lo que se dejó en Drive—.
	otra, err := traer.Traer(t.Context(), zipRuta, destino(datos))
	if err != nil {
		t.Fatal(err)
	}
	cuenta(t, "otra vez", otra, map[string][5]int{"calendar": {0, 1, 0, 0, 0}, "contacts": {0, 2, 0, 0, 0}, "docs": {0, 1, 0, 0, 0}, "mail": {0, 1, 0, 0, 0}})
	if p := parte(otra, "photos"); p.Importados != 0 || p.Repetidos != 6 {
		t.Errorf("las fotos se repiten: %+v", p)
	}
	if p := parte(otra, "notes"); p.Importados != 0 || p.Repetidos != 4 {
		t.Errorf("las notas se repiten: %+v", p)
	}
	if p := parte(otra, "drive"); p.Importados != 0 || p.YaEnDrive != 4 { // y la tabla de Drive
		t.Errorf("lo de Drive se copia otra vez: %+v", p)
	}
	if _, copiados, _ := otra.Total(); copiados != 0 {
		t.Errorf("la segunda vez se copian %d ficheros tal cual: %+v", copiados, otra.Partes)
	}
	if raw, _ := os.ReadFile(filepath.Join(carpeta, "Drive", "Obras", "planos.pdf")); string(raw) != "%PDF-1.7" {
		t.Error("se ha pisado el primero")
	}
	if repetidos := copiasRepetidas(t, filepath.Join(datos, "Drive")); len(repetidos) > 0 {
		t.Errorf("se ha copiado otra vez a Drive: %v", repetidos)
	}
}

// copiasRepetidas son los ficheros de dir que se llaman «x (2).ext» o así: lo que se
// copió otra vez sin pisar lo que había.
func copiasRepetidas(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(ruta string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), " (2)") {
			out = append(out, ruta)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// HU-07: repetir «Traer» no copia otra vez a Drive lo que ya se trajo, otro día tampoco,
// aunque la persona haya movido o renombrado la carpeta de lo traído (se encuentra y
// Photos recoloca sus enlazadas allí); si la borró, se copia otra vez.
func TestRepetirTraerNoCopiaOtraVezADrive(t *testing.T) {
	ctx := t.Context()
	zipRuta := tusDatos(t, completo(t, "json"))
	datos := t.TempDir()
	enDia := func(dia int) traer.Destino {
		d := destino(datos)
		d.Ahora = func() time.Time { return time.Date(2026, 11, dia, 9, 0, 0, 0, time.UTC) }
		return d
	}
	if _, err := traer.Traer(ctx, zipRuta, enDia(24)); err != nil {
		t.Fatal(err)
	}
	primera := filepath.Join(datos, "Drive", "Traído de Sytue 2026-11-24")

	// Otro día, nada nuevo que dejar: ni siquiera hay carpeta de ese día.
	inf, err := traer.Traer(ctx, zipRuta, enDia(25))
	if err != nil {
		t.Fatal(err)
	}
	if _, copiados, _ := inf.Total(); copiados != 0 || inf.Carpeta != "" {
		t.Errorf("otro día se copia otra vez: %d en %q", copiados, inf.Carpeta)
	}
	if _, enDrive := inf.YaEstaban(); enDrive < 3 {
		t.Errorf("no dice lo que ya estaba en Drive: %+v", inf.Partes)
	}

	// La persona mueve y renombra la carpeta: se encuentra allí y no se copia nada.
	movida := filepath.Join(datos, "Drive", "Archivo", "De Sytue")
	if err := os.MkdirAll(filepath.Dir(movida), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(primera, movida); err != nil {
		t.Fatal(err)
	}
	inf, err = traer.Traer(ctx, zipRuta, enDia(26))
	if err != nil {
		t.Fatal(err)
	}
	if _, copiados, _ := inf.Total(); copiados != 0 || inf.Carpeta != "" {
		t.Errorf("con la carpeta movida se copia otra vez: %d en %q", copiados, inf.Carpeta)
	}
	if p := parte(inf, "drive"); p.Importados != 0 || p.YaEnDrive != 4 {
		t.Errorf("con la carpeta movida, Drive: %+v", p)
	}
	repo, cerrar := fototeca(t, datos)
	linea, _, err := repo.Timeline(ctx, "", 50, false, "")
	cerrar()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range linea {
		if f.Externo != "" && (!strings.HasPrefix(f.Externo, movida) || f.EstadoExterno != "") {
			t.Errorf("%s no se ha recolocado en la carpeta movida: %q %q", f.Name, f.Externo, f.EstadoExterno)
		}
	}

	// La persona la borra: se copia otra vez, y las fotos se recolocan en la nueva.
	if err := os.RemoveAll(movida); err != nil {
		t.Fatal(err)
	}
	inf, err = traer.Traer(ctx, zipRuta, enDia(27))
	if err != nil {
		t.Fatal(err)
	}
	nueva := filepath.Join(datos, "Drive", "Traído de Sytue 2026-11-27")
	if p := parte(inf, "drive"); p.Importados != 3 || p.Copiados != 1 || p.YaEnDrive != 0 || inf.Carpeta != nueva {
		t.Errorf("con la carpeta borrada no se copia otra vez: %+v en %q", p, inf.Carpeta)
	}
	if p := parte(inf, "photos"); p.Importados != 0 || p.Repetidos != 6 {
		t.Errorf("con la carpeta borrada, fotos: %+v", p)
	}
	for _, f := range []string{"Drive/Obras/planos.pdf", "Drive/Fotos/playa.jpg", "Drive/Fotos/paseo.webm", "account/profile.json"} {
		if _, err := os.Stat(filepath.Join(nueva, filepath.FromSlash(f))); err != nil {
			t.Errorf("no se ha vuelto a dejar %s: %v", f, err)
		}
	}
	repo, cerrar = fototeca(t, datos)
	defer cerrar()
	linea, _, err = repo.Timeline(ctx, "", 50, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range linea {
		if f.Externo != "" && (!strings.HasPrefix(f.Externo, nueva) || f.EstadoExterno != "") {
			t.Errorf("%s no se ha recolocado en la carpeta nueva: %q %q", f.Name, f.Externo, f.EstadoExterno)
		}
	}
}

// Un ZIP que no es de «Tus datos», o con rutas que se salen de su carpeta, no se trae.
func TestNoTraeLoQueNoEsDeSytue(t *testing.T) {
	for nombre, ficheros := range map[string]map[string][]byte{
		"sin raíz":    {"summary.json": []byte(`[]`)},
		"dos raíces":  {"a/summary.json": []byte(`[]`), "b/summary.json": []byte(`[]`)},
		"se sale":     {"sytue-one/summary.json": []byte(`[]`), "sytue-one/../../fuera.txt": []byte("x")},
		"absoluta":    {"/etc/sytue-one/summary.json": []byte(`[]`)},
		"desconocido": {"fotos-de-otro/IMG.jpg": []byte("x")},
	} {
		ruta := tusDatos(t, ficheros)
		if _, err := traer.Mirar(ruta); !errors.Is(err, traer.ErrNoEsDeSytue) {
			t.Errorf("%s: Mirar = %v", nombre, err)
		}
		datos := t.TempDir()
		if _, err := traer.Traer(t.Context(), ruta, destino(datos)); !errors.Is(err, traer.ErrNoEsDeSytue) {
			t.Errorf("%s: Traer = %v", nombre, err)
		}
		if es, _ := os.ReadDir(datos); len(es) != 0 {
			t.Errorf("%s: ha escrito en la carpeta de datos", nombre)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(t.TempDir()), "fuera.txt")); err == nil || !strings.Contains(err.Error(), "no such") && !os.IsNotExist(err) {
		t.Error("ha escrito fuera")
	}
}
