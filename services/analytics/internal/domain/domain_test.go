// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain_test

import (
	"bytes"
	"crypto/rand"
	"net/netip"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/analytics/internal/domain"
)

func randSalt(t *testing.T) []byte {
	t.Helper()
	s := make([]byte, 32)
	if _, err := rand.Read(s); err != nil {
		t.Fatalf("generar sal: %v", err)
	}
	return s
}

// El ADR 0026 vende «no se puede perfilar entre días» como una IMPOSIBILIDAD, no
// como una política. Esto lo comprueba: con la sal de otro día, el mismo
// visitante (misma IP, mismo user-agent, mismo sitio) produce un identificador
// sin ninguna relación con el de ayer. Y como la sal de ayer se borra, nadie
// puede rehacer el cálculo ni con acceso root.
func TestVisitorEsIncorrelacionableEntreDias(t *testing.T) {
	t.Parallel()
	const (
		site = "11111111-1111-1111-1111-111111111111"
		ip   = "203.0.113.7"
		ua   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/126.0.0.0"
	)
	ayer, hoy := randSalt(t), randSalt(t)

	a := domain.Visitor(ayer, site, ip, ua)
	b := domain.Visitor(hoy, site, ip, ua)

	if len(a) != 16 || len(b) != 16 {
		t.Fatalf("el identificador debe medir 16 bytes: %d y %d", len(a), len(b))
	}
	if bytes.Equal(a, b) {
		t.Fatal("mismo identificador en dos días distintos: el perfilado entre días sería posible")
	}
	// Determinista dentro del mismo día (si no, no se contarían únicos).
	if !bytes.Equal(a, domain.Visitor(ayer, site, ip, ua)) {
		t.Fatal("el identificador no es estable dentro del mismo día")
	}
}

// La misma persona en los sitios de dos clientes: incorrelacionable, y no por
// contrato sino porque el site_id va DENTRO del HMAC.
func TestVisitorEsIncorrelacionableEntreSitios(t *testing.T) {
	t.Parallel()
	const (
		ip = "203.0.113.7"
		ua = "Mozilla/5.0 (X11; Linux x86_64) Firefox/128.0"
	)
	salt := randSalt(t)

	a := domain.Visitor(salt, "11111111-1111-1111-1111-111111111111", ip, ua)
	b := domain.Visitor(salt, "22222222-2222-2222-2222-222222222222", ip, ua)

	if bytes.Equal(a, b) {
		t.Fatal("mismo identificador en dos sitios: se podría seguir a una persona entre clientes")
	}
}

// El identificador no debe filtrar la entrada ni siquiera parcialmente.
func TestVisitorNoContieneLaEntrada(t *testing.T) {
	t.Parallel()
	const ip = "198.51.100.23"
	const ua = "Mozilla/5.0 (Windows NT 10.0) Chrome/126.0.0.0"
	v := domain.Visitor(randSalt(t), "site", ip, ua)
	if bytes.Contains(v, []byte(ip)) || bytes.Contains(v, []byte("Windows")) {
		t.Fatal("el identificador contiene material de la entrada en claro")
	}
}

func TestCleanPathTiraLaQueryString(t *testing.T) {
	t.Parallel()
	casos := []struct{ in, want string }{
		{"/precios?email=alguien@ejemplo.com", "/precios"},
		{"/reset?reset_token=abc123&x=1", "/reset"},
		{"/a#seccion", "/a"},
		{"/a?b#c", "/a"},
		{"sin-barra", "/sin-barra"},
		{"", "/"},
		{"/con\x00control", "/concontrol"},
	}
	for _, c := range casos {
		if got := domain.CleanPath(c.in); got != c.want {
			t.Errorf("CleanPath(%q) = %q, se esperaba %q", c.in, got, c.want)
		}
	}
	largo := "/" + strings.Repeat("a", 400)
	if got := domain.CleanPath(largo); len(got) != domain.MaxPath {
		t.Errorf("una ruta larguísima debe recortarse a %d, midió %d", domain.MaxPath, len(got))
	}
}

func TestCleanHostSoloDevuelveHosts(t *testing.T) {
	t.Parallel()
	casos := []struct{ in, want string }{
		{"https://Noticias.Ejemplo.com/articulo?utm_source=x", "noticias.ejemplo.com"},
		{"ejemplo.com:8443", "ejemplo.com"},
		{"http://usuario:clave@ejemplo.com/x", "ejemplo.com"},
		{"localhost", ""},
		{"no un host", ""},
		{"", ""},
	}
	for _, c := range casos {
		if got := domain.CleanHost(c.in); got != c.want {
			t.Errorf("CleanHost(%q) = %q, se esperaba %q", c.in, got, c.want)
		}
	}
}

// Sin allowlist, data-sytue-event="pepe@correo.com" convertiría el almacén
// anónimo en un canal de exfiltración.
func TestCleanEventNameExigeAllowlist(t *testing.T) {
	t.Parallel()
	allow := []string{"alta"}
	if got := domain.CleanEventName("alta", allow); got != "alta" {
		t.Errorf("un evento declarado debe pasar, salió %q", got)
	}
	if got := domain.CleanEventName("pepe@correo.com", allow); got != "" {
		t.Errorf("un evento NO declarado debe descartarse, salió %q", got)
	}
	if got := domain.CleanEventName("", allow); got != domain.EventPageview {
		t.Errorf("sin nombre se asume pageview, salió %q", got)
	}
	if got := domain.CleanEventName("outbound", nil); got != "outbound" {
		t.Errorf("los automáticos no pasan por la allowlist, salió %q", got)
	}
}

func TestIsBot(t *testing.T) {
	t.Parallel()
	bots := []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"Mozilla/5.0 (compatible; UptimeRobot/2.0; http://www.uptimerobot.com/)",
		"facebookexternalhit/1.1",
		"Twitterbot/1.0",
		"curl/8.4.0",
		"", // sin user-agent no hay navegador
	}
	humanos := []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	}
	for _, ua := range bots {
		if !domain.IsBot(ua) {
			t.Errorf("no detectado como bot: %q", ua)
		}
	}
	for _, ua := range humanos {
		if domain.IsBot(ua) {
			t.Errorf("falso positivo, es un navegador real: %q", ua)
		}
	}
}

func TestParseUA(t *testing.T) {
	t.Parallel()
	casos := []struct {
		ua                  string
		browser, os, device string
	}{
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36", "Chrome", "macOS", "desktop"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 Version/17.5 Mobile/15E148 Safari/604.1", "Safari", "iOS", "mobile"},
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/126.0.0.0 Mobile Safari/537.36", "Chrome", "Android", "mobile"},
		{"Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 Version/17.5 Safari/604.1", "Safari", "iOS", "tablet"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36 Edg/126.0", "Edge", "Windows", "desktop"},
	}
	for _, c := range casos {
		got := domain.ParseUA(c.ua)
		if got.Browser != c.browser || got.OS != c.os || got.Device != c.device {
			t.Errorf("ParseUA(%.40s…) = %+v, se esperaba %s/%s/%s", c.ua, got, c.browser, c.os, c.device)
		}
	}
}

// El país se resuelve en memoria a partir de la IP; la IP no se guarda. El
// fichero real de DB-IP viaja en la imagen, así que aquí se ejercita el parser y
// la búsqueda binaria con un extracto del mismo formato.
func TestCountriesLookup(t *testing.T) {
	t.Parallel()
	csv := strings.Join([]string{
		"0.0.0.0,0.255.255.255,ZZ",
		"1.0.0.0,1.0.0.255,AU",
		"81.0.0.0,81.255.255.255,ES",
		"90.0.0.0,90.255.255.255,fr",
		"2001:db8::,2001:db8:ffff:ffff:ffff:ffff:ffff:ffff,DE",
	}, "\n")
	c, err := domain.ParseCountries(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("parsear rangos: %v", err)
	}
	casos := []struct{ ip, want string }{
		{"81.44.1.2", "ES"},
		{"90.1.1.1", "FR"},
		{"1.0.0.9", "AU"},
		{"2001:db8::1", "DE"},
		{"::ffff:81.44.1.2", "ES"}, // IPv4 mapeada
		{"8.8.8.8", domain.CountryUnknown},
		{"2001:dead::1", domain.CountryUnknown},
	}
	for _, k := range casos {
		addr := netip.MustParseAddr(k.ip)
		if got := c.Of(addr); got != k.want {
			t.Errorf("Of(%s) = %q, se esperaba %q", k.ip, got, k.want)
		}
	}
	// Sin fichero cargado el servicio sigue midiendo, solo que sin país.
	var vacio *domain.Countries
	if got := vacio.Of(netip.MustParseAddr("81.44.1.2")); got != domain.CountryUnknown {
		t.Errorf("sin base de países debe salir %q, salió %q", domain.CountryUnknown, got)
	}
	if got := c.Of(netip.Addr{}); got != domain.CountryUnknown {
		t.Errorf("una IP inválida debe salir %q, salió %q", domain.CountryUnknown, got)
	}
}
