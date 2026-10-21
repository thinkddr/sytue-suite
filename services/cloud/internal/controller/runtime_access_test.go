// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

func runtimeAccessConfig() config.Config {
	cfg := testCfg()
	cfg.CloudAPIClusterRole = "sytue-one-cloud-api"
	cfg.CloudAPIServiceAccount = "cloud-api-custom"
	cfg.CloudAPINamespace = "sytue-platform"
	return cfg
}

func managedTenantNamespace(tenant string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "tenant-" + tenant,
		Labels: map[string]string{
			"app.kubernetes.io/part-of": "sytue-one",
			"sytue.eu/namespace-kind":   "tenant",
			"sytue.eu/tenant":           tenant,
		},
	}}
}

func assertNoKubeWrites(t *testing.T, cs *fake.Clientset) {
	t.Helper()
	for _, action := range cs.Actions() {
		if action.GetVerb() != "get" && action.GetVerb() != "list" {
			t.Errorf("no debe escribir %s %s en %s", action.GetVerb(), action.GetResource().Resource, action.GetNamespace())
		}
	}
}

func TestCloudRuntimeAccessConfigurationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*config.Config)
		wantErr bool
	}{
		{"completa", func(*config.Config) {}, false},
		{"falta rol", func(c *config.Config) { c.CloudAPIClusterRole = "" }, true},
		{"falta cuenta", func(c *config.Config) { c.CloudAPIServiceAccount = "" }, true},
		{"falta namespace", func(c *config.Config) { c.CloudAPINamespace = "" }, true},
		{"rol inválido", func(c *config.Config) { c.CloudAPIClusterRole = "otro/rol" }, true},
		{"cuenta inválida", func(c *config.Config) { c.CloudAPIServiceAccount = "otra/cuenta" }, true},
		{"namespace inválido", func(c *config.Config) { c.CloudAPINamespace = "OtroNamespace" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := runtimeAccessConfig()
			tc.mutate(&cfg)
			enabled, err := cloudRuntimeAccessEnabled(cfg)
			wantEnabled := !tc.wantErr
			if (err != nil) != tc.wantErr || enabled != wantEnabled {
				t.Fatalf("enabled=%v, err=%v; wantErr=%v", enabled, err, tc.wantErr)
			}
		})
	}
	for _, env := range []string{"", "dev", "staging", "prod"} {
		t.Run("sin identidad en "+env, func(t *testing.T) {
			cfg := testCfg()
			cfg.Env = pcfg.Environment(env)
			enabled, err := cloudRuntimeAccessEnabled(cfg)
			wantErr := env == "prod" || env == "staging"
			if enabled || (err != nil) != wantErr {
				t.Fatalf("enabled=%v, err=%v; wantErr=%v", enabled, err, wantErr)
			}
		})
	}
	cfg := testCfg()
	cfg.Env = "prod"
	cfg.KubeconfigPath = "/no-existe/kubeconfig"
	if _, err := NewKubePlatform(cfg); err == nil || !strings.Contains(err.Error(), "CLOUD_API_CLUSTER_ROLE") {
		t.Fatalf("la identidad incompleta debe impedir construir la plataforma antes de leer kubeconfig: %v", err)
	}
}

func TestRuntimeAccessDoesNotSilentlySkipPartialConfig(t *testing.T) {
	p, cs := newTestPlatform()
	if err := p.ReconcileTenantRuntimeAccess(context.Background()); err != nil {
		t.Fatalf("el fake de dev sin identidad conserva el no-op: %v", err)
	}
	p.cfg.CloudAPIClusterRole = "rol-parcial"
	if err := p.ReconcileTenantRuntimeAccess(context.Background()); err == nil {
		t.Fatal("una identidad parcial debe fallar incluso en dev")
	}
	if len(cs.Actions()) != 0 {
		t.Fatalf("no debe consultar ni escribir con identidad incompleta: %+v", cs.Actions())
	}
}

func TestEnsureNamespaceCreatesTenantScopedRuntimeAccess(t *testing.T) {
	p, cs := newTestPlatform()
	p.cfg = runtimeAccessConfig()
	ctx := context.Background()
	if err := p.EnsureNamespace(ctx, "tenant-acme", "acme", domain.QuotasFor(domain.PlanFree)); err != nil {
		t.Fatal(err)
	}
	binding, err := cs.RbacV1().RoleBindings("tenant-acme").Get(ctx, cloudRuntimeBinding, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantRole := rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: p.cfg.CloudAPIClusterRole}
	wantSubjects := []rbacv1.Subject{{Kind: "ServiceAccount", Name: p.cfg.CloudAPIServiceAccount, Namespace: p.cfg.CloudAPINamespace}}
	if binding.RoleRef != wantRole || !slices.Equal(binding.Subjects, wantSubjects) {
		t.Fatalf("el binding debe usar la identidad configurada y solo su rol: %+v", binding)
	}
	for _, action := range cs.Actions() {
		if action.GetResource().Resource == "clusterrolebindings" {
			t.Fatal("el controller no debe escribir ni consultar un ClusterRoleBinding")
		}
		if action.GetResource().Resource == "rolebindings" && action.GetNamespace() != "tenant-acme" {
			t.Fatalf("binding fuera del tenant: %+v", action)
		}
	}
}

func TestEnsureNamespaceRejectsInvalidTenantBeforeAnyAPIRequest(t *testing.T) {
	for _, tc := range []struct{ namespace, tenant string }{
		{"kube-system", "acme"},
		{"tenant-otra", "acme"},
		{"tenant-acme", ""},
		{"tenant-Acme", "Acme"},
		{"tenant-acme/otra", "acme/otra"},
		{"tenant-" + strings.Repeat("a", 57), strings.Repeat("a", 57)},
	} {
		t.Run(tc.namespace, func(t *testing.T) {
			p, cs := newTestPlatform()
			p.cfg = runtimeAccessConfig()
			if err := p.EnsureNamespace(context.Background(), tc.namespace, tc.tenant, domain.PlanQuota{}); err == nil {
				t.Fatal("debe rechazar namespace/tenant inválidos")
			}
			if len(cs.Actions()) != 0 {
				t.Fatalf("la validación debe preceder cualquier petición API: %+v", cs.Actions())
			}
		})
	}
	p, cs := newTestPlatform()
	p.cfg.CloudAPIClusterRole = "rol-parcial"
	if err := p.EnsureNamespace(context.Background(), "tenant-acme", "acme", domain.PlanQuota{}); err == nil {
		t.Fatal("la configuración parcial no puede omitir el binding")
	}
	if len(cs.Actions()) != 0 {
		t.Fatalf("la configuración parcial no debe crear el namespace: %+v", cs.Actions())
	}
}

func TestEnsureNamespaceDoesNotAdoptForeignNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*corev1.Namespace)
	}{
		{"sin etiquetas", func(ns *corev1.Namespace) { ns.Labels = nil }},
		{"otra aplicación", func(ns *corev1.Namespace) { ns.Labels["app.kubernetes.io/part-of"] = "otra" }},
		{"otro tipo", func(ns *corev1.Namespace) { ns.Labels["sytue.eu/namespace-kind"] = "platform" }},
		{"otro tenant", func(ns *corev1.Namespace) { ns.Labels["sytue.eu/tenant"] = "otra" }},
		{"eliminándose", func(ns *corev1.Namespace) { now := metav1.Now(); ns.DeletionTimestamp = &now }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, cs := newTestPlatform()
			p.cfg = runtimeAccessConfig()
			ns := managedTenantNamespace("acme")
			tc.mutate(ns)
			if err := cs.Tracker().Add(ns); err != nil {
				t.Fatal(err)
			}
			if err := p.EnsureNamespace(context.Background(), ns.Name, "acme", domain.PlanQuota{}); err == nil {
				t.Fatal("no debe adoptar un namespace ajeno o en eliminación")
			}
			assertNoKubeWrites(t, cs)
		})
	}
}

func TestEnsureNamespaceVerifiesOwnershipAfterConcurrentCreation(t *testing.T) {
	p, cs := newTestPlatform()
	p.cfg = runtimeAccessConfig()
	cs.PrependReactor("create", "namespaces", func(ktesting.Action) (bool, runtime.Object, error) {
		foreign := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-acme"}}
		if err := cs.Tracker().Add(foreign); err != nil {
			t.Fatal(err)
		}
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "namespaces"}, foreign.Name)
	})
	if err := p.EnsureNamespace(context.Background(), "tenant-acme", "acme", domain.PlanQuota{}); err == nil {
		t.Fatal("AlreadyExists no prueba la propiedad del namespace")
	}
	for _, action := range cs.Actions() {
		if action.GetResource().Resource != "namespaces" {
			t.Fatalf("no debe añadir objetos a un namespace ajeno: %+v", action)
		}
	}
}

func TestRuntimeAccessAcceptsConcurrentCorrectBinding(t *testing.T) {
	p, cs := newTestPlatform()
	p.cfg = runtimeAccessConfig()
	if err := cs.Tracker().Add(managedTenantNamespace("acme")); err != nil {
		t.Fatal(err)
	}
	cs.PrependReactor("create", "rolebindings", func(action ktesting.Action) (bool, runtime.Object, error) {
		binding := action.(ktesting.CreateAction).GetObject().(*rbacv1.RoleBinding)
		if err := cs.Tracker().Add(binding.DeepCopy()); err != nil {
			t.Fatal(err)
		}
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: rbacv1.GroupName, Resource: "rolebindings"}, cloudRuntimeBinding)
	})
	if err := p.ReconcileTenantRuntimeAccess(context.Background()); err != nil {
		t.Fatalf("una carrera con el binding correcto debe converger: %v", err)
	}
	for _, action := range cs.Actions() {
		if action.GetVerb() == "update" || action.GetVerb() == "delete" || action.GetVerb() == "patch" {
			t.Fatalf("el binding concurrente correcto no necesita reparación: %+v", action)
		}
	}
}

func TestRuntimeAccessBackfillsManagedTenantsAndThenOnlyReads(t *testing.T) {
	p, cs := newTestPlatform()
	p.cfg = runtimeAccessConfig()
	terminating := managedTenantNamespace("retirado")
	now := metav1.Now()
	terminating.DeletionTimestamp = &now
	for _, ns := range []*corev1.Namespace{
		managedTenantNamespace("heredado"), managedTenantNamespace("acme"), terminating,
		{ObjectMeta: metav1.ObjectMeta{Name: "tenant-ajeno"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", Labels: map[string]string{
			"app.kubernetes.io/part-of": "sytue-one", "sytue.eu/namespace-kind": "platform",
		}}},
	} {
		if err := cs.Tracker().Add(ns); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if err := p.ReconcileTenantRuntimeAccess(ctx); err != nil {
		t.Fatal(err)
	}
	bindings, err := cs.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 2 {
		t.Fatalf("solo debe migrar los dos tenants activos y gestionados: %+v", bindings.Items)
	}
	for _, binding := range bindings.Items {
		if binding.Namespace != "tenant-heredado" && binding.Namespace != "tenant-acme" {
			t.Fatalf("binding fuera de los tenants gestionados: %+v", binding)
		}
	}
	cs.ClearActions()
	if err := p.ReconcileTenantRuntimeAccess(ctx); err != nil {
		t.Fatal(err)
	}
	assertNoKubeWrites(t, cs)
}

func TestRuntimeAccessRepairsSubjectsWithoutChangingRole(t *testing.T) {
	p, cs := newTestPlatform()
	p.cfg = runtimeAccessConfig()
	ns := managedTenantNamespace("acme")
	if err := cs.Tracker().Add(ns); err != nil {
		t.Fatal(err)
	}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: cloudRuntimeBinding, Namespace: ns.Name, ResourceVersion: "42", Labels: map[string]string{"operator": "conservar"}},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: p.cfg.CloudAPIClusterRole},
		Subjects:   []rbacv1.Subject{{Kind: "Group", APIGroup: rbacv1.GroupName, Name: "system:authenticated"}},
	}
	if err := cs.Tracker().Add(binding); err != nil {
		t.Fatal(err)
	}
	if err := p.ReconcileTenantRuntimeAccess(context.Background()); err != nil {
		t.Fatal(err)
	}
	repaired, err := cs.RbacV1().RoleBindings(ns.Name).Get(context.Background(), cloudRuntimeBinding, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []rbacv1.Subject{{Kind: "ServiceAccount", Name: p.cfg.CloudAPIServiceAccount, Namespace: p.cfg.CloudAPINamespace}}
	if !slices.Equal(repaired.Subjects, want) || repaired.RoleRef != binding.RoleRef || repaired.ResourceVersion != "42" {
		t.Fatalf("reparación no acotada a la identidad nominal: %+v", repaired)
	}
	if repaired.Labels["operator"] != "conservar" || repaired.Labels["sytue.eu/tenant"] != "acme" {
		t.Fatalf("debe conservar etiquetas ajenas y fijar el tenant: %v", repaired.Labels)
	}
	for _, action := range cs.Actions() {
		if action.GetVerb() == "delete" {
			t.Fatal("la reparación no debe borrar bindings")
		}
	}
}

func TestRuntimeAccessFailsClosedForRoleDriftIncludingCreationRace(t *testing.T) {
	for _, race := range []bool{false, true} {
		t.Run(map[bool]string{false: "existente", true: "creación concurrente"}[race], func(t *testing.T) {
			p, cs := newTestPlatform()
			p.cfg = runtimeAccessConfig()
			ns := managedTenantNamespace("acme")
			if err := cs.Tracker().Add(ns); err != nil {
				t.Fatal(err)
			}
			foreign := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: cloudRuntimeBinding, Namespace: ns.Name},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
			}
			if race {
				cs.PrependReactor("create", "rolebindings", func(ktesting.Action) (bool, runtime.Object, error) {
					if err := cs.Tracker().Add(foreign); err != nil {
						t.Fatal(err)
					}
					return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: rbacv1.GroupName, Resource: "rolebindings"}, cloudRuntimeBinding)
				})
			} else if err := cs.Tracker().Add(foreign); err != nil {
				t.Fatal(err)
			}
			if err := p.ReconcileTenantRuntimeAccess(context.Background()); err == nil || !strings.Contains(err.Error(), "roleRef") {
				t.Fatalf("un roleRef ajeno debe propagarse sin sustituir el objeto: %v", err)
			}
			for _, action := range cs.Actions() {
				if action.GetVerb() == "delete" || action.GetVerb() == "update" || action.GetVerb() == "patch" {
					t.Fatalf("no debe ampliar permisos intentando reparar roleRef: %+v", action)
				}
			}
		})
	}
}

func TestRuntimeAccessPropagatesAPIErrors(t *testing.T) {
	denied := apierrors.NewForbidden(schema.GroupResource{Group: rbacv1.GroupName, Resource: "rolebindings"}, cloudRuntimeBinding, errors.New("denegado"))
	invalid := apierrors.NewInvalid(schema.GroupKind{Group: rbacv1.GroupName, Kind: "RoleBinding"}, cloudRuntimeBinding,
		field.ErrorList{field.Invalid(field.NewPath("subjects"), "sujeto", "rechazado")})
	for _, tc := range []struct {
		verb     string
		resource string
		err      error
	}{
		{"list", "namespaces", denied},
		{"get", "rolebindings", denied},
		{"create", "rolebindings", denied},
		{"update", "rolebindings", invalid},
	} {
		t.Run(tc.verb+" "+tc.resource, func(t *testing.T) {
			p, cs := newTestPlatform()
			p.cfg = runtimeAccessConfig()
			if err := cs.Tracker().Add(managedTenantNamespace("acme")); err != nil {
				t.Fatal(err)
			}
			if tc.verb == "update" {
				if err := cs.Tracker().Add(&rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{Name: cloudRuntimeBinding, Namespace: "tenant-acme"},
					RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: p.cfg.CloudAPIClusterRole},
				}); err != nil {
					t.Fatal(err)
				}
			}
			cs.PrependReactor(tc.verb, tc.resource, func(ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, tc.err
			})
			if err := p.ReconcileTenantRuntimeAccess(context.Background()); !errors.Is(err, tc.err) {
				t.Fatalf("debe propagar el error original para reintentar: %v", err)
			}
			for _, action := range cs.Actions() {
				if action.GetVerb() == "delete" {
					t.Fatal("un error no debe disparar borrado como alternativa")
				}
			}
		})
	}
}

func TestRuntimeAccessContinuesOtherTenantsAfterErrors(t *testing.T) {
	p, cs := newTestPlatform()
	p.cfg = runtimeAccessConfig()
	malformed := managedTenantNamespace("malo")
	malformed.Name = "tenant-no-corresponde"
	for _, ns := range []*corev1.Namespace{managedTenantNamespace("acme"), managedTenantNamespace("beta"), malformed} {
		if err := cs.Tracker().Add(ns); err != nil {
			t.Fatal(err)
		}
	}
	denied := apierrors.NewForbidden(schema.GroupResource{Group: rbacv1.GroupName, Resource: "rolebindings"}, cloudRuntimeBinding, errors.New("denegado"))
	cs.PrependReactor("get", "rolebindings", func(action ktesting.Action) (bool, runtime.Object, error) {
		return action.GetNamespace() == "tenant-acme", nil, denied
	})
	err := p.ReconcileTenantRuntimeAccess(context.Background())
	if !errors.Is(err, denied) || !strings.Contains(err.Error(), malformed.Name) {
		t.Fatalf("debe devolver tanto el error API como el namespace inválido: %v", err)
	}
	if _, err := cs.RbacV1().RoleBindings("tenant-beta").Get(context.Background(), cloudRuntimeBinding, metav1.GetOptions{}); err != nil {
		t.Fatalf("el tenant sano debe migrarse aunque otros fallen: %v", err)
	}
	for _, action := range cs.Actions() {
		if action.GetResource().Resource == "rolebindings" && action.GetNamespace() == malformed.Name {
			t.Fatal("el namespace con propiedad inconsistente no debe recibir binding")
		}
	}
}

type runtimeBackfillPlatform struct {
	StubPlatform
	calls int
	err   error
}

func (p *runtimeBackfillPlatform) ReconcileTenantRuntimeAccess(context.Context) error {
	p.calls++
	return p.err
}

func TestReconcileRepairsRuntimeAccessWithoutPendingDeployments(t *testing.T) {
	for _, fail := range []bool{false, true} {
		plat := &runtimeBackfillPlatform{}
		if fail {
			plat.err = errors.New("RBAC no disponible")
		}
		c := newTestController(&fakeCtrlStore{}, plat, NewStubProvisioner(testCfg()))
		l := &capLogger{}
		c.reconcile(context.Background(), l)
		if plat.calls != 1 || l.warned("reconciliar acceso runtime de tenants") != fail {
			t.Fatalf("el ciclo debe reparar namespaces existentes y registrar errores: calls=%d warns=%v", plat.calls, l.warns)
		}
	}
}
