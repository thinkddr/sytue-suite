//go:build integration

package http

import (
	"net/http"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestMeAppPasswordsJSON — el ciclo entero por API: crear, listar, revocar.
//
// Era la única parte de la cuenta que solo existía como formulario, y es la
// credencial con la que entra un cliente de correo. Lo que se comprueba aquí es
// lo que no se ve mirando: que la contraseña viaja EN CLARO una sola vez, que la
// revocada desaparece de la lista y que un identificador ajeno no revoca nada.
func TestMeAppPasswordsJSON(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "app-passwords", Name: "Passwords"})
	if err != nil {
		t.Fatal(err)
	}
	u := l.addMember(t, org, "ap@id.test", domain.RoleMember)
	otro := l.mkUser(t, "ap-otro@id.test", "")
	tok := l.token(t, u.ID, org.Slug, nil)
	tokOtro := l.token(t, otro.ID, "", nil)

	// Lista vacía al principio.
	rr, out := l.do(t, http.MethodGet, "/api/v1/me/app-passwords", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("listado: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if vs, _ := out["app_passwords"].([]any); len(vs) != 0 {
		t.Fatalf("se esperaba vacío, hay %d", len(vs))
	}

	// Crear devuelve 201 y la contraseña en claro.
	rr, out = l.do(t, http.MethodPost, "/api/v1/me/app-passwords", tok,
		map[string]string{"label": "Correo del portátil"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	clara, _ := out["password"].(string)
	id, _ := out["id"].(string)
	if out["org_id"] != org.ID || out["tenant"] != org.Slug {
		t.Fatalf("binding: %v", out)
	}
	if clara == "" || id == "" {
		t.Fatalf("el alta debe devolver id y contraseña: %v", out)
	}
	if out["label"] != "Correo del portátil" {
		t.Fatalf("etiqueta devuelta=%v", out["label"])
	}

	// Y solo esa vez: el listado trae metadatos, nunca el secreto.
	_, out = l.do(t, http.MethodGet, "/api/v1/me/app-passwords", tok, nil)
	vs, _ := out["app_passwords"].([]any)
	if len(vs) != 1 {
		t.Fatalf("se esperaba una, hay %d", len(vs))
	}
	fila, _ := vs[0].(map[string]any)
	if _, hay := fila["password"]; hay {
		t.Fatal("el listado NO puede traer la contraseña")
	}
	if _, hay := fila["secret_hash"]; hay {
		t.Fatal("el listado NO puede traer el hash")
	}

	// Un id ajeno no revoca nada: el store acota por usuario.
	if rr, _ = l.do(t, http.MethodDelete, "/api/v1/me/app-passwords/"+id, tokOtro, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("revocar la de otro: status=%d, quiero 404", rr.Code)
	}
	_, out = l.do(t, http.MethodGet, "/api/v1/me/app-passwords", tok, nil)
	if vs, _ = out["app_passwords"].([]any); len(vs) != 1 {
		t.Fatalf("la de otro no debía tocarla: quedan %d", len(vs))
	}

	// La propia sí, y desaparece de la lista.
	if rr, _ = l.do(t, http.MethodDelete, "/api/v1/me/app-passwords/"+id, tok, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("revocar: status=%d, quiero 204", rr.Code)
	}
	_, out = l.do(t, http.MethodGet, "/api/v1/me/app-passwords", tok, nil)
	if vs, _ = out["app_passwords"].([]any); len(vs) != 0 {
		t.Fatalf("la revocada debe salir de la lista, quedan %d", len(vs))
	}

	// Revocar dos veces la misma: 404, no 204 mentido.
	if rr, _ = l.do(t, http.MethodDelete, "/api/v1/me/app-passwords/"+id, tok, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("revocar dos veces: status=%d, quiero 404", rr.Code)
	}
}
