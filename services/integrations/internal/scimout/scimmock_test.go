// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package scimout

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// scimMock es un servidor SCIM 2.0 en memoria (httptest) para las pruebas offline:
// implementa /Users y /Groups (create/replace/patch/delete + filtro `userName eq` /
// `displayName eq`), con inyección de códigos (409, 429 con Retry-After, 5xx) y
// contadores por operación para afirmar idempotencia.
type scimMock struct {
	mu     sync.Mutex
	users  map[string]map[string]any // id → recurso
	groups map[string]map[string]any
	seq    int

	// Contadores de operaciones de escritura (idempotencia: 0 en un segundo pase).
	posts, puts, patches, deletes int
	// reqs cuenta TODAS las peticiones recibidas (para el pase idempotente).
	reqs int

	// fails es una cola de respuestas a servir ANTES del handler real (transitorios).
	fails []failResp
}

type failResp struct {
	status     int
	retryAfter string
}

func newSCIMMock() *scimMock {
	return &scimMock{users: map[string]map[string]any{}, groups: map[string]map[string]any{}}
}

// seedUser inserta un usuario preexistente (para probar la ADOPCIÓN por filtro).
func (m *scimMock) seedUser(userName string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "srv-user-existing"
	m.users[id] = map[string]any{"id": id, "userName": userName, "active": true}
	return id
}

func (m *scimMock) resetCounts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.posts, m.puts, m.patches, m.deletes, m.reqs = 0, 0, 0, 0, 0
}

func (m *scimMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.reqs++
	if len(m.fails) > 0 {
		f := m.fails[0]
		m.fails = m.fails[1:]
		m.mu.Unlock()
		if f.retryAfter != "" {
			w.Header().Set("Retry-After", f.retryAfter)
		}
		w.WriteHeader(f.status)
		return
	}
	m.mu.Unlock()

	switch {
	case strings.HasPrefix(r.URL.Path, "/Users"):
		m.serveUsers(w, r)
	case strings.HasPrefix(r.URL.Path, "/Groups"):
		m.serveGroups(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *scimMock) serveUsers(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/Users/")
	switch r.Method {
	case http.MethodGet:
		want := filterValue(r.URL.Query().Get("filter"))
		m.mu.Lock()
		defer m.mu.Unlock()
		var res []map[string]any
		for _, u := range m.users {
			if want == "" || u["userName"] == want {
				res = append(res, u)
			}
		}
		writeList(w, res)
	case http.MethodPost:
		body := decode(r)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.posts++
		m.seq++
		newID := scimID("srv-user", m.seq)
		body["id"] = newID
		m.users[newID] = body
		writeSCIM(w, http.StatusCreated, body)
	case http.MethodPut:
		body := decode(r)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.puts++
		body["id"] = id
		m.users[id] = body
		writeSCIM(w, http.StatusOK, body)
	case http.MethodPatch:
		body := decode(r)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.patches++
		applyActivePatch(m.users[id], body)
		writeSCIM(w, http.StatusOK, m.users[id])
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *scimMock) serveGroups(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/Groups/")
	switch r.Method {
	case http.MethodGet:
		want := filterValue(r.URL.Query().Get("filter"))
		m.mu.Lock()
		defer m.mu.Unlock()
		var res []map[string]any
		for _, g := range m.groups {
			if want == "" || g["displayName"] == want {
				res = append(res, g)
			}
		}
		writeList(w, res)
	case http.MethodPost:
		body := decode(r)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.posts++
		m.seq++
		newID := scimID("srv-group", m.seq)
		body["id"] = newID
		m.groups[newID] = body
		writeSCIM(w, http.StatusCreated, body)
	case http.MethodPut:
		body := decode(r)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.puts++
		body["id"] = id
		m.groups[id] = body
		writeSCIM(w, http.StatusOK, body)
	case http.MethodDelete:
		m.mu.Lock()
		defer m.mu.Unlock()
		m.deletes++
		delete(m.groups, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// applyActivePatch aplica un PATCH SCIM {op:replace, path:active, value:bool}.
func applyActivePatch(res, patch map[string]any) {
	if res == nil {
		return
	}
	ops, _ := patch["Operations"].([]any)
	for _, o := range ops {
		op, _ := o.(map[string]any)
		if strings.EqualFold(str(op["op"]), "replace") && strings.EqualFold(str(op["path"]), "active") {
			res["active"] = op["value"]
		}
	}
}

func writeList(w http.ResponseWriter, resources []map[string]any) {
	if resources == nil {
		resources = []map[string]any{}
	}
	writeSCIM(w, http.StatusOK, map[string]any{
		"schemas":      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": len(resources),
		"Resources":    resources,
	})
}

func writeSCIM(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request) map[string]any {
	var m map[string]any
	_ = json.NewDecoder(r.Body).Decode(&m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

// filterValue extrae el valor de un filtro `attr eq "valor"`.
func filterValue(f string) string {
	i := strings.Index(f, `"`)
	j := strings.LastIndex(f, `"`)
	if i < 0 || j <= i {
		return ""
	}
	return f[i+1 : j]
}

func scimID(prefix string, n int) string { return prefix + "-" + itoa(n) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
