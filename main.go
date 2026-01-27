package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ryanhamamura/via"
	"github.com/ryanhamamura/via/h"
)

const defaultHoursPerShift = 8

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
	names     []string
	totalDays int
	info      PayPeriodInfo
}

func indexPage(c *via.Context) {
	startDate := c.Signal(defaultStart.Format("2006-01-02"))
	hoursPerShift := c.Signal(defaultHoursPerShift)
	namesInput := c.Signal(defaultNames)

	data := newSchedData(defaultStart, defaultHoursPerShift, defaultNames)

	generate := c.Action(func() {
		sd, err := time.Parse("2006-01-02", startDate.String())
		if err != nil {
			sd = defaultStart
		}
		hps := hoursPerShift.Int()
		if hps <= 0 {
			hps = defaultHoursPerShift
		}

		*data = *newSchedData(sd, hps, namesInput.String())
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

func newSchedData(start time.Time, hps int, namesStr string) *schedData {
	names := parseNames(namesStr)
	employees, totalDays, info := buildSchedule(names, hps)
	return &schedData{
		employees: employees,
		headers:   dateHeaders(start, totalDays),
		start:     start,
		hps:       hps,
		names:     names,
		totalDays: totalDays,
		info:      info,
	}
}

func renderScheduleOutput(d *schedData) h.H {
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

	summaryLine := fmt.Sprintf("%d days on, %d days off per pay period | %d hours",
		info.ShiftsPerPeriod, info.OffDaysPerPeriod, info.HoursPerPeriod)
	if info.HoursPerPeriod < maxHoursPerPeriod {
		summaryLine += fmt.Sprintf(" (%dh max)", maxHoursPerPeriod)
	}

	csvURL := fmt.Sprintf("/download?start=%s&hps=%d&names=%s",
		d.start.Format("2006-01-02"), d.hps,
		url.QueryEscape(strings.Join(d.names, ",")),
	)

	return h.Div(
		h.P(h.Strong(h.Text("Schedule: ")), h.Text(periodStr)),
		h.P(h.Em(h.Text(summaryLine))),
		h.P(h.Em(h.Text(fmt.Sprintf("Crews: %d (%d employees minimum)", info.CrewCount, minEmployees)))),
		renderScheduleTable(d.employees, d.headers),
		h.P(h.A(h.Href(csvURL), h.Attr("download", ""), h.Text("Download CSV"))),
	)
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

	names := parseNames(q.Get("names"))
	employees, totalDays, _ := buildSchedule(names, hps)
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
