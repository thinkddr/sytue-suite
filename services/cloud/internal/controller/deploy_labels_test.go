// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package controller

import (
	"context"
	"testing"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// TestLaPoliticaDeProyectoSeleccionaLosPodsDeLaApp ata las dos mitades de una
// misma promesa que vivían en ficheros distintos y se habían separado sin que
// nada lo notara: allow-project-<id> empareja por `sytue.eu/project-id`, y los
// pods de la app NO llevaban esa etiqueta.
//
// El efecto era el fallo exacto que esa política venía a cerrar: la app recibe
// DATABASE_URL y no puede usarla, porque sin ser seleccionada no obtiene egress
// hacia el 5432 y el peer de entrada del Postgres tampoco la reconoce. Peor:
// antes de acotar por proyecto la regla era PodSelector{} —cualquier pod del
// namespace—, así que el camino SÍ funcionaba; al acotarlo se cerró para todos.
//
// Este test no comprueba una cadena de texto: EVALÚA el selector de la política
// real contra las etiquetas reales, así que se rompe igual si cambia cualquiera
// de los dos lados.
func TestLaPoliticaDeProyectoSeleccionaLosPodsDeLaApp(t *testing.T) {
	proj := domain.Project{ID: uuid.New(), Slug: "tienda"}
	dep := domain.Deployment{Tenant: "acme", Target: domain.TargetProduction}
	podLabels := appPodLabels(dep, proj)

	cs := fake.NewSimpleClientset()
	if err := ensureProjectNetworkPolicy(context.Background(), cs, "tenant-acme", "acme", proj.ID.String()); err != nil {
		t.Fatalf("crear la política de proyecto: %v", err)
	}
	np, err := cs.NetworkingV1().NetworkPolicies("tenant-acme").
		Get(context.Background(), projectPolicyName(proj.ID.String()), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("leer la política: %v", err)
	}

	sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
	if err != nil {
		t.Fatalf("selector inválido: %v", err)
	}
	if !sel.Matches(labels.Set(podLabels)) {
		t.Fatalf("la política NO selecciona los pods de la app.\n"+
			"  selector: %s\n  etiquetas: %v\n"+
			"Sin selección no hay egress hacia el 5432: la app no alcanza su base de datos.",
			sel.String(), podLabels)
	}

	// Y la regla de salida tiene que apuntar a los servicios gestionados DE ESTE
	// proyecto: si apuntara a cualquiera del namespace, se cambiaría el fallo por
	// una fuga entre proyectos del mismo inquilino.
	if len(np.Spec.Egress) == 0 || len(np.Spec.Egress[0].To) == 0 {
		t.Fatal("la política no tiene regla de salida hacia los servicios gestionados")
	}
	peer := np.Spec.Egress[0].To[0]
	if peer.PodSelector == nil || peer.PodSelector.MatchLabels[projectLabel] != proj.ID.String() {
		t.Errorf("la salida no está acotada a los servicios de ESTE proyecto: %+v", peer.PodSelector)
	}
}

// TestLasEtiquetasDeAppLlevanElUUIDDelProyecto fija el porqué: el slug no sirve
// para emparejar, porque el aprovisionador de servicios gestionados solo conoce
// el ProjectID. Si alguien quita esta etiqueta por parecer redundante con
// `sytue.eu/project`, esto se pone rojo antes de que lo haga producción.
func TestLasEtiquetasDeAppLlevanElUUIDDelProyecto(t *testing.T) {
	proj := domain.Project{ID: uuid.New(), Slug: "tienda"}
	l := appPodLabels(domain.Deployment{Tenant: "acme", Target: domain.TargetPreview}, proj)

	if l[projectLabel] != proj.ID.String() {
		t.Errorf("%s = %q; se esperaba el UUID del proyecto", projectLabel, l[projectLabel])
	}
	if l["sytue.eu/project"] != proj.Slug {
		t.Errorf("el slug legible tiene que seguir estando: %q", l["sytue.eu/project"])
	}
	if l["sytue.eu/component"] != "cloud-app" {
		t.Errorf("component = %q; se esperaba cloud-app", l["sytue.eu/component"])
	}
}
