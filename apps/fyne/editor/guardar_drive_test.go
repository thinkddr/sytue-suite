// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/office/guardardrive"
)

const ficheroBoveda = "0b3c6f9e-5d1a-4c2b-9e7f-1a2b3c4d5e62"

// bovedaEnMemoria es un Drive de la bóveda mínimo: la mecánica de verdad (cifrado,
// versiones) la prueban `guardardrive` y `drive`; aquí, el enganche del editor.
type bovedaEnMemoria struct {
	mu        sync.Mutex
	ficheros  map[string]api.File
	contenido map[string]string
}

func (b *bovedaEnMemoria) Root(context.Context) (api.Folder, error) {
	return api.Folder{ID: "raiz", IsRoot: true}, nil
}

func (b *bovedaEnMemoria) Children(_ context.Context, carpeta, _ string) (api.Children, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var c api.Children
	if carpeta == "raiz" {
		c.Folders = []api.Folder{{ID: "docs", Name: guardardrive.CarpetaDocumentos}}
	}
	for _, f := range b.ficheros {
		if f.FolderID == carpeta {
			c.Files = append(c.Files, f)
		}
	}
	return c, nil
}

func (*bovedaEnMemoria) CreateFolder(context.Context, string, string) (api.Folder, error) {
	return api.Folder{}, io.ErrUnexpectedEOF
}

func (b *bovedaEnMemoria) Upload(_ context.Context, carpeta, nombre, _ string, cuerpo io.Reader, _ int64) (api.File, error) {
	datos, _ := io.ReadAll(cuerpo)
	b.mu.Lock()
	defer b.mu.Unlock()
	f := api.File{ID: ficheroBoveda, FolderID: carpeta, Name: nombre, Version: 1}
	b.ficheros[f.ID], b.contenido[f.ID] = f, string(datos)
	return f, nil
}

func (b *bovedaEnMemoria) File(_ context.Context, id string) (api.File, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, ok := b.ficheros[id]
	if !ok {
		return api.File{}, &api.Error{Status: http.StatusNotFound}
	}
	return f, nil
}

func (b *bovedaEnMemoria) ReplaceFile(_ context.Context, f api.File, _ string, cuerpo io.Reader, _ int64) (api.File, error) {
	datos, _ := io.ReadAll(cuerpo)
	b.mu.Lock()
	defer b.mu.Unlock()
	f.Version++
	b.ficheros[f.ID], b.contenido[f.ID] = f, string(datos)
	return f, nil
}

type aviso struct {
	tono          ui.TonoAviso
	titulo, texto string
}

// TestGuardarEnDriveConElDriveEnLaBoveda: sin ancla, la entrada aparece en cuanto se
// sabe que el Drive está en la bóveda; pulsarla guarda lo pendiente, exporta, sube a la
// bóveda y ancla el documento por el servicio; la siguiente pulsación ya va a ese
// fichero. Al servicio de Docs solo le llega el ancla.
func TestGuardarEnDriveConElDriveEnLaBoveda(t *testing.T) {
	test.NewTempApp(t)
	var mu sync.Mutex
	var anclas []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/documents/d1/exports":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"export":{"id":"e1"}}`)
		case "GET /api/v1/documents/d1/exports/e1":
			_, _ = io.WriteString(w, "ODF del acta")
		case "PUT /api/v1/documents/d1/vault-anchor":
			var cuerpo map[string]string
			_ = json.NewDecoder(r.Body).Decode(&cuerpo)
			mu.Lock()
			anclas = append(anclas, cuerpo)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("inesperado: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	boveda := &bovedaEnMemoria{ficheros: map[string]api.File{}, contenido: map[string]string{}}
	destinos := &guardardrive.Destinos{
		DriveEnBoveda: func(context.Context) (bool, error) { return true, nil },
		Boveda:        func(context.Context) (guardardrive.Repositorio, error) { return boveda, nil },
	}
	traducir := func(k string, v i18n.Vars) string {
		if v != nil {
			return k + ":" + v["name"].(string)
		}
		return k
	}
	s := Servicio{Svc: api.NewDocs(srv.URL, nil, srv.Client()), Lanzar: func(f func()) { f() }, T: traducir}
	ctx := &Contexto{Titulo: "Acta"}
	var avisos []aviso
	guardados, recompuesto := 0, 0
	s.EngancharGuardarEnDrive(ctx, GuardarEnDrive{
		Destinos: destinos, ID: "d1", Doc: guardardrive.Documento{EsDueno: true, Titulo: "Acta", Kind: api.DocText},
		Antes:      func() func() error { return func() error { guardados++; return nil } },
		Recomponer: func() { recompuesto++ },
		Avisar:     func(tono ui.TonoAviso, titulo, texto string) { avisos = append(avisos, aviso{tono, titulo, texto}) },
	})
	if ctx.GuardarEnDrive == nil || recompuesto != 1 {
		t.Fatal("con el Drive en la bóveda la entrada tiene que salir aunque no haya ancla")
	}
	ctx.GuardarEnDrive()
	if guardados != 1 || len(anclas) != 1 || anclas[0]["source_ref"] != "vault:"+ficheroBoveda || anclas[0]["expect"] != "" {
		t.Fatalf("guardados %d, anclas %v", guardados, anclas)
	}
	if boveda.contenido[ficheroBoveda] != "ODF del acta" || boveda.ficheros[ficheroBoveda].Name != "Acta.odt" {
		t.Fatalf("bóveda = %v", boveda.ficheros)
	}
	if len(avisos) != 1 || avisos[0] != (aviso{ui.AvisoExito, "doc.msg.savedToDriveEncrypted", "doc.msg.savedToDriveEncryptedOnDevice:Acta.odt"}) {
		t.Fatalf("avisos = %v", avisos)
	}
	// La segunda vez, al fichero anclado (versión nueva), sin volver a anclar.
	ctx.GuardarEnDrive()
	if len(anclas) != 1 || boveda.ficheros[ficheroBoveda].Version != 2 || len(boveda.ficheros) != 1 {
		t.Fatalf("segundo guardado: anclas %v, ficheros %v", anclas, boveda.ficheros)
	}
}

// Sin destinos, o sin ancla y con el Drive en el servidor, la entrada no sale; con la
// bóveda cerrada se dice que hay que desbloquearla, sin exportar nada.
func TestGuardarEnDriveSinDondeOCerrada(t *testing.T) {
	test.NewTempApp(t)
	s := Servicio{Svc: api.NewPrivateDocs(), Lanzar: func(f func()) { f() }}
	ctx := &Contexto{}
	s.EngancharGuardarEnDrive(ctx, GuardarEnDrive{ID: "d1"})
	if ctx.GuardarEnDrive != nil {
		t.Fatal("sin destinos no hay entrada")
	}
	enServidor := &guardardrive.Destinos{
		DriveEnBoveda: func(context.Context) (bool, error) { return false, nil },
		Boveda: func(context.Context) (guardardrive.Repositorio, error) {
			return nil, guardardrive.ErrBovedaCerrada
		},
	}
	s.EngancharGuardarEnDrive(ctx, GuardarEnDrive{Destinos: enServidor, ID: "d1"})
	if ctx.GuardarEnDrive != nil {
		t.Fatal("sin ancla y con el Drive en el servidor no hay dónde guardar")
	}
	var avisos []aviso
	avisar := func(tono ui.TonoAviso, titulo, texto string) { avisos = append(avisos, aviso{tono, titulo, texto}) }
	doc := guardardrive.Documento{Ancla: "vault:" + ficheroBoveda}
	s.EngancharGuardarEnDrive(ctx, GuardarEnDrive{Destinos: enServidor, ID: "d1", Doc: doc, Avisar: avisar})
	if ctx.GuardarEnDrive == nil {
		t.Fatal("con un ancla la entrada sale")
	}
	ctx.GuardarEnDrive()
	if len(avisos) != 1 || avisos[0] != (aviso{ui.AvisoAdvertencia, "doc.msg.saveToDriveVaultLocked", ""}) {
		t.Fatalf("avisos = %v", avisos)
	}
}
