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

const MD_ENDPOINT = "https://sandkasten.museum-digital.de/musdb/remote/space_sensors/?token_id=50&token=3aef5c3ea2055bc9dea0c9fad583e0910b0a6f268c84e61bfdc290971908"

type Configuration struct {
    MqttIp string `json:"mqtt_ip"`
    MqttPort string `json:"mqtt_port"`
    MqttUsername string `json:"mqtt_username"`
    MqttPasswort string `json:"mqtt_password"`
    MusdbEndpoint string `json:"musdb_endpoint"`
    SubmitAfterEntries int `json:"submit_after_entries"`
    ChannelsToSpaceIds map[string]int `json:"channels_to_space_ids"`
}

type sensorLogPoint struct {
    SpaceId int `json:"space_id"`
    Value float64 `json:"value"`
    Time string `json:"time"`
}

type sensorLogPointReduced struct {
    Time string `json:"time"`
    Value float64 `json:"value"`
}

type submissionFormat struct {
    SpaceId int `json:"space_id"`
    Humidity []sensorLogPointReduced `json:"humidity"`
    Temperature []sensorLogPointReduced `json:"temperature"`
}

var sensorLogTmp []sensorLogPoint
var sensorLogHum []sensorLogPoint

var configuration Configuration

func echo(text string) {
    dt := time.Now()
    fmt.Printf("%s : %s", dt.String(), text)
}

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

func createClientOptions(clientId string) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()
    opts.AddBroker(fmt.Sprintf("tcp://%s", configuration.MqttIp + ":" + configuration.MqttPort))
	opts.SetUsername(configuration.MqttUsername)
	password := configuration.MqttPasswort
	opts.SetPassword(password)
	opts.SetClientID(clientId)
	return opts
}

func submit() {

    // Wait for some time, to surely capture latest humidity
    time.Sleep(8 * time.Second)

    toSubmit := map[int]submissionFormat{}

    // Humidity

    for _, entry := range sensorLogHum {

        if _, ok := toSubmit[entry.SpaceId]; ok {
        } else {
            toSubmit[entry.SpaceId] = submissionFormat {
                SpaceId: entry.SpaceId,
                Humidity: make([]sensorLogPointReduced, 0),
                Temperature: make([]sensorLogPointReduced, 0),
            }
        }

       if submitEntry, ok := toSubmit[entry.SpaceId]; ok {
           submitEntry.Humidity = append(submitEntry.Humidity, sensorLogPointReduced {
            Time: entry.Time,
            Value: entry.Value,
        })
           toSubmit[entry.SpaceId] = submitEntry
       }

    }

    // Temperature

    for _, entry := range sensorLogTmp {

        if _, ok := toSubmit[entry.SpaceId]; ok {
        } else {
            toSubmit[entry.SpaceId] = submissionFormat {
                SpaceId: entry.SpaceId,
                Humidity: make([]sensorLogPointReduced, 0),
                Temperature: make([]sensorLogPointReduced, 0),
            }
        }

       if submitEntry, ok := toSubmit[entry.SpaceId]; ok {
           submitEntry.Temperature = append(submitEntry.Temperature, sensorLogPointReduced {
            Time: entry.Time,
            Value: entry.Value,
        })
           toSubmit[entry.SpaceId] = submitEntry
       }

    }

    bs, _ := json.Marshal(toSubmit)

    // Submit to server

    fmt.Println("Submitting to server:")
    fmt.Println(string(bs))

    data := url.Values{
        "log": {string(bs)},
    }

    resp, err := http.PostForm(configuration.MusdbEndpoint, data)

    if err != nil {
        log.Fatal(err)
    }

    var res map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&res)
    fmt.Println("Server response:")
    fmt.Println(res["form"])

    // Empty array

    sensorLogTmp = nil
    sensorLogHum = nil

}

func listen(topic string) {
	client := connect()
	client.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {

        currentTime := time.Now()
        now := currentTime.Format("2006-01-02 15:04:05")

        if strings.HasSuffix(msg.Topic(), "/humidity") {
            echo(fmt.Sprintf("* [%s] %s\n", msg.Topic(), string(msg.Payload())))
            if s, err := strconv.ParseFloat(string(msg.Payload()), 32); err == nil {

                if spaceId, ok := configuration.ChannelsToSpaceIds[msg.Topic()]; ok {
                    sensorLogHum = append(sensorLogHum, sensorLogPoint {
                        SpaceId: spaceId,
                        Value: s,
                        Time: now,
                    })
                } else {
                    panic("Unknown space: " + msg.Topic() + ". Enter the combination of topic and target space ID in the configuration file")
                }

                if len(sensorLogTmp) + len(sensorLogHum) > configuration.SubmitAfterEntries {
                    fmt.Printf("Loaded %d entries; more than configured %d required for submitting\n", len(sensorLogTmp) + len(sensorLogHum), configuration.SubmitAfterEntries)
                    go submit()
                }

            }
        }

        if strings.HasSuffix(msg.Topic(), "/temperature") {
            echo(fmt.Sprintf("* [%s] %s\n", msg.Topic(), string(msg.Payload())))

            if s, err := strconv.ParseFloat(string(msg.Payload()), 32); err == nil {

                if spaceId, ok := configuration.ChannelsToSpaceIds[msg.Topic()]; ok {
                    sensorLogTmp = append(sensorLogTmp, sensorLogPoint {
                        SpaceId: spaceId,
                        Value: s,
                        Time: now,
                    })
                } else {
                    panic("Unknown space: " + msg.Topic() + ". Enter the combination of topic and target space ID in the configuration file")
                }

            }
        }

	})
}

func main() {

    // Load config

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

    // Run

    keepAlive := make(chan os.Signal)

    fmt.Printf("Attempting to connect to %s\n", configuration.MqttUsername);
	go listen(MQTT_TOPIC)

    <-keepAlive

}
