// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package builder

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/buildcache"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
)

// cacheFalsa es la caché de builds sin Scaleway: apunta los tenants que le piden.
type cacheFalsa struct {
	pedidos []string
	falla   error
}

func (c *cacheFalsa) Asegurar(_ context.Context, tenant string) (buildcache.Destino, error) {
	c.pedidos = append(c.pedidos, tenant)
	if c.falla != nil {
		return buildcache.Destino{}, c.falla
	}
	return buildcache.Destino{Secret: "build-cache-" + tenant, Repositorio: "rg.fr-par.scw.cloud/sytue-cache-" + tenant}, nil
}

func reqDeCache(buildpacks bool) KanikoRequest {
	return KanikoRequest{
		BuildID: uuid.New(), Tenant: "acme", RepoURL: "https://github.com/a/b", GitRef: "main",
		Dockerfile: "Dockerfile", Destination: "rg.fr-par.scw.cloud/sytue-prod-paas/acme-web:abc123",
		Buildpacks: buildpacks,
	}
}

// TestLaCacheDeRegistryVaConLaCredencialDelTenant: con la caché configurada, una build con
// Dockerfile importa y exporta su caché de capas en la imagen de caché DEL TENANT, y el
// contenedor de build monta SOLO esa credencial (la de plataforma sigue en el push).
func TestLaCacheDeRegistryVaConLaCredencialDelTenant(t *testing.T) {
	cache := &cacheFalsa{}
	k := NewKubeRunner(config.Config{BuildNamespace: "sytue-builds"}, nil, nil).ConCache(cache)
	req := reqDeCache(false)
	spec := k.specFor(req)
	k.conCacheDeRegistry(t.Context(), req, &spec)

	const imagen = "rg.fr-par.scw.cloud/sytue-cache-acme/acme-web:buildkit"
	if spec.CacheSecret != "build-cache-acme" || spec.CacheImage != imagen {
		t.Fatalf("CacheSecret/CacheImage = %q / %q", spec.CacheSecret, spec.CacheImage)
	}
	if !slices.Contains(spec.Args, "--export-cache=type=registry,ref="+imagen+",mode=max") ||
		!slices.Contains(spec.Args, "--import-cache=type=registry,ref="+imagen) ||
		slices.Contains(spec.Args, "--export-cache=type=inline") {
		t.Errorf("args de caché: %v", spec.Args)
	}
	// El resto de la build no cambia: sigue escribiendo un tarball, sin empujar.
	if a := strings.Join(spec.Args, " "); !strings.Contains(a, "dest="+kanikoTarPath) || strings.Contains(a, "type=image") {
		t.Errorf("la build ya no va a un tarball: %v", spec.Args)
	}

	spec.RegistrySecret = "sytue-registry"
	c := newKubeJobClientFor(fake.NewSimpleClientset(), KubeJobConfig{BuildkitImage: "bk:test", CraneImage: "crane:test"})
	job := c.buildJob(spec)
	build := containerByName(job.Spec.Template.Spec.InitContainers, "build")
	push := containerByName(job.Spec.Template.Spec.Containers, "push")
	if build == nil || push == nil {
		t.Fatal("faltan los contenedores build/push")
	}
	if m := mountByName(build.VolumeMounts, "cache-docker-config"); m == nil || m.MountPath != cacheDockerConfigDir || !m.ReadOnly {
		t.Errorf("el build no monta la credencial de caché (ro) en %s: %+v", cacheDockerConfigDir, build.VolumeMounts)
	}
	if !hasEnv(build.Env, "DOCKER_CONFIG", cacheDockerConfigDir) {
		t.Errorf("el build no apunta DOCKER_CONFIG a la credencial de caché: %v", build.Env)
	}
	if mountByName(build.VolumeMounts, "docker-config") != nil {
		t.Error("el build monta la credencial de plataforma")
	}
	if mountByName(push.VolumeMounts, "cache-docker-config") != nil {
		t.Error("el push monta la credencial de caché")
	}
	var vol bool
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == "cache-docker-config" {
			vol = v.Secret != nil && v.Secret.SecretName == "build-cache-acme" &&
				len(v.Secret.Items) == 1 && v.Secret.Items[0].Key == ".dockerconfigjson" && v.Secret.Items[0].Path == "config.json"
		}
	}
	if !vol {
		t.Errorf("el volumen de la credencial de caché no es el Secret del tenant: %+v", job.Spec.Template.Spec.Volumes)
	}
}

// TestSinCacheDeRegistryLaBuildSigueEnLinea: sin provisioner, con un fallo de Scaleway o
// con Buildpacks, la build es la de siempre: caché en línea y ninguna credencial más.
func TestSinCacheDeRegistryLaBuildSigueEnLinea(t *testing.T) {
	cfg := config.Config{BuildNamespace: "sytue-builds"}
	casos := map[string]struct {
		cache      *cacheFalsa
		buildpacks bool
		pide       bool
	}{
		"sin provisioner":       {nil, false, false},
		"fallo de Scaleway":     {&cacheFalsa{falla: errors.New("403")}, false, true},
		"buildpacks (ponytail)": {&cacheFalsa{}, true, false},
	}
	for nombre, cs := range casos {
		k := NewKubeRunner(cfg, nil, nil)
		if cs.cache != nil {
			k.ConCache(cs.cache)
		}
		req := reqDeCache(cs.buildpacks)
		spec := k.specFor(req)
		antes := slices.Clone(spec.Args)
		k.conCacheDeRegistry(t.Context(), req, &spec)
		if spec.CacheSecret != "" || spec.CacheImage != "" || !slices.Equal(spec.Args, antes) {
			t.Errorf("%s: la build cambió: %+v", nombre, spec)
		}
		if cs.cache != nil && (len(cs.cache.pedidos) > 0) != cs.pide {
			t.Errorf("%s: pedidos a la caché = %v", nombre, cs.cache.pedidos)
		}
		job := newKubeJobClientFor(fake.NewSimpleClientset(), KubeJobConfig{BuildkitImage: "bk:test"}).jobFor(spec)
		for _, v := range job.Spec.Template.Spec.Volumes {
			if v.Name == "cache-docker-config" {
				t.Errorf("%s: el Job lleva la credencial de caché", nombre)
			}
		}
	}
}

// TestElProvisionerDeCachePideIAMYRegistryDeScaleway: sin la secret key, la organización
// o un registry de Scaleway no hay caché por registry (fail-closed).
func TestElProvisionerDeCachePideIAMYRegistryDeScaleway(t *testing.T) {
	jobs := newKubeJobClientFor(fake.NewSimpleClientset(), KubeJobConfig{})
	bien := config.Config{SCWSecretKey: "k", SCWOrgID: "o", RegistryURL: "rg.fr-par.scw.cloud/sytue-prod-paas", SCWAPIURL: "https://api.scaleway.com", BuildNamespace: "b"}
	if ProvisionerDeCache(bien, jobs) == nil {
		t.Error("con todo configurado no hay provisioner")
	}
	for nombre, cambia := range map[string]func(*config.Config){
		"sin secret key":     func(c *config.Config) { c.SCWSecretKey = "" },
		"sin organización":   func(c *config.Config) { c.SCWOrgID = "" },
		"registry no de SCW": func(c *config.Config) { c.RegistryURL = "localhost:5000/sytue" },
	} {
		c := bien
		cambia(&c)
		if ProvisionerDeCache(c, jobs) != nil {
			t.Errorf("%s: hay provisioner", nombre)
		}
	}
	if ProvisionerDeCache(bien, nil) != nil {
		t.Error("sin cliente de Kapsule hay provisioner")
	}
}

func TestNombreDeImagen(t *testing.T) {
	for ref, quiero := range map[string]string{
		"rg.fr-par.scw.cloud/sytue-prod-paas/acme-web:abc": "acme-web",
		"rg/x:tag":         "x",
		"localhost:5000/a": "a",
		"reg/app@sha256:0": "app",
		"":                 "app",
	} {
		if got := nombreDeImagen(ref); got != quiero {
			t.Errorf("nombreDeImagen(%q) = %q, quiero %q", ref, got, quiero)
		}
	}
}
