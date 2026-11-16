// SPDX-License-Identifier: Apache-2.0
// Package domain keeps the service surface; authorized devices use the same exporter.
package domain

import "git.sytue.eu/sytue/one/apps/nucleo/notes/model"

type FormatoExport = model.FormatoExport

const (
	FormatoMarkdown      = model.FormatoMarkdown
	FormatoMarkdownLargo = model.FormatoMarkdownLargo
	FormatoTextoPlano    = model.FormatoTextoPlano
	FormatoHTML          = model.FormatoHTML
	FormatoNativo        = model.FormatoNativo
	FormatoPDF           = model.FormatoPDF
)

func FormatoDe(v string) (FormatoExport, bool) { return model.FormatoDe(v) }
func Exportar(nb Notebook, notes []Note, format FormatoExport) []byte {
	return model.Exportar(nb, notes, format)
}

func NotebookAMarkdown(nb Notebook, notes []Note) string { return model.NotebookAMarkdown(nb, notes) }
func TituloDeNota(note Note) string                      { return model.TituloDeNota(note) }
func NombreDeFicheroExportado(title string, format FormatoExport) string {
	return model.NombreDeFicheroExportado(title, format)
}

func NotebookATextoPlano(nb Notebook, notes []Note) string {
	return model.NotebookATextoPlano(nb, notes)
}
func TextoDeHTML(value string) string                  { return model.TextoDeHTML(value) }
func NotebookAHTML(nb Notebook, notes []Note) string   { return model.NotebookAHTML(nb, notes) }
func NotebookAPDF(nb Notebook, notes []Note) []byte    { return model.NotebookAPDF(nb, notes) }
func NotebookANativo(nb Notebook, notes []Note) string { return model.NotebookANativo(nb, notes) }
func ValidateEncryptedBackup(nb Notebook, notes []Note) error {
	return model.ValidateEncryptedBackup(nb, notes)
}

func LeerNativo(raw []byte) (Notebook, []Note, error) { return model.LeerNativo(raw) }

func LeerNativoTransferencia(raw []byte) (Notebook, []Note, error) {
	return model.LeerNativoTransferencia(raw)
}

// LeerImportable acepta el `.onex` (con la lectura estricta de transferencia),
// Markdown suelto o en ZIP y Evernote (.enex).
func LeerImportable(raw []byte, nombre string) (Notebook, []Note, error) {
	return model.LeerImportable(raw, nombre)
}

func ValidateEncryptedBackupFormat(nb Notebook, format FormatoExport) error {
	return model.ValidateEncryptedBackupFormat(nb, format)
}

func ValidateBackupUpload(nb Notebook, notes []Note) error {
	return model.ValidateBackupUpload(nb, notes)
}
