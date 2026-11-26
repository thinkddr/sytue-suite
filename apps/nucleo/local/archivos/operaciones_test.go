// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"archive/zip"
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func escribirPrueba(t *testing.T, r, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(r), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

// La fachada de la suite, enchufada al disco: lo que antes fallaba cerrado ahora va.
func fachada(t *testing.T, a *Archivos) *api.Drive {
	t.Helper()
	d, err := api.NewDrive("https://sin-servidor.invalid", nil, nil).WithPrivateData(a)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestPurgarBorraDeVerdadSoloLoDeLaPapelera(t *testing.T) {
	a, _ := abrirPrueba(t)
	d := fachada(t, a)
	r := filepath.Join(a.Raiz(), "nota.txt")
	escribirPrueba(t, r, "uno")
	id := idDe(r)
	if _, err := a.Upload(t.Context(), idDe(a.Raiz()), "nota.txt", "", strings.NewReader("dos"), 3); err != nil {
		t.Fatal(err)
	}
	if err := d.Purge(t.Context(), api.DriveFile, id); err == nil {
		t.Fatal("algo que no está en la papelera no se purga")
	}
	if err := d.Delete(t.Context(), api.DriveFile, id); err != nil {
		t.Fatal(err)
	}
	if err := d.Purge(t.Context(), api.DriveFile, id); err != nil {
		t.Fatal(err)
	}
	tr, _ := d.Trash(t.Context())
	if len(tr.Files) != 0 {
		t.Fatal("sigue en la papelera", tr.Files)
	}
	if es, _ := os.ReadDir(filepath.Join(a.meta(), "papelera")); len(es) != 0 {
		t.Fatal("quedó contenido en la papelera", es)
	}
	if _, err := os.Stat(a.dirVersiones(r)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("el historial de versiones debe irse con él", err)
	}
	if err := d.Restore(t.Context(), api.DriveFile, id); err == nil {
		t.Fatal("lo purgado no se restaura")
	}
}

func TestCopiarCarpetaConSuArbol(t *testing.T) {
	a, _ := abrirPrueba(t)
	d := fachada(t, a)
	base := filepath.Join(a.Raiz(), "Proyecto")
	escribirPrueba(t, filepath.Join(base, "a.txt"), "a")
	escribirPrueba(t, filepath.Join(base, "sub", "b.txt"), "b")
	escribirPrueba(t, filepath.Join(base, ".oculto"), "x")
	c, err := d.CopyFolder(t.Context(), idDe(base))
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Proyecto (2)" {
		t.Fatal("nombre de la copia", c.Name)
	}
	if b, err := os.ReadFile(filepath.Join(a.Raiz(), "Proyecto (2)", "sub", "b.txt")); err != nil || string(b) != "b" {
		t.Fatal("el subárbol no se copió", string(b), err)
	}
	if _, err := os.Stat(filepath.Join(a.Raiz(), "Proyecto (2)", ".oculto")); err == nil {
		t.Fatal("lo oculto no se copia")
	}
}

func TestCopiarFueraNoPisaYNoSeMeteEnSiMisma(t *testing.T) {
	a, base := abrirPrueba(t)
	fuera := filepath.Join(base, "Escritorio")
	if err := os.Mkdir(fuera, 0o755); err != nil {
		t.Fatal(err)
	}
	r := filepath.Join(a.Raiz(), "informe.txt")
	escribirPrueba(t, r, "hola")
	escribirPrueba(t, filepath.Join(fuera, "informe.txt"), "otro")
	got, err := a.CopiarFuera(t.Context(), idDe(r), fuera)
	if err != nil || filepath.Base(got) != "informe (2).txt" {
		t.Fatal("copia", got, err)
	}
	if b, _ := os.ReadFile(filepath.Join(fuera, "informe.txt")); string(b) != "otro" {
		t.Fatal("pisó lo que había")
	}
	carpeta := filepath.Join(a.Raiz(), "C")
	escribirPrueba(t, filepath.Join(carpeta, "x.txt"), "x")
	if _, err := a.CopiarFuera(t.Context(), idDe(carpeta), carpeta); err == nil {
		t.Fatal("una carpeta no se copia dentro de sí misma")
	}
	got, err = a.CopiarFuera(t.Context(), idDe(carpeta), fuera)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(got, "x.txt")); string(b) != "x" {
		t.Fatal("la carpeta no se copió entera")
	}
}

// «Copiar a…» en el teléfono: el destino es una carpeta elegida con el selector, un árbol
// de documentos y no una ruta del disco. Un fichero y una carpeta, sin pisar lo que hay.
func TestCopiarFueraEnUnArbolDeDocumentos(t *testing.T) {
	if os.Getenv(sobreArbol) != "" {
		t.Skip("el origen ya es un árbol")
	}
	a, _ := abrirPrueba(t)
	destino := NuevoArbol(raizVirtual, nuevaMemoria())
	r := filepath.Join(a.Raiz(), "informe.txt")
	escribirPrueba(t, r, "hola")
	escribirEn(t, destino, filepath.Join(raizVirtual, "informe.txt"), "otro")
	got, err := a.CopiarFueraEn(t.Context(), idDe(r), destino, raizVirtual)
	if err != nil || got != filepath.Join(raizVirtual, "informe (2).txt") {
		t.Fatal("copia", got, err)
	}
	if leerDe(t, destino, got) != "hola" || leerDe(t, destino, filepath.Join(raizVirtual, "informe.txt")) != "otro" {
		t.Fatal("pisó lo que había o no copió")
	}
	carpeta := filepath.Join(a.Raiz(), "C")
	escribirPrueba(t, filepath.Join(carpeta, "sub", "x.txt"), "x")
	escribirPrueba(t, filepath.Join(carpeta, ".oculto"), "no")
	got, err = a.CopiarFueraEn(t.Context(), idDe(carpeta), destino, raizVirtual)
	if err != nil || got != filepath.Join(raizVirtual, "C") {
		t.Fatal("carpeta", got, err)
	}
	if leerDe(t, destino, filepath.Join(got, "sub", "x.txt")) != "x" {
		t.Fatal("la carpeta no se copió entera")
	}
	if _, err := destino.Stat(filepath.Join(got, ".oculto")); err == nil {
		t.Fatal("lo oculto no se copia")
	}
	if _, err := a.CopiarFueraEn(t.Context(), idDe(r), destino, filepath.Join(raizVirtual, "no-esta")); err == nil {
		t.Fatal("un destino que no está")
	}
}

func TestLotes(t *testing.T) {
	a, _ := abrirPrueba(t)
	d := fachada(t, a)
	destino := filepath.Join(a.Raiz(), "Destino")
	if err := os.Mkdir(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	f1, f2 := filepath.Join(a.Raiz(), "1.txt"), filepath.Join(a.Raiz(), "2.txt")
	escribirPrueba(t, f1, "1")
	escribirPrueba(t, f2, "2")
	c := filepath.Join(a.Raiz(), "Carpeta")
	escribirPrueba(t, filepath.Join(c, "dentro.txt"), "d")
	lote, err := d.BatchMove(t.Context(), []string{idDe(f1), idDe(f2), idDe(filepath.Join(a.Raiz(), "no-existe"))}, []string{idDe(c)}, idDe(destino))
	if err != nil || lote.Ficheros != 2 || lote.Carpetas != 1 {
		t.Fatal("mover en lote", lote, err)
	}
	if _, err := os.Stat(filepath.Join(destino, "Carpeta", "dentro.txt")); err != nil {
		t.Fatal(err)
	}
	// El identificador viejo sigue valiendo tras moverlo, y «» es la raíz.
	lote, err = d.BatchMove(t.Context(), []string{idDe(f1)}, nil, "")
	if err != nil || lote.Total() != 1 {
		t.Fatal("volver a la raíz", lote, err)
	}
	lote, err = d.BatchTrash(t.Context(), []string{idDe(f1)}, []string{idDe(filepath.Join(destino, "Carpeta"))})
	if err != nil || lote.Total() != 2 {
		t.Fatal("tirar en lote", lote, err)
	}
	tr, _ := d.Trash(t.Context())
	if len(tr.Files) != 1 || len(tr.Folders) != 1 {
		t.Fatal("papelera", tr)
	}
	if _, err := d.BatchTrash(t.Context(), []string{"basura"}, nil); err == nil {
		t.Fatal("si no se tira nada, se dice por qué")
	}
}

func TestCuotaEsElDisco(t *testing.T) {
	a, _ := abrirPrueba(t)
	q, err := fachada(t, a).Quota(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if q.LimitBytes <= 0 || q.AvailableBytes < 0 || q.UsedBytes+q.AvailableBytes > q.LimitBytes {
		t.Fatal("cuota sin sentido", q)
	}
}

func TestZipEnElDispositivo(t *testing.T) {
	a, _ := abrirPrueba(t)
	r := filepath.Join(a.Raiz(), "suelto.txt")
	escribirPrueba(t, r, "suelto")
	c := filepath.Join(a.Raiz(), "Fotos")
	escribirPrueba(t, filepath.Join(c, "uno.txt"), "uno")
	escribirPrueba(t, filepath.Join(c, "sub", "dos.txt"), "dos")
	escribirPrueba(t, filepath.Join(c, ".DS_Store"), "x")
	rc, n, err := fachada(t, a).Zip(t.Context(), []string{idDe(r)}, []string{idDe(c)}, "")
	if err != nil {
		t.Fatal(err)
	}
	datos, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len("suelto")+len("uno")+len("dos")) {
		t.Fatal("contenido sin comprimir", n)
	}
	zr, err := zip.NewReader(bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatal(err)
	}
	nombres := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		nombres = append(nombres, f.Name)
	}
	slices.Sort(nombres)
	if !slices.Equal(nombres, []string{"Fotos/sub/dos.txt", "Fotos/uno.txt", "suelto.txt"}) {
		t.Fatal("entradas del ZIP", nombres)
	}
}

func TestMiniaturaDeUnaImagen(t *testing.T) {
	a, _ := abrirPrueba(t)
	img := image.NewNRGBA(image.Rect(0, 0, 800, 400))
	for x := range 800 {
		img.Set(x, 10, color.NRGBA{R: 255, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	r := filepath.Join(a.Raiz(), "foto.png")
	escribirPrueba(t, r, b.String())
	rc, err := a.Thumbnail(t.Context(), idDe(r))
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	cfg, _, err := image.DecodeConfig(rc)
	if err != nil || cfg.Width != LadoMiniatura || cfg.Height != LadoMiniatura/2 {
		t.Fatal("miniatura", cfg, err)
	}
	txt := filepath.Join(a.Raiz(), "nota.txt")
	escribirPrueba(t, txt, "x")
	if _, err := a.Thumbnail(t.Context(), idDe(txt)); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("un texto no tiene miniatura", err)
	}
}

func TestCrearDocumentoNativo(t *testing.T) {
	a, _ := abrirPrueba(t)
	for k, ext := range ExtensionDe {
		f, err := a.CrearDocumento(t.Context(), "", k, "Sin título")
		if err != nil {
			t.Fatal(k, err)
		}
		if f.Name != "Sin título"+ext || f.SizeBytes == 0 {
			t.Fatal(k, f)
		}
		datos, _ := os.ReadFile(f.Path)
		// Lo abre la ofimática local: el mismo importador que usan Docus, Calcus y Diapos.
		if imp, err := biblioteca.Importar(f.Name, datos, false); err != nil || imp.Kind != k {
			t.Fatal("la ofimática no lo reconoce", k, imp.Kind, err)
		}
		// El segundo no pisa el primero.
		g, err := a.CrearDocumento(t.Context(), "", k, "Sin título")
		if err != nil || g.Name != "Sin título (2)"+ext {
			t.Fatal("segundo", g, err)
		}
	}
}

// Sin las interfaces opcionales, la fachada sigue fallando cerrada como antes.
func TestSinInterfacesOpcionalesFallaCerrado(t *testing.T) {
	d, err := api.NewDrive("https://sin-servidor.invalid", nil, nil).WithPrivateData(soloDatos{})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Purge(t.Context(), api.DriveFile, "x"); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("Purge", err)
	}
	if _, err := d.Quota(t.Context()); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("Quota", err)
	}
	if _, err := d.BatchMove(t.Context(), []string{"x"}, nil, ""); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("BatchMove", err)
	}
	if _, err := d.CopyFolder(t.Context(), "x"); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("CopyFolder", err)
	}
}

// soloDatos es un DriveData mínimo, sin ninguna interfaz opcional.
type soloDatos struct{ api.DriveData }

func (soloDatos) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "t", Subject: "s", Product: "drive"}
}
