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
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/local/nube"
)

// nubeFalsa es un proveedor en memoria con rutas como identificadores (como WebDAV).
type nubeFalsa struct {
	mu       sync.Mutex
	ficheros map[string][]byte
	cuando   map[string]time.Time
	bajadas  int
}

func (n *nubeFalsa) Listar(_ context.Context, carpeta string) ([]nube.Elemento, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []nube.Elemento
	vistas := map[string]bool{}
	for r, d := range n.ficheros {
		dir, nombre := path.Split(r)
		dir = strings.TrimSuffix(dir, "/")
		switch {
		case dir == carpeta:
			out = append(out, nube.Elemento{ID: r, Nombre: nombre, Tamano: int64(len(d)), Modificado: n.cuando[r]})
		case strings.HasPrefix(dir, carpeta+"/"):
			sub := carpeta + "/" + strings.SplitN(strings.TrimPrefix(dir, carpeta+"/"), "/", 2)[0]
			if !vistas[sub] {
				vistas[sub] = true
				out = append(out, nube.Elemento{ID: sub, Nombre: path.Base(sub), Carpeta: true})
			}
		}
	}
	return out, nil
}

func (n *nubeFalsa) Descargar(_ context.Context, e nube.Elemento) ([]byte, string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.bajadas++
	d, ok := n.ficheros[e.ID]
	if !ok {
		return nil, "", errors.New("no está")
	}
	return append([]byte(nil), d...), e.Nombre, nil
}

func (n *nubeFalsa) Subir(_ context.Context, carpeta, nombre string, datos []byte) (nube.Elemento, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	r := carpeta + "/" + nombre
	n.ficheros[r] = append([]byte(nil), datos...)
	n.cuando[r] = n.cuando[r].Add(time.Minute).Add(time.Duration(len(n.cuando)) * time.Second)
	return nube.Elemento{ID: r, Nombre: nombre}, nil
}

func (n *nubeFalsa) cambiarFuera(r, contenido string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ficheros[r] = []byte(contenido)
	n.cuando[r] = n.cuando[r].Add(time.Hour)
}

func abrirRemotas(t *testing.T) (*Remotas, *nubeFalsa) {
	t.Helper()
	a, base := abrirPrueba(t)
	t0 := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	n := &nubeFalsa{
		ficheros: map[string][]byte{"/Informes/a.docx": []byte("DOCX v1"), "/Informes/Fotos/b.png": []byte("PNG")},
		cuando:   map[string]time.Time{"/Informes/a.docx": t0, "/Informes/Fotos/b.png": t0},
	}
	cuentas := func() []nube.Cuenta { return []nube.Cuenta{{ID: "c1", Tipo: nube.WebDAV, Rotulo: "Nextcloud de casa"}} }
	r := ConRemotas(a, cuentas, func(id string) (nube.Proveedor, error) {
		if id != "c1" {
			return nil, nube.ErrSinCuenta
		}
		return n, nil
	}, filepath.Join(base, "cache"))
	return r, n
}

func TestUnidadRemotaSeNavegaAbreYSube(t *testing.T) {
	r, n := abrirRemotas(t)
	ctx := t.Context()
	us := r.Unidades()
	if len(us) != 1 || us[0].Nombre != "Nextcloud de casa" || !EsRemoto(us[0].ID) {
		t.Fatal("unidades", us)
	}
	d, err := api.NewDrive("https://sin-servidor.invalid", nil, nil).WithPrivateData(r)
	if err != nil {
		t.Fatal(err)
	}
	raiz, err := d.Folder(ctx, us[0].ID)
	if err != nil || raiz.Name != "Nextcloud de casa" || raiz.ParentID != nil {
		t.Fatal("raíz de la unidad", raiz, err)
	}
	hijos, err := d.Children(ctx, us[0].ID, "")
	if err != nil || len(hijos.Folders) != 1 || hijos.Folders[0].Name != "Informes" {
		t.Fatal("raíz", hijos, err)
	}
	inf, err := d.Children(ctx, hijos.Folders[0].ID, "")
	if err != nil || len(inf.Files) != 1 || len(inf.Folders) != 1 {
		t.Fatal("Informes", inf, err)
	}
	// Abrir baja a la caché y da la ruta; una segunda vez no vuelve a bajar.
	f, err := d.File(ctx, inf.Files[0].ID)
	if err != nil || !strings.HasPrefix(f.Path, filepath.Join(filepath.Dir(r.Raiz()), "cache")) || filepath.Base(f.Path) != "a.docx" {
		t.Fatal("abrir", f, err)
	}
	if b, _ := os.ReadFile(f.Path); string(b) != "DOCX v1" {
		t.Fatal("contenido de la caché", string(b))
	}
	if _, err := d.File(ctx, inf.Files[0].ID); err != nil || n.bajadas != 1 {
		t.Fatal("se volvió a bajar sin cambios", n.bajadas, err)
	}
	// Sin cambios, sincronizar no sube nada.
	if s := r.Sincronizar(ctx); len(s) != 0 {
		t.Fatal("subió sin cambios", s)
	}
	// Guardar en la caché (lo que hace Docus) y sincronizar sube encima.
	if err := os.WriteFile(f.Path, []byte("DOCX v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := r.Sincronizar(ctx)
	if len(s) != 1 || s[0].Err != nil || s[0].Conflicto || string(n.ficheros["/Informes/a.docx"]) != "DOCX v2" {
		t.Fatal("subir al guardar", s, string(n.ficheros["/Informes/a.docx"]))
	}
	// Alguien la cambia en la nube mientras aquí también se cambia: no se pisa.
	n.cambiarFuera("/Informes/a.docx", "DOCX de otra persona")
	if err := os.WriteFile(f.Path, []byte("DOCX v3 de aquí"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Children(ctx, hijos.Folders[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	s = r.Sincronizar(ctx)
	if len(s) != 1 || !s[0].Conflicto || !strings.HasPrefix(s[0].Como, "a (conflicto ") {
		t.Fatal("conflicto", s)
	}
	if string(n.ficheros["/Informes/a.docx"]) != "DOCX de otra persona" || string(n.ficheros["/Informes/"+s[0].Como]) != "DOCX v3 de aquí" {
		t.Fatal("se pisó lo de la otra persona", n.ficheros)
	}
	// Subir (y soltar) a una carpeta de la unidad.
	sub, err := d.Upload(ctx, hijos.Folders[0].ID, "nuevo.txt", "text/plain", strings.NewReader("hola"), 4)
	if err != nil || sub.Name != "nuevo.txt" || string(n.ficheros["/Informes/nuevo.txt"]) != "hola" {
		t.Fatal("subir", sub, err)
	}
	// Lo que el proveedor no expone falla cerrado, también por las interfaces opcionales.
	if err := d.Rename(ctx, api.DriveFile, sub.ID, "x.txt"); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("renombrar en la nube", err)
	}
	if _, err := d.CreateFolder(ctx, us[0].ID, "Nueva"); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("crear carpeta en la nube", err)
	}
	if err := d.Delete(ctx, api.DriveFile, sub.ID); !errors.Is(err, api.ErrPrivateDriveOperation) {
		t.Fatal("tirar en la nube", err)
	}
	// Y lo local sigue yendo al disco.
	raizLocal, _ := d.Root(ctx)
	if _, err := d.Upload(ctx, raizLocal.ID, "local.txt", "text/plain", strings.NewReader("aqui"), 4); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.Raiz(), "local.txt")); err != nil {
		t.Fatal("lo local no fue al disco", err)
	}
}

func TestEstadoDeLaCacheSobreviveAReabrir(t *testing.T) {
	r, n := abrirRemotas(t)
	ctx := t.Context()
	raiz, _ := r.Children(ctx, r.Unidades()[0].ID, "")
	inf, _ := r.Children(ctx, raiz.Folders[0].ID, "")
	f, err := r.File(ctx, inf.Files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.Path, []byte("cambiado con la app cerrada"), 0o600); err != nil {
		t.Fatal(err)
	}
	otra := ConRemotas(r.Archivos, r.cuentas, r.proveedor, r.cache)
	if s := otra.Sincronizar(ctx); len(s) != 1 || s[0].Err != nil || string(n.ficheros["/Informes/a.docx"]) != "cambiado con la app cerrada" {
		t.Fatal("tras reabrir no subió lo pendiente", s)
	}
}
