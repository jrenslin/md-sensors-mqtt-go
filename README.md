# Connector from Shelly H&T sensors to museum-digital:musdb

This app listens to an MQTT server's general channel (#) and attempts to read
all temperature and humidity values sent by the present Shelly H&T devices based
on their respective submission channels. A mapping between the channels (= the
respective sensors) and spaces in musdb can be done in the configuration file.

Sensor data are then pooled and submitted to the server once a sufficient amount of
data has been collected to reduce load on the server.

## Configuration

This tool expects a JSON configuration file to be present in
`~/.config/md-sensors-mqtt-go/conf.json`, formed according to the example set in
[./conf.sample.json].

## Relevant Assumptions

Thus far, this app requires the sensors to use Celsius as the default temperature unit.

## An Example Setup

In the development setup, Shelly H&T sensors are connected to a Wifi network. In the same network, a Raspberry Pi runs an MQTT server (here realized using [Mosquito](https://mosquitto.org/)) and runs this app. The MQTT server could however also be run on another device.

The Shelly H&Ts - once set up for using MQTT - send sensor data via MQTT in two ways:
- A JSON representation of all relevant measurements in one channel
- Numeric representations of the sensor's data (e.g. humidity and temperature) in dedicated channels

This app requires the dedicated channels for numeric representations of temperature and humidity (e.g. `shellies/shellyht-012345/sensor/temperature` and `shellies/shellyht-012345/sensor/humidity`) to be mapped to spaces. It subscribes to the general channel (`#`) to check if any message has been logged to any of the relevant channels and gathers them in a list.

Once a given number of log entries has been collected, the data is sent to the configured endpoint of [museum-digital:musdb](https://en.about.museum-digital.org/software/musdb/) in bulk. It is advisable to select a number of collected values required for submitting to the server that meets the conditions of the concrete installation to ensure a good balance between timely updates and resource saving. The H&Ts generally send their log data quite rarely if they are battery run, wired ones are much more verbose. It thus makes sense to select a larger number of collected values if one uses a setup with wired H&Ts. Similarly, if there are a lot of configured sensors in the network (we aim for about 15 sensors in one network in the first production installation at the [Goethe Museum Frankfurt](https://freies-deutsches-hochstift.de/)), there will be a lot more data to log and the number of collected entries can be set quite high.
