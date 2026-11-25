// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// # El cobro incierto sobrevive a cerrar la app (ERP-09)
//
// La web guarda la petición de cobro ANTES de enviarla en su almacén sin conexión
// (`mutateOfflineValue(cuenta, 'erp-tpv', 'cobro')`), acotada por organización y
// persona: recargar la página tras perder la respuesta recupera la misma operación, y
// otra pestaña no puede pisar un cobro todavía sin confirmar. Aquí es lo mismo con el
// almacén sin conexión de la suite (`sesion.Almacen`, cifrado), a nombre de la cuenta
// —emisor, persona y organización—: el mismo espacio y la misma clave que la web, y
// la misma forma (`CobroPendiente`).
//
// Se borra al confirmarse el cobro, con un rechazo de validación del PRIMER envío (que
// prueba que el servicio no registró nada) y con «Nueva venta». Cualquier otro final
// —sin red, un 5xx, cerrar la app a medias— lo deja para reintentar.

// Espacio y clave del cobro pendiente en el almacén, los de la web.
const (
	EspacioCobroTPV = "erp-tpv"
	ClaveCobroTPV   = "cobro"
)

// CobroPendienteTPV es `CobroPendiente` de PuntoVenta.tsx: el cuerpo enviado, la cesta
// tal como se vio y el nombre del cliente, para volver a pintarla igual.
type CobroPendienteTPV struct {
	Cuerpo        api.ERPCobroTPV `json:"cuerpo"`
	Lineas        []LineaTPV      `json:"lineas"`
	NombreCliente string          `json:"nombre_cliente"`
}

// ErrOtroCobroTPV: ya hay guardado OTRO cobro sin confirmar (otra ventana, o uno que se
// recuperó después de armar esta cesta). No se pisa: se recupera y se reintenta ese.
var ErrOtroCobroTPV = errors.New("erp: hay otro cobro del punto de venta sin confirmar")

// AlmacenCobroTPV es donde vive el cobro pendiente.
type AlmacenCobroTPV interface {
	// Leer es el cobro guardado, o nil si no hay ninguno.
	Leer() (*CobroPendienteTPV, error)
	// Guardar lo deja guardado, salvo que haya otro con distinta operación
	// (ErrOtroCobroTPV).
	Guardar(c CobroPendienteTPV) error
	// Quitar borra el de esa operación; otro distinto se queda.
	Quitar(operacion string) error
}

// AlmacenCobroTPVDe guarda el cobro pendiente en `s` a nombre de la cuenta que diga
// `cuenta` en cada momento. Sin almacén o sin nadie dentro devuelve nil: el cobro vive
// lo que la pantalla, como antes.
func AlmacenCobroTPVDe(s *offline.Store, cuenta func() string) AlmacenCobroTPV {
	if s == nil || cuenta == nil {
		return nil
	}
	return almacenCobroTPV{s: s, cuenta: cuenta}
}

type almacenCobroTPV struct {
	s      *offline.Store
	cuenta func() string
}

func (a almacenCobroTPV) dueno() (string, bool) {
	c := a.cuenta()
	return c, c != ""
}

func (a almacenCobroTPV) Leer() (*CobroPendienteTPV, error) {
	dueno, ok := a.dueno()
	if !ok {
		return nil, nil //nolint:nilnil // sin nadie dentro no hay nada guardado
	}
	var c CobroPendienteTPV
	hay, err := a.s.Read(dueno, EspacioCobroTPV, ClaveCobroTPV, &c)
	if err != nil || !hay {
		return nil, err
	}
	return &c, nil
}

func (a almacenCobroTPV) Guardar(c CobroPendienteTPV) error {
	dueno, ok := a.dueno()
	if !ok {
		return nil
	}
	return a.s.Mutate(dueno, EspacioCobroTPV, ClaveCobroTPV, func(anterior json.RawMessage) (any, error) {
		if previo, ok := cobroDe(anterior); ok && previo.Cuerpo.Operacion != c.Cuerpo.Operacion {
			return nil, ErrOtroCobroTPV
		}
		return c, nil
	})
}

func (a almacenCobroTPV) Quitar(operacion string) error {
	dueno, ok := a.dueno()
	if !ok {
		return nil
	}
	return a.s.Mutate(dueno, EspacioCobroTPV, ClaveCobroTPV, func(anterior json.RawMessage) (any, error) {
		if previo, ok := cobroDe(anterior); ok && previo.Cuerpo.Operacion != operacion {
			return previo, nil // otro cobro: se queda como estaba
		}
		return nil, nil
	})
}

func cobroDe(crudo json.RawMessage) (CobroPendienteTPV, bool) {
	var c CobroPendienteTPV
	if len(crudo) == 0 || string(crudo) == "null" || json.Unmarshal(crudo, &c) != nil {
		return c, false
	}
	return c, c.Cuerpo.Operacion != ""
}

// guardarPendiente guarda el cobro que se va a enviar con la cesta de ahora.
func (t *TPV) guardarPendiente(c api.ERPCobroTPV) error {
	if t.Pendientes == nil {
		return nil
	}
	return t.Pendientes.Guardar(CobroPendienteTPV{Cuerpo: c, Lineas: t.Lineas, NombreCliente: t.NombreCliente})
}

// quitarPendiente borra el cobro guardado de la operación en curso. Dice si pudo (o si
// no había nada que borrar).
func (t *TPV) quitarPendiente() bool {
	if t.Pendientes == nil || t.pendiente == nil {
		return true
	}
	return t.Pendientes.Quitar(t.pendiente.Operacion) == nil
}

// Restaurar recupera el cobro guardado, si lo hay y no se está armando ya otro: la
// cesta, la caja, el cliente, el medio y lo recibido vuelven tal cual y el cobro queda
// incierto, para reintentarlo con la MISMA operación. Es el `recuperarCobro` de la web.
func (t *TPV) Restaurar() {
	t.restaurado = true
	if t.Pendientes == nil || t.Ocupado || t.Venta != nil {
		return
	}
	c, err := t.Pendientes.Leer()
	if err != nil || c == nil {
		return
	}
	if t.pendiente != nil && t.pendiente.Operacion == c.Cuerpo.Operacion {
		return
	}
	cuerpo := c.Cuerpo
	t.pendiente, t.operacion = &cuerpo, cuerpo.Operacion
	t.Lineas = c.Lineas
	t.Sesion = cuerpo.Sesion
	t.Cliente, t.NombreCliente = cuerpo.Cliente, c.NombreCliente
	t.cliente.Valor, t.cliente.Etiqueta = cuerpo.Cliente, c.NombreCliente
	t.cliente.Refresh()
	t.Medio = cuerpo.Medio
	t.Recibido = strconv.FormatFloat(float64(cuerpo.Recibido)/100, 'f', -1, 64)
	// Un reintento tras recuperar no prueba nada con un rechazo: se conserva.
	t.Incierto, t.recuperado = true, true
}

// --- «Crear presupuesto» -----------------------------------------------------------

// CrearPresupuesto manda la cesta como presupuesto del cliente elegido. Sin cliente no
// hay presupuesto («Obligatorio: Cliente», como la web).
func (t *TPV) CrearPresupuesto() {
	if !t.Editable() || t.AlPresupuesto == nil || len(t.Lineas) == 0 {
		return
	}
	if t.Cliente == "" {
		t.Fallo = t.t("form.required") + ": " + t.t("pos.customer")
		t.cambio()
		return
	}
	if t.ofertaOperacion == "" {
		t.ofertaOperacion = uuid.NewString()
	}
	p := api.ERPPresupuestoTPV{Operacion: t.ofertaOperacion, Cliente: t.Cliente}
	for _, l := range t.Lineas {
		precio, _ := PrecioTPV(l.Producto)
		p.Lineas = append(p.Lineas, api.ERPLineaTPV{Producto: l.Producto.ID, Cantidad: l.Cantidad, Precio: precio, TipoIVA: tipoTPV(l.Producto)})
	}
	t.Ocupado, t.Fallo = true, ""
	t.cambio()
	t.AlPresupuesto(p)
}

// Presupuestado recibe la respuesta. Con éxito quien llama abre el presupuesto; la cesta
// se queda, por si se cobra después.
func (t *TPV) Presupuestado(err error, mensaje string) {
	t.Ocupado = false
	if err != nil {
		t.Fallo = mensaje
	}
}
