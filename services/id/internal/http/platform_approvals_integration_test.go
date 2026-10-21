// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Dos personas para lo irreversible (mig 0046), contra PostgreSQL real.
//
// Hay tres formas de que esto parezca funcionar sin hacerlo, y son las tres que
// se prueban: que la operación se ejecute igualmente al pedirla, que la apruebe
// quien la pidió, y que una plataforma con un solo admin global se quede sin
// poder suspender a nadie nunca.
package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// dosAdmins deja dos admin globales elevados y devuelve sus ids.
func dosAdmins(t *testing.T, l *liveServer) (string, string) {
	t.Helper()
	a := pimActor(t, l, "pide@id.test")
	b := pimActor(t, l, "aprueba@id.test")
	for _, sub := range []string{a, b} {
		rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", sub,
			[]string{"pwd", "mfa"}, `{"justification":"pruebas"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("elevar %s: código=%d body=%s", sub, rec.Code, rec.Body)
		}
	}
	return a, b
}

// idAprobacion saca el id de la solicitud del cuerpo de un 409.
func idAprobacion(t *testing.T, body string) string {
	t.Helper()
	var resp struct {
		Code     string `json:"code"`
		Approval struct {
			ID     string `json:"id"`
			Action string `json:"action"`
		} `json:"approval"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("respuesta ilegible: %v (%s)", err, body)
	}
	if resp.Code != "approval_required" {
		t.Fatalf("code=%q, quiero approval_required: el cliente no distingue «esperando» de «ha fallado»", resp.Code)
	}
	if resp.Approval.ID == "" {
		t.Fatalf("el 409 no trae la solicitud creada: %s", body)
	}
	return resp.Approval.ID
}

func TestPlatformAprobacion_SuspenderExigeSegundaPersona(t *testing.T) {
	l := newLiveServer(t)
	pide, aprueba := dosAdmins(t, l)
	conMFA := []string{"pwd", "mfa"}

	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	// 1. Pedir la suspensión NO la aplica.
	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/orgs/"+org.ID+"/status", pide, conMFA,
		`{"status":"suspended","reason":"impago de tres meses"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("pedir suspensión: código=%d, quiero 409 (body=%s)", rec.Code, rec.Body)
	}
	solicitud := idAprobacion(t, rec.Body.String())
	if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusActive {
		t.Fatalf("la organización quedó en %q: pedirlo la ha suspendido igualmente", o.Status)
	}

	// 2. Quien pidió NO puede aprobar. Es el invariante entero de esto.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/approvals/"+solicitud+"/approve", pide, conMFA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("autoaprobación: código=%d, quiero 403 (body=%s)", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "same_person") {
		t.Errorf("el 403 no dice que el problema es ser la misma persona: %s", rec.Body)
	}

	// 3. El segundo admin aprueba, y ESO la ejecuta.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/approvals/"+solicitud+"/approve", aprueba, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("aprobar: código=%d body=%s", rec.Code, rec.Body)
	}
	o, err := l.st.GetPlatformOrg(l.ctx, org.ID)
	if err != nil || o.Status != domain.StatusSuspended {
		t.Fatalf("tras aprobar, la organización está %q (%v): la firma no ejecutó nada", o.Status, err)
	}

	// 4. Y no se puede aprobar dos veces: la segunda no vuelve a ejecutar.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/approvals/"+solicitud+"/approve", aprueba, conMFA, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("doble aprobación: código=%d, quiero 409", rec.Code)
	}
}

// Eliminar una organización exige segunda firma, y NO borra la fila.
//
// El estado `deleted` estaba modelado en el dominio y admitido por el CHECK de
// `organizations` desde la primera migración, pero no había forma de pedirlo:
// `handlePlatformSetOrgStatus` solo aceptaba `active` y `suspended`. Una
// capacidad completa a la que no llegaba ninguna puerta.
//
// Se comprueba lo que importa de la decisión: que pedirlo NO lo aplica, que la
// segunda firma sí, y que después la organización SIGUE EXISTIENDO en estado
// `deleted` --si se borrara la fila, se llevaría por delante las membresías, los
// buzones y la auditoría, que es justo lo que hay que conservar cuando una
// empresa se va--.
func TestPlatformAprobacion_EliminarExigeSegundaPersonaYConservaLaFila(t *testing.T) {
	l := newLiveServer(t)
	pide, aprueba := dosAdmins(t, l)
	conMFA := []string{"pwd", "mfa"}

	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "difunta", Name: "Difunta"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	// 1. Pedirlo no lo aplica: queda esperando firma.
	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/orgs/"+org.ID+"/status", pide, conMFA,
		`{"status":"deleted","reason":"alta de prueba que hay que retirar"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("pedir eliminación: código=%d, quiero 409 (body=%s)", rec.Code, rec.Body)
	}
	solicitud := idAprobacion(t, rec.Body.String())
	if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusActive {
		t.Fatalf("la organización quedó en %q: pedirlo la ha eliminado igualmente", o.Status)
	}

	// 2. Quien lo pidió no puede firmarlo.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/approvals/"+solicitud+"/approve", pide, conMFA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("autoaprobación: código=%d, quiero 403 (body=%s)", rec.Code, rec.Body)
	}

	// 3. La segunda firma sí la ejecuta.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/approvals/"+solicitud+"/approve", aprueba, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("aprobar: código=%d, quiero 200 (body=%s)", rec.Code, rec.Body)
	}

	// 4. Y la fila SIGUE AHÍ, en `deleted`. Esto es el corazón de la prueba.
	o, err := l.st.GetPlatformOrg(l.ctx, org.ID)
	if err != nil {
		t.Fatalf("la organización dejó de ser legible tras eliminarla: %v", err)
	}
	if o.Status != domain.StatusDeleted {
		t.Fatalf("estado tras eliminar: %q, quiero %q", o.Status, domain.StatusDeleted)
	}

	// 5. Y se puede deshacer: reactivar no exige segunda firma.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/orgs/"+org.ID+"/status", pide, conMFA,
		`{"status":"active","reason":"era una prueba"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reactivar una eliminada: código=%d, quiero 200 (body=%s)", rec.Code, rec.Body)
	}
	if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusActive {
		t.Fatalf("tras reactivar quedó en %q", o.Status)
	}
}

func TestPlatformAprobacion_ReactivarNoLaExige(t *testing.T) {
	l := newLiveServer(t)
	pide, _ := dosAdmins(t, l)
	conMFA := []string{"pwd", "mfa"}

	org, _ := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "vuelve", Name: "Vuelve"})
	if _, err := l.st.SetOrgStatus(l.ctx, nil, org.ID, domain.StatusSuspended); err != nil {
		t.Fatalf("SetOrgStatus: %v", err)
	}

	// Devolver el servicio no es el peligro: ponerle trabas convertiría el
	// arreglo de un error en otra espera.
	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/orgs/"+org.ID+"/status", pide, conMFA,
		`{"status":"active","reason":"pagado"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reactivar: código=%d body=%s", rec.Code, rec.Body)
	}
	if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusActive {
		t.Fatalf("la organización sigue %q tras reactivarla", o.Status)
	}
}

// Con UN solo admin global no hay segunda persona posible. Bloquear ahí dejaría
// la plataforma sin poder suspender ni fusionar nunca, y se descubriría el día
// que hace falta con urgencia: se sigue adelante y queda escrito.
func TestPlatformAprobacion_UnSoloAdminNoSeQuedaBloqueado(t *testing.T) {
	l := newLiveServer(t)
	solo := pimActor(t, l, "unico@id.test")
	conMFA := []string{"pwd", "mfa"}
	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/activate", solo, conMFA,
		`{"justification":"pruebas"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("elevar: código=%d body=%s", rec.Code, rec.Body)
	}

	org, _ := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "sola", Name: "Sola"})
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/orgs/"+org.ID+"/status", solo, conMFA,
		`{"status":"suspended","reason":"incidente de seguridad"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("con un solo admin: código=%d, quiero 200 (body=%s)", rec.Code, rec.Body)
	}
	if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusSuspended {
		t.Fatalf("la organización está %q: con un solo admin no se pudo suspender", o.Status)
	}
}

func TestPlatformAprobacion_RechazarLaCierraSinEjecutar(t *testing.T) {
	l := newLiveServer(t)
	pide, aprueba := dosAdmins(t, l)
	conMFA := []string{"pwd", "mfa"}
	org, _ := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "sigue", Name: "Sigue"})

	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/orgs/"+org.ID+"/status", pide, conMFA,
		`{"status":"suspended","reason":"error de soporte"}`)
	solicitud := idAprobacion(t, rec.Body.String())

	// Rechazar SÍ puede hacerlo quien pidió: retirar la propia solicitud no
	// ejecuta nada, y buscar a otro para deshacer un error sería absurdo.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/approvals/"+solicitud+"/reject", pide, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("rechazar: código=%d body=%s", rec.Code, rec.Body)
	}
	if o, _ := l.st.GetPlatformOrg(l.ctx, org.ID); o.Status != domain.StatusActive {
		t.Fatalf("la organización está %q tras un rechazo", o.Status)
	}
	// Y ya no se puede aprobar lo rechazado.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/approvals/"+solicitud+"/approve", aprueba, conMFA, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("aprobar lo rechazado: código=%d, quiero 409", rec.Code)
	}

	// El listado enseña el histórico con quién pidió y quién decidió.
	rec = l.pimReq(t, http.MethodGet, "/api/v1/platform/approvals", aprueba, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("listado: código=%d", rec.Code)
	}
	var lista struct {
		Items []struct {
			ID              string `json:"id"`
			EffectiveStatus string `json:"effective_status"`
			RequesterEmail  string `json:"requester_email"`
			DeciderEmail    string `json:"decider_email"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatalf("listado ilegible: %v", err)
	}
	if len(lista.Items) != 1 || lista.Items[0].EffectiveStatus != "rejected" {
		t.Fatalf("listado = %+v", lista.Items)
	}
	if lista.Items[0].RequesterEmail != "pide@id.test" || lista.Items[0].DeciderEmail != "pide@id.test" {
		t.Errorf("el histórico no dice quién pidió ni quién decidió: %+v", lista.Items[0])
	}
}

// La revisión periódica del alcance global (mig 0047). Lo que se prueba es que
// no sea un botón: nadie confirma su propio acceso, y una fila sin revisar desde
// hace tiempo se marca sola en la lectura, sin barrido que la actualice.
func TestPlatformRevision_NadieSeConfirmaASiMismo(t *testing.T) {
	l := newLiveServer(t)
	uno, dos := dosAdmins(t, l)
	conMFA := []string{"pwd", "mfa"}

	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/admins/"+uno+"/review", uno, conMFA, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("autorrevisión: código=%d, quiero 403 (body=%s)", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "same_person") {
		t.Errorf("el 403 no dice por qué: %s", rec.Body)
	}

	// Revisar el de otro sí: es el caso normal al repasar la lista.
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/admins/"+uno+"/review", dos, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revisar el de otro: código=%d body=%s", rec.Code, rec.Body)
	}
}

func TestPlatformRevision_LaFechaVencidaSeCalculaAlLeer(t *testing.T) {
	l := newLiveServer(t)
	uno, dos := dosAdmins(t, l)
	conMFA := []string{"pwd", "mfa"}

	// Un acceso revisado hace más de un año. Se escribe con fecha pasada porque
	// lo que importa es que NADIE lo marcó como vencido: la lista tiene que verlo
	// por sí misma.
	if _, err := l.st.DB().Exec(l.ctx,
		`UPDATE platform_admins SET reviewed_at = now() - interval '400 days' WHERE user_id = $1`,
		uno); err != nil {
		t.Fatalf("envejecer la revisión: %v", err)
	}

	rec := l.pimReq(t, http.MethodGet, "/api/v1/platform/admins", dos, conMFA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("listado: código=%d", rec.Code)
	}
	var resp struct {
		ReviewDays int `json:"review_days"`
		Items      []struct {
			UserID    string `json:"user_id"`
			ReviewDue bool   `json:"review_due"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("listado ilegible: %v", err)
	}
	if resp.ReviewDays <= 0 {
		t.Errorf("review_days=%d: el panel no puede decir cada cuánto toca", resp.ReviewDays)
	}
	vencidos, alDia := 0, 0
	for _, a := range resp.Items {
		if a.UserID == uno && !a.ReviewDue {
			t.Error("un acceso sin revisar en 400 días no está marcado: la caducidad no se evalúa al leer")
		}
		if a.ReviewDue {
			vencidos++
		} else {
			alDia++
		}
	}
	// El recién dado de alta NO vence: conceder el acceso cuenta como mirarlo.
	if vencidos != 1 || alDia != 1 {
		t.Errorf("%d vencidos y %d al día, quiero 1 y 1", vencidos, alDia)
	}
}

// Avisar de quién gana o pierde el alcance global (tarea #13).
//
// Lo que importa no es que salga UN correo, sino A QUIÉN: al afectado y al resto
// de administradores globales. Que alguien reciba el permiso más amplio del
// sistema sin que nadie más se entere es justo el agujero; la auditoría ya lo
// escribía, pero a un registro se llega tarde.
func TestPlatformAviso_AlAfectadoYAlResto(t *testing.T) {
	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	pide, aprueba := dosAdmins(t, l) // pide@id.test y aprueba@id.test, ambos elevados
	conMFA := []string{"pwd", "mfa"}

	// Un usuario cualquiera al que se le concede el alcance.
	nuevo := l.mkUser(t, "recien@id.test", "")
	_ = nuevo
	// Conceder el alcance global exige segunda persona como suspender o fusionar:
	// pedirlo deja la solicitud esperando, no inscribe a nadie.
	rec := l.pimReq(t, http.MethodPost, "/api/v1/platform/admins", pide, conMFA,
		`{"email":"recien@id.test","reason":"entra al equipo de plataforma"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("pedir: código=%d body=%s, quiero 409: una sola firma no debe dar el alcance más amplio", rec.Code, rec.Body)
	}
	id := idAprobacion(t, rec.Body.String())
	// Y mientras espera, no está dentro. Se comprueba contra la lista y no contra
	// el correo: el aviso sale en segundo plano y «todavía no ha llegado» no
	// distingue de «no va a llegar».
	rec = l.pimReq(t, http.MethodGet, "/api/v1/platform/admins", aprueba, conMFA, "")
	if strings.Contains(rec.Body.String(), "recien@id.test") {
		t.Fatalf("pedir el alcance ya lo concedió: %s", rec.Body)
	}
	rec = l.pimReq(t, http.MethodPost, "/api/v1/platform/approvals/"+id+"/approve", aprueba, conMFA, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("aprobar: código=%d body=%s", rec.Code, rec.Body)
	}

	// El aviso sale en segundo plano: se espera a que llegue en vez de dormir un
	// tiempo fijo, que en una máquina cargada falla sin que nada esté roto.
	destinos := esperarAvisos(t, mc, 3)
	for _, quien := range []string{"recien@id.test", "pide@id.test", "aprueba@id.test"} {
		if !destinos[quien] {
			t.Errorf("nadie avisó a %s: %v", quien, destinos)
		}
	}
	mc.mu.Lock()
	sujeto, actor, concedido := mc.scopeSujeto, mc.scopeActor, mc.scopeConcedido
	mc.mu.Unlock()
	if sujeto != "recien@id.test" || !concedido {
		t.Errorf("el aviso no dice de quién es el alcance: sujeto=%q concedido=%v", sujeto, concedido)
	}
	if actor != "aprueba@id.test" {
		t.Errorf("actor=%q: el aviso no dice quién lo hizo, y es a quien hay que preguntar", actor)
	}
}

// esperarAvisos espera hasta `n` destinatarios distintos y los devuelve.
func esperarAvisos(t *testing.T, mc *captureMailer, n int) map[string]bool {
	t.Helper()
	for range 100 {
		mc.mu.Lock()
		vistos := make(map[string]bool, len(mc.scopeTo))
		for _, to := range mc.scopeTo {
			vistos[to] = true
		}
		mc.mu.Unlock()
		if len(vistos) >= n {
			return vistos
		}
		time.Sleep(50 * time.Millisecond)
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	t.Fatalf("solo se avisó a %v; quiero %d destinatarios", mc.scopeTo, n)
	return nil
}
