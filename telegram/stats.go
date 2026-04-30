package telegram

import (
	"bytes"
	"fmt"
	"image/color"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
)

type StatsRow struct {
	Date        time.Time
	Subscribers float64
	Note        string
}

func parseStatsMessage(text string) ([]StatsRow, error) {
	lines := strings.Split(text, "\n")
	var rows []StatsRow
	currentYear := ""

	yearRegex := regexp.MustCompile(`^(\d{4}):\s*$`)
	entryRegex := regexp.MustCompile(`^(\d{1,2})\s+(\w+):?\s+(\d+)(.*)$`)

	months := map[string]time.Month{
		"gennaio":   time.January,
		"febbraio":  time.February,
		"marzo":     time.March,
		"aprile":    time.April,
		"maggio":    time.May,
		"giugno":    time.June,
		"luglio":    time.July,
		"agosto":    time.August,
		"settembre": time.September,
		"ottobre":   time.October,
		"novembre":  time.November,
		"dicembre":  time.December,
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if yearMatch := yearRegex.FindStringSubmatch(line); yearMatch != nil {
			currentYear = yearMatch[1]
			continue
		}

		if currentYear == "" {
			continue
		}

		if entryMatch := entryRegex.FindStringSubmatch(line); entryMatch != nil {
			day, _ := strconv.Atoi(entryMatch[1])
			monthStr := strings.ToLower(entryMatch[2])
			subs, _ := strconv.ParseFloat(entryMatch[3], 64)
			note := strings.TrimSpace(entryMatch[4])

			month, ok := months[monthStr]
			if !ok {
				continue
			}

			year, _ := strconv.Atoi(currentYear)
			date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
			rows = append(rows, StatsRow{
				Date:        date,
				Subscribers: subs,
				Note:        note,
			})
		}
	}

	if len(rows) < 2 {
		return nil, fmt.Errorf("not enough data points")
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Date.Before(rows[j].Date)
	})

	return rows, nil
}

func createLinearPlot(rows []StatsRow) ([]byte, error) {
	p := plot.New()
	p.Title.Text = "Crescita Iscritti nel Tempo"
	p.X.Label.Text = "Data"
	p.Y.Label.Text = "Numero Iscritti"
	p.Add(plotter.NewGrid())

	pts := make(plotter.XYs, len(rows))
	for i, row := range rows {
		pts[i].X = float64(row.Date.Unix())
		pts[i].Y = row.Subscribers
	}

	line, points, err := plotter.NewLinePoints(pts)
	if err != nil {
		return nil, err
	}
	line.Color = color.RGBA{R: 0, G: 0, B: 255, A: 255}
	p.Add(line, points)

	p.X.Tick.Marker = plot.TimeTicks{Format: "2006-01-02"}

	// Annotations
	for _, row := range rows {
		if row.Note != "" {
			labels, err := plotter.NewLabels(plotter.XYLabels{
				XYs:    []plotter.XY{{X: float64(row.Date.Unix()), Y: row.Subscribers}},
				Labels: []string{row.Note},
			})
			if err == nil {
				labels.Offset = vg.Point{X: 0, Y: -20}
				labels.TextStyle[0].XAlign = draw.XCenter
				p.Add(labels)
			}
		}
	}

	wt, err := p.WriterTo(12*vg.Inch, 7*vg.Inch, "png")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	_, err = wt.WriteTo(&buf)
	return buf.Bytes(), err
}

func createPredictionPlot(rows []StatsRow, degree int) ([]byte, error) {
	if degree < 1 {
		degree = 4
	}

	minDate := rows[0].Date
	pts := make(plotter.XYs, len(rows))
	xValues := make([]float64, len(rows))
	yValues := make([]float64, len(rows))
	for i, row := range rows {
		days := float64(row.Date.Sub(minDate) / (24 * time.Hour))
		pts[i].X = days
		pts[i].Y = row.Subscribers
		xValues[i] = days
		yValues[i] = row.Subscribers
	}

	// Polynomial fitting
	coeffs := polyFit(xValues, yValues, degree)

	polyFunc := func(x float64) float64 {
		return polyEval(coeffs, x)
	}

	// Calculate R-squared
	yPred := make([]float64, len(yValues))
	for i, x := range xValues {
		yPred[i] = polyFunc(x)
	}
	rSquared := stat.RSquaredFrom(yValues, yPred, nil)

	p := plot.New()
	p.Title.Text = "Crescita Iscritti: Dati Reali e Previsione"
	p.X.Label.Text = "Giorni dall'inizio"
	p.Y.Label.Text = "Numero Iscritti"
	p.Add(plotter.NewGrid())

	// Real data
	realLine, realPoints, _ := plotter.NewLinePoints(pts)
	realLine.Color = color.RGBA{R: 0, G: 0, B: 255, A: 255}
	p.Add(realLine, realPoints)
	p.Legend.Add("Iscritti Reali", realLine, realPoints)

	// Fitted curve
	fittedPts := make(plotter.XYs, 100)
	maxX := xValues[len(xValues)-1]
	for i := 0; i < 100; i++ {
		x := float64(i) * maxX / 99
		fittedPts[i].X = x
		fittedPts[i].Y = polyFunc(x)
	}
	fittedLine, _ := plotter.NewLine(fittedPts)
	fittedLine.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	fittedLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
	p.Add(fittedLine)
	p.Legend.Add(fmt.Sprintf("Curva Fittata (R²=%.4f)", rSquared), fittedLine)

	// Prediction (next 365 days)
	futurePts := make(plotter.XYs, 100)
	for i := 0; i < 100; i++ {
		x := maxX + float64(i)*365/99
		futurePts[i].X = x
		futurePts[i].Y = polyFunc(x)
	}
	futureLine, _ := plotter.NewLine(futurePts)
	futureLine.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255}
	futureLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
	p.Add(futureLine)
	p.Legend.Add("Previsione", futureLine)

	p.Legend.Top = true

	wt, err := p.WriterTo(12*vg.Inch, 6*vg.Inch, "png")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	_, err = wt.WriteTo(&buf)
	return buf.Bytes(), err
}

func polyFit(x, y []float64, degree int) []float64 {
	a := mat.NewDense(len(x), degree+1, nil)
	for i := 0; i < len(x); i++ {
		for j := 0; j <= degree; j++ {
			a.Set(i, j, math.Pow(x[i], float64(j)))
		}
	}
	b := mat.NewDense(len(y), 1, y)
	var xMat mat.Dense
	err := xMat.Solve(a, b)
	if err != nil {
		log.Printf("Polyfit error: %v", err)
		return nil
	}
	coeffs := make([]float64, degree+1)
	for i := 0; i <= degree; i++ {
		coeffs[i] = xMat.At(i, 0)
	}
	return coeffs
}

func polyEval(coeffs []float64, x float64) float64 {
	res := 0.0
	for i, c := range coeffs {
		res += c * math.Pow(x, float64(i))
	}
	return res
}

func HandleStatsMessage(bot *EscarBot, msg *tgbotapi.Message) {
	bot.StateMutex.RLock()
	enabled := bot.StatsFeature
	targetChatID := bot.StatsChatID
	degree := bot.StatsPolynomialDegree
	bot.StateMutex.RUnlock()

	if !enabled || msg.Chat.ID != targetChatID {
		return
	}

	rows, err := parseStatsMessage(msg.Text)
	if err != nil {
		return
	}

	plot1, err := createLinearPlot(rows)
	if err != nil {
		log.Printf("Error creating linear plot: %v", err)
		return
	}

	plot2, err := createPredictionPlot(rows, degree)
	if err != nil {
		log.Printf("Error creating prediction plot: %v", err)
		return
	}

	photo1 := tgbotapi.NewInputMediaPhoto(tgbotapi.FileBytes{Name: "growth.png", Bytes: plot1})
	photo2 := tgbotapi.NewInputMediaPhoto(tgbotapi.FileBytes{Name: "prediction.png", Bytes: plot2})

	mediaGroup := tgbotapi.NewMediaGroup(msg.Chat.ID, []tgbotapi.InputMedia{&photo1, &photo2})
	mediaGroup.ReplyParameters = tgbotapi.ReplyParameters{MessageID: msg.MessageID}
	if msg.MessageThreadID != 0 {
		mediaGroup.MessageThreadID = msg.MessageThreadID
	}

	_, err = bot.Bot.Send(mediaGroup)
	if err != nil {
		log.Printf("Error sending stats plots: %v", err)
	}
}
