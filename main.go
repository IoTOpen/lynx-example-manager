package main

import (
	"fmt"
	"github.com/IoTOpen/go-lynx"
	"github.com/spf13/viper"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var client *lynx.Client

func configure() {
	viper.SetConfigName("lynx-manager")
	viper.SetConfigType("yml")
	viper.AddConfigPath(".")

	viper.SetDefault("api.base", "https://domain.tld")
	viper.SetDefault("api.key", "secret")

	viper.SetDefault("mqtt.broker", "tcp://domain.tld:port")
	viper.SetDefault("mqtt.client_id", "manager-example")
	viper.SetDefault("mqtt.connection_log", true)

	viper.SetDefault("lynx.installation_id", 1)

	if err := viper.ReadInConfig(); err != nil {
		_ = viper.SafeWriteConfig()
		log.Fatalln("Config:", err)
	}
}

func lynxClientSetup() {
	opts := lynx.NewMqttOptions(viper.Sub("mqtt"), nil, nil)
	client = lynx.NewClient(&lynx.Options{
		Authenticator: lynx.AuthApiKey{
			Key: viper.GetString("api.key"),
		},
		APIBase:     viper.GetString("api.base"),
		MqttOptions: opts,
	})

	for err := client.MQTTConnect(); err != nil; err = client.MQTTConnect() {
		log.Println("MQTT error connecting:", err)
		time.Sleep(time.Second * 5)
	}
}

func generateTemperature(t int64) float64 {
	return float64(math.Sin(float64(t)/1000) * 20)
}

func getOrCreateDevice(installationID int64) *lynx.Device {
	devices, err := client.GetDevices(installationID, map[string]string{
		"example.type": "go-lynx",
	})
	if err != nil {
		log.Fatalln("Failed to get device:", err)
	}
	if len(devices) == 0 {
		dev := &lynx.Device{
			Type:           "virtual",
			InstallationID: installationID,
			Meta: lynx.Meta{
				"name":         "Go-lynx-example",
				"example.type": "go-lynx",
			},
		}
		dev, err = client.CreateDevice(dev)
		if err != nil {
			log.Fatalln("Failed to create device:", err)
		}
		_, err = client.CreateFunction(&lynx.Function{
			Type:           "temperature",
			InstallationID: installationID,
			Meta: lynx.Meta{
				"name":         fmt.Sprintf("%d - temperature", dev.ID),
				"device_id":    fmt.Sprintf("%d", dev.ID),
				"topic_read":   "obj/example/temperature",
				"example.type": "go-lynx",
			},
		})
		if err != nil {
			log.Fatalln("Failed to create function:", err)
		}
		return dev
	}
	return devices[0]
}

func main() {
	configure()
	lynxClientSetup()
	installationID := viper.GetInt64("lynx.installation_id")
	installation, err := client.GetInstallation(installationID)
	if err != nil {
		log.Fatalln("Failed to get installation:", err)
	}
	dev := getOrCreateDevice(installationID)
	functions, err := client.GetFunctions(installationID, map[string]string{
		"example.type": "go-lynx",
		"device_id":    fmt.Sprintf("%d", dev.ID),
	})
	if err != nil {
		log.Fatalln("Failed to get functions:", err)
	}
	ticker := time.NewTicker(time.Minute)
	done := make(chan bool)
	sigc := make(chan os.Signal)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				t := time.Now().Unix()
				publish(functions[0], installation.ClientID, &lynx.Message{
					Value:     generateTemperature(t),
					Timestamp: t,
				})
			}
		}
	}()
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	<-sigc
	ticker.Stop()
	done <- true
}

func publish(fn *lynx.Function, clientID int64, message *lynx.Message) {
	topic := fmt.Sprintf("%d/%s", clientID, fn.Meta["topic_read"])
	if err := client.Publish(topic, message, 0); err != nil {
		log.Println("Failed to publish on MQTT:", err)
	}
}
