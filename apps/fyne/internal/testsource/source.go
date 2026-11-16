// SPDX-License-Identifier: Apache-2.0
// Package testsource classifies support code for repository architecture tests.
package testsource

import (
	"errors"
	"go/build/constraint"
	"strings"
)

// IntegrationOnly reports a positive integration requirement in the build
// expression. OS and feature alternatives remain visible on every test host.
// An unrecognized/negated condition is kept visible, never assumed test-only.
func IntegrationOnly(source []byte) (bool, error) {
	var expression constraint.Expr
	for line := range strings.SplitSeq(string(source), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			break
		}
		if !constraint.IsGoBuild(line) {
			continue
		}
		if expression != nil {
			return false, errors.New("duplicate go:build constraint")
		}
		parsed, err := constraint.Parse(line)
		if err != nil {
			return false, err
		}
		expression = parsed
	}
	return requiresIntegration(expression), nil
}

func requiresIntegration(expression constraint.Expr) bool {
	switch value := expression.(type) {
	case *constraint.TagExpr:
		return value.Tag == "integration"
	case *constraint.AndExpr:
		return requiresIntegration(value.X) || requiresIntegration(value.Y)
	case *constraint.OrExpr:
		return requiresIntegration(value.X) && requiresIntegration(value.Y)
	default:
		return false
	}
}
