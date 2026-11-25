// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package buildcache

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// SecretosKube guarda las credenciales como Secrets `kubernetes.io/dockerconfigjson` del
// namespace de builds, con la etiqueta del tenant.
type SecretosKube struct {
	cs kubernetes.Interface
	ns string
}

// NewSecretosKube construye el almacén sobre el namespace de builds.
func NewSecretosKube(cs kubernetes.Interface, namespace string) *SecretosKube {
	return &SecretosKube{cs: cs, ns: namespace}
}

var _ Secretos = (*SecretosKube)(nil)

// Existe implementa Secretos.
func (s *SecretosKube) Existe(ctx context.Context, name string) (bool, error) {
	_, err := s.cs.CoreV1().Secrets(s.ns).Get(ctx, name, metav1.GetOptions{})
	switch {
	case err == nil:
		return true, nil
	case apierrors.IsNotFound(err):
		return false, nil
	}
	return false, fmt.Errorf("buildcache: leer Secret %s: %w", name, err)
}

// Guardar implementa Secretos: crea el Secret o, si ya está (otra réplica del builder lo
// guardó a la vez), lo borra y lo crea de nuevo: la key que vale es la última acuñada, que
// borró la anterior. Borrar y crear, y no actualizar: es lo que el rol del builder ya
// puede hacer con los Secrets de su namespace (create/delete, como los de git).
func (s *SecretosKube) Guardar(ctx context.Context, name, tenant string, dockerConfigJSON []byte) error {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.ns,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "sytue-one",
				"sytue.eu/component":        "cloud-build-cache",
				"sytue.eu/tenant":           tenant,
			},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigJSON},
	}
	_, err := s.cs.CoreV1().Secrets(s.ns).Create(ctx, sec, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		if err = s.cs.CoreV1().Secrets(s.ns).Delete(ctx, name, metav1.DeleteOptions{}); err == nil || apierrors.IsNotFound(err) {
			_, err = s.cs.CoreV1().Secrets(s.ns).Create(ctx, sec, metav1.CreateOptions{})
		}
	}
	if err != nil {
		return fmt.Errorf("buildcache: guardar Secret %s: %w", name, err)
	}
	return nil
}
