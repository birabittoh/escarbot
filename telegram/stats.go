package telegram

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"github.com/rivo/uniseg"
	"golang.org/x/image/font/opentype"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/font"
	"gonum.org/v1/plot/font/liberation"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/text"
	"gonum.org/v1/plot/vg"
	vgdraw "gonum.org/v1/plot/vg/draw"
)

//go:embed DejaVuSans.ttf
var dejaVuSansData []byte

var (
	emojiCache     = make(map[string]image.Image)
	emojiCacheMu   sync.RWMutex
	emojiTransport = &http.Client{Timeout: 5 * time.Second}
)

type FallbackHandler struct {
	fonts *font.Cache
}

type dateTicker struct{}

func (dateTicker) Ticks(min, max float64) []plot.Tick {
	if max <= min {
		return nil
	}

	const labelWidth = 80.0 // Approximate width of "2006-01-02" in points
	plotWidth := 12.0 * 72.0 // Plot width is 12 inches
	maxTicks := int(plotWidth / (labelWidth * 1.5))
	if maxTicks < 2 {
		maxTicks = 2
	}

	duration := time.Duration(int64(max-min)) * time.Second
	step := duration / time.Duration(maxTicks)

	// Round step to something sensible
	switch {
	case step > 365*24*time.Hour:
		step = 365 * 24 * time.Hour
	case step > 90*24*time.Hour:
		step = 90 * 24 * time.Hour
	case step > 30*24*time.Hour:
		step = 30 * 24 * time.Hour
	case step > 14*24*time.Hour:
		step = 14 * 24 * time.Hour
	case step > 7*24*time.Hour:
		step = 7 * 24 * time.Hour
	case step > 24*time.Hour:
		step = 24 * time.Hour
	default:
		step = 24 * time.Hour
	}

	var ticks []plot.Tick
	start := time.Unix(int64(min), 0).Truncate(step)
	if start.Unix() < int64(min) {
		start = start.Add(step)
	}

	for t := start; t.Unix() <= int64(max); t = t.Add(step) {
		ticks = append(ticks, plot.Tick{
			Value: float64(t.Unix()),
			Label: t.Format("2006-01-02"),
		})
	}

	// Add minor ticks (unsatisfied labels)
	minorStep := step / 4
	if minorStep >= 24*time.Hour {
		for t := start.Add(-step); t.Unix() <= int64(max)+int64(step.Seconds()); t = t.Add(minorStep) {
			val := float64(t.Unix())
			if val < min || val > max {
				continue
			}
			isMajor := false
			for _, maj := range ticks {
				if math.Abs(maj.Value-val) < 1.0 {
					isMajor = true
					break
				}
			}
			if !isMajor {
				ticks = append(ticks, plot.Tick{Value: val})
			}
		}
	}

	return ticks
}

func (h FallbackHandler) Cache() *font.Cache { return h.fonts }
func (h FallbackHandler) Lines(s string) []string { return strings.Split(s, "\n") }
func (h FallbackHandler) Extents(fnt font.Font) font.Extents {
	face := h.fonts.Lookup(fnt, fnt.Size)
	return face.Extents()
}

func (h FallbackHandler) findFace(r rune, preferred font.Font) font.Face {
	// Try preferred font first
	face := h.fonts.Lookup(preferred, preferred.Size)
	if hasGlyph(face.Face, r) {
		return face
	}

	// If preferred was Liberation, try DejaVu Sans first
	if strings.HasPrefix(string(preferred.Typeface), "Liberation") {
		fnt := preferred
		fnt.Typeface = "DejaVu Sans"
		fnt.Variant = ""
		f := h.fonts.Lookup(fnt, fnt.Size)
		if hasGlyph(f.Face, r) {
			return f
		}
	}

	// Try DejaVu Sans as a general fallback
	if preferred.Typeface != "DejaVu Sans" {
		fnt := preferred
		fnt.Typeface = "DejaVu Sans"
		fnt.Variant = ""
		f := h.fonts.Lookup(fnt, fnt.Size)
		if hasGlyph(f.Face, r) {
			return f
		}
	}

	return face
}

func isEmoji(s string) bool {
	if s == "" {
		return false
	}
	r := []rune(s)[0]
	// Basic check for emoji range
	return (r >= 0x1F000 && r <= 0x1FFFF) ||
		(r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0x2300 && r <= 0x23FF) ||
		(r >= 0x2B00 && r <= 0x2BFF)
}

func hasGlyph(f *opentype.Font, r rune) bool {
	if f == nil {
		return false
	}
	idx, _ := f.GlyphIndex(nil, r)
	return idx != 0
}

func getEmojiImage(emoji string) image.Image {
	emojiCacheMu.RLock()
	img, ok := emojiCache[emoji]
	emojiCacheMu.RUnlock()
	if ok {
		return img
	}

	// Convert emoji to hex string for Twemoji URL
	var hexParts []string
	for _, r := range emoji {
		hexParts = append(hexParts, fmt.Sprintf("%x", r))
	}
	hexStr := strings.Join(hexParts, "-")
	url := fmt.Sprintf("https://abs.twimg.com/emoji/v2/72x72/%s.png", hexStr)

	resp, err := emojiTransport.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	img, _, err = image.Decode(resp.Body)
	if err != nil {
		return nil
	}

	emojiCacheMu.Lock()
	emojiCache[emoji] = img
	emojiCacheMu.Unlock()
	return img
}

func (h FallbackHandler) Box(txt string, fnt font.Font) (vg.Length, vg.Length, vg.Length) {
	lines := h.Lines(txt)
	var maxW vg.Length
	var hgt, depth vg.Length
	extPrimary := h.Extents(fnt)

	for i, line := range lines {
		var w vg.Length
		gr := uniseg.NewGraphemes(line)
		for gr.Next() {
			cluster := gr.Str()
			if isEmoji(cluster) {
				// Emojis are treated as square based on font size
				w += fnt.Size
				if fnt.Size > hgt {
					hgt = fnt.Size
				}
			} else {
				for _, r := range cluster {
					face := h.findFace(r, fnt)
					w += face.Width(string(r))
					ext := face.Extents()
					if ext.Ascent > hgt {
						hgt = ext.Ascent
					}
					if ext.Descent > depth {
						depth = ext.Descent
					}
				}
			}
		}
		if w > maxW {
			maxW = w
		}
		if i > 0 {
			hgt += extPrimary.Height
		}
	}
	return maxW, hgt, depth
}

func (h FallbackHandler) Draw(c vg.Canvas, txt string, sty text.Style, pt vg.Point) {
	lines := h.Lines(txt)
	if len(lines) == 0 {
		return
	}

	c.Push()
	defer c.Pop()
	if sty.Rotation != 0 {
		c.Translate(pt)
		c.Rotate(sty.Rotation)
		pt = vg.Point{}
	}

	_, hgt, d := h.Box(txt, sty.Font)
	extPrimary := h.Extents(sty.Font)
	c.SetColor(sty.Color)

	totalHeight := vg.Length(len(lines)-1)*extPrimary.Height + hgt + d

	var yOffset vg.Length
	switch sty.YAlign {
	case vgdraw.YTop:
		yOffset = -hgt
	case vgdraw.YCenter:
		yOffset = totalHeight/2 - hgt
	case vgdraw.YBottom:
		yOffset = d
	}

	for i, line := range lines {
		lpt := pt
		lpt.Y = pt.Y + yOffset - vg.Length(i)*extPrimary.Height

		lw, _, _ := h.Box(line, sty.Font)
		lpt.X = pt.X + vg.Length(sty.XAlign)*lw

		gr := uniseg.NewGraphemes(line)
		for gr.Next() {
			cluster := gr.Str()
			if isEmoji(cluster) {
				img := getEmojiImage(cluster)
				if img != nil {
					rect := vg.Rectangle{
						Min: vg.Point{X: lpt.X, Y: lpt.Y},
						Max: vg.Point{X: lpt.X + sty.Font.Size, Y: lpt.Y + sty.Font.Size},
					}
					c.DrawImage(rect, img)
				}
				lpt.X += sty.Font.Size
			} else {
				var currentFace font.Face
				var currentStart int
				var currentWidth vg.Length
				runes := []rune(cluster)
				for j, r := range runes {
					face := h.findFace(r, sty.Font)
					if j == 0 {
						currentFace = face
						currentStart = 0
					} else if face.Name() != currentFace.Name() {
						c.FillString(currentFace, lpt, string(runes[currentStart:j]))
						lpt.X += currentWidth
						currentFace = face
						currentStart = j
						currentWidth = 0
					}
					currentWidth += face.Width(string(r))
				}
				if len(runes) > 0 {
					c.FillString(currentFace, lpt, string(runes[currentStart:]))
					lpt.X += currentWidth
				}
			}
		}
	}
}

func init() {
	var coll font.Collection

	if face, err := opentype.Parse(dejaVuSansData); err == nil {
		coll = append(coll, font.Face{
			Font: font.Font{Typeface: "DejaVu Sans"},
			Face: face,
		})
	} else {
		log.Printf("Warning: failed to parse embedded DejaVuSans font: %v", err)
	}

	coll = append(coll, liberation.Collection()...)

	cache := font.NewCache(coll)
	plot.DefaultTextHandler = FallbackHandler{fonts: cache}
	plot.DefaultFont.Typeface = "DejaVu Sans"
	plot.DefaultFont.Variant = ""
	plotter.DefaultFont = plot.DefaultFont
}

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

func createLinearPlot(rows []StatsRow, showNotes bool) ([]byte, error) {
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

	p.X.Tick.Marker = plot.TimeTicks{
		Ticker: dateTicker{},
		Format: "2006-01-02",
	}

	// Annotations
	if showNotes {
		for _, row := range rows {
			if row.Note != "" {
				labels, err := plotter.NewLabels(plotter.XYLabels{
					XYs:    []plotter.XY{{X: float64(row.Date.Unix()), Y: row.Subscribers}},
					Labels: []string{row.Note},
				})
				if err == nil {
					labels.Offset = vg.Point{X: 0, Y: -20}
					labels.TextStyle[0].XAlign = vgdraw.XCenter
					labels.TextStyle[0].Handler = plot.DefaultTextHandler
					p.Add(labels)
				}
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
		days := float64(row.Date.Sub(minDate)) / float64(24*time.Hour)
		pts[i].X = float64(row.Date.Unix())
		pts[i].Y = row.Subscribers
		xValues[i] = days
		yValues[i] = row.Subscribers
	}

	daysToUnix := func(days float64) float64 {
		return float64(minDate.Add(time.Duration(days * float64(24*time.Hour))).Unix())
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
	p.X.Label.Text = "Data"
	p.Y.Label.Text = "Numero Iscritti"
	p.Add(plotter.NewGrid())

	p.X.Tick.Marker = plot.TimeTicks{
		Ticker: dateTicker{},
		Format: "2006-01-02",
	}

	// Real data
	realLine, realPoints, _ := plotter.NewLinePoints(pts)
	realLine.Color = color.RGBA{R: 0, G: 0, B: 255, A: 255}
	p.Add(realLine, realPoints)
	p.Legend.Add("Iscritti Reali", realLine, realPoints)

	// Fitted curve
	fittedPts := make(plotter.XYs, 100)
	maxX := xValues[len(xValues)-1]
	for i := 0; i < 100; i++ {
		days := float64(i) * maxX / 99
		fittedPts[i].X = daysToUnix(days)
		fittedPts[i].Y = polyFunc(days)
	}
	fittedLine, _ := plotter.NewLine(fittedPts)
	fittedLine.Color = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	fittedLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
	p.Add(fittedLine)
	p.Legend.Add(fmt.Sprintf("Curva Fittata (R²=%.4f)", rSquared), fittedLine)

	// Prediction (next 365 days)
	futurePts := make(plotter.XYs, 100)
	for i := 0; i < 100; i++ {
		days := maxX + float64(i)*365/99
		futurePts[i].X = daysToUnix(days)
		futurePts[i].Y = polyFunc(days)
	}
	futureLine, _ := plotter.NewLine(futurePts)
	futureLine.Color = color.RGBA{R: 0, G: 255, B: 0, A: 255}
	futureLine.Dashes = []vg.Length{vg.Points(5), vg.Points(5)}
	p.Add(futureLine)
	p.Legend.Add("Previsione", futureLine)

	p.Legend.Top = true
	p.Legend.Left = true
	p.Legend.Padding = vg.Points(10)
	// Move legend further from the top/left edges to avoid axis labels
	p.Legend.XOffs = vg.Points(40)
	p.Legend.YOffs = -vg.Points(40)

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
	showNotes := bot.StatsShowNotes
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

	plot1, err := createLinearPlot(rows, showNotes)
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

	_, err = bot.Bot.Request(mediaGroup)
	if err != nil {
		log.Printf("Error sending stats plots: %v", err)
	}
}
