// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package dominios implementa la gestión del ciclo de vida de los dominios propios
// de las webs de Sytue Web (PAGES-ENCARGO §8.3): verificación TXT, comprobación
// de enrutado sin SSRF, prueba HTTPS en la 2.ª IP, revalidación y barrido periódico.
package dominios

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/net/publicsuffix"

	"git.sytue.eu/sytue/one/platform/pkg/dominio"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// DNSResolver define el subconjunto de resolución DNS requerido.
type DNSResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupIP(ctx context.Context, network, host string) ([]net.IP, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
}

// DialerFunc permite interceptar la conexión de la prueba HTTPS en tests.
type DialerFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// Servicio gestiona el alta, verificación, activación y barrido de dominios propios.
type Servicio struct {
	store    *store.Store
	cfg      config.Config
	escritor *mapa.Escritor
	hub      *hubnotify.Client
	logger   *slog.Logger
	resolver DNSResolver
	dialer   DialerFunc
}

// Nuevo construye el servicio de dominios.
func Nuevo(st *store.Store, cfg config.Config, escritor *mapa.Escritor, hub *hubnotify.Client, logger *slog.Logger) *Servicio {
	return &Servicio{
		store:    st,
		cfg:      cfg,
		escritor: escritor,
		hub:      hub,
		logger:   logger,
		resolver: net.DefaultResolver,
	}
}

// ConResolver sustituye el resolver DNS (para pruebas herméticas).
func (s *Servicio) ConResolver(r DNSResolver) *Servicio {
	s.resolver = r
	return s
}

// ConDialer sustituye el dialer de la prueba HTTPS (para pruebas con TLS local).
func (s *Servicio) ConDialer(d DialerFunc) *Servicio {
	s.dialer = d
	return s
}

// Listar devuelve todos los dominios de un sitio con sus instrucciones formateadas.
func (s *Servicio) Listar(ctx context.Context, tenant string, siteID uuid.UUID) ([]domain.DominioDTO, error) {
	if s.store == nil {
		return []domain.DominioDTO{}, nil
	}

	site, err := s.store.SitioCompleto(ctx, siteID)
	if err != nil {
		return nil, err
	}

	doms, err := s.store.ListarDominiosPorSitio(ctx, tenant, siteID)
	if err != nil {
		return nil, err
	}

	dtos := make([]domain.DominioDTO, 0, len(doms))
	for _, d := range doms {
		dtos = append(dtos, s.toDTO(d, site.CanonicalHost))
	}
	return dtos, nil
}

// Alta registra un dominio propio (o el par ápex + www si corresponde).
func (s *Servicio) Alta(ctx context.Context, tenant string, siteID uuid.UUID, rawHost, createdBy string) ([]domain.DominioDTO, error) {
	if s.store == nil {
		return nil, domain.ErrNoExiste
	}

	host, err := dominio.Normalizar(rawHost)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrDominioInvalido, err)
	}

	if s.esZonaPlataforma(host) {
		return nil, domain.ErrDominioReservado
	}

	site, err := s.store.SitioCompleto(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if site.Status == "suspended" {
		return nil, domain.ErrSitioSuspendido
	}

	// Tope de 5 grupos de dominios por sitio (ápex + www cuentan como 1, §8.3)
	grupos, err := s.store.ContarGruposDominios(ctx, tenant, siteID)
	if err != nil {
		return nil, err
	}
	if grupos >= 5 {
		return nil, domain.ErrLimiteDominios
	}

	apexHost, wwwHost, tienePar := detectarPar(host)

	if err := s.comprobarColisiones(ctx, tenant, siteID, host, apexHost, wwwHost, tienePar); err != nil {
		return nil, err
	}

	token, err := dominio.NewDomainToken()
	if err != nil {
		return nil, fmt.Errorf("generar token de dominio: %w", err)
	}

	creados, err := s.insertarDominios(ctx, tenant, siteID, createdBy, token, host, apexHost, wwwHost, tienePar)
	if err != nil {
		return nil, err
	}

	resultado := make([]domain.DominioDTO, 0, len(creados))
	for _, d := range creados {
		resultado = append(resultado, s.toDTO(d, site.CanonicalHost))
	}
	return resultado, nil
}

func detectarPar(host string) (apexHost, wwwHost string, tienePar bool) {
	tldPlusOne, _ := publicsuffix.EffectiveTLDPlusOne(host)
	if tldPlusOne == host {
		return host, "www." + host, true
	}
	if strings.HasPrefix(host, "www.") {
		trimmed := strings.TrimPrefix(host, "www.")
		if tld, _ := publicsuffix.EffectiveTLDPlusOne(trimmed); tld == trimmed {
			return trimmed, host, true
		}
	}
	return host, "", false
}

func (s *Servicio) comprobarColisiones(ctx context.Context, tenant string, siteID uuid.UUID, host, apexHost, wwwHost string, tienePar bool) error {
	existentes, err := s.store.ListarDominiosPorSitio(ctx, tenant, siteID)
	if err != nil {
		return err
	}

	hostsComprobar := []string{host}
	if tienePar {
		hostsComprobar = []string{apexHost, wwwHost}
	}

	for _, h := range hostsComprobar {
		for _, d := range existentes {
			if strings.EqualFold(d.Host, h) {
				return domain.ErrDominioEnUso
			}
		}
		enUso, err := s.store.DominioEnUso(ctx, h)
		if err != nil {
			return err
		}
		if enUso {
			return domain.ErrDominioEnUso
		}
	}
	return nil
}

func (s *Servicio) insertarDominios(ctx context.Context, tenant string, siteID uuid.UUID, createdBy, token, host, apexHost, wwwHost string, tienePar bool) ([]domain.Domain, error) {
	now := time.Now().UTC()
	if tienePar {
		apexID := uuid.New()
		dApex := &domain.Domain{
			ID:        apexID,
			Tenant:    tenant,
			SiteID:    siteID,
			Host:      apexHost,
			ParentID:  nil,
			Token:     token,
			Status:    domain.DomainStatusPending,
			CreatedBy: createdBy,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.store.CrearDominio(ctx, dApex); err != nil {
			return nil, err
		}

		wwwID := uuid.New()
		dWww := &domain.Domain{
			ID:        wwwID,
			Tenant:    tenant,
			SiteID:    siteID,
			Host:      wwwHost,
			ParentID:  &apexID,
			Token:     token,
			Status:    domain.DomainStatusPending,
			CreatedBy: createdBy,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.store.CrearDominio(ctx, dWww); err != nil {
			return nil, err
		}
		return []domain.Domain{*dApex, *dWww}, nil
	}

	domID := uuid.New()
	d := &domain.Domain{
		ID:        domID,
		Tenant:    tenant,
		SiteID:    siteID,
		Host:      host,
		ParentID:  nil,
		Token:     token,
		Status:    domain.DomainStatusPending,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.CrearDominio(ctx, d); err != nil {
		return nil, err
	}
	return []domain.Domain{*d}, nil
}

// Comprobar ejecuta a demanda la verificación completa de un dominio (§8.3):
// 1. Verificación TXT (_sytue.<ápex>).
// 2. Enrutado (A al ápex o CNAME/A a la 2.ª IP, sin AAAA).
// 3. Prueba HTTPS con SNI contra la 2.ª IP fija.
func (s *Servicio) Comprobar(ctx context.Context, tenant string, siteID, domainID uuid.UUID) (*domain.DominioDTO, error) {
	if s.store == nil {
		return nil, domain.ErrNoExiste
	}

	d, err := s.store.ObtenerDominio(ctx, tenant, siteID, domainID)
	if err != nil {
		return nil, err
	}

	site, err := s.store.SitioCompleto(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if site.Status == "suspended" {
		return nil, domain.ErrSitioSuspendido
	}

	// 1. Determinar el host ápex para el registro TXT
	apexHost := d.Host
	if d.ParentID != nil {
		apexHost = strings.TrimPrefix(d.Host, "www.")
	} else if tld, _ := publicsuffix.EffectiveTLDPlusOne(d.Host); tld != "" {
		apexHost = tld
	}

	now := time.Now().UTC()
	d.CheckedAt = &now

	check := dominio.CheckDomainTXT(ctx, s.resolver, apexHost, d.Token)
	if check.Status != dominio.DomainVerified {
		switch check.Status {
		case dominio.DomainMismatch:
			d.Detail = "txt_mismatch"
		case dominio.DomainUnreachable:
			d.Detail = "dns_unreachable"
		default:
			d.Detail = "txt_missing"
		}
		_ = s.store.ActualizarDominio(ctx, d)
		dto := s.toDTO(*d, site.CanonicalHost)
		return &dto, nil
	}

	// 2. Comprobar enrutado (sin SSRF)
	tld, _ := publicsuffix.EffectiveTLDPlusOne(d.Host)
	esApex := tld == d.Host
	enrutadoOK, detalle := s.comprobarEnrutado(ctx, d.Host, esApex)
	if !enrutadoOK {
		d.Detail = detalle
		_ = s.store.ActualizarDominio(ctx, d)
		dto := s.toDTO(*d, site.CanonicalHost)
		return &dto, nil
	}

	// Pasa a verified. Si otro sitio ya tiene ese host verificado, no hay transición y no
	// se sigue: antes el error se ignoraba y el flujo llegaba a fijar el canónico.
	d.Status = domain.DomainStatusVerified
	d.VerifiedAt = &now
	d.Detail = ""
	if err := s.guardar(ctx, d); err != nil {
		return nil, err
	}

	// 3. Prueba HTTPS: dial siempre a PAGES_IP_APEX:443 con SNI=host
	httpsOK := s.probarHTTPS(ctx, d.Host)
	if !httpsOK {
		d.Detail = "certificate"
		_ = s.store.ActualizarDominio(ctx, d)
		dto := s.toDTO(*d, site.CanonicalHost)
		return &dto, nil
	}

	// Activo!
	d.Status = domain.DomainStatusActive
	d.ActivatedAt = &now
	d.Detail = ""
	if err := s.guardar(ctx, d); err != nil {
		return nil, err
	}

	// El primer dominio que pasa a activo se hace canónico/principal si no hay ninguno (§8.3)
	if site.CanonicalHost == nil || *site.CanonicalHost == "" {
		if err := s.store.FijarCanonico(ctx, tenant, siteID, d.Host); err == nil {
			canon := d.Host
			site.CanonicalHost = &canon
		}
	}

	// Aviso al Hub
	if s.hub != nil {
		_ = s.hub.Send(ctx, hubnotify.Notification{
			Tenant:  tenant,
			Subject: d.CreatedBy,
			Source:  "pages",
			Type:    "pages.domain.active",
			Level:   "info",
			Title:   fmt.Sprintf("Dominio activo: %s", d.Host),
			Params: map[string]string{
				"domain":  d.Host,
				"site_id": siteID.String(),
				"message": fmt.Sprintf("Publica de nuevo para que %s sea la dirección principal", d.Host),
			},
		})
	}

	// Regenerar mapa
	if s.escritor != nil {
		_ = s.escritor.Regenerar(ctx)
	}

	dto := s.toDTO(*d, site.CanonicalHost)
	return &dto, nil
}

// guardar persiste el estado de un dominio. Si otro sitio ya tiene ese host verificado
// o activo (índice único `domains_host_en_uso`), es ErrDominioEnUso.
func (s *Servicio) guardar(ctx context.Context, d *domain.Domain) error {
	err := s.store.ActualizarDominio(ctx, d)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return domain.ErrDominioEnUso
	}
	return err
}

// HacerPrincipal convierte un dominio activo en la dirección canónica del sitio.
func (s *Servicio) HacerPrincipal(ctx context.Context, tenant string, siteID, domainID uuid.UUID) (*domain.DominioDTO, error) {
	if s.store == nil {
		return nil, domain.ErrNoExiste
	}

	d, err := s.store.ObtenerDominio(ctx, tenant, siteID, domainID)
	if err != nil {
		return nil, err
	}
	if d.Status != domain.DomainStatusActive {
		return nil, errors.New("el dominio debe estar activo para ser principal")
	}

	canon := d.Host
	if err := s.store.FijarCanonico(ctx, tenant, siteID, canon); err != nil {
		return nil, err
	}

	if s.escritor != nil {
		_ = s.escritor.Regenerar(ctx)
	}

	dto := s.toDTO(*d, &canon)
	return &dto, nil
}

// Borrar elimina un dominio y su par acompañante.
func (s *Servicio) Borrar(ctx context.Context, tenant string, siteID, domainID uuid.UUID) error {
	if s.store == nil {
		return domain.ErrNoExiste
	}

	if _, err := s.store.ObtenerDominio(ctx, tenant, siteID, domainID); err != nil {
		return err
	}

	if _, err := s.store.SitioCompleto(ctx, siteID); err != nil {
		return err
	}

	// Si era el canónico, el store lo desasigna en la misma transacción.
	if err := s.store.BorrarDominio(ctx, tenant, siteID, domainID); err != nil {
		return err
	}

	if s.escritor != nil {
		_ = s.escritor.Regenerar(ctx)
	}

	return nil
}

// IniciarBarrido arranca los procesos periódicos en segundo plano:
// - Cada 5 min: barrido de dominios pending (elimina tras 7 días o reintenta verificación).
// - Diario: revalidación de dominios active (falla tras 7 días de errores).
func (s *Servicio) IniciarBarrido(ctx context.Context) {
	if s.store == nil {
		return
	}

	go func() {
		ticker5m := time.NewTicker(5 * time.Minute)
		ticker24h := time.NewTicker(24 * time.Hour)
		defer ticker5m.Stop()
		defer ticker24h.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker5m.C:
				s.barridoPendientes(ctx)
			case <-ticker24h.C:
				s.revalidacionActivos(ctx)
			}
		}
	}()
}

func (s *Servicio) barridoPendientes(ctx context.Context) {
	pendientes, err := s.store.ObtenerDominiosPendientesBarrido(ctx)
	if err != nil {
		return
	}
	limite7d := time.Now().UTC().Add(-7 * 24 * time.Hour)
	for _, d := range pendientes {
		if d.CreatedAt.Before(limite7d) {
			_ = s.store.EliminarDominioPorID(ctx, d.Tenant, d.ID)
			continue
		}
		_, _ = s.Comprobar(ctx, d.Tenant, d.SiteID, d.ID)
	}
}

func (s *Servicio) revalidacionActivos(ctx context.Context) {
	activos, err := s.store.ObtenerDominiosActivosRevalidacion(ctx)
	if err != nil {
		return
	}
	for _, d := range activos {
		apexHost := d.Host
		if d.ParentID != nil {
			apexHost = strings.TrimPrefix(d.Host, "www.")
		} else if tld, _ := publicsuffix.EffectiveTLDPlusOne(d.Host); tld != "" {
			apexHost = tld
		}

		check := dominio.CheckDomainTXT(ctx, s.resolver, apexHost, d.Token)
		tldApex, _ := publicsuffix.EffectiveTLDPlusOne(d.Host)
		esApex := tldApex == d.Host
		enrutadoOK, detalle := s.comprobarEnrutado(ctx, d.Host, esApex)

		now := time.Now().UTC()
		d.RevalidatedAt = &now

		if check.Status != dominio.DomainVerified || !enrutadoOK {
			if detalle == "" {
				detalle = "txt_missing"
			}
			d.Detail = detalle
			// Si lleva 7 días fallando, se pasa a failed y sale del mapa (§8.3)
			if d.CheckedAt != nil && d.CheckedAt.Before(time.Now().UTC().Add(-7*24*time.Hour)) {
				d.Status = domain.DomainStatusFailed
				_ = s.store.ActualizarDominio(ctx, &d)
				if s.escritor != nil {
					_ = s.escritor.Regenerar(ctx)
				}
				if s.hub != nil {
					_ = s.hub.Send(ctx, hubnotify.Notification{
						Tenant:  d.Tenant,
						Subject: d.CreatedBy,
						Source:  "pages",
						Type:    "pages.domain.failed",
						Level:   "warning",
						Title:   fmt.Sprintf("Dominio fallido: %s", d.Host),
						Params: map[string]string{
							"domain":  d.Host,
							"site_id": d.SiteID.String(),
							"reason":  d.Detail,
						},
					})
				}
				continue
			}
			_ = s.store.ActualizarDominio(ctx, &d)
		} else {
			d.Detail = ""
			_ = s.store.ActualizarDominio(ctx, &d)
		}
	}
}

func (s *Servicio) comprobarEnrutado(ctx context.Context, host string, esApex bool) (bool, string) {
	if s.resolver == nil {
		return false, "routing"
	}

	ipApex := s.cfg.IPApex
	if ipApex == "" && s.cfg.Env == "dev" {
		ipApex = "127.0.0.1"
	}

	if esApex {
		return s.comprobarEnrutadoApex(ctx, host, ipApex)
	}
	return s.comprobarEnrutadoSubdominio(ctx, host, ipApex)
}

func (s *Servicio) comprobarEnrutadoApex(ctx context.Context, host, ipApex string) (bool, string) {
	if ipApex == "" {
		return false, "routing"
	}
	// Ápex: todos sus A iguales a PAGES_IP_APEX y ningún AAAA (§8.3)
	v6, err := s.resolver.LookupIP(ctx, "ip6", host)
	if err == nil && len(v6) > 0 {
		return false, "routing"
	}
	v4, err := s.resolver.LookupIP(ctx, "ip4", host)
	if err != nil || len(v4) == 0 {
		return false, "routing"
	}
	for _, ip := range v4 {
		if ip.String() != ipApex {
			return false, "routing"
		}
	}
	return true, ""
}

func (s *Servicio) comprobarEnrutadoSubdominio(ctx context.Context, host, ipApex string) (bool, string) {
	// Subdominio: LookupCNAME = sites.<dominio> o todos sus A iguales a la 2.ª IP y ningún AAAA
	cname, err := s.resolver.LookupCNAME(ctx, host)
	if err == nil {
		cnameLimpio := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cname)), ".")
		// El anterior sigue valiendo: un dominio propio que ya apuntaba a él no se cae al
		// pasar las webs al dominio de clientes (WEB-09).
		for _, destino := range []string{s.cfg.SitesDomain, s.cfg.SitesDomainAnterior} {
			destino = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(destino)), ".")
			if destino != "" && (cnameLimpio == destino || cnameLimpio == "sites."+destino) {
				return true, ""
			}
		}
	}

	// Fallback por registros A
	if ipApex != "" {
		v6, err := s.resolver.LookupIP(ctx, "ip6", host)
		if err == nil && len(v6) > 0 {
			return false, "routing"
		}
		v4, err := s.resolver.LookupIP(ctx, "ip4", host)
		if err == nil && len(v4) > 0 {
			for _, ip := range v4 {
				if ip.String() != ipApex {
					return false, "routing"
				}
			}
			return true, ""
		}
	}

	return false, "routing"
}

func (s *Servicio) probarHTTPS(ctx context.Context, host string) bool {
	ipApex := s.cfg.IPApex
	if ipApex == "" && s.cfg.Env == "dev" {
		ipApex = "127.0.0.1"
	}
	if ipApex == "" {
		return false
	}

	dialer := s.dialer
	if dialer == nil {
		d := &net.Dialer{Timeout: 60 * time.Second}
		dialer = func(ctx context.Context, network, _ string) (net.Conn, error) {
			target := net.JoinHostPort(ipApex, "443")
			return d.DialContext(ctx, network, target)
		}
	}

	transport := &http.Transport{
		DialContext: dialer,
		TLSClientConfig: &tls.Config{
			ServerName: host,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/", http.NoBody)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		if s.logger != nil {
			s.logger.Debug("prueba HTTPS fallo", "host", host, "err", err)
		}
		return false
	}
	_ = resp.Body.Close()
	return true
}

func (s *Servicio) esZonaPlataforma(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))

	// sytue.eu, sytue.com, sytue.localhost y todos sus subdominios (§8.3)
	reservados := []string{"sytue.eu", "sytue.com", "sytue.localhost"}
	for _, res := range reservados {
		if h == res || strings.HasSuffix(h, "."+res) {
			return true
		}
	}

	// Las zonas de las webs (la de hoy y la anterior) y el dominio de clientes ENTERO:
	// en él viven también los túneles NAS y las apps de Cloud de otros clientes (WEB-09).
	for _, zona := range []string{s.cfg.SitesDomain, s.cfg.SitesDomainAnterior, s.cfg.DominioDeClientes} {
		zona = strings.ToLower(strings.TrimSpace(zona))
		if zona != "" && (h == zona || strings.HasSuffix(h, "."+zona)) {
			return true
		}
	}

	// La zona de PUBLIC_URL: su host sin la primera etiqueta, que es la del producto
	// (`web.<zona>`; `pages.<zona>` hasta el 2026-10-04). Se quita LA ETIQUETA y no un
	// nombre concreto: con `TrimPrefix("pages.")`, al cambiar el host a `web.` la zona
	// de un despliegue fuera de sytue.eu dejaba de estar reservada entera y solo se
	// protegía `web.<zona>` — cualquiera habría podido dar de alta `mail.<zona>` como
	// dominio propio de su web.
	if s.cfg.PublicURL != "" {
		if u, err := url.Parse(s.cfg.PublicURL); err == nil {
			if _, zona, ok := strings.Cut(strings.ToLower(u.Hostname()), "."); ok && strings.Contains(zona, ".") &&
				(h == zona || strings.HasSuffix(h, "."+zona)) {
				return true
			}
		}
	}

	return false
}

func (s *Servicio) toDTO(d domain.Domain, canonicalHost *string) domain.DominioDTO {
	isPrimary := canonicalHost != nil && *canonicalHost == d.Host

	apexHost := d.Host
	if d.ParentID != nil {
		apexHost = strings.TrimPrefix(d.Host, "www.")
	} else if tld, _ := publicsuffix.EffectiveTLDPlusOne(d.Host); tld != "" {
		apexHost = tld
	}

	targetCNAME := s.cfg.SitesDomain
	if targetCNAME == "" {
		targetCNAME = "sites.sytue.localhost"
	}
	targetA := s.cfg.IPApex
	if targetA == "" && s.cfg.Env == "dev" {
		targetA = "127.0.0.1"
	}
	registroTipo, registroValor := "CNAME", targetCNAME
	if d.Host == apexHost {
		registroTipo, registroValor = "A", targetA
	}

	return domain.DominioDTO{
		ID:       d.ID,
		Host:     d.Host,
		ParentID: d.ParentID,
		Status:   d.Status,
		Detail:   d.Detail,
		Primary:  isPrimary,
		TXT: domain.TXT{
			Name:  dominio.RecordName(apexHost),
			Value: dominio.RecordValue(d.Token),
		},
		Target: domain.Target{
			CNAME: targetCNAME,
			A:     targetA,
			Type:  registroTipo,
			Name:  d.Host,
			Value: registroValor,
		},
		CheckedAt:   d.CheckedAt,
		VerifiedAt:  d.VerifiedAt,
		ActivatedAt: d.ActivatedAt,
	}
}
