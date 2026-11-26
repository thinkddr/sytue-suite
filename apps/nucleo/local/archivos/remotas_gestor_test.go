// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// nubeGestora es nubeFalsa que además gestiona carpetas, con rutas como id (WebDAV).
type nubeGestora struct {
	*nubeFalsa
	carpetas map[string]bool
	tirados  []string
}

var _ nube.Gestor = (*nubeGestora)(nil)

func (n *nubeGestora) Listar(ctx context.Context, carpeta string) ([]nube.Elemento, error) {
	es, err := n.nubeFalsa.Listar(ctx, carpeta)
	n.mu.Lock()
	defer n.mu.Unlock()
	for c := range n.carpetas {
		if path.Dir(c) == carpeta || (carpeta == "" && path.Dir(c) == "/") {
			ya := false
			for _, e := range es {
				ya = ya || e.ID == c
			}
			if !ya {
				es = append(es, nube.Elemento{ID: c, Nombre: path.Base(c), Carpeta: true})
			}
		}
	}
	return es, err
}

func (n *nubeGestora) CrearCarpeta(_ context.Context, padre, nombre string) (nube.Elemento, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := padre + "/" + nombre
	n.carpetas[c] = true
	return nube.Elemento{ID: c, Nombre: nombre, Carpeta: true}, nil
}

func (n *nubeGestora) mover(viejo, nuevo string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for r, d := range n.ficheros {
		if r == viejo || strings.HasPrefix(r, viejo+"/") {
			delete(n.ficheros, r)
			n.ficheros[nuevo+strings.TrimPrefix(r, viejo)] = d
			n.cuando[nuevo+strings.TrimPrefix(r, viejo)] = n.cuando[r]
		}
	}
	for c := range n.carpetas {
		if c == viejo || strings.HasPrefix(c, viejo+"/") {
			delete(n.carpetas, c)
			n.carpetas[nuevo+strings.TrimPrefix(c, viejo)] = true
		}
	}
}

func (n *nubeGestora) Renombrar(_ context.Context, e nube.Elemento, nombre string) (nube.Elemento, error) {
	nuevo := path.Join(path.Dir(e.ID), nombre)
	n.mover(e.ID, nuevo)
	return nube.Elemento{ID: nuevo, Nombre: nombre, Carpeta: e.Carpeta}, nil
}

func (n *nubeGestora) Mover(_ context.Context, e nube.Elemento, destino string) (nube.Elemento, error) {
	nuevo := destino + "/" + path.Base(e.ID)
	n.mover(e.ID, nuevo)
	return nube.Elemento{ID: nuevo, Nombre: path.Base(e.ID), Carpeta: e.Carpeta}, nil
}

func (n *nubeGestora) Borrar(_ context.Context, e nube.Elemento) error {
	n.mu.Lock()
	n.tirados = append(n.tirados, e.ID)
	n.mu.Unlock()
	n.mover(e.ID, "/.papelera"+e.ID)
	return nil
}

func TestUnidadQueGestionaCarpetasCreaRenombraMueveCopiaYTira(t *testing.T) {
	r, falsa := abrirRemotas(t)
	n := &nubeGestora{nubeFalsa: falsa, carpetas: map[string]bool{}}
	r.proveedor = func(string) (nube.Proveedor, error) { return n, nil }
	ctx := t.Context()
	d, err := api.NewDrive("https://sin-servidor.invalid", nil, nil).WithPrivateData(r)
	if err != nil {
		t.Fatal(err)
	}
	unidad := r.Unidades()[0].ID
	raiz, _ := d.Children(ctx, unidad, "")
	informes := raiz.Folders[0].ID
	inf, _ := d.Children(ctx, informes, "")

	// Crear carpeta.
	nueva, err := d.CreateFolder(ctx, informes, "Archivo")
	if err != nil || nueva.Name != "Archivo" || !n.carpetas["/Informes/Archivo"] {
		t.Fatal("crear carpeta", nueva, err)
	}
	if _, err := d.CreateFolder(ctx, informes, "../fuera"); err == nil {
		t.Fatal("aceptó un nombre con ruta")
	}

	// Abrir y cambiar aquí; renombrar sube antes lo pendiente y no lo pierde.
	f, err := d.File(ctx, inf.Files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.Path, []byte("DOCX cambiado"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.Rename(ctx, api.DriveFile, f.ID, "acta.docx"); err != nil {
		t.Fatal("renombrar", err)
	}
	if string(n.ficheros["/Informes/acta.docx"]) != "DOCX cambiado" || n.ficheros["/Informes/a.docx"] != nil {
		t.Fatal("renombrar perdió lo de aquí", n.ficheros)
	}
	if _, err := os.Stat(f.Path); !os.IsNotExist(err) {
		t.Fatal("la caché vieja sigue ahí", err)
	}
	if s := r.Sincronizar(ctx); len(s) != 0 {
		t.Fatal("tras renombrar quedó algo por subir que recrearía el viejo", s)
	}

	// Mover a la carpeta nueva (misma unidad) y luego abrirlo donde está ahora.
	inf, _ = d.Children(ctx, informes, "")
	var acta api.File
	for _, x := range inf.Files {
		if x.Name == "acta.docx" {
			acta = x
		}
	}
	if err := d.Move(ctx, api.DriveFile, acta.ID, nueva.ID); err != nil {
		t.Fatal("mover", err)
	}
	if string(n.ficheros["/Informes/Archivo/acta.docx"]) != "DOCX cambiado" {
		t.Fatal("mover", n.ficheros)
	}
	arch, _ := d.Children(ctx, nueva.ID, "")
	if len(arch.Files) != 1 {
		t.Fatal("la carpeta nueva", arch)
	}
	g, err := d.File(ctx, arch.Files[0].ID)
	if err != nil || filepath.Base(g.Path) != "acta.docx" {
		t.Fatal("abrir tras mover", g, err)
	}
	// Una carpeta no se mete en sí misma.
	if err := d.Move(ctx, api.DriveFolder, raiz.Folders[0].ID, nueva.ID); err == nil {
		t.Fatal("movió una carpeta dentro de sí misma")
	}

	// Copiar deja «acta (2).docx» al lado.
	c, err := d.CopyFile(ctx, arch.Files[0].ID)
	if err != nil || c.Name != "acta (2).docx" || string(n.ficheros["/Informes/Archivo/acta (2).docx"]) != "DOCX cambiado" {
		t.Fatal("copiar", c, err)
	}

	// Tirar va a la papelera del proveedor; la raíz de la unidad no se tira.
	if err := d.Delete(ctx, api.DriveFile, c.ID); err != nil || len(n.tirados) != 1 {
		t.Fatal("tirar", n.tirados, err)
	}
	if err := d.Delete(ctx, api.DriveFolder, unidad); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("tiró la unidad", err)
	}

	// Mover de la nube al disco: copia, comprueba y tira el original a la papelera del
	// proveedor (antes no estaba: «Copiar a…»).
	local, _ := d.Root(ctx)
	if err := d.Move(ctx, api.DriveFile, arch.Files[0].ID, local.ID); err != nil {
		t.Fatal("mover de la nube al disco", err)
	}
	if b, err := os.ReadFile(filepath.Join(r.Raiz(), "acta.docx")); err != nil || string(b) != "DOCX cambiado" {
		t.Fatal("no llegó al disco", string(b), err)
	}
	if n.ficheros["/Informes/Archivo/acta.docx"] != nil || string(n.ficheros["/.papelera/Informes/Archivo/acta.docx"]) != "DOCX cambiado" {
		t.Fatal("el original no fue a la papelera de la nube", n.ficheros)
	}
}

func TestVersionRemotaUsaLaDelProveedor(t *testing.T) {
	a := nube.Elemento{Version: `"e1"`, Tamano: 3}
	b := nube.Elemento{Tamano: 3}
	if versionRemota(a) != `v:"e1"` || strings.HasPrefix(versionRemota(b), "v:") {
		t.Fatal(versionRemota(a), versionRemota(b))
	}
}
