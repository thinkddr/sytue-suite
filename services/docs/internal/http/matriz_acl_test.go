// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Matriz ACL única de Docus, Calcus y Diapos (docs/security.md#matriz-acl). La tabla de ese
// documento ES la fuente: este test la lee y comprueba, ruta por ruta y rol por rol, que
// el servicio concede y deniega exactamente lo que dice. Si cambia una autorización sin
// cambiar el documento (o al revés), falla. Y falla también si una ruta de la tabla no
// tiene su manejador aquí o al revés, para que la tabla no quede coja.
//
// Lo que se comprueba es la frontera de autorización: «sí» = la petición pasa del
// control de acceso (cualquier respuesta menos 403/404, incluido que el fake no tenga
// dependencias y el manejador se estrelle ya dentro); «no» = 403 sin llegar a tocar nada.
// Los efectos de cada operación se prueban en sus propios tests.

const cuerpoOps = `{"ops":[{"kind":"insert","dot":{"a":"x","c":1},"value":"\"H\"","lamport":1}]}`

type rutaACL struct {
	h      func(*Handler) http.HandlerFunc
	target string
	params map[string]string
	body   string
}

var rutasACL = map[string]rutaACL{
	"GET /documents/{id}":                               {h: func(h *Handler) http.HandlerFunc { return h.getDocument }},
	"PATCH /documents/{id}":                             {h: func(h *Handler) http.HandlerFunc { return h.renameDocument }, body: `{"title":"Otro"}`},
	"DELETE /documents/{id}":                            {h: func(h *Handler) http.HandlerFunc { return h.deleteDocument }},
	"PUT /documents/{id}/star":                          {h: func(h *Handler) http.HandlerFunc { return h.starDocument }, body: `{"starred":true}`},
	"GET /documents/{id}/state":                         {h: func(h *Handler) http.HandlerFunc { return h.getState }},
	"POST /documents/{id}/ops":                          {h: func(h *Handler) http.HandlerFunc { return h.postOps }, body: cuerpoOps},
	"GET /documents/{id}/ops":                           {h: func(h *Handler) http.HandlerFunc { return h.getOps }},
	"PUT /documents/{id}/presence":                      {h: func(h *Handler) http.HandlerFunc { return h.putPresence }, body: `{"cursor":{}}`},
	"GET /documents/{id}/versions":                      {h: func(h *Handler) http.HandlerFunc { return h.listVersions }},
	"POST /documents/{id}/versions":                     {h: func(h *Handler) http.HandlerFunc { return h.createVersion }, body: `{}`},
	"POST /documents/{id}/versions/{versionId}/restore": {h: func(h *Handler) http.HandlerFunc { return h.restoreVersion }, params: map[string]string{"versionId": "v1"}},
	"GET /documents/{id}/comments":                      {h: func(h *Handler) http.HandlerFunc { return h.listComments }},
	"POST /documents/{id}/comments":                     {h: func(h *Handler) http.HandlerFunc { return h.createComment }, body: `{"body":"hola"}`},
	"POST /documents/{id}/comments/import":              {h: func(h *Handler) http.HandlerFunc { return h.importComments }, body: `{"comments":[]}`},
	"PUT /documents/{id}/comments/{commentId}/resolve":  {h: func(h *Handler) http.HandlerFunc { return h.resolveComment }, params: map[string]string{"commentId": "c1"}, body: `{"resolved":true}`},
	"DELETE /documents/{id}/comments/{commentId}":       {h: func(h *Handler) http.HandlerFunc { return h.deleteComment }, params: map[string]string{"commentId": "c1"}},
	"GET /documents/{id}/permissions":                   {h: func(h *Handler) http.HandlerFunc { return h.listPermissions }},
	"PUT /documents/{id}/permissions":                   {h: func(h *Handler) http.HandlerFunc { return h.upsertPermission }, body: `{"principal_kind":"user","principal_id":"u2","role":"viewer"}`},
	"DELETE /documents/{id}/permissions":                {h: func(h *Handler) http.HandlerFunc { return h.revokePermission }, target: "?principal_kind=user&principal_id=u2"},
	"GET /documents/{id}/shares":                        {h: func(h *Handler) http.HandlerFunc { return h.listShares }},
	"POST /documents/{id}/shares":                       {h: func(h *Handler) http.HandlerFunc { return h.createShare }, body: `{"role":"viewer"}`},
	"DELETE /documents/{id}/shares/{linkId}":            {h: func(h *Handler) http.HandlerFunc { return h.revokeShare }, params: map[string]string{"linkId": "l1"}},
	"PUT /documents/{id}/shares/{linkId}/password":      {h: func(h *Handler) http.HandlerFunc { return h.setSharePassword }, params: map[string]string{"linkId": "l1"}, body: `{"password":"contraseña-de-prueba"}`},
	"POST /documents/{id}/edit-requests":                {h: func(h *Handler) http.HandlerFunc { return h.pedirEdicion }},
	"GET /documents/{id}/edit-requests":                 {h: func(h *Handler) http.HandlerFunc { return h.listarPeticiones }},
	"POST /documents/{id}/edit-requests/{sub}/grant":    {h: func(h *Handler) http.HandlerFunc { return h.concederPeticion }, params: map[string]string{"sub": "u2"}},
	"DELETE /documents/{id}/edit-requests/{sub}":        {h: func(h *Handler) http.HandlerFunc { return h.rechazarPeticion }, params: map[string]string{"sub": "u2"}},
	"GET /documents/{id}/filter-views":                  {h: func(h *Handler) http.HandlerFunc { return h.getVistasFiltro }},
	"PUT /documents/{id}/filter-views":                  {h: func(h *Handler) http.HandlerFunc { return h.putVistasFiltro }, body: `{"state":{"views":{},"active":{}}}`},
	"GET /documents/{id}/exports":                       {h: func(h *Handler) http.HandlerFunc { return h.listExports }},
	"POST /documents/{id}/exports":                      {h: func(h *Handler) http.HandlerFunc { return h.createExport }, body: `{"format":"html"}`},
	"GET /documents/{id}/exports/{blobId}":              {h: func(h *Handler) http.HandlerFunc { return h.downloadExport }, params: map[string]string{"blobId": "b1"}},
}

var reFilaACL = regexp.MustCompile("^\\|\\s*`([A-Z]+ /documents[^`]*)`\\s*\\|(.*)\\|\\s*$")

// leerMatrizACL devuelve, por ruta, el rol mínimo que dice la tabla de docs/security.md.
func leerMatrizACL(t *testing.T) map[string]domain.Role {
	t.Helper()
	b, err := os.ReadFile("../../../../docs/security.md")
	if err != nil {
		t.Fatalf("no se puede leer docs/security.md: %v", err)
	}
	texto := string(b)
	i := strings.Index(texto, "<!-- matriz-acl:documentos:inicio -->")
	j := strings.Index(texto, "<!-- matriz-acl:documentos:fin -->")
	if i < 0 || j < i {
		t.Fatal("docs/security.md no tiene la tabla de la matriz ACL de documentos (marcas matriz-acl:documentos)")
	}
	out := map[string]domain.Role{}
	for _, linea := range strings.Split(texto[i:j], "\n") {
		m := reFilaACL.FindStringSubmatch(strings.TrimSpace(linea))
		if m == nil {
			continue
		}
		// columnas tras la ruta: acción | rol mínimo | notas
		celdas := strings.Split(m[2], "|")
		if len(celdas) < 2 {
			continue
		}
		rol := domain.Role(strings.Trim(strings.TrimSpace(celdas[1]), "`"))
		out[m[1]] = rol
	}
	return out
}

func TestMatrizACLDeDocumentosCoincideConElServicio(t *testing.T) {
	matriz := leerMatrizACL(t)
	if len(matriz) == 0 {
		t.Fatal("la tabla de la matriz ACL está vacía")
	}
	for ruta := range matriz {
		if _, ok := rutasACL[ruta]; !ok {
			t.Errorf("la matriz del documento menciona %q y el test no sabe ejercitarla", ruta)
		}
	}
	for ruta := range rutasACL {
		if _, ok := matriz[ruta]; !ok {
			t.Errorf("el test ejercita %q y la matriz del documento no lo dice", ruta)
		}
	}
	roles := []domain.Role{"", domain.RoleViewer, domain.RoleCommenter, domain.RoleEditor, domain.RoleOwner}
	for ruta, minimo := range matriz {
		r, ok := rutasACL[ruta]
		if !ok {
			continue
		}
		for _, kind := range []domain.Kind{domain.KindDocument, domain.KindSpreadsheet, domain.KindPresentation} {
			for _, rol := range roles {
				t.Run(ruta+"/"+string(kind)+"/"+string(rol), func(t *testing.T) {
					fs := &fakeStore{doc: &domain.Document{ID: "d1", Kind: kind, OwnerSub: "otro"}}
					if rol != "" {
						fs.perms = []domain.Permission{{PrincipalKind: domain.PrincipalUser, PrincipalID: "owner", Role: rol}}
					}
					params := map[string]string{"id": "d1"}
					for k, v := range r.params {
						params[k] = v
					}
					target := "/v1/x" + r.target
					rec := httptest.NewRecorder()
					metodo := strings.SplitN(ruta, " ", 2)[0]
					pasa := ejecutarSinPanico(func() {
						r.h(newHandler(fs))(rec, req(t, metodo, target, ownerClaims(), params, r.body))
					})
					permitido := rol != "" && rol.AtLeast(minimo)
					autorizado := !pasa || (rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound)
					if autorizado != permitido {
						t.Fatalf("rol %q en %s (mínimo %q): HTTP %d, se estrelló dentro=%v; la matriz dice permitido=%v", rol, ruta, minimo, rec.Code, !pasa, permitido)
					}
				})
			}
		}
	}
}

// ejecutarSinPanico: false si f se estrelló. Un manejador de un fake sin dependencias
// (S3, hub de avisos) puede estrellarse DESPUÉS de autorizar; eso cuenta como «pasó».
func ejecutarSinPanico(f func()) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	f()
	return true
}
