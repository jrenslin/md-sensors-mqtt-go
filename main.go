package main

import (
	"fmt"
	"log"
	"time"
    "os"
    "errors"
    "encoding/json"

    "strings"
    "strconv"
    "net/http"
    "net/url"
    // "bytes"
    // "os/exec"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const MQTT_TOPIC = "#"

type Configuration struct {
    MqttIp string `json:"mqtt_ip"`
    MqttPort string `json:"mqtt_port"`
    MqttUsername string `json:"mqtt_username"`
    MqttPasswort string `json:"mqtt_password"`
    MusdbEndpoint string `json:"musdb_endpoint"`
    SubmitAfterEntries int `json:"submit_after_entries"`
    ChannelsToSpaceIds map[string]int `json:"channels_to_space_ids"`
}

// A log point in the cached lists of either temperature or humidity
// measurements.
type sensorLogPoint struct {
    SpaceId int `json:"space_id"`
    Value float64 `json:"value"`
    Time string `json:"time"`
}

// Sensor log points (either for tmp or humidity) formatted according
// to musdb's logging API's required format.
type sensorLogPointReduced struct {
    Time string `json:"time"`
    Value float64 `json:"value"`
}

// Format to describe a single measurement in musdb's logging API.
type submissionFormat struct {
    SpaceId int `json:"space_id"`
    Humidity []sensorLogPointReduced `json:"humidity"`
    Temperature []sensorLogPointReduced `json:"temperature"`
}

// "Global" variables

var configuration Configuration

// Local list of temperature measurements. Gets filled upon messages in the
// MQTT temperature channel and cleared upon submitting the data to the server.
var sensorLogTmp []sensorLogPoint

// Local list of humidity measurements. Gets filled upon messages in the
// MQTT humidity channel and cleared upon submitting the data to the server.
var sensorLogHum []sensorLogPoint

// Small wrapper around Printf for CLI output
func echo(text string) {
    dt := time.Now()
    fmt.Printf("%s : %s", dt.String(), text)
}

// Sets the options for connecting to the MQTT server, e.g. setting username
// and password.
func createClientOptions(clientId string) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()
    opts.AddBroker(fmt.Sprintf("tcp://%s", configuration.MqttIp + ":" + configuration.MqttPort))
	opts.SetUsername(configuration.MqttUsername)
	password := configuration.MqttPasswort
	opts.SetPassword(password)
	opts.SetClientID(clientId)
	return opts
}

// Connects to MQTT server
func connect() mqtt.Client {

    clientId := "md_sensors_php_mqtt";

	opts := createClientOptions(clientId)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		log.Fatal(err)
	}
	return client
}

// Submits preformulated sensor data (as defined through submit() to the
// configured server.
func submitToServer(toSubmit map[int]submissionFormat) {

    bs, _ := json.Marshal(toSubmit)

    // Submit to server

    fmt.Println("Submitting to server:")
    fmt.Println(string(bs))

    data := url.Values{
        "log": {string(bs)},
    }

    resp, err := http.PostForm(configuration.MusdbEndpoint, data)

    if err != nil {
        fmt.Println("Failed to submit to server")
        return
    }

    var res map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&res)
    fmt.Println("Server response:")
    fmt.Println(res["form"])

}

// Sets up an empty measurement in the submission format md.
func createEmptySubmissionMeasurement(spaceId int) submissionFormat {
    return submissionFormat {
        SpaceId: spaceId,
        Humidity: make([]sensorLogPointReduced, 0),
        Temperature: make([]sensorLogPointReduced, 0),
    }
}

// Function called when the list of cached measurements is long enough to warrant
// submission to the server. Reformulates the lists of logged data into the format
// required by the logging API at museum-digital and then submits the data.
// Finally, the cache variables are cleared.
func submit() {

    // Wait for some time, to surely capture latest humidity
    time.Sleep(8 * time.Second)

    toSubmit := map[int]submissionFormat{}

    // Transfer humidity measurements to the submission format
    for _, entry := range sensorLogHum {

        if _, ok := toSubmit[entry.SpaceId]; ok {
        } else {
            toSubmit[entry.SpaceId] = createEmptySubmissionMeasurement(entry.SpaceId)
        }

        if submitEntry, ok := toSubmit[entry.SpaceId]; ok {
            submitEntry.Humidity = append(submitEntry.Humidity, sensorLogPointReduced {
                Time: entry.Time,
                Value: entry.Value,
            })
            toSubmit[entry.SpaceId] = submitEntry
        }

    }

    // Transfer temperature measurements to the submission format
    for _, entry := range sensorLogTmp {

        if _, ok := toSubmit[entry.SpaceId]; ok {
        } else {
            toSubmit[entry.SpaceId] = createEmptySubmissionMeasurement(entry.SpaceId)
        }

        if submitEntry, ok := toSubmit[entry.SpaceId]; ok {
            submitEntry.Temperature = append(submitEntry.Temperature, sensorLogPointReduced {
                Time: entry.Time,
                Value: entry.Value,
            })
            toSubmit[entry.SpaceId] = submitEntry
        }

    }

    submitToServer(toSubmit)

    // Empty cache variables
    sensorLogTmp = nil
    sensorLogHum = nil

}

// Returns the parsed measurement as a float and space ID (in musdb) for
// a given MQTT topic and payload if possible.
// The third return value is a boolean for quickly grasping success or
// failure of parsing measurement and space ID.
func parseMeasurementAndSpace(topic string, payload string) (float64, int, bool) {

    echo(fmt.Sprintf("* [%s] %s\n", topic, string(payload)))

    if s, err := strconv.ParseFloat(payload, 32); err == nil {

        if spaceId, ok := configuration.ChannelsToSpaceIds[topic]; ok {
            return s, spaceId, true
        } else {
            log.Fatal("Unknown space: " + topic + ". Enter the combination of topic and target space ID in the configuration file")
        }

    }

    return 0.0, 0, false

}

// Main function for listening to MQTT and reacting to possible messages.
func listen(topic string) {

	client := connect()
	client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {

        currentTime := time.Now()
        now := currentTime.Format("2006-01-02 15:04:05")

        if strings.HasSuffix(msg.Topic(), "/humidity") {
            if s, spaceId, ok := parseMeasurementAndSpace(msg.Topic(), string(msg.Payload())); ok {
                sensorLogHum = append(sensorLogHum, sensorLogPoint {
                    SpaceId: spaceId,
                    Value: s,
                    Time: now,
                })
            }

            if len(sensorLogTmp) + len(sensorLogHum) > configuration.SubmitAfterEntries {
                fmt.Printf("Loaded %d entries; more than configured %d required for submitting\n", len(sensorLogTmp) + len(sensorLogHum), configuration.SubmitAfterEntries)
                go submit()
            }
        }

        if strings.HasSuffix(msg.Topic(), "/temperature") {
            if s, spaceId, ok := parseMeasurementAndSpace(msg.Topic(), string(msg.Payload())); ok {
                sensorLogTmp = append(sensorLogTmp, sensorLogPoint {
                    SpaceId: spaceId,
                    Value: s,
                    Time: now,
                })
            }
        }

	})
}

func main() {

    // Load configuration
    homeDir, err := os.UserHomeDir()
    if err != nil {
        panic( err )
    }

    if _, err := os.Stat(homeDir + "/.config/md-sensors-mqtt-go"); errors.Is(err, os.ErrNotExist) {
        os.Mkdir(homeDir + "/.config/md-sensors-mqtt-go", 0700)
        fmt.Println("Created directory " + homeDir + "/.config/md-sensors-mqtt-go")
    }

    if _, err := os.Stat(homeDir + "/.config/md-sensors-mqtt-go/conf.json"); errors.Is(err, os.ErrNotExist) {
        panic("Config file " + homeDir + "/.config/md-sensors-mqtt-go/conf.json does not exist");
    }

    file, _ := os.Open(homeDir + "/.config/md-sensors-mqtt-go/conf.json")
    defer file.Close()
    decoder := json.NewDecoder(file)
    configuration = Configuration{}
    fileDecodeErr := decoder.Decode(&configuration)
    if fileDecodeErr != nil {
      fmt.Println("Error parsing configuration:", fileDecodeErr)
    }

    if (configuration.SubmitAfterEntries == 0) {
        configuration.SubmitAfterEntries = 3
    }

    // Listen to MQTT server and react (until manual shutdown)

    keepAlive := make(chan os.Signal)

    fmt.Printf("Attempting to connect to %s\n", configuration.MqttUsername);
	go listen(MQTT_TOPIC)

    <-keepAlive

}
