// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

const ficheroID = "7b0c6a52-1a8e-4d59-9a38-7c3f2a0d9e11"

// driveFalso es la puerta interna de Drive: apunta lo que se le pide y contesta
// con `estado`.
type driveFalso struct {
	mu     sync.Mutex
	estado int
	// porRuta manda sobre `estado` para una ruta concreta.
	porRuta map[string]int
	// resultado es el `result` de una ruta concreta; si no, «done».
	resultado map[string]string
	pedidos   []map[string]string
	rutas     []string
	secretos  []string
}

func (d *driveFalso) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c map[string]string
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &c)
		d.mu.Lock()
		d.pedidos = append(d.pedidos, c)
		d.rutas = append(d.rutas, r.URL.Path)
		d.secretos = append(d.secretos, r.Header.Get("Authorization"))
		estado := d.estado
		if e, ok := d.porRuta[r.URL.Path]; ok {
			estado = e
		}
		d.mu.Unlock()
		if estado == 0 {
			estado = http.StatusOK
		}
		res := "done"
		if r2, ok := d.resultado[r.URL.Path]; ok {
			res = r2
		}
		w.WriteHeader(estado)
		_, _ = w.Write([]byte(`{"result":"` + res + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// papeleraFalsa guarda lo que se le pide.
type papeleraFalsa struct {
	docs        []domain.Document
	restaurados []string
	purgados    []string
	sincro      []string
	caducados   []store.Caducado
	restaurado  string
}

func (p *papeleraFalsa) ListarPapelera(_ context.Context, _ string, kind domain.Kind, _ *store.CursorPapelera, _ int) ([]domain.Document, error) {
	var out []domain.Document
	for _, d := range p.docs {
		if kind == "" || d.Kind == kind {
			out = append(out, d)
		}
	}
	return out, nil
}

func (p *papeleraFalsa) DocumentoEnPapelera(_ context.Context, id, owner string) (*domain.Document, error) {
	for i := range p.docs {
		if p.docs[i].ID == id && p.docs[i].OwnerSub == owner {
			return &p.docs[i], nil
		}
	}
	return nil, domain.ErrNotFound
}

func (p *papeleraFalsa) RestaurarDocumento(_ context.Context, id string) error {
	p.restaurados = append(p.restaurados, id)
	return nil
}

func (p *papeleraFalsa) PurgarDocumento(_ context.Context, id string) error {
	p.purgados = append(p.purgados, id)
	return nil
}

func (p *papeleraFalsa) PapeleraCaducada(_ context.Context, _ time.Time, despues *store.CursorPapelera, _ int) ([]store.Caducado, error) {
	if despues != nil {
		return nil, nil
	}
	return p.caducados, nil
}

func (p *papeleraFalsa) SigueCaducado(_ context.Context, id string, _ time.Time) (bool, error) {
	return id != p.restaurado, nil
}

func (p *papeleraFalsa) PurgarCaducado(_ context.Context, id string, _ time.Time) error {
	p.purgados = append(p.purgados, id)
	return nil
}

func (p *papeleraFalsa) SinReconciliar(context.Context) ([]domain.Document, error) {
	return p.docs, nil
}

func (p *papeleraFalsa) SincronizarConDrive(_ context.Context, fileID, owner string, kind domain.Kind, estado string) error {
	p.sincro = append(p.sincro, fileID+"|"+owner+"|"+string(kind)+"|"+estado)
	return nil
}

// storeConBorrado cuenta los borrados del documento.
type storeConBorrado struct {
	*fakeStore
	borrados int
}

func (s *storeConBorrado) SoftDeleteDocument(context.Context, string) error {
	s.borrados++
	return nil
}

func conTenant(r *http.Request) *http.Request {
	return r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
}

func handlerConPapelera(t *testing.T, df *driveFalso, pf *papeleraFalsa, fs domain.Store) *Handler {
	t.Helper()
	svc := domain.NewService(fs)
	return &Handler{
		svc: svc, store: fs, papelera: pf,
		drive: drive.Nuevo(df.servidor(t).URL, 5*time.Second).ConSecretoInterno("s3cr3t"),
	}
}

func docConFichero(id string) domain.Document {
	return domain.Document{ID: id, OwnerSub: "owner", Kind: domain.KindDocument, SourceRef: "drive:" + ficheroID}
}

// TestBorrarTiraElFicheroAntes: el fichero va a la papelera de Drive antes que el
// documento, y si Drive falla el documento NO se borra.
func TestBorrarTiraElFicheroAntes(t *testing.T) {
	doc := docConFichero("d1")
	for _, c := range []struct {
		drive, codigo, borrados int
	}{
		{http.StatusOK, http.StatusNoContent, 1},
		{http.StatusServiceUnavailable, http.StatusServiceUnavailable, 0},
	} {
		df := &driveFalso{estado: c.drive}
		fs := &storeConBorrado{fakeStore: &fakeStore{doc: &doc}}
		h := handlerConPapelera(t, df, &papeleraFalsa{}, fs)
		rec := httptest.NewRecorder()
		h.deleteDocument(rec, conTenant(req(t, "DELETE", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, "")))
		if rec.Code != c.codigo || fs.borrados != c.borrados {
			t.Fatalf("Drive %d: código %d y %d borrados; quiero %d y %d", c.drive, rec.Code, fs.borrados, c.codigo, c.borrados)
		}
		if len(df.pedidos) != 1 || df.rutas[0] != "/internal/docs/files/"+ficheroID ||
			df.pedidos[0]["action"] != drive.AccionTirar || df.pedidos[0]["owner_sub"] != "owner" ||
			df.pedidos[0]["kind"] != "document" || df.pedidos[0]["tenant"] != "acme" ||
			df.secretos[0] != "Bearer s3cr3t" {
			t.Fatalf("a Drive llegó %v %v %v", df.rutas, df.pedidos, df.secretos)
		}
	}
}

// TestBorrarSinFicheroNoLlamaADrive: un documento nacido antes del hogar en Drive
// se borra solo aquí.
func TestBorrarSinFicheroNoLlamaADrive(t *testing.T) {
	df := &driveFalso{}
	fs := &storeConBorrado{fakeStore: &fakeStore{doc: ownedDoc()}}
	h := handlerConPapelera(t, df, &papeleraFalsa{}, fs)
	rec := httptest.NewRecorder()
	h.deleteDocument(rec, conTenant(req(t, "DELETE", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, "")))
	if rec.Code != http.StatusNoContent || fs.borrados != 1 || len(df.pedidos) != 0 {
		t.Fatalf("código %d, borrados %d, llamadas a Drive %d", rec.Code, fs.borrados, len(df.pedidos))
	}
}

// TestRestaurarYPurgar: Drive primero; si dice que no, Docs no cambia.
func TestRestaurarYPurgar(t *testing.T) {
	casos := []struct {
		nombre, metodo, ruta, accion string
		drive, codigo                int
		cambia                       bool
	}{
		{"restaurar", "POST", "/v1/trash/d1/restore", drive.AccionRestaurar, 200, 204, true},
		{"restaurar con el nombre ocupado", "POST", "/v1/trash/d1/restore", drive.AccionRestaurar, 409, 409, false},
		{"purgar", "DELETE", "/v1/trash/d1", drive.AccionPurgar, 200, 204, true},
		{"purgar con retención legal", "DELETE", "/v1/trash/d1", drive.AccionPurgar, 423, 423, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			df := &driveFalso{estado: c.drive}
			pf := &papeleraFalsa{docs: []domain.Document{docConFichero("5f3d1f8e-7c2a-4e5b-8f1a-2b3c4d5e6f70")}}
			pf.docs[0].ID = "5f3d1f8e-7c2a-4e5b-8f1a-2b3c4d5e6f70"
			h := handlerConPapelera(t, df, pf, &fakeStore{})
			rec := httptest.NewRecorder()
			r := conTenant(req(t, c.metodo, c.ruta, ownerClaims(), map[string]string{"id": pf.docs[0].ID}, ""))
			if c.accion == drive.AccionRestaurar {
				h.restaurarDePapelera(rec, r)
			} else {
				h.purgarDePapelera(rec, r)
			}
			if rec.Code != c.codigo {
				t.Fatalf("código %d, quiero %d (%s)", rec.Code, c.codigo, rec.Body.String())
			}
			if len(df.pedidos) != 1 || df.pedidos[0]["action"] != c.accion {
				t.Fatalf("a Drive llegó %v", df.pedidos)
			}
			cambio := len(pf.restaurados)+len(pf.purgados) > 0
			if cambio != c.cambia {
				t.Fatalf("restaurados %v, purgados %v", pf.restaurados, pf.purgados)
			}
		})
	}
}

// TestPapeleraAjenaNoExiste: el documento de otra persona es 404.
func TestPapeleraAjenaNoExiste(t *testing.T) {
	df := &driveFalso{}
	pf := &papeleraFalsa{docs: []domain.Document{{ID: "5f3d1f8e-7c2a-4e5b-8f1a-2b3c4d5e6f70", OwnerSub: "otra"}}}
	h := handlerConPapelera(t, df, pf, &fakeStore{})
	rec := httptest.NewRecorder()
	h.purgarDePapelera(rec, conTenant(req(t, "DELETE", "/v1/trash/x", ownerClaims(),
		map[string]string{"id": "5f3d1f8e-7c2a-4e5b-8f1a-2b3c4d5e6f70"}, "")))
	if rec.Code != http.StatusNotFound || len(df.pedidos) != 0 || len(pf.purgados) != 0 {
		t.Fatalf("código %d, Drive %v, purgados %v", rec.Code, df.pedidos, pf.purgados)
	}
}

// TestVaciarCuentaLoQueNoPuede: lo que Drive retiene se queda y se cuenta; un
// documento sin fichero también pregunta por la retención; y con `?kind=` no se
// toca lo de otro tipo.
func TestVaciarCuentaLoQueNoPuede(t *testing.T) {
	df := &driveFalso{porRuta: map[string]int{"/internal/docs/files/" + ficheroID: http.StatusLocked}}
	pf := &papeleraFalsa{docs: []domain.Document{
		docConFichero("a"),
		{ID: "b", OwnerSub: "owner", Kind: domain.KindDocument},
		{ID: "hoja", OwnerSub: "owner", Kind: domain.KindSpreadsheet},
	}}
	h := handlerConPapelera(t, df, pf, &fakeStore{})
	rec := httptest.NewRecorder()
	h.vaciarPapelera(rec, conTenant(req(t, "DELETE", "/v1/trash?kind=document", ownerClaims(), nil, "")))
	var out struct {
		Purged, Failed int
		Next           string
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusOK || out.Purged != 1 || out.Failed != 1 || out.Next != "" ||
		strings.Join(pf.purgados, ",") != "b" {
		t.Fatalf("código %d, %+v, purgados %v", rec.Code, out, pf.purgados)
	}
	if len(df.rutas) != 2 || df.rutas[1] != "/internal/docs/files/"+drive.SinFichero {
		t.Fatalf("a Drive llegó %v: el documento sin fichero también pregunta por la retención", df.rutas)
	}
}

// TestPurgarSinFicheroConRetencion: sin fichero, la retención legal de su dueño
// también impide borrarlo para siempre.
func TestPurgarSinFicheroConRetencion(t *testing.T) {
	const id = "5f3d1f8e-7c2a-4e5b-8f1a-2b3c4d5e6f70"
	df := &driveFalso{estado: http.StatusLocked}
	pf := &papeleraFalsa{docs: []domain.Document{{ID: id, OwnerSub: "owner", Kind: domain.KindDocument}}}
	h := handlerConPapelera(t, df, pf, &fakeStore{})
	rec := httptest.NewRecorder()
	h.purgarDePapelera(rec, conTenant(req(t, "DELETE", "/v1/trash/x", ownerClaims(), map[string]string{"id": id}, "")))
	if rec.Code != http.StatusLocked || len(pf.purgados) != 0 {
		t.Fatalf("código %d, purgados %v", rec.Code, pf.purgados)
	}
}

// TestSinSecretoCadaPapeleraPorSuLado: sin el secreto no se llama a Drive y borrar
// sigue funcionando; y un Drive anterior a la puerta (404) no deja a nadie sin borrar.
func TestSinSecretoCadaPapeleraPorSuLado(t *testing.T) {
	doc := docConFichero("d1")
	for _, c := range []struct {
		nombre   string
		secreto  string
		estado   int
		llamadas int
	}{
		{"sin secreto", "", http.StatusOK, 0},
		{"Drive sin la puerta", "s3cr3t", http.StatusNotFound, 1},
	} {
		df := &driveFalso{estado: c.estado}
		fs := &storeConBorrado{fakeStore: &fakeStore{doc: &doc}}
		h := handlerConPapelera(t, df, &papeleraFalsa{}, fs)
		h.drive.ConSecretoInterno(c.secreto)
		rec := httptest.NewRecorder()
		h.deleteDocument(rec, conTenant(req(t, "DELETE", "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, "")))
		if rec.Code != http.StatusNoContent || fs.borrados != 1 || len(df.pedidos) != c.llamadas {
			t.Fatalf("%s: código %d, borrados %d, llamadas %d", c.nombre, rec.Code, fs.borrados, len(df.pedidos))
		}
	}
}

// TestDesdeDrive: solo se aplica un estado conocido de un fichero de Docs con
// dueño; el tipo sale de la extensión.
func TestDesdeDrive(t *testing.T) {
	pf := &papeleraFalsa{}
	h := &Handler{papelera: pf}
	cuerpo := `{"tenant":"acme","files":[
		{"id":"` + ficheroID + `","state":"trashed","owner_sub":"ana","name":"Informe.ODT"},
		{"id":"` + ficheroID + `","state":"live","owner_sub":"ana","name":"Cuentas.ods"},
		{"id":"` + ficheroID + `","state":"gone","owner_sub":"ana","name":"Charla.odp"},
		{"id":"` + ficheroID + `","state":"trashed","owner_sub":"ana","name":"Informe.docx"},
		{"id":"no-es-uuid","state":"trashed","owner_sub":"ana","name":"x.odt"},
		{"id":"` + ficheroID + `","state":"borrado","owner_sub":"ana","name":"x.odt"},
		{"id":"` + ficheroID + `","state":"trashed","owner_sub":"","name":"x.odt"}]}`
	rec := httptest.NewRecorder()
	h.desdeDrive(rec, req(t, "POST", "/internal/drive/files", nil, nil, cuerpo))
	quiero := []string{
		ficheroID + "|ana|document|" + store.DrivePapelera,
		ficheroID + "|ana|spreadsheet|" + store.DriveVivo,
		ficheroID + "|ana|presentation|" + store.DrivePurgado,
	}
	if rec.Code != http.StatusNoContent || strings.Join(pf.sincro, "\n") != strings.Join(quiero, "\n") {
		t.Fatalf("código %d; aplicado:\n%s", rec.Code, strings.Join(pf.sincro, "\n"))
	}
	rec = httptest.NewRecorder()
	h.desdeDrive(rec, req(t, "POST", "/internal/drive/files", nil, nil, `{"tenant":"../x","files":[]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("tenant inválido: código %d", rec.Code)
	}
}

// TestPurgarPapeleraCaducada: cada caducado pregunta a Drive (retención y fichero)
// antes de borrarse. Lo retenido, lo que tiene el fichero vivo, lo que Drive no sabe
// contestar (un 404: no se sabría si hay retención) y lo restaurado entretanto se
// quedan; y sin la papelera compartida no se borra nada.
func TestPurgarPapeleraCaducada(t *testing.T) {
	viejo := InicioCaducidad
	InicioCaducidad = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	defer func() { InicioCaducidad = viejo }()
	const (
		fRetenido  = "7b0c6a52-1a8e-4d59-9a38-7c3f2a0d9e12"
		fVivo      = "7b0c6a52-1a8e-4d59-9a38-7c3f2a0d9e13"
		fSinPuerta = "7b0c6a52-1a8e-4d59-9a38-7c3f2a0d9e14"
	)
	conF := func(id, f string) domain.Document {
		return domain.Document{ID: id, OwnerSub: "ana", Kind: domain.KindDocument, SourceRef: "drive:" + f}
	}
	df := &driveFalso{porRuta: map[string]int{
		"/internal/docs/files/" + fRetenido:  http.StatusLocked,
		"/internal/docs/files/" + fSinPuerta: http.StatusNotFound,
	}, resultado: map[string]string{"/internal/docs/files/" + fVivo: "live"}}
	pf := &papeleraFalsa{restaurado: "restaurado", caducados: []store.Caducado{
		{Tenant: "acme", Doc: domain.Document{ID: "sin", OwnerSub: "ana", Kind: domain.KindDocument}},
		{Tenant: "otra", Doc: conF("retenido", fRetenido)},
		{Tenant: "acme", Doc: conF("vivo", fVivo)},
		{Tenant: "acme", Doc: conF("sinpuerta", fSinPuerta)},
		{Tenant: "acme", Doc: conF("restaurado", ficheroID)},
	}}
	h := handlerConPapelera(t, df, pf, &fakeStore{})
	c, err := h.PurgarPapeleraCaducada(context.Background())
	quiero := Caducidad{Purgados: 1, Retenidos: 1, Vivos: 1, Errores: 1}
	if err != nil || c != quiero || strings.Join(pf.purgados, ",") != "sin" {
		t.Fatalf("%+v, %v; borrados %v", c, err, pf.purgados)
	}
	if len(df.pedidos) != 4 || df.pedidos[0]["tenant"] != "acme" || df.pedidos[1]["tenant"] != "otra" ||
		df.rutas[0] != "/internal/docs/files/"+drive.SinFichero || df.pedidos[0]["action"] != drive.AccionPurgar {
		t.Fatalf("a Drive llegó %v %v (el restaurado no tenía que llegar)", df.rutas, df.pedidos)
	}
	h.drive.ConSecretoInterno("")
	pf.purgados = nil
	if _, err := h.PurgarPapeleraCaducada(context.Background()); err == nil || len(pf.purgados) != 0 {
		t.Fatalf("sin la papelera compartida no se borra nada: %v, %v", err, pf.purgados)
	}
	// Antes de que se cumplan 30 días desde que existe la papelera, nada caduca.
	InicioCaducidad = time.Now()
	h.drive.ConSecretoInterno("s3cr3t")
	if c, _ := h.PurgarPapeleraCaducada(context.Background()); c != (Caducidad{}) || len(pf.purgados) != 0 {
		t.Fatalf("con el reloj recién empezado se purgó: %+v %v", c, pf.purgados)
	}
}

// TestReconciliarPapelera: en seco pregunta «check» y no mueve nada; aplicando,
// «trash». Nunca purga.
func TestReconciliarPapelera(t *testing.T) {
	for _, c := range []struct {
		aplicar bool
		accion  string
	}{{false, drive.AccionComprobar}, {true, drive.AccionTirar}} {
		df := &driveFalso{}
		borrado := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		a, b := docConFichero("a"), docConFichero("b")
		a.DeletedAt, b.DeletedAt = &borrado, &borrado
		b.SourceRef = "drive:7b0c6a52-1a8e-4d59-9a38-7c3f2a0d9e15"
		df.resultado = map[string]string{"/internal/docs/files/7b0c6a52-1a8e-4d59-9a38-7c3f2a0d9e15": "modified"}
		pf := &papeleraFalsa{docs: []domain.Document{a, b}}
		h := handlerConPapelera(t, df, pf, &fakeStore{})
		r, err := h.ReconciliarPapelera(context.Background(), "acme", c.aplicar)
		if err != nil || r.Candidatos != 2 || r.APapelera != 1 || r.Modificados != 1 {
			t.Fatalf("aplicar=%v: %+v, %v", c.aplicar, r, err)
		}
		for _, p := range df.pedidos {
			if p["action"] != c.accion || p["tenant"] != "acme" || p["deleted_at"] != "2026-05-01T00:00:00Z" {
				t.Fatalf("aplicar=%v: a Drive llegó %v", c.aplicar, df.pedidos)
			}
		}
	}
	h := handlerConPapelera(t, &driveFalso{}, &papeleraFalsa{}, &fakeStore{})
	if _, err := h.ReconciliarPapelera(context.Background(), "../x", false); err == nil {
		t.Fatal("una organización inválida no se reconcilia")
	}
}
