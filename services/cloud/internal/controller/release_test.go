// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"errors"
	"testing"

	batchv1 "k8s.io/api/batch/v1"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// releasePlatform es una Platform que solo sabe del paso de release: devuelve el
// estado configurado y cuenta las llamadas. Embebe StubPlatform para heredar el
// resto de la interfaz.
type releasePlatform struct {
	StubPlatform
	state ReleaseState
	err   error
	calls int
}

func (p *releasePlatform) RunRelease(context.Context, AppSpec) (ReleaseState, error) {
	p.calls++
	return p.state, p.err
}

// El criterio que decide si el despliegue sigue adelante. Es lo único con ramas
// del paso de release, y equivocarse en cualquiera de ellas tiene consecuencias
// caras: arrancar una versión cuya migración falló, o dejar un despliegue colgado
// para siempre.
func TestReleaseDone(t *testing.T) {
	t.Parallel()
	fallo := errors.New("api caída")
	casos := []struct {
		nombre  string
		comando string
		state   ReleaseState
		err     error
		quiero  bool
		llamada bool
	}{
		{
			nombre:  "sin comando no hay paso: sigue sin tocar el clúster",
			comando: "",
			quiero:  true,
			llamada: false,
		},
		{
			nombre:  "terminado con éxito: sigue",
			comando: "npm run migrate",
			state:   ReleaseSucceeded,
			quiero:  true,
			llamada: true,
		},
		{
			nombre:  "todavía corriendo: espera al próximo ciclo",
			comando: "npm run migrate",
			state:   ReleaseRunning,
			quiero:  false,
			llamada: true,
		},
		{
			nombre:  "falló: NO se arranca la versión nueva",
			comando: "npm run migrate",
			state:   ReleaseFailed,
			quiero:  false,
			llamada: true,
		},
		{
			// Un error hablando con Kubernetes no es una migración fallida: se
			// reintenta, y sobre todo NO se marca el deployment en error.
			nombre:  "error del clúster: ni sigue ni condena el despliegue",
			comando: "npm run migrate",
			err:     fallo,
			quiero:  false,
			llamada: true,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			plat := &releasePlatform{state: c.state, err: c.err}
			fs := &fakeCtrlStore{}
			ctrl := newTestController(fs, plat, nil)
			spec := AppSpec{ReleaseCommand: c.comando}
			dep := domain.Deployment{Tenant: "acme"}

			got := ctrl.releaseDone(context.Background(), dep, spec, &capLogger{})
			if got != c.quiero {
				t.Errorf("releaseDone = %v, quiero %v", got, c.quiero)
			}
			if (plat.calls > 0) != c.llamada {
				t.Errorf("llamadas a RunRelease = %d; se esperaba llamada=%v", plat.calls, c.llamada)
			}
			// Solo el fallo REAL de la migración condena el despliegue. Un error de
			// red dejándolo en error obligaría a redesplegar a mano por un hipo.
			condenado := hasStatus(fs.statusSet, dep.ID, domain.DeployError)
			if quiero := c.state == ReleaseFailed && c.err == nil; condenado != quiero {
				t.Errorf("deployment marcado en error = %v, quiero %v", condenado, quiero)
			}
		})
	}
}

// releaseStateOf traduce el Job a las tres respuestas del reconciliador. Con
// backoffLimit 0, un `Failed` es definitivo: no hay reintento que esperar.
func TestReleaseStateOf(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre string
		status batchv1.JobStatus
		quiero ReleaseState
	}{
		{"recién creado", batchv1.JobStatus{}, ReleaseRunning},
		{"un pod corriendo", batchv1.JobStatus{Active: 1}, ReleaseRunning},
		{"terminado bien", batchv1.JobStatus{Succeeded: 1}, ReleaseSucceeded},
		{"terminado mal", batchv1.JobStatus{Failed: 1}, ReleaseFailed},
	}
	for _, c := range casos {
		if got := releaseStateOf(&batchv1.Job{Status: c.status}); got != c.quiero {
			t.Errorf("%s: releaseStateOf = %q, quiero %q", c.nombre, got, c.quiero)
		}
	}
}
