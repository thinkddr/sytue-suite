// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

const (
	idScript = "5c1f0000-0000-4000-8000-000000000001"
	idDoc    = "0b6f0000-0000-4000-8000-000000000001"
)

// fakeScripts guarda lo justo para ejercitar las REGLAS DE ACCESO del manejador; las del
// almacén (estados, hash, cuotas, inmutabilidad, RLS) las prueba store/scripts_integration_test.go.
type fakeScripts struct {
	sc         domain.Script
	versiones  map[int]domain.VersionScript
	reservadas []domain.EjecucionScript
	denegadas  []string
	cuotaLlena bool
	revisada   *struct {
		v       int
		hash    string
		aprobar bool
	}
}

func (f *fakeScripts) CrearScript(_ context.Context, sc domain.Script, codigo string, m []byte) (domain.Script, domain.VersionScript, error) {
	sc.ID = idScript
	f.sc = sc
	v := domain.VersionScript{ScriptID: idScript, Version: 1, Code: codigo, CodeHash: domain.HashCodigoScript(codigo), Manifest: m, Status: domain.EstadoBorrador}
	f.versiones = map[int]domain.VersionScript{1: v}
	return sc, v, nil
}

func (f *fakeScripts) ListarScripts(context.Context, string, bool, int) ([]domain.Script, error) {
	return []domain.Script{f.sc}, nil
}

func (f *fakeScripts) ObtenerScript(context.Context, string) (domain.Script, error) { return f.sc, nil }

func (f *fakeScripts) VersionesScript(context.Context, string, int) ([]domain.VersionScript, error) {
	var out []domain.VersionScript
	for i := len(f.versiones); i >= 1; i-- {
		out = append(out, f.versiones[i])
	}
	return out, nil
}

func (f *fakeScripts) ObtenerVersionScript(_ context.Context, _ string, n int) (domain.VersionScript, error) {
	v, ok := f.versiones[n]
	if !ok {
		return v, domain.ErrNotFound
	}
	return v, nil
}

func (f *fakeScripts) NuevaVersionScript(_ context.Context, _, codigo string, m []byte, _ string) (domain.VersionScript, error) {
	n := len(f.versiones) + 1
	v := domain.VersionScript{ScriptID: idScript, Version: n, Code: codigo, CodeHash: domain.HashCodigoScript(codigo), Manifest: m, Status: domain.EstadoBorrador}
	f.versiones[n] = v
	return v, nil
}

func (f *fakeScripts) CambiarAmbitoScript(_ context.Context, _, a string) error {
	f.sc.Scope = a
	return nil
}
func (f *fakeScripts) BorrarScript(context.Context, string) error { return nil }
func (f *fakeScripts) ProponerVersionScript(context.Context, string, int) error {
	return nil
}

func (f *fakeScripts) RevisarVersionScript(_ context.Context, _ string, v int, hash string, ap bool, _ string) error {
	f.revisada = &struct {
		v       int
		hash    string
		aprobar bool
	}{v, hash, ap}
	return nil
}

func (f *fakeScripts) ReservarDenegada(_ context.Context, _ domain.EjecucionScript, codigo string) (int64, error) {
	f.denegadas = append(f.denegadas, codigo)
	return 1, nil
}

func (f *fakeScripts) ReservarEjecucion(_ context.Context, e domain.EjecucionScript, _ int) (int64, error) {
	if f.cuotaLlena {
		return 0, domain.DecidirCuota(domain.UsoCuota{EjecucionesUsuario: domain.CuotaEjecucionesUsuarioHora}, 1)
	}
	f.reservadas = append(f.reservadas, e)
	return int64(len(f.reservadas)), nil
}

func (f *fakeScripts) CerrarEjecucion(context.Context, int64, string, string, string, int, int) error {
	return nil
}

func (f *fakeScripts) EjecucionesScript(context.Context, string, int64, int) ([]domain.EjecucionScript, error) {
	return []domain.EjecucionScript{{ID: 7}}, nil
}

func (f *fakeScripts) UsoCuotaScripts(context.Context, string) (domain.UsoCuota, error) {
	return domain.UsoCuota{}, nil
}

var (
	ana   = &authn.Claims{Subject: "ana", Tenant: "acme"}
	beto  = &authn.Claims{Subject: "beto", Tenant: "acme"}
	admin = &authn.Claims{Subject: "root", Tenant: "acme", Roles: []string{"admin"}}
)

const (
	manifiestoLectura   = `{"v":1,"commands":["bold"],"read":["cells"],"write":false}`
	manifiestoEscritura = `{"v":1,"commands":["bold"],"read":["cells"],"write":true}`
)

func entornoScripts(kind domain.Kind, perms ...domain.Permission) (*Handler, *fakeScripts) {
	fs := &fakeStore{doc: &domain.Document{ID: idDoc, OwnerSub: "owner", Kind: kind, Title: "T"}, perms: perms}
	h := newHandler(fs)
	fk := &fakeScripts{}
	h.scripts = fk
	return h, fk
}

func permiso(sub string, rol domain.Role) domain.Permission {
	return domain.Permission{PrincipalKind: domain.PrincipalUser, PrincipalID: sub, Role: rol}
}

func llamarScripts(t *testing.T, f http.HandlerFunc, metodo string, quien *authn.Claims, params map[string]string, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f(rec, req(t, metodo, "/", quien, params, body))
	return rec
}

func crear(t *testing.T, h *Handler, quien *authn.Claims, scope, manifiesto string) {
	t.Helper()
	body := `{"name":"Negrita","kind":"spreadsheet","scope":"` + scope + `","code":"sytue.v1.run('bold')","manifest":` + manifiesto + `}`
	if rec := llamarScripts(t, h.crearScript, "POST", quien, nil, body); rec.Code != http.StatusCreated {
		t.Fatalf("crear: %d %s", rec.Code, rec.Body)
	}
}

func correr(t *testing.T, h *Handler, quien *authn.Claims, version string) *httptest.ResponseRecorder {
	t.Helper()
	return llamarScripts(t, h.ejecutarScript, "POST", quien,
		map[string]string{"sid": idScript, "v": version}, `{"document_id":"`+idDoc+`"}`)
}

// Un script de la organización no lo ejecuta nadie más hasta que un administrador aprueba
// ESA versión; el intento denegado queda en el registro.
func TestScriptDeOrganizacionNecesitaAprobacion(t *testing.T) {
	h, fk := entornoScripts(domain.KindSpreadsheet, permiso("beto", domain.RoleViewer), permiso("ana", domain.RoleViewer))
	crear(t, h, ana, "org", manifiestoLectura)
	fk.sc.ApprovedVersion = 0

	// Sin aprobar, ni siquiera lo ve (404): no existe para quien no lo revisa.
	if rec := correr(t, h, beto, "1"); rec.Code != http.StatusNotFound {
		t.Fatalf("beto ejecuta un script sin aprobar: %d %s", rec.Code, rec.Body)
	}
	// Aprobada la versión 1 (el almacén lo refleja en el script y en la versión).
	v := fk.versiones[1]
	v.Status = domain.EstadoAprobada
	fk.versiones[1] = v
	fk.sc.ApprovedVersion = 1
	if rec := correr(t, h, beto, "1"); rec.Code != http.StatusCreated {
		t.Fatalf("beto no puede ejecutar la versión aprobada: %d %s", rec.Code, rec.Body)
	}
	// Una versión nueva sale en borrador y NO hereda la aprobación.
	if rec := llamarScripts(t, h.nuevaVersionScript, "POST", ana, map[string]string{"sid": idScript},
		`{"code":"sytue.v1.run('italic')","manifest":`+manifiestoLectura+`}`); rec.Code != http.StatusCreated {
		t.Fatalf("nueva versión: %d %s", rec.Code, rec.Body)
	}
	rec := correr(t, h, beto, "2")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "script_not_approved") {
		t.Fatalf("la versión 2 sin aprobar se ejecutó o falló mal: %d %s", rec.Code, rec.Body)
	}
	if len(fk.denegadas) != 1 || fk.denegadas[0] != "not_approved" {
		t.Fatalf("el rechazo no quedó auditado: %v", fk.denegadas)
	}
	// El autor sí puede probar su borrador.
	if rec := correr(t, h, ana, "2"); rec.Code != http.StatusCreated {
		t.Fatalf("el autor no prueba su versión: %d %s", rec.Code, rec.Body)
	}
}

// Los scripts privados de otra persona no existen para nadie, administradores incluidos.
func TestScriptPrivadoAjenoNoExiste(t *testing.T) {
	h, _ := entornoScripts(domain.KindSpreadsheet, permiso("root", domain.RoleEditor), permiso("beto", domain.RoleEditor))
	crear(t, h, ana, "private", manifiestoLectura)
	for _, quien := range []*authn.Claims{beto, admin} {
		if rec := llamarScripts(t, h.verScript, "GET", quien, map[string]string{"sid": idScript}, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s ve un script privado ajeno: %d", quien.Subject, rec.Code)
		}
		if rec := correr(t, h, quien, "1"); rec.Code != http.StatusNotFound {
			t.Fatalf("%s ejecuta un script privado ajeno: %d", quien.Subject, rec.Code)
		}
		if rec := llamarScripts(t, h.borrarScript, "DELETE", quien, map[string]string{"sid": idScript}, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s borra un script privado ajeno: %d", quien.Subject, rec.Code)
		}
	}
}

// El permiso REAL sobre el documento manda: un manifiesto que escribe exige editor, y el
// script solo corre sobre su tipo de documento y su documento.
func TestEjecutarRespetaElPermisoDelDocumento(t *testing.T) {
	h, fk := entornoScripts(domain.KindSpreadsheet, permiso("ana", domain.RoleViewer))
	crear(t, h, ana, "private", manifiestoEscritura)
	if rec := correr(t, h, ana, "1"); rec.Code != http.StatusForbidden {
		t.Fatalf("una lectora ejecuta un script que escribe: %d %s", rec.Code, rec.Body)
	}
	if len(fk.reservadas) != 0 || len(fk.denegadas) != 1 || fk.denegadas[0] != "forbidden" {
		t.Fatalf("reservó o no auditó: res=%d den=%v", len(fk.reservadas), fk.denegadas)
	}

	// Con un manifiesto de solo lectura, lectora basta.
	h, _ = entornoScripts(domain.KindSpreadsheet, permiso("ana", domain.RoleViewer))
	crear(t, h, ana, "private", manifiestoLectura)
	if rec := correr(t, h, ana, "1"); rec.Code != http.StatusCreated {
		t.Fatalf("lectora con manifiesto de lectura: %d %s", rec.Code, rec.Body)
	}

	// Tipo de documento distinto.
	h, fk = entornoScripts(domain.KindDocument, permiso("ana", domain.RoleEditor))
	crear(t, h, ana, "private", manifiestoLectura)
	if rec := correr(t, h, ana, "1"); rec.Code != http.StatusForbidden || len(fk.denegadas) != 1 || fk.denegadas[0] != "wrong_document" {
		t.Fatalf("script de hoja sobre un documento de texto: %d %v", rec.Code, fk.denegadas)
	}
	// Atado a otro documento.
	h, fk = entornoScripts(domain.KindSpreadsheet, permiso("ana", domain.RoleEditor))
	crear(t, h, ana, "private", manifiestoLectura)
	fk.sc.DocumentID = "0b6f0000-0000-4000-8000-0000000000ff"
	if rec := correr(t, h, ana, "1"); rec.Code != http.StatusForbidden {
		t.Fatalf("script atado a otro documento: %d", rec.Code)
	}
	// Sin permiso alguno en el documento: tampoco.
	h, _ = entornoScripts(domain.KindSpreadsheet)
	crear(t, h, ana, "private", manifiestoLectura)
	if rec := correr(t, h, ana, "1"); rec.Code == http.StatusCreated {
		t.Fatalf("sin permiso en el documento se ejecutó")
	}
}

// Una versión ya guardada cuyo manifiesto declara un comando que los scripts no admiten
// (aquí, el relleno con IA) deja de poder ejecutarse, aunque esté aprobada, y el intento
// queda en el registro. Guardarla hoy tampoco se puede.
func TestVersionConComandoVetadoNoSeEjecutaNiSeGuarda(t *testing.T) {
	h, fk := entornoScripts(domain.KindSpreadsheet, permiso("ana", domain.RoleEditor), permiso("beto", domain.RoleEditor))
	crear(t, h, ana, "org", manifiestoLectura)
	for _, vetado := range []string{"aifill", "share"} {
		viejo := `{"v":1,"commands":["bold","` + vetado + `"],"read":[],"write":true,"scope":{},"max_ops":1000,"max_ms":5000,"unattended":false}`
		fk.versiones[1] = domain.VersionScript{
			ScriptID: idScript, Version: 1, Code: "x", CodeHash: domain.HashCodigoScript("x"),
			Manifest: []byte(viejo), Status: domain.EstadoAprobada,
		}
		fk.sc.ApprovedVersion = 1
		fk.denegadas, fk.reservadas = nil, nil
		for _, quien := range []*authn.Claims{ana, beto} {
			rec := correr(t, h, quien, "1")
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), vetado) {
				t.Fatalf("%s ejecuta una versión que declara %s: %d %s", quien.Subject, vetado, rec.Code, rec.Body)
			}
		}
		if len(fk.reservadas) != 0 || len(fk.denegadas) != 2 || fk.denegadas[0] != "manifest_not_allowed" {
			t.Fatalf("%s: reservó o no auditó: res=%d den=%v", vetado, len(fk.reservadas), fk.denegadas)
		}
		nuevo := `{"v":1,"commands":["` + vetado + `"],"read":[],"write":true}`
		if rec := llamarScripts(t, h.nuevaVersionScript, "POST", ana, map[string]string{"sid": idScript},
			`{"code":"x","manifest":`+nuevo+`}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), vetado) {
			t.Fatalf("se guardó una versión que declara %s: %d %s", vetado, rec.Code, rec.Body)
		}
	}
}

func TestEjecutarAgotaLaCuota(t *testing.T) {
	h, fk := entornoScripts(domain.KindSpreadsheet, permiso("ana", domain.RoleEditor))
	crear(t, h, ana, "private", manifiestoLectura)
	fk.cuotaLlena = true
	rec := correr(t, h, ana, "1")
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "quota_exceeded") {
		t.Fatalf("cuota llena: %d %s", rec.Code, rec.Body)
	}
}

func TestRevisarSoloAdministradoresYConHash(t *testing.T) {
	h, fk := entornoScripts(domain.KindSpreadsheet)
	crear(t, h, ana, "org", manifiestoLectura)
	hash := fk.versiones[1].CodeHash
	params := map[string]string{"sid": idScript, "v": "1"}
	body := `{"decision":"approve","code_hash":"` + hash + `"}`
	// Ni el autor ni un miembro cualquiera aprueban.
	for _, q := range []*authn.Claims{ana, beto} {
		if rec := llamarScripts(t, h.revisarVersionScript, "POST", q, params, body); rec.Code != http.StatusForbidden {
			t.Fatalf("%s aprueba sin ser administrador: %d", q.Subject, rec.Code)
		}
	}
	// Hash mal formado: 400 sin llegar al almacén.
	if rec := llamarScripts(t, h.revisarVersionScript, "POST", admin, params, `{"decision":"approve","code_hash":"abc"}`); rec.Code != http.StatusBadRequest || fk.revisada != nil {
		t.Fatalf("hash inválido: %d", rec.Code)
	}
	if rec := llamarScripts(t, h.revisarVersionScript, "POST", admin, params, body); rec.Code != http.StatusNoContent {
		t.Fatalf("admin aprueba: %d %s", rec.Code, rec.Body)
	}
	if fk.revisada == nil || fk.revisada.hash != hash || !fk.revisada.aprobar {
		t.Fatalf("la aprobación no llevó el hash: %+v", fk.revisada)
	}
	// Un script privado no se revisa.
	h, _ = entornoScripts(domain.KindSpreadsheet)
	crear(t, h, admin, "private", manifiestoLectura)
	if rec := llamarScripts(t, h.revisarVersionScript, "POST", admin, params, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("revisar un privado: %d", rec.Code)
	}
}

func TestSoloElAutorEditaYPropone(t *testing.T) {
	h, _ := entornoScripts(domain.KindSpreadsheet)
	crear(t, h, ana, "org", manifiestoLectura)
	fakeAprobado := h.scripts.(*fakeScripts)
	fakeAprobado.sc.ApprovedVersion = 1 // visible para los demás
	params := map[string]string{"sid": idScript, "v": "1"}
	if rec := llamarScripts(t, h.nuevaVersionScript, "POST", beto, params, `{"code":"x","manifest":`+manifiestoLectura+`}`); rec.Code != http.StatusForbidden {
		t.Fatalf("beto edita el script de Ana: %d", rec.Code)
	}
	if rec := llamarScripts(t, h.proponerVersionScript, "POST", beto, params, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("beto propone el script de Ana: %d", rec.Code)
	}
	if rec := llamarScripts(t, h.ambitoScript, "PUT", beto, params, `{"scope":"private"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("beto cambia el ámbito: %d", rec.Code)
	}
	if rec := llamarScripts(t, h.borrarScript, "DELETE", beto, params, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("beto borra el script de Ana: %d", rec.Code)
	}
	if rec := llamarScripts(t, h.borrarScript, "DELETE", admin, params, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("un administrador retira un script de la organización: %d", rec.Code)
	}
}

func TestVerScriptNoEnseñaBorradoresAAjenos(t *testing.T) {
	h, fk := entornoScripts(domain.KindSpreadsheet)
	crear(t, h, ana, "org", manifiestoLectura)
	llamarScripts(t, h.nuevaVersionScript, "POST", ana, map[string]string{"sid": idScript}, `{"code":"y","manifest":`+manifiestoLectura+`}`)
	v := fk.versiones[1]
	v.Status = domain.EstadoAprobada
	fk.versiones[1] = v
	fk.sc.ApprovedVersion = 1
	versiones := func(q *authn.Claims) int {
		rec := llamarScripts(t, h.verScript, "GET", q, map[string]string{"sid": idScript}, "")
		var got struct {
			Versions []domain.VersionScript `json:"versions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return len(got.Versions)
	}
	if n := versiones(beto); n != 1 {
		t.Fatalf("beto ve %d versiones, quiero solo la aprobada", n)
	}
	if n := versiones(ana); n != 2 {
		t.Fatalf("la autora ve %d versiones, quiero 2", n)
	}
	if n := versiones(admin); n != 2 {
		t.Fatalf("el administrador ve %d versiones, quiero 2", n)
	}
}

func TestManifiestoYCodigoInvalidosSeRechazan(t *testing.T) {
	h, _ := entornoScripts(domain.KindSpreadsheet)
	casos := map[string]string{
		"campo desconocido":   `{"v":1,"commands":[],"read":[],"write":false,"root":true}`,
		"versión":             `{"v":2,"commands":[],"read":[],"write":false}`,
		"comando raro":        `{"v":1,"commands":["a;b"],"read":[],"write":false}`,
		"lectura desconocida": `{"v":1,"commands":[],"read":["cookies"],"write":false}`,
		"tiempo desmedido":    `{"v":1,"commands":[],"read":[],"write":false,"max_ms":999999}`,
		"rango":               `{"v":1,"commands":[],"read":[],"write":false,"scope":{"ranges":["A1:ZZZZ9"]}}`,
	}
	for nombre, m := range casos {
		body := `{"name":"x","kind":"spreadsheet","code":"1","manifest":` + m + `}`
		if rec := llamarScripts(t, h.crearScript, "POST", ana, nil, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", nombre, rec.Code, rec.Body)
		}
	}
	if rec := llamarScripts(t, h.crearScript, "POST", ana, nil, `{"name":"x","kind":"spreadsheet","code":"","manifest":`+manifiestoLectura+`}`); rec.Code != http.StatusBadRequest {
		t.Errorf("código vacío: %d", rec.Code)
	}
	if rec := llamarScripts(t, h.crearScript, "POST", ana, nil, `{"name":"x","kind":"hoja","code":"1","manifest":`+manifiestoLectura+`}`); rec.Code != http.StatusBadRequest {
		t.Errorf("tipo desconocido: %d", rec.Code)
	}
}

func TestAuditoriaDeScriptsSoloAdministradoresYPropietario(t *testing.T) {
	h, _ := entornoScripts(domain.KindSpreadsheet, permiso("beto", domain.RoleEditor))
	if rec := llamarScripts(t, h.auditoriaScriptsOrg, "GET", beto, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("un miembro lee la auditoría de la organización: %d", rec.Code)
	}
	if rec := llamarScripts(t, h.auditoriaScriptsOrg, "GET", admin, nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("admin: %d", rec.Code)
	}
	p := map[string]string{"id": idDoc}
	if rec := llamarScripts(t, h.auditoriaScriptsDoc, "GET", beto, p, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("un editor lee la auditoría del documento: %d", rec.Code)
	}
	if rec := llamarScripts(t, h.auditoriaScriptsDoc, "GET", ownerClaims(), p, ""); rec.Code != http.StatusOK {
		t.Fatalf("propietario: %d", rec.Code)
	}
	// limit fuera de rango.
	rec := httptest.NewRecorder()
	h.auditoriaScriptsOrg(rec, req(t, "GET", "/?limit=100000", admin, nil, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("limit desmedido: %d", rec.Code)
	}
}
