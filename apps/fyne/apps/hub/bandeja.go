// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"slices"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El ciclo de la bandeja de avisos: cargar, escuchar el flujo mientras se mira, y las
// tres acciones —abrir, marcar todas y silenciar—, todas optimistas como en la web: el
// acuse del Hub no cambia nada que la persona tenga que esperar.

// EsperaFlujo es lo que se espera para reconectar el flujo cuando se corta. La isla de
// la web no reintenta por su cuenta: lo hace `EventSource`, con su `retry` por defecto
// de unos segundos y sin espera creciente. El panel de la campana sí la lleva
// (`ui.EsperaAvisosMax`) porque vive en las trece apps a la vez; la bandeja es UNA
// pantalla abierta.
const EsperaFlujo = 3 * time.Second

// ConBuzon conecta la bandeja con el Hub.
func (v *Vista) ConBuzon(b Buzon) *Vista {
	v.buzon = b
	v.avisos.AlAbrir = v.abrirAviso
	v.avisos.AlMarcarLeido = v.marcarLeido
	v.avisos.AlMarcarTodas = v.marcarTodas
	v.avisos.AlSilenciar = v.silenciar
	v.avisos.AlCargarMas = v.cargarMas
	v.avisos.AlReintentarReglas = func() {
		v.mu.Lock()
		ctx := v.ctxBandeja
		v.mu.Unlock()
		if ctx != nil {
			go v.cargarReglas(ctx)
		}
	}
	v.avisos.AlReintentar = func() {
		v.cerrarBandeja()
		v.abrirBandeja()
	}
	return v
}

// abrirBandeja carga los avisos y se queda escuchando. Si ya escucha, no hace nada.
func (v *Vista) abrirBandeja() {
	if v.buzon == nil {
		return
	}
	v.mu.Lock()
	if v.pararFlujo != nil {
		v.mu.Unlock()
		return
	}
	ctx, parar := context.WithCancel(context.Background())
	v.pararFlujo, v.ctxBandeja = parar, ctx
	v.mu.Unlock()
	go v.cargarAvisos(ctx)
	go v.cargarReglas(ctx)
	go v.escucharAvisos(ctx)
}

// RecargarAvisos vuelve a pedir la lista si la bandeja está abierta. Lo usa el
// receptor de la entrada privada de One (ADR 0086): al guardar en el almacén privado
// un aviso nuevo, la lista de la bandeja ya lo tiene y hay que volver a leerla. Con la
// bandeja cerrada no hace nada: se carga al abrirla.
func (v *Vista) RecargarAvisos() {
	v.mu.Lock()
	ctx := v.ctxBandeja
	abierta := v.pararFlujo != nil && ctx != nil && ctx.Err() == nil
	v.mu.Unlock()
	if abierta {
		go v.cargarAvisos(ctx)
	}
}

// cerrarBandeja corta el flujo. Al volver se carga de nuevo, como al remontar la isla.
func (v *Vista) cerrarBandeja() {
	v.mu.Lock()
	parar := v.pararFlujo
	v.pararFlujo = nil
	v.mu.Unlock()
	if parar != nil {
		parar()
	}
}

// cargarAvisos pide los cien últimos. Las reglas van aparte (`cargarReglas`): si el Hub
// no las sirve, la lista sigue siendo útil.
func (v *Vista) cargarAvisos(ctx context.Context) {
	v.enUI(func() { v.avisos.Estado, v.avisos.Items, v.avisos.HayMas = Cargando, nil, false; v.generacion++ })
	c, cancelar := context.WithTimeout(ctx, 15*time.Second)
	defer cancelar()

	items, err := v.buzon.Notifications(c, MaxAvisosBandeja)
	if ctx.Err() != nil {
		return // se ha ido de la pantalla: nada que pintar
	}
	v.enUI(func() {
		v.avisos.Items, v.avisos.Estado = items, EstadoDe(err)
		v.avisos.HayMas = err == nil && len(items) >= MaxAvisosBandeja
	})
}

// cargarReglas pide las reglas de silencio con su propio ciclo. Antes, si fallaban, la
// sección se quedaba con todo ENCENDIDO aunque hubiera cosas calladas; ahora lo dice y
// no ofrece interruptores hasta saber en qué posición están.
func (v *Vista) cargarReglas(ctx context.Context) {
	org := v.org()
	v.enUI(func() {
		v.avisos.CargaReglas, v.avisos.Organizacion = ReglasCargando, org
		v.avisos.FalloRegla = nil
	})
	c, cancelar := context.WithTimeout(ctx, 15*time.Second)
	defer cancelar()
	reglas, err := v.buzon.NotificationPrefs(c)
	if ctx.Err() != nil {
		return
	}
	v.enUI(func() {
		if err != nil {
			v.avisos.CargaReglas = ReglasFallo
			return
		}
		v.avisos.Reglas, v.avisos.CargaReglas = reglas, ReglasListas
	})
}

// cargarMas trae la página de detrás del último aviso de la lista y la añade sin
// repetir: lo de «Cargar más» de la bandeja de la web (`masAntiguos`).
func (v *Vista) cargarMas() {
	v.mu.Lock()
	ctx, gen := v.ctxBandeja, v.generacion
	var ultimo api.HubNotification
	hay := ctx != nil && ctx.Err() == nil && len(v.avisos.Items) > 0 && v.avisos.HayMas && !v.cargandoMas
	if hay {
		ultimo = v.avisos.Items[len(v.avisos.Items)-1]
		v.cargandoMas = true
	}
	v.mu.Unlock()
	if !hay {
		return
	}
	// Con el contexto de la bandeja: si se sale de la pantalla, se cancela.
	go func() {
		c, cancelar := context.WithTimeout(ctx, 15*time.Second)
		defer cancelar()
		mas, err := v.buzon.NotificationsAntes(c, MaxAvisosBandeja, ultimo)
		v.enUI(func() {
			v.cargandoMas = false
			// Un error deja el botón para volver a probar. Y si la lista ya es otra
			// (se recargó, o se fue de la pantalla), esta página no es de ella.
			if err != nil || ctx.Err() != nil || gen != v.generacion ||
				len(v.avisos.Items) == 0 || v.avisos.Items[len(v.avisos.Items)-1].ID != ultimo.ID {
				return
			}
			for _, n := range mas {
				if !slices.ContainsFunc(v.avisos.Items, func(x api.HubNotification) bool { return x.ID == n.ID }) {
					v.avisos.Items = append(v.avisos.Items, n)
				}
			}
			v.avisos.HayMas = len(mas) >= MaxAvisosBandeja
		})
	}()
}

// escucharAvisos mantiene el flujo mientras la bandeja está abierta.
func (v *Vista) escucharAvisos(ctx context.Context) {
	for ctx.Err() == nil {
		_ = v.buzon.StreamNotifications(ctx, v.llegaAviso)
		select {
		case <-ctx.Done():
			return
		case <-time.After(EsperaFlujo):
		}
	}
}

// llegaAviso pone delante lo que trae el flujo, sin repetirlo: puede reenviar lo que ya
// se cargó.
func (v *Vista) llegaAviso(n api.HubNotification) {
	v.enUI(func() {
		v.avisos.Items = append([]api.HubNotification{n},
			slices.DeleteFunc(v.avisos.Items, func(x api.HubNotification) bool { return x.ID == n.ID })...)
		if v.avisos.Estado == Cargando {
			v.avisos.Estado = Listo
		}
	})
}

// abrirAviso lo marca como leído y, si lleva a algún sitio, lo abre EN SU APP: el aviso
// es de otro producto de la suite, igual que en el panel de la campana.
func (v *Vista) abrirAviso(n api.HubNotification) {
	v.marcarLeido(n)
	if n.URL == "" || v.abrir == nil {
		return
	}
	clave := n.Source
	if p, ok := ProductoDe(n.Source); ok {
		clave = p.Marca.Clave()
	}
	_ = api.AbrirURL(v.abrir, clave, n.URL)
}

// marcarLeido marca UN aviso como leído sin abrirlo: el punto azul de la fila, que es un
// botón. Optimista, como en la web (`avisos.leer`).
func (v *Vista) marcarLeido(n api.HubNotification) {
	v.enUI(func() {
		for i := range v.avisos.Items {
			if v.avisos.Items[i].ID == n.ID {
				v.avisos.Items[i].Read = true
			}
		}
	})
	v.segundoPlano(func(ctx context.Context) { _ = v.buzon.MarkRead(ctx, n.ID) })
}

func (v *Vista) marcarTodas() {
	v.enUI(func() {
		for i := range v.avisos.Items {
			v.avisos.Items[i].Read = true
		}
	})
	v.segundoPlano(func(ctx context.Context) { _ = v.buzon.MarkAllRead(ctx) })
}

// silenciar es optimista, como en la web, pero no a ciegas: si el Hub no lo guarda se
// deshace SOLO la regla de ese origen —las demás pueden haberse guardado entretanto— y
// su fila lo dice. Mientras guarda, el interruptor no admite otro clic.
func (v *Vista) silenciar(origen string, silenciar bool) {
	// Bajo el cerrojo y no en enUI: `antes` y `ocupado` hacen falta ya, y enUI puede
	// correr más tarde.
	v.mu.Lock()
	if v.avisos.GuardandoRegla[origen] {
		v.mu.Unlock()
		return
	}
	antes := Silenciado(v.avisos.Reglas, origen)
	v.avisos.Reglas = ConRegla(v.avisos.Reglas, origen, silenciar)
	v.avisos.GuardandoRegla = conClave(v.avisos.GuardandoRegla, origen, true)
	v.avisos.FalloRegla = conClave(v.avisos.FalloRegla, origen, false)
	v.mu.Unlock()
	v.enUI(func() {}) // repinta
	v.segundoPlano(func(ctx context.Context) {
		_, err := v.buzon.SetNotificationPref(ctx, api.HubNotificationPref{Source: origen, Muted: silenciar})
		v.enUI(func() {
			if err != nil {
				v.avisos.Reglas = ConRegla(v.avisos.Reglas, origen, antes)
				v.avisos.FalloRegla = conClave(v.avisos.FalloRegla, origen, true)
			}
			v.avisos.GuardandoRegla = conClave(v.avisos.GuardandoRegla, origen, false)
		})
	})
}

// conClave devuelve una copia del conjunto con la clave dentro o fuera: el renderer lo
// lee en su rutina y no se le cambia por debajo.
func conClave(m map[string]bool, clave string, dentro bool) map[string]bool {
	out := make(map[string]bool, len(m)+1)
	for k := range m {
		out[k] = true
	}
	if dentro {
		out[clave] = true
	} else {
		delete(out, clave)
	}
	return out
}

// segundoPlano lanza un acuse al Hub con su propio plazo. Falla en silencio, como en la
// web: perder un «visto» no puede quitarle a nadie lo que acaba de pulsar.
func (v *Vista) segundoPlano(f func(context.Context)) {
	go func() {
		ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelar()
		f(ctx)
	}()
}
