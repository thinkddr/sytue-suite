// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Los métodos reales siguen componiendo la URL del proveedor y autenticando
// la petición; sólo el transporte del test la envía al servidor TLS de prueba.
func capacityTransport(t *testing.T, host string, handler http.HandlerFunc) http.RoundTripper {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	local, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := server.Client().Transport
	return dropboxTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" || req.URL.Host != host {
			t.Fatalf("destino inesperado: %s", req.URL)
		}
		forward := req.Clone(req.Context())
		forward.URL.Scheme, forward.URL.Host = local.Scheme, local.Host
		return transport.RoundTrip(forward)
	})
}

func capacityPtr(n int64) *int64 { return &n }

func requireCapacity(t *testing.T, got, want Capacity) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		t.Fatalf("capacidad: got %s; want %s", g, w)
	}
}

func TestGoogleDriveCapacityUsesSharedUsageAndPreservesUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       Capacity
		bad        bool
	}{
		{"shared", `{"storageQuota":{"limit":"100","usage":"30","usageInDrive":"10"}}`, Capacity{TotalBytes: capacityPtr(100), UsedBytes: capacityPtr(30), AvailableBytes: capacityPtr(70)}, false},
		{"zero", `{"storageQuota":{"limit":"0","usage":"0"}}`, Capacity{TotalBytes: capacityPtr(0), UsedBytes: capacityPtr(0), AvailableBytes: capacityPtr(0)}, false},
		{"over_quota", `{"storageQuota":{"limit":"20","usage":"30"}}`, Capacity{TotalBytes: capacityPtr(20), UsedBytes: capacityPtr(30), AvailableBytes: capacityPtr(0)}, false},
		{"no_limit", `{"storageQuota":{"usage":"30"}}`, Capacity{UsedBytes: capacityPtr(30)}, false},
		{"no_usage", `{"storageQuota":{"limit":"100"}}`, Capacity{TotalBytes: capacityPtr(100)}, false},
		{"missing", `{}`, Capacity{}, false},
		{"null", `{"storageQuota":{"limit":null,"usage":"0"}}`, Capacity{UsedBytes: capacityPtr(0)}, false},
		{"negative", `{"storageQuota":{"limit":"-1","usage":"0"}}`, Capacity{}, true},
		{"overflow", `{"storageQuota":{"limit":"9223372036854775808","usage":"0"}}`, Capacity{}, true},
		{"fraction", `{"storageQuota":{"limit":"1.5","usage":"0"}}`, Capacity{}, true},
		{"malformed", `{"storageQuota":`, Capacity{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGoogleDrive(func(context.Context) (string, error) { return "consent", nil }, nil)
			g.http.Transport = capacityTransport(t, "www.googleapis.com", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/drive/v3/about" ||
					r.URL.Query().Get("fields") != "storageQuota(limit,usage)" || r.Header.Get("Authorization") != "Bearer consent" {
					t.Errorf("petición de cuota incorrecta: %s %s %v", r.Method, r.URL, r.Header)
				}
				_, _ = io.WriteString(w, tc.body)
			})
			got, err := g.Capacity(context.Background())
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v; want bad=%v", err, tc.bad)
			}
			requireCapacity(t, got, tc.want)
		})
	}
}

func TestOneDriveCapacityPreservesExplicitRemaining(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       Capacity
		bad        bool
	}{
		{"explicit", `{"quota":{"total":100,"used":30,"remaining":5}}`, Capacity{TotalBytes: capacityPtr(100), UsedBytes: capacityPtr(30), AvailableBytes: capacityPtr(5)}, false},
		{"zero_remaining", `{"quota":{"total":100,"used":30,"remaining":0}}`, Capacity{TotalBytes: capacityPtr(100), UsedBytes: capacityPtr(30), AvailableBytes: capacityPtr(0)}, false},
		{"derived", `{"quota":{"total":100,"used":30}}`, Capacity{TotalBytes: capacityPtr(100), UsedBytes: capacityPtr(30), AvailableBytes: capacityPtr(70)}, false},
		{"all_zero", `{"quota":{"total":0,"used":0,"remaining":0}}`, Capacity{TotalBytes: capacityPtr(0), UsedBytes: capacityPtr(0), AvailableBytes: capacityPtr(0)}, false},
		{"remaining_only", `{"quota":{"remaining":4}}`, Capacity{AvailableBytes: capacityPtr(4)}, false},
		{"missing", `{"quota":{}}`, Capacity{}, false},
		{"negative", `{"quota":{"total":100,"used":-1,"remaining":80}}`, Capacity{}, true},
		{"overflow", `{"quota":{"total":100,"used":0,"remaining":9223372036854775808}}`, Capacity{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := NewOneDrive(func(context.Context) (string, error) { return "consent", nil })
			o.http.Transport = capacityTransport(t, "graph.microsoft.com", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1.0/me/drive" ||
					r.URL.Query().Get("$select") != "quota" || r.Header.Get("Authorization") != "Bearer consent" {
					t.Errorf("petición de cuota incorrecta: %s %s %v", r.Method, r.URL, r.Header)
				}
				_, _ = io.WriteString(w, tc.body)
			})
			got, err := o.Capacity(context.Background())
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v; want bad=%v", err, tc.bad)
			}
			requireCapacity(t, got, tc.want)
		})
	}
}

func TestDropboxCapacityAccountsForTeamAndMemberLimits(t *testing.T) {
	team := func(kind, limit, used string) string {
		return fmt.Sprintf(`{"used":40,"allocation":{".tag":"team","allocated":1000,"used":800,"user_within_team_space_allocated":%s,"user_within_team_space_used_cached":%s,"user_within_team_space_limit_type":{".tag":%q}}}`, limit, used, kind)
	}
	for _, tc := range []struct {
		name, body string
		want       Capacity
		bad        bool
	}{
		{"individual", `{"used":30,"allocation":{".tag":"individual","allocated":100}}`, Capacity{TotalBytes: capacityPtr(100), UsedBytes: capacityPtr(30), AvailableBytes: capacityPtr(70)}, false},
		{"zero", `{"used":0,"allocation":{".tag":"individual","allocated":0}}`, Capacity{TotalBytes: capacityPtr(0), UsedBytes: capacityPtr(0), AvailableBytes: capacityPtr(0)}, false},
		{"team_off", team("off", "100", "70"), Capacity{TotalBytes: capacityPtr(240), UsedBytes: capacityPtr(40), AvailableBytes: capacityPtr(200)}, false},
		{"team_alert", team("alert_only", "100", "70"), Capacity{TotalBytes: capacityPtr(240), UsedBytes: capacityPtr(40), AvailableBytes: capacityPtr(200)}, false},
		{"team_member_limit", team("stop_sync", "100", "70"), Capacity{TotalBytes: capacityPtr(70), UsedBytes: capacityPtr(40), AvailableBytes: capacityPtr(30)}, false},
		{"team_limit_tighter", team("stop_sync", "1000", "70"), Capacity{TotalBytes: capacityPtr(240), UsedBytes: capacityPtr(40), AvailableBytes: capacityPtr(200)}, false},
		{"team_member_full", team("stop_sync", "100", "101"), Capacity{TotalBytes: capacityPtr(40), UsedBytes: capacityPtr(40), AvailableBytes: capacityPtr(0)}, false},
		{"team_no_member_limit", team("stop_sync", "0", "70"), Capacity{TotalBytes: capacityPtr(240), UsedBytes: capacityPtr(40), AvailableBytes: capacityPtr(200)}, false},
		{"team_missing_member_usage", team("stop_sync", "100", "null"), Capacity{UsedBytes: capacityPtr(40)}, false},
		{"team_unknown_limit", team("new_limit", "100", "70"), Capacity{UsedBytes: capacityPtr(40)}, false},
		{"other_members_fill_team", `{"used":40,"allocation":{".tag":"team","allocated":1000,"used":995,"user_within_team_space_limit_type":{".tag":"off"}}}`, Capacity{TotalBytes: capacityPtr(45), UsedBytes: capacityPtr(40), AvailableBytes: capacityPtr(5)}, false},
		{"effective_total_overflow", `{"used":9223372036854775807,"allocation":{".tag":"team","allocated":1000,"used":999,"user_within_team_space_limit_type":{".tag":"off"}}}`, Capacity{}, true},
		{"missing_allocation", `{"used":30}`, Capacity{UsedBytes: capacityPtr(30)}, false},
		{"missing", `{}`, Capacity{}, false},
		{"negative", `{"used":-1,"allocation":{".tag":"individual","allocated":100}}`, Capacity{}, true},
		{"overflow", `{"used":0,"allocation":{".tag":"individual","allocated":18446744073709551615}}`, Capacity{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDropbox(func(context.Context) (string, error) { return "consent", nil })
			d.http.Transport = capacityTransport(t, "api.dropboxapi.com", func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodPost || r.URL.Path != "/2/users/get_space_usage" ||
					r.Header.Get("Authorization") != "Bearer consent" || strings.TrimSpace(string(body)) != "null" {
					t.Errorf("petición de cuota incorrecta: %s %s %v body=%s", r.Method, r.URL, r.Header, body)
				}
				_, _ = io.WriteString(w, tc.body)
			})
			got, err := d.Capacity(context.Background())
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v; want bad=%v", err, tc.bad)
			}
			requireCapacity(t, got, tc.want)
		})
	}
}

func TestCapacityProviderErrorsDoNotGrantSpace(t *testing.T) {
	for _, tc := range []struct {
		name, host string
		newReader  func(TokenSource, http.RoundTripper) CapacityReader
	}{
		{"google", "www.googleapis.com", func(tok TokenSource, tr http.RoundTripper) CapacityReader {
			g := NewGoogleDrive(tok, nil)
			g.http.Transport = tr
			return g
		}},
		{"onedrive", "graph.microsoft.com", func(tok TokenSource, tr http.RoundTripper) CapacityReader {
			o := NewOneDrive(tok)
			o.http.Transport = tr
			return o
		}},
		{"dropbox", "api.dropboxapi.com", func(tok TokenSource, tr http.RoundTripper) CapacityReader {
			d := NewDropbox(tok)
			d.http.Transport = tr
			return d
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
				t.Run(fmt.Sprintf("http_%d", code), func(t *testing.T) {
					reader := tc.newReader(func(context.Context) (string, error) { return "consent", nil },
						capacityTransport(t, tc.host, func(w http.ResponseWriter, _ *http.Request) {
							w.WriteHeader(code)
							_, _ = io.WriteString(w, `{"error":"missing_scope"}`)
						}))
					got, err := reader.Capacity(context.Background())
					if err == nil {
						t.Fatal("un error del proveedor no puede acreditar espacio")
					}
					requireCapacity(t, got, Capacity{})
				})
			}
			tokenError := errors.New("token revocado")
			reader := tc.newReader(func(ctx context.Context) (string, error) {
				deadline, has := ctx.Deadline()
				if !has || time.Until(deadline) > 3*time.Second {
					t.Error("el sondeo de capacidad tiene que tener un plazo total máximo de tres segundos")
				}
				return "", tokenError
			},
				dropboxTransport(func(*http.Request) (*http.Response, error) {
					t.Fatal("sin token no se hace HTTP")
					return nil, tokenError
				}))
			got, err := reader.Capacity(context.Background())
			if !errors.Is(err, tokenError) {
				t.Fatalf("se perdió el error de token: %v", err)
			}
			requireCapacity(t, got, Capacity{})
		})
	}
}

func TestWebDAVCapacityUnderstandsPropertyStatusAndScope(t *testing.T) {
	response := func(href, props string) string {
		return `<D:multistatus xmlns:D="DAV:"><D:response><D:href>` + href + `</D:href>` + props + `</D:response></D:multistatus>`
	}
	props := func(code int, value string) string {
		return fmt.Sprintf(`<D:propstat><D:prop>%s</D:prop><D:status>HTTP/1.1 %d status</D:status></D:propstat>`, value, code)
	}
	for _, tc := range []struct {
		name, body string
		code       int
		want       Capacity
		bad        bool
	}{
		{"known", response("/Sytue/", props(200, `<D:quota-used-bytes>20</D:quota-used-bytes><D:quota-available-bytes>80</D:quota-available-bytes>`)), 207, Capacity{TotalBytes: capacityPtr(100), UsedBytes: capacityPtr(20), AvailableBytes: capacityPtr(80)}, false},
		{"zero", response("/Sytue/", props(200, `<D:quota-used-bytes>0</D:quota-used-bytes><D:quota-available-bytes>0</D:quota-available-bytes>`)), 207, Capacity{TotalBytes: capacityPtr(0), UsedBytes: capacityPtr(0), AvailableBytes: capacityPtr(0)}, false},
		{"missing_property", response("/Sytue/", props(404, `<D:quota-available-bytes/>`)+props(200, `<D:quota-used-bytes>20</D:quota-used-bytes>`)), 207, Capacity{UsedBytes: capacityPtr(20)}, false},
		{"available_only", response("/Sytue/", props(200, `<D:quota-available-bytes>80</D:quota-available-bytes>`)), 207, Capacity{AvailableBytes: capacityPtr(80)}, false},
		{"unknown", response("/Sytue/", props(404, `<D:quota-used-bytes/><D:quota-available-bytes/>`)), 207, Capacity{}, false},
		{"other_collection", response("/otra/", props(200, `<D:quota-available-bytes>9999</D:quota-available-bytes>`)), 207, Capacity{}, false},
		{"other_origin", response("https://otra.example/Sytue/", props(200, `<D:quota-available-bytes>9999</D:quota-available-bytes>`)), 207, Capacity{}, false},
		{"wrong_namespace", response("/Sytue/", props(200, `<X:quota-available-bytes xmlns:X="other:">9999</X:quota-available-bytes>`)), 207, Capacity{}, false},
		{"empty", response("/Sytue/", props(200, `<D:quota-available-bytes/>`)), 207, Capacity{}, true},
		{"negative", response("/Sytue/", props(200, `<D:quota-available-bytes>-3</D:quota-available-bytes>`)), 207, Capacity{}, true},
		{"overflow", response("/Sytue/", props(200, `<D:quota-available-bytes>9223372036854775808</D:quota-available-bytes>`)), 207, Capacity{}, true},
		{"total_overflow", response("/Sytue/", props(200, `<D:quota-used-bytes>9223372036854775807</D:quota-used-bytes><D:quota-available-bytes>1</D:quota-available-bytes>`)), 207, Capacity{}, true},
		{"duplicate", response("/Sytue/", props(200, `<D:quota-available-bytes>1</D:quota-available-bytes><D:quota-available-bytes>9999</D:quota-available-bytes>`)), 207, Capacity{}, true},
		{"duplicate_response", `<D:multistatus xmlns:D="DAV:"><D:response><D:href>/Sytue/</D:href></D:response><D:response><D:href>/Sytue/</D:href></D:response></D:multistatus>`, 207, Capacity{}, true},
		{"malformed_property_status", response("/Sytue/", `<D:propstat><D:prop><D:quota-available-bytes>9999</D:quota-available-bytes></D:prop><D:status>HTTP/1.1</D:status></D:propstat>`), 207, Capacity{}, false},
		{"not_http_property_status", response("/Sytue/", `<D:propstat><D:prop><D:quota-available-bytes>9999</D:quota-available-bytes></D:prop><D:status>other 200 status</D:status></D:propstat>`), 207, Capacity{}, false},
		{"malformed", `<D:multistatus`, 207, Capacity{}, true},
		{"resource_missing", ``, 404, Capacity{}, false},
		{"unsupported", ``, 405, Capacity{}, false},
		{"forbidden", ``, 403, Capacity{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := NewWebDAV("https://nas.example/", "Sytue", "usuario", "clave")
			if err != nil {
				t.Fatal(err)
			}
			w.http.Transport = capacityTransport(t, "nas.example", func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				user, password, auth := r.BasicAuth()
				if r.Method != "PROPFIND" || r.URL.Path != "/Sytue/" || r.Header.Get("Depth") != "0" ||
					!auth || user != "usuario" || password != "clave" || !strings.Contains(string(body), "quota-available-bytes") || !strings.Contains(string(body), "quota-used-bytes") {
					t.Errorf("PROPFIND de cuota incorrecto: %s %s %v body=%s", r.Method, r.URL, r.Header, body)
				}
				w.WriteHeader(tc.code)
				_, _ = io.WriteString(w, tc.body)
			})
			got, err := w.Capacity(context.Background())
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v; want bad=%v", err, tc.bad)
			}
			requireCapacity(t, got, tc.want)
		})
	}
}

func TestCapacityReadsAreBoundedAndWebDAVDoesNotRedirect(t *testing.T) {
	g := NewGoogleDrive(func(context.Context) (string, error) { return "consent", nil }, nil)
	g.http.Transport = capacityTransport(t, "www.googleapis.com", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"padding":"`+strings.Repeat("x", maxCapacityResponse)+`"}`)
	})
	got, err := g.Capacity(context.Background())
	if err == nil {
		t.Fatal("se aceptó una respuesta de cuota sin cota")
	}
	requireCapacity(t, got, Capacity{})
	w, err := NewWebDAV("https://nas.example/", "", "usuario", "clave")
	if err != nil {
		t.Fatal(err)
	}
	w.http.Transport = capacityTransport(t, "nas.example", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://otra.example/privado")
		w.WriteHeader(http.StatusMovedPermanently)
	})
	got, err = w.Capacity(context.Background())
	if err == nil {
		t.Fatal("la cuota WebDAV siguió o aceptó una redirección")
	}
	requireCapacity(t, got, Capacity{})
}
