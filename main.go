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
		log.Fatalf("Electricity failed: %s", err.Error())
	}

	if len(*signalUser) > 0 && len(*signalRecipient) > 0 {

		var args []string
		args = append(args, "-u")
		args = append(args, *signalUser)
		args = append(args, "send")
		args = append(args, strings.Split(*signalRecipient, " ")...)
		if len(text) > 0 {
			args = append(args, "-m")
			args = append(args, text)
		}
		if len(images) > 0 {
			args = append(args, "-a")
			args = append(args, images...)
		}
		log.Printf("signal-cli %v\n", args)
		cmd := exec.Command("signal-cli", args...)

		stdout, err := cmd.CombinedOutput()
		if err != nil {
			log.Println(err.Error())
		}
		log.Println(string(stdout))
	}

	for _, s := range images {
		os.Remove(s)
	}

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
	minCo2 := math.MaxFloat32
	maxVoc := math.SmallestNonzeroFloat32
	minVoc := math.MaxFloat32
	maxPm10 := math.SmallestNonzeroFloat32
	minPm10 := math.MaxFloat32
	maxPm25 := math.SmallestNonzeroFloat32
	minPm25 := math.MaxFloat32

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
		}
		if s < minCo2 {
			minCo2 = s
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
		}
		if s < minPm10 {
			minPm10 = s
		}

		s, err = strconv.ParseFloat(result.Data[i].Pm25, 32)
		if err != nil {
			return "", nil, errors.New("parse float failed - " + err.Error())
		}
		pm25 = append(pm25, s)
		if s > maxPm25 {
			maxPm25 = s
		}
		if s < minPm25 {
			minPm25 = s
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
		}
		if s < minVoc {
			minVoc = s
		}
	}

	// text
	//
	text := fmt.Sprintf("Co2 ranges from %.1f ppm to %.1f ppm, VOC ranges from %.1f ppb to %.1f ppb, PM10 ranges from %.1f ug/m3 to %.1f ug/m3, PM2.5 ranges from %.1f ug/m3 to %.1f ug/m3", minCo2, maxCo2, minVoc, maxVoc, minPm10, maxPm10, minPm25, maxPm25)

	// chart
	//
	var ticks []chart.Tick
	for _, t := range xaxis {
		ticks = append(ticks, chart.Tick{Value: float64(t.UnixNano()), Label: t.Format("Jan-02-06 15:04")})
	}

	graph := chart.Chart{
		Title:      "PM2.5",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "um/m3",
			NameStyle: chart.Style{FontColor: chart.ColorRed},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: pm25,
				Style:   chart.Style{StrokeColor: chart.ColorRed, DotWidth: 3, DotColor: chart.ColorRed},
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

	graph = chart.Chart{
		Title:      "PM10",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "ug/m3",
			NameStyle: chart.Style{FontColor: chart.ColorRed},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: pm10,
				Style:   chart.Style{StrokeColor: chart.ColorRed, DotWidth: 3, DotColor: chart.ColorRed},
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
		Title:      "VOC",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "ppb",
			NameStyle: chart.Style{FontColor: chart.ColorRed},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: tvoc,
				Style:   chart.Style{StrokeColor: chart.ColorRed, DotWidth: 3, DotColor: chart.ColorRed},
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
		Title:      "CO2",
		Background: chart.Style{Padding: chart.Box{Top: 20, Left: 20, Right: 20, Bottom: 20}},
		XAxis: chart.XAxis{
			Style: chart.Style{TextRotationDegrees: 90.0, FontSize: 6},
			Ticks: ticks,
		},
		YAxis: chart.YAxis{
			Name:      "ppm",
			NameStyle: chart.Style{FontColor: chart.ColorRed},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: co2,
				Style:   chart.Style{StrokeColor: chart.ColorRed, DotWidth: 3, DotColor: chart.ColorRed},
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
			Name:      "C",
			NameStyle: chart.Style{FontColor: chart.ColorRed},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: temperature,
				Style:   chart.Style{StrokeColor: chart.ColorRed, DotWidth: 3, DotColor: chart.ColorRed},
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
			NameStyle: chart.Style{FontColor: chart.ColorRed},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: xaxis,
				YValues: humidity,
				Style:   chart.Style{StrokeColor: chart.ColorRed, DotWidth: 3, DotColor: chart.ColorRed},
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
