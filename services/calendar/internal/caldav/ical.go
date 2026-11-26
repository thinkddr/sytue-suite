// SPDX-License-Identifier: Apache-2.0
package caldav

import (
	"time"

	"github.com/emersion/go-ical"

	calendarical "git.sytue.eu/sytue/one/apps/nucleo/calendar/ical"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
)

const (
	ProductID    = calendarical.ProductID
	AmendExclude = calendarical.AmendExclude
	AmendUntil   = calendarical.AmendUntil
)

type (
	ParsedObject = calendarical.ParsedObject
	AmendMode    = calendarical.AmendMode
)

func DecodeCalendarObject(data, calendarID string) (ParsedObject, error) {
	return calendarical.DecodeCalendarObject(data, calendarID)
}

func EncodeEvent(ev domain.Event, method domain.SchedulingMethod) (string, error) {
	return calendarical.EncodeEvent(ev, method)
}

func DecodeICSFeed(data string, maxObjects int) ([]ParsedObject, error) {
	return calendarical.DecodeICSFeed(data, maxObjects)
}

// AmendMasterRecurrence recorta la serie con la EXDATE nueva escrita como el DTSTART del
// maestro (en la hora de su zona, con su TZID), como las escribe EncodeEvent.
func AmendMasterRecurrence(data string, recurrenceID time.Time, mode AmendMode) (string, error) {
	return calendarical.RecortarSerie(data, recurrenceID, mode, calendarical.OpcionesRecorte{ComoElInicio: true})
}

// MoverConLaSerie: ver calendarical.MoverConLaSerie.
func MoverConLaSerie(fechas []time.Time, antes, ahora time.Time) []time.Time {
	return calendarical.MoverConLaSerie(fechas, antes, ahora)
}

// ConservarForma: ver calendarical.ConservarForma.
func ConservarForma(nuevo, anterior string) (string, error) {
	return calendarical.ConservarForma(nuevo, anterior)
}

func CarryOver(nuevo, anterior string, propios bool) (string, error) {
	return calendarical.CarryOver(nuevo, anterior, propios)
}
func Recordatorios(data string) []int { return calendarical.Recordatorios(data) }
func ConRecordatorios(data string, minutos []int, descripcion string) (string, error) {
	return calendarical.ConRecordatorios(data, minutos, descripcion)
}

func propTime(props ical.Props, name string) (time.Time, bool, error) {
	return calendarical.PropertyTime(props, name)
}

func propText(props ical.Props, name string) string {
	return calendarical.PropertyText(props, name)
}

func propTimes(props ical.Props, name string) []time.Time {
	return calendarical.PropertyTimes(props, name)
}

func propDuration(props ical.Props) (time.Duration, bool) {
	return calendarical.PropertyDuration(props)
}

func tzidOf(props ical.Props, name string) string {
	return calendarical.TimezoneIDOf(props, name)
}

func cleanCalAddress(v string) string {
	return calendarical.CleanCalAddress(v)
}
