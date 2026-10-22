// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ---------------------------------------------------------------------------
// Redirecciones y cabeceras en el borde
// ---------------------------------------------------------------------------
//
// Un Middleware de Traefik por regla, encadenados en el Ingress. Es lo que hace
// que una redirección funcione ANTES de llegar a la app —que es el punto: sirve
// también para un sitio estático, donde no hay código que la resuelva—.

// ensureHTTPRules materializa los middlewares de redirección y cabeceras del
// deployment y devuelve sus referencias EN ORDEN.
//
// El orden importa y es este: primero las redirecciones (si la petición se va,
// no hay nada más que hacer) y después las cabeceras (que se añaden a la
// respuesta que sí se sirve).
//
// Los objetos se nombran por deployment, así que un despliegue nuevo trae los
// suyos y `RetireApp` se los lleva: no hay estado compartido que limpiar aparte.
func (p *KubePlatform) ensureHTTPRules(ctx context.Context, spec AppSpec) ([]string, error) {
	rules := spec.HTTPRules
	if len(rules.Redirects) == 0 && len(rules.Headers) == 0 {
		return nil, nil
	}
	base := "rules-" + shortID(spec.DeploymentID)
	refs := make([]string, 0, len(rules.Redirects)+1)

	for i, rd := range rules.Redirects {
		pattern, replacement := domain.RedirectRegex(rd)
		name := fmt.Sprintf("%s-r%d", base, i)
		mw := p.middlewareObject(spec, name, map[string]any{
			"redirectRegex": map[string]any{
				"regex":       pattern,
				"replacement": replacement,
				// Traefik llama «permanent» al 301/308. El código exacto lo elige
				// domain.RedirectStatus (308 por defecto, que preserva el método).
				"permanent": domain.RedirectStatus(rd) == 301 || domain.RedirectStatus(rd) == 308,
			},
		})
		if err := p.upsertMiddleware(ctx, spec.Namespace, name, mw); err != nil {
			return nil, err
		}
		refs = append(refs, middlewareRef(spec.Namespace, name))
	}

	if len(rules.Headers) > 0 {
		headers := make(map[string]any, len(rules.Headers))
		for k, v := range rules.Headers {
			headers[k] = v
		}
		name := base + "-h"
		mw := p.middlewareObject(spec, name, map[string]any{
			"headers": map[string]any{"customResponseHeaders": headers},
		})
		if err := p.upsertMiddleware(ctx, spec.Namespace, name, mw); err != nil {
			return nil, err
		}
		refs = append(refs, middlewareRef(spec.Namespace, name))
	}
	return refs, nil
}

// middlewareObject arma el CRD con la especificación dada.
func (p *KubePlatform) middlewareObject(spec AppSpec, name string, mwSpec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "Middleware",
			"metadata": map[string]any{
				"name":      name,
				"namespace": spec.Namespace,
				"labels": map[string]any{
					"app.kubernetes.io/part-of": "sytue-one",
					"sytue.eu/tenant":           spec.Tenant,
					"sytue.eu/deployment":       spec.DeploymentID,
				},
			},
			"spec": mwSpec,
		},
	}
}

// upsertMiddleware crea o actualiza un Middleware de Traefik.
func (p *KubePlatform) upsertMiddleware(
	ctx context.Context, namespace, name string, mw *unstructured.Unstructured,
) error {
	api := p.dyn.Resource(traefikMiddlewareGVR).Namespace(namespace)
	existing, err := api.Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, cerr := api.Create(ctx, mw, metav1.CreateOptions{}); cerr != nil && !apierrors.IsAlreadyExists(cerr) {
			return fmt.Errorf("controller: crear Middleware %s: %w", name, cerr)
		}
		return nil
	case err != nil:
		return fmt.Errorf("controller: consultar Middleware %s: %w", name, err)
	default:
		mw.SetResourceVersion(existing.GetResourceVersion())
		if _, uerr := api.Update(ctx, mw, metav1.UpdateOptions{}); uerr != nil {
			return fmt.Errorf("controller: actualizar Middleware %s: %w", name, uerr)
		}
		return nil
	}
}

// middlewareRef es como Traefik nombra un middleware declarado por CRD.
func middlewareRef(namespace, name string) string {
	return fmt.Sprintf("%s-%s@kubernetescrd", namespace, name)
}

// removeHTTPRules borra los middlewares de reglas de un deployment. Se barren por
// ETIQUETA y no por nombre: el número de reglas puede haber cambiado entre
// despliegues, y borrar «los que me sé» dejaría sueltos los que sobran.
func (p *KubePlatform) removeHTTPRules(ctx context.Context, namespace, deploymentID string) {
	bg := metav1.DeletePropagationBackground
	_ = p.dyn.Resource(traefikMiddlewareGVR).Namespace(namespace).DeleteCollection(ctx,
		metav1.DeleteOptions{PropagationPolicy: &bg},
		metav1.ListOptions{LabelSelector: "sytue.eu/deployment=" + deploymentID},
	)
}
