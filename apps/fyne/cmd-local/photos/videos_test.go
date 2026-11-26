// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/carrete"
	"git.sytue.eu/sytue/one/apps/nucleo/local/videoteca"
)

// carreteFalso es la galería de un teléfono: lo que lista y, al abrir, una COPIA en un
// temporal sin extensión, como hace el puente de Android (`sytue-carrete-*`).
type carreteFalso struct {
	elementos []carrete.Elemento
	datos     map[string][]byte
	abiertos  int
}

func (*carreteFalso) Disponible() bool         { return true }
func (*carreteFalso) Permiso() carrete.Permiso { return carrete.Concedido }
func (*carreteFalso) PedirPermiso(context.Context) (carrete.Permiso, error) {
	return carrete.Concedido, nil
}
func (*carreteFalso) RedMedida() bool { return false }
func (*carreteFalso) Guardar(context.Context, string, string) (string, error) {
	return "", carrete.ErrNoDisponible
}

func (*carreteFalso) Convertir(context.Context, string) (string, func(), error) {
	return "", func() {}, carrete.ErrNoDisponible
}

func (g *carreteFalso) Listar(context.Context) ([]carrete.Elemento, error) {
	return append([]carrete.Elemento(nil), g.elementos...), nil
}

func (g *carreteFalso) Abrir(_ context.Context, e carrete.Elemento) (string, func(), error) {
	f, err := os.CreateTemp("", "sytue-carrete-*")
	if err != nil {
		return "", func() {}, err
	}
	_, err = f.Write(g.datos[e.ID])
	_ = f.Close()
	g.abiertos++
	return f.Name(), func() { _ = os.Remove(f.Name()) }, err
}

func leerWebM(t *testing.T) []byte {
	t.Helper()
	webm, err := os.ReadFile("../../video/testdata/dos-segundos.webm")
	if err != nil {
		t.Fatal(err)
	}
	return webm
}

// Del carrete del teléfono entran las fotos y los VÍDEOS (FO-09), COPIADOS (allí no hay
// ruta estable a la que enlazar): el vídeo con su duración y su póster medidos del
// fichero y, si el fichero no se deja medir, con lo que dijo el carrete (MediaStore,
// PhotoKit) y la fecha de la toma. Una segunda ronda no trae nada, por las huellas.
func TestElCarreteTraeLosVideosCopiados(t *testing.T) {
	repo, cli := fototecaDePrueba(t)
	foto, webm := jpegDe(t, 40), leerWebM(t)
	raro := []byte("un vídeo que nadie sabe medir aquí")
	tomado := time.Date(2021, 8, 15, 10, 0, 0, 0, time.UTC)
	g := &carreteFalso{
		elementos: []carrete.Elemento{
			{ID: "11", Nombre: "IMG_1.jpg", Tipo: "image/jpeg", Tomada: tomado, Tamano: int64(len(foto))},
			{ID: "v21", Nombre: "VID_1.webm", Tipo: "video/webm", Tomada: tomado.Add(time.Hour), Tamano: int64(len(webm)), Video: true, Duracion: time.Minute},
			{
				ID: "v22", Nombre: "VID_2.mp4", Tipo: "video/mp4", Tomada: tomado.Add(2 * time.Hour), Tamano: int64(len(raro)), Video: true,
				Duracion: 83500 * time.Millisecond, Ancho: 1920, Alto: 1080,
			},
		},
		datos: map[string][]byte{"11": foto, "v21": webm, "v22": raro},
	}
	abiertos := &elementosAbiertos{}
	copia := photos.NuevaCopia(soloImportables{GaleriaSistema: g, videos: true, abiertos: abiertos}, "")
	destino := func() *destinoLocal {
		d := nuevoDestinoLocal(repo, cli)
		d.medir, d.abiertos = medirVideo, abiertos
		return d
	}
	p, err := copia.Ronda(t.Context(), "local", destino(), nil)
	if err != nil || p.Subidas != 3 || p.Fallidas != 0 {
		t.Fatalf("ronda: %+v %v", p, err)
	}
	linea, _, err := cli.Timeline(t.Context(), "", 100, false, "")
	if err != nil || len(linea) != 3 {
		t.Fatal("la galería debe tener la foto y los dos vídeos:", len(linea), err)
	}
	por := map[string]api.Photo{}
	for _, f := range linea {
		if f.Externo != "" {
			t.Fatalf("lo del carrete se copia, no se enlaza: %+v", f)
		}
		// El motor de la copia les pone delante la fecha de la toma («2021-08-15 12.00.00 …»).
		for _, nombre := range []string{"IMG_1.jpg", "VID_1.webm", "VID_2.mp4"} {
			if strings.HasSuffix(f.Name, " "+nombre) {
				por[nombre] = f
			}
		}
	}
	if v := por["VID_1.webm"]; !v.EsVideo() || v.DurationMs < 1500 || v.DurationMs > 2500 || v.Width == 0 || !v.TakenAt.Equal(tomado.Add(time.Hour)) {
		t.Fatalf("el WebM, medido del fichero y con la fecha de la toma: %+v", v)
	}
	if v := por["VID_2.mp4"]; !v.EsVideo() || v.DurationMs != 83500 || v.Width != 1920 || v.Height != 1080 || v.SizeBytes != int64(len(raro)) {
		t.Fatalf("el MP4 que no se deja medir, con lo que dijo el carrete: %+v", v)
	}
	if f := por["IMG_1.jpg"]; f.EsVideo() || f.ID == "" {
		t.Fatalf("la foto: %+v", f)
	}
	var original bytes.Buffer
	if err := repo.Original(t.Context(), por["VID_1.webm"].ID, &original); err != nil || !bytes.Equal(original.Bytes(), webm) {
		t.Fatal("el vídeo copiado no es el del carrete:", err)
	}
	// Su miniatura sale del fotograma.
	r, err := cli.Variant(t.Context(), por["VID_1.webm"].ID, api.PhotoThumb)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(r); err != nil {
		t.Fatal("la miniatura del vídeo:", err)
	}
	_ = r.Close()
	// Lo apuntado de cada vídeo abierto se olvida al cerrarlo.
	if abiertos.de("cualquiera") != nil {
		t.Fatal("algo se quedó apuntado")
	}
	n := 0
	abiertos.m.Range(func(any, any) bool { n++; return true })
	if n != 0 {
		t.Fatal("quedan vídeos apuntados:", n)
	}
	// Otra ronda, con un registro nuevo: nada, por las huellas.
	if p, err := photos.NuevaCopia(soloImportables{GaleriaSistema: g, videos: true, abiertos: abiertos}, "").Ronda(t.Context(), "local", destino(), nil); err != nil || p.Subidas != 0 {
		t.Fatalf("segunda ronda: %+v %v", p, err)
	}
	// Sin «videos», el carrete los deja fuera, como antes.
	sin := photos.NuevaCopia(soloImportables{GaleriaSistema: g}, "")
	if p, err := sin.Ronda(t.Context(), "local", destino(), nil); err != nil || p.Total != 1 {
		t.Fatalf("sin vídeos: %+v %v", p, err)
	}
}

// «Añadir fotos» y soltar un vídeo (App.ImportarVideo) lo COPIAN a la fototeca con su
// duración y su póster; el mismo otra vez no se duplica; y lo que no es un vídeo que la
// galería enseñe, no entra.
func TestUnVideoElegidoSeCopia(t *testing.T) {
	repo, cli := fototecaDePrueba(t)
	dir := t.TempDir()
	ruta := filepath.Join(dir, "cumple.webm")
	if err := os.WriteFile(ruta, leerWebM(t), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := importarVideoElegido(t.Context(), repo, ruta); err != nil {
			t.Fatal("importar el vídeo:", err)
		}
	}
	linea, _, err := cli.Timeline(t.Context(), "", 100, false, "")
	if err != nil || len(linea) != 1 {
		t.Fatal("un vídeo, una vez:", len(linea), err)
	}
	if v := linea[0]; !v.EsVideo() || v.Externo != "" || v.Name != "cumple.webm" || v.DurationMs < 1500 || v.Width == 0 {
		t.Fatalf("el vídeo copiado: %+v", v)
	}
	txt := filepath.Join(dir, "notas.txt")
	if err := os.WriteFile(txt, []byte("hola"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := importarVideoElegido(t.Context(), repo, txt); err == nil {
		t.Fatal("un texto no es un vídeo")
	}
	if got := extensionesParaAnadir(); len(got) < 10 || got[len(got)-1] != "avi" {
		t.Fatal("«Añadir fotos» debe ofrecer las imágenes y los vídeos:", got)
	}
}

// copiasParaVer da al visor un fichero con el vídeo copiado y su extensión: un enlace al
// original de la fototeca (sin copiarlo) o, si no se puede, una copia; las dos con sus
// bytes. Lo de un vídeo que ya no está en la galería se poda.
func TestUnVideoCopiadoSeVeDesdeUnFicheroConSuExtension(t *testing.T) {
	repo := abrirFototecaDePrueba(t, t.TempDir())
	webm := leerWebM(t)
	fuente := filepath.Join(t.TempDir(), "VID_0001.webm")
	if err := os.WriteFile(fuente, webm, 0o600); err != nil {
		t.Fatal(err)
	}
	v, err := copiarVideo(t.Context(), repo, medirVideo, fuente, "VID_0001.webm", "video/webm", nil)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	for _, enlazar := range []bool{true, false} {
		enClaro := repo.RutaEnClaro
		if !enlazar {
			enClaro = func(string) (string, bool) { return "", false }
		}
		dir := filepath.Join(cache, map[bool]string{true: "enlace", false: "copia"}[enlazar])
		c := nuevasCopiasParaVer(dir, repo, enClaro)
		ruta, err := c.ruta(t.Context(), v)
		if err != nil {
			t.Fatal("la ruta para verlo:", err)
		}
		if filepath.Ext(ruta) != ".webm" {
			t.Fatal("sin su extensión:", ruta)
		}
		if en, err := os.ReadFile(ruta); err != nil || !bytes.Equal(en, webm) {
			t.Fatal("lo que se ve no es el vídeo:", err)
		}
		info, err := os.Lstat(ruta)
		if err != nil {
			t.Fatal(err)
		}
		if enlace := info.Mode()&os.ModeSymlink != 0; enlazar && runtime.GOOS != "windows" && !enlace {
			t.Fatal("con el original en claro debe ser un enlace, no una copia")
		} else if !enlazar && enlace {
			t.Fatal("sin el original en claro tiene que ser una copia")
		}
		// La segunda vez, la misma.
		if otra, err := c.ruta(t.Context(), v); err != nil || otra != ruta {
			t.Fatal("la segunda vez:", otra, err)
		}
	}
	if err := repo.Trash(t.Context(), v.ID); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"enlace", "copia"} {
		c := nuevasCopiasParaVer(filepath.Join(cache, sub), repo, repo.RutaEnClaro)
		c.podar(t.Context())
		if entradas, _ := os.ReadDir(filepath.Join(cache, sub)); len(entradas) != 0 {
			t.Fatal("lo de un vídeo en la papelera sigue en la caché:", sub, entradas)
		}
	}
}

// «Abrir en Cast» con un vídeo COPIADO a la fototeca: Cast no lee la caché de Photos (que
// además se poda), así que el vídeo se le deja en la carpeta de sus vídeos, que su
// videoteca recoge como propia: un enlace duro al original de la fototeca (o una copia si no
// se puede), con su nombre. Pulsarlo otra vez da el mismo fichero; otro vídeo con el mismo
// nombre va a «nombre (2)»; y que la fototeca de Photos suelte el original (borrado para
// siempre) no se lo lleva a Cast. Sin Cast instalado se da el fichero de
// verlo aquí, sin dejar nada en su carpeta, y uno enlazado va tal cual.
func TestAbrirEnCastDejaElVideoCopiadoEnLaCarpetaDeCast(t *testing.T) {
	ctx := t.Context()
	repo := abrirFototecaDePrueba(t, t.TempDir())
	copiar := func(datos []byte) api.Photo {
		t.Helper()
		fuente := filepath.Join(t.TempDir(), "cumple.webm")
		if err := os.WriteFile(fuente, datos, 0o600); err != nil {
			t.Fatal(err)
		}
		v, err := copiarVideo(ctx, repo, medirVideo, fuente, "cumple.webm", "video/webm", nil)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	webm := leerWebM(t)
	vp8, err := os.ReadFile("../../video/testdata/vp8.webm")
	if err != nil {
		t.Fatal(err)
	}
	v, otro := copiar(webm), copiar(vp8)

	antes := hayApp
	t.Cleanup(func() { hayApp = antes })
	hayApp = func(string) (bool, bool) { return true, true }
	carpeta := videoteca.CarpetaEn(t.TempDir())
	copias := nuevasCopiasParaVer(filepath.Join(t.TempDir(), "photos-videos"), repo, repo.RutaEnClaro)
	p := &reproductorFoto{foto: v, copias: copias, carpetaCast: carpeta}
	ruta, err := p.rutaParaCast()
	if err != nil || ruta != filepath.Join(carpeta, "cumple.webm") {
		t.Fatalf("para Cast: %q %v", ruta, err)
	}
	if en, err := os.ReadFile(ruta); err != nil || !bytes.Equal(en, webm) {
		t.Fatal("lo que recibe Cast no es el vídeo:", err)
	}
	huella, err := repo.HuellaOriginal(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	origen, ok := repo.RutaEnClaro(huella)
	if !ok {
		t.Fatal("sin el original en claro")
	}
	a, _ := os.Stat(ruta)
	b, _ := os.Stat(origen)
	if os.SameFile(a, b) {
		t.Fatal("lo de Cast no puede ser el MISMO fichero que el original de la fototeca (un enlace duro)")
	}
	if otra, err := p.rutaParaCast(); err != nil || otra != ruta {
		t.Fatal("la segunda vez:", otra, err)
	}
	// Un programa que reescribe en su sitio el fichero de Cast no toca el original.
	if fd, err := os.OpenFile(ruta, os.O_WRONLY, 0); err != nil {
		t.Fatal(err)
	} else {
		_, err = fd.WriteAt([]byte("XXXX"), 0)
		if cerr := fd.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if en, err := os.ReadFile(origen); err != nil || !bytes.Equal(en, webm) {
		t.Fatal("escribir en el vídeo de Cast cambió el original de Photos:", err)
	}
	if err := os.WriteFile(ruta, webm, 0o600); err != nil {
		t.Fatal(err)
	}
	r2, err := (&reproductorFoto{foto: otro, copias: copias, carpetaCast: carpeta}).rutaParaCast()
	if err != nil || r2 != filepath.Join(carpeta, "cumple (2).webm") {
		t.Fatalf("otro vídeo con el mismo nombre: %q %v", r2, err)
	}
	if en, err := os.ReadFile(r2); err != nil || !bytes.Equal(en, vp8) {
		t.Fatal("el otro vídeo no es el suyo:", err)
	}

	// Sin el original en claro (otro disco), una copia, sin temporales a la vista.
	carpeta2 := videoteca.CarpetaEn(t.TempDir())
	sinEnlace := nuevasCopiasParaVer(filepath.Join(t.TempDir(), "photos-videos"), repo, func(string) (string, bool) { return "", false })
	r3, err := sinEnlace.paraCast(ctx, v, carpeta2)
	if err != nil || filepath.Base(r3) != "cumple.webm" {
		t.Fatalf("copia: %q %v", r3, err)
	}
	c, _ := os.Stat(r3)
	if en, err := os.ReadFile(r3); err != nil || !bytes.Equal(en, webm) || os.SameFile(c, b) {
		t.Fatal("sin enlace tiene que ser una copia con sus bytes:", err)
	}
	if entradas, _ := os.ReadDir(carpeta2); len(entradas) != 1 {
		t.Fatal("en la carpeta de Cast queda algo más que el vídeo:", entradas)
	}

	// Sin Cast instalado: el fichero de verlo aquí, y nada nuevo en su carpeta.
	hayApp = func(string) (bool, bool) { return false, true }
	if r, err := (&reproductorFoto{foto: v, copias: copias, carpetaCast: carpeta2}).rutaParaCast(); err != nil || filepath.Dir(r) == carpeta2 {
		t.Fatalf("sin Cast: %q %v", r, err)
	}
	// Uno enlazado, tal cual, haya Cast o no.
	hayApp = func(string) (bool, bool) { return true, true }
	enlazado := api.Photo{ID: "e1", Name: "fiesta.mp4", Externo: "/Vídeos/fiesta.mp4"}
	if r, err := (&reproductorFoto{foto: enlazado, copias: copias, carpetaCast: carpeta}).rutaParaCast(); err != nil || r != enlazado.Externo {
		t.Fatalf("enlazado: %q %v", r, err)
	}

	// La fototeca de Photos suelta el original: Cast sigue teniendo el suyo.
	if err := os.Remove(origen); err != nil {
		t.Fatal(err)
	}
	if en, err := os.ReadFile(ruta); err != nil || !bytes.Equal(en, webm) {
		t.Fatal("al soltarlo Photos, Cast se queda sin el vídeo:", err)
	}

	// El nombre: sin carpetas ni lo que no acepta un sistema de ficheros, sin ocultarse,
	// y con la extensión de su tipo si no la trae.
	for nombre, quiero := range map[string][2]string{
		"../viaje/.Día 1: playa.MP4": {"Día 1_ playa", ".MP4"},
		"sin extensión":              {"sin extensión", ".webm"},
		"":                           {"v9", ".webm"},
	} {
		base, ext := nombreParaCast(api.Photo{ID: "v9", Name: nombre, ContentType: "video/webm"})
		if base != quiero[0] || ext != quiero[1] {
			t.Errorf("%q → %q %q", nombre, base, ext)
		}
	}
}
