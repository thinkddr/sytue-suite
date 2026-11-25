// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package buildcache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// fakeAPI es la API de Scaleway en memoria: apunta lo que se le pide.
type fakeAPI struct {
	mu        sync.Mutex
	proyectos map[string]string
	politicas []politica
	rotadas   int
	falla     error
}

type politica struct{ nombre, app, proyecto, permiso string }

func nuevoFakeAPI() *fakeAPI { return &fakeAPI{proyectos: map[string]string{}} }

func (f *fakeAPI) EnsureProject(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.falla != nil {
		return "", f.falla
	}
	if f.proyectos[name] == "" {
		f.proyectos[name] = "proj-" + name
	}
	return f.proyectos[name], nil
}

func (f *fakeAPI) EnsureRegistryNamespace(_ context.Context, projectID, name string) (string, error) {
	return "rg.fr-par.scw.cloud/" + name, nil
}

func (f *fakeAPI) EnsureApplication(_ context.Context, name string) (string, error) {
	return "app-" + name, nil
}

func (f *fakeAPI) EnsureProjectPolicy(_ context.Context, name, appID, projectID, permissionSet string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.politicas = append(f.politicas, politica{name, appID, projectID, permissionSet})
	return nil
}

func (f *fakeAPI) RotateAPIKey(_ context.Context, appID string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rotadas++
	return "SCWAK", "secreto-" + appID, nil
}

// fakeSecretos guarda en memoria.
type fakeSecretos struct {
	mu    sync.Mutex
	datos map[string][]byte
}

func (s *fakeSecretos) Existe(_ context.Context, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.datos[name]
	return ok, nil
}

func (s *fakeSecretos) Guardar(_ context.Context, name, _ string, cfg []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.datos[name] = cfg
	return nil
}

// TestLaCredencialDeCacheVaAcotadaAlProyectoDelTenant: la primera build de un tenant crea
// su proyecto de caché, su espacio de nombres y una application con UNA política de
// registry SOLO sobre ese proyecto, y guarda la key como config.json del registry; las
// siguientes no acuñan otra (rotarla invalidaría la de una build en curso).
func TestLaCredencialDeCacheVaAcotadaAlProyectoDelTenant(t *testing.T) {
	api, sec := nuevoFakeAPI(), &fakeSecretos{datos: map[string][]byte{}}
	p := NewProvisioner(api, sec, "rg.fr-par.scw.cloud")
	d, err := p.Asegurar(t.Context(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if d.Secret != "build-cache-acme" || d.Repositorio != "rg.fr-par.scw.cloud/sytue-cache-acme" {
		t.Errorf("destino = %+v", d)
	}
	if got := d.Imagen("acme-web", "buildkit"); got != "rg.fr-par.scw.cloud/sytue-cache-acme/acme-web:buildkit" {
		t.Errorf("imagen de caché = %s", got)
	}
	quiero := politica{"sytue-cloud-cache-acme", "app-sytue-cloud-cache-acme", "proj-sytue-cloud-cache-acme", PermisoRegistry}
	if len(api.politicas) != 1 || api.politicas[0] != quiero {
		t.Errorf("políticas = %+v, quiero solo %+v", api.politicas, quiero)
	}
	var cfg struct {
		Auths map[string]struct {
			Username, Password, Auth string
		} `json:"auths"`
	}
	if err := json.Unmarshal(sec.datos["build-cache-acme"], &cfg); err != nil {
		t.Fatal(err)
	}
	a, ok := cfg.Auths["rg.fr-par.scw.cloud"]
	if !ok || len(cfg.Auths) != 1 || a.Username != "nologin" || a.Password != "secreto-app-sytue-cloud-cache-acme" ||
		a.Auth != base64.StdEncoding.EncodeToString([]byte("nologin:secreto-app-sytue-cloud-cache-acme")) {
		t.Errorf("config.json = %+v", cfg)
	}

	if _, err := p.Asegurar(t.Context(), "acme"); err != nil {
		t.Fatal(err)
	}
	if api.rotadas != 1 {
		t.Errorf("con el Secret ya guardado se ha rotado la key: %d rotaciones", api.rotadas)
	}
	// Otro tenant: su proyecto, su política, su Secret.
	if _, err := p.Asegurar(t.Context(), "globex"); err != nil {
		t.Fatal(err)
	}
	if len(api.politicas) != 2 || api.politicas[1].proyecto != "proj-sytue-cloud-cache-globex" {
		t.Errorf("la política del segundo tenant no va a su proyecto: %+v", api.politicas)
	}
}

// TestDosBuildsALaVezAcunanUnaSolaKey: la segunda borraría la key de la primera.
func TestDosBuildsALaVezAcunanUnaSolaKey(t *testing.T) {
	api, sec := nuevoFakeAPI(), &fakeSecretos{datos: map[string][]byte{}}
	p := NewProvisioner(api, sec, "rg.fr-par.scw.cloud")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := p.Asegurar(context.Background(), "acme"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if api.rotadas != 1 {
		t.Errorf("%d rotaciones a la vez; quiero 1", api.rotadas)
	}
}

// TestSinTenantValidoNiScalewayNoSeAcunaNada: un tenant que no vale como nombre no llega a
// Scaleway, y un fallo de Scaleway no deja Secret a medias.
func TestSinTenantValidoNiScalewayNoSeAcunaNada(t *testing.T) {
	api, sec := nuevoFakeAPI(), &fakeSecretos{datos: map[string][]byte{}}
	p := NewProvisioner(api, sec, "rg.fr-par.scw.cloud")
	for _, malo := range []string{"", "Acme", "acme/../x", "-acme", "a b", "acme_"} {
		if _, err := p.Asegurar(t.Context(), malo); !errors.Is(err, ErrTenant) {
			t.Errorf("tenant %q: err = %v", malo, err)
		}
	}
	if len(api.proyectos) != 0 {
		t.Errorf("un tenant no válido llegó a Scaleway: %v", api.proyectos)
	}
	api.falla = errors.New("503")
	if _, err := p.Asegurar(t.Context(), "acme"); err == nil {
		t.Error("un fallo de Scaleway no se dice")
	}
	if len(sec.datos) != 0 || api.rotadas != 0 {
		t.Error("un fallo de Scaleway deja Secret o key")
	}
}

func TestRegionYHostDelRegistry(t *testing.T) {
	casos := map[string][2]string{
		"rg.fr-par.scw.cloud/sytue-prod-paas": {"rg.fr-par.scw.cloud", "fr-par"},
		"https://rg.nl-ams.scw.cloud/x":       {"rg.nl-ams.scw.cloud", "nl-ams"},
		"localhost:5000/sytue":                {"localhost:5000", ""},
		"ghcr.io/thinkddr":                    {"ghcr.io", ""},
	}
	for url, quiero := range casos {
		if h, r := HostDeRegistry(url), RegionDeRegistry(url); h != quiero[0] || r != quiero[1] {
			t.Errorf("%s: host %q región %q, quiero %v", url, h, r, quiero)
		}
	}
}

// TestElSecretEsDelRegistryYDelTenant: Secret dockerconfigjson con la etiqueta del tenant;
// si otra réplica lo guardó a la vez, se sustituye (vale la última key acuñada).
func TestElSecretEsDelRegistryYDelTenant(t *testing.T) {
	cs := fake.NewSimpleClientset()
	s := NewSecretosKube(cs, "sytue-paas-builds")
	if ok, err := s.Existe(t.Context(), "build-cache-acme"); ok || err != nil {
		t.Fatalf("Existe antes de guardar = %v, %v", ok, err)
	}
	for _, clave := range []string{"uno", "dos"} {
		if err := s.Guardar(t.Context(), "build-cache-acme", "acme", []byte(clave)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := cs.CoreV1().Secrets("sytue-paas-builds").Get(t.Context(), "build-cache-acme", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != corev1.SecretTypeDockerConfigJson || string(got.Data[corev1.DockerConfigJsonKey]) != "dos" || got.Labels["sytue.eu/tenant"] != "acme" {
		t.Errorf("Secret = %+v", got)
	}
	if ok, err := s.Existe(t.Context(), "build-cache-acme"); !ok || err != nil {
		t.Errorf("Existe tras guardar = %v, %v", ok, err)
	}
}
