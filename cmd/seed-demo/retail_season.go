package main

import (
	"math"
	"time"
)

// The retail scenario's shape over the year. A flat daily sales rate made
// every Dashboard period look the same: the monthly revenue bars were all
// alike and the top products of the last 3 months were those of the last 12.
// A Tunisian appliance shop's year has a clear shape instead, which this file
// models:
//
//   - traffic rises into the summer (weddings, the heat) and dips after the
//     back-to-school month and in the winter (seasonFactor);
//   - the business grows, so a later year sells more than an earlier one
//     (growthFactor);
//   - what sells follows the season (kindWeight): air conditioners and fans
//     in the summer, water heaters in the winter, kitchen appliances during
//     Ramadan, freezers before Eid al-Adha, the big wedding-list appliances
//     from June to September, televisions around a big football tournament.
//
// Dates of the religious holidays are the actual ones (they move ~11 days
// earlier every year), so the peaks land where a Tunisian user expects them.

// monthTraffic is the counter's relative traffic by calendar month.
var monthTraffic = [13]float64{
	0,
	0.80, // January
	0.80, // February
	0.95, // March
	1.00, // April
	1.10, // May
	1.30, // June — weddings start, first heat
	1.45, // July
	1.30, // August
	0.85, // September — back to school, money goes elsewhere
	0.90, // October
	0.95, // November
	1.10, // December — year-end promotions
}

type dateRange struct{ from, to time.Time }

func ymd(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func (r dateRange) contains(t time.Time) bool { return !t.Before(r.from) && !t.After(r.to) }

// ramadan holds the month of Ramadan for the years a run can cover.
var ramadan = []dateRange{
	{ymd(2024, 3, 11), ymd(2024, 4, 9)},
	{ymd(2025, 3, 1), ymd(2025, 3, 30)},
	{ymd(2026, 2, 18), ymd(2026, 3, 19)},
	{ymd(2027, 2, 8), ymd(2027, 3, 9)},
}

// eidAlAdha is the feast day; families buy a freezer in the two weeks before
// it for the meat of the sacrifice.
var eidAlAdha = []time.Time{ymd(2024, 6, 16), ymd(2025, 6, 6), ymd(2026, 5, 27), ymd(2027, 5, 16)}

// footballTournaments sell televisions: the 2024 European Championship, the
// 2025-26 Africa Cup of Nations and the 2026 World Cup.
var footballTournaments = []dateRange{
	{ymd(2024, 6, 1), ymd(2024, 7, 14)},
	{ymd(2025, 12, 7), ymd(2026, 1, 18)},
	{ymd(2026, 5, 25), ymd(2026, 7, 19)},
}

// quietMonth is one month that sold much less than its season says — road
// works in front of the shop — so a year's bars aren't a smooth curve.
var quietMonth = dateRange{ymd(2026, 1, 1), ymd(2026, 1, 31)}

func inAny(t time.Time, ranges []dateRange) bool {
	for _, r := range ranges {
		if r.contains(t) {
			return true
		}
	}
	return false
}

// beforeEidAlAdha reports whether t is in the two weeks before the feast.
func beforeEidAlAdha(t time.Time) bool {
	for _, eid := range eidAlAdha {
		if !t.After(eid) && eid.Sub(t) <= 14*24*time.Hour {
			return true
		}
	}
	return false
}

// seasonFactor is the counter's traffic on t relative to an average day,
// before growth.
func seasonFactor(t time.Time) float64 {
	f := monthTraffic[t.Month()]
	if inAny(t, ramadan) {
		f *= 1.10
	}
	if quietMonth.contains(t) {
		f *= 0.65
	}
	return f
}

// growthFactor rises linearly over the run, from 0.75 on its first day to
// 1.20 on its last: a business ~20% bigger each year.
func (s *Seeder) growthFactor(t time.Time) float64 {
	span := s.cfg.EndDate.Sub(s.start).Hours()
	if span <= 0 {
		return 1
	}
	elapsed := t.Sub(s.start).Hours() / span
	return 0.75 + 0.45*math.Max(0, math.Min(1, elapsed))
}

var (
	weddingKinds = map[string]bool{
		"Réfrigérateur": true, "Machine à laver": true, "Téléviseur": true,
		"Cuisinière gaz/électrique": true, "Congélateur": true, "Lave-vaisselle": true,
	}
	ramadanKinds = map[string]bool{
		"Four à micro-ondes": true, "Cuisinière gaz/électrique": true, "Robot ménager": true,
		"Mixeur": true, "Batteur": true, "Extracteur de jus": true, "Cafetière": true,
	}
)

// kindWeight is how much more (or less) likely an appliance of kind is to be
// sold on t than on an ordinary day. Services and unknown kinds weigh 1.
func kindWeight(kind string, t time.Time) float64 {
	m := t.Month()
	w := 1.0
	switch kind {
	case "Climatiseur":
		switch {
		case m == time.June || m == time.July:
			w = 5
		case m == time.May || m == time.August:
			w = 3.5
		case m == time.April || m == time.September:
			w = 1.3
		default:
			w = 0.2
		}
	case "Ventilateur":
		switch {
		case m >= time.June && m <= time.August:
			w = 4
		case m == time.May || m == time.September:
			w = 2
		default:
			w = 0.3
		}
	case "Chauffe-eau":
		switch {
		case m >= time.November || m <= time.February:
			w = 3
		case m == time.October || m == time.March:
			w = 1.5
		case m >= time.May && m <= time.September:
			w = 0.4
		}
	case "Congélateur":
		if beforeEidAlAdha(t) {
			w = 4
		}
	case "Téléviseur":
		if inAny(t, footballTournaments) {
			w = 2.5
		}
	}
	if weddingKinds[kind] && m >= time.June && m <= time.September {
		w *= 1.6
	}
	if ramadanKinds[kind] && inAny(t, ramadan) {
		w *= 2.5
	}
	return w
}

// poissonish turns an expected count into a whole number: its integer part,
// plus one more with probability equal to the fraction — so a low-volume run
// still averages the expected rate instead of rounding every day to the same
// value.
func (s *Seeder) poissonish(expected float64) int {
	if expected <= 0 {
		return 0
	}
	n := int(expected)
	if s.rng.Chance(expected - float64(n)) {
		n++
	}
	return n
}
