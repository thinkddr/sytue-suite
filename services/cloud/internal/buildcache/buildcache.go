// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package buildcache acuña, por tenant, la credencial del Container Registry ACOTADA a la
// imagen de caché de sus builds.
//
// La caché de capas de una build tiene que vivir en un registry y la escribe el contenedor
// que ejecuta el código del tenant (el RUN de su Dockerfile): ahí no puede ir la
// credencial de push de la plataforma (registry-cred), que alcanza los repositorios de
// TODOS los tenants. Hasta ahora la caché iba en línea dentro de la propia imagen
// (`--export-cache=type=inline`) y nada se reutilizaba de una build a otra sin pedir la
// imagen anterior con la credencial de plataforma.
//
// El alcance en Scaleway: los permission sets de IAM se acotan a un PROYECTO, nunca a un
// espacio de nombres del registry ni a una imagen. Así que cada tenant tiene su proyecto
// de caché (`sytue-cloud-cache-<tenant>`) con un único espacio de nombres privado
// (`sytue-cache-<tenant>`), y una application IAM con una política de
// `ContainerRegistryFullAccess` SOLO sobre ese proyecto: su API key empuja y lee la caché
// del tenant y nada más. Se guarda como Secret `kubernetes.io/dockerconfigjson` en el
// namespace de builds y se monta en el contenedor de build de ese tenant.
//
// Idempotente: proyecto, espacio de nombres, application y política se buscan antes de
// crearse, y la API key solo se acuña (rotando la anterior) si el Secret no existe. Sin
// IAM configurado no hay caché por registry (fail-closed): la build sigue con la caché en
// línea de siempre, nunca con una credencial más ancha.
package buildcache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// PermisoRegistry es el permission set de Scaleway IAM que lleva la política: lectura y
// escritura del registry, acotada por la regla al proyecto de caché del tenant.
const PermisoRegistry = "ContainerRegistryFullAccess"

// usuarioRegistry es el usuario del registry de Scaleway con una API key: la contraseña es
// su secret key.
const usuarioRegistry = "nologin"

// API es lo que la caché necesita de Scaleway: proyectos (Account), espacios de nombres
// del registry (Registry) y applications, políticas y API keys (IAM). Un fake la
// implementa en tests: no hace falta un Scaleway vivo.
type API interface {
	// EnsureProject busca o crea el proyecto llamado name y devuelve su id.
	EnsureProject(ctx context.Context, name string) (projectID string, err error)
	// EnsureRegistryNamespace busca o crea, PRIVADO, el espacio de nombres name del
	// proyecto y devuelve su endpoint (`rg.<región>.scw.cloud/<name>`).
	EnsureRegistryNamespace(ctx context.Context, projectID, name string) (endpoint string, err error)
	// EnsureApplication busca o crea la application IAM llamada name.
	EnsureApplication(ctx context.Context, name string) (appID string, err error)
	// EnsureProjectPolicy busca o crea la política name de la application con UNA regla:
	// el permission set dado, solo sobre el proyecto. Nunca de organización.
	EnsureProjectPolicy(ctx context.Context, name, appID, projectID, permissionSet string) error
	// RotateAPIKey borra las API keys de la application y acuña una nueva.
	RotateAPIKey(ctx context.Context, appID string) (accessKey, secretKey string, err error)
}

// Secretos es dónde se guarda la credencial: el Secret del namespace de builds.
type Secretos interface {
	Existe(ctx context.Context, name string) (bool, error)
	Guardar(ctx context.Context, name, tenant string, dockerConfigJSON []byte) error
}

// Destino es la caché de un tenant: el Secret con su credencial y el repositorio
// (`rg.fr-par.scw.cloud/sytue-cache-acme`) donde van sus imágenes de caché.
type Destino struct {
	Secret      string
	Repositorio string
}

// Imagen es la referencia de la imagen de caché de la app `imagen` (el nombre del
// repositorio de la app, sin registry ni tag) para el motor `tag` (`buildkit`).
func (d Destino) Imagen(imagen, tag string) string {
	return d.Repositorio + "/" + imagen + ":" + tag
}

// Provisioner acuña y guarda las credenciales de caché.
type Provisioner struct {
	api API
	sec Secretos
	// registry es el host del registry (`rg.fr-par.scw.cloud`): la clave del config.json.
	registry string

	mu        sync.Mutex
	porTenant map[string]*sync.Mutex
}

// NewProvisioner construye el provisioner. registryHost es el host del Container Registry.
func NewProvisioner(api API, sec Secretos, registryHost string) *Provisioner {
	return &Provisioner{api: api, sec: sec, registry: registryHost, porTenant: map[string]*sync.Mutex{}}
}

// reTenant es un slug de tenant que vale en los nombres de Scaleway y de Kubernetes.
var reTenant = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// ErrTenant es un tenant que no vale como nombre de proyecto, de espacio de nombres o de
// Secret: no se acuña nada con él.
var ErrTenant = errors.New("buildcache: tenant no válido")

// Nombres son los nombres de las piezas de la caché de un tenant.
func Nombres(tenant string) (proyecto, espacio, secret string) {
	return "sytue-cloud-cache-" + tenant, "sytue-cache-" + tenant, "build-cache-" + tenant
}

// Asegurar deja lista la caché del tenant y dice dónde está. Con el Secret ya guardado
// no acuña ninguna key nueva: rotarla en cada build invalidaría la de una build en curso.
func (p *Provisioner) Asegurar(ctx context.Context, tenant string) (Destino, error) {
	if !reTenant.MatchString(tenant) {
		return Destino{}, ErrTenant
	}
	// Dos builds del mismo tenant a la vez no acuñan dos keys (la segunda borraría la de
	// la primera). Entre réplicas del builder el hueco queda en la primera build de cada
	// tenant: después el Secret ya existe y nadie rota.
	cerrojo := p.cerrojo(tenant)
	cerrojo.Lock()
	defer cerrojo.Unlock()

	proyecto, espacio, secret := Nombres(tenant)
	projectID, err := p.api.EnsureProject(ctx, proyecto)
	if err != nil {
		return Destino{}, fmt.Errorf("buildcache: proyecto de caché: %w", err)
	}
	endpoint, err := p.api.EnsureRegistryNamespace(ctx, projectID, espacio)
	if err != nil {
		return Destino{}, fmt.Errorf("buildcache: espacio de nombres de caché: %w", err)
	}
	destino := Destino{Secret: secret, Repositorio: strings.TrimRight(endpoint, "/")}
	if ok, err := p.sec.Existe(ctx, secret); err != nil {
		return Destino{}, fmt.Errorf("buildcache: leer el Secret: %w", err)
	} else if ok {
		return destino, nil
	}
	appID, err := p.api.EnsureApplication(ctx, proyecto)
	if err != nil {
		return Destino{}, fmt.Errorf("buildcache: application: %w", err)
	}
	if err := p.api.EnsureProjectPolicy(ctx, proyecto, appID, projectID, PermisoRegistry); err != nil {
		return Destino{}, fmt.Errorf("buildcache: política: %w", err)
	}
	_, clave, err := p.api.RotateAPIKey(ctx, appID)
	if err != nil {
		return Destino{}, fmt.Errorf("buildcache: API key: %w", err)
	}
	cfg, err := DockerConfig(p.registry, usuarioRegistry, clave)
	if err != nil {
		return Destino{}, err
	}
	if err := p.sec.Guardar(ctx, secret, tenant, cfg); err != nil {
		return Destino{}, fmt.Errorf("buildcache: guardar el Secret: %w", err)
	}
	return destino, nil
}

func (p *Provisioner) cerrojo(tenant string) *sync.Mutex {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := p.porTenant[tenant]
	if m == nil {
		m = &sync.Mutex{}
		p.porTenant[tenant] = m
	}
	return m
}

// DockerConfig es el `.dockerconfigjson` con la credencial de UN registry.
func DockerConfig(registry, usuario, clave string) ([]byte, error) {
	type auth struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Auth     string `json:"auth"`
	}
	b, err := json.Marshal(struct {
		Auths map[string]auth `json:"auths"`
	}{Auths: map[string]auth{registry: {
		Username: usuario, Password: clave,
		Auth: base64.StdEncoding.EncodeToString([]byte(usuario + ":" + clave)),
	}}})
	if err != nil {
		return nil, fmt.Errorf("buildcache: config.json: %w", err)
	}
	return b, nil
}

// HostDeRegistry es el host de una URL de registry (`rg.fr-par.scw.cloud/sytue` da
// `rg.fr-par.scw.cloud`) y RegionDeRegistry su región de Scaleway (`fr-par`), o "".
func HostDeRegistry(registryURL string) string {
	h, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(registryURL, "https://"), "http://"), "/")
	return h
}

// RegionDeRegistry es la región de Scaleway de un host `rg.<región>.scw.cloud`, o "".
func RegionDeRegistry(registryURL string) string {
	h := HostDeRegistry(registryURL)
	if !strings.HasPrefix(h, "rg.") || !strings.HasSuffix(h, ".scw.cloud") {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(h, "rg."), ".scw.cloud")
}
