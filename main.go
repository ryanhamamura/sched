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

const (
	defaultHoursPerShift = 8
	defaultOnDays        = 3
	defaultOffBlocks     = 2
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
	onDays    int
	offBlocks int
	names     []string
	totalDays int
}

func indexPage(c *via.Context) {
	startDate := c.Signal(defaultStart.Format("2006-01-02"))
	hoursPerShift := c.Signal(defaultHoursPerShift)
	onDays := c.Signal(defaultOnDays)
	offBlocks := c.Signal(defaultOffBlocks)
	namesInput := c.Signal(defaultNames)

	data := newSchedData(defaultStart, defaultHoursPerShift, defaultOnDays, defaultOffBlocks, defaultNames)

	generate := c.Action(func() {
		sd, err := time.Parse("2006-01-02", startDate.String())
		if err != nil {
			sd = defaultStart
		}
		hps := hoursPerShift.Int()
		if hps <= 0 {
			hps = defaultHoursPerShift
		}
		od := onDays.Int()
		if od <= 0 {
			od = defaultOnDays
		}
		ob := offBlocks.Int()
		if ob < 0 {
			ob = defaultOffBlocks
		}

		*data = *newSchedData(sd, hps, od, ob, namesInput.String())
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
						h.Label(h.Attr("for", "ondays"), h.Text("Days On")),
						h.Input(h.Type("number"), h.ID("ondays"), h.Attr("min", "1"), h.Attr("max", "50"), onDays.Bind()),
					),
					h.Div(
						h.Label(h.Attr("for", "offblocks"), h.Text("Blocks Off")),
						h.Input(h.Type("number"), h.ID("offblocks"), h.Attr("min", "0"), h.Attr("max", "50"), offBlocks.Bind()),
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

func newSchedData(start time.Time, hps, onDays, offBlocks int, namesStr string) *schedData {
	names := parseNames(namesStr)
	employees, totalDays := buildSchedule(names, hps, onDays, offBlocks)
	return &schedData{
		employees: employees,
		headers:   dateHeaders(start, totalDays),
		start:     start,
		hps:       hps,
		onDays:    onDays,
		offBlocks: offBlocks,
		names:     names,
		totalDays: totalDays,
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

	offDays := d.offBlocks * d.onDays

	var patternNote h.H
	if offDays == 0 {
		patternNote = h.P(h.Em(h.Text(
			fmt.Sprintf("Pattern: %d days on, 0 days off (same employees cover every block)", d.onDays),
		)))
	} else {
		patternNote = h.P(h.Em(h.Text(
			fmt.Sprintf("Pattern: %d days on, %d days off", d.onDays, offDays),
		)))
	}

	csvURL := fmt.Sprintf("/download?start=%s&hps=%d&ondays=%d&offblocks=%d&names=%s",
		d.start.Format("2006-01-02"), d.hps, d.onDays, d.offBlocks,
		url.QueryEscape(strings.Join(d.names, ",")),
	)

	return h.Div(
		h.P(h.Strong(h.Text("Schedule: ")), h.Text(periodStr)),
		patternNote,
		renderScheduleTable(d.employees, d.headers),
		h.P(h.A(h.Href(csvURL), h.Attr("download", ""), h.Text("Download CSV"))),
	)
}

func renderScheduleTable(employees []Employee, headers []string) h.H {
	headerCells := []h.H{h.Th(h.Text("Employee"))}
	for _, hdr := range headers {
		headerCells = append(headerCells, h.Th(h.Text(hdr)))
	}
	headerCells = append(headerCells, h.Th(h.Text("Hours")))

	var rows []h.H
	for _, emp := range employees {
		cells := []h.H{h.Td(h.Text(emp.Name))}
		for _, a := range emp.Assignments {
			if a == "" {
				cells = append(cells, h.Td())
			} else {
				cls := "cell-" + cellClass(a)
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
	od, _ := strconv.Atoi(q.Get("ondays"))
	if od <= 0 {
		od = defaultOnDays
	}
	ob, _ := strconv.Atoi(q.Get("offblocks"))
	if ob < 0 {
		ob = defaultOffBlocks
	}

	names := parseNames(q.Get("names"))
	employees, totalDays := buildSchedule(names, hps, od, ob)
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
`
