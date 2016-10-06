package main

import (
	"bytes"
	"fmt"
	"html/template"
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
		line := fmt.Sprintf("%v,%v\n", temperatureData.Temperature, temperatureData.Humidity)
		_, err = f.WriteString(line)

		if err != nil {
			log.Printf("Error while writing line to the log file: %s", err)
		}
	} else {
		log.Println("Skipping log write since values are 0")
	}
}

func startLoops() {
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
	t.Execute(w, temperatureData)
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
