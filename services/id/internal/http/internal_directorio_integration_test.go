// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// La puerta sin sesión del directorio (reconstrucción de menciones de Docs): con su
// secreto propio, solo la organización que se pide, y sin montar si no hay secreto.
func TestDirectorioInterno(t *testing.T) {
	l := newLiveServer(t)
	cierre := peticiones.DirectorioInternoCierre
	peticiones.DirectorioInternoCierre = time.Now().Add(time.Hour)
	defer func() { peticiones.DirectorioInternoCierre = cierre }()
	nueva := func(slug string) domain.Organization {
		o, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: slug, Name: slug})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	acme, globex := nueva("acme-dir"), nueva("globex-dir")
	alta := func(org domain.Organization, email string) domain.User {
		u, err := l.st.CreateUser(l.ctx, nil, domain.User{Email: email, EmailVerified: true, Name: email})
		if err != nil {
			t.Fatal(err)
		}
		if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
			t.Fatal(err)
		}
		return u
	}
	ana := alta(acme, "ana@dir.test")
	alta(globex, "zoe@dir.test")

	puerta := httpx.SoloDesdeDentro("s")(http.HandlerFunc(l.srv.handleInternalDirectoryResolve))
	pedir := func(secreto, cuerpo string) (*httptest.ResponseRecorder, map[string]string) {
		r := httptest.NewRequest(http.MethodPost, "/internal/directory/resolve", strings.NewReader(cuerpo))
		r.Header.Set("Authorization", "Bearer "+secreto)
		w := httptest.NewRecorder()
		puerta.ServeHTTP(w, r)
		var v struct {
			Users map[string]string `json:"users"`
		}
		_ = json.NewDecoder(w.Body).Decode(&v)
		return w, v.Users
	}

	if w, u := pedir("s", `{"org":"acme-dir","emails":["Ana@dir.test","zoe@dir.test","no-es-correo"]}`); w.Code != http.StatusOK || len(u) != 1 || u["ana@dir.test"] != ana.ID {
		t.Fatalf("acme: %d %v; quiero solo a Ana", w.Code, u)
	}
	if w, u := pedir("s", `{"org":"no-existe","emails":["ana@dir.test"]}`); w.Code != http.StatusOK || len(u) != 0 {
		t.Fatalf("organización que no existe: %d %v; quiero vacío", w.Code, u)
	}
	if w, _ := pedir("otro", `{"org":"acme-dir","emails":["ana@dir.test"]}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("con otro secreto: %d, quiero 401", w.Code)
	}
	muchas := `{"org":"acme-dir","emails":["` + strings.Repeat(`a@b.es","`, maxDireccionesDirectorio) + `a@b.es"]}`
	if w, _ := pedir("s", muchas); w.Code != http.StatusBadRequest {
		t.Fatalf("más de %d direcciones: %d, quiero 400", maxDireccionesDirectorio, w.Code)
	}
	// Desde fuera (Traefik pone X-Forwarded-For) no existe, aunque traiga el secreto.
	fuera := httptest.NewRequest(http.MethodPost, "/internal/directory/resolve", strings.NewReader(`{"org":"acme-dir","emails":["ana@dir.test"]}`))
	fuera.Header.Set("Authorization", "Bearer s")
	fuera.Header.Set("X-Forwarded-For", "203.0.113.9")
	wf := httptest.NewRecorder()
	puerta.ServeHTTP(wf, fuera)
	if wf.Code != http.StatusNotFound {
		t.Fatalf("desde internet: %d, quiero 404", wf.Code)
	}
	// Y sin el secreto bueno, también 404: desde fuera ni se confirma que existe.
	fuera.Header.Set("Authorization", "Bearer otro")
	wf = httptest.NewRecorder()
	puerta.ServeHTTP(wf, fuera)
	if wf.Code != http.StatusNotFound {
		t.Fatalf("desde internet con otro secreto: %d, quiero 404 (un 401 confirma la ruta)", wf.Code)
	}
	// Una organización suspendida no tiene a nadie.
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE organizations SET status = 'suspended' WHERE id = $1`, globex.ID); err != nil {
		t.Fatal(err)
	}
	if w, u := pedir("s", `{"org":"globex-dir","emails":["zoe@dir.test"]}`); w.Code != http.StatusOK || len(u) != 0 {
		t.Fatalf("organización suspendida: %d %v; quiero vacío", w.Code, u)
	}
	// Quien colabora desde otra organización: sale solo si es uno de los candidatos, y
	// nunca alguien de la propia organización (esos salen por `users`).
	zoe := alta(globex, "zoe-glob@dir.test")
	pedirCon := func(cuerpo string) peticiones.Resueltas {
		r := httptest.NewRequest(http.MethodPost, "/internal/directory/resolve", strings.NewReader(cuerpo))
		r.Header.Set("Authorization", "Bearer s")
		w := httptest.NewRecorder()
		puerta.ServeHTTP(w, r)
		var v peticiones.Resueltas
		_ = json.NewDecoder(w.Body).Decode(&v)
		if w.Code != http.StatusOK {
			t.Fatalf("con candidatos: %d", w.Code)
		}
		return v
	}
	if got := pedirCon(`{"org":"acme-dir","candidatos":{"zoe-glob@dir.test":["` + zoe.ID + `"]}}`); got.ConPermiso["zoe-glob@dir.test"] != zoe.ID || len(got.Miembros) != 0 {
		t.Fatalf("de fuera con su sub entre los candidatos: %+v, quiero a Zoe", got)
	}
	if got := pedirCon(`{"org":"acme-dir","candidatos":{"zoe-glob@dir.test":["` + ana.ID + `"]}}`); len(got.ConPermiso) != 0 {
		t.Fatalf("de fuera sin su sub entre los candidatos: %+v, quiero nada", got)
	}
	if got := pedirCon(`{"org":"acme-dir","emails":["zoe-glob@dir.test"]}`); len(got.ConPermiso)+len(got.Miembros) != 0 {
		t.Fatalf("sin candidatos: %+v, quiero nada", got)
	}
	if got := pedirCon(`{"org":"acme-dir","candidatos":{"ana@dir.test":["` + ana.ID + `"]}}`); len(got.ConPermiso) != 0 {
		t.Fatalf("alguien de acme por la vía de fuera: %+v, quiero nada", got)
	}
	muchos := func(n int) string {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = `"` + uuid.NewString() + `"`
		}
		return strings.Join(ids, ",")
	}
	// Lo que no es un sub no cuenta, ni siquiera para el tope: 200 buenos y uno malo pasan.
	var justos []string
	for i := range peticiones.MaxCandidatosDirectorio / peticiones.MaxCandidatosPorDireccion {
		justos = append(justos, fmt.Sprintf(`"j%d@b.es":[%s]`, i, muchos(peticiones.MaxCandidatosPorDireccion)))
	}
	if w, _ := pedir("s", `{"org":"acme-dir","candidatos":{`+strings.Join(justos, ",")+`,"x@b.es":["no-es-un-uuid",""]}}`); w.Code != http.StatusOK {
		t.Fatalf("candidatos que no son un sub cuentan: %d, quiero 200", w.Code)
	}
	// Una dirección que es el correo de entrar de alguien de acme y el buzón de alguien de
	// otra organización no es de nadie: tampoco por `users`.
	if _, err := l.st.DB().Exec(l.ctx, `INSERT INTO org_mailboxes (org_id, user_id, address, primary_address)
		VALUES ($1, $2, 'ana@dir.test', false)`, globex.ID, zoe.ID); err != nil {
		t.Fatal(err)
	}
	if got := pedirCon(`{"org":"acme-dir","emails":["ana@dir.test"]}`); len(got.Miembros) != 0 {
		t.Fatalf("una dirección de dos personas salió por users: %+v", got)
	}
	// Quién sigue en la organización: de los `subs` dados, solo los de acme.
	if got := pedirCon(`{"org":"acme-dir","subs":["` + ana.ID + `","` + zoe.ID + `","basura"]}`); !slices.Equal(got.Activos, []string{ana.ID}) {
		t.Fatalf("activos %v, quiero solo a Ana", got.Activos)
	}
	if w, _ := pedir("s", `{"org":"acme-dir","candidatos":{"a@b.es":[`+muchos(peticiones.MaxCandidatosPorDireccion+1)+`]}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("más de %d candidatos para una dirección: %d, quiero 400", peticiones.MaxCandidatosPorDireccion, w.Code)
	}
	var cinco []string
	for i := range 5 {
		cinco = append(cinco, fmt.Sprintf(`"c%d@b.es":[%s]`, i, muchos(peticiones.MaxCandidatosPorDireccion)))
	}
	if w, _ := pedir("s", `{"org":"acme-dir","candidatos":{`+strings.Join(cinco, ",")+`}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("más de %d candidatos en total: %d, quiero 400", peticiones.MaxCandidatosDirectorio, w.Code)
	}
	// Candidatos y `subs` juntos no: se cobran en limitadores distintos.
	if w, _ := pedir("s", `{"org":"acme-dir","candidatos":{"a@b.es":[`+muchos(1)+`]},"subs":[`+muchos(1)+`]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("candidatos y subs en la misma llamada: %d, quiero 400", w.Code)
	}
	// Las vías que responden por elemento se cobran por elemento (en otra organización,
	// para no chocar con su límite por llamadas).
	nueva("cobro-dir")
	cobro := func(cuerpo string) int {
		r := httptest.NewRequest(http.MethodPost, "/internal/directory/resolve", strings.NewReader(cuerpo))
		r.Header.Set("Authorization", "Bearer s")
		w := httptest.NewRecorder()
		puerta.ServeHTTP(w, r)
		return w.Code
	}
	parejas := func() string {
		var dirs []string
		for i := range peticiones.MaxCandidatosDirectorio / peticiones.MaxCandidatosPorDireccion {
			dirs = append(dirs, fmt.Sprintf(`"k%d@b.es":[%s]`, i, muchos(peticiones.MaxCandidatosPorDireccion)))
		}
		return `{"org":"cobro-dir","candidatos":{` + strings.Join(dirs, ",") + `}}`
	}
	var codigos []int
	for range 3 {
		codigos = append(codigos, cobro(parejas()))
	}
	// Cobrando por llamada, tres nunca se acabarían una ráfaga de 400: alguna tiene que ser
	// 429 (y la primera no, que el cubo lleva ya un rato recargando lo de antes).
	if !slices.Contains(codigos, http.StatusTooManyRequests) {
		t.Fatalf("200 parejas por llamada contra 400: %v, alguna tenía que ser 429", codigos)
	}
	codigos = nil
	for range 4 {
		codigos = append(codigos, cobro(`{"org":"cobro-dir","subs":[`+muchos(peticiones.MaxSubsDirectorio)+`]}`))
	}
	// Cobrando por llamada, cuatro no se acabarían una ráfaga de 3000.
	if codigos[0] != http.StatusOK || !slices.Contains(codigos, http.StatusTooManyRequests) {
		t.Fatalf("1000 subs por llamada contra 3000: %v, la primera pasa y alguna tenía que ser 429", codigos)
	}
	// El límite por organización: agotado, 429 (y otra organización sigue pasando).
	for range 25 {
		pedir("s", `{"org":"acme-dir","emails":["ana@dir.test"]}`)
	}
	if w, _ := pedir("s", `{"org":"acme-dir","emails":["ana@dir.test"]}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("agotado el límite de la organización: %d, quiero 429", w.Code)
	}
	// Pasado el cierre, 410.
	peticiones.DirectorioInternoCierre = time.Now().Add(-time.Minute)
	if w, _ := pedir("s", `{"org":"acme-dir","emails":["ana@dir.test"]}`); w.Code != http.StatusGone {
		t.Fatalf("pasado el cierre: %d, quiero 410", w.Code)
	}
	// Sin DirectorioSecret (el servidor del harness no lo tiene) la ruta ni existe.
	r := httptest.NewRequest(http.MethodPost, "/internal/directory/resolve", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	l.mux.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("sin secreto la ruta responde %d: está montada", w.Code)
	}
}
