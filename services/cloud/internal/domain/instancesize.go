// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Tamaño de instancia y número de instancias (paridad DigitalOcean App Platform)
// ---------------------------------------------------------------------------
//
// Hasta ahora TODA app corría con los mismos recursos —100m/128Mi de petición y
// 1 CPU/512Mi de límite, escritos a mano en el controlador— y el número de
// réplicas existía en la base de datos y en el AppSpec pero NINGÚN endpoint ni el
// CLI lo escribían. Es decir: una app con carga real no se podía escalar y una app
// pequeña pagaba lo mismo que una grande.
//
// «Instance size + instance count» es el concepto de primera clase con el que se
// vende DigitalOcean App Platform, y es el que se adopta: un catálogo CERRADO de
// tamaños con nombre (no CPU/memoria libres) porque así el precio y las cuotas del
// plan son predecibles, y porque un cliente que teclea «17.3 CPU» acaba con un pod
// que no cabe en ningún nodo.

// InstanceSize es un tamaño de instancia del catálogo: lo que se pide y lo que se
// limita en el contenedor de la app.
type InstanceSize struct {
	// Name es el identificador estable que se persiste y viaja por la API.
	Name string `json:"name"`
	// Label es el nombre en lenguaje llano para la interfaz.
	Label string `json:"label"`
	// CPURequest/MemoryRequest son la RESERVA garantizada (lo que el planificador
	// aparta). CPULimit/MemoryLimit son el techo. Cadenas en notación Kubernetes.
	CPURequest    string `json:"cpu_request"`
	MemoryRequest string `json:"memory_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryLimit   string `json:"memory_limit"`
	// MemoryGiB es el límite de memoria en GiB, y es lo que se FACTURA como
	// compute_gb_hours. Se guarda derivado para que el metering no tenga que
	// interpretar la notación de Kubernetes en cada muestra.
	MemoryGiB float64 `json:"memory_gib"`
}

// InstanceSizes es el catálogo, de menor a mayor. `basic` reproduce EXACTAMENTE
// los recursos que el controlador tenía escritos a mano, así que ninguna app
// existente cambia de comportamiento ni de factura al migrar.
var InstanceSizes = []InstanceSize{
	{
		Name: "basic", Label: "Básico",
		CPURequest: "100m", MemoryRequest: "128Mi",
		CPULimit: "1000m", MemoryLimit: "512Mi",
		MemoryGiB: 0.5,
	},
	{
		Name: "standard", Label: "Estándar",
		CPURequest: "250m", MemoryRequest: "512Mi",
		CPULimit: "1000m", MemoryLimit: "1Gi",
		MemoryGiB: 1,
	},
	{
		Name: "pro", Label: "Pro",
		CPURequest: "500m", MemoryRequest: "1Gi",
		CPULimit: "2000m", MemoryLimit: "2Gi",
		MemoryGiB: 2,
	},
	{
		Name: "pro-xl", Label: "Pro XL",
		CPURequest: "1000m", MemoryRequest: "2Gi",
		CPULimit: "4000m", MemoryLimit: "4Gi",
		MemoryGiB: 4,
	},
}

// CPULimitMilli devuelve el techo de CPU en milicores.
//
// El catálogo guarda la notación de Kubernetes («1000m», «2») porque es lo que
// viaja al PodSpec; para enseñar «usas 300 de 1000» hace falta el número, y
// parsearlo en el navegador sería llevar la gramática de Kubernetes al frontend.
// Solo se admiten las dos formas que el catálogo usa: entero de núcleos, o
// milicores con sufijo `m`.
func (s InstanceSize) CPULimitMilli() int {
	raw := strings.TrimSpace(s.CPULimit)
	if raw == "" {
		return 0
	}
	if strings.HasSuffix(raw, "m") {
		n, err := strconv.Atoi(strings.TrimSuffix(raw, "m"))
		if err != nil {
			return 0
		}
		return n
	}
	cores, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return int(cores * 1000)
}

// DefaultInstanceSizeName es el tamaño de una app que no elige ninguno.
const DefaultInstanceSizeName = "basic"

// MaxInstanceCount acota el número de instancias por app. El techo REAL lo pone la
// cuota del plan (ResourceQuota/MaxPods del namespace, plan_quota.go): esto solo
// evita que un número absurdo llegue a Kubernetes, donde el error sería opaco.
const MaxInstanceCount = 50

// InstanceSizeByName busca un tamaño del catálogo.
func InstanceSizeByName(name string) (InstanceSize, bool) {
	for _, s := range InstanceSizes {
		if s.Name == name {
			return s, true
		}
	}
	return InstanceSize{}, false
}

// InstanceSizeOrDefault devuelve el tamaño pedido o el por defecto. Nunca falla:
// un valor desconocido persistido (por una migración a medias o un despliegue
// antiguo) degrada al básico en vez de dejar la app sin recursos declarados.
func InstanceSizeOrDefault(name string) InstanceSize {
	if s, ok := InstanceSizeByName(name); ok {
		return s
	}
	s, _ := InstanceSizeByName(DefaultInstanceSizeName)
	return s
}

// ValidateInstanceSize acepta un nombre del catálogo; vacío = «usa el defecto».
func ValidateInstanceSize(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := InstanceSizeByName(name); !ok {
		return fmt.Errorf("%w: tamaño de instancia desconocido %q", ErrInvalidInput, name)
	}
	return nil
}

// ValidateInstanceCount acepta 1..MaxInstanceCount; 0 = «usa el defecto» (1).
func ValidateInstanceCount(n int) error {
	if n == 0 {
		return nil
	}
	if n < 0 || n > MaxInstanceCount {
		return fmt.Errorf("%w: el número de instancias debe estar entre 1 y %d",
			ErrInvalidInput, MaxInstanceCount)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Autoescalado: techo y objetivo (migración 0031)
// ---------------------------------------------------------------------------
//
// `instance_count` es el MÍNIMO del HPA. El techo y el objetivo de CPU eran
// constantes del controlador, así que la única palanca para aguantar un pico era
// subir el mínimo —que se paga siempre—. Aquí viven las dos que faltaban.

const (
	// DefaultCPUTarget es el objetivo de utilización de CPU del HPA cuando el
	// proyecto no fija otro. Es el valor que el controlador tenía escrito a mano.
	DefaultCPUTarget = 75
	// MinCPUTarget / MaxCPUTarget acotan el objetivo. Por debajo del 10 % el HPA
	// escala con ruido de medición y una app ociosa mantendría el máximo de pods
	// encendidos, que es una factura inesperada con forma de error de config.
	MinCPUTarget = 10
	MaxCPUTarget = 100
	// autoscaleAutoFactor es la regla del techo AUTOMÁTICO (autoscale_max_count =
	// 0), la que el controlador aplicaba antes de que el techo fuese configurable:
	// 5× las instancias configuradas.
	autoscaleAutoFactor = 5
)

// ValidateAutoscaleMaxCount acepta 1..MaxInstanceCount; 0 = «automático».
//
// NO comprueba que el techo quede por encima del mínimo: los dos campos se
// actualizan por separado (PATCH parcial), así que exigir la coherencia aquí
// obligaría a un orden concreto de peticiones —y subir instancias por encima del
// techo tendría que fallar, lo cual sería absurdo—. La coherencia la garantiza
// EffectiveAutoscaleMax, que sube el techo al mínimo cuando haría falta.
func ValidateAutoscaleMaxCount(n int) error {
	if n == 0 {
		return nil
	}
	if n < 0 || n > MaxInstanceCount {
		return fmt.Errorf("%w: el máximo de instancias debe estar entre 1 y %d (0 = automático)",
			ErrInvalidInput, MaxInstanceCount)
	}
	return nil
}

// ValidateAutoscaleCPUTarget acepta MinCPUTarget..MaxCPUTarget; 0 = «usa el
// defecto» (DefaultCPUTarget).
func ValidateAutoscaleCPUTarget(n int) error {
	if n == 0 {
		return nil
	}
	if n < MinCPUTarget || n > MaxCPUTarget {
		return fmt.Errorf("%w: el objetivo de CPU debe estar entre %d y %d por ciento",
			ErrInvalidInput, MinCPUTarget, MaxCPUTarget)
	}
	return nil
}
