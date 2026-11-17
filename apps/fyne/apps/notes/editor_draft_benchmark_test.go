// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/notas"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// BenchmarkEditorDraftPersistence measures UI admission separately from eventual
// durability: editor input preparation, real encrypted Store and the
// real Linux session-file lease. It deliberately excludes layout and SDK/network
// publication. The key provider is disposable in-memory test storage; keyring
// startup and Apple/Binder IPC are not measured. AEAD, files and OS locks are real.
func BenchmarkEditorDraftPersistence(b *testing.B) {
	if runtime.GOOS != "linux" {
		b.Skip("requires remote Linux IO accounting and session-file locks")
	}
	for _, size := range []int{100 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			b.Setenv("XDG_CONFIG_HOME", b.TempDir())
			identity := auth.IdentitySnapshot{Issuer: "https://id.example.test", Client: "notes-benchmark", Subject: "alice", Organization: "acme"}
			storeID := "sesion-cccccccccccccccccccccccccccccccc"
			if err := ajustes.PonerAccesoSesionCon(b.Context(), storeID, identity.IdentidadSesion(), true); err != nil {
				b.Fatal(err)
			}
			access, err := ajustes.LeerAccesoSesionCon(b.Context(), storeID)
			if err != nil {
				b.Fatal(err)
			}
			witness := auth.TestigoTrabajo{Almacen: storeID, Acceso: access}
			directory := b.TempDir()
			journal, err := offline.OpenWithSecrets(directory, &authtest.Secrets{})
			if err != nil {
				b.Fatal(err)
			}
			scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}
			work, err := notas.NewLocalWork(b.Context(), scope, journal, witness.Compartido)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(work.Close)
			html := "<p>" + strings.Repeat("x", size-7) + "</p>"
			note := api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Content: html, UpdatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
			screen := &Pantalla{Almacen: journal, lifetime: vidaPantalla{parent: b.Context(), work: work, dispatch: func(func()) {}}, activo: Cuaderno{ID: "c1"}, notas: DeNotas([]api.Note{note})}
			editing := &sesionEdicion{nota: note.ID, editor: &EditorNota{doc: Leer(html)}}
			persist := func() {
				_, done, err := screen.admitir()
				if err != nil {
					b.Fatal(err)
				}
				defer done()
				if err := screen.conservarEdicion(editing, html); err != nil {
					b.Fatal(err)
				}
			}
			persist()
			<-editing.durable.Done()
			before := draftProcessWrites(b)
			latencies := make([]time.Duration, b.N)
			durableLatencies := make([]time.Duration, b.N)
			b.SetBytes(int64(len(html)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				started := time.Now()
				persist()
				latencies[i] = time.Since(started)
				<-editing.durable.Done()
				if durable, err := editing.durable.Status(); !durable || err != nil {
					b.Fatal(durable, err)
				}
				durableLatencies[i] = time.Since(started)
			}
			b.StopTimer()
			after := draftProcessWrites(b)
			slices.Sort(latencies)
			slices.Sort(durableLatencies)
			b.ReportMetric(float64(latencies[len(latencies)/2].Nanoseconds()), "admission-p50-ns/op")
			b.ReportMetric(float64(latencies[(len(latencies)*95+99)/100-1].Nanoseconds()), "admission-p95-ns/op")
			b.ReportMetric(float64(durableLatencies[(len(durableLatencies)*95+99)/100-1].Nanoseconds()), "durable-p95-ns/op")
			b.ReportMetric(float64(after.syscall-before.syscall)/float64(b.N), "write-syscall-B/op")
			b.ReportMetric(float64(after.disk-before.disk)/float64(b.N), "disk-write-B/op")
			total := int64(0)
			err = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() {
					info, err := entry.Info()
					if err != nil {
						return err
					}
					total += info.Size()
				}
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportMetric(float64(total), "sealed-directory-B")
		})
	}
}

type draftWriteCounters struct{ syscall, disk uint64 }

func draftProcessWrites(b *testing.B) draftWriteCounters {
	b.Helper()
	raw, err := os.ReadFile("/proc/self/io")
	if err != nil {
		b.Skip("requires remote Linux IO accounting")
	}
	var result draftWriteCounters
	for line := range strings.SplitSeq(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			b.Fatal(err)
		}
		switch fields[0] {
		case "wchar:":
			result.syscall = value
		case "write_bytes:":
			result.disk = value
		}
	}
	return result
}
