package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	payPeriodDays     = 14
	maxHoursPerPeriod = 80
	displayPeriods    = 2
)

type Employee struct {
	Name        string
	Shift       string   // assigned shift type: "D", "S", or "N"
	Assignments []string // "D", "S", "N", or "" per day
	Hours       int
}

type PayPeriodInfo struct {
	ShiftsPerPeriod  int
	HoursPerPeriod   int
	OffDaysPerPeriod int
	CrewCount        int
	PatternLength    int
	PatternString    string
	UncoveredDays    []int
}

type RotationPattern struct {
	Days []bool // true=on, false=off; length = cycle length
}

// ParsePattern parses an O/F (or 1/0) string into a RotationPattern.
// Whitespace and dashes are stripped.
func ParsePattern(s string) (RotationPattern, error) {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
	s = strings.ToUpper(s)
	if len(s) == 0 {
		return RotationPattern{}, fmt.Errorf("empty pattern")
	}
	days := make([]bool, len(s))
	for i, ch := range s {
		switch ch {
		case 'O', '1':
			days[i] = true
		case 'F', '0':
			days[i] = false
		default:
			return RotationPattern{}, fmt.Errorf("invalid character %q at position %d (use O/F or 1/0)", string(ch), i)
		}
	}
	return RotationPattern{Days: days}, nil
}

func SimplePattern(daysOn, crewCount int) RotationPattern {
	cycle := crewCount * daysOn
	days := make([]bool, cycle)
	for i := 0; i < daysOn; i++ {
		days[i] = true
	}
	return RotationPattern{Days: days}
}

func (p RotationPattern) String() string {
	var b strings.Builder
	for _, d := range p.Days {
		if d {
			b.WriteByte('O')
		} else {
			b.WriteByte('F')
		}
	}
	return b.String()
}

// MinCrewsForCoverage returns the minimum number of crews needed so that
// evenly-spaced offsets cover every day of the cycle.
func MinCrewsForCoverage(p RotationPattern) int {
	cycleLen := len(p.Days)
	if cycleLen == 0 {
		return 0
	}
	for crews := 1; crews <= cycleLen; crews++ {
		if len(ValidateCoverage(p, crews)) == 0 {
			return crews
		}
	}
	return cycleLen
}

// ValidateCoverage returns day indices (0-based within the cycle) that are
// not covered by any crew using evenly-spaced offsets.
func ValidateCoverage(p RotationPattern, crewCount int) []int {
	cycleLen := len(p.Days)
	if cycleLen == 0 || crewCount <= 0 {
		return nil
	}
	var uncovered []int
	for day := 0; day < cycleLen; day++ {
		covered := false
		for crew := 0; crew < crewCount; crew++ {
			offset := crew * cycleLen / crewCount
			pos := ((day - offset) % cycleLen + cycleLen) % cycleLen
			if p.Days[pos] {
				covered = true
				break
			}
		}
		if !covered {
			uncovered = append(uncovered, day)
		}
	}
	return uncovered
}

func parseNames(s string) []string {
	var names []string
	for _, part := range strings.Split(s, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func buildSchedule(names []string, hoursPerShift int, pattern RotationPattern, crewCount int) ([]Employee, int, PayPeriodInfo) {
	info := PayPeriodInfo{}
	cycleLen := len(pattern.Days)
	if len(names) == 0 || hoursPerShift <= 0 || cycleLen == 0 {
		return nil, 0, info
	}

	totalDays := payPeriodDays * displayPeriods

	type crewPattern struct {
		on             []bool
		shiftsInPeriod int
	}
	crews := make([]crewPattern, crewCount)
	for crew := 0; crew < crewCount; crew++ {
		offset := crew * cycleLen / crewCount
		on := make([]bool, totalDays)
		shiftsInPeriod := 0
		for day := 0; day < totalDays; day++ {
			pos := ((day - offset) % cycleLen + cycleLen) % cycleLen
			if pattern.Days[pos] {
				on[day] = true
				if day < payPeriodDays {
					shiftsInPeriod++
				}
			}
		}
		crews[crew] = crewPattern{on: on, shiftsInPeriod: shiftsInPeriod}
	}

	shiftsPerPeriod := crews[0].shiftsInPeriod
	hoursPerPeriod := shiftsPerPeriod * hoursPerShift
	uncovered := ValidateCoverage(pattern, crewCount)

	info = PayPeriodInfo{
		ShiftsPerPeriod:  shiftsPerPeriod,
		HoursPerPeriod:   hoursPerPeriod,
		OffDaysPerPeriod: payPeriodDays - shiftsPerPeriod,
		CrewCount:        crewCount,
		PatternLength:    cycleLen,
		PatternString:    pattern.String(),
		UncoveredDays:    uncovered,
	}

	n := len(names)
	employees := make([]Employee, n)
	for i, name := range names {
		employees[i] = Employee{
			Name:        name,
			Assignments: make([]string, totalDays),
		}
	}

	shifts := []string{"D", "S", "N"}

	for crew := 0; crew < crewCount; crew++ {
		cp := crews[crew]
		for si := 0; si < 3; si++ {
			empIdx := crew*3 + si
			if empIdx >= n {
				break
			}
			shift := shifts[si]
			hours := 0
			for day := 0; day < totalDays; day++ {
				if cp.on[day] {
					employees[empIdx].Assignments[day] = shift
					hours += hoursPerShift
				}
			}
			employees[empIdx].Hours = hours
		}
	}

	return employees, totalDays, info
}

func dateHeaders(start time.Time, periodDays int) []string {
	headers := make([]string, periodDays)
	for i := 0; i < periodDays; i++ {
		d := start.AddDate(0, 0, i)
		headers[i] = d.Format("Mon 01/02")
	}
	return headers
}

func writeCSV(w io.Writer, employees []Employee, headers []string) {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	row := append([]string{"Employee"}, headers...)
	row = append(row, "Total Hours")
	cw.Write(row)

	for _, emp := range employees {
		row := append([]string{emp.Name}, emp.Assignments...)
		row = append(row, fmt.Sprintf("%d", emp.Hours))
		cw.Write(row)
	}
}
