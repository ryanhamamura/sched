package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
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
	Assignments []string // "D", "S", "N", or "" per day
	Hours       int
}

type PayPeriodInfo struct {
	ShiftsPerPeriod  int
	HoursPerPeriod   int
	OffDaysPerPeriod int
	CrewCount        int
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

func buildSchedule(names []string, hoursPerShift int) ([]Employee, int, PayPeriodInfo) {
	info := PayPeriodInfo{}
	if len(names) == 0 || hoursPerShift <= 0 {
		return nil, 0, info
	}

	shiftsPerPeriod := maxHoursPerPeriod / hoursPerShift
	if shiftsPerPeriod > payPeriodDays {
		shiftsPerPeriod = payPeriodDays
	}
	if shiftsPerPeriod == 0 {
		return nil, 0, info
	}

	crewCount := int(math.Ceil(float64(payPeriodDays) / float64(shiftsPerPeriod)))
	stride := int(math.Ceil(float64(payPeriodDays) / float64(crewCount)))

	info = PayPeriodInfo{
		ShiftsPerPeriod:  shiftsPerPeriod,
		HoursPerPeriod:   shiftsPerPeriod * hoursPerShift,
		OffDaysPerPeriod: payPeriodDays - shiftsPerPeriod,
		CrewCount:        crewCount,
	}

	totalDays := payPeriodDays * displayPeriods

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
		start := crew * stride

		// Build which days in the 14-day period this crew works
		onPattern := make([]bool, payPeriodDays)
		for d := 0; d < shiftsPerPeriod; d++ {
			onPattern[(start+d)%payPeriodDays] = true
		}

		for si := 0; si < 3; si++ {
			empIdx := crew*3 + si
			if empIdx >= n {
				break
			}
			shift := shifts[si]
			for day := 0; day < totalDays; day++ {
				if onPattern[day%payPeriodDays] {
					employees[empIdx].Assignments[day] = shift
				}
			}
			employees[empIdx].Hours = shiftsPerPeriod * hoursPerShift * displayPeriods
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
