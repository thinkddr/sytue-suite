// SPDX-License-Identifier: Apache-2.0
package domain

import "git.sytue.eu/sytue/one/apps/nucleo/notes/model"

func Hashtags(text string) []string                 { return model.Hashtags(text) }
func HashtagDeConsulta(query string) (string, bool) { return model.HashtagDeConsulta(query) }
