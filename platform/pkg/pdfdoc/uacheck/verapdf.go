// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package uacheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrNoVeraPDF indica que veraPDF no está instalado: los tests que lo usan se saltan.
var ErrNoVeraPDF = errors.New("uacheck: veraPDF no disponible")

// VeraPDF valida pdf contra PDF/UA-1 con veraPDF (https://verapdf.org, GPL-3/MPL-2) y devuelve
// las reglas falladas. Es SOLO una herramienta de desarrollo y de CI: no se distribuye ni
// entra en producción. El ejecutable sale de $SYTUE_VERAPDF o, si no, del PATH; en el
// frontal de los agentes está en /srv/agentes/verapdf/inst/verapdf.
func VeraPDF(pdf []byte) ([]string, error) {
	bin := os.Getenv("SYTUE_VERAPDF")
	if bin == "" {
		var err error
		if bin, err = exec.LookPath("verapdf"); err != nil {
			return nil, ErrNoVeraPDF
		}
	}
	dir, err := os.MkdirTemp("", "uacheck")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	f := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(f, pdf, 0o600); err != nil {
		return nil, err
	}
	// veraPDF sale con código distinto de cero si el fichero no es conforme: el informe va igual.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--flavour", "ua1", "--format", "json", "--maxfailuresdisplayed", "2", f).Output() //nolint:gosec // ejecutable fijado por el entorno de desarrollo
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		return nil, err
	}
	var rep struct {
		Report struct {
			Jobs []struct {
				ValidationResult []struct {
					Compliant bool `json:"compliant"`
					Details   struct {
						RuleSummaries []struct {
							RuleStatus  string `json:"ruleStatus"` //nolint:tagliatelle // el nombre lo fija veraPDF
							Clause      string `json:"clause"`
							TestNumber  int    `json:"testNumber"` //nolint:tagliatelle // el nombre lo fija veraPDF
							Description string `json:"description"`
							Checks      []struct {
								ErrorMessage string `json:"errorMessage"` //nolint:tagliatelle // el nombre lo fija veraPDF
							} `json:"checks"`
						} `json:"ruleSummaries"` //nolint:tagliatelle // el nombre lo fija veraPDF
					} `json:"details"`
				} `json:"validationResult"` //nolint:tagliatelle // el nombre lo fija veraPDF
			} `json:"jobs"`
		} `json:"report"`
	}
	if err := json.Unmarshal(out, &rep); err != nil || len(rep.Report.Jobs) == 0 || len(rep.Report.Jobs[0].ValidationResult) == 0 {
		return nil, fmt.Errorf("uacheck: informe de veraPDF ilegible: %w", err)
	}
	r := rep.Report.Jobs[0].ValidationResult[0]
	var fallos []string
	for _, s := range r.Details.RuleSummaries {
		if s.RuleStatus == "FAILED" {
			msg := s.Description
			if len(s.Checks) > 0 {
				msg += " — " + s.Checks[0].ErrorMessage
			}
			fallos = append(fallos, fmt.Sprintf("%s test %d: %s", s.Clause, s.TestNumber, msg))
		}
	}
	if !r.Compliant && len(fallos) == 0 {
		fallos = append(fallos, "veraPDF: no conforme")
	}
	return fallos, nil
}
