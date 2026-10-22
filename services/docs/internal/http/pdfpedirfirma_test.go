// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/connectors"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// peticionesFirmaEnMemoria es la 0011 sin base, con sus mismas reglas: una pendiente
// por fichero y destinatario, y cada transición solo desde «pendiente» y por quien toca.
type peticionesFirmaEnMemoria struct {
	mu    sync.Mutex
	filas []store.PeticionFirma
	// bloqueos son quien → bloqueado → nombre; politicas, organización → política.
	bloqueos  map[string]map[string]string
	politicas map[string]store.PoliticaFirma
	// fallarCopia hace fallar GuardarCopiaFirma: la petición dejó de estar mientras se
	// traía la copia.
	fallarCopia bool
}

func (m *peticionesFirmaEnMemoria) CrearPeticionFirma(_ context.Context, p store.PeticionFirma) (store.PeticionFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.filas) >= store.MaxPeticionesFirmaPorHora {
		return store.PeticionFirma{}, store.ErrDemasiadasPeticionesFirma
	}
	for _, x := range m.filas {
		if x.Estado == store.FirmaPendiente && x.Fichero == p.Fichero && x.Destinatario == p.Destinatario {
			return store.PeticionFirma{}, store.ErrPeticionFirmaRepetida
		}
	}
	if _, si := m.bloqueos[p.Destinatario][p.Solicitante]; si && p.Externo {
		p.Silenciada = true // lo que hace el INSERT con docs_firma_bloqueado
	}
	p.ID = "00000000-0000-4000-8000-" + strconv.FormatInt(int64(200000000000+len(m.filas)), 10)
	p.Estado, p.Creada = store.FirmaPendiente, time.Now()
	m.filas = append(m.filas, p)
	return p, nil
}

func (m *peticionesFirmaEnMemoria) PeticionesFirma(_ context.Context, sub string) (r, s []store.PeticionFirma, _ error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.filas {
		if x.Destinatario == sub && !x.Silenciada {
			r = append(r, x)
		}
		if x.Solicitante == sub {
			s = append(s, x)
		}
	}
	return r, s, nil
}

func (m *peticionesFirmaEnMemoria) PeticionFirmaPendiente(_ context.Context, id, dest string) (store.PeticionFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.filas {
		if x.ID == id && x.Destinatario == dest && x.Estado == store.FirmaPendiente && !x.Silenciada {
			return x, nil
		}
	}
	return store.PeticionFirma{}, store.ErrPeticionFirmaNoEsta
}

func (m *peticionesFirmaEnMemoria) PeticionDeFueraRecibida(_ context.Context, id, dest string) (store.PeticionFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.filas {
		if x.ID == id && x.Destinatario == dest && x.Externo && !x.Silenciada {
			return x, nil
		}
	}
	return store.PeticionFirma{}, store.ErrPeticionFirmaNoEsta
}

func (m *peticionesFirmaEnMemoria) BloquearFirma(_ context.Context, quien, bloqueado, nombre, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bloqueos == nil {
		m.bloqueos = map[string]map[string]string{}
	}
	if m.bloqueos[quien] == nil {
		m.bloqueos[quien] = map[string]string{}
	}
	m.bloqueos[quien][bloqueado] = nombre
	for i, x := range m.filas {
		if x.Destinatario == quien && x.Solicitante == bloqueado && x.Externo && x.Estado == store.FirmaPendiente {
			m.filas[i].Silenciada = true
		}
	}
	return nil
}

func (m *peticionesFirmaEnMemoria) DesbloquearFirma(_ context.Context, quien, bloqueado string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bloqueos[quien], bloqueado)
	return nil
}

func (m *peticionesFirmaEnMemoria) BloqueosFirma(_ context.Context, quien string) ([]store.BloqueoFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.BloqueoFirma, 0, len(m.bloqueos[quien]))
	for sub, nombre := range m.bloqueos[quien] {
		out = append(out, store.BloqueoFirma{Bloqueado: sub, Nombre: nombre})
	}
	return out, nil
}

func (m *peticionesFirmaEnMemoria) FirmaBloqueada(_ context.Context, dest, sol string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, si := m.bloqueos[dest][sol]
	return si, nil
}

// FirmaAdmitida es docs_firma_admite: sin política, abierta.
func (m *peticionesFirmaEnMemoria) FirmaAdmitida(_ context.Context, orgs []string, origen string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range orgs {
		p, hay := m.politicas[o]
		if o != origen && (!hay || p.Modo == store.PoliticaAbierta ||
			p.Modo == store.PoliticaConfianza && slices.Contains(p.Confianza, origen)) {
			return true, nil
		}
	}
	return false, nil
}

func (m *peticionesFirmaEnMemoria) PoliticaDeFirma(ctx context.Context) (store.PoliticaFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, _ := tenancy.From(ctx)
	if p, hay := m.politicas[t.Slug]; hay {
		return p, nil
	}
	return store.PoliticaFirma{Modo: store.PoliticaAbierta, Confianza: []string{}}, nil
}

func (m *peticionesFirmaEnMemoria) GuardarPoliticaDeFirma(ctx context.Context, p store.PoliticaFirma, _ string) (store.PoliticaFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, _ := tenancy.From(ctx)
	if m.politicas == nil {
		m.politicas = map[string]store.PoliticaFirma{}
	}
	m.politicas[t.Slug] = p
	return p, nil
}

func (m *peticionesFirmaEnMemoria) ResolverPeticionFirma(_ context.Context, id, quien, estado, firmado, ruta string, enlace []byte, huella string) (store.PeticionFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, x := range m.filas {
		dueño := x.Destinatario
		if estado == store.FirmaCancelada {
			dueño = x.Solicitante
		}
		if x.ID == id && dueño == quien && x.Estado == store.FirmaPendiente {
			f := &m.filas[i]
			f.Estado, f.Firmado, f.FirmadoRuta, f.Enlace, f.FirmadoHuella = estado, firmado, ruta, enlace, huella
			return *f, nil
		}
	}
	return store.PeticionFirma{}, store.ErrPeticionFirmaNoEsta
}

func (m *peticionesFirmaEnMemoria) PuedePedirFirma(context.Context, store.PeticionFirma) error {
	return nil
}

func (m *peticionesFirmaEnMemoria) AnfitrionPeticionFirma(context.Context, string, string) (string, error) {
	return "", nil
}

func (m *peticionesFirmaEnMemoria) OcuparPeticionFirma(_ context.Context, id, sub string, recibida bool) (store.PeticionFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.filas {
		if x.ID != id || !x.Externo {
			continue
		}
		if recibida && x.Destinatario == sub && x.Estado == store.FirmaPendiente && !x.Silenciada ||
			!recibida && x.Solicitante == sub && x.Estado == store.FirmaFirmada && x.Enlace != nil {
			return x, nil
		}
	}
	return store.PeticionFirma{}, store.ErrPeticionFirmaNoEsta
}

func (m *peticionesFirmaEnMemoria) SoltarPeticionFirma(context.Context, string, string) error {
	return nil
}

func (m *peticionesFirmaEnMemoria) GuardarCopiaFirma(_ context.Context, id, dest, copia, ruta string) (store.PeticionFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, x := range m.filas {
		if x.ID == id && x.Destinatario == dest && x.Estado == store.FirmaPendiente && x.Externo && !x.Silenciada && !m.fallarCopia {
			m.filas[i].Copia, m.filas[i].CopiaRuta = copia, ruta
			return m.filas[i], nil
		}
	}
	return store.PeticionFirma{}, store.ErrPeticionFirmaNoEsta
}

func (m *peticionesFirmaEnMemoria) GuardarFirmadoTraido(_ context.Context, id, sol, firmado, ruta string) (store.PeticionFirma, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, x := range m.filas {
		if x.ID == id && x.Solicitante == sol && x.Estado == store.FirmaFirmada && x.Externo && x.Enlace != nil {
			m.filas[i].Firmado, m.filas[i].FirmadoRuta, m.filas[i].Enlace = firmado, ruta, nil
			return m.filas[i], nil
		}
	}
	return store.PeticionFirma{}, store.ErrPeticionFirmaNoEsta
}

func (m *peticionesFirmaEnMemoria) OlvidarEnlaceFirma(_ context.Context, id, sol string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, x := range m.filas {
		if x.ID == id && x.Solicitante == sol {
			m.filas[i].EnlaceShare = ""
		}
	}
	return nil
}

// copiasDePrueba numera las copias: con len(d) un id se repetiría tras borrar una.
var copiasDePrueba atomic.Int64

// driveDePrueba son ficheros por id: nombre y contenido.
type driveDePrueba map[string]struct {
	nombre string
	datos  []byte
}

func (d driveDePrueba) Metadatos(_ context.Context, _, id string) (drive.Fichero, error) {
	f, ok := d[id]
	if !ok {
		return drive.Fichero{}, drive.ErrNoAutorizado
	}
	return drive.Fichero{ID: id, Name: f.nombre}, nil
}

func (d driveDePrueba) Descargar(_ context.Context, _, id string, _ int64) ([]byte, error) {
	f, ok := d[id]
	if !ok {
		return nil, drive.ErrNoAutorizado
	}
	return f.datos, nil
}

// Los enlaces viven en el mismo mapa, como «enlace:<token>» → id del fichero. Un
// fichero que se llama «bloqueado.pdf» lleva una etiqueta que no deja sacarlo fuera.
func (d driveDePrueba) CompartirEnlace(_ context.Context, _, id string, _ time.Time) (string, string, error) {
	f, ok := d[id]
	if !ok || f.nombre == "bloqueado.pdf" {
		return "", "", drive.ErrProhibido
	}
	tok := "tok-" + id + "-" + strconv.Itoa(len(d))
	d["enlace:"+tok] = struct {
		nombre string
		datos  []byte
	}{nombre: id}
	return "share-" + tok, tok, nil
}

func (d driveDePrueba) BorrarFichero(_ context.Context, _, id string) error {
	delete(d, id)
	return nil
}

func (d driveDePrueba) QuitarEnlace(_ context.Context, _, enlaceID string) error {
	delete(d, "enlace:"+strings.TrimPrefix(enlaceID, "share-"))
	return nil
}

func (d driveDePrueba) DescargarEnlace(_ context.Context, tok string, _ int64) ([]byte, string, error) {
	e, ok := d["enlace:"+tok]
	if !ok {
		return nil, "", drive.ErrNoAutorizado
	}
	return d[e.nombre].datos, d[e.nombre].nombre, nil
}

func (d driveDePrueba) GuardarPDF(_ context.Context, _, nombre string, datos []byte) (drive.Fichero, error) {
	id := "copia-" + strconv.FormatInt(copiasDePrueba.Add(1), 10)
	d[id] = struct {
		nombre string
		datos  []byte
	}{nombre, datos}
	return drive.Fichero{ID: id, Name: nombre, Path: "/Sytue Docs/" + nombre}, nil
}

// directorioDePrueba es el directory:resolve de Sytue ID con dos personas de acme.
func directorioDePrueba(t *testing.T) *httptest.Server {
	t.Helper()
	personas := map[string]string{"ana@acme.eu": "11111111-1111-4111-8111-111111111111", "irati@acme.eu": "22222222-2222-4222-8222-222222222222"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]string
		_ = json.NewDecoder(r.Body).Decode(&q)
		// Las organizaciones de una persona (ADR 0059), con el secreto «s».
		if r.URL.Path == "/internal/people/orgs" {
			if r.Header.Get("Authorization") != "Bearer s" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			orgs := `[]`
			personal := "false"
			switch q["user_id"] {
			case leire:
				orgs = `["globex"]`
			case mikel: // de globex y con cuenta personal
				orgs, personal = `["globex"]`, "true"
			}
			_, _ = io.WriteString(w, `{"orgs":`+orgs+`,"personal":`+personal+`}`)
			return
		}
		// people:resolve (ADR 0040): cualquiera de Sytue One, sin su correo.
		if strings.HasSuffix(r.URL.Path, "/people:resolve") {
			if q["query"] == "leire@globex.eu" || q["query"] == "@leire" {
				_, _ = io.WriteString(w, `{"user_id":"`+leire+`","name":"Leire","username":"leire"}`)
				return
			}
			if q["query"] == "@mikel" {
				_, _ = io.WriteString(w, `{"user_id":"`+mikel+`","name":"Mikel"}`)
				return
			}
			if q["query"] == "@irati" { // de la organización, por su @usuario
				_, _ = io.WriteString(w, `{"user_id":"22222222-2222-4222-8222-222222222222","name":"Irati"}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for correo, sub := range personas {
			if q["email"] == correo || q["user_id"] == sub {
				_, _ = io.WriteString(w, `{"user_id":"`+sub+`","email":"`+correo+`"}`)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// leire es de globex, otra organización; mikel, de globex y con cuenta personal.
const (
	leire = "33333333-3333-4333-8333-333333333333"
	mikel = "44444444-4444-4444-8444-444444444444"
)

func llamarJSON(t *testing.T, h http.HandlerFunc, metodo, ruta, sub, cuerpo string, params map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	org := "acme"
	if sub == leire || sub == mikel {
		org = "globex"
	}
	return llamarEn(t, h, metodo, ruta, authn.Claims{Subject: sub, Tenant: org, Name: "Ana"}, cuerpo, params)
}

func llamarEn(t *testing.T, h http.HandlerFunc, metodo, ruta string, c authn.Claims, cuerpo string, params map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	r.Header.Set("Content-Type", "application/json")
	rc := chi.NewRouteContext()
	for k, v := range params {
		rc.URLParams.Add(k, v)
	}
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rc)
	if c.Tenant != "" { // lo que hace RequireTenant
		ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: c.Tenant})
	}
	r = r.WithContext(authn.Into(ctx, c))
	rec := httptest.NewRecorder()
	h(rec, r)
	return rec
}

func TestPedirFirmaAOtraPersona(t *testing.T) {
	const ana, irati = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	w := pdfsimple.Nuevo()
	w.Texto("contrato de servicios", "F1", 12)
	ficheros := driveDePrueba{
		"original": {"Contrato.pdf", w.Bytes()},
		"hoja":     {"notas.txt", []byte("hola")},
	}
	pet := &peticionesFirmaEnMemoria{}
	h := &Handler{firmas: &firmasEnMemoria{}, peticionesFirma: pet, ficheros: ficheros, idBaseURL: directorioDePrueba(t).URL}

	pedir := func(cuerpo string) *httptest.ResponseRecorder {
		return llamarJSON(t, h.pedirFirma, http.MethodPost, "/api/v1/pdf/signature-requests", ana, cuerpo, nil)
	}
	for _, c := range []struct {
		nombre, cuerpo string
		estado         int
	}{
		{"a nadie", `{"file_id":"original","path":"/PDF/Contrato.pdf"}`, http.StatusBadRequest},
		{"a dos a la vez", `{"file_id":"original","path":"/PDF/Contrato.pdf","to":"` + irati + `","email":"irati@acme.eu"}`, http.StatusBadRequest},
		{"con una ruta que no es de un PDF", `{"file_id":"original","path":"/PDF/../x.pdf","email":"irati@acme.eu"}`, http.StatusBadRequest},
		{"a alguien de fuera", `{"file_id":"original","path":"/PDF/Contrato.pdf","email":"otro@globex.eu"}`, http.StatusNotFound},
		{"a un sub de fuera", `{"file_id":"original","path":"/PDF/Contrato.pdf","to":"` + leire + `"}`, http.StatusNotFound},
		{"a un @usuario que no existe", `{"file_id":"original","path":"/PDF/Contrato.pdf","email":"@nadie"}`, http.StatusNotFound},
		{"a sí misma", `{"file_id":"original","path":"/PDF/Contrato.pdf","email":"ana@acme.eu"}`, http.StatusBadRequest},
		{"de algo que no es un PDF", `{"file_id":"hoja","path":"/PDF/Contrato.pdf","email":"irati@acme.eu"}`, http.StatusNotFound},
		{"de un fichero que no ve", `{"file_id":"ajeno","path":"/PDF/Contrato.pdf","email":"irati@acme.eu"}`, http.StatusNotFound},
		{"con la ruta de otro fichero", `{"file_id":"original","path":"/PDF/Otro.pdf","email":"irati@acme.eu"}`, http.StatusNotFound},
	} {
		if rec := pedir(c.cuerpo); rec.Code != c.estado {
			t.Errorf("%s: %d %s", c.nombre, rec.Code, rec.Body)
		}
	}

	rec := pedir(`{"file_id":"original","path":"/PDF/Contrato.pdf","email":"irati@acme.eu","message":"Firma antes del viernes"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("pedir: %d %s", rec.Code, rec.Body)
	}
	var p store.PeticionFirma
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p.Destinatario != irati || p.DestinatarioCorreo != "irati@acme.eu" || p.Estado != store.FirmaPendiente || p.SolicitanteNombre != "Ana" {
		t.Errorf("petición = %+v", p)
	}
	if rec := pedir(`{"file_id":"original","path":"/PDF/Contrato.pdf","to":"` + irati + `"}`); rec.Code != http.StatusConflict {
		t.Errorf("dos veces: %d %s", rec.Code, rec.Body)
	}

	rec = llamarJSON(t, h.listarPeticionesFirma, http.MethodGet, "/", irati, "", nil)
	var lista struct{ Received, Sent []store.PeticionFirma }
	_ = json.Unmarshal(rec.Body.Bytes(), &lista)
	if len(lista.Received) != 1 || len(lista.Sent) != 0 || lista.Received[0].Mensaje != "Firma antes del viernes" {
		t.Errorf("lo que ve Irati: %s", rec.Body)
	}

	firmar := func(sub, fichero string) *httptest.ResponseRecorder {
		return llamarJSON(t, h.firmarPeticion, http.MethodPost, "/", sub,
			`{"signed_file_id":"`+fichero+`","signed_path":"/PDF/Contrato (firmado).pdf"}`, map[string]string{"id": p.ID})
	}
	// El firmado de verdad: Irati lo firma en dos pasos con su certificado.
	cert, clave := certificadoDeFirma(t)
	prep := preparar(t, h, irati, cert)
	sig, _ := rsa.SignPKCS1v15(rand.Reader, clave, crypto.SHA256, prep.Resumen)
	firmado := completar(t, h, irati, prep, prep.Preparado, sig, map[string]string{"sin_sello": "1"})
	if firmado.Code != http.StatusOK {
		t.Fatalf("firmar el PDF: %d %s", firmado.Code, firmado.Body)
	}
	ficheros["firmado"] = struct {
		nombre string
		datos  []byte
	}{"Contrato (firmado).pdf", firmado.Body.Bytes()}

	if rec := firmar(ana, "firmado"); rec.Code != http.StatusNotFound {
		t.Errorf("quien pidió no firma por la otra: %d %s", rec.Code, rec.Body)
	}
	if rec := llamarJSON(t, h.firmarPeticion, http.MethodPost, "/", irati,
		`{"signed_file_id":"original","signed_path":"/PDF/Contrato.pdf"}`, map[string]string{"id": p.ID}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("sin firmar: %d %s", rec.Code, rec.Body)
	}
	if rec := llamarJSON(t, h.firmarPeticion, http.MethodPost, "/", irati,
		`{"signed_file_id":"firmado","signed_path":"/PDF/Otro nombre.pdf"}`, map[string]string{"id": p.ID}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("con la ruta de otro fichero: %d %s", rec.Code, rec.Body)
	}
	if rec := firmar(irati, "firmado"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"firmada"`) {
		t.Fatalf("firmada: %d %s", rec.Code, rec.Body)
	}
	if rec := firmar(irati, "firmado"); rec.Code != http.StatusNotFound {
		t.Errorf("dos veces: %d %s", rec.Code, rec.Body)
	}
	if rec := llamarJSON(t, h.cancelarPeticionFirma, http.MethodDelete, "/", ana, "", map[string]string{"id": p.ID}); rec.Code != http.StatusNotFound {
		t.Errorf("cancelar una ya firmada: %d %s", rec.Code, rec.Body)
	}
}

// TestNadiePideFirmasSinFin: cada petición es un aviso y un correo a otra persona.
func TestNadiePideFirmasSinFin(t *testing.T) {
	const ana, irati = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	pet := &peticionesFirmaEnMemoria{}
	ficheros := driveDePrueba{}
	h := &Handler{peticionesFirma: pet, ficheros: ficheros, idBaseURL: directorioDePrueba(t).URL}
	for i := range store.MaxPeticionesFirmaPorHora + 1 {
		id := "f" + strconv.Itoa(i)
		ficheros[id] = struct {
			nombre string
			datos  []byte
		}{"a.pdf", nil}
		rec := llamarJSON(t, h.pedirFirma, http.MethodPost, "/", ana, `{"file_id":"`+id+`","path":"/a.pdf","to":"`+irati+`"}`, nil)
		quiere := http.StatusCreated
		if i == store.MaxPeticionesFirmaPorHora {
			quiere = http.StatusTooManyRequests
		}
		if rec.Code != quiere {
			t.Fatalf("la %d: %d %s", i+1, rec.Code, rec.Body)
		}
	}
}

func TestRechazarYCancelarUnaPeticionDeFirma(t *testing.T) {
	const ana, irati = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	pet := &peticionesFirmaEnMemoria{}
	h := &Handler{peticionesFirma: pet, ficheros: driveDePrueba{"f": {"a.pdf", nil}}, idBaseURL: directorioDePrueba(t).URL}
	crear := func() string {
		rec := llamarJSON(t, h.pedirFirma, http.MethodPost, "/", ana, `{"file_id":"f","path":"/a.pdf","to":"`+irati+`"}`, nil)
		var p store.PeticionFirma
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.ID == "" {
			t.Fatalf("pedir: %d %s", rec.Code, rec.Body)
		}
		return p.ID
	}
	id := crear()
	if rec := llamarJSON(t, h.cancelarPeticionFirma, http.MethodDelete, "/", irati, "", map[string]string{"id": id}); rec.Code != http.StatusNotFound {
		t.Errorf("quien recibe no cancela: %d", rec.Code)
	}
	if rec := llamarJSON(t, h.rechazarPeticionFirma, http.MethodPost, "/", ana, "", map[string]string{"id": id}); rec.Code != http.StatusNotFound {
		t.Errorf("quien pide no rechaza: %d", rec.Code)
	}
	if rec := llamarJSON(t, h.rechazarPeticionFirma, http.MethodPost, "/", irati, "", map[string]string{"id": id}); rec.Code != http.StatusOK {
		t.Errorf("rechazar: %d %s", rec.Code, rec.Body)
	}
	id = crear() // rechazada, se puede volver a pedir
	if rec := llamarJSON(t, h.cancelarPeticionFirma, http.MethodDelete, "/", ana, "", map[string]string{"id": id}); rec.Code != http.StatusNoContent {
		t.Errorf("cancelar: %d %s", rec.Code, rec.Body)
	}
	if rec := llamarJSON(t, h.rechazarPeticionFirma, http.MethodPost, "/", irati, "", map[string]string{"id": "no-es-uuid"}); rec.Code != http.StatusNotFound {
		t.Errorf("id que no es uuid: %d", rec.Code)
	}
}

// pdfCon es un PDF de una página con `texto`.
func pdfCon(texto string) []byte {
	w := pdfsimple.Nuevo()
	w.Texto(texto, "F1", 12)
	return w.Bytes()
}

// TestPedirFirmaAFuera es el ADR 0057 de punta a punta: Ana (acme) pide a Leire
// (globex) que firme; el PDF sale por un enlace de Drive que nadie ve, Leire se trae
// una copia a su unidad, la firma y la entrega, y Ana se trae la firmada a la suya.
func TestPedirFirmaAFuera(t *testing.T) {
	const ana = "11111111-1111-4111-8111-111111111111"
	ficheros := driveDePrueba{
		"original":   {"Contrato.pdf", pdfCon("contrato de servicios")},
		"etiquetado": {"bloqueado.pdf", pdfCon("contrato de servicios")},
	}
	sellador, err := connectors.NewSealer(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	pet := &peticionesFirmaEnMemoria{}
	h := &Handler{
		firmas: &firmasEnMemoria{}, peticionesFirma: pet, ficheros: ficheros, sealer: sellador,
		idBaseURL: directorioDePrueba(t).URL, idSecreto: "s",
	}
	pedir := func(cuerpo string) *httptest.ResponseRecorder {
		return llamarJSON(t, h.pedirFirma, http.MethodPost, "/", ana, cuerpo, nil)
	}
	enlaces := func() int {
		n := 0
		for k := range ficheros {
			if strings.HasPrefix(k, "enlace:") {
				n++
			}
		}
		return n
	}
	const confirmado = `,"confirm":"` + leire + `"`
	antes := limiteFirmaFuera
	limiteFirmaFuera = httpx.NewTokenBucket(1000, 1000)
	t.Cleanup(func() { limiteFirmaFuera = antes })

	// Sin confirmar: se explica, y no se crea nada. El 409 dice a quién se confirma.
	rec := pedir(`{"file_id":"original","path":"/Legal/Contrato.pdf","email":"leire@globex.eu"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "pdf_signature_recipient_external") ||
		!strings.Contains(rec.Body.String(), `"name":"Leire"`) || !strings.Contains(rec.Body.String(), `"confirm":"`+leire+`"`) ||
		len(pet.filas) != 0 || enlaces() != 0 {
		t.Fatalf("sin confirmar: %d %s", rec.Code, rec.Body)
	}
	// Confirmar a OTRA persona no vale.
	if rec := pedir(`{"file_id":"original","path":"/Legal/Contrato.pdf","email":"@leire","confirm":"otra"}`); rec.Code != http.StatusConflict || enlaces() != 0 {
		t.Errorf("confirmado a otra: %d %s", rec.Code, rec.Body)
	}
	// Un @usuario de la organización es de dentro: sin enlace ni confirmar.
	if rec := pedir(`{"file_id":"original","path":"/Legal/Contrato.pdf","email":"@irati"}`); rec.Code != http.StatusCreated || enlaces() != 0 || pet.filas[0].Externo {
		t.Fatalf("@usuario de dentro: %d %s", rec.Code, rec.Body)
	}
	pet.filas = nil
	// Sin un PDF suyo, no se pregunta por nadie.
	if rec := pedir(`{"file_id":"ajeno","path":"/Legal/Contrato.pdf","email":"@leire"}`); rec.Code != http.StatusNotFound {
		t.Errorf("sin fichero: %d %s", rec.Code, rec.Body)
	}
	// Una etiqueta que no deja sacarlo fuera, lo para también aquí.
	if rec := pedir(`{"file_id":"etiquetado","path":"/Legal/bloqueado.pdf","email":"@leire"` + confirmado + `}`); rec.Code != http.StatusForbidden || len(pet.filas) != 0 {
		t.Errorf("etiquetado: %d %s", rec.Code, rec.Body)
	}
	// Sin la clave con que cifrar el token, no se ofrece.
	h.sealer = nil
	if rec := pedir(`{"file_id":"original","path":"/Legal/Contrato.pdf","email":"@leire"` + confirmado + `}`); rec.Code != http.StatusServiceUnavailable || enlaces() != 0 {
		t.Errorf("sin clave: %d %s", rec.Code, rec.Body)
	}
	h.sealer = sellador

	rec = pedir(`{"file_id":"original","path":"/Legal/Contrato.pdf","email":"@leire"` + confirmado + `}`)
	if rec.Code != http.StatusCreated || enlaces() != 1 {
		t.Fatalf("pedir: %d %s", rec.Code, rec.Body)
	}
	fila := pet.filas[0]
	if !fila.Externo || fila.Destinatario != leire || fila.EnlaceShare == "" || len(fila.Enlace) == 0 || fila.Huella == "" {
		t.Fatalf("fila = %+v", fila)
	}
	for k := range ficheros {
		if tok, ok := strings.CutPrefix(k, "enlace:"); ok && (bytes.Contains(fila.Enlace, []byte(tok)) || strings.Contains(rec.Body.String(), tok)) {
			t.Errorf("el token del enlace va en claro")
		}
	}

	// Leire la ve, sin la ruta de la otra organización.
	rec = llamarJSON(t, h.listarPeticionesFirma, http.MethodGet, "/", leire, "", nil)
	var lista struct{ Received, Sent []store.PeticionFirma }
	_ = json.Unmarshal(rec.Body.Bytes(), &lista)
	if len(lista.Received) != 1 || !lista.Received[0].Externo || lista.Received[0].Nombre != "Contrato.pdf" ||
		strings.Contains(rec.Body.String(), "/Legal") || strings.Contains(rec.Body.String(), `"original"`) {
		t.Fatalf("lo que ve Leire: %s", rec.Body)
	}
	id := map[string]string{"id": fila.ID}

	// Firmar sin haberla abierto no vale: no hay copia contra la que comprobar.
	if rec := llamarJSON(t, h.firmarPeticion, http.MethodPost, "/", leire,
		`{"signed_file_id":"original","signed_path":"/Contrato.pdf"}`, id); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("sin abrir: %d %s", rec.Code, rec.Body)
	}
	// Si el original cambia antes de abrirla, lo que sirve el enlace no es lo pedido.
	bueno := ficheros["original"]
	ficheros["original"] = struct {
		nombre string
		datos  []byte
	}{bueno.nombre, pdfCon("otra cosa")}
	if rec := llamarJSON(t, h.abrirPeticionFirma, http.MethodPost, "/", leire, "", id); rec.Code != http.StatusConflict {
		t.Errorf("original cambiado: %d %s", rec.Code, rec.Body)
	}
	ficheros["original"] = bueno
	// Abrirla la trae a su unidad; la segunda vez, la misma copia; si la borra, otra.
	abrir := func() store.PeticionFirma {
		rec := llamarJSON(t, h.abrirPeticionFirma, http.MethodPost, "/", leire, "", id)
		var p store.PeticionFirma
		if err := json.Unmarshal(rec.Body.Bytes(), &p); rec.Code != http.StatusOK || err != nil {
			t.Fatalf("abrir: %d %s", rec.Code, rec.Body)
		}
		return p
	}
	copia := abrir()
	if copia.Ruta != "/Sytue Docs/Contrato.pdf" || !strings.HasPrefix(copia.Fichero, "copia-") {
		t.Fatalf("copia = %+v", copia)
	}
	if otra := abrir(); otra.Fichero != copia.Fichero {
		t.Errorf("dos copias: %s y %s", copia.Fichero, otra.Fichero)
	}
	delete(ficheros, copia.Fichero)
	if otra := abrir(); otra.Fichero == copia.Fichero || otra.Fichero == "" {
		t.Errorf("sin volver a traerla: %+v", otra)
	}
	copia = abrir()
	if rec := llamarJSON(t, h.abrirPeticionFirma, http.MethodPost, "/", ana, "", id); rec.Code != http.StatusNotFound {
		t.Errorf("Ana no la abre por Leire: %d", rec.Code)
	}

	// Leire firma su copia.
	cert, clave := certificadoDeFirma(t)
	prep := preparar(t, h, leire, cert)
	sig, _ := rsa.SignPKCS1v15(rand.Reader, clave, crypto.SHA256, prep.Resumen)
	firmado := completar(t, h, leire, prep, prep.Preparado, sig, map[string]string{"sin_sello": "1"})
	if firmado.Code != http.StatusOK {
		t.Fatalf("firmar el PDF: %d %s", firmado.Code, firmado.Body)
	}
	// Lo firmado tiene que ser EL documento: otro, aunque venga firmado, no vale.
	if err := firmaNueva(pdfCon("otra cosa"), firmado.Body.Bytes()); err == nil {
		t.Error("un PDF firmado con otro texto pasa por el firmado")
	}
	ficheros["firmado-globex"] = struct {
		nombre string
		datos  []byte
	}{"Contrato (firmado).pdf", firmado.Body.Bytes()}
	entregar := func() *httptest.ResponseRecorder {
		return llamarJSON(t, h.firmarPeticion, http.MethodPost, "/", leire,
			`{"signed_file_id":"firmado-globex","signed_path":"/Mis cosas/Contrato (firmado).pdf"}`, id)
	}
	// Si cambia su copia antes de firmar, firmaría otro documento.
	suya := ficheros[copia.Fichero]
	ficheros[copia.Fichero] = struct {
		nombre string
		datos  []byte
	}{suya.nombre, pdfCon("contrato de servicios, con otra cláusula")}
	if rec := entregar(); rec.Code != http.StatusConflict {
		t.Errorf("copia cambiada: %d %s", rec.Code, rec.Body)
	}
	ficheros[copia.Fichero] = suya
	rec = entregar()
	if rec.Code != http.StatusOK || pet.filas[0].FirmadoRuta != "" || pet.filas[0].Enlace == nil || pet.filas[0].FirmadoHuella == "" {
		t.Fatalf("entregar: %d %s / %+v", rec.Code, rec.Body, pet.filas[0])
	}

	// Ana no ve dónde lo guarda Leire hasta que se lo trae. Y al listar, el enlace del
	// original, que ya no hace falta, se revoca.
	rec = llamarJSON(t, h.listarPeticionesFirma, http.MethodGet, "/", ana, "", nil)
	if strings.Contains(rec.Body.String(), "Mis cosas") || strings.Contains(rec.Body.String(), "firmado-globex") {
		t.Errorf("lo que ve Ana antes de traerlo: %s", rec.Body)
	}
	if _, ok := ficheros["enlace:"+strings.TrimPrefix(fila.EnlaceShare, "share-")]; ok || pet.filas[0].EnlaceShare != "" {
		t.Errorf("el enlace del original sigue vivo")
	}
	// Si Leire cambia el fichero después de entregarlo, lo que sirve el enlace no vale.
	bueno = ficheros["firmado-globex"]
	ficheros["firmado-globex"] = struct {
		nombre string
		datos  []byte
	}{bueno.nombre, pdfCon("contrato de servicios")}
	if rec := llamarJSON(t, h.traerFirmado, http.MethodPost, "/", ana, "", id); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("cambiado después: %d %s", rec.Code, rec.Body)
	}
	ficheros["firmado-globex"] = bueno
	if rec := llamarJSON(t, h.traerFirmado, http.MethodPost, "/", leire, "", id); rec.Code != http.StatusNotFound {
		t.Errorf("Leire no se lo trae por Ana: %d", rec.Code)
	}
	rec = llamarJSON(t, h.traerFirmado, http.MethodPost, "/", ana, "", id)
	var traida store.PeticionFirma
	if err := json.Unmarshal(rec.Body.Bytes(), &traida); rec.Code != http.StatusOK || err != nil ||
		traida.FirmadoRuta != "/Sytue Docs/Contrato (firmado).pdf" || pet.filas[0].Enlace != nil {
		t.Fatalf("traer: %d %s", rec.Code, rec.Body)
	}
	if rec := llamarJSON(t, h.traerFirmado, http.MethodPost, "/", ana, "", id); rec.Code != http.StatusNotFound {
		t.Errorf("dos veces: %d", rec.Code)
	}
	// Cada vuelta baja y sube hasta 25 MiB: con el cubo vacío, 429.
	limiteFirmaFuera = httpx.NewTokenBucket(0.0001, 1)
	llamarJSON(t, h.traerFirmado, http.MethodPost, "/", ana, "", id)
	if rec := llamarJSON(t, h.traerFirmado, http.MethodPost, "/", ana, "", id); rec.Code != http.StatusTooManyRequests {
		t.Errorf("sin tope: %d", rec.Code)
	}
}

// TestElTokenNoValeEnOtraPeticion: el token cifrado va atado a su petición.
func TestElTokenNoValeEnOtraPeticion(t *testing.T) {
	sellador, _ := connectors.NewSealer(bytes.Repeat([]byte{7}, 32))
	blob, err := sellador.SealPara([]byte("tok"), paraFirmado("a"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sellador.OpenPara(blob, paraFirmado("b")); err == nil {
		t.Error("se abre para otra petición")
	}
	if got, err := sellador.OpenPara(blob, paraFirmado("a")); err != nil || string(got) != "tok" {
		t.Errorf("no se abre para la suya: %q %v", got, err)
	}
}

// TestCancelarAFueraQuitaElEnlace: el enlace de Drive se va con la petición.
func TestCancelarAFueraQuitaElEnlace(t *testing.T) {
	const ana = "11111111-1111-4111-8111-111111111111"
	sellador, _ := connectors.NewSealer(bytes.Repeat([]byte{7}, 32))
	ficheros := driveDePrueba{"f": {"a.pdf", pdfCon("a")}}
	pet := &peticionesFirmaEnMemoria{}
	h := &Handler{peticionesFirma: pet, ficheros: ficheros, sealer: sellador, idBaseURL: directorioDePrueba(t).URL, idSecreto: "s"}
	rec := llamarJSON(t, h.pedirFirma, http.MethodPost, "/", ana, `{"file_id":"f","path":"/a.pdf","email":"@leire","confirm":"`+leire+`"}`, nil)
	if rec.Code != http.StatusCreated || len(ficheros) != 2 {
		t.Fatalf("pedir: %d %s", rec.Code, rec.Body)
	}
	if rec := llamarJSON(t, h.cancelarPeticionFirma, http.MethodDelete, "/", ana, "", map[string]string{"id": pet.filas[0].ID}); rec.Code != http.StatusNoContent || len(ficheros) != 1 {
		t.Errorf("cancelar: %d, quedan %d", rec.Code, len(ficheros))
	}
}

// anfitrionaFija es un store cuya única petición de fuera es la de globex→acme.
type anfitrionaFija struct{ peticionesFirmaEnMemoria }

func (*anfitrionaFija) AnfitrionPeticionFirma(_ context.Context, id, sub string) (string, error) {
	if id == "00000000-0000-4000-8000-000000000001" && sub == leire {
		return "acme", nil
	}
	return "", nil
}

// TestPeticionEnAnfitrion: solo el destinatario de fuera pasa a la organización de la
// petición, y sin sus grupos ni su organización.
func TestPeticionEnAnfitrion(t *testing.T) {
	h := &Handler{peticionesFirma: &anfitrionaFija{}}
	var visto struct {
		tenant string
		subj   domain.Subject
	}
	fin := h.peticionEnAnfitrion(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		tn, _ := tenancy.From(r.Context())
		visto.tenant = tn.Slug
		visto.subj, _ = subjectFrom(r)
	}))
	llamar := func(sub, org, id string) {
		visto.tenant, visto.subj = "", domain.Subject{}
		r := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", id)
		ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rc)
		ctx = tenancy.Into(authn.Into(ctx, authn.Claims{Subject: sub, Tenant: org, Roles: []string{"owner"}}), tenancy.Tenant{Slug: org})
		fin.ServeHTTP(httptest.NewRecorder(), r.WithContext(ctx))
	}
	const suya = "00000000-0000-4000-8000-000000000001"
	llamar(leire, "globex", suya)
	if visto.tenant != "acme" || visto.subj.Tenant != "" || len(visto.subj.Groups) != 0 || visto.subj.Sub != leire {
		t.Errorf("Leire en su petición: %s %+v", visto.tenant, visto.subj)
	}
	for _, c := range []struct{ sub, org, id string }{
		{leire, "globex", "00000000-0000-4000-8000-000000000002"}, // otra petición
		{"11111111-1111-4111-8111-111111111111", "globex", suya},  // otra persona
		{leire, "globex", "no-es-uuid"},
	} {
		llamar(c.sub, c.org, c.id)
		if visto.tenant != c.org || len(visto.subj.Groups) == 0 {
			t.Errorf("%+v: pasó a %s %+v", c, visto.tenant, visto.subj)
		}
	}
}

// entornoFuera es Ana (acme) pidiendo a Leire (globex), con un Hub que apunta los avisos.
func entornoFuera(t *testing.T) (*Handler, *peticionesFirmaEnMemoria, driveDePrueba, chan hubnotify.Notification) {
	t.Helper()
	avisos := make(chan hubnotify.Notification, 16)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(hub.Close)
	sellador, _ := connectors.NewSealer(bytes.Repeat([]byte{7}, 32))
	ficheros := driveDePrueba{"f": {"Contrato.pdf", pdfCon("contrato")}, "g": {"Otro.pdf", pdfCon("otro")}}
	pet := &peticionesFirmaEnMemoria{}
	h := &Handler{
		peticionesFirma: pet, ficheros: ficheros, sealer: sellador, idBaseURL: directorioDePrueba(t).URL, idSecreto: "s",
		hubNotifier: hubnotify.New(hub.URL, "s"), publicURL: "https://docs.sytue.eu",
	}
	return h, pet, ficheros, avisos
}

// hubo dice si llegó un aviso (y cuál) en un rato.
func hubo(avisos chan hubnotify.Notification) (hubnotify.Notification, bool) {
	select {
	case n := <-avisos:
		return n, true
	case <-time.After(400 * time.Millisecond):
		return hubnotify.Notification{}, false
	}
}

const anaAcme = "11111111-1111-4111-8111-111111111111"

func pedirALeire(t *testing.T, h *Handler, fichero string) *httptest.ResponseRecorder {
	t.Helper()
	return llamarJSON(t, h.pedirFirma, http.MethodPost, "/", anaAcme,
		`{"file_id":"`+fichero+`","path":"/`+map[string]string{"f": "Contrato.pdf", "g": "Otro.pdf"}[fichero]+`","email":"@leire","confirm":"`+leire+`"}`, nil)
}

// sinCambiosVisibles: dos respuestas de pedir se distinguen solo en lo que cambia
// siempre (id, fecha, fichero, ruta).
func sinCambiosVisibles(t *testing.T, a, b *httptest.ResponseRecorder) {
	t.Helper()
	var x, y map[string]any
	_ = json.Unmarshal(a.Body.Bytes(), &x)
	_ = json.Unmarshal(b.Body.Bytes(), &y)
	for _, k := range []string{"id", "created_at", "file_id", "path"} {
		delete(x, k)
		delete(y, k)
	}
	if a.Code != b.Code || fmt.Sprint(x) != fmt.Sprint(y) {
		t.Errorf("se distinguen: %d %v / %d %v", a.Code, x, b.Code, y)
	}
}

// TestBloquearAQuienPideFirma (ADR 0059): Leire bloquea a Ana desde su petición. La
// petición deja de llegarle, y las siguientes se «envían» igual pero no llegan: ni
// aviso, ni lista, ni se abren. Ana no nota nada. Y desde sus ajustes se quita.
func TestBloquearAQuienPideFirma(t *testing.T) {
	h, pet, ficheros, avisos := entornoFuera(t)
	normal := pedirALeire(t, h, "f")
	if normal.Code != http.StatusCreated {
		t.Fatalf("pedir: %d %s", normal.Code, normal.Body)
	}
	if n, ok := hubo(avisos); !ok || n.Subject != leire {
		t.Fatalf("sin aviso a Leire: %+v", n)
	}
	id := pet.filas[0].ID
	if rec := llamarJSON(t, h.bloquearDesdePeticion, http.MethodPost, "/", anaAcme, "", map[string]string{"id": id}); rec.Code != http.StatusNotFound {
		t.Errorf("Ana se bloquea en la de Leire: %d", rec.Code)
	}
	if rec := llamarJSON(t, h.bloquearDesdePeticion, http.MethodPost, "/", leire, "", map[string]string{"id": id}); rec.Code != http.StatusNoContent {
		t.Fatalf("bloquear: %d %s", rec.Code, rec.Body)
	}
	if !pet.filas[0].Silenciada {
		t.Error("la pendiente sigue llegando")
	}
	rec := llamarJSON(t, h.listarPeticionesFirma, http.MethodGet, "/", leire, "", nil)
	var lista struct {
		Received []store.PeticionFirma
		Blocked  []store.BloqueoFirma
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &lista)
	if len(lista.Received) != 0 || len(lista.Blocked) != 1 || lista.Blocked[0].Bloqueado != anaAcme || lista.Blocked[0].Nombre != "Ana" {
		t.Fatalf("lo que ve Leire: %s", rec.Body)
	}
	if rec := llamarJSON(t, h.abrirPeticionFirma, http.MethodPost, "/", leire, "", map[string]string{"id": id}); rec.Code != http.StatusNotFound {
		t.Errorf("se abre una bloqueada: %d", rec.Code)
	}

	// La siguiente: la misma respuesta, y nada llega.
	bloqueada := pedirALeire(t, h, "g")
	sinCambiosVisibles(t, normal, bloqueada)
	if n, ok := hubo(avisos); ok {
		t.Errorf("aviso a quien bloqueó: %+v", n)
	}
	if !pet.filas[1].Silenciada || len(ficheros) != 4 {
		t.Errorf("fila = %+v, ficheros %d (el enlace se crea igual)", pet.filas[1], len(ficheros))
	}
	// Ana la ve en su lista como cualquier otra.
	rec = llamarJSON(t, h.listarPeticionesFirma, http.MethodGet, "/", anaAcme, "", nil)
	if strings.Contains(rec.Body.String(), "silenc") || strings.Count(rec.Body.String(), `"status":"pendiente"`) != 2 {
		t.Errorf("lo que ve Ana: %s", rec.Body)
	}

	// Desde sus ajustes lo quita; lo que llegó silenciado sigue sin llegar.
	if rec := llamarJSON(t, h.desbloquearFirma, http.MethodDelete, "/", leire, "", map[string]string{"sub": "no"}); rec.Code != http.StatusNotFound {
		t.Errorf("sub inválido: %d", rec.Code)
	}
	if rec := llamarJSON(t, h.desbloquearFirma, http.MethodDelete, "/", leire, "", map[string]string{"sub": anaAcme}); rec.Code != http.StatusNoContent {
		t.Fatalf("desbloquear: %d", rec.Code)
	}
	if b, _ := pet.FirmaBloqueada(context.Background(), leire, anaAcme); b || !pet.filas[1].Silenciada {
		t.Error("sigue bloqueada, o la silenciada llegó")
	}
}

// TestLaPoliticaDeLaOrganizacion (ADR 0059): cerrada, las de fuera no llegan; de
// confianza, solo de esas organizaciones; y desde una organización que no las admite
// no se abre. Quien pide no nota nada.
func TestLaPoliticaDeLaOrganizacion(t *testing.T) {
	h, pet, _, avisos := entornoFuera(t)
	normal := pedirALeire(t, h, "f")
	hubo(avisos)
	pet.politicas = map[string]store.PoliticaFirma{"globex": {Modo: store.PoliticaCerrada}}
	cerrada := pedirALeire(t, h, "g")
	sinCambiosVisibles(t, normal, cerrada)
	if _, ok := hubo(avisos); ok || !pet.filas[1].Silenciada {
		t.Errorf("con globex cerrada llegó: %+v", pet.filas[1])
	}
	// Y la que ya estaba no se abre desde globex.
	ctx := context.WithValue(context.Background(), claveExterno{}, true)
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	r := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", pet.filas[0].ID)
	r = r.WithContext(authn.Into(context.WithValue(ctx, chi.RouteCtxKey, rc), authn.Claims{Subject: leire, Tenant: "globex"}))
	rec := httptest.NewRecorder()
	h.abrirPeticionFirma(rec, r)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "pdf_signature_org_closed") {
		t.Errorf("abrir con globex cerrada: %d %s", rec.Code, rec.Body)
	}

	pet.filas = pet.filas[:0]
	pet.politicas = map[string]store.PoliticaFirma{"globex": {Modo: store.PoliticaConfianza, Confianza: []string{"initech"}}}
	pedirALeire(t, h, "f")
	if _, ok := hubo(avisos); ok || !pet.filas[0].Silenciada {
		t.Error("de una organización que no es de confianza, llegó")
	}
	pet.politicas["globex"] = store.PoliticaFirma{Modo: store.PoliticaConfianza, Confianza: []string{"acme"}}
	pedirALeire(t, h, "g")
	if _, ok := hubo(avisos); !ok || pet.filas[1].Silenciada {
		t.Error("de una de confianza, no llegó")
	}

	// Sin saber sus organizaciones, no se crea nada: dejarla pasar se saltaría la política.
	h.idSecreto = "otro"
	pet.filas = pet.filas[:0]
	if rec := llamarJSON(t, h.pedirFirma, http.MethodPost, "/", anaAcme,
		`{"file_id":"f","path":"/Contrato.pdf","email":"@leire","confirm":"`+leire+`"}`, nil); rec.Code != http.StatusServiceUnavailable || len(pet.filas) != 0 {
		t.Errorf("sin Sytue ID: %d %s", rec.Code, rec.Body)
	}
}

// TestLaPoliticaSeValida: modos conocidos, organizaciones con nombre válido, sin la
// propia ni repetidas, y sin lista si no es de confianza.
func TestLaPoliticaSeValida(t *testing.T) {
	for _, c := range []struct {
		in     store.PoliticaFirma
		ok     bool
		quiero []string
	}{
		{store.PoliticaFirma{Modo: "abierta", Confianza: []string{"x"}}, true, []string{}},
		{store.PoliticaFirma{Modo: "cerrada"}, true, []string{}},
		{store.PoliticaFirma{Modo: "otra"}, false, nil},
		{store.PoliticaFirma{Modo: "confianza", Confianza: []string{" Initech ", "initech", "acme", "globex"}}, true, []string{"initech", "globex"}},
		{store.PoliticaFirma{Modo: "confianza", Confianza: []string{"no vale"}}, false, nil},
		{store.PoliticaFirma{Modo: "confianza", Confianza: make([]string, store.MaxConfianza+1)}, false, nil},
	} {
		got, err := politicaValida(c.in, "acme")
		if (err == nil) != c.ok || (c.ok && fmt.Sprint(got.Confianza) != fmt.Sprint(c.quiero)) {
			t.Errorf("%+v → %+v %v", c.in, got, err)
		}
	}
	h, pet, _, _ := entornoFuera(t)
	rec := llamarEn(t, h.guardarPoliticaFirma, http.MethodPut, "/", authn.Claims{Subject: anaAcme, Tenant: "acme"},
		`{"mode":"confianza","trusted":["globex"]}`, nil)
	if rec.Code != http.StatusOK || pet.politicas["acme"].Modo != "confianza" {
		t.Errorf("guardar: %d %s", rec.Code, rec.Body)
	}
	rec = llamarEn(t, h.politicaFirma, http.MethodGet, "/", authn.Claims{Subject: anaAcme, Tenant: "acme"}, "", nil)
	if !strings.Contains(rec.Body.String(), `"trusted":["globex"]`) {
		t.Errorf("leer: %s", rec.Body)
	}
}

// TestBloqueoYPoliticaSinOraculos (ADR 0059, revisión de seguridad): bloquear vale
// también tras rechazar; sin Sytue ID, una bloqueada da el mismo 503 que cualquiera;
// la cuenta personal admite aunque su organización cierre; firmar desde una
// organización cerrada no vale; y una copia que ya no pertenece a nada se borra.
func TestBloqueoYPoliticaSinOraculos(t *testing.T) {
	h, pet, ficheros, avisos := entornoFuera(t)
	pedirALeire(t, h, "f")
	hubo(avisos)
	id := map[string]string{"id": pet.filas[0].ID}
	if rec := llamarJSON(t, h.rechazarPeticionFirma, http.MethodPost, "/", leire, "", id); rec.Code != http.StatusOK {
		t.Fatalf("rechazar: %d", rec.Code)
	}
	if rec := llamarJSON(t, h.bloquearDesdePeticion, http.MethodPost, "/", leire, "", id); rec.Code != http.StatusNoContent {
		t.Fatalf("bloquear tras rechazar: %d %s", rec.Code, rec.Body)
	}
	// Sin Sytue ID, la bloqueada falla igual que las demás.
	h.idSecreto = "otro"
	if rec := pedirALeire(t, h, "g"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("bloqueada sin Sytue ID: %d (se distinguiría)", rec.Code)
	}
	h.idSecreto = "s"

	// Mikel: globex cerrada, pero con cuenta personal: le llega.
	pet.politicas = map[string]store.PoliticaFirma{"globex": {Modo: store.PoliticaCerrada}}
	pet.filas = pet.filas[:0]
	rec := llamarJSON(t, h.pedirFirma, http.MethodPost, "/", anaAcme,
		`{"file_id":"f","path":"/Contrato.pdf","email":"@mikel","confirm":"`+mikel+`"}`, nil)
	if rec.Code != http.StatusCreated || pet.filas[0].Silenciada {
		t.Fatalf("a quien tiene cuenta personal: %d %+v", rec.Code, pet.filas[0])
	}
	if _, ok := hubo(avisos); !ok {
		t.Error("sin aviso con cuenta personal")
	}
	// Firmar desde la sesión de globex (cerrada), no.
	ctx := context.WithValue(context.Background(), claveExterno{}, true)
	ctx = tenancy.Into(ctx, tenancy.Tenant{Slug: "acme"})
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"signed_file_id":"x","signed_path":"/x.pdf"}`))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", pet.filas[0].ID)
	r = r.WithContext(authn.Into(context.WithValue(ctx, chi.RouteCtxKey, rc), authn.Claims{Subject: mikel, Tenant: "globex"}))
	w := httptest.NewRecorder()
	h.firmarPeticion(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("firmar desde globex cerrada: %d %s", w.Code, w.Body)
	}

	// Una copia que ya no pertenece a ninguna petición se borra.
	pet.politicas = nil
	antes := len(ficheros)
	pet.fallarCopia = true
	if rec := llamarJSON(t, h.abrirPeticionFirma, http.MethodPost, "/", mikel, "", map[string]string{"id": pet.filas[0].ID}); rec.Code == http.StatusOK {
		t.Errorf("abrir con la petición desaparecida: %d", rec.Code)
	}
	if len(ficheros) != antes {
		t.Errorf("queda una copia huérfana: %d → %d", antes, len(ficheros))
	}
}
