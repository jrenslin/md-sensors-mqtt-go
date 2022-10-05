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
