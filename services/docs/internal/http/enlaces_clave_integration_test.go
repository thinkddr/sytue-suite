// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

type clavePrueba struct {
	*escenarioPermisos
}

func (e clavePrueba) crear(cuerpo string) (token, id, crudo string) {
	e.t.Helper()
	rec := httptest.NewRecorder()
	e.hd.createShare(rec, e.peticion("POST", "owner", cuerpo, nil, "/shares"))
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("crear el enlace: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Share struct {
			ID, Token string
		} `json:"share"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	return out.Share.Token, out.Share.ID, rec.Body.String()
}

// abrir pide la ficha y la vista del enlace con esa prueba; las dos deben contestar igual.
func (e clavePrueba) abrir(token, prueba string) int {
	e.t.Helper()
	pedir := func(f http.HandlerFunc, sufijo string) int {
		r := req(e.t, "GET", "/v1/share/"+token+sufijo, nil, map[string]string{"token": token}, "")
		if prueba != "" {
			r.Header.Set(cabeceraPrueba, prueba)
		}
		rec := httptest.NewRecorder()
		f(rec, r)
		if rec.Code != http.StatusOK && strings.Contains(rec.Body.String(), "Cuentas") {
			e.t.Fatalf("una respuesta %d lleva el título del documento: %s", rec.Code, rec.Body)
		}
		return rec.Code
	}
	a, b := pedir(e.hd.resolveShare, ""), pedir(e.hd.viewShare, "/view")
	if a != b {
		e.t.Fatalf("la ficha (%d) y la vista (%d) no coinciden", a, b)
	}
	return a
}

// desbloquear devuelve el estado, el cuerpo tal cual y la prueba.
func (e clavePrueba) desbloquear(token, password, ip string) (int, string, string) {
	e.t.Helper()
	cuerpo, _ := json.Marshal(map[string]string{"token": token, "password": password})
	r := req(e.t, "POST", "/v1/share/unlock", nil, nil, string(cuerpo))
	r.RemoteAddr = ip + ":40000"
	rec := httptest.NewRecorder()
	e.hd.unlockShare(rec, r)
	var out struct {
		Proof string `json:"proof"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, rec.Body.String(), out.Proof
}

// canjear lo intenta `sub`, de otra organización, con su sesión.
func (e clavePrueba) canjear(sub, token, prueba string) int {
	e.t.Helper()
	cuerpo, _ := json.Marshal(map[string]string{"token": token, "proof": prueba})
	r := req(e.t, "POST", "/v1/share/redeem", &authn.Claims{Subject: sub, Tenant: "globex"}, nil, string(cuerpo))
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "globex"}))
	rec := httptest.NewRecorder()
	e.hd.redeemShare(rec, r)
	return rec.Code
}

func (e clavePrueba) cambiar(sub, linkID, password string) int {
	e.t.Helper()
	cuerpo, _ := json.Marshal(map[string]string{"password": password})
	rec := httptest.NewRecorder()
	e.hd.setSharePassword(rec, e.peticion("PUT", sub, string(cuerpo), map[string]string{"linkId": linkID}, "/shares/"+linkID+"/password"))
	return rec.Code
}

// La contraseña de un enlace, por la API directa: sin ella no se abre la vista, ni la
// ficha, ni se canjea con una sesión; un fallo no dice si el enlace existe; la contraseña
// y su hash no salen por ninguna respuesta; y cambiarla invalida lo ya desbloqueado.
func TestClaveDeEnlacePorLaAPI(t *testing.T) {
	e := clavePrueba{nuevoEscenarioPermisos(t, domain.KindDocument)}
	e.hd.canje = e.st
	// Los límites tienen su propia prueba; aquí no deben estorbar.
	e.hd.limitesClave = &limitesClave{porEnlace: httpx.NewTokenBucket(100, 1000), porIP: httpx.NewTokenBucket(100, 1000), argon: make(chan struct{}, 8)}
	const clave = "clave-del-enlace-1"

	token, id, crudo := e.crear(`{"role":"editor","password":"` + clave + `"}`)
	if !strings.Contains(crudo, `"has_password":true`) || strings.Contains(crudo, clave) || strings.Contains(crudo, "argon2") {
		t.Fatalf("la respuesta de crear lleva la contraseña o su hash, o no dice que la tiene: %s", crudo)
	}
	libre, _, crudoLibre := e.crear(`{"role":"editor"}`)
	if !strings.Contains(crudoLibre, `"has_password":false`) {
		t.Fatalf("un enlace sin contraseña dice que la tiene: %s", crudoLibre)
	}
	// Una contraseña que no vale no crea el enlace sin ella.
	rec := httptest.NewRecorder()
	e.hd.createShare(rec, e.peticion("POST", "owner", `{"role":"viewer","password":"corta"}`, nil, "/shares"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("contraseña corta: %d", rec.Code)
	}
	if ls, _ := e.st.ListShareLinks(e.ctx, e.doc.ID); len(ls) != 2 {
		t.Fatalf("la contraseña rechazada dejó un enlace: %d", len(ls))
	}

	// Sin prueba, con una inventada o con la contraseña en su lugar: nada.
	for _, prueba := range []string{"", "inventada", clave} {
		if c := e.abrir(token, prueba); c != http.StatusUnauthorized {
			t.Fatalf("abrir con prueba %q: %d", prueba, c)
		}
		if c := e.canjear("fuera", token, prueba); c != http.StatusUnauthorized {
			t.Fatalf("canjear con prueba %q: %d", prueba, c)
		}
	}
	if perms, _ := e.st.ListPermissions(e.ctx, e.doc.ID); len(perms) != 4 {
		t.Fatalf("un canje sin contraseña dejó un permiso: %d", len(perms))
	}
	if c := e.abrir(libre, ""); c != http.StatusOK {
		t.Fatalf("un enlace sin contraseña pide prueba: %d", c)
	}

	// Un fallo es el mismo fallo: contraseña errónea, enlace inventado, enlace sin
	// contraseña, revocado. Ni el estado ni el cuerpo los distinguen.
	revocado, idRevocado, _ := e.crear(`{"role":"viewer","password":"` + clave + `"}`)
	if err := e.st.RevokeShareLink(e.ctx, e.doc.ID, idRevocado); err != nil {
		t.Fatal(err)
	}
	c0, cuerpo0, _ := e.desbloquear(token, "no-es-la-clave", "10.0.0.1")
	if c0 != http.StatusForbidden {
		t.Fatalf("contraseña errónea: %d %s", c0, cuerpo0)
	}
	for nombre, caso := range map[string][2]string{
		"enlace inventado":       {"no-existe", clave},
		"enlace sin contraseña":  {libre, clave},
		"enlace revocado":        {revocado, clave},
		"contraseña vacía":       {token, ""},
		"contraseña desmesurada": {token, strings.Repeat("x", 5000)},
	} {
		c, cuerpo, prueba := e.desbloquear(caso[0], caso[1], "10.0.0.2")
		if c != c0 || cuerpo != cuerpo0 || prueba != "" {
			t.Errorf("%s: %d %s (el fallo de referencia: %d %s)", nombre, c, cuerpo, c0, cuerpo0)
		}
	}

	// Con la contraseña: la prueba abre la vista y el canje.
	c, cuerpo, prueba := e.desbloquear(token, clave, "10.0.0.3")
	if c != http.StatusOK || prueba == "" || strings.Contains(cuerpo, "argon2") || strings.Contains(cuerpo, clave) {
		t.Fatalf("desbloquear: %d %s", c, cuerpo)
	}
	if c := e.abrir(token, prueba); c != http.StatusOK {
		t.Fatalf("abrir con la prueba: %d", c)
	}
	// La prueba de un enlace no abre otro con la misma contraseña (otra sal).
	otro, _, _ := e.crear(`{"role":"viewer","password":"` + clave + `"}`)
	if c := e.abrir(otro, prueba); c != http.StatusUnauthorized {
		t.Fatalf("la prueba de un enlace abrió otro: %d", c)
	}
	if c := e.canjear("fuera", token, prueba); c != http.StatusOK {
		t.Fatalf("canjear con la prueba: %d", c)
	}

	// Lo que lista el propietario: que tiene contraseña, y nada más.
	rec = httptest.NewRecorder()
	e.hd.listShares(rec, e.peticion("GET", "owner", "", nil, "/shares"))
	if lista := rec.Body.String(); !strings.Contains(lista, `"has_password":true`) || strings.Contains(lista, "argon2") || strings.Contains(lista, clave) {
		t.Fatalf("la lista de enlaces: %s", lista)
	}

	// Cambiarla: solo el propietario, y la prueba anterior deja de valer.
	const nueva = "otra-clave-distinta-2"
	for _, sub := range []string{"ana", "comenta", "lector", "intruso"} {
		if c := e.cambiar(sub, id, nueva); c != http.StatusForbidden {
			t.Fatalf("%s cambió la contraseña: %d", sub, c)
		}
	}
	if c := e.cambiar("owner", id, "corta"); c != http.StatusBadRequest {
		t.Fatalf("contraseña corta al cambiar: %d", c)
	}
	if c := e.abrir(token, prueba); c != http.StatusOK {
		t.Fatal("un cambio rechazado invalidó la prueba")
	}
	if c := e.cambiar("owner", id, nueva); c != http.StatusNoContent {
		t.Fatalf("cambiar: %d", c)
	}
	if c := e.abrir(token, prueba); c != http.StatusUnauthorized {
		t.Fatalf("la prueba de la contraseña anterior sigue abriendo: %d", c)
	}
	if c, _, _ := e.desbloquear(token, clave, "10.0.0.4"); c != http.StatusForbidden {
		t.Fatalf("la contraseña anterior sigue valiendo: %d", c)
	}
	_, _, prueba2 := e.desbloquear(token, nueva, "10.0.0.5")
	if c := e.abrir(token, prueba2); c != http.StatusOK {
		t.Fatalf("la contraseña nueva: %d", c)
	}
	// Un enlace de otro documento no se toca desde este.
	if c := e.cambiar("owner", "00000000-0000-4000-8000-000000000000", nueva); c != http.StatusNotFound {
		t.Fatalf("cambiar un enlace que no es del documento: %d", c)
	}
	// Quitarla: vuelve a abrirse solo con el token.
	if c := e.cambiar("owner", id, ""); c != http.StatusNoContent {
		t.Fatalf("quitar: %d", c)
	}
	if c := e.abrir(token, ""); c != http.StatusOK {
		t.Fatalf("sin contraseña ya: %d", c)
	}
}

// Los intentos se acotan por enlace y por IP ANTES de verificar: agotada la ráfaga, ni la
// contraseña correcta entra hasta que se recarga, y la respuesta dice cuándo.
func TestClaveDeEnlaceLimitaLosIntentos(t *testing.T) {
	e := clavePrueba{nuevoEscenarioPermisos(t, domain.KindDocument)}
	const clave = "clave-del-enlace-1"
	token, _, _ := e.crear(`{"role":"viewer","password":"` + clave + `"}`)
	otro, _, _ := e.crear(`{"role":"viewer","password":"` + clave + `"}`)

	// Por enlace: tres intentos, cada uno desde una IP distinta.
	e.hd.limitesClave = &limitesClave{porEnlace: httpx.NewTokenBucket(1.0/3600, 3), porIP: httpx.NewTokenBucket(100, 1000), argon: make(chan struct{}, 8)}
	for i, ip := range []string{"10.1.0.1", "10.1.0.2", "10.1.0.3"} {
		if c, _, _ := e.desbloquear(token, "no-es-la-clave", ip); c != http.StatusForbidden {
			t.Fatalf("intento %d: %d", i, c)
		}
	}
	cuerpo, _ := json.Marshal(map[string]string{"token": token, "password": clave})
	r := req(t, "POST", "/v1/share/unlock", nil, nil, string(cuerpo))
	r.RemoteAddr = "10.1.0.4:40000"
	rec := httptest.NewRecorder()
	e.hd.unlockShare(rec, r)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || strings.Contains(rec.Body.String(), "proof") {
		t.Fatalf("enlace bloqueado, contraseña correcta: %d %q %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body)
	}
	// El bloqueo es de ESE enlace.
	if c, _, p := e.desbloquear(otro, clave, "10.1.0.4"); c != http.StatusOK || p == "" {
		t.Fatalf("otro enlace quedó bloqueado: %d", c)
	}

	// Por IP: la misma dirección probando enlaces distintos.
	e.hd.limitesClave = &limitesClave{porEnlace: httpx.NewTokenBucket(100, 1000), porIP: httpx.NewTokenBucket(1.0/3600, 2), argon: make(chan struct{}, 8)}
	for _, tk := range []string{"inventado-1", "inventado-2"} {
		if c, _, _ := e.desbloquear(tk, clave, "10.2.0.1"); c != http.StatusForbidden {
			t.Fatalf("%s: %d", tk, c)
		}
	}
	if c, _, _ := e.desbloquear(token, clave, "10.2.0.1"); c != http.StatusTooManyRequests {
		t.Fatalf("IP bloqueada: %d", c)
	}
	// Una cabecera X-Forwarded-For puesta por quien llama no le cambia de IP.
	r = req(t, "POST", "/v1/share/unlock", nil, nil, string(cuerpo))
	r.RemoteAddr = "203.0.113.9:40000"
	e.hd.limitesClave.porIP = httpx.NewTokenBucket(1.0/3600, 1)
	for i, falsa := range []string{"1.1.1.1", "2.2.2.2"} {
		r2 := r.Clone(r.Context())
		r2.Body = httptest.NewRequest("POST", "/", strings.NewReader(string(cuerpo))).Body
		r2.Header.Set("X-Forwarded-For", falsa)
		rec := httptest.NewRecorder()
		e.hd.unlockShare(rec, r2)
		if quiere := []int{http.StatusOK, http.StatusTooManyRequests}[i]; rec.Code != quiere {
			t.Fatalf("XFF falsa %s: %d, se esperaba %d", falsa, rec.Code, quiere)
		}
	}
}
