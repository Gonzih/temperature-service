package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/julienschmidt/httprouter"
)

var gpioMutex sync.Mutex

func readTemperature() (float64, float64, error) {
	cmd := exec.Command("/home/debian/bin/temperature.py")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err != nil {
		return 0, 0, err
	}

	output := strings.Trim(stdout.String(), "\n")
	vals := strings.Split(output, ",")

	log.Printf("%v", vals)

	if len(vals) < 2 {
		return 0, 0, fmt.Errorf("Can't parse output, stdout: '%s', stderr: '%s'", output, stderr.String())
	}

	temperature, err := strconv.ParseFloat(vals[0], 64)

	if err != nil {
		return 0, 0, err
	}

	humidity, err := strconv.ParseFloat(vals[1], 64)

	if err != nil {
		return 0, 0, err
	}

	return temperature, humidity, nil
}

func temperatureHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	gpioMutex.Lock()
	defer gpioMutex.Unlock()

	w.Header().Add("Content-Type", "text/plain")

	temperature, humidity, err := readTemperature()

	if err != nil {
		fmt.Fprintf(w, "Error occured during temp readout: %s", err)
	} else {
		fmt.Fprintf(w, "Temperature = %v*C, Humidity = %v%%", temperature, humidity)
	}
}

func main() {
	router := httprouter.New()
	router.GET("/", temperatureHandler)

	log.Println("Ready to serve!")
	log.Fatal(http.ListenAndServe(":8080", router))
}
