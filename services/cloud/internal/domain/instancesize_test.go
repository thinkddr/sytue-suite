// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// TestBasicReproduceElComportamientoAnterior es la garantía de que esta función no
// mueve la factura ni los recursos de NINGUNA app existente: 'basic' —el tamaño por
// defecto— tiene que ser exactamente lo que el controlador tenía escrito a mano y
// lo que el metering asumía. Si alguien retoca estos valores, rompe este test.
func TestBasicReproduceElComportamientoAnterior(t *testing.T) {
	s := InstanceSizeOrDefault("")
	if s.Name != DefaultInstanceSizeName {
		t.Fatalf("el defecto es %q, quiero %q", s.Name, DefaultInstanceSizeName)
	}
	want := map[string]string{
		"cpu_request": "100m", "memory_request": "128Mi",
		"cpu_limit": "1000m", "memory_limit": "512Mi",
	}
	got := map[string]string{
		"cpu_request": s.CPURequest, "memory_request": s.MemoryRequest,
		"cpu_limit": s.CPULimit, "memory_limit": s.MemoryLimit,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, quiero %q (era el valor escrito a mano en el controlador)", k, got[k], w)
		}
	}
	// 512Mi de límite ⇒ 0,5 GiB facturables, que es la constante que asumía el metering.
	if s.MemoryGiB != 0.5 {
		t.Errorf("memoria facturable = %v GiB, quiero 0.5 (lo que se facturaba antes)", s.MemoryGiB)
	}
}

func TestInstanceSizeOrDefaultDegrada(t *testing.T) {
	// Un nombre desconocido (fila antigua, migración a medias) NO puede dejar el
	// contenedor sin recursos declarados: degrada al básico.
	for _, name := range []string{"", "inventado", "PRO"} {
		if got := InstanceSizeOrDefault(name); got.Name != DefaultInstanceSizeName {
			t.Errorf("%q → %q, quiero el básico", name, got.Name)
		}
	}
	if got := InstanceSizeOrDefault("pro"); got.Name != "pro" {
		t.Errorf("un nombre del catálogo debe respetarse, dio %q", got.Name)
	}
}

func TestCatalogoCoherente(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range InstanceSizes {
		if seen[s.Name] {
			t.Errorf("nombre duplicado en el catálogo: %q", s.Name)
		}
		seen[s.Name] = true
		if s.Label == "" {
			t.Errorf("%q sin etiqueta para la interfaz", s.Name)
		}
		// Sin memoria facturable, ese tamaño saldría gratis en la factura.
		if s.MemoryGiB <= 0 {
			t.Errorf("%q tiene memoria facturable %v", s.Name, s.MemoryGiB)
		}
		for field, v := range map[string]string{
			"CPURequest": s.CPURequest, "MemoryRequest": s.MemoryRequest,
			"CPULimit": s.CPULimit, "MemoryLimit": s.MemoryLimit,
		} {
			if v == "" {
				t.Errorf("%q sin %s", s.Name, field)
			}
		}
	}
	if !seen[DefaultInstanceSizeName] {
		t.Fatalf("el catálogo no contiene el tamaño por defecto %q", DefaultInstanceSizeName)
	}
}

func TestValidacionDeTamanoYNumero(t *testing.T) {
	if err := ValidateInstanceSize(""); err != nil {
		t.Errorf("vacío significa «usa el defecto»: %v", err)
	}
	if err := ValidateInstanceSize("pro"); err != nil {
		t.Errorf("un tamaño del catálogo debe valer: %v", err)
	}
	if err := ValidateInstanceSize("gigante"); err == nil {
		t.Error("un tamaño fuera del catálogo debe rechazarse")
	}
	for _, ok := range []int{0, 1, 10, MaxInstanceCount} {
		if err := ValidateInstanceCount(ok); err != nil {
			t.Errorf("%d debería valer: %v", ok, err)
		}
	}
	for _, bad := range []int{-1, MaxInstanceCount + 1, 10_000} {
		if err := ValidateInstanceCount(bad); err == nil {
			t.Errorf("%d debería rechazarse", bad)
		}
	}
}

func TestEffectiveInstanceCountAcota(t *testing.T) {
	var p Project
	if got := p.EffectiveInstanceCount(); got != 1 {
		t.Errorf("sin configurar = %d, quiero 1", got)
	}
	// Un valor imposible persistido por otra vía no puede llegar a Kubernetes.
	p.InstanceCount = 10_000
	if got := p.EffectiveInstanceCount(); got != MaxInstanceCount {
		t.Errorf("= %d, quiero acotado a %d", got, MaxInstanceCount)
	}
	p.InstanceCount = 4
	if got := p.EffectiveInstanceCount(); got != 4 {
		t.Errorf("= %d, quiero 4", got)
	}
}

// TestAutoscaleAutomaticoReproduceLaReglaAnterior es la garantía de que la
// migración 0031 no cambia el techo de NINGUNA app existente: sin configurar,
// EffectiveAutoscaleMax tiene que dar exactamente lo que el controlador calculaba
// a mano (5× las instancias, suelo 3) y el objetivo tiene que seguir siendo 75.
func TestAutoscaleAutomaticoReproduceLaReglaAnterior(t *testing.T) {
	for _, c := range []struct{ instancias, quiero int }{
		{0, 5},  // sin configurar = 1 instancia => 5
		{1, 5},  // 1×5
		{2, 10}, // 2×5
		{4, 20},
	} {
		p := Project{InstanceCount: c.instancias}
		if got := p.EffectiveAutoscaleMax(); got != c.quiero {
			t.Errorf("con %d instancias y techo automático = %d, quiero %d",
				c.instancias, got, c.quiero)
		}
	}
	var p Project
	if got := p.EffectiveAutoscaleCPUTarget(); got != DefaultCPUTarget {
		t.Errorf("objetivo sin configurar = %d, quiero %d", got, DefaultCPUTarget)
	}
}

// TestAutoscaleMaxNuncaPorDebajoDelMinimo cubre el estado que Kubernetes RECHAZA:
// maxReplicas < minReplicas. Se alcanza sin hacer nada raro —fijar un techo de 2 y
// más tarde subir a 5 instancias, dos PATCH independientes— y si llegara al
// clúster la app se quedaría sin poder aplicar ningún objeto, no solo sin escalar.
func TestAutoscaleMaxNuncaPorDebajoDelMinimo(t *testing.T) {
	p := Project{InstanceCount: 5, AutoscaleMaxCount: 2}
	if got := p.EffectiveAutoscaleMax(); got != 5 {
		t.Errorf("techo 2 con 5 instancias = %d, quiero 5 (el mínimo manda)", got)
	}
	p = Project{InstanceCount: 2, AutoscaleMaxCount: 30}
	if got := p.EffectiveAutoscaleMax(); got != 30 {
		t.Errorf("un techo por encima del mínimo se respeta: = %d, quiero 30", got)
	}
}

// TestAutoscaleCPUTargetDegradaFueraDeRango: un valor imposible persistido por
// cualquier vía (migración a medias, escritura futura sin validar) no puede llegar
// a Kubernetes; degrada al defecto.
func TestAutoscaleCPUTargetDegradaFueraDeRango(t *testing.T) {
	for _, malo := range []int{-1, 0, MinCPUTarget - 1, MaxCPUTarget + 1, 10_000} {
		p := Project{AutoscaleCPUTarget: malo}
		if got := p.EffectiveAutoscaleCPUTarget(); got != DefaultCPUTarget {
			t.Errorf("objetivo %d = %d, quiero degradar a %d", malo, got, DefaultCPUTarget)
		}
	}
	p := Project{AutoscaleCPUTarget: 40}
	if got := p.EffectiveAutoscaleCPUTarget(); got != 40 {
		t.Errorf("un objetivo válido se respeta: = %d, quiero 40", got)
	}
}

func TestValidacionDeAutoescalado(t *testing.T) {
	for _, ok := range []int{0, 1, MaxInstanceCount} {
		if err := ValidateAutoscaleMaxCount(ok); err != nil {
			t.Errorf("techo %d debería valer: %v", ok, err)
		}
	}
	for _, bad := range []int{-1, MaxInstanceCount + 1} {
		if err := ValidateAutoscaleMaxCount(bad); err == nil {
			t.Errorf("techo %d debería rechazarse", bad)
		}
	}
	for _, ok := range []int{0, MinCPUTarget, 75, MaxCPUTarget} {
		if err := ValidateAutoscaleCPUTarget(ok); err != nil {
			t.Errorf("objetivo %d debería valer: %v", ok, err)
		}
	}
	for _, bad := range []int{-1, MinCPUTarget - 1, MaxCPUTarget + 1} {
		if err := ValidateAutoscaleCPUTarget(bad); err == nil {
			t.Errorf("objetivo %d debería rechazarse", bad)
		}
	}
}
