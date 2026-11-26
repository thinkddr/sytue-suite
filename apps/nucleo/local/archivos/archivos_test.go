// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func abrirPrueba(t *testing.T) (*Archivos, string) {
	t.Helper()
	base := t.TempDir()
	a, err := Abrir(filepath.Join(base, "Drive"), Opciones{Instalacion: "inst-1", Autor: "Ana", MaxVersiones: 3})
	if err != nil {
		t.Fatal(err)
	}
	return a, base
}

func leer(rc io.ReadCloser, err error) func(*testing.T) string {
	return func(t *testing.T) string { return leerYa(t, rc, err) }
}

func leerYa(t *testing.T, rc io.ReadCloser, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFachadaPDFSobreLaRaiz(t *testing.T) {
	a, _ := abrirPrueba(t)
	pdf, err := api.NewPDF("https://docs.invalid", "https://drive.invalid", nil, nil).WithPrivateDrive(a)
	if err != nil {
		t.Fatal("el puerto no sirve como DriveData privado", err)
	}
	f, err := pdf.Subir(t.Context(), "contrato.pdf", strings.NewReader("%PDF-1.4 uno"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(a.Raiz(), "PDF", "contrato.pdf")); err != nil {
		t.Fatal("Subir no dejó el fichero en la carpeta PDF", err)
	}
	por, err := pdf.PorRuta(t.Context(), "/PDF/contrato.pdf")
	if err != nil || por.ID != f.ID || por.Checksum == "" {
		t.Fatal("la ruta de Drive no resuelve", por, err)
	}
	// Guardar una copia al lado no toca el original.
	c, err := pdf.GuardarCopia(t.Context(), por, "girado", []byte("%PDF-1.4 dos"))
	if err != nil || c.Name != "contrato (girado).pdf" {
		t.Fatal("copia", c, err)
	}
	lista, err := pdf.Lista(t.Context())
	// El orden es el más reciente primero (abierto o modificado); la copia se escribe
	// después de abrir el original y, según la resolución del reloj del sistema de
	// ficheros, puede empatar con él: aquí solo se exige que estén los dos.
	if err != nil || len(lista) != 2 || !slices.ContainsFunc(lista, func(f api.File) bool { return f.Name == "contrato.pdf" }) ||
		!slices.ContainsFunc(lista, func(f api.File) bool { return f.Name == "contrato (girado).pdf" }) {
		t.Fatal("la portada debe listar los dos PDF", lista, err)
	}
	if a.PDFAuthorName() != "Ana" {
		t.Fatal("autor")
	}
}

func TestFicheroSueltoNoSeCopiaYSeGuardaEnSuSitioConVersion(t *testing.T) {
	a, base := abrirPrueba(t)
	fuera := filepath.Join(base, "Escritorio")
	if err := os.MkdirAll(fuera, 0o755); err != nil {
		t.Fatal(err)
	}
	suelto := filepath.Join(fuera, "factura.pdf")
	if err := os.WriteFile(suelto, []byte("%PDF-1.4 original"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Antes de abrirlo, no es accesible.
	if _, err := a.File(t.Context(), idDe(suelto)); !errors.Is(err, ErrFuera) {
		t.Fatal("un fichero no abierto no debe ser accesible", err)
	}
	f, err := a.FileByPath(t.Context(), suelto)
	if err != nil || f.Path != suelto {
		t.Fatal(err, f)
	}
	// No se copió a la raíz.
	var copias int
	_ = filepath.WalkDir(a.Raiz(), func(_ string, e os.DirEntry, _ error) error {
		if e != nil && e.Name() == "factura.pdf" {
			copias++
		}
		return nil
	})
	if copias != 0 {
		t.Fatal("el fichero suelto se copió dentro de la raíz")
	}

	nuevo, err := a.ReplaceFile(t.Context(), f, "application/pdf", strings.NewReader("%PDF-1.4 anotado"), 16)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(suelto); string(b) != "%PDF-1.4 anotado" || nuevo.Version != 2 {
		t.Fatal("no se guardó en su sitio", string(b), nuevo.Version)
	}
	if fi, _ := os.Stat(suelto); fi.Mode().Perm() != 0o640 {
		t.Fatal("se perdieron los permisos", fi.Mode())
	}
	// Sin temporales a la vista.
	es, _ := os.ReadDir(fuera)
	if len(es) != 1 {
		t.Fatal("quedó basura junto al fichero", es)
	}
	vs, _, err := a.Versions(t.Context(), f.ID, "")
	if err != nil || len(vs) != 2 || !vs[0].IsCurrent || vs[1].Version != 1 {
		t.Fatal("versiones", vs, err)
	}
	if got := leer(a.DownloadVersion(t.Context(), f.ID, 1))(t); got != "%PDF-1.4 original" {
		t.Fatal("la versión anterior no se guardó", got)
	}
	if !strings.HasPrefix(a.rutaVersion(suelto, 1), filepath.Join(a.Raiz(), Meta, "versiones")) {
		t.Fatal("las versiones deben ir a .sytue/versiones")
	}

	// Guardar con el original viejo (otra ventana) se rechaza.
	if _, err := a.ReplaceFile(t.Context(), f, "application/pdf", strings.NewReader("x"), 1); !errors.Is(err, ErrDesfasado) {
		t.Fatal("un guardado desfasado debe fallar", err)
	}
	// Restaurar la 1 deja la anotada como versión.
	if err := a.RestoreVersion(t.Context(), f.ID, 1); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(suelto); string(b) != "%PDF-1.4 original" {
		t.Fatal("no se restauró", string(b))
	}
	vs, _, _ = a.Versions(t.Context(), f.ID, "")
	if len(vs) != 3 || vs[0].Version != 3 {
		t.Fatal("restaurar debe sumar una versión", vs)
	}
	// La copia de una operación va a su lado, en la carpeta del suelto.
	c, err := a.Upload(t.Context(), f.FolderID, "factura (firmado).pdf", "application/pdf", bytes.NewReader([]byte("%PDF-1.4 f")), 10)
	if err != nil || filepath.Dir(c.Path) != fuera {
		t.Fatal("copia al lado del suelto", c, err)
	}
	// Recientes y portada.
	rs, _, _ := a.Recent(t.Context(), "")
	if len(rs) != 1 || rs[0].Path != suelto {
		t.Fatal("recientes", rs)
	}
	ls, _ := a.ListPDFs(t.Context())
	if len(ls) != 1 || ls[0].Path != suelto {
		t.Fatal("portada", ls)
	}
	// El índice persiste: al reabrir sigue siendo accesible.
	b, err := Abrir(a.Raiz(), Opciones{Instalacion: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.File(t.Context(), f.ID); err != nil {
		t.Fatal("el reciente se olvidó al reabrir", err)
	}
}

func TestTopeDeVersiones(t *testing.T) {
	a, _ := abrirPrueba(t)
	f, err := a.Upload(t.Context(), idDe(a.Raiz()), "n.txt", "text/plain", strings.NewReader("0"), 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 6; i++ {
		s := strings.Repeat("x", i)
		if f, err = a.ReplaceFile(t.Context(), f, "text/plain", strings.NewReader(s), int64(len(s))); err != nil {
			t.Fatal(err)
		}
	}
	vs, _, _ := a.Versions(t.Context(), f.ID, "")
	if len(vs) != 4 || vs[0].Version != 7 || vs[3].Version != 4 {
		t.Fatal("tope de versiones", vs)
	}
	es, _ := os.ReadDir(a.dirVersiones(f.Path))
	if len(es) != 4 { // 3 versiones + historial.json
		t.Fatal("quedaron versiones viejas en disco", len(es))
	}
}

func TestRenombrarMoverPapeleraYBuscar(t *testing.T) {
	a, _ := abrirPrueba(t)
	raiz := idDe(a.Raiz())
	c, err := a.CreateFolder(t.Context(), raiz, "Facturas")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateFolder(t.Context(), raiz, "Facturas"); !errors.Is(err, ErrExiste) {
		t.Fatal("carpeta repetida", err)
	}
	if _, err := a.CreateFolder(t.Context(), raiz, "../fuera"); !errors.Is(err, ErrNombre) {
		t.Fatal("nombre con separadores", err)
	}
	f, err := a.Upload(t.Context(), raiz, "a.pdf", "", strings.NewReader("uno"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if f, err = a.ReplaceFile(t.Context(), f, "", strings.NewReader("dos"), 3); err != nil {
		t.Fatal(err)
	}
	if err := a.Rename(t.Context(), api.DriveFile, f.ID, "b.pdf"); err != nil {
		t.Fatal(err)
	}
	// El identificador viejo sigue valiendo y el historial se fue con el fichero.
	g, err := a.File(t.Context(), f.ID)
	if err != nil || g.Name != "b.pdf" || g.Version != 2 {
		t.Fatal("tras renombrar", g, err)
	}
	if err := a.Move(t.Context(), api.DriveFile, g.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	h, err := a.File(t.Context(), f.ID)
	if err != nil || filepath.Base(filepath.Dir(h.Path)) != "Facturas" {
		t.Fatal("tras mover", h, err)
	}
	if got := leer(a.DownloadVersion(t.Context(), h.ID, 1))(t); got != "uno" {
		t.Fatal("la versión no siguió al fichero", got)
	}
	r, err := a.Search(t.Context(), "B.PD")
	if err != nil || len(r.Files) != 1 {
		t.Fatal("buscar", r, err)
	}
	ch, _ := a.Children(t.Context(), raiz, "")
	for _, x := range ch.Folders {
		if x.Name == Meta {
			t.Fatal(".sytue no debe listarse")
		}
	}
	cp, err := a.CopyFile(t.Context(), h.ID)
	if err != nil || cp.Name != "b (2).pdf" {
		t.Fatal("copiar", cp, err)
	}
	if err := a.Delete(t.Context(), api.DriveFile, h.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(h.Path); !os.IsNotExist(err) {
		t.Fatal("borrar debe quitarlo de su sitio")
	}
	tr, _ := a.Trash(t.Context())
	if len(tr.Files) != 1 || tr.Files[0].DeletedAt == nil {
		t.Fatal("papelera", tr)
	}
	if err := a.Restore(t.Context(), api.DriveFile, tr.Files[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := leer(a.Download(t.Context(), h.ID))(t); got != "dos" {
		t.Fatal("restaurar de la papelera", got)
	}
	if _, err := a.File(t.Context(), idDe(filepath.Join(a.Raiz(), Meta, "indice.json"))); err == nil {
		t.Fatal(".sytue no debe ser accesible")
	}
	if _, err := a.FileByPath(t.Context(), "/no/existe.pdf"); err == nil {
		t.Fatal("un fichero que no existe")
	}
}
