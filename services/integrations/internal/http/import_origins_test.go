// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

// conexion fabrica una instancia de la organización como la devuelve ListInstances.
func conexion(id, plantilla string, estado domain.Status, secretos bool, cfg map[string]string, creada time.Time) domain.AppInstance {
	raw, _ := json.Marshal(cfg)
	return domain.AppInstance{
		ID: id, TemplateKey: plantilla, Name: plantilla + " " + id, Config: raw, Status: estado,
		HasSecrets: secretos, CreatedAt: creada, UpdatedAt: creada, ConsentTemplateEnabled: true,
	}
}

func ids(os []importOrigin) [][]string {
	out := make([][]string, 0, len(os))
	for _, o := range os {
		out = append(out, o.InstanceIDs)
	}
	return out
}

// TestImportOriginsSoloLasPlantillasQueMigran es el duplicado que se veía: la misma
// cuenta de Google conectada para Drive, Meet y Chat, y la de Microsoft para Teams y
// para el correo de una persona, salían como un «Traerlo de…» más cada una — y todos
// menos uno fallaban al pulsarlos, porque la pasarela solo migra desde dos plantillas.
func TestImportOriginsSoloLasPlantillasQueMigran(t *testing.T) {
	plantillas := []string{
		"google-drive", "google-meet", "google-chat", "google-chat-conversations",
		"microsoft-teams", "microsoft-teams-rooms", "microsoft-teams-chat", "microsoft-mail",
		"gmail-mail", "onedrive", "github", "google-workspace", "microsoft365",
	}
	conexiones := make([]domain.AppInstance, 0, len(plantillas))
	for i, k := range plantillas {
		conexiones = append(conexiones, conexion(k, k, domain.StatusActive, true, nil, t0.Add(time.Duration(i)*time.Minute)))
	}
	got := importOrigins(conexiones)
	if want := [][]string{{"google-workspace"}, {"microsoft365"}}; !slices.EqualFunc(ids(got), want, slices.Equal) {
		t.Fatalf("orígenes = %v, quiero solo %v", ids(got), want)
	}
	if got[0].Source != domain.SourceGoogle || got[1].Source != domain.SourceM365 {
		t.Fatalf("proveedores = %q, %q", got[0].Source, got[1].Source)
	}
}

// TestImportOriginsUnaPorCuenta: la misma cuenta conectada dos veces es UN origen, que
// lanza con la conexión que puede leer y, entre las que pueden, la autorizada más tarde.
func TestImportOriginsUnaPorCuenta(t *testing.T) {
	vieja := conexion("a", "google-workspace", domain.StatusActive, true,
		map[string]string{"domain": "Acme.com", configKeyAuthorizedAt: t0.Format(time.RFC3339)}, t0)
	nueva := conexion("b", "google-workspace", domain.StatusActive, true,
		map[string]string{
			configKeyImportAccount: "google:acme.com", configKeyImportAccountLabel: "acme.com",
			configKeyAuthorizedAt: t0.Add(48 * time.Hour).Format(time.RFC3339),
		}, t0.Add(-time.Hour))
	rota := conexion("c", "google-workspace", domain.StatusError, true,
		map[string]string{"domain": "acme.com", configKeyAuthorizedAt: t0.Add(72 * time.Hour).Format(time.RFC3339)}, t0.Add(time.Hour))

	got := importOrigins([]domain.AppInstance{vieja, rota, nueva})
	if len(got) != 1 {
		t.Fatalf("la misma cuenta salió %d veces: %+v", len(got), got)
	}
	o := got[0]
	if o.InstanceID != "b" || !slices.Equal(o.InstanceIDs, []string{"b", "a", "c"}) {
		t.Fatalf("elegida %q de %v; quiero la autorizada más tarde que puede leer (b), y la rota al final", o.InstanceID, o.InstanceIDs)
	}
	if o.AccountKey != "google:acme.com" || o.AccountLabel != "acme.com" || o.State != importOriginReady {
		t.Fatalf("origen = %+v", o)
	}
}

// TestImportOriginsCuentaDesconocida: una conexión de antes de recordar la cuenta se
// suma a la ÚNICA conocida de su proveedor; con dos conocidas no se adivina.
func TestImportOriginsCuentaDesconocida(t *testing.T) {
	conocida := conexion("k", "microsoft365", domain.StatusActive, true,
		map[string]string{configKeyImportAccount: "m365:contoso.onmicrosoft.com", configKeyImportAccountLabel: "contoso.com"}, t0)
	desconocida := conexion("u", "microsoft365", domain.StatusActive, true, nil, t0.Add(time.Hour))

	got := importOrigins([]domain.AppInstance{conocida, desconocida})
	if len(got) != 1 || !slices.Equal(got[0].InstanceIDs, []string{"u", "k"}) || got[0].AccountLabel != "contoso.com" {
		t.Fatalf("con una sola cuenta conocida, la desconocida debía sumarse a ella: %+v", got)
	}

	otra := conexion("o", "microsoft365", domain.StatusActive, true,
		map[string]string{"tenant": "fabrikam.onmicrosoft.com"}, t0)
	got = importOrigins([]domain.AppInstance{conocida, desconocida, otra})
	if want := [][]string{{"u"}, {"k"}, {"o"}}; !slices.EqualFunc(ids(got), want, slices.Equal) {
		t.Fatalf("con dos cuentas conocidas no se adivina: %v, quiero %v", ids(got), want)
	}
}

// TestImportOriginsQueNoSonOrigen: un consentimiento sin terminar, una conexión que la
// organización apagó o una plantilla que la plataforma retiró no ofrecen nada.
func TestImportOriginsQueNoSonOrigen(t *testing.T) {
	retirada := conexion("r", "google-workspace", domain.StatusActive, true, nil, t0)
	retirada.ConsentTemplateEnabled = false
	got := importOrigins([]domain.AppInstance{
		conexion("d", "google-workspace", domain.StatusDraft, false, nil, t0),
		conexion("x", "google-workspace", domain.StatusDisabled, true, nil, t0),
		retirada,
	})
	if len(got) != 0 {
		t.Fatalf("no debía haber orígenes: %+v", got)
	}
}

// TestImportOriginsNoDisponible: si la mejor conexión de la cuenta no puede leer, el
// origen se enseña apagado en vez de ofrecer un botón que siempre falla.
func TestImportOriginsNoDisponible(t *testing.T) {
	for nombre, c := range map[string]domain.AppInstance{
		"falló su prueba":     conexion("e", "google-workspace", domain.StatusError, true, nil, t0),
		"sin credencial":      conexion("s", "google-workspace", domain.StatusActive, false, nil, t0),
		"caducada y sin nada": conexion("n", "microsoft365", domain.StatusError, false, nil, t0),
	} {
		got := importOrigins([]domain.AppInstance{c})
		if len(got) != 1 || got[0].State != importOriginUnavailable {
			t.Fatalf("%s: %+v, quiero un origen %q", nombre, got, importOriginUnavailable)
		}
	}
}

// TestSelfImportOrigin: lanzar con una conexión vieja de la cuenta usa la buena; las
// que no son origen o no pueden leer se rechazan con su propio código.
func TestSelfImportOrigin(t *testing.T) {
	origins := importOrigins([]domain.AppInstance{
		conexion("a", "google-workspace", domain.StatusActive, true, map[string]string{"domain": "acme.com"}, t0),
		conexion("b", "google-workspace", domain.StatusActive, true, map[string]string{"domain": "acme.com"}, t0.Add(time.Hour)),
		conexion("m", "microsoft365", domain.StatusError, true, nil, t0),
	})
	o, err := selfImportOrigin(origins, "a", domain.SourceGoogle)
	if err != nil || o.InstanceID != "b" {
		t.Fatalf("con la conexión vieja (a) debía lanzar con la buena (b): %+v, %v", o, err)
	}
	for _, c := range []struct {
		nombre, id string
		src        domain.MigrationSource
		status     int
		code       string
	}{
		{"no es origen", "zzz", domain.SourceGoogle, http.StatusNotFound, "import_origin_not_found"},
		{"otro proveedor", "a", domain.SourceM365, http.StatusBadRequest, "bad_request"},
		{"no puede leer", "m", domain.SourceM365, http.StatusConflict, "import_connection_unavailable"},
	} {
		_, err := selfImportOrigin(origins, c.id, c.src)
		var apiErr *httpx.Error
		if !errors.As(err, &apiErr) || apiErr.Status != c.status || apiErr.Code != c.code {
			t.Fatalf("%s: %v, quiero %d %s", c.nombre, err, c.status, c.code)
		}
	}
}

// TestSelfJobsCuentaYMotivo: cada trabajo dice de qué cuenta viene —también los de una
// conexión vieja de la misma cuenta— y por qué falló, en código que se traduce.
func TestSelfJobsCuentaYMotivo(t *testing.T) {
	origins := importOrigins([]domain.AppInstance{
		conexion("a", "google-workspace", domain.StatusActive, true, map[string]string{"domain": "acme.com"}, t0),
		conexion("b", "google-workspace", domain.StatusActive, true, map[string]string{"domain": "acme.com"}, t0.Add(time.Hour)),
	})
	jobs := selfJobs([]domain.MigrationJob{
		{ID: "1", InstanceID: "a", Status: domain.MigrationError, LastError: "gws: el token endpoint devolvió invalid_grant: Token has been expired or revoked."},
		{ID: "2", InstanceID: "b", Status: domain.MigrationDone},
		{ID: "3", InstanceID: "borrada", Status: domain.MigrationDone},
	}, origins)
	if jobs[0].AccountKey != "google:acme.com" || jobs[1].AccountKey != "google:acme.com" || jobs[0].AccountLabel != "acme.com" {
		t.Fatalf("los dos trabajos son de la misma cuenta: %+v", jobs)
	}
	if jobs[2].AccountKey != "" {
		t.Fatalf("un trabajo de una conexión que ya no es origen no tiene cuenta: %+v", jobs[2])
	}
	if jobs[0].ErrorCode != "connection_expired" || jobs[1].ErrorCode != "" {
		t.Fatalf("códigos = %q, %q", jobs[0].ErrorCode, jobs[1].ErrorCode)
	}
	raw, _ := json.Marshal(jobs[0])
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	if wire["id"] != "1" || wire["account_key"] != "google:acme.com" || wire["error_code"] != "connection_expired" {
		t.Fatalf("el trabajo debe seguir saliendo plano, con la cuenta al lado: %s", raw)
	}
}

// TestImportErrorCode: las marcas son las de nuestros propios errores; si alguien cambia
// su redacción, este test es el que lo dice.
func TestImportErrorCode(t *testing.T) {
	for msg, want := range map[string]string{
		"gws: el token endpoint devolvió invalid_grant: Bad Request":                                   "connection_expired",
		"migrate: listar \"/\" en Google Workspace: el origen respondió HTTP 401: Invalid Credentials": "connection_expired",
		`connector: consulta inválida: falta el secreto "service_account_key" de la instancia`:         "needs_admin",
		"connector: consulta inválida: faltan credenciales de la instancia":                            "needs_admin",
		"gws: el token endpoint devolvió unauthorized_client: Client is unauthorized":                  "needs_admin",
		"migrate: 3 entidades no se pudieron copiar; reanuda el trabajo para reintentarlas":            "",
		"": "",
	} {
		if got := importErrorCode(msg); got != want {
			t.Fatalf("importErrorCode(%q) = %q, quiero %q", msg, got, want)
		}
	}
}
