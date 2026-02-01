package main

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ryanhamamura/via"
	"github.com/ryanhamamura/via/h"
)

const (
	defaultHoursPerShift = 8
	defaultDaysOn        = 3

	// Named rotation patterns (28-day cycles, 50% duty, 4 crews)
	patternPanama = "OOFFOOO" + "FFOOFFF" + "OOFFOOO" + "FFOOFFF"
	patternDuPont = "OOOO" + "FFF" + "OOO" + "F" + "OOO" + "FFF" + "OOOO" + "FFFFFFF"
)

var (
	defaultStart = time.Date(2026, 1, 26, 0, 0, 0, 0, time.UTC)
	defaultNames = "Alice, Bob, Charlie, Diana, Ethan, Fiona, George"
)

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
	mux.HandleFunc("GET /download", handleCSVDownload)

	v.Start()
}

type schedData struct {
	employees []Employee
	headers   []string
	start     time.Time
	hps       int
	pattern   string
	crews     int
	names     []string
	totalDays int
	info      PayPeriodInfo
	errMsg    string
}

func indexPage(c *via.Context) {
	startDate := c.Signal(defaultStart.Format("2006-01-02"))
	hoursPerShift := c.Signal(defaultHoursPerShift)
	namesInput := c.Signal(defaultNames)
	daysOnInput := c.Signal(defaultDaysOn)
	presetInput := c.Signal("simple")
	patternInput := c.Signal(SimplePattern(defaultDaysOn, 3).String())
	crewOverride := c.Signal(0)

	defaultPattern := SimplePattern(defaultDaysOn, 3)
	defaultCrews := int(math.Ceil(float64(len(parseNames(defaultNames))) / 3.0))
	data := newSchedData(defaultStart, defaultHoursPerShift, defaultPattern, defaultCrews, defaultNames)

	// Fill pattern from preset when preset changes
	applyPreset := c.Action(func() {
		switch presetInput.String() {
		case "panama":
			patternInput.SetValue(patternPanama)
		case "dupont":
			patternInput.SetValue(patternDuPont)
		case "simple":
			don := daysOnInput.Int()
			if don < 1 {
				don = 1
			}
			names := parseNames(namesInput.String())
			crews := int(math.Ceil(float64(len(names)) / 3.0))
			if crews < 1 {
				crews = 1
			}
			patternInput.SetValue(SimplePattern(don, crews).String())
		}
		c.Sync()
	})

	generate := c.Action(func() {
		sd, err := time.Parse("2006-01-02", startDate.String())
		if err != nil {
			sd = defaultStart
		}
		hps := hoursPerShift.Int()
		if hps <= 0 {
			hps = defaultHoursPerShift
		}

		pat, err := ParsePattern(patternInput.String())
		if err != nil {
			data.errMsg = fmt.Sprintf("Invalid pattern: %v", err)
			data.employees = nil
			c.Sync()
			return
		}

		crews := crewOverride.Int()
		if crews <= 0 {
			crews = MinCrewsForCoverage(pat)
		}

		*data = *newSchedData(sd, hps, pat, crews, namesInput.String())
		c.Sync()
	})

	c.View(func() h.H {
		return h.Main(
			h.H1(h.Text("24/7 Shift Schedule")),

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
					h.Div(
						h.Label(h.Attr("for", "preset"), h.Text("Pattern Preset")),
						h.Select(h.ID("preset"), presetInput.Bind(), applyPreset.OnChange(),
							h.Option(h.Value("simple"), h.Text("Simple (N-on / off)")),
							h.Option(h.Value("panama"), h.Text("Panama (28-day)")),
							h.Option(h.Value("dupont"), h.Text("DuPont (28-day)")),
							h.Option(h.Value("custom"), h.Text("Custom")),
						),
					),
				),
				h.Div(h.Class("grid"),
					h.Div(
						h.Label(h.Attr("for", "don"), h.Text("Days On (simple mode)")),
						h.Input(h.Type("number"), h.ID("don"), h.Attr("min", "1"), h.Attr("max", "14"), daysOnInput.Bind()),
					),
					h.Div(
						h.Label(h.Attr("for", "crews"), h.Text("Crew Override (0 = auto)")),
						h.Input(h.Type("number"), h.ID("crews"), h.Attr("min", "0"), h.Attr("max", "50"), crewOverride.Bind()),
					),
				),
				h.Div(
					h.Label(h.Attr("for", "pattern"), h.Text("Rotation Pattern (O=on, F=off)")),
					h.Input(h.Type("text"), h.ID("pattern"), patternInput.Bind(),
						h.Attr("placeholder", "e.g. OOFFOOO...")),
				),
				h.Div(
					h.Label(h.Attr("for", "names"), h.Text("Employees (comma-separated)")),
					h.Textarea(h.ID("names"), h.Attr("rows", "3"), namesInput.Bind()),
				),
				h.Button(h.Text("Generate"), generate.OnClick()),
			),

			renderScheduleOutput(data),
		)
	})
}

func newSchedData(start time.Time, hps int, pat RotationPattern, crews int, namesStr string) *schedData {
	names := parseNames(namesStr)
	employees, totalDays, info := buildSchedule(names, hps, pat, crews)
	return &schedData{
		employees: employees,
		headers:   dateHeaders(start, totalDays),
		start:     start,
		hps:       hps,
		pattern:   pat.String(),
		crews:     crews,
		names:     names,
		totalDays: totalDays,
		info:      info,
	}
}

func renderScheduleOutput(d *schedData) h.H {
	if d.errMsg != "" {
		return h.P(h.Strong(h.Text(d.errMsg)))
	}
	if len(d.employees) == 0 {
		return h.P(h.Em(h.Text("No employees to schedule")))
	}

	endDate := d.start.AddDate(0, 0, d.totalDays-1)
	periodStr := fmt.Sprintf("%s – %s (%d days)",
		d.start.Format("January 02, 2006"),
		endDate.Format("January 02, 2006"),
		d.totalDays,
	)

	info := d.info
	minEmployees := info.CrewCount * 3

	summaryLine := fmt.Sprintf("Pattern: %s (%d-day cycle, %d crews) | %d shifts, %d hours per pay period",
		info.PatternString, info.PatternLength, info.CrewCount, info.ShiftsPerPeriod, info.HoursPerPeriod)
	if info.HoursPerPeriod > maxHoursPerPeriod {
		summaryLine += " (exceeds 80h cap)"
	}

	elements := []h.H{
		h.P(h.Strong(h.Text("Schedule: ")), h.Text(periodStr)),
		h.P(h.Em(h.Text(summaryLine))),
		h.P(h.Em(h.Text(fmt.Sprintf("Crews: %d (%d employees minimum)", info.CrewCount, minEmployees)))),
	}

	if len(info.UncoveredDays) > 0 {
		dayStrs := make([]string, len(info.UncoveredDays))
		for i, d := range info.UncoveredDays {
			dayStrs[i] = fmt.Sprintf("%d", d+1)
		}
		elements = append(elements, h.P(h.Strong(
			h.Text(fmt.Sprintf("Warning: %d uncovered day(s) in cycle: %s",
				len(info.UncoveredDays), strings.Join(dayStrs, ", "))))))
	}

	csvURL := fmt.Sprintf("/download?start=%s&hps=%d&pattern=%s&crews=%d&names=%s",
		d.start.Format("2006-01-02"), d.hps,
		url.QueryEscape(d.pattern), d.crews,
		url.QueryEscape(strings.Join(d.names, ",")),
	)

	elements = append(elements,
		renderScheduleTable(d.employees, d.headers),
		h.P(h.A(h.Href(csvURL), h.Attr("download", ""), h.Text("Download CSV"))),
	)

	return h.Div(elements...)
}

func renderScheduleTable(employees []Employee, headers []string) h.H {
	headerCells := []h.H{h.Th(h.Text("Employee"))}
	for i, hdr := range headers {
		if i == payPeriodDays {
			headerCells = append(headerCells, h.Th(h.Class("period-boundary"), h.Text(hdr)))
		} else {
			headerCells = append(headerCells, h.Th(h.Text(hdr)))
		}
	}
	headerCells = append(headerCells, h.Th(h.Text("Hours")))

	var rows []h.H
	for _, emp := range employees {
		cells := []h.H{h.Td(h.Text(emp.Name))}
		for i, a := range emp.Assignments {
			boundary := i == payPeriodDays
			if a == "" {
				if boundary {
					cells = append(cells, h.Td(h.Class("period-boundary")))
				} else {
					cells = append(cells, h.Td())
				}
			} else {
				cls := "cell-" + cellClass(a)
				if boundary {
					cls += " period-boundary"
				}
				cells = append(cells, h.Td(h.Class(cls), h.Strong(h.Text(a))))
			}
		}
		cells = append(cells, h.Td(h.Textf("%d", emp.Hours)))
		rows = append(rows, h.Tr(cells...))
	}

	return h.Div(h.Class("table-wrap"),
		h.Table(h.Class("schedule"),
			h.THead(h.Tr(headerCells...)),
			h.TBody(rows...),
		),
	)
}

func handleCSVDownload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	sd, err := time.Parse("2006-01-02", q.Get("start"))
	if err != nil {
		sd = defaultStart
	}
	hps, _ := strconv.Atoi(q.Get("hps"))
	if hps <= 0 {
		hps = defaultHoursPerShift
	}

	patStr := q.Get("pattern")
	pat, err := ParsePattern(patStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid pattern: %v", err), http.StatusBadRequest)
		return
	}

	crews, _ := strconv.Atoi(q.Get("crews"))
	if crews <= 0 {
		crews = MinCrewsForCoverage(pat)
	}

	names := parseNames(q.Get("names"))
	employees, totalDays, _ := buildSchedule(names, hps, pat, crews)
	headers := dateHeaders(sd, totalDays)

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="schedule.csv"`)
	writeCSV(w, employees, headers)
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
	table.schedule {
		font-size: 0.8rem;
		border-collapse: collapse;
		white-space: nowrap;
	}
	table.schedule th, table.schedule td {
		padding: 0.25rem 0.4rem;
		text-align: center;
	}
	.cell-day    { background: #dbeafe; color: #1e40af; }
	.cell-swing  { background: #ffedd5; color: #c2410c; }
	.cell-night  { background: #ede9fe; color: #6d28d9; }
	.cell-off    { background: #f3f4f6; color: #6b7280; }
	.config { margin-bottom: 2rem; }
	.period-boundary { border-left: 3px solid #374151; }
`
