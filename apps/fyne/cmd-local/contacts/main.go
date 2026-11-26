// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command contacts es Sytue Contacts de Sytue Office: las mismas libretas, ficha y
// formulario de la suite, con los contactos en este equipo y en claro, sin cuenta y sin
// servidor (apps/fyne/PLAN-EDICION-LOCAL.md §6.6).
//
// Comparado con `cmd/contacts`: no hay `sesion.Abrir` ni `cifrado.NewPrivateApplication`.
// La libreta es la base SQLite de `nucleo/local/libreta` (`Datos/contacts/libreta.sqlite`,
// el plan B de PLAN-EDICION-LOCAL §5.6): con índices, y con las MISMAS reglas del
// repositorio de Contacts (`apps/nucleo/contacts`), que prepara cada escritura. La
// fachada `api.Contacts` va enchufada a ella. La primera vez que se abre con un almacén
// de registros de antes en la carpeta (el de las versiones anteriores), pasa todo a la
// base y lo comprueba, con una pantalla de avance como la de Photos; el almacén viejo se
// queda como estaba y no se borra solo (se borra desde Ajustes → Datos, con la migración
// comprobada: `local/ajustes_antes.go`).
//
// No se montan: la libreta «Organización» y el directorio (`Directorio` nil), «Trae tus
// contactos» ni los servicios vinculados. Exportar a vCard (`BatchVCard`) y deshacer un
// borrado en lote (`BatchRestore`) tienen camino local en `nucleo/contacts/lote.go`.
//
// Cuentas (K-05, K-06): las libretas de las cuentas CardDAV, Google y Microsoft que la
// persona dio de alta en Ajustes → Cuentas se sincronizan con `local/agenda`
// (`cuentas.go`) y salen agrupadas por cuenta en el menú.
//
// Importar `.vcf` y `.csv` va por el selector del sistema y por la fachada local; además,
// un `.vcf` o `.csv` que llega por «Abrir con», por los argumentos o soltado en la ventana
// entra en la libreta abierta por el mismo camino de la pantalla, sin HTTP.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/contactos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
)

func main() {
	a := local.NuevaApp("eu.sytue.office.contacts")
	l, err := local.Abrir(a, "contacts")
	if err != nil {
		slog.Error("contacts", "error", err)
		os.Exit(1)
	}
	tema.ElegirCromo(tema.CromoPapel)
	t, err := l.Traductor(catalogos.Contacts())
	if err != nil {
		panic(err)
	}

	w := a.NewWindow(a.Metadata().Name)
	if a.Metadata().Name == "" {
		w.SetTitle("Sytue Contacts")
	}
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))

	// La primera vez con la libreta del almacén de antes, se pasa a la base (§5.6, plan B)
	// enseñando cómo va, como Photos: con 20.000 contactos son unos segundos
	// (nucleo/local/libreta/migrar.go).
	if libreta.HayQueMigrar(l.Espacio.Rutas.Producto("contacts")) {
		avance := pantallaDeMigrar(l, w)
		go func() {
			repo, err := abrirLibreta(context.Background(), l, avance)
			fyne.Do(func() {
				if err != nil {
					fallaLaLibreta(l, w, err)
					return
				}
				if err := montar(a, l, w, t, repo); err != nil {
					slog.Error("contacts", "error", err)
					os.Exit(1)
				}
			})
		}()
		w.ShowAndRun()
		return
	}
	repo, err := abrirLibreta(context.Background(), l, nil)
	if err != nil {
		slog.Error("contacts: no se pudo abrir la libreta local", "error", err)
		os.Exit(1)
	}
	if err := montar(a, l, w, t, repo); err != nil {
		slog.Error("contacts", "error", err)
		os.Exit(1)
	}
	w.ShowAndRun()
}

// pantallaDeMigrar pone en la ventana que se están poniendo al día los contactos, con una
// barra (sin medida mientras se lee la libreta de antes; con ella, al pasarla), y da cómo
// moverla. Es la de Photos (cmd-local/photos/main.go).
func pantallaDeMigrar(l *local.App, w fyne.Window) func(libreta.Avance) {
	barra := widget.NewProgressBarInfinite()
	medida := widget.NewProgressBar()
	medida.Hide()
	cuerpo := container.NewVBox(ui.NuevoVacio(texto(l, "migrar"), texto(l, "migrar.texto")), container.NewStack(barra, medida))
	w.SetContent(l.Montar(w, iconos.Contacts, container.NewCenter(container.NewGridWrap(fyne.NewSize(520, cuerpo.MinSize().Height), cuerpo))))
	return func(a libreta.Avance) {
		if a.Fase != libreta.FaseCopiar || a.Total == 0 {
			return
		}
		fyne.Do(func() {
			if barra.Visible() {
				barra.Stop()
				barra.Hide()
				medida.Show()
			}
			medida.SetValue(float64(a.Hechas) / float64(a.Total))
		})
	}
}

// fallaLaLibreta dice en la ventana que los contactos no se pudieron pasar a la libreta
// nueva, y por qué: siguen en el almacén de antes y se vuelve a intentar al abrir.
func fallaLaLibreta(l *local.App, w fyne.Window, err error) {
	slog.Error("contacts: no se pudo abrir la libreta local", "error", err)
	w.SetContent(l.Montar(w, iconos.Contacts, ui.NuevoVacio(texto(l, "migrar.fallo"), err.Error())))
}

// montar pone la libreta abierta en la ventana: la pantalla compartida de Contacts con lo
// que esta edición le añade.
func montar(a fyne.App, l *local.App, w fyne.Window, t *i18n.Translator, repo *libreta.Libreta) error {
	ctx := context.Background()
	// Eliminar para siempre se va del disco en el acto (local/libreta, vaciado.go).
	facade, err := api.NewContacts("", nil, nil).WithPrivateData(repo)
	if err != nil {
		return err
	}

	c := contactos.NuevaApp(t, facade)
	alcance := repo.PrivateStorageScope()
	c.Yo = func() string { return alcance.Subject }
	c.Org = func() string { return alcance.Tenant }
	// El selector de ficheros, salvo que haya un fichero que llegó de fuera esperando: ese
	// se entrega sin abrir el selector (ver `entregarFichero`).
	sel := &selector{}
	if runtime.GOOS != "js" {
		c.ElegirFichero = sel.elegir
	}
	// IA «Rellenar» con la IA de Ajustes (K-03): la pantalla recibe un `api.Asistente`.
	c.IA = asistente(l.Asistir())
	l.AlCambiarIA(func() { c.IA = asistente(l.Asistir()) })

	// Las libretas de las cuentas (CardDAV, Google, Microsoft), agrupadas por cuenta y
	// sincronizadas solas (K-05, K-06); arrancan con la pantalla, más abajo.
	cuentas := nuevaSincronia(l, repo)
	l.AlAbrirFichero(func(_ context.Context, ap api.Apertura) error {
		nombre, datos, err := local.LeerLocal(ap.Local)
		if err != nil {
			return err
		}
		var importar func()
		switch strings.ToLower(path.Ext(strings.ReplaceAll(nombre, `\`, "/"))) {
		case ".vcf", ".vcard":
			importar = c.Pantalla.AlImportarVcard
		case ".csv":
			importar = c.Pantalla.AlImportarCSV
		}
		if importar == nil {
			return errNoEsLibreta
		}
		// Lo que llega al abrir la app (los argumentos) llega antes que las libretas: la
		// pantalla importa en la ABIERTA, así que se espera a que haya una.
		if !esperarLibreta(c) {
			return errSinLibreta
		}
		soloLectura := false
		fyne.DoAndWait(func() { soloLectura = c.Pantalla.DeSoloLectura() })
		if soloLectura {
			// Lo importado no subiría a la cuenta: no se importa.
			return errSoloLectura
		}
		fyne.Do(func() {
			sel.pendiente = datos
			importar()
			sel.pendiente = nil // si la pantalla no lo pidió (sin libreta abierta), se tira
		})
		return nil
	}, func(err error) {
		if errors.Is(err, errSoloLectura) {
			local.Info(w, texto(l, "sync.sololectura"), texto(l, "sync.sololectura.aviso"))
			return
		}
		l.Fallo(w, err)
	})
	l.AceptarFicheros(w)
	// «Exportar en formatos abiertos»: todos los contactos en vCard y en CSV.
	l.AlExportarTodo(func(ctx context.Context, carpeta string) error { return exportar.Contactos(ctx, repo, carpeta) })
	// «Guardar en la nube»: todos los contactos en CSV.
	l.AlExportar(func() (string, []byte, error) { return repo.ExportCSV(context.Background()) })

	w.SetContent(l.Montar(w, iconos.Contacts, c.Objeto()))
	// Las letras de la web (n, j, k, /, e, s, x, ?).
	w.Canvas().SetOnTypedRune(c.Atajo)
	l.AlCerrar(c.Cerrar)
	cuentas.arrancar(ctx, a, c, w)
	c.IniciarCarga()
	// El contacto que manda el Hub (`?clase=contacto&id=…`) o un aviso, en su ficha.
	l.AlAbrirElemento(func(clase, id string) {
		if clase == "contacto" {
			c.AbrirContacto(id)
		}
	})
	return nil
}

var (
	errNoEsLibreta = errors.New("contacts: el fichero no es una vCard (.vcf) ni un CSV")
	errSinLibreta  = errors.New("contacts: no hay ninguna libreta abierta en la que importar")
	errSoloLectura = errors.New("contacts: la libreta abierta es de solo lectura en su cuenta")
)

// esperarLibreta espera, fuera del hilo de la interfaz, a que la pantalla tenga una
// libreta abierta y sin otra importación en marcha. Corre en la gorutina de entrega.
func esperarLibreta(c *contactos.App) bool {
	for range 100 {
		lista := false
		fyne.DoAndWait(func() { lista = c.Pantalla.Activa != nil && !c.Pantalla.ImportOcupado })
		if lista {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// abrirLibreta abre la libreta de este equipo (local/libreta) y, la primera vez, pasa a
// ella lo del almacén de registros de antes, si lo hay: Contacts es de una sola copia y
// las demás apps que escriben en Contacts (el Hub al traer) lo hacen con Contacts cerrada.
// Sin ninguna libreta crea la personal: la pantalla sin ninguna no deja crear contactos.
// La base entra en la copia de seguridad en caliente. avance, si no es nil, ve cómo va la
// migración.
func abrirLibreta(ctx context.Context, l *local.App, avance func(libreta.Avance)) (*libreta.Libreta, error) {
	repo, err := libreta.AbrirCon(ctx, l.Espacio.Rutas.Producto("contacts"), l.Espacio.ID, libreta.Opciones{Avance: avance})
	if err != nil {
		return nil, err
	}
	l.AlCerrar(repo.Close)
	l.AlCerrar(l.Copia.BaseAbierta(repo.Ruta(), repo.CopiarEnCaliente))
	libros, err := repo.AddressBooks(ctx)
	if err != nil {
		return nil, err
	}
	if len(libros) == 0 {
		if _, err := repo.CreateAddressBook(ctx, texto(l, "libreta")); err != nil {
			slog.Warn("contacts: no se pudo crear la libreta personal", "error", err)
		}
	}
	return repo, nil
}

// selector es App.ElegirFichero: entrega el fichero que llegó de fuera si lo hay y, si
// no, abre el selector del sistema. Solo se toca en el hilo de la interfaz.
type selector struct{ pendiente []byte }

func (s *selector) elegir(extensiones []string, listo func(datos []byte)) {
	if datos := s.pendiente; datos != nil {
		s.pendiente = nil
		listo(datos)
		return
	}
	elegirFichero(extensiones, listo)
}

// elegirFichero es el de la edición conectada: el selector del sistema, el primer
// fichero de esas extensiones, y su contenido a `listo` en el hilo de la interfaz.
func elegirFichero(extensiones []string, listo func(datos []byte)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("contacts: el selector de ficheros ha fallado", "error", err)
			}
			return
		}
		for _, r := range rutas {
			if !slices.Contains(extensiones, strings.ToLower(filepath.Ext(r))) {
				continue
			}
			datos, err := os.ReadFile(r)
			if err != nil {
				slog.Warn("contacts: no se puede leer el fichero elegido", "error", err)
				return
			}
			fyne.Do(func() { listo(datos) })
			return
		}
	}()
}

// asistente es la IA de Ajustes como `api.Asistente`; nil si está apagada (y entonces el
// formulario no ofrece «Rellenar»). Un nil de interfaz, no una función nil envuelta.
func asistente(asistir func(context.Context, api.PeticionIA) (string, error)) api.Asistente {
	if asistir == nil {
		return nil
	}
	return api.FuncionAsistente(asistir)
}
