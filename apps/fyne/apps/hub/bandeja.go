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
	v.avisos.AlMarcarTodas = v.marcarTodas
	v.avisos.AlSilenciar = v.silenciar
	v.avisos.AlCargarMas = v.cargarMas
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
	go v.escucharAvisos(ctx)
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

// cargarAvisos pide los cien últimos y, aparte, las reglas: si el Hub no sirve las
// reglas, la bandeja sigue siendo útil y solo se queda esa sección con lo de siempre.
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

	reglas, err := v.buzon.NotificationPrefs(c)
	if err == nil && ctx.Err() == nil {
		v.enUI(func() { v.avisos.Reglas = reglas })
	}
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
	v.enUI(func() {
		for i := range v.avisos.Items {
			if v.avisos.Items[i].ID == n.ID {
				v.avisos.Items[i].Read = true
			}
		}
	})
	v.segundoPlano(func(ctx context.Context) { _ = v.buzon.MarkRead(ctx, n.ID) })
	if n.URL == "" || v.abrir == nil {
		return
	}
	clave := n.Source
	if p, ok := ProductoDe(n.Source); ok {
		clave = p.Marca.Clave()
	}
	_ = api.AbrirURL(v.abrir, clave, n.URL)
}

func (v *Vista) marcarTodas() {
	v.enUI(func() {
		for i := range v.avisos.Items {
			v.avisos.Items[i].Read = true
		}
	})
	v.segundoPlano(func(ctx context.Context) { _ = v.buzon.MarkAllRead(ctx) })
}

func (v *Vista) silenciar(origen string, silenciar bool) {
	v.enUI(func() { v.avisos.Reglas = ConRegla(v.avisos.Reglas, origen, silenciar) })
	v.segundoPlano(func(ctx context.Context) {
		_, _ = v.buzon.SetNotificationPref(ctx, api.HubNotificationPref{Source: origen, Muted: silenciar})
	})
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
