package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

type Employee struct {
	ID          string
	Shift       string
	OffPattern  []int
	Assignments []string // 14 entries: "D","S","N","OFF"
	Hours       int
}

type TeamConfig struct {
	Size  int
	Split map[string]int // e.g. {"D": 6, "S": 5, "N": 5}
}

type ShiftCoverage struct {
	D, S, N int
}

func (sc ShiftCoverage) Total() int { return sc.D + sc.S + sc.N }

func (sc ShiftCoverage) Get(shift string) int {
	switch shift {
	case "D":
		return sc.D
	case "S":
		return sc.S
	case "N":
		return sc.N
	}
	return 0
}

// daysOffPattern returns staggered consecutive-days-off patterns.
// Each inner slice is a pair of weekday indices (0=Mon … 6=Sun).
// Sizes 5 and 6 use hand-tuned patterns; other sizes are generated
// algorithmically using stride-3 slot assignment (coprime to 7).
func daysOffPattern(groupSize int) [][]int {
	if groupSize <= 0 {
		return nil
	}
	switch groupSize {
	case 5:
		return [][]int{
			{2, 3}, // Wed–Thu
			{5, 6}, // Sat–Sun
			{0, 1}, // Mon–Tue
			{3, 4}, // Thu–Fri
			{6, 0}, // Sun–Mon
		}
	case 6:
		return [][]int{
			{0, 1}, // Mon–Tue
			{2, 3}, // Wed–Thu
			{4, 5}, // Fri–Sat
			{6, 0}, // Sun–Mon
			{1, 2}, // Tue–Wed
			{3, 4}, // Thu–Fri
		}
	default:
		// 7 possible consecutive-day-off slots in a week (Mon-Tue … Sun-Mon).
		// Stride 3 is coprime to 7, producing sequence 0,3,6,2,5,1,4.
		pattern := make([][]int, groupSize)
		for i := 0; i < groupSize; i++ {
			slot := (i * 3) % 7
			pattern[i] = []int{slot, (slot + 1) % 7}
		}
		return pattern
	}
}

func buildTeam(name string, config TeamConfig, periodDays, hoursPerShift int) []Employee {
	prefix := "T1"
	if name == "Team 2" {
		prefix = "T2"
	}

	var employees []Employee
	empNum := 1

	// Iterate shifts in fixed order
	for _, shiftCode := range []string{"D", "S", "N"} {
		count := config.Split[shiftCode]
		pattern := daysOffPattern(count)

		for i := 0; i < count; i++ {
			offDays := make(map[int]bool)
			for _, d := range pattern[i] {
				offDays[d] = true
			}

			assignments := make([]string, periodDays)
			for dayIdx := 0; dayIdx < periodDays; dayIdx++ {
				weekday := dayIdx % 7 // 0=Mon since start is Monday
				if offDays[weekday] {
					assignments[dayIdx] = "OFF"
				} else {
					assignments[dayIdx] = shiftCode
				}
			}

			workDays := 0
			for _, a := range assignments {
				if a != "OFF" {
					workDays++
				}
			}

			employees = append(employees, Employee{
				ID:          fmt.Sprintf("%s-%02d", prefix, empNum),
				Shift:       shiftCode,
				OffPattern:  pattern[i],
				Assignments: assignments,
				Hours:       workDays * hoursPerShift,
			})
			empNum++
		}
	}

	return employees
}

func coverageGrid(employees []Employee, periodDays int) []ShiftCoverage {
	grid := make([]ShiftCoverage, periodDays)
	for dayIdx := 0; dayIdx < periodDays; dayIdx++ {
		for _, emp := range employees {
			switch emp.Assignments[dayIdx] {
			case "D":
				grid[dayIdx].D++
			case "S":
				grid[dayIdx].S++
			case "N":
				grid[dayIdx].N++
			}
		}
	}
	return grid
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

	row := append([]string{"Employee", "Shift"}, headers...)
	row = append(row, "Total Hours")
	cw.Write(row)

	for _, emp := range employees {
		row := append([]string{emp.ID, emp.Shift}, emp.Assignments...)
		row = append(row, fmt.Sprintf("%d", emp.Hours))
		cw.Write(row)
	}
}
