package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
)

const logFile = "/tmp/temperature.log"

// TemperatureData stores humidity and temperature
type TemperatureData struct {
	Temperature float64
	Humidity    float64
}

// LogLine defines log data structure
type LogLine struct {
	Time        time.Time
	Temperature float64
	Humidity    float64
}

// TemplateInput holds data for index template
type TemplateInput struct {
	CurrentTemperature TemperatureData
	LogLines           []LogLine
}

var gpioMutex sync.Mutex
var tempeHumidMutex sync.RWMutex

var temperatureData TemperatureData

func readTemperature() error {
	binPath := "/usr/local/bin/temperature.py"
	log.Printf("Attempting to execute %s\n", binPath)

	cmd := exec.Command(binPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	gpioMutex.Lock()
	defer gpioMutex.Unlock()

	err := cmd.Run()

	if err != nil {
		return err
	}

	output := strings.Trim(stdout.String(), "\n")
	vals := strings.Split(output, ",")

	log.Printf("%v", vals)

	if len(vals) < 2 {
		return fmt.Errorf("Can't parse output, stdout: '%s', stderr: '%s'", output, stderr.String())
	}

	tempTemperature, err := strconv.ParseFloat(vals[0], 64)

	if err != nil {
		return err
	}

	tempHumidity, err := strconv.ParseFloat(vals[1], 64)

	if err != nil {
		return err
	}

	tempeHumidMutex.Lock()
	defer tempeHumidMutex.Unlock()
	temperatureData.Temperature = tempTemperature
	temperatureData.Humidity = tempHumidity

	return nil
}

func writeDataToLog() {
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		os.Create(logFile)
	}

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0666)

	if err != nil {
		log.Printf("Error while opening log file: %s\n", err)
		return
	}

	defer f.Close()

	tempeHumidMutex.RLock()
	defer tempeHumidMutex.RUnlock()

	if temperatureData.Temperature > 0 && temperatureData.Humidity > 0 {
		line := LogLine{
			Time:        time.Now(),
			Temperature: temperatureData.Temperature,
			Humidity:    temperatureData.Humidity,
		}

		encodedLine, err := json.Marshal(line)

		_, err = f.Write(encodedLine)
		if err != nil {
			log.Printf("Error while writing line to the log file: %s", err)
		}

		_, err = f.WriteString("\n")
		if err != nil {
			log.Printf("Error while writing line to the log file: %s", err)
		}
	} else {
		log.Println("Skipping log write since values are 0")
	}
}

func loadDataFromLog() (values []LogLine) {
	blob, err := ioutil.ReadFile(logFile)

	if err != nil {
		log.Printf("Error while reading log file data: %s", err)
	}

	lines := bytes.Split(blob, []byte("\n"))

	timeToFilterBy := time.Now().Add(-24 * time.Hour)

	for _, encodedLine := range lines {
		var line LogLine
		err := json.Unmarshal(encodedLine, &line)

		if err != nil {
			log.Printf("Error unmarshaling line: %s for data '%s'", err, string(encodedLine))
		} else {
			if line.Time.After(timeToFilterBy) {
				values = append(values, line)
			}
		}
	}

	return
}

func startLoops() {
	_ = readTemperature()

	go func() {
		for {
			err := readTemperature()

			if err != nil {
				log.Printf("Error during temperature readot: %s\n", err)
			}

			time.Sleep(time.Minute)
		}
	}()

	go func() {
		for {
			writeDataToLog()
			time.Sleep(time.Minute * 10)
		}
	}()
}

func rawTemperatureHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Header().Add("Content-Type", "text/plain")

	tempeHumidMutex.RLock()
	defer tempeHumidMutex.RUnlock()
	fmt.Fprintf(w, "T = %v*C, H = %v%%", temperatureData.Temperature, temperatureData.Humidity)
}

func indexHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := template.ParseFiles("templates/index.html")

	logLines := loadDataFromLog()

	tempeHumidMutex.RLock()
	defer tempeHumidMutex.RUnlock()

	input := TemplateInput{
		CurrentTemperature: temperatureData,
		LogLines:           logLines,
	}

	t.Execute(w, input)
}

func main() {
	router := httprouter.New()
	router.GET("/", indexHandler)
	router.GET("/raw.txt", rawTemperatureHandler)
	router.ServeFiles("/public/*filepath", http.Dir("public/"))

	startLoops()

	log.Println("Ready to serve!")
	log.Fatal(http.ListenAndServe(":8080", router))
}
