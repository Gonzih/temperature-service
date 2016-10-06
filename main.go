package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
)

var gpioMutex sync.Mutex
var tempeHumidMutex sync.RWMutex

var temperature, humidity float64

func readTemperature() error {
	usr, err := user.Current()

	if err != nil {
		return fmt.Errorf("Error while getting info about current user: %s", err)
	}

	cmd := exec.Command(usr.HomeDir + "/bin/temperature.py")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	gpioMutex.Lock()
	defer gpioMutex.Unlock()

	err = cmd.Run()

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
	temperature = tempTemperature
	humidity = tempHumidity

	return nil
}

func rawTemperatureHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	gpioMutex.Lock()
	defer gpioMutex.Unlock()

	w.Header().Add("Content-Type", "text/plain")

	tempeHumidMutex.RLock()
	defer tempeHumidMutex.RUnlock()
	fmt.Fprintf(w, "T = %v*C, H = %v%%", temperature, humidity)
}

func startLoop() {
	go func() {
		for {
			err := readTemperature()

			if err != nil {
				log.Printf("Error during temperature readot: %s\n", err)
			}

			time.Sleep(time.Minute)
		}
	}()
}

func main() {
	router := httprouter.New()
	router.GET("/raw.txt", rawTemperatureHandler)

	startLoop()

	log.Println("Ready to serve!")
	log.Fatal(http.ListenAndServe(":8080", router))
}
