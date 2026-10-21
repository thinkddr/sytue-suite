// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// appDePrueba es una entrada equivalente a las del catálogo, pero con la imagen
// ya «replicada»: el catálogo real está entero pendiente de espejo, y lo que
// aquí se prueba es cómo se materializa, no si hay espejo.
const registroPrueba = "rg.fr-par.scw.cloud/sytue-prod-paas"

func appDePrueba() domain.CatalogApp {
	return domain.CatalogApp{
		ID: "meilisearch", Name: "Meilisearch", Description: "Buscador.",
		Category: domain.AppCategorySearch, License: "MIT",
		Homepage:       "https://www.meilisearch.com/",
		Upstream:       "docker.io/getmeili/meilisearch:v1.52.0",
		UpstreamDigest: "sha256:" + strings.Repeat("a", 64),
		MirroredDigest: "sha256:" + strings.Repeat("a", 64),
		Version:        "1.13",
		Ports:          []domain.AppPort{{Name: "http", Port: 7700}},
		Exports:        []domain.AppEnvExport{{Key: "MEILI_URL", Value: "http://{host}:{port}"}},
		Env: []domain.AppEnvVar{
			{Key: "MEILI_MASTER_KEY", Generate: true, Export: true},
			{Key: "MEILI_ENV", Value: "production"},
		},
		DiskGiB: 10, DataPath: "/meili_data",
		MemoryMiB: 1024, CPUMilli: 1000, RunAsUser: 1000,
		HealthPath: "/health",
	}
}

func servicioApp() domain.ManagedService {
	return domain.ManagedService{
		ID: uuid.New(), Tenant: "acme", ProjectID: uuid.New(),
		Kind: domain.ServiceApp, Name: "buscador",
		Config: map[string]string{"app_id": "meilisearch"},
	}
}

// TestElPodNoPuedeSalIRSEDeRestricted: el namespace del cliente exige
// PodSecurity `restricted`. Un pod que pida cualquiera de estas cosas ni siquiera
// se admite, y descubrirlo en el primer alta de un cliente es tarde.
func TestElPodNoPuedeSalIRSEDeRestricted(t *testing.T) {
	dep := appDeployment("svc-abc", "tenant-acme", registroPrueba, map[string]string{}, appDePrueba())
	spec := dep.Spec.Template.Spec
	c := spec.Containers[0]

	if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
		t.Error("falta runAsNonRoot")
	}
	if c.SecurityContext.RunAsUser == nil || *c.SecurityContext.RunAsUser != 1000 {
		t.Errorf("runAsUser = %v; se esperaba el UID del catálogo", c.SecurityContext.RunAsUser)
	}
	if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Error("allowPrivilegeEscalation debería ser false")
	}
	if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("la raíz debería ser de solo lectura")
	}
	if len(c.SecurityContext.Capabilities.Drop) != 1 || c.SecurityContext.Capabilities.Drop[0] != "ALL" {
		t.Errorf("capacidades = %v; se esperaba drop ALL", c.SecurityContext.Capabilities.Drop)
	}
	if c.SecurityContext.SeccompProfile == nil ||
		c.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("falta el perfil seccomp por defecto")
	}
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Error("no debería montarse el token de la ServiceAccount: la app no habla con la API de Kubernetes")
	}
	if spec.HostNetwork || spec.HostPID || spec.HostIPC {
		t.Error("ningún espacio de nombres del host")
	}
	for _, v := range spec.Volumes {
		if v.HostPath != nil {
			t.Errorf("volumen hostPath en %q", v.Name)
		}
	}
}

// TestLosDirectoriosDeclaradosSonESCRIBIBLES: con la raíz de solo lectura, el
// arranque de RabbitMQ —que genera su configuración en /etc/rabbitmq— moriría.
// Se montan como emptyDir para no renunciar a readOnlyRootFilesystem por un
// directorio.
func TestLosDirectoriosDeclaradosSonESCRIBIBLES(t *testing.T) {
	a := appDePrueba()
	a.WritablePaths = []string{"/etc/rabbitmq", "/var/log/app"}
	dep := appDeployment("svc-abc", "tenant-acme", registroPrueba, nil, a)
	montados := map[string]bool{}
	for _, m := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
		montados[m.MountPath] = true
	}
	for _, w := range a.WritablePaths {
		if !montados[w] {
			t.Errorf("%s no se montó como escribible", w)
		}
	}
	// Y son EFÍMEROS: nada persiste ahí sin que alguien lo haya declarado disco.
	efimeros := 0
	for _, v := range dep.Spec.Template.Spec.Volumes {
		if v.EmptyDir != nil {
			efimeros++
		}
	}
	if efimeros != 3 { // /tmp + los dos declarados
		t.Errorf("volúmenes efímeros = %d; se esperaban 3", efimeros)
	}
}

// TestConLaRaizDeSoloLecturaHaceFaltaUnTmp: media imagen no arranca sin un sitio
// donde escribir temporales, y renunciar a readOnlyRootFilesystem por eso sería
// pagar demasiado.
func TestConLaRaizDeSoloLecturaHaceFaltaUnTmp(t *testing.T) {
	dep := appDeployment("svc-abc", "tenant-acme", registroPrueba, nil, appDePrueba())
	var tmp bool
	for _, m := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
		if m.MountPath == "/tmp" {
			tmp = true
		}
	}
	if !tmp {
		t.Error("falta /tmp escribible")
	}
}

// TestElDiscoSeMontaDONDEDiceElCatalogo: montarlo donde no es deja al servicio
// arrancando en vacío cada vez, sin que nada falle.
func TestElDiscoSeMontaDONDEDiceElCatalogo(t *testing.T) {
	dep := appDeployment("svc-abc", "tenant-acme", registroPrueba, nil, appDePrueba())
	var montado string
	for _, m := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
		if m.Name == "data" {
			montado = m.MountPath
		}
	}
	if montado != "/meili_data" {
		t.Errorf("disco montado en %q; el catálogo dice /meili_data", montado)
	}
	// Y el volumen tiene que ser escribible por el UID que corre.
	if fsg := dep.Spec.Template.Spec.SecurityContext.FSGroup; fsg == nil || *fsg != 1000 {
		t.Errorf("fsGroup = %v; sin él el volumen nace de root y el servicio muere al escribir", fsg)
	}
}

// TestUnaAppSinDiscoNoPideVolumen: pedir un PVC que nadie monta es cobrarle al
// cliente un disco que no usa.
func TestUnaAppSinDiscoNoPideVolumen(t *testing.T) {
	a := appDePrueba()
	a.DiskGiB, a.DataPath = 0, ""
	dep := appDeployment("svc-abc", "tenant-acme", registroPrueba, nil, a)
	for _, v := range dep.Spec.Template.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			t.Errorf("pidió un PVC sin tener disco: %v", v.Name)
		}
	}
}

// TestLaEstrategiaEsRecreate: con un disco ReadWriteOnce, un rollout progresivo
// deja el pod nuevo esperando un volumen que todavía tiene el viejo.
func TestLaEstrategiaEsRecreate(t *testing.T) {
	dep := appDeployment("svc-abc", "tenant-acme", registroPrueba, nil, appDePrueba())
	if dep.Spec.Strategy.Type != "Recreate" {
		t.Errorf("estrategia = %q", dep.Spec.Strategy.Type)
	}
}

// TestLosLimitesSalenDelCatalogoYLasRequestsSonMenores: el catálogo declara el
// techo, y reservarlo entero le quitaría a la cuota cuatro veces lo que se usa.
func TestLosLimitesSalenDelCatalogoYLasRequestsSonMenores(t *testing.T) {
	dep := appDeployment("svc-abc", "tenant-acme", registroPrueba, nil, appDePrueba())
	r := dep.Spec.Template.Spec.Containers[0].Resources
	if got := r.Limits.Memory().Value(); got != 1024*1024*1024 {
		t.Errorf("límite de memoria = %d; se esperaban 1024 MiB", got)
	}
	if got := r.Limits.Cpu().MilliValue(); got != 1000 {
		t.Errorf("límite de cpu = %dm", got)
	}
	// Reservar el techo entero multiplicaría por cuatro lo que el proyecto le
	// quita a la cuota sin llegar a usarlo.
	if r.Requests.Memory().Value() >= r.Limits.Memory().Value() {
		t.Error("la request de memoria no puede igualar al límite")
	}
	if r.Requests.Cpu().MilliValue() >= r.Limits.Cpu().MilliValue() {
		t.Error("la request de cpu no puede igualar al límite")
	}
}

// TestSiempreHaySonda: sin ella el Service manda tráfico a un pod que aún
// arranca, y la app del cliente ve conexiones rechazadas que parecen fallo suyo.
func TestSiempreHaySonda(t *testing.T) {
	dep := appDeployment("svc-abc", "tenant-acme", registroPrueba, nil, appDePrueba())
	probe := dep.Spec.Template.Spec.Containers[0].ReadinessProbe
	if probe == nil || probe.HTTPGet == nil || probe.HTTPGet.Path != "/health" {
		t.Fatalf("con health_path la sonda debe ser HTTP: %+v", probe)
	}
	a := appDePrueba()
	a.HealthPath = ""
	probe = appDeployment("svc-abc", "tenant-acme", registroPrueba, nil, a).Spec.Template.Spec.Containers[0].ReadinessProbe
	if probe == nil || probe.TCPSocket == nil {
		t.Errorf("sin health_path la sonda debe ser TCP: %+v", probe)
	}
}

// --- Aprovisionamiento completo --------------------------------------------

func provisionerConCatalogo(t *testing.T) *KubeProvisioner {
	t.Helper()
	cs := fake.NewSimpleClientset()
	return newKubeProvisionerFor(cs, config.Config{RegistryURL: "rg.fr-par.scw.cloud/sytue"}, nil, nil)
}

// TestNoSeAprovisionaUnaAppQueNoEstaEnElCatalogo: un id que no existe no puede
// acabar en un Deployment con la imagen vacía.
func TestNoSeAprovisionaUnaAppQueNoEstaEnElCatalogo(t *testing.T) {
	p := provisionerConCatalogo(t)
	svc := servicioApp()
	svc.Config["app_id"] = "no-existe"
	if _, err := p.provisionApp(context.Background(), svc); err == nil {
		t.Fatal("se aprovisionó una app inexistente")
	}
}

// TestUnaAppPendienteDeEspejoFallaCONMOTIVO: aprovisionar algo que no se puede
// descargar deja el servicio en «provisioning» para siempre, después de
// habérselo prometido al cliente. El error tiene que decir por qué.
//
// El catálogo va FABRICADO y no leído del repo: el digest del espejo lo anota el CI
// según va replicando imágenes, así que la versión anterior de este test —que usaba
// meilisearch «porque hoy está pendiente»— se puso roja sola en cuanto el CI la replicó.
func TestUnaAppPendienteDeEspejoFallaCONMOTIVO(t *testing.T) {
	cat := domain.AppCatalog{PendingMirror: []string{"buscador-nuevo"}}
	_, err := appFromCatalog(cat, registroPrueba, "buscador-nuevo")
	if err == nil {
		t.Fatal("se aprovisionó una app cuya imagen no está replicada")
	}
	if !strings.Contains(err.Error(), "replicado") {
		t.Errorf("el error debería explicar que falta el espejo: %v", err)
	}
	if !strings.Contains(err.Error(), registroPrueba) {
		t.Errorf("el error debería decir en qué registro falta: %v", err)
	}
}

// TestElSecretoGeneradoNoCambiaAlREAPROVISIONAR es el fallo silencioso que este
// test existe para impedir: el reconciliador repite el aprovisionamiento, y
// regenerar la clave dejaría al servicio y a la app con credenciales distintas
// sin que nada fallara.
func TestElSecretoGeneradoNoCambiaAlREAPROVISIONAR(t *testing.T) {
	cs := fake.NewSimpleClientset()
	p := newKubeProvisionerFor(cs, config.Config{RegistryURL: "rg"}, nil, nil)
	ctx := context.Background()
	app := appDePrueba()

	// Sin marcadores que resolver: esta app no declara ninguno, y el mapa vacío es
	// lo que recibe el camino normal cuando la app no se publica ni pide base.
	primera, err := p.resolveAppEnv(ctx, "tenant-acme", "svc-abc-app-env", app, nil)
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if primera["MEILI_MASTER_KEY"] == "" {
		t.Fatal("no se generó la clave maestra")
	}
	if primera["MEILI_ENV"] != "production" {
		t.Errorf("el valor literal se perdió: %q", primera["MEILI_ENV"])
	}

	// Se persiste como lo haría provisionApp…
	_, err = cs.CoreV1().Secrets("tenant-acme").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "svc-abc-app-env", Namespace: "tenant-acme"},
		StringData: primera,
		Data:       map[string][]byte{"MEILI_MASTER_KEY": []byte(primera["MEILI_MASTER_KEY"])},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("crear secreto: %v", err)
	}

	segunda, err := p.resolveAppEnv(ctx, "tenant-acme", "svc-abc-app-env", app, nil)
	if err != nil {
		t.Fatalf("resolver otra vez: %v", err)
	}
	if segunda["MEILI_MASTER_KEY"] != primera["MEILI_MASTER_KEY"] {
		t.Error("la clave maestra cambió al reaprovisionar: el servicio y la app quedarían descolgados")
	}
}

// TestLoQueRecibeLaAppDelCliente: URLs del catálogo más los secretos marcados
// como exportables, y nada más.
func TestLoQueRecibeLaAppDelCliente(t *testing.T) {
	app := appDePrueba()
	creds := domain.RenderExports(app, "svc-abc-app.tenant-acme.svc.cluster.local")
	creds["MEILI_MASTER_KEY"] = "clave"
	raw, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("serializar: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("deserializar: %v", err)
	}
	if got["MEILI_URL"] != "http://svc-abc-app.tenant-acme.svc.cluster.local:7700" {
		t.Errorf("MEILI_URL = %q", got["MEILI_URL"])
	}
	if got["MEILI_ENV"] != "" {
		t.Error("una variable NO exportable no debe llegar a la app del cliente")
	}
}

// TestElServicioNoSeExponeFueraDelNamespace: publicar un panel de administración
// con la contraseña recién generada sería el peor regalo posible.
func TestElServicioNoSeExponeFueraDelNamespace(t *testing.T) {
	s := appService("svc-abc", "tenant-acme", nil, appDePrueba())
	if s.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("tipo = %q; solo ClusterIP", s.Spec.Type)
	}
	if len(s.Spec.Ports) != 1 || s.Spec.Ports[0].Port != 7700 {
		t.Errorf("puertos = %+v", s.Spec.Ports)
	}
}

// --- Integración entre apps del catálogo -----------------------------------

// catalogoConIntegracion fabrica un catálogo de dos entradas donde el CMS declara
// que sabe usar la caché. Va fabricado y no leído del repo por la misma razón que
// el de arriba: el catálogo real cambia con cada app que se añade.
func catalogoConIntegracion() domain.AppCatalog {
	cache := appDePrueba()
	cache.ID = "cache"
	cache.Ports = []domain.AppPort{{Name: "redis", Port: 6379}}

	cms := appDePrueba()
	cms.ID = "cms"
	cms.Integrations = []domain.AppIntegration{{
		App:  "cache",
		Docs: "https://ejemplo.org/docs/cache",
		Env: []domain.AppEnvExport{
			{Key: "CACHE_ADAPTER", Value: "redis"},
			{Key: "CACHE_URL", Value: "redis://{host}:{port}"},
		},
	}}
	return domain.AppCatalog{Apps: []domain.CatalogApp{cache, cms}}
}

// servicioDeCache registra en el fake el Service que representa a la caché ya
// levantada, con las dos etiquetas por las que se la encuentra.
func servicioDeCache(t *testing.T, p *KubeProvisioner, ns, nombre, projectID string) {
	t.Helper()
	_, err := p.cs.CoreV1().Services(ns).Create(context.Background(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nombre,
			Namespace: ns,
			Labels:    map[string]string{appIDLabel: "cache", projectLabel: projectID},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("crear Service de la caché: %v", err)
	}
}

// TestSeConectanSolasAunqueSeCreenDeUnaEnUna es la promesa del producto: elegir
// dos productos y que arranquen conectados, sin copiar una URL de un panel a otro.
func TestSeConectanSolasAunqueSeCreenDeUnaEnUna(t *testing.T) {
	ctx := context.Background()
	cat := catalogoConIntegracion()
	cms := cat.Apps[1]
	svc := servicioApp()
	svc.Config["app_id"] = "cms"
	ns := "tenant-" + svc.Tenant

	// 1. La caché todavía no está: la integración no aporta nada. Inyectar la URL
	// de algo que no existe dejaría al CMS arrancando contra un host que no resuelve.
	p := provisionerConCatalogo(t)
	sola, err := p.integrationEnv(ctx, ns, svc, cms, cat)
	if err != nil {
		t.Fatalf("integrationEnv sin la hermana: %v", err)
	}
	if len(sola) != 0 {
		t.Errorf("sin la caché levantada no debería inyectarse nada: %v", sola)
	}

	// 2. Con la caché levantada, las variables salen resueltas contra SU servicio.
	servicioDeCache(t, p, ns, "svc-cache-app", svc.ProjectID.String())
	env, err := p.integrationEnv(ctx, ns, svc, cms, cat)
	if err != nil {
		t.Fatalf("integrationEnv con la hermana: %v", err)
	}
	if env["CACHE_ADAPTER"] != "redis" {
		t.Errorf("CACHE_ADAPTER = %q", env["CACHE_ADAPTER"])
	}
	quiero := "redis://svc-cache-app." + ns + ".svc.cluster.local:6379"
	if env["CACHE_URL"] != quiero {
		t.Errorf("CACHE_URL = %q, quiero %q", env["CACHE_URL"], quiero)
	}
}

// TestNoSeConectaConLaCacheDeOtroProyecto. El namespace es del TENANT y dentro
// caben varios proyectos: sin el filtro por proyecto, la app de uno se conectaría
// a la caché de otro —mismos datos, distinto proyecto— y ninguna de las dos
// partes se enteraría.
func TestNoSeConectaConLaCacheDeOtroProyecto(t *testing.T) {
	ctx := context.Background()
	cat := catalogoConIntegracion()
	svc := servicioApp()
	svc.Config["app_id"] = "cms"
	ns := "tenant-" + svc.Tenant

	p := provisionerConCatalogo(t)
	servicioDeCache(t, p, ns, "svc-cache-ajena", uuid.New().String())
	env, err := p.integrationEnv(ctx, ns, svc, cat.Apps[1], cat)
	if err != nil {
		t.Fatalf("integrationEnv: %v", err)
	}
	if len(env) != 0 {
		t.Errorf("se conectó a la caché de otro proyecto: %v", env)
	}
}

// TestLaAppPublicadaTieneIngressYLaOtraNo: un CMS sin URL no sirve de nada, y un
// panel de administración con URL es una brecha. Lo decide el catálogo.
func TestLaAppPublicadaTieneIngressYLaOtraNo(t *testing.T) {
	ctx := context.Background()
	p := provisionerConCatalogo(t)
	app := appDePrueba()
	app.WebPort = 7700
	labels := map[string]string{publishedLabel: "true"}

	if err := p.applyAppIngress(ctx, "tenant-acme", "svc-abc", "cms-web.apps.sytue.eu", app, labels); err != nil {
		t.Fatalf("applyAppIngress: %v", err)
	}
	ing, err := p.cs.NetworkingV1().Ingresses("tenant-acme").Get(ctx, "svc-abc-app", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("el Ingress no se creó: %v", err)
	}
	if ing.Spec.Rules[0].Host != "cms-web.apps.sytue.eu" {
		t.Errorf("host = %q", ing.Spec.Rules[0].Host)
	}
	if got := ing.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number; got != 7700 {
		t.Errorf("puerto publicado = %d, quiero el web_port del catálogo", got)
	}
	if len(ing.Spec.TLS) != 1 || ing.Spec.TLS[0].Hosts[0] != "cms-web.apps.sytue.eu" {
		t.Errorf("sin TLS para el host: %+v", ing.Spec.TLS)
	}
	// Y la parte que evita medir el HTML de otro: la ruta de Analytics no se cuela
	// en el Ingress de una app ajena.
	for _, ruta := range ing.Spec.Rules[0].HTTP.Paths {
		if strings.HasPrefix(ruta.Path, "/_sytue") {
			t.Error("el Ingress de una app del catálogo no debe enrutar la analítica")
		}
	}
}
