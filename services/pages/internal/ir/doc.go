// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ir es el espejo en Go del IR de Sytue Web (el formato del diseño de una web), cuyo
// original vive en apps/frontend/packages/pages-render/src/ir/. Guarda los tipos, la tabla de
// campos, el validador en dos fases (estructural sobre el JSON crudo y semántica sobre los tipos,
// con los modos borrador y publicar) y la codificación. El validador de Go es la AUTORIDAD: el de
// TypeScript recorre las mismas fixtures de testdata/ y tiene que dar el mismo conjunto de
// problemas. Dos guardianes leen los ficheros de TypeScript y fallan si el espejo se desvía.
//
// El servicio guarda el IR validado tal cual lo recibió y lo devuelve igual (A20): solo se
// reserializa con Codificar lo que genera Go (plantillas, IA y migraciones).
package ir

//go:generate go run ./generar
