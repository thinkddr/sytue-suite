// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package analyticshttp implementa las dos superficies HTTP de Sytue Analytics:
// el COLECTOR (público, sin autenticar, puerto 8094) y la API DE LECTURA (JWT de
// Sytue ID, puerto 8095). Son dos routers y dos binarios distintos a propósito:
// el colector es el endpoint de mayor volumen y menor confianza de la
// plataforma, y no debe compartir proceso con nada que tenga credenciales.
package analyticshttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/clientip"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/analytics/internal/config"
	"git.sytue.eu/sytue/one/services/analytics/internal/domain"
	"git.sytue.eu/sytue/one/services/analytics/internal/store"
)

// maxBeaconBytes acota el cuerpo del beacon. Un evento legítimo mide ~200 B;
// esto deja margen de sobra y corta en seco cualquier intento de usar el
// colector como buzón.
const maxBeaconBytes = 4 << 10

// beacon es lo que envía el script. Nombres de una letra porque viaja en cada
// navegación de cada visitante de cada cliente.
type beacon struct {
	Name   string            `json:"n"`
	Path   string            `json:"p"`
	Ref    string            `json:"r"`
	UTM    map[string]string `json:"q"`
	Target string            `json:"x"`
}

// El colector declara como interfaces mínimas lo que necesita de la
// persistencia; las implementan *store.Store, *store.Salts y *store.Writer.
//
// No es abstracción por gusto: son las costuras por las que el test verifica SIN
// PostgreSQL las tres promesas del ADR que se sostienen o se rompen justo aquí
// —que la IP y el user-agent crudos no llegan a persistirse, que GPC/DNT no
// registran nada y que la query string se cae—. Una garantía que solo se
// comprueba con Docker delante es una garantía que un día deja de comprobarse.
type (
	// SiteResolver traduce la cabecera Host al sitio medido.
	SiteResolver interface {
		SiteByHost(ctx context.Context, host string) (store.Site, error)
	}
	// SaltSource entrega la sal del día vigente.
	SaltSource interface {
		Today(ctx context.Context) ([]byte, error)
	}
	// EventSink recibe los eventos aceptados.
	EventSink interface {
		Enqueue(domain.Event) bool
	}
)

// IngestDeps son las dependencias del colector.
type IngestDeps struct {
	Cfg           config.Config
	Sites         SiteResolver
	Salts         SaltSource
	Sink          EventSink
	Countries     *domain.Countries
	ReadyCheckers []httpx.Checker
}

type collector struct {
	cfg     config.Config
	sites   *siteCache
	salts   SaltSource
	sink    EventSink
	geo     *domain.Countries
	perSite *httpx.TokenBucket
	perIP   *httpx.TokenBucket
}

// NewIngestRouter construye el router del colector. Solo tres rutas: el script,
// el evento y las sondas. Superficie mínima porque es superficie hostil.
func NewIngestRouter(d IngestDeps) http.Handler {
	c := &collector{
		cfg:     d.Cfg,
		sites:   newSiteCache(d.Sites, d.Cfg.SiteCacheTTL),
		salts:   d.Salts,
		sink:    d.Sink,
		geo:     d.Countries,
		perSite: httpx.NewTokenBucket(d.Cfg.RatePerSite, d.Cfg.BurstPerSite),
		perIP:   httpx.NewTokenBucket(d.Cfg.RatePerIP, d.Cfg.BurstPerIP),
	}
	// Sin CORS: el colector vive bajo el dominio del propio sitio (/_sytue), así
	// que toda petición legítima es same-origin. Declarar CORS solo habilitaría
	// llamadas desde otros orígenes, que es exactamente lo que aquí se rechaza.
	r := httpx.NewRouter(httpx.RouterOptions{
		MaxBodyBytes:   maxBeaconBytes,
		RequestTimeout: 5 * time.Second,
	})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second, d.ReadyCheckers...)
	r.Get("/_sytue/s.js", c.serveScript)
	r.Post("/_sytue/e", c.collect)
	// NewRouter registra las sondas por defecto para que hasta los 404 atraviesen
	// la cadena común. Chi prohíbe Use después de registrar cualquier ruta, así
	// que el contexto de proxies se aplica envolviendo el handler ya construido.
	// Así también llegan a las sondas los mismos proxies de confianza que al
	// colector, sin alterar el orden inmutable del mux.
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.ServeHTTP(w, req.WithContext(clientip.WithTrustedProxies(req.Context(), d.Cfg.TrustedProxies)))
	})
}

// collect recibe un evento. SIEMPRE responde 204, se guarde o no: distinguir
// «sitio desconocido» de «bot» o de «GPC activo» sería un oráculo gratis para
// quien sondee el colector, y al script la respuesta le da igual.
//
//nolint:gocognit,gocyclo // es una cadena de guardas planas, cada una con su motivo; partirla las dispersa.
func (c *collector) collect(w http.ResponseWriter, r *http.Request) {
	// 1. Señales de privacidad del navegador, antes que nada. El servidor honra
	// Sec-GPC y DNT por si alguien esquiva el script; no hay interruptor.
	if r.Header.Get("Sec-Gpc") == "1" || r.Header.Get("Dnt") == "1" {
		httpx.NoContent(w)
		return
	}

	// 2. El JS de una página no puede falsificar Sec-Fetch-Site (cabecera
	// prohibida), así que un beacon marcado cross-site no viene del script.
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		httpx.NoContent(w)
		return
	}
	host := hostOnly(r.Host)
	if origin := r.Header.Get("Origin"); origin != "" {
		if u, err := url.Parse(origin); err != nil || hostOnly(u.Host) != host {
			httpx.NoContent(w)
			return
		}
	}

	// 3. EL SITIO ES EL HOSTNAME. No hay identificador en el script, así que no
	// se puede apuntar el beacon a A diciendo que eres B.
	site, ok := c.sites.get(r.Context(), host)
	if !ok {
		httpx.NoContent(w)
		return
	}

	// 4. Límite por sitio y por visitante. En memoria y no en PostgreSQL: un
	// viaje a la base por evento sería el cuello de botella del firehose.
	// Techo conocido y aceptado: con N réplicas el límite efectivo es N veces
	// el configurado — suficiente para amortiguar abuso.
	ip := c.clientAddr(r)
	ctx := r.Context()
	// Primero el de la IP: una IP ya limitada no gasta el cubo del sitio (COM-13).
	if !allowed(ctx, c.perIP, site.ID+"|"+ip.String()) || !allowed(ctx, c.perSite, site.ID) {
		httpx.Fail(w, r, httpx.ErrTooManyRequests)
		return
	}

	// 5. Bots ANTES de cualquier escritura: se descartan, no se marcan.
	userAgent := r.Header.Get("User-Agent")
	if domain.IsBot(userAgent) {
		httpx.NoContent(w)
		return
	}

	var b beacon
	body, err := io.ReadAll(r.Body)
	if err != nil || json.Unmarshal(body, &b) != nil {
		httpx.NoContent(w)
		return
	}

	name := domain.CleanEventName(b.Name, site.EventAllowlist)
	if name == "" {
		// Nombre de evento no declarado en el sitio: se descarta entero.
		httpx.NoContent(w)
		return
	}

	// 6. La sal del día. Sin ella no hay identidad y no se escribe nada: es
	// preferible perder eventos a inventarse un identificador estable.
	salt, err := c.salts.Today(r.Context())
	if err != nil {
		log.From(r.Context()).Error("sal del día no disponible", "error", err)
		httpx.NoContent(w)
		return
	}

	// 7. AQUÍ MUERE LA IP. Entra en el HMAC y en la búsqueda de país, y no se
	// asigna a ningún campo de domain.Event — que no tiene ninguno donde ponerla.
	// Lo mismo el user-agent crudo: de él solo sobreviven tres enums.
	dims := domain.ParseUA(userAgent)
	ev := domain.Event{
		Tenant:   site.Tenant,
		SiteID:   site.ID,
		TS:       time.Now().UTC(),
		Visitor:  domain.Visitor(salt, site.ID, ip.String(), userAgent),
		Name:     name,
		Path:     domain.CleanPath(b.Path),
		Referrer: refererHost(b.Ref, host),
		Target:   targetOf(name, b.Target),
		Country:  c.geo.Of(ip),
		Browser:  dims.Browser,
		OS:       dims.OS,
		Device:   dims.Device,
	}
	for i, k := range domain.UTMKeys {
		ev.UTM[i] = domain.CleanShort(b.UTM[k], domain.MaxUTM)
	}

	c.sink.Enqueue(ev)
	httpx.NoContent(w)
}

func allowed(ctx context.Context, tb *httpx.TokenBucket, key string) bool {
	res, err := tb.Allow(ctx, key)
	return err == nil && res.Allowed
}

// refererHost se queda SOLO con el host del referente y descarta el propio
// sitio (navegación interna, que no es una fuente de tráfico).
func refererHost(raw, self string) string {
	h := domain.CleanHost(raw)
	if h == self {
		return ""
	}
	return h
}

// targetOf acota el detalle a lo que cada evento automático admite: un hostname
// de destino o una extensión. Cualquier otra cosa se descarta.
func targetOf(name, raw string) string {
	switch name {
	case domain.EventOutbound:
		return domain.CleanHost(raw)
	case domain.EventDownload:
		ext := strings.ToLower(domain.CleanShort(raw, 8))
		for i := 0; i < len(ext); i++ {
			if (ext[i] < 'a' || ext[i] > 'z') && (ext[i] < '0' || ext[i] > '9') {
				return ""
			}
		}
		return ext
	default:
		return ""
	}
}

// hostOnly normaliza un Host de petición: minúsculas y sin puerto.
func hostOnly(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

// clientAddr resuelve la IP del cliente solo a través de un proxy de confianza.
// X-Real-IP no interviene: un cliente puede fijarla por sí mismo.
func (c *collector) clientAddr(r *http.Request) netip.Addr {
	ip := clientip.TrustedProxiesFrom(r.Context()).ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

// siteCache cachea la resolución Host -> sitio, incluidos los FALLOS: sin
// memoria negativa, cualquiera puede obligar al colector a consultar PostgreSQL
// una vez por petición con solo inventarse hostnames.
type siteCache struct {
	st  SiteResolver
	ttl time.Duration
	mu  sync.RWMutex
	m   map[string]cachedSite
}

type cachedSite struct {
	site  store.Site
	found bool
	at    time.Time
}

// maxCachedSites acota la memoria de la caché.
//
// ponytail: al desbordar se vacía entera en vez de expulsar por LRU. Con miles
// de sitios reales nunca se llega; con hostnames inventados, vaciar es más
// barato que mantener un orden de uso. Si algún día hay más sitios legítimos que
// esto, aquí entra una LRU.
const maxCachedSites = 10000

func newSiteCache(st SiteResolver, ttl time.Duration) *siteCache {
	return &siteCache{st: st, ttl: ttl, m: make(map[string]cachedSite)}
}

func (c *siteCache) get(ctx context.Context, host string) (store.Site, bool) {
	if host == "" {
		return store.Site{}, false
	}
	now := time.Now()
	c.mu.RLock()
	entry, ok := c.m[host]
	c.mu.RUnlock()
	if ok && now.Sub(entry.at) < c.ttl {
		return entry.site, entry.found
	}

	site, err := c.st.SiteByHost(ctx, host)
	switch {
	case err == nil:
		entry = cachedSite{site: site, found: true, at: now}
	case errors.Is(err, store.ErrNotFound):
		entry = cachedSite{found: false, at: now}
	default:
		// Fallo de la base: no se envenena la caché; se descarta el evento.
		log.From(ctx).Warn("resolver sitio falló", "error", err)
		return store.Site{}, false
	}

	c.mu.Lock()
	if len(c.m) >= maxCachedSites {
		c.m = make(map[string]cachedSite, 64)
	}
	c.m[host] = entry
	c.mu.Unlock()
	return entry.site, entry.found
}
