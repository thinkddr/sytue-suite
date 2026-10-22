// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ---------------------------------------------------------------------------
// Previews cerradas con contraseña
// ---------------------------------------------------------------------------
//
// Una preview es la rama de alguien SIN REVISAR, con datos de prueba dentro y una
// URL pública que Google puede indexar. Hasta ahora salía a internet abierta,
// igual que producción, y la única forma de que no lo estuviera era no crearla.
//
// Vercel lo llama Deployment Protection y Coolify lo resuelve con BasicAuth por
// servicio; aquí es un middleware de Traefik enganchado SOLO al Ingress de las
// previews. Producción no se toca jamás: cerrar producción con una contraseña
// básica sería romper el sitio del cliente.

// traefikMiddlewareGVR es el CRD Middleware de Traefik. Mismo patrón que el
// Certificate de cert-manager: vía cliente dinámico, sin arrastrar el módulo Go
// de Traefik al go.mod compartido por una estructura de tres campos.
var traefikMiddlewareGVR = schema.GroupVersionResource{
	Group:    "traefik.io",
	Version:  "v1alpha1",
	Resource: "middlewares",
}

// previewAuthName es el nombre del Middleware y del Secret de una preview. Va
// ligado al DEPLOYMENT y no al proyecto: cada preview vive y muere con su PR, y
// compartir el objeto entre varias dejaría el de la última en pie cuando se
// retirara la primera.
func previewAuthName(spec AppSpec) string {
	return "auth-" + shortID(spec.DeploymentID)
}

// ensurePreviewAuth materializa el Secret htpasswd y el Middleware de Traefik que
// cierran una preview, y devuelve la referencia que hay que poner en el Ingress
// (`<namespace>-<nombre>@kubernetescrd`, que es como Traefik nombra un middleware
// declarado por CRD).
//
// Cadena vacía y sin error cuando no hay que proteger nada: así el llamante no
// tiene que repetir la condición.
func (p *KubePlatform) ensurePreviewAuth(ctx context.Context, spec AppSpec) (string, error) {
	if spec.Target != "preview" || spec.PreviewAuthHash == "" {
		return "", nil
	}
	name := previewAuthName(spec)

	// El Secret lleva el htpasswd ya calculado por el control-plane: aquí no se
	// hace criptografía, solo se materializa. La contraseña en claro no pasa por
	// el clúster en ningún momento.
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: spec.Namespace, Labels: appLabels(spec)},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"users": []byte(spec.PreviewAuthHash)},
	}
	secAPI := p.cs.CoreV1().Secrets(spec.Namespace)
	switch existing, serr := secAPI.Get(ctx, name, metav1.GetOptions{}); {
	case apierrors.IsNotFound(serr):
		if _, cerr := secAPI.Create(ctx, sec, metav1.CreateOptions{}); cerr != nil {
			return "", ignoreAlreadyExists(cerr, "crear Secret de preview")
		}
	case serr != nil:
		return "", fmt.Errorf("controller: consultar Secret de preview: %w", serr)
	default:
		sec.ResourceVersion = existing.ResourceVersion
		if _, uerr := secAPI.Update(ctx, sec, metav1.UpdateOptions{}); uerr != nil {
			return "", fmt.Errorf("controller: actualizar Secret de preview: %w", uerr)
		}
	}

	mw := &unstructured.Unstructured{
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
			"spec": map[string]any{
				"basicAuth": map[string]any{
					"secret": name,
					// El navegador enseña este texto en su ventana de credenciales:
					// que diga de qué sitio es, no «Restricted».
					"realm": "Sytue Cloud preview",
					// Sin esto, Traefik pasa la cabecera Authorization a la app, que
					// vería una credencial que no es suya y podría confundirla con la
					// de su propio login.
					"removeHeader": true,
				},
			},
		},
	}
	api := p.dyn.Resource(traefikMiddlewareGVR).Namespace(spec.Namespace)
	existing, err := api.Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		if _, cerr := api.Create(ctx, mw, metav1.CreateOptions{}); cerr != nil && !apierrors.IsAlreadyExists(cerr) {
			return "", fmt.Errorf("controller: crear Middleware de preview: %w", cerr)
		}
	case err != nil:
		return "", fmt.Errorf("controller: consultar Middleware de preview: %w", err)
	default:
		mw.SetResourceVersion(existing.GetResourceVersion())
		if _, uerr := api.Update(ctx, mw, metav1.UpdateOptions{}); uerr != nil {
			return "", fmt.Errorf("controller: actualizar Middleware de preview: %w", uerr)
		}
	}
	// Traefik referencia los middlewares de CRD por «<namespace>-<nombre>@kubernetescrd».
	return fmt.Sprintf("%s-%s@kubernetescrd", spec.Namespace, name), nil
}

// removePreviewAuth retira el Middleware y el Secret de una preview. Se llama al
// desmontar el deployment: dejarlos vivos acumularía un objeto por PR cerrado en
// el namespace del cliente.
func (p *KubePlatform) removePreviewAuth(ctx context.Context, namespace, deploymentID string) {
	name := "auth-" + shortID(deploymentID)
	bg := metav1.DeletePropagationBackground
	opts := metav1.DeleteOptions{PropagationPolicy: &bg}
	_ = p.dyn.Resource(traefikMiddlewareGVR).Namespace(namespace).Delete(ctx, name, opts)
	_ = p.cs.CoreV1().Secrets(namespace).Delete(ctx, name, opts)
}
