// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// Cada lista de columnas tiene que tener tantos destinos como columnas, o la
// consulta que las junta revienta EN EJECUCIÓN con «number of field descriptions
// must equal number of destinations».
//
// Esto no es teoría: pasó. Se añadieron `commit_message` a los deployments y las
// seis columnas de dimensionado a los proyectos, se actualizaron los scans de una
// fila y NO el de la consulta que junta las dos tablas. Resultado: 41 columnas
// contra 34 destinos, `ListPendingDeployments` fallando en cada barrido y Sytue
// Cloud sin reconciliar un solo despliegue durante días. El único síntoma era una
// línea WARN cada quince segundos.
//
// Sin base de datos y en microsegundos: falla en cuanto alguien añade una columna
// y se olvida del destino, que es exactamente cuando hay que enterarse.
func TestColumnasYDestinosCuadran(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre   string
		cols     string
		destinos int
	}{
		{"deployCols", deployCols, len(deployDests(&domain.Deployment{}))},
		{"projectCols", projectCols, len(projectDests(&domain.Project{}))},
	}
	for _, c := range casos {
		if n := contarColumnas(c.cols); n != c.destinos {
			t.Errorf("%s declara %d columnas y hay %d destinos", c.nombre, n, c.destinos)
		}
	}
}

// contarColumnas cuenta los nombres de una lista SQL. Vale porque son nombres
// simples: si algún día entra una expresión con comas dentro (una función), esto
// se queda corto y hay que contar de otra forma — mejor eso que un falso verde.
func contarColumnas(cols string) int {
	n := 0
	for _, c := range strings.Split(cols, ",") {
		if strings.TrimSpace(c) != "" {
			n++
		}
	}
	return n
}

// Y la consulta de deployments pendientes junta las dos listas EN ESE ORDEN, así
// que su scan tiene que recibir la suma exacta. Es la que se rompió.
func TestDestinosDeLaConsultaJunta(t *testing.T) {
	t.Parallel()
	var d domain.Deployment
	var p domain.Project
	juntos := len(append(deployDests(&d), projectDests(&p)...))
	quiero := contarColumnas(deployCols) + contarColumnas(projectCols)
	if juntos != quiero {
		t.Errorf("la consulta junta %d columnas y el scan recibe %d destinos", quiero, juntos)
	}
}
