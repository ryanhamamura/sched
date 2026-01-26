package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ryanhamamura/via"
	"github.com/ryanhamamura/via/h"
)

const (
	defaultPeriodDays    = 14
	defaultHoursPerShift = 8
)

var defaultStart = time.Date(2026, 1, 26, 0, 0, 0, 0, time.UTC)

func main() {
	v := via.New()
	v.Config(via.Options{
		DocumentTitle: "Shift Schedule Generator",
		ServerAddress: ":7331",
	})

	v.AppendToHead(
		h.Link(h.Rel("stylesheet"), h.Href("https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.classless.min.css")),
		h.StyleEl(h.Raw(cssStyles)),
	)

	v.Page("/", indexPage)

	mux := v.HTTPServeMux()
	mux.HandleFunc("GET /download/{team}", handleCSVDownload)

	v.Start()
}

type schedData struct {
	team1, team2 []Employee
	cov1, cov2   []ShiftCoverage
	headers      []string
	start        time.Time
	hps          int
	t1Cfg, t2Cfg TeamConfig
}

func indexPage(c *via.Context) {
	startDate := c.Signal(defaultStart.Format("2006-01-02"))
	hoursPerShift := c.Signal(defaultHoursPerShift)
	t1D := c.Signal(6)
	t1S := c.Signal(5)
	t1N := c.Signal(5)
	t2D := c.Signal(5)
	t2S := c.Signal(5)
	t2N := c.Signal(5)

	// Generate default schedule on load
	data := generateScheduleData(defaultStart, defaultHoursPerShift,
		TeamConfig{Size: 16, Split: map[string]int{"D": 6, "S": 5, "N": 5}},
		TeamConfig{Size: 15, Split: map[string]int{"D": 5, "S": 5, "N": 5}},
	)

	generate := c.Action(func() {
		sd, err := time.Parse("2006-01-02", startDate.String())
		if err != nil {
			sd = defaultStart
		}
		hps := hoursPerShift.Int()
		if hps <= 0 {
			hps = defaultHoursPerShift
		}

		clamp := func(v, lo, hi int) int {
			if v < lo {
				return lo
			}
			if v > hi {
				return hi
			}
			return v
		}
		d1, s1, n1 := clamp(t1D.Int(), 0, 50), clamp(t1S.Int(), 0, 50), clamp(t1N.Int(), 0, 50)
		d2, s2, n2 := clamp(t2D.Int(), 0, 50), clamp(t2S.Int(), 0, 50), clamp(t2N.Int(), 0, 50)

		cfg1 := TeamConfig{
			Size:  d1 + s1 + n1,
			Split: map[string]int{"D": d1, "S": s1, "N": n1},
		}
		cfg2 := TeamConfig{
			Size:  d2 + s2 + n2,
			Split: map[string]int{"D": d2, "S": s2, "N": n2},
		}

		*data = *generateScheduleData(sd, hps, cfg1, cfg2)
		c.Sync()
	})

	c.View(func() h.H {
		sizeInput := func(sig interface{ Bind() h.H }) h.H {
			return h.Input(h.Type("number"), h.Attr("min", "0"), h.Attr("max", "50"), sig.Bind())
		}

		return h.Main(
			h.H1(h.Text("24/7 Shift Schedule")),

			// Config form
			h.Section(h.Class("config"),
				h.H2(h.Text("Parameters")),
				h.Div(h.Class("grid"),
					h.Div(
						h.Label(h.Attr("for", "start"), h.Text("Start Date")),
						h.Input(h.Type("date"), h.ID("start"), startDate.Bind()),
					),
					h.Div(
						h.Label(h.Attr("for", "hps"), h.Text("Hours per Shift")),
						h.Input(h.Type("number"), h.ID("hps"), h.Attr("min", "1"), h.Attr("max", "24"), hoursPerShift.Bind()),
					),
				),
				h.Div(h.Class("grid"),
					h.FieldSet(
						h.Legend(h.Text("Team 1 Shift Sizes")),
						h.Div(h.Class("grid"),
							h.Div(h.Label(h.Text("Day")), sizeInput(t1D)),
							h.Div(h.Label(h.Text("Swing")), sizeInput(t1S)),
							h.Div(h.Label(h.Text("Night")), sizeInput(t1N)),
						),
					),
					h.FieldSet(
						h.Legend(h.Text("Team 2 Shift Sizes")),
						h.Div(h.Class("grid"),
							h.Div(h.Label(h.Text("Day")), sizeInput(t2D)),
							h.Div(h.Label(h.Text("Swing")), sizeInput(t2S)),
							h.Div(h.Label(h.Text("Night")), sizeInput(t2N)),
						),
					),
				),
				h.Button(h.Text("Generate"), generate.OnClick()),
			),

			// Schedule output
			renderScheduleOutput(data),
		)
	})
}

func generateScheduleData(start time.Time, hps int, cfg1, cfg2 TeamConfig) *schedData {
	team1 := buildTeam("Team 1", cfg1, defaultPeriodDays, hps)
	team2 := buildTeam("Team 2", cfg2, defaultPeriodDays, hps)
	return &schedData{
		team1:   team1,
		team2:   team2,
		cov1:    coverageGrid(team1, defaultPeriodDays),
		cov2:    coverageGrid(team2, defaultPeriodDays),
		headers: dateHeaders(start, defaultPeriodDays),
		start:   start,
		hps:     hps,
		t1Cfg:   cfg1,
		t2Cfg:   cfg2,
	}
}

func renderScheduleOutput(d *schedData) h.H {
	endDate := d.start.AddDate(0, 0, defaultPeriodDays-1)
	periodStr := fmt.Sprintf("%s – %s", d.start.Format("January 02, 2006"), endDate.Format("January 02, 2006"))

	csvURL := func(team string, cfg TeamConfig) string {
		return fmt.Sprintf("/download/%s?start=%s&hps=%d&d=%d&s=%d&n=%d",
			team, d.start.Format("2006-01-02"), d.hps,
			cfg.Split["D"], cfg.Split["S"], cfg.Split["N"],
		)
	}

	return h.Div(
		h.P(h.Strong(h.Text("Pay period: ")), h.Text(periodStr)),
		renderTeamSection("Team 1", d.team1, d.cov1, d.headers, d.t1Cfg, csvURL("1", d.t1Cfg)),
		renderTeamSection("Team 2", d.team2, d.cov2, d.headers, d.t2Cfg, csvURL("2", d.t2Cfg)),
		h.H2(h.Text("Combined Coverage")),
		renderCombinedCoverage(d.cov1, d.cov2, d.headers),
	)
}

func renderTeamSection(name string, employees []Employee, cov []ShiftCoverage, headers []string, cfg TeamConfig, csvLink string) h.H {
	splitStr := fmt.Sprintf("%d Day / %d Swing / %d Night", cfg.Split["D"], cfg.Split["S"], cfg.Split["N"])

	if len(employees) == 0 {
		return h.Section(
			h.H2(h.Textf("%s — 0 employees (%s)", name, splitStr)),
			h.P(h.Em(h.Text("No employees assigned"))),
		)
	}

	return h.Section(
		h.H2(h.Textf("%s — %d employees (%s)", name, cfg.Size, splitStr)),
		h.H3(h.Text("Schedule")),
		renderScheduleTable(employees, headers),
		h.P(h.Em(h.Textf("All employees: %d hours/pay period", employees[0].Hours))),
		h.H3(h.Text("Daily Coverage")),
		renderCoverageTable(cov, headers),
		h.P(h.A(h.Href(csvLink), h.Attr("download", ""), h.Text("Download CSV"))),
	)
}

func renderScheduleTable(employees []Employee, headers []string) h.H {
	headerCells := []h.H{h.Th(h.Text("Employee")), h.Th(h.Text("Shift"))}
	for _, hdr := range headers {
		headerCells = append(headerCells, h.Th(h.Text(hdr)))
	}

	var rows []h.H
	currentShift := ""
	for _, emp := range employees {
		if emp.Shift != currentShift {
			currentShift = emp.Shift
			label := shiftLabel(currentShift)
			sepCells := []h.H{h.Class("shift-separator"), h.Td(h.Strong(h.Text(label))), h.Td()}
			for range headers {
				sepCells = append(sepCells, h.Td())
			}
			rows = append(rows, h.Tr(sepCells...))
		}

		cells := []h.H{h.Td(h.Text(emp.ID)), h.Td(h.Text(emp.Shift))}
		for _, a := range emp.Assignments {
			cls := "cell-" + cellClass(a)
			if a == "OFF" {
				cells = append(cells, h.Td(h.Class(cls), h.Text("OFF")))
			} else {
				cells = append(cells, h.Td(h.Class(cls), h.Strong(h.Text(a))))
			}
		}
		rows = append(rows, h.Tr(cells...))
	}

	return h.Div(h.Class("table-wrap"),
		h.Table(h.Class("schedule"),
			h.THead(h.Tr(headerCells...)),
			h.TBody(rows...),
		),
	)
}

func renderCoverageTable(cov []ShiftCoverage, headers []string) h.H {
	headerCells := []h.H{h.Th(h.Text("Shift"))}
	for _, hdr := range headers {
		headerCells = append(headerCells, h.Th(h.Text(hdr)))
	}

	var rows []h.H
	for _, sc := range []string{"D", "S", "N"} {
		cells := []h.H{h.Td(h.Text(shiftLabel(sc)))}
		for _, c := range cov {
			cells = append(cells, h.Td(h.Textf("%d", c.Get(sc))))
		}
		rows = append(rows, h.Tr(cells...))
	}

	totalCells := []h.H{h.Td(h.Strong(h.Text("Total")))}
	for _, c := range cov {
		totalCells = append(totalCells, h.Td(h.Strong(h.Textf("%d", c.Total()))))
	}
	rows = append(rows, h.Tr(totalCells...))

	return h.Div(h.Class("table-wrap"),
		h.Table(h.Class("coverage"),
			h.THead(h.Tr(headerCells...)),
			h.TBody(rows...),
		),
	)
}

func renderCombinedCoverage(cov1, cov2 []ShiftCoverage, headers []string) h.H {
	headerCells := []h.H{h.Th(h.Text("Shift"))}
	for _, hdr := range headers {
		headerCells = append(headerCells, h.Th(h.Text(hdr)))
	}

	var rows []h.H
	for _, sc := range []string{"D", "S", "N"} {
		cells := []h.H{h.Td(h.Text(shiftLabel(sc)))}
		for i := range cov1 {
			total := cov1[i].Get(sc) + cov2[i].Get(sc)
			cells = append(cells, h.Td(h.Textf("%d", total)))
		}
		rows = append(rows, h.Tr(cells...))
	}

	totalCells := []h.H{h.Td(h.Strong(h.Text("Total")))}
	for i := range cov1 {
		total := cov1[i].Total() + cov2[i].Total()
		totalCells = append(totalCells, h.Td(h.Strong(h.Textf("%d", total))))
	}
	rows = append(rows, h.Tr(totalCells...))

	return h.Div(h.Class("table-wrap"),
		h.Table(h.Class("coverage"),
			h.THead(h.Tr(headerCells...)),
			h.TBody(rows...),
		),
	)
}

func handleCSVDownload(w http.ResponseWriter, r *http.Request) {
	team := r.PathValue("team")
	q := r.URL.Query()

	sd, err := time.Parse("2006-01-02", q.Get("start"))
	if err != nil {
		sd = defaultStart
	}
	hps, _ := strconv.Atoi(q.Get("hps"))
	if hps <= 0 {
		hps = defaultHoursPerShift
	}
	d, err := strconv.Atoi(q.Get("d"))
	if err != nil {
		d = 5
	}
	s, err := strconv.Atoi(q.Get("s"))
	if err != nil {
		s = 5
	}
	n, err := strconv.Atoi(q.Get("n"))
	if err != nil {
		n = 5
	}

	teamName := "Team 1"
	if team == "2" {
		teamName = "Team 2"
	}

	cfg := TeamConfig{
		Size:  d + s + n,
		Split: map[string]int{"D": d, "S": s, "N": n},
	}
	employees := buildTeam(teamName, cfg, defaultPeriodDays, hps)
	headers := dateHeaders(sd, defaultPeriodDays)

	filename := fmt.Sprintf("team_%s.csv", team)
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	writeCSV(w, employees, headers)
}

func shiftLabel(code string) string {
	switch code {
	case "D":
		return "Day"
	case "S":
		return "Swing"
	case "N":
		return "Night"
	}
	return code
}

func cellClass(assignment string) string {
	switch assignment {
	case "D":
		return "day"
	case "S":
		return "swing"
	case "N":
		return "night"
	default:
		return "off"
	}
}

const cssStyles = `
	.table-wrap {
		overflow-x: auto;
	}
	table.schedule, table.coverage {
		font-size: 0.8rem;
		border-collapse: collapse;
		white-space: nowrap;
	}
	table.schedule th, table.schedule td,
	table.coverage th, table.coverage td {
		padding: 0.25rem 0.4rem;
		text-align: center;
	}
	.cell-day    { background: #dbeafe; color: #1e40af; }
	.cell-swing  { background: #ffedd5; color: #c2410c; }
	.cell-night  { background: #ede9fe; color: #6d28d9; }
	.cell-off    { background: #f3f4f6; color: #6b7280; }
	.shift-separator td { border-top: 2px solid #9ca3af; }
	.config { margin-bottom: 2rem; }
`
