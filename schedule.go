package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

type Employee struct {
	Name        string
	Assignments []string // "D", "S", "N", or "" per day
	Hours       int
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

func buildSchedule(names []string, hoursPerShift, onDays, offBlocks int) ([]Employee, int) {
	if len(names) == 0 {
		return nil, 0
	}

	n := len(names)
	blocksPerEmp := int(math.Ceil(80.0 / float64(hoursPerShift*onDays)))
	crewCount := 1 + offBlocks

	totalRounds := blocksPerEmp * crewCount
	totalDays := totalRounds * onDays

	employees := make([]Employee, n)
	for i, name := range names {
		employees[i] = Employee{
			Name:        name,
			Assignments: make([]string, totalDays),
		}
	}

	shifts := []string{"D", "S", "N"}

	for crew := 0; crew < crewCount; crew++ {
		for si := 0; si < 3; si++ {
			empIdx := crew*3 + si
			if empIdx >= n {
				break
			}
			shift := shifts[si]
			for block := 0; block < blocksPerEmp; block++ {
				round := crew + block*crewCount
				dayStart := round * onDays
				for d := 0; d < onDays; d++ {
					employees[empIdx].Assignments[dayStart+d] = shift
				}
				employees[empIdx].Hours += onDays * hoursPerShift
			}
		}
	}

	return employees, totalDays
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
