package main

import (
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/rainu/samsung-remote-mqtt/internal"
	"github.com/rainu/samsung-remote-mqtt/internal/mqtt"
	"github.com/rainu/samsung-remote-mqtt/internal/mqtt/hassio"
	samsungRemoteHTTP "github.com/rainu/samsung-remote/http"
	samsungRemoteWS "github.com/rainu/samsung-remote/ws"
	"go.uber.org/zap"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var executor mqtt.Executor
var statusWorker internal.StatusWorker
var wsRemote samsungRemoteWS.SamsungRemote
var httpRemote samsungRemoteHTTP.SamsungRemote
var remoteConnectionLostHandler samsungRemoteWS.ConnectionLostHandler
var signals = make(chan os.Signal, 1)

func main() {
	defer zap.L().Sync()

	LoadConfig()
	//reacting to signals (interrupt)
	defer close(signals)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	//remote initialisation
	httpRemote = samsungRemoteHTTP.NewRemote(samsungRemoteHTTP.SamsungRemoteConfig{
		BaseUrl: fmt.Sprintf("https://%s:8002", *Config.SamsungTvHost),
	})
	executor.SamsungRemoteHttp = httpRemote
	wsRemote = samsungRemoteWS.NewRemote(samsungRemoteWS.SamsungRemoteConfig{
		BaseUrl: fmt.Sprintf("wss://%s:8002", *Config.SamsungTvHost),
		Name:    *Config.SamsungRemoteName,
		Token:   readToken(),
	})
	executor.SamsungRemoteWS = wsRemote
	remoteConnectionLostHandler = handleRemoteConnectionLost

	//mqtt initialisation
	client := MQTT.NewClient(Config.GetMQTTOpts(
		handleOnConnection,
		handleOnConnectionLost,
	))
	executor.MqttClient = client
	statusWorker.MqttClient = client

	connectToMqttBroker(client)

	executor.Initialise(*Config.TopicPrefix, byte(*Config.SubscribeQOS), byte(*Config.PublishQOS))
	statusWorker.Initialise(*Config.TopicPrefix, *Config.SamsungTvHost)

	token := connectToWebsocket()
	if err := writeToken(token); err != nil {
		zap.L().Warn("Unable to write token to file: %s", zap.Error(err))
	}

	//if hassio is enabled -> publish the hassio mqtt-discovery configs
	if *Config.HomeassistantEnable {
		information, err := httpRemote.GetInformation()
		if err != nil {
			zap.L().Warn("Could not get device information: %s", zap.Error(err))
		}

		haClient := hassio.Client{
			DeviceInfo:        information,
			RemoteName:        *Config.SamsungRemoteName,
			TopicPrefix:       *Config.TopicPrefix,
			HassioTopicPrefix: *Config.HomeassistantTopic,
			MqttClient:        client,
		}
		haClient.PublishDiscoveryConfig()
	}

	// wait for interrupt
	<-signals

	shutdown(client)
}

func connectToWebsocket() string {
	tokenChan := make(chan string, 1)

	go func() {
		defer close(tokenChan)

		for {
			token, err := wsRemote.Connect(remoteConnectionLostHandler)
			if err != nil {
				zap.L().Error("Error while connecting to samsung tv: %s", zap.Error(err))
				time.Sleep(1 * time.Second)
			} else {
				tokenChan <- token
				return
			}
		}
	}()

	select {
	case token := <-tokenChan:
		return token
	case <-signals:
		zap.L().Info("Shutting down...")
		os.Exit(0)
		return ""
	}
}

func connectToMqttBroker(client MQTT.Client) {
	conChan := make(chan interface{}, 1)

	go func() {
		defer close(conChan)
		for {
			if token := client.Connect(); token.Wait() && token.Error() != nil {
				zap.L().Error("Error while connecting to mqtt broker: %s", zap.Error(token.Error()))
				time.Sleep(1 * time.Second)
			} else {
				conChan <- true
				return
			}
		}
	}()

	select {
	case <-conChan:
		return
	case <-signals:
		zap.L().Info("Shutting down...")
		os.Exit(0)
		return
	}
}

var handleOnConnection = func(client MQTT.Client) {
	if !executor.IsInitialised() {
		return
	}

	zap.L().Info("Reinitialise...")
	executor.ReInitialise()
}

var handleOnConnectionLost = func(client MQTT.Client, err error) {
	zap.L().Warn("Connection lost to broker.", zap.Error(err))
}

func handleRemoteConnectionLost(err error) {
	zap.L().Debug("Connection lost to samsung remote. Try to reconnect.", zap.Error(err))
	wsRemote.Close()
	token := connectToWebsocket()
	if err := writeToken(token); err != nil {
		zap.L().Warn("Unable to write token to file: %s", zap.Error(err))
	}
}

func readToken() string {
	token, err := ioutil.ReadFile(*Config.SamsungTokenPath)
	if err != nil {
		zap.L().Warn("Unable to read token from file: %s", zap.Error(err))
		return ""
	}
	return string(token)
}

func writeToken(token string) error {
	return ioutil.WriteFile(*Config.SamsungTokenPath, []byte(token), 0644)
}

func shutdown(client MQTT.Client) {
	zap.L().Info("Shutting down...")

	type closable interface {
		Close() error
	}
	closeables := []closable{wsRemote, httpRemote, &executor, &statusWorker}

	//most operating systems wait a maximum of 30 seconds

	wg := sync.WaitGroup{}
	wg.Add(len(closeables))

	for _, c := range closeables {
		go func(c closable) {
			defer wg.Done()

			if err := c.Close(); err != nil {
				zap.L().Error("Timeout while waiting for graceful shutdown!", zap.Error(err))
			}
		}(c)
	}
	wg.Wait()

	//we have to disconnect at last because one closeable unsubscribe all topics
	client.Disconnect(10 * 1000) //wait 10sek at most
}
