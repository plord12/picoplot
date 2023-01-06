package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

/*
Plot pico usage
*/
func main() {

	today := time.Now()
	yesterday := time.Now().AddDate(0, 0, -1)
	defaultFrom := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, yesterday.Location())
	defaultTo := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	// Parse arguments
	//
	serialNum := flag.String("serialNum", "", "Pico serial number")
	from := flag.String("from", defaultFrom.Format("2006-01-02T15:04:05Z"), "Report start date/time")
	to := flag.String("to", defaultTo.Format("2006-01-02T15:04:05Z"), "Report end date/time")
	signalUser := flag.String("signaluser", "", "Signal messenger username")
	signalGroup := flag.String("signalgroup", "", "Signal messenger groupid")
	signalRecipient := flag.String("signalrecipient", "", "Signal messenger recipient")

	flag.Parse()

	if len(*serialNum) == 0 {
		log.Println("serial number must be provided")
		flag.PrintDefaults()
		os.Exit(1)
	}

	fromTime, err := time.Parse(time.RFC3339, *from)
	if err != nil {
		log.Fatalf("failed to parse from: %s", err.Error())
	}
	toTime, err := time.Parse(time.RFC3339, *to)
	if err != nil {
		log.Fatalf("failed to parse to: %s", err.Error())
	}

	text, images, err := report(serialNum, &fromTime, &toTime)
	if err != nil {
		log.Fatalf("report failed: %s", err.Error())
	}

	alert(signalUser, signalRecipient, signalGroup, text, images)

	for _, s := range images {
		os.Remove(s)
	}

}

// send an alert via signal
func alert(signalUser *string, signalRecipient *string, signalGroup *string, message string, attachments []string) error {
	if (len(*signalUser) > 0) && (len(*signalGroup) > 0 || len(*signalRecipient) > 0) {

		// keep signal happy
		//
		cmd := exec.Command("signal-cli", "-u", *signalUser, "receive")
		stdout, err := cmd.CombinedOutput()
		if err != nil {
			return errors.New("signal-cli failed - " + string(stdout))
		}
		//log.Println(string(stdout[:]))

		var args []string
		args = append(args, "-u")
		args = append(args, *signalUser)
		args = append(args, "send")
		if len(*signalGroup) > 0 {
			args = append(args, "-g")
			args = append(args, *signalGroup)
		} else {
			args = append(args, strings.Split(*signalRecipient, " ")...)
		}
		if len(message) > 0 {
			args = append(args, "-m")
			args = append(args, message)
		}
		if len(attachments) > 0 {
			args = append(args, "-a")
			args = append(args, attachments...)
		}
		log.Printf("signal-cli %v\n", args)
		cmd = exec.Command("signal-cli", args...)

		stdout, err = cmd.CombinedOutput()
		if err != nil {
			return errors.New("signal-cli failed - " + string(stdout))
		}
	}

	return nil
}

func bold(original string) string {

	makeBold := func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r - 'A' + '𝐀'
		case r >= 'a' && r <= 'z':
			return r - 'a' + '𝐚'
		case r >= '0' && r <= '9':
			return r - '0' + '𝟎'
		}

		return r
	}
	return strings.Map(makeBold, original)
}

type Data struct {
	Tvoc        string
	ReportTime  string
	SerialNum   string
	Pm25        string
	Lng         string
	Ip          string
	Co2         string
	Pm10        string
	Temperature string
	Humidity    string
	Lat         string
}

type Result struct {
	Result string
	Data   []Data
}

func report(serialNum *string, from *time.Time, to *time.Time) (string, []string, error) {

	var images []string

	fromString := from.Format("20060102150405")
	toString := to.Format("20060102150405")

	// get all data
	//
	var jsonValue = []byte(`{"serialNum":"` + *serialNum + `","startTime":"` + fromString + `","endTime":"` + toString + `","type":"Co2,Humid,Pm10,Pm25,Temperature,Tvoc"}`)
	resp, err := http.Post("http://mqtt.brilcom.com:8080/mqtt/GetAirQualityForChart", "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return "", nil, errors.New("unable to get pico data - " + err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, errors.New("unable to get pico data - " + err.Error())
	}

	// parse

	var result Result
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", nil, errors.New("json parse failed - " + err.Error())
	}

	var xaxis []time.Time
	var co2 []float64
	var humidity []float64
	var pm10 []float64
	var pm25 []float64
	var temperature []float64
	var tvoc []float64

	maxCo2 := math.SmallestNonzeroFloat32
	maxCo2Time := time.Now()
	maxVoc := math.SmallestNonzeroFloat32
	maxVocTime := time.Now()
	maxPm10 := math.SmallestNonzeroFloat32
	maxPm10Time := time.Now()
	maxPm25 := math.SmallestNonzeroFloat32
	maxPm25Time := time.Now()

	for i := 0; i < len(result.Data); i++ {
		t, err := time.Parse("2006-01-02T15:04:05", result.Data[i].ReportTime)
		if err != nil {
			return "", nil, errors.New("parse time failed - " + err.Error())
		}
		xaxis = append(xaxis, t)

		s, err := strconv.ParseFloat(result.Data[i].Co2, 32)
		if err != nil {
			return "", nil, errors.New("parse float failed - " + err.Error())
		}
		co2 = append(co2, s)
		if s > maxCo2 {
			maxCo2 = s
			maxCo2Time = t
		}

		s, err = strconv.ParseFloat(result.Data[i].Humidity, 32)
		if err != nil {
			return "", nil, errors.New("parse float failed - " + err.Error())
		}
		humidity = append(humidity, s)

		s, err = strconv.ParseFloat(result.Data[i].Pm10, 32)
		if err != nil {
			return "", nil, errors.New("parse float failed - " + err.Error())
		}
		pm10 = append(pm10, s)
		if s > maxPm10 {
			maxPm10 = s
			maxPm10Time = t
		}

		s, err = strconv.ParseFloat(result.Data[i].Pm25, 32)
		if err != nil {
			return "", nil, errors.New("parse float failed - " + err.Error())
		}
		pm25 = append(pm25, s)
		if s > maxPm25 {
			maxPm25 = s
			maxPm25Time = t
		}

		s, err = strconv.ParseFloat(result.Data[i].Temperature, 32)
		if err != nil {
			return "", nil, errors.New("parse float failed - " + err.Error())
		}
		temperature = append(temperature, s)

		s, err = strconv.ParseFloat(result.Data[i].Tvoc, 32)
		if err != nil {
			return "", nil, errors.New("parse float failed - " + err.Error())
		}
		tvoc = append(tvoc, s)
		if s > maxVoc {
			maxVoc = s
			maxVocTime = t
		}
	}

	// text
	//
	text := ""
	if maxCo2 > 800 {
		text = fmt.Sprintf("%s CO₂ level %s at %s", text, bold(fmt.Sprintf("high (%0.1f ppm)", maxCo2)), maxCo2Time.Format("15:04"))
	}
	if maxPm25 > 15 {
		text = fmt.Sprintf("%s PM2.5 level %s at %s", text, bold(fmt.Sprintf("high (%0.1f µg/m³)", maxPm25)), maxPm25Time.Format("15:04"))
	}
	if maxPm10 > 30 {
		text = fmt.Sprintf("%s PM10 level %s at %s", text, bold(fmt.Sprintf("high (%0.1f µg/m³)", maxPm10)), maxPm10Time.Format("15:04"))
	}
	if maxVoc > 250 {
		text = fmt.Sprintf("%s VOC level %s at %s", text, bold(fmt.Sprintf("high (%0.1f ppb)", maxVoc)), maxVocTime.Format("15:04"))
	}

	// chart
	//
	var ticks []chart.Tick
	for _, t := range xaxis {
		ticks = append(ticks, chart.Tick{Value: float64(t.UnixNano()), Label: t.Format("Jan-02-06 15:04")})
	}

	Pm25Color := func(xr, yr chart.Range, index int, x, y float64) drawing.Color {
		if y < 15 {
			return chart.ColorBlue
		}
		if y < 35 {
			return chart.ColorGreen
		}
		if y < 75 {
			return chart.ColorOrange
		}
		return chart.ColorRed
	}

	graph := chart.Chart{
		Title:      "PM2.5",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "µg/m³",
			NameStyle: chart.Style{FontColor: chart.ColorBlack},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: pm25,
				Style:   chart.Style{StrokeColor: chart.ColorBlack, DotWidth: 3, DotColorProvider: Pm25Color},
			},
		},
	}
	f, _ := os.CreateTemp("", "*.png")
	defer f.Close()
	renderError := graph.Render(chart.PNG, f)
	if renderError != nil {
		return "", nil, errors.New("failed render chart: " + renderError.Error())
	}
	images = append(images, f.Name())

	Pm10Color := func(xr, yr chart.Range, index int, x, y float64) drawing.Color {
		if y < 30 {
			return chart.ColorBlue
		}
		if y < 80 {
			return chart.ColorGreen
		}
		if y < 150 {
			return chart.ColorOrange
		}
		return chart.ColorRed
	}

	graph = chart.Chart{
		Title:      "PM10",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "µg/m³",
			NameStyle: chart.Style{FontColor: chart.ColorBlack},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: pm10,
				Style:   chart.Style{StrokeColor: chart.ColorBlack, DotWidth: 3, DotColorProvider: Pm10Color},
			},
		},
	}
	f, _ = os.CreateTemp("", "*.png")
	defer f.Close()
	renderError = graph.Render(chart.PNG, f)
	if renderError != nil {
		return "", nil, errors.New("failed render chart: " + renderError.Error())
	}
	images = append(images, f.Name())

	VocColor := func(xr, yr chart.Range, index int, x, y float64) drawing.Color {
		if y < 249 {
			return chart.ColorBlue
		}
		if y < 449 {
			return chart.ColorGreen
		}
		return chart.ColorRed
	}

	graph = chart.Chart{
		Title:      "VOC",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "ppb",
			NameStyle: chart.Style{FontColor: chart.ColorBlack},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: tvoc,
				Style:   chart.Style{StrokeColor: chart.ColorBlack, DotWidth: 3, DotColorProvider: VocColor},
			},
		},
	}
	f, _ = os.CreateTemp("", "*.png")
	defer f.Close()
	renderError = graph.Render(chart.PNG, f)
	if renderError != nil {
		return "", nil, errors.New("failed render chart: " + renderError.Error())
	}
	images = append(images, f.Name())

	Co2Color := func(xr, yr chart.Range, index int, x, y float64) drawing.Color {
		if y < 800 {
			return chart.ColorBlue
		}
		if y < 1000 {
			return chart.ColorGreen
		}
		if y < 2000 {
			return chart.ColorOrange
		}
		return chart.ColorRed
	}

	graph = chart.Chart{
		Title:      "CO₂",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "ppm",
			NameStyle: chart.Style{FontColor: chart.ColorBlack},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: co2,
				Style:   chart.Style{StrokeColor: chart.ColorBlack, DotWidth: 3, DotColorProvider: Co2Color},
			},
		},
	}
	f, _ = os.CreateTemp("", "*.png")
	defer f.Close()
	renderError = graph.Render(chart.PNG, f)
	if renderError != nil {
		return "", nil, errors.New("failed render chart: " + renderError.Error())
	}
	images = append(images, f.Name())

	graph = chart.Chart{
		Title:      "Temperature",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "°C",
			NameStyle: chart.Style{FontColor: chart.ColorBlack},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: temperature,
				Style:   chart.Style{StrokeColor: chart.ColorBlack, DotWidth: 3, DotColor: chart.ColorBlack},
			},
		},
	}
	f, _ = os.CreateTemp("", "*.png")
	defer f.Close()
	renderError = graph.Render(chart.PNG, f)
	if renderError != nil {
		return "", nil, errors.New("failed render chart: " + renderError.Error())
	}
	images = append(images, f.Name())

	graph = chart.Chart{
		Title:      "Humidity",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "%",
			NameStyle: chart.Style{FontColor: chart.ColorBlack},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: humidity,
				Style:   chart.Style{StrokeColor: chart.ColorBlack, DotWidth: 3, DotColor: chart.ColorBlack},
			},
		},
	}
	f, _ = os.CreateTemp("", "*.png")
	defer f.Close()
	renderError = graph.Render(chart.PNG, f)
	if renderError != nil {
		return "", nil, errors.New("failed render chart: " + renderError.Error())
	}
	images = append(images, f.Name())

	return text, images, nil
}
