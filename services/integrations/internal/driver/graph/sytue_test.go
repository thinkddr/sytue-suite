// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package graph_test

// Prueba de EXTREMO A EXTREMO del adaptador inverso: el MISMO driver de Microsoft Graph
// apuntado a la PROPIA suite. El httptest levanta las dos mitades reales del camino —el
// token endpoint de Sytue ID (canje RFC 8693 acotado a una persona) y la respuesta de
// /compat/graph de Sytue Drive, con el formato que produce services/drive— y se afirma
// que el driver la consume y devuelve la tabla correcta.
//
// A DIFERENCIA del resto de tests del paquete, este NO relaja la barrera SSRF: httptest
// escucha en loopback, que la política de producción bloquea igual que bloquearía
// http://drive:8082. Es deliberado — es lo que demuestra que el camino interno
// (policy.RuntimeParams.Internal) funciona de verdad y no solo compila.

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/graph"
	sytueoauth2 "git.sytue.eu/sytue/one/services/integrations/internal/oauth2"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
)

// Credenciales y tokens del canje. delegateClientID/Secret son los del client DELEGABLE
// de la organización (lo que el gateway inyecta desde store.DelegatedClient); ownToken es
// el token propio del client (client_credentials) y personToken el YA acotado a la
// persona, único que /compat/graph acepta.
const (
	delegateClientID = "acme-delegate"
	delegateSecret   = "shh-delegado"
	ownToken         = "token-propio-del-client"
	personToken      = "token-acotado-a-ana"
	personSubject    = "ana@acme.eu"
)

// sytuePage1 es la primera página tal como la serializa services/drive
// (compat.DriveItemCollection): `value` + `@odata.nextLink` ABSOLUTO, con las facetas
// file/folder y una anotación de control @odata.etag que el driver debe descartar.
const sytuePage1 = `{
  "value": [
    {
      "@odata.etag": "\"c1\"",
      "id": "0f9a5c3e-1b2d-4f6a-8c7e-5d4b3a2f1e0d",
      "name": "Informe trimestral.xlsx",
      "size": 20480,
      "createdDateTime": "2026-07-27T08:30:00Z",
      "lastModifiedDateTime": "2026-07-27T09:00:00Z",
      "file": {"mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "hashes": {"sha256Hash": "cafe"}},
      "parentReference": {"id": "11111111-1111-4111-8111-111111111111"}
    },
    {
      "id": "22222222-2222-4222-8222-222222222222",
      "name": "Contratos",
      "size": 0,
      "createdDateTime": "2026-07-20T10:00:00Z",
      "lastModifiedDateTime": "2026-07-26T18:45:00Z",
      "folder": {"childCount": 2},
      "parentReference": {"id": "11111111-1111-4111-8111-111111111111"}
    }
  ],
  "@odata.nextLink": "%s/compat/graph/v1.0/me/drive/root/children?%%24skiptoken=PAGINA-2&%%24top=200"
}`

const sytuePage2 = `{
  "value": [
    {
      "id": "33333333-3333-4333-8333-333333333333",
      "name": "Notas.md",
      "size": 512,
      "createdDateTime": "2026-07-28T07:00:00Z",
      "lastModifiedDateTime": "2026-07-28T07:05:00Z",
      "file": {"mimeType": "text/markdown"},
      "parentReference": {"id": "11111111-1111-4111-8111-111111111111"}
    }
  ]
}`

// sytueSuite levanta la mitad de la suite con la que habla el driver: Sytue ID
// (/oauth2/token) y Sytue Drive (/compat/graph/...). Devuelve el servidor, los
// formularios que recibió el token endpoint y el Authorization que vio Drive.
func sytueSuite(t *testing.T) (*httptest.Server, *[]url.Values, *string) {
	t.Helper()
	var (
		base     string
		forms    []url.Values
		lastAuth string
	)
	mux := http.NewServeMux()

	// Sytue ID: client_credentials (probar quién pide) + token-exchange (actuar en
	// nombre de la persona). Rechaza cualquier canje que no venga acotado.
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		forms = append(forms, r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		switch f := r.PostForm; f.Get("grant_type") {
		case "client_credentials":
			if f.Get("client_id") != delegateClientID || f.Get("client_secret") != delegateSecret {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"credenciales del client delegable inválidas"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"`+ownToken+`","token_type":"Bearer","expires_in":300}`)
		case "urn:ietf:params:oauth:grant-type:token-exchange":
			if f.Get("subject_token") != ownToken ||
				f.Get("subject_token_type") != "urn:ietf:params:oauth:token-type:access_token" ||
				f.Get("requested_subject") != personSubject ||
				f.Get("audience") != "sytue-drive" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"error":"invalid_request","error_description":"el canje no viene acotado a la persona"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"`+personToken+`","token_type":"Bearer","expires_in":300}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"unsupported_grant_type"}`)
		}
	})

	// Sytue Drive: /me/drive (lo que sondea Driver.Test) y la colección paginada.
	// Sin el token de la PERSONA responde 401 en el sobre de error de Graph, igual
	// que services/drive.
	unauthorized := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"unauthenticated","message":"hace falta un token acotado a la persona"}}`)
	}
	mux.HandleFunc("/compat/graph/v1.0/me/drive", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer "+personToken {
			unauthorized(w)
			return
		}
		_, _ = io.WriteString(w, `{"id":"11111111-1111-4111-8111-111111111111","driveType":"business","name":"Sytue Drive"}`)
	})
	mux.HandleFunc("/compat/graph/v1.0/me/drive/root/children", func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		if lastAuth != "Bearer "+personToken {
			unauthorized(w)
			return
		}
		if r.URL.Query().Get("$skiptoken") == "PAGINA-2" {
			_, _ = io.WriteString(w, sytuePage2)
			return
		}
		_, _ = fmt.Fprintf(w, sytuePage1, base)
	})

	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)
	return srv, &forms, &lastAuth
}

// sytueInstance es la instancia TAL COMO LA DEJA EL GATEWAY: el administrador solo puso
// `subject`; token_url, graph_base, client_id, audience y el client_secret los inyecta el
// servicio al despachar (integrationshttp.loadInstance).
func sytueInstance(base string) connector.Instance {
	return connector.Instance{
		ID: "inst-sytue", OrgID: "acme", TemplateKey: "sytue",
		Config: map[string]string{
			graph.ConfigSubject:   personSubject,
			graph.ConfigTokenURL:  base + "/oauth2/token",
			graph.ConfigGraphBase: base + "/compat/graph",
			graph.ConfigClientID:  delegateClientID,
			graph.ConfigAudience:  "sytue-drive",
		},
		Secrets: map[string]string{"client_secret": delegateSecret},
	}
}

// internalRuntime es el Runtime que fabrica el gateway para una instancia de la propia
// suite: Internal ⇒ el destino puede vivir en una IP privada, con el resto de la cadena
// (rate-limit, auditoría, política, redirects acotados) intacta.
func internalRuntime() connector.Runtime {
	return policy.BuildRuntime(policy.RuntimeParams{
		OrgID: "acme", InstanceID: "inst-sytue", Internal: true, Log: slog.Default(),
	})
}

// TestSytueDriveEndToEnd: canje RFC 8693 contra Sytue ID + lectura paginada de
// /compat/graph ⇒ la tabla que llega a Sheets. Sin relajar la barrera SSRF.
func TestSytueDriveEndToEnd(t *testing.T) {
	srv, forms, lastAuth := sytueSuite(t)
	inst := sytueInstance(srv.URL)
	drv := graph.New(sytueoauth2.NewCache())

	// Probar la conexión (lo primero que pulsa un admin) sondea /me/drive, no $metadata.
	if err := drv.Test(t.Context(), inst, internalRuntime()); err != nil {
		t.Fatalf("Test contra la propia suite: %v", err)
	}

	spec := []byte(`{"resource":"me/drive/root/children","select":["name","size"]}`)
	tbl, err := drv.Query(t.Context(), inst, internalRuntime(), spec)
	if err != nil {
		t.Fatalf("Query contra /compat/graph: %v", err)
	}

	// Paginación por @odata.nextLink absoluto: 2 filas de la página 1 + 1 de la 2.
	if len(tbl.Rows) != 3 {
		t.Fatalf("filas = %d, quiero 3 (se sigue el @odata.nextLink de Drive)", len(tbl.Rows))
	}
	if len(tbl.Columns) < 2 || tbl.Columns[0].Name != "name" || tbl.Columns[1].Name != "size" {
		t.Fatalf("columnas = %+v, quiero el orden del $select (name, size) primero", tbl.Columns)
	}
	if tbl.Columns[0].Type != "text" || tbl.Columns[1].Type != "number" {
		t.Fatalf("tipos = %q/%q, quiero text/number", tbl.Columns[0].Type, tbl.Columns[1].Type)
	}
	for _, c := range tbl.Columns {
		if strings.HasPrefix(c.Name, "@") {
			t.Fatalf("la columna %q es una anotación de control OData: debe descartarse", c.Name)
		}
	}
	if tbl.Rows[0][0] != "Informe trimestral.xlsx" || tbl.Rows[0][1] != float64(20480) {
		t.Fatalf("fila 0 = %#v, quiero el fichero con su tamaño", tbl.Rows[0])
	}
	if tbl.Rows[1][0] != "Contratos" {
		t.Fatalf("fila 1 = %#v, quiero la carpeta", tbl.Rows[1])
	}
	if tbl.Rows[2][0] != "Notas.md" {
		t.Fatalf("fila 2 = %#v, quiero la fila de la SEGUNDA página", tbl.Rows[2])
	}

	// Drive vio el token de la PERSONA, no el del client: /compat/graph es /me.
	if *lastAuth != "Bearer "+personToken {
		t.Fatalf("Authorization en Drive = %q, quiero el token acotado a la persona", *lastAuth)
	}

	// El canje fue el mismo que usa la migración: client_credentials y luego
	// token-exchange con requested_subject. Dos, y solo dos: Test no mintea nada
	// (sondea sin token, y por eso Drive le respondió 401 sin que eso sea un fallo).
	if len(*forms) != 2 {
		t.Fatalf("canjes al token endpoint = %d, quiero 2 (los de la consulta)", len(*forms))
	}
	first, second := (*forms)[0], (*forms)[1]
	if first.Get("grant_type") != "client_credentials" || first.Get("client_id") != delegateClientID {
		t.Fatalf("primer canje = %v, quiero client_credentials del client delegable", first)
	}
	if second.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" ||
		second.Get("requested_subject") != personSubject {
		t.Fatalf("segundo canje = %v, quiero token-exchange acotado a %s", second, personSubject)
	}
	// El secreto viaja en el CUERPO, nunca en la URL ni en cabecera.
	if second.Get("client_secret") != delegateSecret {
		t.Fatalf("el canje no llevó el secreto del client delegable en el cuerpo")
	}
}

// TestSytueDriveNeedsInternalRuntime afirma que el camino interno es LO QUE LO HACE
// POSIBLE: con el Runtime de un tercero, el mismo destino (loopback, como drive:8082)
// lo corta la barrera SSRF. Si esto dejara de fallar, Internal habría dejado de ser
// necesario… o la barrera habría dejado de proteger a los conectores de terceros.
func TestSytueDriveNeedsInternalRuntime(t *testing.T) {
	srv, _, _ := sytueSuite(t)
	_, err := graph.New(sytueoauth2.NewCache()).Query(t.Context(), sytueInstance(srv.URL), runtime(),
		[]byte(`{"resource":"me/drive/root/children"}`))
	if !errors.Is(err, ssrfhttp.ErrBlocked) {
		t.Fatalf("error = %v, quiero ssrfhttp.ErrBlocked (sin Internal el destino privado se corta)", err)
	}
}
