// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"strings"
)

// Grupos dinámicos (paridad Entra/Google dynamic groups): la membresía la calcula una
// REGLA sobre atributos del usuario, en vez de asignarse a mano. Este fichero define
// la regla y su MOTOR PURO (EvaluateRule) — sin BD ni HTTP, 100% testeable. El store
// materializa el resultado en group_members (ver store/dynamicgroups.go), así el motor
// no sabe nada de persistencia.

// RuleOp es el operador de una condición. Catálogo cerrado (paridad con los ops de
// Entra/Google: eq, ne, contains, startsWith, in).
type RuleOp string

const (
	OpEquals     RuleOp = "eq"          // igualdad exacta (case-insensitive)
	OpNotEquals  RuleOp = "ne"          // desigualdad (un atributo ausente ≠ cualquier valor)
	OpContains   RuleOp = "contains"    // subcadena
	OpStartsWith RuleOp = "starts_with" // prefijo
	OpIn         RuleOp = "in"          // pertenencia a un conjunto
)

// Atributos de usuario que una regla puede referenciar. Catálogo FIJO que mapea 1:1 a
// columnas de la tabla users (el store construye el mapa con estas claves). email/name
// ya existían; department/title/location los añade la mig 0021.
const (
	AttrEmail      = "email"
	AttrName       = "name"
	AttrDepartment = "department"
	AttrTitle      = "title"
	AttrLocation   = "location"
)

// RuleAttributes es la allow-list ordenada (para la UI del constructor y la validación).
var RuleAttributes = []string{AttrEmail, AttrName, AttrDepartment, AttrTitle, AttrLocation}

var ruleAttributeSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(RuleAttributes))
	for _, a := range RuleAttributes {
		m[a] = struct{}{}
	}
	return m
}()

// Condition es una condición atómica: «attribute op value(s)».
type Condition struct {
	Attribute string   `json:"attribute"`
	Op        RuleOp   `json:"op"`
	Value     string   `json:"value,omitempty"`  // eq/ne/contains/starts_with
	Values    []string `json:"values,omitempty"` // in
}

// Rule es la regla de membresía de un grupo dinámico: un conjunto de condiciones
// combinadas con AND ("all") u OR ("any").
type Rule struct {
	Match      string      `json:"match"` // "all" (AND) | "any" (OR)
	Conditions []Condition `json:"conditions"`
}

const (
	maxRuleConditions = 50
	maxRuleValueLen   = 256
	maxRuleInValues   = 100
)

// Validate comprueba que la regla es bien formada ANTES de guardarla o previsualizarla
// (el handler traduce el error a 400). Es deny-by-default: una regla sin condiciones es
// inválida (un grupo dinámico debe tener al menos una condición; en evaluación, además,
// una regla vacía no incluye a nadie).
func (r Rule) Validate() error {
	switch strings.ToLower(strings.TrimSpace(r.Match)) {
	case "all", "any":
	default:
		return fmt.Errorf("%w: match debe ser 'all' o 'any'", ErrInvalidInput)
	}
	if n := len(r.Conditions); n == 0 || n > maxRuleConditions {
		return fmt.Errorf("%w: la regla debe tener entre 1 y %d condiciones", ErrInvalidInput, maxRuleConditions)
	}
	for _, c := range r.Conditions {
		if _, ok := ruleAttributeSet[c.Attribute]; !ok {
			return fmt.Errorf("%w: atributo desconocido %q", ErrInvalidInput, c.Attribute)
		}
		switch c.Op {
		case OpIn:
			if n := len(c.Values); n == 0 || n > maxRuleInValues {
				return fmt.Errorf("%w: 'in' requiere entre 1 y %d valores", ErrInvalidInput, maxRuleInValues)
			}
			for _, v := range c.Values {
				if strings.TrimSpace(v) == "" || len(v) > maxRuleValueLen {
					return fmt.Errorf("%w: valor de 'in' vacío o demasiado largo", ErrInvalidInput)
				}
			}
		case OpEquals, OpNotEquals, OpContains, OpStartsWith:
			if strings.TrimSpace(c.Value) == "" || len(c.Value) > maxRuleValueLen {
				return fmt.Errorf("%w: la condición requiere un valor (≤ %d caracteres)", ErrInvalidInput, maxRuleValueLen)
			}
		default:
			return fmt.Errorf("%w: operador desconocido %q", ErrInvalidInput, c.Op)
		}
	}
	return nil
}

// EvaluateRule es la DECISIÓN de membresía PURA: ¿el usuario con estos atributos cumple
// la regla? attrs mapea nombre de atributo → valor (un atributo ausente cuenta como ""
// — cadena vacía, no error). Comparación case-insensitive (como Entra) y con los
// extremos recortados. Una regla sin condiciones no incluye a nadie (deny-by-default).
//
//   - match "all" (AND): true solo si TODAS las condiciones se cumplen.
//   - match "any" (OR):  true si al menos UNA se cumple.
func EvaluateRule(rule Rule, attrs map[string]string) bool {
	if len(rule.Conditions) == 0 {
		return false
	}
	orMode := strings.EqualFold(strings.TrimSpace(rule.Match), "any")
	for _, c := range rule.Conditions {
		ok := evalCondition(c, attrs[c.Attribute])
		if orMode && ok {
			return true
		}
		if !orMode && !ok {
			return false
		}
	}
	// AND: se recorrieron todas sin fallo ⇒ true. OR: ninguna se cumplió ⇒ false.
	return !orMode
}

func evalCondition(c Condition, actual string) bool {
	a := strings.ToLower(strings.TrimSpace(actual))
	switch c.Op {
	case OpEquals:
		return a == norm(c.Value)
	case OpNotEquals:
		return a != norm(c.Value)
	case OpContains:
		return strings.Contains(a, norm(c.Value))
	case OpStartsWith:
		return strings.HasPrefix(a, norm(c.Value))
	case OpIn:
		for _, v := range c.Values {
			if a == norm(v) {
				return true
			}
		}
		return false
	default:
		return false // operador desconocido ⇒ no cumple (deny-by-default)
	}
}

func norm(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
