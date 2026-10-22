// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/validate/content"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/config"
)

// Nombre fijo: el RBAC del controller limita get/update a estos RoleBindings.
const (
	cloudRuntimeBinding    = "sytue-cloud-runtime"
	cloudControllerBinding = "sytue-cloud-controller"
)

const tenantNamespaceSelector = "app.kubernetes.io/part-of=sytue-one,sytue.eu/namespace-kind=tenant"

// tenantRuntimeAccessReconciler es la reparación opcional de namespaces anteriores
// al aislamiento de cloud-api. Sigue postgresRoleRepairer: el stub no tiene clúster.
type tenantRuntimeAccessReconciler interface {
	ReconcileTenantRuntimeAccess(context.Context) error
}

var _ tenantRuntimeAccessReconciler = (*KubePlatform)(nil)

func (c *Controller) reconcileTenantRuntimeAccess(ctx context.Context) error {
	r, ok := c.platform.(tenantRuntimeAccessReconciler)
	if !ok {
		return nil
	}
	return r.ReconcileTenantRuntimeAccess(ctx)
}

// cloudRuntimeAccessEnabled no permite degradar una configuración parcial a no-op.
// Solo los fakes/stub de dev conservan el caso histórico sin las tres identidades.
func cloudRuntimeAccessEnabled(cfg config.Config) (bool, error) {
	if cfg.CloudAPIClusterRole == "" && cfg.CloudAPIServiceAccount == "" && cfg.CloudAPINamespace == "" &&
		(cfg.Env == "" || cfg.Env == pcfg.EnvDev) {
		return false, nil
	}
	if cfg.CloudAPIClusterRole == "" || cfg.CloudAPIServiceAccount == "" || cfg.CloudAPINamespace == "" {
		return false, fmt.Errorf("controller: CLOUD_API_CLUSTER_ROLE, CLOUD_API_SERVICE_ACCOUNT y CLOUD_API_NAMESPACE deben estar completos")
	}
	if problemas := content.IsPathSegmentName(cfg.CloudAPIClusterRole); len(problemas) > 0 {
		return false, fmt.Errorf("controller: CLOUD_API_CLUSTER_ROLE inválido: %v", problemas)
	}
	if problemas := validation.IsDNS1123Subdomain(cfg.CloudAPIServiceAccount); len(problemas) > 0 {
		return false, fmt.Errorf("controller: CLOUD_API_SERVICE_ACCOUNT inválido: %v", problemas)
	}
	if problemas := validation.IsDNS1123Label(cfg.CloudAPINamespace); len(problemas) > 0 {
		return false, fmt.Errorf("controller: CLOUD_API_NAMESPACE inválido: %v", problemas)
	}
	if _, err := cloudControllerAccessEnabled(cfg); err != nil {
		return false, err
	}
	return true, nil
}

func cloudControllerAccessEnabled(cfg config.Config) (bool, error) {
	if cfg.CloudControllerClusterRole == "" && cfg.CloudControllerServiceAccount == "" && cfg.CloudControllerNamespace == "" {
		return false, nil
	}
	if cfg.CloudControllerClusterRole == "" || cfg.CloudControllerServiceAccount == "" || cfg.CloudControllerNamespace == "" {
		return false, fmt.Errorf("controller: CLOUD_CONTROLLER_CLUSTER_ROLE, CLOUD_CONTROLLER_SERVICE_ACCOUNT y CLOUD_CONTROLLER_NAMESPACE deben estar completos")
	}
	if problemas := content.IsPathSegmentName(cfg.CloudControllerClusterRole); len(problemas) > 0 {
		return false, fmt.Errorf("controller: CLOUD_CONTROLLER_CLUSTER_ROLE inválido: %v", problemas)
	}
	if problemas := validation.IsDNS1123Subdomain(cfg.CloudControllerServiceAccount); len(problemas) > 0 {
		return false, fmt.Errorf("controller: CLOUD_CONTROLLER_SERVICE_ACCOUNT inválido: %v", problemas)
	}
	if problemas := validation.IsDNS1123Label(cfg.CloudControllerNamespace); len(problemas) > 0 {
		return false, fmt.Errorf("controller: CLOUD_CONTROLLER_NAMESPACE inválido: %v", problemas)
	}
	return true, nil
}

func validateTenantNamespace(namespace, tenant string) error {
	tnt, err := tenancy.New(tenant)
	if err != nil {
		return fmt.Errorf("controller: namespace de tenant: %w", err)
	}
	if namespace != tnt.Namespace() || len(validation.IsDNS1123Label(namespace)) > 0 {
		return fmt.Errorf("controller: namespace %q no corresponde al tenant %q", namespace, tenant)
	}
	return nil
}

// ownedTenantNamespace impide adoptar un namespace ajeno solo por su nombre.
func ownedTenantNamespace(ns *corev1.Namespace, tenant string) error {
	if err := validateTenantNamespace(ns.Name, tenant); err != nil {
		return err
	}
	if ns.Labels["app.kubernetes.io/part-of"] != "sytue-one" ||
		ns.Labels["sytue.eu/namespace-kind"] != "tenant" || ns.Labels["sytue.eu/tenant"] != tenant {
		return fmt.Errorf("controller: namespace %s no pertenece al tenant gestionado %s", ns.Name, tenant)
	}
	if ns.DeletionTimestamp != nil {
		return fmt.Errorf("controller: namespace %s se está eliminando", ns.Name)
	}
	return nil
}

// ReconcileTenantRuntimeAccess alcanza también tenants sin despliegues recientes.
// Un namespace roto no impide migrar los demás; todos los errores se propagan para
// que el ciclo registre y reintente. En régimen estable no escribe ningún objeto.
func (p *KubePlatform) ReconcileTenantRuntimeAccess(ctx context.Context) error {
	enabled, err := cloudRuntimeAccessEnabled(p.cfg)
	if err != nil || !enabled {
		return err
	}
	namespaces, err := p.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: tenantNamespaceSelector})
	if err != nil {
		return fmt.Errorf("controller: listar namespaces de tenant para acceso runtime: %w", err)
	}
	var errs []error
	for i := range namespaces.Items {
		ns := &namespaces.Items[i]
		if ns.DeletionTimestamp != nil {
			continue
		}
		if err := ownedTenantNamespace(ns, ns.Labels["sytue.eu/tenant"]); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := p.ensureCloudRuntimeBinding(ctx, ns.Name, ns.Labels["sytue.eu/tenant"]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *KubePlatform) ensureCloudRuntimeBinding(ctx context.Context, namespace, tenant string) error {
	if err := validateTenantNamespace(namespace, tenant); err != nil {
		return err
	}
	enabled, err := cloudRuntimeAccessEnabled(p.cfg)
	if err != nil || !enabled {
		return err
	}
	if err := p.ensureTenantRoleBinding(
		ctx, namespace, tenant,
		cloudRuntimeBinding, p.cfg.CloudAPIClusterRole, p.cfg.CloudAPIServiceAccount, p.cfg.CloudAPINamespace,
	); err != nil {
		return err
	}
	ctrlEnabled, err := cloudControllerAccessEnabled(p.cfg)
	if err != nil || !ctrlEnabled {
		return err
	}
	return p.ensureTenantRoleBinding(
		ctx, namespace, tenant,
		cloudControllerBinding, p.cfg.CloudControllerClusterRole, p.cfg.CloudControllerServiceAccount, p.cfg.CloudControllerNamespace,
	)
}

func (p *KubePlatform) ensureTenantRoleBinding(
	ctx context.Context, namespace, tenant, bindingName, clusterRole, saName, saNamespace string,
) error {
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName, Namespace: namespace, Labels: map[string]string{
			"app.kubernetes.io/managed-by": "sytue-cloud", "sytue.eu/tenant": tenant,
		}},
		RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: clusterRole},
		Subjects: []rbacv1.Subject{{
			Kind: "ServiceAccount", Name: saName,
			Namespace: saNamespace,
		}},
	}
	api := p.cs.RbacV1().RoleBindings(namespace)
	existing, err := api.Get(ctx, bindingName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = api.Create(ctx, desired, metav1.CreateOptions{})
		if !apierrors.IsAlreadyExists(err) {
			if err != nil {
				return fmt.Errorf("controller: crear acceso runtime de %s: %w", namespace, err)
			}
			return nil
		}
		// Una creación concurrente no autoriza a dar por bueno un binding ajeno.
		existing, err = api.Get(ctx, bindingName, metav1.GetOptions{})
	}
	if err != nil {
		return fmt.Errorf("controller: consultar acceso runtime de %s: %w", namespace, err)
	}
	if existing.RoleRef != desired.RoleRef {
		// roleRef es inmutable en Kubernetes. No pedimos delete ni intentamos
		// cambiar a ciegas un permiso: exige revisar el objeto en conflicto.
		return fmt.Errorf("controller: acceso runtime de %s tiene un roleRef ajeno: %+v", namespace, existing.RoleRef)
	}
	changed := !slices.Equal(existing.Subjects, desired.Subjects)
	for k, v := range desired.Labels {
		changed = changed || existing.Labels[k] != v
	}
	if !changed {
		return nil
	}
	updated := existing.DeepCopy()
	updated.Subjects = desired.Subjects
	if updated.Labels == nil {
		updated.Labels = make(map[string]string)
	}
	maps.Copy(updated.Labels, desired.Labels)
	if _, err := api.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("controller: reparar acceso runtime de %s: %w", namespace, err)
	}
	return nil
}
