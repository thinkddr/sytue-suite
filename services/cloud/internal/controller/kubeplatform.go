// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// certManagerGVR es el recurso Certificate de cert-manager (cert-manager.io/v1),
// gestionado vía cliente dinámico (no está en el esquema tipado de client-go).
var certManagerGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
}

// appContainerPort es el puerto que exponen los contenedores de las apps de
// cliente (convención de las plantillas de Dockerfile: 8080).
const appContainerPort = 8080

const tenantRegistrySecretName = "sytue-registry" // #nosec G101 -- nombre de objeto Kubernetes, no una credencial.

// platformNamespaceSelector selecciona el namespace de PLATAFORMA, que es donde
// corren Traefik y cloud-api. Lo etiqueta el chart con `sytue.eu/role: ingress`
// (deploy/helm/sytue-one/templates/platform-namespace.yaml), y es la etiqueta que
// ya usa la NetworkPolicy baseline. Antes aquí se seleccionaba por
// `kubernetes.io/metadata.name: traefik`, un namespace que NO existe en este
// despliegue: el selector no casaba con nada y las apps de cliente quedaban
// inalcanzables desde el ingress. Si ops separa Traefik a otro namespace, hay que
// etiquetar ambos; sin la etiqueta el tráfico queda denegado (fail-closed), nunca
// abierto de más.
func platformNamespaceSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{"sytue.eu/role": "ingress"}}
}

// privateEgressExcept son los destinos que el egress a Internet de un tenant
// nunca alcanza: rangos privados RFC1918 y link-local (metadata
// 169.254.169.254). Los CIDR de pods/services del clúster se suman desde
// config (CLUSTER_CIDRS).
var privateEgressExcept = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"169.254.0.0/16",
}

// KubePlatform es la implementación de producción de Platform sobre Kapsule con
// client-go. Aplica de forma declarativa (Server-Side Apply) los objetos de cada
// app —Deployment, Service, Ingress (Traefik), HPA— en el namespace del tenant
// (tenant-<slug>), que asegura con ResourceQuota, LimitRange y NetworkPolicy
// deny-by-default;
// gestiona TLS con cert-manager (recurso Certificate vía cliente dinámico) y
// verifica dominios propios comprobando DNS.
type KubePlatform struct {
	cs       kubernetes.Interface
	dyn      dynamic.Interface
	cfg      config.Config
	resolver dnsResolver
	// httpProbe verifica que un hostname sirve TLS válido; inyectable en tests.
	httpProbe func(ctx context.Context, hostname string) bool
}

// dnsResolver es el subconjunto de *net.Resolver que usamos para verificar
// dominios; se abstrae para poder inyectar un stub en tests sin red.
type dnsResolver interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupTXT(ctx context.Context, host string) ([]string, error)
}

// NewKubePlatform construye la plataforma sobre Kapsule. Con KubeconfigPath vacío
// usa la configuración in-cluster (el controller corre dentro del clúster con una
// ServiceAccount con permisos sobre los recursos que gestiona).
func NewKubePlatform(cfg config.Config) (*KubePlatform, error) {
	if _, err := cloudRuntimeAccessEnabled(cfg); err != nil {
		return nil, err
	}
	restCfg, err := kubeRestConfig(cfg.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("controller: crear clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("controller: crear cliente dinámico: %w", err)
	}
	return newKubePlatformFor(cs, dyn, cfg), nil
}

// newKubePlatformFor permite inyectar clientes (fakes en tests).
func newKubePlatformFor(cs kubernetes.Interface, dyn dynamic.Interface, cfg config.Config) *KubePlatform {
	p := &KubePlatform{cs: cs, dyn: dyn, cfg: cfg, resolver: net.DefaultResolver}
	p.httpProbe = defaultTLSProbe
	return p
}

// kubeRestConfig deriva la configuración REST: in-cluster o del kubeconfig dado.
func kubeRestConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath == "" {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("controller: config in-cluster: %w", err)
		}
		return cfg, nil
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("controller: cargar kubeconfig %q: %w", kubeconfigPath, err)
	}
	return cfg, nil
}

// EnsureNamespace crea el namespace tenant-<slug> con ResourceQuota (límites de
// cómputo del tenant según plan), LimitRange (defaults/máximos por contenedor) y
// NetworkPolicy deny-by-default (§0.7) si no existen. Idempotente. Implementa
// Platform.
func (p *KubePlatform) EnsureNamespace(ctx context.Context, namespace, tenant string, quota domain.PlanQuota) error {
	if err := validateTenantNamespace(namespace, tenant); err != nil {
		return err
	}
	if _, err := cloudRuntimeAccessEnabled(p.cfg); err != nil {
		return err
	}
	if quota.MaxProjects == 0 {
		quota = domain.QuotasFor(domain.PlanFree)
	}
	nsLabels := map[string]string{
		"app.kubernetes.io/part-of": "sytue-one",
		"sytue.eu/tenant":           tenant,
		"sytue.eu/namespace-kind":   "tenant",
		// PodSecurity restricted (§0.7): rechaza pods privilegiados.
		"pod-security.kubernetes.io/enforce": "restricted",
		"pod-security.kubernetes.io/audit":   "restricted",
		"pod-security.kubernetes.io/warn":    "restricted",
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: nsLabels},
	}
	if err := p.upsertNamespace(ctx, ns); err != nil {
		return err
	}
	if err := p.ensureCloudRuntimeBinding(ctx, namespace, tenant); err != nil {
		return err
	}
	if err := p.ensureTenantRegistrySecret(ctx, namespace, tenant); err != nil {
		return err
	}
	if err := p.ensureResourceQuota(ctx, namespace, tenant, quota); err != nil {
		return err
	}
	if err := p.ensureLimitRange(ctx, namespace, tenant); err != nil {
		return err
	}
	return p.ensureNetworkPolicies(ctx, namespace, tenant)
}

// ensureTenantRegistrySecret materializa en cada namespace de cliente una
// credencial de SOLO LECTURA. Los namespaces no comparten Secrets y las imágenes
// del PaaS viven en un registry privado: sin esta copia, cualquier despliegue
// correcto termina en ImagePullBackOff. En producción se falla cerrado si falta
// la credencial; nunca se degrada a imágenes públicas.
func (p *KubePlatform) ensureTenantRegistrySecret(ctx context.Context, namespace, tenant string) error {
	if p.cfg.RegistryUsername == "" || p.cfg.RegistryPassword == "" {
		if p.cfg.Env.IsProd() {
			return fmt.Errorf("controller: credenciales del registry privado ausentes")
		}
		return nil
	}
	host := strings.TrimPrefix(strings.TrimPrefix(p.cfg.RegistryURL, "https://"), "http://")
	host = strings.SplitN(strings.TrimSuffix(host, "/"), "/", 2)[0]
	if host == "" {
		return fmt.Errorf("controller: REGISTRY_URL no contiene un host")
	}
	auth := base64.StdEncoding.EncodeToString([]byte(p.cfg.RegistryUsername + ":" + p.cfg.RegistryPassword))
	dockerConfig, err := json.Marshal(map[string]any{"auths": map[string]any{
		host: map[string]string{
			"username": p.cfg.RegistryUsername,
			"password": p.cfg.RegistryPassword,
			"auth":     auth,
		},
	}})
	if err != nil {
		return fmt.Errorf("controller: serializar credencial del registry: %w", err)
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tenantRegistrySecretName,
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/tenant": tenant, "app.kubernetes.io/managed-by": "sytue-cloud"},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: dockerConfig},
	}
	api := p.cs.CoreV1().Secrets(namespace)
	existing, getErr := api.Get(ctx, sec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(getErr) {
		_, createErr := api.Create(ctx, sec, metav1.CreateOptions{})
		return ignoreAlreadyExists(createErr, "crear Secret del registry del tenant")
	}
	if getErr != nil {
		return fmt.Errorf("controller: consultar Secret del registry del tenant: %w", getErr)
	}
	sec.ResourceVersion = existing.ResourceVersion
	if _, updateErr := api.Update(ctx, sec, metav1.UpdateOptions{}); updateErr != nil {
		return fmt.Errorf("controller: actualizar Secret del registry del tenant: %w", updateErr)
	}
	return nil
}

func tenantImagePullSecrets() []corev1.LocalObjectReference {
	return []corev1.LocalObjectReference{{Name: tenantRegistrySecretName}}
}

func (p *KubePlatform) upsertNamespace(ctx context.Context, ns *corev1.Namespace) error {
	existing, err := p.cs.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		existing, err = p.cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			// Otro proceso ganó la creación: se verifica su propiedad antes de
			// crear bindings, cuotas o secretos dentro del namespace.
			existing, err = p.cs.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
		}
	}
	if err != nil {
		return fmt.Errorf("controller: consultar namespace %s: %w", ns.Name, err)
	}
	return ownedTenantNamespace(existing, ns.Labels["sytue.eu/tenant"])
}

// ensureResourceQuota fija los topes de cómputo/objetos del tenant según plan.
func (p *KubePlatform) ensureResourceQuota(ctx context.Context, namespace, tenant string, quota domain.PlanQuota) error {
	rq := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-quota",
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/tenant": tenant, "sytue.eu/plan": string(quota.Plan)},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse(quota.RequestCPU),
				corev1.ResourceRequestsMemory: resource.MustParse(quota.RequestMemory),
				corev1.ResourceLimitsCPU:      resource.MustParse(quota.LimitCPU),
				corev1.ResourceLimitsMemory:   resource.MustParse(quota.LimitMemory),
				corev1.ResourcePods:           resource.MustParse(fmt.Sprintf("%d", quota.MaxPods)),
				corev1.ResourceServices:       resource.MustParse("20"),
			},
		},
	}
	_, err := p.cs.CoreV1().ResourceQuotas(namespace).Get(ctx, rq.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, cerr := p.cs.CoreV1().ResourceQuotas(namespace).Create(ctx, rq, metav1.CreateOptions{}); cerr != nil && !apierrors.IsAlreadyExists(cerr) {
			return fmt.Errorf("controller: crear ResourceQuota: %w", cerr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("controller: consultar ResourceQuota: %w", err)
	}
	if _, uerr := p.cs.CoreV1().ResourceQuotas(namespace).Update(ctx, rq, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar ResourceQuota: %w", uerr)
	}
	return nil
}

// ensureLimitRange fija defaults y máximos de recursos por contenedor en el
// namespace del tenant, para que ningún pod (incl. servicios gestionados) quede
// sin requests/limits ni exceda topes por contenedor; complementa la
// ResourceQuota. Valores coherentes con los del Deployment de app (100m/128Mi
// request, 1/512Mi limit).
func (p *KubePlatform) ensureLimitRange(ctx context.Context, namespace, tenant string) error {
	lr := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-limits",
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/tenant": tenant},
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypeContainer,
				DefaultRequest: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Default: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Max: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			}},
		},
	}
	api := p.cs.CoreV1().LimitRanges(namespace)
	_, err := api.Get(ctx, lr.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, cerr := api.Create(ctx, lr, metav1.CreateOptions{}); cerr != nil && !apierrors.IsAlreadyExists(cerr) {
			return fmt.Errorf("controller: crear LimitRange: %w", cerr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("controller: consultar LimitRange: %w", err)
	}
	if _, uerr := api.Update(ctx, lr, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar LimitRange: %w", uerr)
	}
	return nil
}

// ensureNetworkPolicies aplica una NetworkPolicy deny-by-default (todo el tráfico
// ingress/egress bloqueado) más una política que permite el ingress desde el
// namespace de Traefik y, como egress, DNS e Internet público —0.0.0.0/0 con
// except sobre RFC1918, link-local/metadata y los CIDR del clúster, de modo que
// el tenant nunca alcanza PgBouncer/Dragonfly/kube-API—. Defensa en profundidad
// (§0.7).
func (p *KubePlatform) ensureNetworkPolicies(ctx context.Context, namespace, tenant string) error {
	deny := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-deny-all",
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/tenant": tenant},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // todos los pods
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	tcp := corev1.ProtocolTCP
	udp := corev1.ProtocolUDP
	dnsPort := intstr.FromInt32(53)
	httpPort := intstr.FromInt32(appContainerPort)

	// Config vacía (construcción directa, sin Load) no puede degradar el
	// aislamiento: caen los defaults de Kapsule.
	clusterCIDRs := p.cfg.ClusterCIDRs
	if len(clusterCIDRs) == 0 {
		clusterCIDRs = strings.Split(config.DefaultClusterCIDRs, ",")
	}
	except := make([]string, 0, len(privateEgressExcept)+len(clusterCIDRs))
	except = append(except, privateEgressExcept...)
	except = append(except, clusterCIDRs...)

	allow := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-ingress-and-dns",
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/tenant": tenant},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"sytue.eu/component": "cloud-app"},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From:  []networkingv1.NetworkPolicyPeer{{NamespaceSelector: platformNamespaceSelector()}},
				Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &httpPort}},
			}},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					// DNS (kube-dns) hacia cualquier destino del clúster.
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &dnsPort},
						{Protocol: &tcp, Port: &dnsPort},
					},
				},
				{
					// Internet público en cualquier puerto; nunca redes
					// privadas, metadata ni pods/services del clúster.
					To: []networkingv1.NetworkPolicyPeer{{
						IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0", Except: except},
					}},
				},
			},
		},
	}

	postgrestPort := intstr.FromInt32(3000)
	pgPort := intstr.FromInt32(5432)
	allowManaged := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-managed-ingress",
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/tenant": tenant},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"sytue.eu/component": "cloud-managed-service"},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: platformNamespaceSelector()}},
					// Traefik alcanza los servicios gestionados expuestos: PostgREST
					// (3000) y las funciones HTTP (appContainerPort). Postgres no
					// escucha en esos puertos, así que abrirlos no lo expone.
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &postgrestPort}, {Protocol: &tcp, Port: &httpPort}},
				},
				{
					// PostgREST ↔ Postgres. Antes era PodSelector{} —CUALQUIER pod del
					// namespace, o sea la app de cualquier otro proyecto del tenant—;
					// ahora solo entre servicios gestionados. El camino app ↔ base de
					// datos lo abre allow-project-<id>, acotado por proyecto.
					// ponytail: queda un residual intra-tenant (el PostgREST de un
					// proyecto ve el 5432 de otro, sin sus credenciales). Cierre pleno:
					// borrar esta regla cuando todos los servicios gestionados vivos
					// lleven ya la etiqueta sytue.eu/project-id.
					From: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"sytue.eu/component": "cloud-managed-service"},
					}}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &pgPort}, {Protocol: &tcp, Port: &postgrestPort}},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &dnsPort},
						{Protocol: &tcp, Port: &dnsPort},
					},
				},
				{
					// Simétrica de la ingress de arriba: entre servicios gestionados, no
					// hacia cualquier pod del namespace.
					To: []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"sytue.eu/component": "cloud-managed-service"},
					}}},
				},
				{
					To: []networkingv1.NetworkPolicyPeer{{
						IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0", Except: except},
					}},
				},
			},
		},
	}

	// Apps del catálogo publicadas en internet (web_port en su entrada). Hace falta
	// una política aparte porque allow-managed-ingress abre desde Traefik SOLO 3000
	// y 8080, y un Nextcloud escucha en el 80: con la regla de arriba el Ingress se
	// crearía, el certificado se emitiría y la URL daría timeout, sin nada en los
	// logs que apuntara a la red.
	//
	// El selector es la etiqueta que pone el aprovisionador cuando —y solo cuando—
	// el CATÁLOGO declara la app publicable, así que esto no abre nada que una
	// entrada revisada no haya decidido abrir. Sin `Ports` a propósito: cada app
	// escucha donde quiere, y lo que de verdad acota la superficie es que Traefik
	// solo enruta hacia donde hay un Ingress.
	allowPublished := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-published-apps",
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/tenant": tenant},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{publishedLabel: "true"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: platformNamespaceSelector()}},
			}},
		},
	}

	for _, np := range []*networkingv1.NetworkPolicy{deny, allow, allowManaged, allowPublished} {
		if err := upsertNetworkPolicy(ctx, p.cs, namespace, np); err != nil {
			return err
		}
	}
	return nil
}

// publishedLabel marca los pods de una app del catálogo que el catálogo declara
// publicable. Es lo que selecciona la NetworkPolicy que deja entrar a Traefik.
const publishedLabel = "sytue.eu/published"

// projectLabel es la etiqueta que empareja los pods de un mismo PROYECTO dentro
// del namespace del tenant (apps y servicios gestionados). Lleva el UUID del
// proyecto y no su slug porque el aprovisionador de servicios solo conoce el
// ProjectID; sytue.eu/project sigue llevando el slug, para leerlo a ojo.
const projectLabel = "sytue.eu/project-id"

// ensureProjectNetworkPolicy abre el ÚNICO camino intra-namespace que una app
// necesita: hablar con los servicios gestionados DE SU MISMO PROYECTO (Postgres
// 5432, PostgREST 3000) y dejarse consultar por DB Studio (los pods de cloud-api
// del namespace de plataforma) en el 5432. Sin ella la app recibía DATABASE_URL y
// no podía usarla: el egress del tenant excluye los CIDR del clúster y no tenía
// ninguna regla hacia pods del namespace.
//
// Va acotada por proyecto (projectLabel) y no a nivel de namespace: una regla
// hacia PodSelector{} cambiaría el fallo por una fuga —cada proyecto vería la base
// de datos de los demás—. La aplican tanto ApplyApp (en cada reconciliación de la
// app) como Provision (un proyecto puede tener base de datos sin app desplegada).
// Idempotente.
//
// ponytail: no se borra al eliminar el proyecto; queda una política que no
// selecciona ningún pod (inerte). Barrerla exige listar por etiqueta al borrar el
// proyecto, y ese camino hoy no pasa por aquí.
func ensureProjectNetworkPolicy(ctx context.Context, cs kubernetes.Interface, namespace, tenant, projectID string) error {
	tcp := corev1.ProtocolTCP
	pgPort := intstr.FromInt32(5432)
	restPort := intstr.FromInt32(3000)
	dbPorts := []networkingv1.NetworkPolicyPort{
		{Protocol: &tcp, Port: &pgPort},
		{Protocol: &tcp, Port: &restPort},
	}
	sameProject := map[string]string{projectLabel: projectID}
	projectManaged := map[string]string{
		"sytue.eu/component": "cloud-managed-service",
		projectLabel:         projectID,
	}

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectPolicyName(projectID),
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/tenant": tenant, projectLabel: projectID},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: sameProject},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From:  []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: sameProject}}},
					Ports: dbPorts,
				},
				{
					// DB Studio: SOLO los pods de cloud-api del namespace de plataforma
					// (namespaceSelector + podSelector en el MISMO peer = conjunción, así
					// que Traefik, que comparte namespace, no alcanza el 5432).
					From: []networkingv1.NetworkPolicyPeer{{
						NamespaceSelector: platformNamespaceSelector(),
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app.kubernetes.io/component": "cloud-api"},
						},
					}},
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &pgPort}},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To:    []networkingv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: projectManaged}}},
				Ports: dbPorts,
			}},
		},
	}
	return upsertNetworkPolicy(ctx, cs, namespace, np)
}

func projectPolicyName(projectID string) string {
	return "allow-project-" + dnsSafe(projectID)
}

func upsertNetworkPolicy(ctx context.Context, cs kubernetes.Interface, namespace string, np *networkingv1.NetworkPolicy) error {
	api := cs.NetworkingV1().NetworkPolicies(namespace)
	existing, err := api.Get(ctx, np.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, cerr := api.Create(ctx, np, metav1.CreateOptions{}); cerr != nil && !apierrors.IsAlreadyExists(cerr) {
			return fmt.Errorf("controller: crear NetworkPolicy %s: %w", np.Name, cerr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("controller: consultar NetworkPolicy %s: %w", np.Name, err)
	}
	np.ResourceVersion = existing.ResourceVersion
	if _, uerr := api.Update(ctx, np, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar NetworkPolicy %s: %w", np.Name, uerr)
	}
	return nil
}

// ApplyApp aplica el Deployment/Service/Ingress/HPA de una app y devuelve si el
// despliegue está disponible (todas las réplicas listas). Idempotente vía
// upsert. Implementa Platform.
func (p *KubePlatform) ApplyApp(ctx context.Context, spec AppSpec) (bool, error) {
	l := log.From(ctx).With("namespace", spec.Namespace, "deployment", spec.DeploymentID)
	name := appResourceName(spec)

	// La app necesita alcanzar los servicios gestionados de SU proyecto; se
	// (re)afirma en cada reconciliación, no solo al aprovisionar la base de datos.
	if spec.ProjectID != "" {
		if err := ensureProjectNetworkPolicy(ctx, p.cs, spec.Namespace, spec.Tenant, spec.ProjectID); err != nil {
			return false, err
		}
	}
	if err := p.applySecret(ctx, spec, name); err != nil {
		return false, err
	}
	if err := p.applyDeployment(ctx, spec, name); err != nil {
		return false, err
	}
	if err := p.applyService(ctx, spec, name); err != nil {
		return false, err
	}
	if err := p.applyIngress(ctx, spec, name); err != nil {
		return false, err
	}
	if spec.Target == "production" {
		if spec.Paused {
			// Un HPA vivo devolvería la app a su mínimo en el acto: pausar y
			// autoescalar son órdenes contrarias, y gana la del cliente.
			if err := p.deleteHPA(ctx, spec.Namespace, name); err != nil {
				return false, err
			}
		} else if err := p.applyHPA(ctx, spec, name); err != nil {
			return false, err
		}
	}

	if spec.Paused {
		// Con cero réplicas no hay ninguna que esperar: el deployment está en su
		// estado deseado. Sin esto se quedaría en «deploying» para siempre.
		l.Debug("app pausada: cero réplicas")
		return true, nil
	}
	ready, err := p.deploymentReady(ctx, spec.Namespace, name)
	if err != nil {
		return false, err
	}
	l.Debug("app aplicada", "ready", ready)
	return ready, nil
}

// deleteHPA retira el autoescalado de una app. Que no exista no es un error: es
// el caso de una preview o de una app que nunca llegó a tenerlo.
func (p *KubePlatform) deleteHPA(ctx context.Context, namespace, name string) error {
	err := p.cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).
		Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("controller: retirar HPA: %w", err)
	}
	return nil
}

// RetireApp elimina los objetos de un deployment (preview cerrado, rollback).
// Implementa Platform.
func (p *KubePlatform) RetireApp(ctx context.Context, namespace, deploymentID string) error {
	name := "app-" + shortID(deploymentID)
	del := func(kind string, delFn func() error) {
		if err := delFn(); err != nil && !apierrors.IsNotFound(err) {
			log.From(ctx).Warn("retirar app: eliminar recurso", "kind", kind, "name", name, "error", err)
		}
	}
	bg := metav1.DeletePropagationBackground
	opts := metav1.DeleteOptions{PropagationPolicy: &bg}
	del("hpa", func() error {
		return p.cs.AutoscalingV2().HorizontalPodAutoscalers(namespace).Delete(ctx, name, opts)
	})
	del("ingress", func() error { return p.cs.NetworkingV1().Ingresses(namespace).Delete(ctx, name, opts) })
	del("service", func() error { return p.cs.CoreV1().Services(namespace).Delete(ctx, name, opts) })
	del("deployment", func() error { return p.cs.AppsV1().Deployments(namespace).Delete(ctx, name, opts) })
	del("secret", func() error { return p.cs.CoreV1().Secrets(namespace).Delete(ctx, name, opts) })
	// Middleware + Secret de la preview cerrada: sin esto quedaría un objeto por
	// cada PR cerrado en el namespace del cliente.
	p.removePreviewAuth(ctx, namespace, deploymentID)
	// Los middlewares de redirecciones/cabeceras se barren por etiqueta: su
	// número cambia entre despliegues y borrar «los que me sé» dejaría sueltos
	// los que sobran.
	p.removeHTTPRules(ctx, namespace, deploymentID)
	return nil
}

// applySecret materializa los valores sensibles (secretos + credenciales de
// servicios gestionados) en un Secret de Kubernetes montado como envFrom.
func (p *KubePlatform) applySecret(ctx context.Context, spec AppSpec, name string) error {
	data := make(map[string][]byte, len(spec.Secrets))
	for k, v := range spec.Secrets {
		data[k] = []byte(v)
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: spec.Namespace,
			Labels:    appLabels(spec),
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	api := p.cs.CoreV1().Secrets(spec.Namespace)
	existing, err := api.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, sec, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Secret")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Secret: %w", err)
	}
	sec.ResourceVersion = existing.ResourceVersion
	_, uerr := api.Update(ctx, sec, metav1.UpdateOptions{})
	if uerr != nil {
		return fmt.Errorf("controller: actualizar Secret: %w", uerr)
	}
	return nil
}

// applyDeployment aplica el Deployment de la app con securityContext restrictivo,
// env no sensible inline, secretos vía envFrom, sondas y límites de recursos.
func (p *KubePlatform) applyDeployment(ctx context.Context, spec AppSpec, name string) error {
	replicas := clampReplicas(spec.Replicas)
	// Pausado: cero réplicas. No se retira nada más —Ingress, Service y
	// certificado se quedan— para que reanudar no vuelva a pasar por Let's
	// Encrypt ni cambie ninguna URL.
	if spec.Paused {
		replicas = 0
	}
	runAsNonRoot := true
	readOnlyRoot := true
	allowPrivEsc := false
	runAsUser := int64(1000)
	automountSA := false
	tmpSize := resource.MustParse("256Mi")

	env := make([]corev1.EnvVar, 0, len(spec.Env))
	for k, v := range spec.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}

	selector := map[string]string{"sytue.eu/deployment": spec.DeploymentID}
	podLabels := appLabels(spec)
	for k, v := range selector {
		podLabels[k] = v
	}

	port := clampPort(spec.Port)
	// Un AppSpec sin tamaño resuelto (llamada antigua o test) degrada al básico en
	// vez de generar un contenedor sin recursos declarados, que el planificador
	// metería en cualquier nodo y podría tumbar a un vecino.
	size := domain.InstanceSizeOrDefault(spec.Size.Name)
	// Ruta de sondeo del proyecto; el saneado vive aquí porque un valor inválido en
	// el Deployment fallaría de forma opaca (el pod nunca pasa a Ready).
	healthPath := spec.HealthPath
	if healthPath == "" || !strings.HasPrefix(healthPath, "/") {
		healthPath = domain.DefaultHealthPath
	}

	container := corev1.Container{
		Name:  "app",
		Image: spec.Image,
		Ports: []corev1.ContainerPort{{ContainerPort: port, Name: "http"}},
		Env:   append(env, corev1.EnvVar{Name: "TMPDIR", Value: "/tmp"}),
		EnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: name},
			},
		}},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &runAsNonRoot,
			RunAsUser:                &runAsUser,
			AllowPrivilegeEscalation: &allowPrivEsc,
			ReadOnlyRootFilesystem:   &readOnlyRoot,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
			SeccompProfile:           &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		// Recursos del TAMAÑO DE INSTANCIA elegido (domain.InstanceSizes). Antes
		// estaban escritos a mano aquí, así que toda app —grande o pequeña— corría
		// igual y pagaba igual. `basic` reproduce exactamente aquellos valores.
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(size.CPURequest),
				corev1.ResourceMemory: resource.MustParse(size.MemoryRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(size.CPULimit),
				corev1.ResourceMemory: resource.MustParse(size.MemoryLimit),
			},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: healthPath, Port: intstr.FromInt32(port)},
			},
			InitialDelaySeconds: 3, PeriodSeconds: 5, FailureThreshold: 6,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{Path: healthPath, Port: intstr.FromInt32(port)},
			},
			InitialDelaySeconds: 10, PeriodSeconds: 15, FailureThreshold: 4,
		},
		VolumeMounts: []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}},
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: spec.Namespace, Labels: appLabels(spec)},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &automountSA,
					ImagePullSecrets:             tenantImagePullSecrets(),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &runAsNonRoot,
						RunAsUser:      &runAsUser,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{container},
					Volumes: []corev1.Volume{{
						Name: "tmp",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: &tmpSize,
						}},
					}},
				},
			},
		},
	}

	api := p.cs.AppsV1().Deployments(spec.Namespace)
	existing, err := api.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, dep, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Deployment")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Deployment: %w", err)
	}
	dep.ResourceVersion = existing.ResourceVersion
	if _, uerr := api.Update(ctx, dep, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar Deployment: %w", uerr)
	}
	return nil
}

// applyService aplica el Service ClusterIP de la app.
func (p *KubePlatform) applyService(ctx context.Context, spec AppSpec, name string) error {
	port := clampPort(spec.Port)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: spec.Namespace, Labels: appLabels(spec)},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"sytue.eu/deployment": spec.DeploymentID},
			Ports: []corev1.ServicePort{{
				Name: "http", Port: 80, TargetPort: intstr.FromInt32(port), Protocol: corev1.ProtocolTCP,
			}},
		},
	}
	api := p.cs.CoreV1().Services(spec.Namespace)
	existing, err := api.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, svc, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Service")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar Service: %w", err)
	}
	svc.ResourceVersion = existing.ResourceVersion
	svc.Spec.ClusterIP = existing.Spec.ClusterIP // ClusterIP es inmutable
	if _, uerr := api.Update(ctx, svc, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar Service: %w", uerr)
	}
	return nil
}

// Ruta y destino de la medición en primera parte (ADR 0026). El prefijo empieza
// por «_» para no chocar con rutas reales de una app de cliente.
const (
	analyticsPathPrefix  = "/_sytue"
	analyticsServiceName = "sytue-analytics"
	analyticsIngestPort  = 8094
)

// ensureAnalyticsAlias crea en el namespace del tenant el Service ExternalName que
// apunta al colector de la plataforma. Idempotente.
func (p *KubePlatform) ensureAnalyticsAlias(ctx context.Context, namespace string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      analyticsServiceName,
			Namespace: namespace,
			Labels:    map[string]string{"sytue.eu/component": "analytics-alias"},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: p.cfg.AnalyticsIngestHost,
			Ports:        []corev1.ServicePort{{Port: analyticsIngestPort, TargetPort: intstr.FromInt32(analyticsIngestPort)}},
		},
	}
	api := p.cs.CoreV1().Services(namespace)
	existing, err := api.Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, svc, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear alias de analytics")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar alias de analytics: %w", err)
	}
	svc.ResourceVersion = existing.ResourceVersion
	// ClusterIP vacío a propósito: un ExternalName no lo tiene y copiarlo del
	// anterior (si alguien cambió el tipo) haría fallar el Update.
	if _, uerr := api.Update(ctx, svc, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar alias de analytics: %w", uerr)
	}
	return nil
}

// applyIngress aplica el Ingress (Traefik) que enruta los hostnames de la app a
// su Service, con TLS gestionado por cert-manager.
func (p *KubePlatform) applyIngress(ctx context.Context, spec AppSpec, name string) error {
	pathType := networkingv1.PathTypePrefix
	className := "traefik"

	// Medición en PRIMERA PARTE (ADR 0026): con el colector configurado, cada app
	// sirve /_sytue en SU PROPIO dominio. Eso es lo que hace que el sitio se
	// identifique por la cabecera Host, así que el script no lleva identificador
	// alguno y la clase entera de «inflar los números de otro» deja de existir. De
	// paso no hay CORS, no hay un dominio que un bloqueador pueda vetar y el
	// cliente no toca su CSP.
	//
	// La ruta va ANTES que "/" en la lista y es más específica, así que Traefik la
	// prefiere. NO decide quién mide: eso es la fila en analytics_sites, y el
	// colector responde sin guardar nada a un Host que no esté dado de alta. Aquí
	// solo se abre el camino.
	analytics := p.cfg.AnalyticsIngestHost != ""
	rules := make([]networkingv1.IngressRule, 0, len(spec.Hostnames))
	for _, h := range spec.Hostnames {
		if h == "" {
			continue
		}
		paths := make([]networkingv1.HTTPIngressPath, 0, 2)
		if analytics {
			paths = append(paths, networkingv1.HTTPIngressPath{
				Path:     analyticsPathPrefix,
				PathType: &pathType,
				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: analyticsServiceName,
						Port: networkingv1.ServiceBackendPort{Number: analyticsIngestPort},
					},
				},
			})
		}
		paths = append(paths, networkingv1.HTTPIngressPath{
			Path:     "/",
			PathType: &pathType,
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: name,
					Port: networkingv1.ServiceBackendPort{Number: 80},
				},
			},
		})
		rules = append(rules, networkingv1.IngressRule{
			Host:             h,
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: paths}},
		})
	}

	// El backend de un Ingress tiene que vivir en SU MISMO namespace, y el colector
	// está en el de plataforma: el puente es un Service ExternalName, que es solo un
	// CNAME interno (no abre red por sí mismo).
	if analytics {
		if err := p.ensureAnalyticsAlias(ctx, spec.Namespace); err != nil {
			return err
		}
	}

	tlsBlocks := make([]networkingv1.IngressTLS, 0, len(spec.Hostnames))
	for _, h := range spec.Hostnames {
		if h == "" {
			continue
		}
		tlsBlocks = append(tlsBlocks, networkingv1.IngressTLS{
			Hosts:      []string{h},
			SecretName: "tls-" + dnsSafe(h),
		})
	}

	annotations := map[string]string{
		"traefik.ingress.kubernetes.io/router.entrypoints": "websecure",
		"traefik.ingress.kubernetes.io/router.tls":         "true",
		"cert-manager.io/cluster-issuer":                   p.cfg.CertIssuer,
	}
	// Cadena de middlewares del router, EN ORDEN:
	//  1. la contraseña de la preview (si no pasas, no hay nada más que discutir),
	//  2. las redirecciones (si la petición se va, tampoco),
	//  3. las cabeceras, que se añaden a la respuesta que sí se sirve.
	//
	// `ensurePreviewAuth` devuelve vacío en producción: cerrarla con una
	// contraseña básica sería tirar el sitio del cliente.
	chain := make([]string, 0, 4)
	authRef, err := p.ensurePreviewAuth(ctx, spec)
	if err != nil {
		return err
	}
	if authRef != "" {
		chain = append(chain, authRef)
	}
	ruleRefs, err := p.ensureHTTPRules(ctx, spec)
	if err != nil {
		return err
	}
	chain = append(chain, ruleRefs...)
	if len(chain) > 0 {
		annotations["traefik.ingress.kubernetes.io/router.middlewares"] = strings.Join(chain, ",")
	}

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   spec.Namespace,
			Labels:      appLabels(spec),
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &className,
			Rules:            rules,
			TLS:              tlsBlocks,
		},
	}

	api := p.cs.NetworkingV1().Ingresses(spec.Namespace)
	existing, gerr := api.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(gerr) {
		_, cerr := api.Create(ctx, ing, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear Ingress")
	}
	if gerr != nil {
		return fmt.Errorf("controller: consultar Ingress: %w", gerr)
	}
	ing.ResourceVersion = existing.ResourceVersion
	if _, uerr := api.Update(ctx, ing, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar Ingress: %w", uerr)
	}
	return nil
}

// applyHPA aplica un HorizontalPodAutoscaler para producción (escala por CPU).
//
// El techo y el objetivo llegan resueltos en la spec (configuración del proyecto,
// migración 0031). Si vienen a cero —specs construidas a mano por otros caminos
// internos o por los tests— se cae a la regla histórica en vez de crear un HPA
// que no escala: un maxReplicas 0 lo rechazaría el API server y dejaría la app
// sin ningún objeto aplicado.
func (p *KubePlatform) applyHPA(ctx context.Context, spec AppSpec, name string) error {
	minR := clampReplicas(spec.Replicas)
	maxR := clampReplicas(spec.AutoscaleMax)
	if spec.AutoscaleMax <= 0 {
		maxR = minR * 5
	}
	if maxR < minR {
		maxR = minR
	}
	targetCPU := int32(domain.DefaultCPUTarget)
	if spec.AutoscaleCPUTarget >= domain.MinCPUTarget && spec.AutoscaleCPUTarget <= domain.MaxCPUTarget {
		targetCPU = int32(spec.AutoscaleCPUTarget)
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: spec.Namespace, Labels: appLabels(spec)},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1", Kind: "Deployment", Name: name,
			},
			MinReplicas: &minR,
			MaxReplicas: maxR,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: &targetCPU,
					},
				},
			}},
		},
	}

	api := p.cs.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace)
	existing, err := api.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, cerr := api.Create(ctx, hpa, metav1.CreateOptions{})
		return ignoreAlreadyExists(cerr, "crear HPA")
	}
	if err != nil {
		return fmt.Errorf("controller: consultar HPA: %w", err)
	}
	hpa.ResourceVersion = existing.ResourceVersion
	if _, uerr := api.Update(ctx, hpa, metav1.UpdateOptions{}); uerr != nil {
		return fmt.Errorf("controller: actualizar HPA: %w", uerr)
	}
	return nil
}

// deploymentReady indica si todas las réplicas deseadas están disponibles.
func (p *KubePlatform) deploymentReady(ctx context.Context, namespace, name string) (bool, error) {
	dep, err := p.cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("controller: estado Deployment: %w", err)
	}
	want := int32(1)
	if dep.Spec.Replicas != nil {
		want = *dep.Spec.Replicas
	}
	return dep.Status.AvailableReplicas >= want && want > 0, nil
}

// challengeLabel es el subdominio donde vive el reto de propiedad, tal y como lo
// publica la API al añadir el dominio (handlers.AddDomain → dns.txt_record).
const challengeLabel = "_sytue-challenge."

// VerifyDomain comprueba la PROPIEDAD del dominio exigiendo el registro TXT
// `_sytue-challenge.<host>` con el token secreto emitido para ESE dominio.
// Implementa Platform.
//
// Antes bastaba con que el hostname apuntara al ingress (CNAME/A). Eso NO es
// prueba de propiedad, sino de enrutado, y abría un secuestro de dominio por DNS
// colgante: si un cliente apunta su CNAME y luego retira el dominio de Sytue (o lo
// configura antes de reclamarlo), cualquier otro tenant podía registrar ese
// hostname, pasar la verificación con el CNAME ajeno y obtener un certificado
// válido sirviendo SU aplicación en el dominio de la víctima. La API ya instruía a
// crear el TXT, pero el token que recibe esta función no se llegaba a usar.
//
// Solo el control de la zona DNS permite publicar ese TXT, así que es la prueba de
// propiedad. El enrutado no se comprueba aquí: es requisito de la EMISIÓN del
// certificado (el ClusterIssuer usa HTTP-01, que exige que el host resuelva a
// nuestro ingress), de modo que un dominio verificado pero sin apuntar se queda en
// `verified` reintentando el TLS, sin conceder nada.
func (p *KubePlatform) VerifyDomain(ctx context.Context, hostname, token, _ string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		// Sin token no hay reto que satisfacer: fail-closed (nunca se verifica solo).
		return false, nil
	}
	records, err := p.resolver.LookupTXT(ctx, challengeLabel+strings.TrimSuffix(hostname, "."))
	if err != nil {
		// NXDOMAIN o DNS sin propagar todavía: sigue pendiente, no es un fallo duro.
		return false, nil //nolint:nilerr // se reintenta en el siguiente ciclo.
	}
	for _, rec := range records {
		// Algunos resolvers/proveedores devuelven el valor entrecomillado.
		if subtle.ConstantTimeCompare(
			[]byte(strings.Trim(strings.TrimSpace(rec), `"`)), []byte(token),
		) == 1 {
			return true, nil
		}
	}
	return false, nil
}

// EnsureTLS aplica el Certificate de cert-manager para el hostname y devuelve si
// ya está emitido (el certificado sirve tráfico TLS válido). Implementa Platform.
func (p *KubePlatform) EnsureTLS(ctx context.Context, req TLSRequest) (bool, error) {
	namespace := "tenant-" + req.Tenant
	certName := "cert-" + dnsSafe(req.Hostname)
	secretName := "tls-" + dnsSafe(req.Hostname)

	cert := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]any{
				"name":      certName,
				"namespace": namespace,
				"labels": map[string]any{
					"app.kubernetes.io/part-of": "sytue-one",
					"sytue.eu/tenant":           req.Tenant,
				},
			},
			"spec": map[string]any{
				"secretName": secretName,
				"dnsNames":   []any{req.Hostname},
				"issuerRef": map[string]any{
					"name": req.Issuer,
					"kind": "ClusterIssuer",
				},
			},
		},
	}

	api := p.dyn.Resource(certManagerGVR).Namespace(namespace)
	existing, err := api.Get(ctx, certName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, cerr := api.Create(ctx, cert, metav1.CreateOptions{}); cerr != nil && !apierrors.IsAlreadyExists(cerr) {
			return false, fmt.Errorf("controller: crear Certificate: %w", cerr)
		}
		return false, nil // recién creado; aún emitiéndose
	}
	if err != nil {
		return false, fmt.Errorf("controller: consultar Certificate: %w", err)
	}

	// El Secret TLS existe => cert-manager ya emitió el certificado.
	if _, serr := p.cs.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{}); serr == nil {
		// Confirmación adicional: el hostname sirve TLS válido.
		if p.httpProbe == nil || p.httpProbe(ctx, req.Hostname) {
			return true, nil
		}
	}

	// Mantén el Certificate al día (idempotente).
	cert.SetResourceVersion(existing.GetResourceVersion())
	if _, uerr := api.Update(ctx, cert, metav1.UpdateOptions{}); uerr != nil {
		return false, fmt.Errorf("controller: actualizar Certificate: %w", uerr)
	}
	return certReady(existing), nil
}

// RemoveDomainTLS elimina Certificate y Secret TLS de un hostname. Implementa Platform.
func (p *KubePlatform) RemoveDomainTLS(ctx context.Context, tenant, hostname string) error {
	namespace := "tenant-" + tenant
	certName := "cert-" + dnsSafe(hostname)
	secretName := "tls-" + dnsSafe(hostname)
	bg := metav1.DeletePropagationBackground
	opts := metav1.DeleteOptions{PropagationPolicy: &bg}
	_ = p.dyn.Resource(certManagerGVR).Namespace(namespace).Delete(ctx, certName, opts)
	if err := p.cs.CoreV1().Secrets(namespace).Delete(ctx, secretName, opts); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("controller: eliminar secret TLS: %w", err)
	}
	return nil
}

// certReady inspecciona el estado del Certificate (condición Ready=True).
func certReady(u *unstructured.Unstructured) bool {
	conds, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conds {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if cm["type"] == "Ready" && cm["status"] == "True" {
			return true
		}
	}
	return false
}

// defaultTLSProbe verifica que el hostname acepta una conexión TLS con
// certificado válido (puerto 443).
func defaultTLSProbe(ctx context.Context, hostname string) bool {
	d := &tls.Dialer{Config: &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}}
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := d.DialContext(dctx, "tcp", net.JoinHostPort(hostname, "443"))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func appResourceName(spec AppSpec) string {
	return "app-" + shortID(spec.DeploymentID)
}

func appLabels(spec AppSpec) map[string]string {
	base := map[string]string{
		"app.kubernetes.io/part-of":    "sytue-one",
		"app.kubernetes.io/managed-by": "sytue-cloud",
		"sytue.eu/component":           "cloud-app",
		"sytue.eu/tenant":              spec.Tenant,
		"sytue.eu/project":             spec.ProjectSlug,
		"sytue.eu/deployment":          spec.DeploymentID,
		"sytue.eu/target":              string(spec.Target),
	}
	// Clave de emparejamiento con los servicios gestionados del mismo proyecto
	// (allow-project-<id>). Vacía en llamadas antiguas/tests: mejor sin etiqueta
	// que con una etiqueta vacía que casaría con selectores ajenos.
	if spec.ProjectID != "" {
		base[projectLabel] = spec.ProjectID
	}
	for k, v := range spec.Labels {
		base[k] = v
	}
	return base
}

// dnsSafe convierte un hostname en un nombre de recurso DNS-1123 seguro.
// clampReplicas acota las réplicas al rango operativo [1, 512]; evita el
// overflow int→int32 (gosec G115) con specs corruptas o hostiles.
func clampReplicas(n int) int32 {
	switch {
	case n < 1:
		return 1
	case n > 512:
		return 512
	default:
		return int32(n)
	}
}

// clampPort acota un puerto TCP declarado en la spec al rango válido; fuera de
// [1, 65535] se usa el puerto de contenedor por defecto.
func clampPort(n int) int32 {
	if n < 1 || n > 65535 {
		return appContainerPort
	}
	return int32(n)
}

func dnsSafe(h string) string {
	out := make([]byte, 0, len(h))
	for i := 0; i < len(h); i++ {
		ch := h[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9':
			out = append(out, ch)
		case ch >= 'A' && ch <= 'Z':
			out = append(out, ch+('a'-'A'))
		default:
			out = append(out, '-')
		}
	}
	s := strings.Trim(string(out), "-")
	if len(s) > 253 {
		s = s[:253]
	}
	if s == "" {
		s = "host"
	}
	return s
}

func ignoreAlreadyExists(err error, action string) error {
	if err == nil || apierrors.IsAlreadyExists(err) {
		return nil
	}
	return fmt.Errorf("controller: %s: %w", action, err)
}
