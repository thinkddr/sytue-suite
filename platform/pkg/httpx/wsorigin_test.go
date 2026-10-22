package httpx

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestWSOriginPatterns_anadeElHostDeLaPeticionYNadaMas(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://corporate.sytue.eu/rooms/api/v1/ws", http.NoBody)
	got := WSOriginPatterns(r, []string{"rooms.sytue.eu", "one.sytue.eu"})
	want := []string{"rooms.sytue.eu", "one.sytue.eu", "corporate.sytue.eu"}
	if !slices.Equal(got, want) {
		t.Fatalf("WSOriginPatterns = %v, quería %v", got, want)
	}
	// Ningún patrón casa una publicación de cliente ni un host que solo se parece.
	for _, host := range []string{"x.apps.sytue.eu", "pr-42.preview.sytue.eu", "corporate.sytue.eu.atacante.com"} {
		for _, p := range got {
			if ok, _ := filepath.Match(p, host); ok {
				t.Errorf("el patrón %q autoriza %q", p, host)
			}
		}
	}
}

// El caso que rompió Rooms en producción: la página vive en el host de la
// organización y el socket va al del servicio, así que el `Origin` del navegador
// no es ninguno de los dos que había en la lista.
func TestWSOriginPatterns_anadeElHostDeLaOrganizacion(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://rooms.sytue.eu/ws", http.NoBody)
	got := WSOriginPatterns(r, []string{"rooms.sytue.eu", "one.sytue.eu"}, "corporate")

	if !slices.Contains(got, "corporate.sytue.eu") {
		t.Fatalf("falta el host de la organización: %v", got)
	}
	// Y sigue sin abrirse a lo que no debe.
	for _, host := range []string{
		"x.apps.sytue.eu", "pr-42.preview.sytue.eu",
		"corporate.sytue.eu.atacante.com", "otra.sytue.eu",
	} {
		for _, p := range got {
			if ok, _ := filepath.Match(p, host); ok {
				t.Errorf("el patrón %q autoriza %q", p, host)
			}
		}
	}
}

func TestWSOriginPatterns_leeLaOrganizacionDelContexto(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://docs.sytue.eu/api/v1/documents/x/ws", http.NoBody)
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	if got := WSOriginPatterns(r, []string{"docs.sytue.eu"}); !slices.Contains(got, "acme.sytue.eu") {
		t.Fatalf("sin slug explícito debía salir del contexto: %v", got)
	}
}

// Un slug inválido NO se convierte en patrón: con `*` o `?`, `filepath.Match`
// abriría el socket a media zona.
func TestWSOriginPatterns_slugInvalidoNoEntra(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "https://rooms.sytue.eu/ws", http.NoBody)
	for _, malo := range []string{"*", "?", "a.b", "-mal", "MAYUS", ""} {
		got := WSOriginPatterns(r, []string{"rooms.sytue.eu"}, malo)
		if len(got) != 2 {
			t.Errorf("el slug %q se coló como patrón: %v", malo, got)
		}
	}
}

// Sin zona no hay dónde colgar la organización, y no se inventa una.
func TestWSOriginPatterns_hostSinZona(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://localhost:4321/ws", http.NoBody)
	got := WSOriginPatterns(r, nil, "acme")
	if !slices.Equal(got, []string{"localhost:4321"}) {
		t.Fatalf("WSOriginPatterns = %v, quería solo el host de la petición", got)
	}
}

// El puerto se conserva, porque el Origin que compara nhooyr también lo lleva.
func TestWSOriginPatterns_conservaElPuerto(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://rooms.sytue.localhost:8085/ws", http.NoBody)
	if got := WSOriginPatterns(r, nil, "acme"); !slices.Contains(got, "acme.sytue.localhost:8085") {
		t.Fatalf("faltó el puerto: %v", got)
	}
}
