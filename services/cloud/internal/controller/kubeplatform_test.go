// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func testCfg() config.Config {
	c := config.Config{}
	c.CertIssuer = "sytue-letsencrypt"
	c.AppsDomain = "apps.sytue.eu"
	c.S3Endpoint = "s3.fr-par.scw.cloud"
	c.S3Region = "fr-par"
	c.OIDCIssuer = "https://id.sytue.eu"
	c.ClusterCIDRs = []string{"100.64.0.0/14", "10.32.0.0/12"}
	return c
}

func newTestPlatform() (*KubePlatform, *fake.Clientset) {
	cs := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		certManagerGVR: "CertificateList",
		// El Middleware de Traefik cierra las previews con contraseña: sin
		// registrarlo aquí, el cliente falso no sabe resolver el GVR y RetireApp
		// —que lo borra siempre— se cae en los tests.
		traefikMiddlewareGVR: "MiddlewareList",
	})
	p := newKubePlatformFor(cs, dyn, testCfg())
	return p, cs
}

func TestEnsureNamespaceCreatesGuards(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()

	if err := p.EnsureNamespace(ctx, "tenant-acme", "acme", domain.QuotasFor(domain.PlanFree)); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}
	ns, err := cs.CoreV1().Namespaces().Get(ctx, "tenant-acme", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("namespace no creado: %v", err)
	}
	if ns.Labels["pod-security.kubernetes.io/enforce"] != "restricted" {
		t.Errorf("PodSecurity restricted no aplicado")
	}
	if _, err := cs.CoreV1().ResourceQuotas("tenant-acme").Get(ctx, "tenant-quota", metav1.GetOptions{}); err != nil {
		t.Errorf("ResourceQuota no creada: %v", err)
	}
	deny, err := cs.NetworkingV1().NetworkPolicies("tenant-acme").Get(ctx, "default-deny-all", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("NetworkPolicy deny no creada: %v", err)
	}
	if len(deny.Spec.PolicyTypes) != 2 {
		t.Errorf("deny debe cubrir Ingress y Egress")
	}
	allow, err := cs.NetworkingV1().NetworkPolicies("tenant-acme").Get(ctx, "allow-ingress-and-dns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("NetworkPolicy allow no creada: %v", err)
	}

	// Egress a Internet: 0.0.0.0/0 exceptuando RFC1918, link-local/metadata y
	// los CIDR de pods/services del clúster (configurados en testCfg).
	inet := internetIPBlock(allow)
	if inet == nil {
		t.Fatalf("falta la regla de egress a Internet (0.0.0.0/0)")
	}
	for _, want := range []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16",
		"100.64.0.0/14", "10.32.0.0/12",
	} {
		if !slices.Contains(inet.Except, want) {
			t.Errorf("el egress a Internet debe exceptuar %s; except=%v", want, inet.Except)
		}
	}
	// El egress DNS (53) se conserva.
	hasDNS := false
	for _, rule := range allow.Spec.Egress {
		for _, port := range rule.Ports {
			if port.Port != nil && port.Port.IntValue() == 53 {
				hasDNS = true
			}
		}
	}
	if !hasDNS {
		t.Errorf("el egress DNS (puerto 53) debe conservarse")
	}

	// LimitRange por contenedor: defaults y máximos.
	lr, err := cs.CoreV1().LimitRanges("tenant-acme").Get(ctx, "tenant-limits", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("LimitRange no creado: %v", err)
	}
	if len(lr.Spec.Limits) != 1 || lr.Spec.Limits[0].Type != corev1.LimitTypeContainer {
		t.Fatalf("LimitRange debe tener un item de tipo Container: %+v", lr.Spec.Limits)
	}
	item := lr.Spec.Limits[0]
	for name, want := range map[string]struct {
		got  resource.Quantity
		want string
	}{
		"defaultRequest.cpu": {item.DefaultRequest[corev1.ResourceCPU], "100m"},
		"defaultRequest.mem": {item.DefaultRequest[corev1.ResourceMemory], "128Mi"},
		"default.cpu":        {item.Default[corev1.ResourceCPU], "1"},
		"default.mem":        {item.Default[corev1.ResourceMemory], "512Mi"},
		"max.cpu":            {item.Max[corev1.ResourceCPU], "2"},
		"max.mem":            {item.Max[corev1.ResourceMemory], "2Gi"},
	} {
		if want.got.Cmp(resource.MustParse(want.want)) != 0 {
			t.Errorf("LimitRange %s=%s, se esperaba %s", name, want.got.String(), want.want)
		}
	}

	// Idempotencia: segunda llamada no falla.
	if err := p.EnsureNamespace(ctx, "tenant-acme", "acme", domain.QuotasFor(domain.PlanFree)); err != nil {
		t.Fatalf("EnsureNamespace idempotente falló: %v", err)
	}
}

func TestEnsureNamespaceCreatesPrivateRegistryPullSecret(t *testing.T) {
	cs := fake.NewSimpleClientset()
	scheme := runtime.NewScheme()
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		certManagerGVR: "CertificateList",
		// El Middleware de Traefik cierra las previews con contraseña: sin
		// registrarlo aquí, el cliente falso no sabe resolver el GVR y RetireApp
		// —que lo borra siempre— se cae en los tests.
		traefikMiddlewareGVR: "MiddlewareList",
	})
	cfg := runtimeAccessConfig()
	cfg.Env = "prod"
	cfg.RegistryURL = "https://rg.fr-par.scw.cloud/sytue-prod-paas"
	cfg.RegistryUsername = "nologin"
	cfg.RegistryPassword = "read-only-test-key"
	p := newKubePlatformFor(cs, dyn, cfg)

	if err := p.EnsureNamespace(context.Background(), "tenant-private", "private", domain.QuotasFor(domain.PlanFree)); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}
	secret, err := cs.CoreV1().Secrets("tenant-private").Get(context.Background(), tenantRegistrySecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret de pull no creado: %v", err)
	}
	if secret.Type != corev1.SecretTypeDockerConfigJson {
		t.Fatalf("tipo de Secret=%q, se esperaba dockerconfigjson", secret.Type)
	}
	var dockerConfig struct {
		Auths map[string]struct {
			Username string `json:"username"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &dockerConfig); err != nil {
		t.Fatalf("dockerconfigjson inválido: %v", err)
	}
	got, ok := dockerConfig.Auths["rg.fr-par.scw.cloud"]
	if !ok || got.Username != "nologin" {
		t.Fatalf("credencial del host privado ausente: %+v", dockerConfig.Auths)
	}
}

func TestEnsureNamespaceFailsClosedWithoutRegistryCredentialsInProd(t *testing.T) {
	p, _ := newTestPlatform()
	p.cfg = runtimeAccessConfig()
	p.cfg.Env = "prod"
	if err := p.EnsureNamespace(context.Background(), "tenant-no-creds", "no-creds", domain.QuotasFor(domain.PlanFree)); err == nil {
		t.Fatal("producción debe rechazar un namespace sin credenciales del registry privado")
	}
}

// internetIPBlock devuelve el ipBlock 0.0.0.0/0 de la política, si existe.
func internetIPBlock(np *networkingv1.NetworkPolicy) *networkingv1.IPBlock {
	for _, rule := range np.Spec.Egress {
		for _, to := range rule.To {
			if to.IPBlock != nil && to.IPBlock.CIDR == "0.0.0.0/0" {
				return to.IPBlock
			}
		}
	}
	return nil
}

// Sin ClusterCIDRs configurados, el except no puede quedar sin los CIDR del
// clúster: caen los defaults de Kapsule.
func TestEnsureNamespaceDefaultClusterCIDRs(t *testing.T) {
	p, cs := newTestPlatform()
	p.cfg.ClusterCIDRs = nil
	ctx := context.Background()
	if err := p.EnsureNamespace(ctx, "tenant-beta", "beta", domain.QuotasFor(domain.PlanFree)); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}
	allow, err := cs.NetworkingV1().NetworkPolicies("tenant-beta").Get(ctx, "allow-ingress-and-dns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("NetworkPolicy allow no creada: %v", err)
	}
	inet := internetIPBlock(allow)
	if inet == nil {
		t.Fatalf("falta la regla de egress a Internet")
	}
	for _, want := range []string{"100.64.0.0/14", "10.32.0.0/12"} {
		if !slices.Contains(inet.Except, want) {
			t.Errorf("sin config deben aplicarse los defaults de Kapsule (%s); except=%v", want, inet.Except)
		}
	}
}

// D3 — las NetworkPolicy seleccionaban el namespace por
// `kubernetes.io/metadata.name: traefik`, un namespace que este despliegue NO tiene
// (Helm etiqueta el de plataforma con `sytue.eu/role: ingress`). El selector no
// casaba con nada: las apps de cliente quedaban inalcanzables desde el ingress.
func TestNetworkPolicyIngressSeleccionaElNamespaceQueExiste(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	if err := p.EnsureNamespace(ctx, "tenant-acme", "acme", domain.QuotasFor(domain.PlanFree)); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}
	for _, name := range []string{"allow-ingress-and-dns", "allow-managed-ingress"} {
		np, err := cs.NetworkingV1().NetworkPolicies("tenant-acme").Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("NetworkPolicy %s: %v", name, err)
		}
		sel := np.Spec.Ingress[0].From[0].NamespaceSelector
		if sel == nil || sel.MatchLabels["sytue.eu/role"] != "ingress" {
			t.Errorf("%s debe aceptar tráfico del namespace etiquetado sytue.eu/role=ingress; got %+v", name, sel)
		}
		if sel != nil && sel.MatchLabels["kubernetes.io/metadata.name"] != "" {
			t.Errorf("%s sigue seleccionando un namespace por nombre inexistente: %+v", name, sel)
		}
	}

	// La regla PostgREST↔Postgres ya no abre el namespace entero (PodSelector{}).
	managed, err := cs.NetworkingV1().NetworkPolicies("tenant-acme").Get(ctx, "allow-managed-ingress", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("allow-managed-ingress: %v", err)
	}
	for i, rule := range managed.Spec.Ingress {
		for _, from := range rule.From {
			if from.PodSelector != nil && len(from.PodSelector.MatchLabels) == 0 && len(from.PodSelector.MatchExpressions) == 0 {
				t.Errorf("ingress[%d]: PodSelector{} deja entrar a cualquier pod del namespace", i)
			}
		}
	}
	for i, rule := range managed.Spec.Egress {
		for _, to := range rule.To {
			if to.PodSelector != nil && len(to.PodSelector.MatchLabels) == 0 && len(to.PodSelector.MatchExpressions) == 0 {
				t.Errorf("egress[%d]: PodSelector{} deja salir hacia cualquier pod del namespace", i)
			}
		}
	}
}

// D4 — la app recibía DATABASE_URL y no podía usarla: su egress excluía los CIDR
// del clúster y no tenía ninguna regla hacia pods del namespace, y DB Studio
// tampoco alcanzaba el 5432. La apertura va ACOTADA AL PROYECTO: abrir el
// namespace cambiaría el fallo por una fuga entre proyectos del mismo tenant.
func TestProjectNetworkPolicyAbreLaBaseDeDatosDelProyecto(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	const projID = "6f1c9a4e-1111-2222-3333-444455556666"
	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web", ProjectID: projID,
		DeploymentID: "cccc1111-2222-3333-4444-555555555555",
		Target:       domain.TargetProduction, Image: "img", Replicas: 1, Port: 8080,
		Hostnames: []string{"web.apps.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}

	// Los pods de la app llevan la clave de emparejamiento con sus servicios.
	dep, err := cs.AppsV1().Deployments("tenant-acme").Get(ctx, appResourceName(spec), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment: %v", err)
	}
	if got := dep.Spec.Template.Labels[projectLabel]; got != projID {
		t.Errorf("los pods de la app deben llevar %s=%s, llevan %q", projectLabel, projID, got)
	}

	np, err := cs.NetworkingV1().NetworkPolicies("tenant-acme").Get(ctx, projectPolicyName(projID), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("NetworkPolicy del proyecto no creada: %v", err)
	}
	if np.Spec.PodSelector.MatchLabels[projectLabel] != projID {
		t.Errorf("la política debe seleccionar solo los pods del proyecto: %+v", np.Spec.PodSelector)
	}

	// Egress: hacia los servicios gestionados DE ESE proyecto, en 5432 y 3000.
	if len(np.Spec.Egress) != 1 || len(np.Spec.Egress[0].To) != 1 {
		t.Fatalf("egress inesperado: %+v", np.Spec.Egress)
	}
	to := np.Spec.Egress[0].To[0].PodSelector
	if to == nil || to.MatchLabels[projectLabel] != projID ||
		to.MatchLabels["sytue.eu/component"] != "cloud-managed-service" {
		t.Errorf("el egress debe acotarse a los servicios gestionados del proyecto: %+v", to)
	}
	if !hasPorts(np.Spec.Egress[0].Ports, 5432, 3000) {
		t.Errorf("la app debe alcanzar Postgres (5432) y PostgREST (3000): %+v", np.Spec.Egress[0].Ports)
	}

	// Ingress: del propio proyecto, y de DB Studio (cloud-api) en el 5432.
	if len(np.Spec.Ingress) != 2 {
		t.Fatalf("se esperaban 2 reglas de ingress: %+v", np.Spec.Ingress)
	}
	if from := np.Spec.Ingress[0].From[0].PodSelector; from == nil || from.MatchLabels[projectLabel] != projID {
		t.Errorf("la primera regla de ingress debe acotarse al proyecto: %+v", from)
	}
	studio := np.Spec.Ingress[1].From[0]
	if studio.NamespaceSelector == nil || studio.NamespaceSelector.MatchLabels["sytue.eu/role"] != "ingress" {
		t.Errorf("DB Studio debe venir del namespace de plataforma: %+v", studio.NamespaceSelector)
	}
	if studio.PodSelector == nil || studio.PodSelector.MatchLabels["app.kubernetes.io/component"] != "cloud-api" {
		t.Errorf("solo cloud-api puede alcanzar el 5432 desde plataforma: %+v", studio.PodSelector)
	}
	if !hasPorts(np.Spec.Ingress[1].Ports, 5432) {
		t.Errorf("DB Studio necesita el 5432: %+v", np.Spec.Ingress[1].Ports)
	}

	// Idempotente.
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp idempotente: %v", err)
	}
}

// hasPorts comprueba que la lista contiene exactamente los puertos dados.
func hasPorts(ports []networkingv1.NetworkPolicyPort, want ...int) bool {
	if len(ports) != len(want) {
		return false
	}
	for _, w := range want {
		found := false
		for _, p := range ports {
			if p.Port != nil && p.Port.IntValue() == w {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestApplyAppProductionResources(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()

	spec := AppSpec{
		Tenant:       "acme",
		Namespace:    "tenant-acme",
		ProjectSlug:  "web",
		DeploymentID: "11111111-2222-3333-4444-555555555555",
		Target:       domain.TargetProduction,
		Image:        "rg.fr-par.scw.cloud/sytue/web@sha256:abc",
		Replicas:     2,
		Port:         8080,
		Env:          map[string]string{"SYTUE_PROJECT": "web"},
		Secrets:      map[string]string{"DATABASE_URL": "postgres://x"},
		Hostnames:    []string{"web.apps.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}

	name := appResourceName(spec)
	dep, err := cs.AppsV1().Deployments("tenant-acme").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment no creado: %v", err)
	}
	if *dep.Spec.Replicas != 2 {
		t.Errorf("réplicas esperadas 2, son %d", *dep.Spec.Replicas)
	}
	sc := dep.Spec.Template.Spec.Containers[0].SecurityContext
	if sc == nil || sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Errorf("el contenedor de la app debe correr rootless")
	}
	if len(dep.Spec.Template.Spec.Containers[0].EnvFrom) == 0 {
		t.Errorf("los secretos deben inyectarse vía envFrom")
	}
	sec, err := cs.CoreV1().Secrets("tenant-acme").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Secret no creado: %v", err)
	}
	if string(sec.Data["DATABASE_URL"]) != "postgres://x" {
		t.Errorf("secreto no materializado")
	}
	if _, err := cs.CoreV1().Services("tenant-acme").Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Errorf("Service no creado: %v", err)
	}
	ing, err := cs.NetworkingV1().Ingresses("tenant-acme").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Ingress no creado: %v", err)
	}
	if ing.Annotations["cert-manager.io/cluster-issuer"] != "sytue-letsencrypt" {
		t.Errorf("issuer de cert-manager no anotado")
	}
	if len(ing.Spec.TLS) == 0 {
		t.Errorf("el Ingress debe declarar TLS")
	}
	// HPA solo en producción.
	if _, err := cs.AutoscalingV2().HorizontalPodAutoscalers("tenant-acme").Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Errorf("HPA no creado en producción: %v", err)
	}

	// Idempotencia: re-aplicar no falla.
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp idempotente falló: %v", err)
	}
}

func TestApplyAppPreviewNoHPA(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID: "aaaa1111-2222-3333-4444-555555555555",
		Target:       domain.TargetPreview, Image: "img", Replicas: 1, Port: 8080,
		Hostnames: []string{"web-pr-3.preview.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp preview: %v", err)
	}
	name := appResourceName(spec)
	if _, err := cs.AutoscalingV2().HorizontalPodAutoscalers("tenant-acme").Get(ctx, name, metav1.GetOptions{}); err == nil {
		t.Errorf("las previews no deben tener HPA")
	}
}

func TestRetireApp(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID: "bbbb1111-2222-3333-4444-555555555555",
		Target:       domain.TargetProduction, Image: "img", Replicas: 1, Port: 8080,
		Hostnames: []string{"web.apps.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}
	if err := p.RetireApp(ctx, "tenant-acme", spec.DeploymentID); err != nil {
		t.Fatalf("RetireApp: %v", err)
	}
	name := appResourceName(spec)
	if _, err := cs.AppsV1().Deployments("tenant-acme").Get(ctx, name, metav1.GetOptions{}); err == nil {
		t.Errorf("el Deployment debe haberse eliminado")
	}
}

func TestUserAppGetsBoundedWritableTmp(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID: "cccc1111-2222-3333-4444-555555555555",
		Target:       domain.TargetProduction, Image: "img", Replicas: 1, Port: 8080,
		Hostnames: []string{"web.apps.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}
	dep, err := cs.AppsV1().Deployments(spec.Namespace).Get(
		ctx, appResourceName(spec), metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("leer Deployment: %v", err)
	}
	pod := dep.Spec.Template.Spec
	if len(pod.Volumes) != 1 || pod.Volumes[0].Name != "tmp" ||
		pod.Volumes[0].EmptyDir == nil || pod.Volumes[0].EmptyDir.SizeLimit == nil ||
		pod.Volumes[0].EmptyDir.SizeLimit.String() != "256Mi" {
		t.Fatalf("/tmp debe ser un EmptyDir limitado a 256Mi: %#v", pod.Volumes)
	}
	container := pod.Containers[0]
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].MountPath != "/tmp" {
		t.Fatalf("/tmp no montado en la app: %#v", container.VolumeMounts)
	}
	gotTmpDir := ""
	for _, variable := range container.Env {
		if variable.Name == "TMPDIR" {
			gotTmpDir = variable.Value
		}
	}
	if gotTmpDir != "/tmp" {
		t.Fatalf("TMPDIR=%q, se esperaba /tmp", gotTmpDir)
	}
}

func TestDeploymentReady(t *testing.T) {
	cs := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "ns"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptrInt32(2)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 2},
	})
	scheme := runtime.NewScheme()
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		certManagerGVR: "CertificateList",
		// El Middleware de Traefik cierra las previews con contraseña: sin
		// registrarlo aquí, el cliente falso no sabe resolver el GVR y RetireApp
		// —que lo borra siempre— se cae en los tests.
		traefikMiddlewareGVR: "MiddlewareList",
	})
	p := newKubePlatformFor(cs, dyn, testCfg())
	ready, err := p.deploymentReady(context.Background(), "ns", "d")
	if err != nil || !ready {
		t.Fatalf("deploymentReady=%v err=%v; se esperaba ready", ready, err)
	}
}

func TestEnsureTLSCreatesCertificate(t *testing.T) {
	p, _ := newTestPlatform()
	ctx := context.Background()
	// Sin Secret TLS ni Certificate previo: primera pasada crea el Certificate y
	// devuelve issued=false.
	issued, err := p.EnsureTLS(ctx, TLSRequest{Hostname: "custom.example.com", Issuer: "sytue-letsencrypt", Tenant: "acme"})
	if err != nil {
		t.Fatalf("EnsureTLS: %v", err)
	}
	if issued {
		t.Errorf("un certificado recién solicitado no debe estar emitido")
	}
	got, err := p.dyn.Resource(certManagerGVR).Namespace("tenant-acme").Get(ctx, "cert-"+dnsSafe("custom.example.com"), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Certificate no creado: %v", err)
	}
	if got.GetName() == "" {
		t.Errorf("Certificate sin nombre")
	}
}

func TestEnsureTLSIssuedWhenSecretPresent(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	host := "ready.example.com"
	// Pre-crea el Certificate y el Secret TLS emitido.
	if _, err := p.EnsureTLS(ctx, TLSRequest{Hostname: host, Issuer: "i", Tenant: "acme"}); err != nil {
		t.Fatalf("EnsureTLS setup: %v", err)
	}
	if _, err := cs.CoreV1().Secrets("tenant-acme").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-" + dnsSafe(host), Namespace: "tenant-acme"},
		Type:       corev1.SecretTypeTLS,
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("crear Secret TLS: %v", err)
	}
	// Sustituye la sonda TLS por una que confirma emisión (sin red).
	p.httpProbe = func(context.Context, string) bool { return true }
	issued, err := p.EnsureTLS(ctx, TLSRequest{Hostname: host, Issuer: "i", Tenant: "acme"})
	if err != nil {
		t.Fatalf("EnsureTLS: %v", err)
	}
	if !issued {
		t.Errorf("con Secret TLS presente y sonda OK, debe reportar emitido")
	}
}

// TestVerifyDomainExigeTXT fija el contrato de SEGURIDAD de la verificación de
// dominios: la propiedad se prueba con el TXT `_sytue-challenge.<host>`, y apuntar
// el DNS al ingress NO basta. Antes bastaba (CNAME/A), lo que permitía secuestrar
// por DNS colgante el dominio de otro tenant.
func TestVerifyDomainExigeTXT(t *testing.T) {
	const host, token = "match.example.com", "sytue-verify=abc123"

	t.Run("solo enrutado no verifica", func(t *testing.T) {
		p, _ := newTestPlatform()
		// CNAME y A apuntan al ingress, pero no hay TXT: es exactamente el caso del
		// secuestro por DNS colgante y debe quedarse pendiente.
		p.resolver = fakeResolverStub{}
		ok, err := p.VerifyDomain(context.Background(), host, token, "ingress.sytue.eu")
		if err != nil {
			t.Fatalf("VerifyDomain: %v", err)
		}
		if ok {
			t.Error("apuntar el DNS al ingress NO puede verificar la propiedad")
		}
	})

	t.Run("TXT correcto verifica", func(t *testing.T) {
		p, _ := newTestPlatform()
		p.resolver = txtResolverStub{want: "_sytue-challenge." + host, txt: []string{token}}
		ok, err := p.VerifyDomain(context.Background(), host, token, "ingress.sytue.eu")
		if err != nil {
			t.Fatalf("VerifyDomain: %v", err)
		}
		if !ok {
			t.Error("con el TXT del reto publicado debe verificar")
		}
	})

	t.Run("token de OTRO dominio no verifica", func(t *testing.T) {
		p, _ := newTestPlatform()
		p.resolver = txtResolverStub{want: "_sytue-challenge." + host, txt: []string{"sytue-verify=de-otro"}}
		ok, _ := p.VerifyDomain(context.Background(), host, token, "ingress.sytue.eu")
		if ok {
			t.Error("un TXT con otro token no puede verificar")
		}
	})

	t.Run("valor entrecomillado por el proveedor DNS", func(t *testing.T) {
		p, _ := newTestPlatform()
		p.resolver = txtResolverStub{want: "_sytue-challenge." + host, txt: []string{`"` + token + `"`}}
		ok, _ := p.VerifyDomain(context.Background(), host, token, "ingress.sytue.eu")
		if !ok {
			t.Error("debe tolerar el valor entre comillas")
		}
	})

	t.Run("sin token emitido nunca verifica (fail-closed)", func(t *testing.T) {
		p, _ := newTestPlatform()
		// Un TXT vacío publicado por el atacante no debe colar si no hay reto.
		p.resolver = txtResolverStub{want: "_sytue-challenge." + host, txt: []string{""}}
		ok, _ := p.VerifyDomain(context.Background(), host, "", "ingress.sytue.eu")
		if ok {
			t.Error("sin token emitido no puede haber verificación")
		}
	})

	t.Run("DNS sin propagar sigue pendiente, no falla", func(t *testing.T) {
		p, _ := newTestPlatform()
		p.resolver = txtResolverStub{err: errors.New("NXDOMAIN")}
		ok, err := p.VerifyDomain(context.Background(), host, token, "ingress.sytue.eu")
		if err != nil {
			t.Fatalf("un DNS sin propagar no es un error duro: %v", err)
		}
		if ok {
			t.Error("sin TXT no puede verificar")
		}
	})
}

// txtResolverStub devuelve TXT solo para el host de reto esperado.
type txtResolverStub struct {
	want string
	txt  []string
	err  error
}

func (txtResolverStub) LookupCNAME(context.Context, string) (string, error) {
	return "otro.destino.example.", nil
}

func (txtResolverStub) LookupHost(context.Context, string) ([]string, error) {
	return []string{"203.0.113.7"}, nil
}

func (s txtResolverStub) LookupTXT(_ context.Context, host string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.want != "" && host != s.want {
		return nil, errors.New("NXDOMAIN")
	}
	return s.txt, nil
}

func TestDNSSafe(t *testing.T) {
	cases := map[string]string{
		"App.Example.COM": "app-example-com",
		"a_b":             "a-b",
		"":                "host",
		"--x--":           "x",
	}
	for in, want := range cases {
		if got := dnsSafe(in); got != want {
			t.Errorf("dnsSafe(%q)=%q, se esperaba %q", in, got, want)
		}
	}
}

func ptrInt32(v int32) *int32 { return &v }

// fakeResolverStub simula un resolver DNS: el CNAME nunca coincide, pero el
// hostname y el ingress target resuelven a la misma IP (verificación por A).
type fakeResolverStub struct{}

func (fakeResolverStub) LookupCNAME(context.Context, string) (string, error) {
	return "otro.destino.example.", nil
}

func (fakeResolverStub) LookupHost(context.Context, string) ([]string, error) {
	return []string{"203.0.113.7"}, nil
}

// Sin TXT publicado: enrutado correcto pero propiedad NO probada.
func (fakeResolverStub) LookupTXT(context.Context, string) ([]string, error) {
	return nil, errors.New("NXDOMAIN")
}

// TestIngressSirveLaMedicionEnPrimeraParte fija la decisión central del ADR 0026:
// el colector se sirve BAJO EL DOMINIO DEL CLIENTE, no en uno nuestro. De ahí sale
// que el sitio se identifique por el Host y que el script no lleve identificador,
// que es lo que hace imposible inflar los números de otro.
func TestIngressSirveLaMedicionEnPrimeraParte(t *testing.T) {
	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "tienda",
		Hostnames: []string{"tienda.apps.sytue.eu"},
	}

	t.Run("sin colector configurado no se añade la ruta", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		p := &KubePlatform{cs: cs, cfg: config.Config{}}
		if err := p.applyIngress(context.Background(), spec, "app"); err != nil {
			t.Fatalf("applyIngress: %v", err)
		}
		ing, err := cs.NetworkingV1().Ingresses(spec.Namespace).
			Get(context.Background(), "app", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("leer ingress: %v", err)
		}
		paths := ing.Spec.Rules[0].HTTP.Paths
		if len(paths) != 1 || paths[0].Path != "/" {
			t.Fatalf("sin configurar no puede haber ruta de medición: %+v", paths)
		}
		// Y tampoco el alias: una app no estrena objetos que no va a usar.
		if _, err := cs.CoreV1().Services(spec.Namespace).
			Get(context.Background(), analyticsServiceName, metav1.GetOptions{}); err == nil {
			t.Error("se creó el alias del colector sin haberlo configurado")
		}
	})

	t.Run("con colector, /_sytue va PRIMERO y apunta al alias", func(t *testing.T) {
		cs := fake.NewSimpleClientset()
		p := &KubePlatform{cs: cs, cfg: config.Config{
			AnalyticsIngestHost: "analytics-ingest.sytue-one.svc.cluster.local",
		}}
		if err := p.applyIngress(context.Background(), spec, "app"); err != nil {
			t.Fatalf("applyIngress: %v", err)
		}
		ing, err := cs.NetworkingV1().Ingresses(spec.Namespace).
			Get(context.Background(), "app", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("leer ingress: %v", err)
		}
		paths := ing.Spec.Rules[0].HTTP.Paths
		if len(paths) != 2 {
			t.Fatalf("rutas = %d; se esperaban 2 (/_sytue y /)", len(paths))
		}
		// El orden importa: si «/» fuese primero, se tragaría /_sytue y el colector
		// no recibiría nada — la medición existiría y no mediría.
		if paths[0].Path != analyticsPathPrefix {
			t.Errorf("la primera ruta es %q; tiene que ser %q", paths[0].Path, analyticsPathPrefix)
		}
		if got := paths[0].Backend.Service.Name; got != analyticsServiceName {
			t.Errorf("la medición apunta a %q y no al alias del colector", got)
		}
		if paths[1].Path != "/" {
			t.Errorf("la app tiene que seguir sirviendo en «/»: %q", paths[1].Path)
		}

		// El alias es un CNAME interno hacia el namespace de plataforma: el backend
		// de un Ingress no puede vivir en otro namespace.
		alias, err := cs.CoreV1().Services(spec.Namespace).
			Get(context.Background(), analyticsServiceName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("el alias del colector no se creó: %v", err)
		}
		if alias.Spec.Type != corev1.ServiceTypeExternalName {
			t.Errorf("el alias es %q; tiene que ser ExternalName", alias.Spec.Type)
		}
		if alias.Spec.ExternalName != "analytics-ingest.sytue-one.svc.cluster.local" {
			t.Errorf("el alias apunta a %q", alias.Spec.ExternalName)
		}
	})
}

// TestHPAUsaLaConfiguracionDelProyecto es el punto donde la configuración de
// autoescalado (migración 0031) o llega al clúster o no sirve de nada: hasta ahora
// el techo y el objetivo estaban escritos aquí, así que una app con 1 instancia no
// pasaba de 3 pods por mucha carga que le entrara.
func TestHPAUsaLaConfiguracionDelProyecto(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID: "dddd1111-2222-3333-4444-555555555555",
		Target:       domain.TargetProduction, Image: "img", Replicas: 2, Port: 8080,
		AutoscaleMax: 12, AutoscaleCPUTarget: 40,
		Hostnames: []string{"web.apps.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers("tenant-acme").
		Get(ctx, appResourceName(spec), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("HPA no creado: %v", err)
	}
	if hpa.Spec.MaxReplicas != 12 {
		t.Errorf("maxReplicas = %d, quiero 12 (el techo configurado)", hpa.Spec.MaxReplicas)
	}
	if hpa.Spec.MinReplicas == nil || *hpa.Spec.MinReplicas != 2 {
		t.Errorf("minReplicas = %d, quiero 2 (las instancias configuradas)", derefInt32(hpa.Spec.MinReplicas))
	}
	got := hpa.Spec.Metrics[0].Resource.Target.AverageUtilization
	if got == nil || *got != 40 {
		t.Errorf("objetivo de CPU = %d, quiero 40", derefInt32(got))
	}
}

// TestHPASinConfigurarConservaLaReglaAnterior: una spec sin autoescalado —las que
// construyen otros caminos internos y los tests— no puede acabar con un
// maxReplicas 0, que el API server rechaza y dejaría la app SIN NINGÚN objeto.
func TestHPASinConfigurarConservaLaReglaAnterior(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID: "eeee1111-2222-3333-4444-555555555555",
		Target:       domain.TargetProduction, Image: "img", Replicas: 3, Port: 8080,
		Hostnames: []string{"web.apps.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers("tenant-acme").
		Get(ctx, appResourceName(spec), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("HPA no creado: %v", err)
	}
	if hpa.Spec.MaxReplicas != 15 {
		t.Errorf("maxReplicas = %d, quiero 15 (3 instancias × 5, la regla de siempre)", hpa.Spec.MaxReplicas)
	}
	got := hpa.Spec.Metrics[0].Resource.Target.AverageUtilization
	if got == nil || *got != domain.DefaultCPUTarget {
		t.Errorf("objetivo de CPU = %d, quiero %d", derefInt32(got), domain.DefaultCPUTarget)
	}
}

// derefInt32 evita que un fallo imprima la DIRECCIÓN del puntero en vez del valor,
// que es justo el momento en que hace falta leerlo.
func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// TestHPATechoPorDebajoDelMinimo: el estado que Kubernetes rechaza (max < min) no
// puede llegar al clúster ni siquiera con una spec construida a mano.
func TestHPATechoPorDebajoDelMinimo(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID: "ffff1111-2222-3333-4444-555555555555",
		Target:       domain.TargetProduction, Image: "img", Replicas: 6, Port: 8080,
		AutoscaleMax: 2,
		Hostnames:    []string{"web.apps.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}
	hpa, err := cs.AutoscalingV2().HorizontalPodAutoscalers("tenant-acme").
		Get(ctx, appResourceName(spec), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("HPA no creado: %v", err)
	}
	if hpa.Spec.MinReplicas == nil || hpa.Spec.MaxReplicas < *hpa.Spec.MinReplicas {
		t.Errorf("maxReplicas=%d < minReplicas=%d: el API server lo rechazaría",
			hpa.Spec.MaxReplicas, derefInt32(hpa.Spec.MinReplicas))
	}
}

// CLOUD-07 / CLOUD-12 — ResourceQuota incluye requests.storage y
// persistentvolumeclaims según el plan; LimitRange incluye ephemeral-storage; y
// el Deployment de la app declara ephemeral-storage y RuntimeClassName=gvisor.
func TestEnsureNamespaceYApplyAppAcotanDiscoYUsanGVisor(t *testing.T) {
	p, cs := newTestPlatform()
	ctx := context.Background()
	if err := p.EnsureNamespace(ctx, "tenant-acme", "acme", domain.QuotasFor(domain.PlanFree)); err != nil {
		t.Fatalf("EnsureNamespace: %v", err)
	}
	rq, err := cs.CoreV1().ResourceQuotas("tenant-acme").Get(ctx, "tenant-quota", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("ResourceQuota: %v", err)
	}
	if _, ok := rq.Spec.Hard[corev1.ResourceRequestsStorage]; !ok {
		t.Error("ResourceQuota debe limitar requests.storage")
	}
	if _, ok := rq.Spec.Hard[corev1.ResourcePersistentVolumeClaims]; !ok {
		t.Error("ResourceQuota debe limitar persistentvolumeclaims")
	}
	lr, err := cs.CoreV1().LimitRanges("tenant-acme").Get(ctx, "tenant-limits", metav1.GetOptions{})
	if err != nil || len(lr.Spec.Limits) == 0 {
		t.Fatalf("LimitRange: %v", err)
	}
	if _, ok := lr.Spec.Limits[0].Max[corev1.ResourceEphemeralStorage]; !ok {
		t.Error("LimitRange debe fijar Max para ephemeral-storage")
	}

	spec := AppSpec{
		Tenant: "acme", Namespace: "tenant-acme", ProjectSlug: "web",
		DeploymentID: "11112222-3333-4444-5555-666677778888",
		Target:       domain.TargetProduction, Image: "img", Replicas: 1, Port: 8080,
		Hostnames: []string{"web.apps.sytue.eu"},
	}
	if _, err := p.ApplyApp(ctx, spec); err != nil {
		t.Fatalf("ApplyApp: %v", err)
	}
	dep, err := cs.AppsV1().Deployments("tenant-acme").Get(ctx, appResourceName(spec), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Deployment app: %v", err)
	}
	if rc := dep.Spec.Template.Spec.RuntimeClassName; rc == nil || *rc != "gvisor" {
		t.Errorf("RuntimeClassName del pod de app = %v, quiero \"gvisor\"", rc)
	}
	c := dep.Spec.Template.Spec.Containers[0]
	if _, ok := c.Resources.Limits[corev1.ResourceEphemeralStorage]; !ok {
		t.Error("el contenedor de app debe limitar ephemeral-storage")
	}
}
