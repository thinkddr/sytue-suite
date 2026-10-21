// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// NASRequest es lo que el cliente declara de su NAS: la dirección LOCAL del
// WebDAV (lo que ve el agente desde dentro de su red), las credenciales y la
// carpeta. El resto —sitio, recurso, nombre público, certificado— lo pone Drive.
type NASRequest struct {
	LocalEndpoint string // http://192.168.1.50:5005 o https://nas.local:5006
	User          string
	Password      string
	BasePath      string
	Label         string
	AcceptTerms   bool
}

// NASStatus es lo que la UI enseña mientras el cliente lanza el agente.
type NASStatus struct {
	Endpoint  string `json:"endpoint"`  // https://nas-<tenant>.<dominio>
	Command   string `json:"command"`   // el docker run para el NAS
	Online    bool   `json:"online"`    // el agente está conectado al proxy
	Reachable bool   `json:"reachable"` // el WebDAV responde a través del túnel
	Error     string `json:"error,omitempty"`
}

// ErrNASTerms se devuelve si no se aceptó el aviso de responsabilidad.
var ErrNASTerms = errors.New("service: hay que aceptar el aviso de responsabilidad")

// ConnectNAS crea el túnel del NAS del cliente y guarda el almacén (apagado hasta
// que el túnel responda). Devuelve el comando que el cliente ejecuta en su NAS.
func (s *Service) ConnectNAS(ctx context.Context, req NASRequest, actor string) (NASStatus, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return NASStatus{}, err
	}
	if s.router == nil || s.router.tunnel == nil {
		return NASStatus{}, ErrTunnelOff
	}
	if !s.router.cipher.Enabled() {
		return NASStatus{}, ErrExternalStorageOff
	}
	if !req.AcceptTerms {
		return NASStatus{}, ErrNASTerms
	}
	esquema, host, puerto, err := direccionLocal(req.LocalEndpoint)
	if err != nil {
		return NASStatus{}, err
	}
	if strings.TrimSpace(req.User) == "" || req.Password == "" {
		return NASStatus{}, fmt.Errorf("%w: usuario y contraseña del NAS", domain.ErrInvalidName)
	}
	// Un túnel por tenant: si ya había uno, se desmonta antes de crear el nuevo.
	if previo, err := s.store.ExternalStorage(ctx); err == nil && previo.Kind == "nas" {
		_ = s.router.tunnel.borrar(ctx, previo.TunnelSiteID, previo.TunnelResourceID)
	}
	subdominio := "nas-" + t.Slug
	siteID, newtID, newtSecret, err := s.router.tunnel.crearSitio(ctx, subdominio)
	if err != nil {
		return NASStatus{}, err
	}
	resourceID, err := s.router.tunnel.crearRecurso(ctx, siteID, subdominio, host, puerto, esquema)
	if err != nil {
		_ = s.router.tunnel.borrar(ctx, siteID, resourceID)
		return NASStatus{}, err
	}
	sellada, err := s.router.cipher.Encrypt(req.Password)
	if err != nil {
		return NASStatus{}, err
	}
	agente, err := s.router.cipher.Encrypt(newtSecret)
	if err != nil {
		return NASStatus{}, err
	}
	now := time.Now()
	publico := "https://" + subdominio + "." + s.router.tunnel.domain
	_, err = s.store.SaveExternalStorage(ctx, store.ExternalStorage{
		Kind: "nas", Endpoint: publico, LocalEndpoint: req.LocalEndpoint, BasePath: strings.Trim(req.BasePath, "/"),
		UseSSL: true, AccessKey: strings.TrimSpace(req.User), SecretKeyEnc: sellada,
		Label: strings.TrimSpace(req.Label), Enabled: false, AcceptedTermsAt: now,
		TunnelSiteID: siteID, TunnelResourceID: resourceID, TunnelNewtID: newtID, TunnelNewtSecretEnc: agente,
		CheckError: "esperando al agente",
	}, actor)
	if err != nil {
		_ = s.router.tunnel.borrar(ctx, siteID, resourceID)
		return NASStatus{}, err
	}
	s.router.Forget(t.Slug)
	return NASStatus{Endpoint: publico, Command: s.router.tunnel.comandoAgente(newtID, newtSecret)}, nil
}

// NASStatus consulta si el agente está conectado y, si lo está, prueba el WebDAV
// a través del túnel; al primer éxito enciende el almacén.
func (s *Service) NASStatus(ctx context.Context) (NASStatus, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return NASStatus{}, err
	}
	if s.router == nil || s.router.tunnel == nil {
		return NASStatus{}, ErrTunnelOff
	}
	e, err := s.store.ExternalStorage(ctx)
	if err != nil {
		return NASStatus{}, err
	}
	if e.Kind != "nas" {
		return NASStatus{}, domain.ErrNotFound
	}
	out := NASStatus{Endpoint: e.Endpoint}
	if secret, derr := s.router.cipher.Decrypt(e.TunnelNewtSecretEnc); derr == nil {
		out.Command = s.router.tunnel.comandoAgente(e.TunnelNewtID, secret)
	}
	out.Online, err = s.router.tunnel.sitioEnLinea(ctx, e.TunnelSiteID)
	if err != nil {
		out.Error = err.Error()
		return out, nil //nolint:nilerr // el estado lleva el error como texto: es información para la pantalla, no un fallo
	}
	if !out.Online {
		return out, nil
	}
	pass, err := s.router.cipher.Decrypt(e.SecretKeyEnc)
	if err != nil {
		out.Error = err.Error()
		return out, nil //nolint:nilerr // ídem: se enseña, no se aborta
	}
	if msg := s.ProbeExternal(ctx, e, pass); msg != "" {
		out.Error = msg
		_ = s.store.MarkExternalStorageChecked(ctx, false, msg)
		return out, nil
	}
	out.Reachable = true
	if err := s.store.MarkExternalStorageChecked(ctx, true, ""); err != nil {
		out.Error = err.Error()
	}
	s.router.Forget(t.Slug)
	return out, nil
}

// TeardownNAS desmonta el túnel de un NAS (antes de desconectar el almacén).
// Best-effort: si el proxy no responde, el almacén se desconecta igual y el
// recurso huérfano se limpia a mano.
func (s *Service) TeardownNAS(ctx context.Context, e store.ExternalStorage) error {
	if e.Kind != "nas" || s.router == nil || s.router.tunnel == nil {
		return nil
	}
	return s.router.tunnel.borrar(ctx, e.TunnelSiteID, e.TunnelResourceID)
}

// direccionLocal descompone la dirección local del NAS en esquema, host y
// puerto. Solo direcciones privadas o nombres locales: el túnel es para lo que
// está detrás de la red del cliente, no para publicar terceros.
func direccionLocal(raw string) (esquema, host string, puerto int, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", "", 0, fmt.Errorf("%w: la dirección local del NAS tiene que ser http(s)://host:puerto", domain.ErrInvalidName)
	}
	puerto = 80
	if u.Scheme == "https" {
		puerto = 443
	}
	if p := u.Port(); p != "" {
		if puerto, err = strconv.Atoi(p); err != nil || puerto < 1 || puerto > 65535 {
			return "", "", 0, fmt.Errorf("%w: puerto", domain.ErrInvalidName)
		}
	}
	host = u.Hostname()
	if !esDireccionDeRedLocal(host) {
		return "", "", 0, fmt.Errorf("%w: la dirección del NAS tiene que ser de tu red (192.168.x.x, 10.x.x.x, 172.16-31.x.x, o un nombre como nas o nas.local)", domain.ErrInvalidName)
	}
	return u.Scheme, host, puerto, nil
}

// esDireccionDeRedLocal acota lo que el túnel puede publicar: una IP privada o
// un nombre que solo existe dentro de la red del cliente. Sin esto, alguien
// podría apuntar su túnel a un sitio cualquiera de Internet y servirlo bajo
// nas-<tenant>.<dominio> con NUESTRO certificado: un proxy abierto con la
// marca de Sytue delante.
func esDireccionDeRedLocal(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "" {
		return false
	}
	if !strings.Contains(h, ".") {
		return true // «nas», «diskstation»: nombres de red local
	}
	for _, suf := range []string{".local", ".lan", ".internal", ".home.arpa", ".intranet"} {
		if strings.HasSuffix(h, suf) {
			return true
		}
	}
	return false
}
