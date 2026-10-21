// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

// atras sigue las salidas que ya ofrecen la lista y las migas: selección, búsqueda,
// carpeta superior y Mi unidad. En su raíz deja volver al sistema.
func (p *Pantalla) atras() bool {
	e := p.explor
	if p.seccion == MiUnidad {
		switch {
		case e.Marcados() > 0:
			p.marcar("")
		case e.EnBusqueda:
			p.salirDeLaBusqueda()
			p.navegar(len(e.Ruta) - 1)
		case len(e.Ruta) > 0:
			p.navegar(len(e.Ruta) - 2)
		default:
			return false
		}
		return true
	}
	if c := p.sec.compartido; p.seccion == CompartidoConmigo && c.Raiz != nil {
		switch {
		case c.Subcamino != "" && c.AlIr != nil:
			c.AlIr(carpetaDeRuta(c.Subcamino))
		case c.AlVolver != nil:
			c.AlVolver()
		default:
			return false
		}
		return true
	}
	if q := p.sec.equipos; p.seccion == UnidadesDeEquipo && q.Abierta != nil {
		switch {
		case len(q.Pila) > 0 && q.AlIr != nil:
			q.AlIr(len(q.Pila) - 2)
		case q.AlVolver != nil:
			q.AlVolver()
		default:
			return false
		}
		return true
	}
	if q := p.sec.nube; p.seccion == Nube && len(q.Pila) > 0 {
		q.AlIr(len(q.Pila) - 2)
		return true
	}
	p.irA(MiUnidad)
	return true
}
