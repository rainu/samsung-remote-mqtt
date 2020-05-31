package main

import (
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/rainu/samsung-remote-mqtt/internal/mqtt"
	samsungRemoteHTTP "github.com/rainu/samsung-remote/http"
	samsungRemoteWS "github.com/rainu/samsung-remote/ws"
	"go.uber.org/zap"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var executor mqtt.Executor
var statusWorker mqtt.StatusWorker
var wsRemote samsungRemoteWS.SamsungRemote
var httpRemote samsungRemoteHTTP.SamsungRemote
var remoteConnectionLostHandler samsungRemoteWS.ConnectionLostHandler

func main() {
	defer zap.L().Sync()

	LoadConfig()
	//reacting to signals (interrupt)
	signals := make(chan os.Signal, 1)
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

	token, err := wsRemote.Connect(remoteConnectionLostHandler)
	if err != nil {
		zap.L().Fatal("Error while connecting to samsung tv: %s", zap.Error(err))
	}
	if err := writeToken(token); err != nil {
		zap.L().Warn("Unable to write token to file: %s", zap.Error(err))
	}

	//mqtt initialisation
	client := MQTT.NewClient(Config.GetMQTTOpts(
		handleOnConnection,
		handleOnConnectionLost,
	))
	executor.MqttClient = client

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		zap.L().Fatal("Error while connecting to mqtt broker: %s", zap.Error(token.Error()))
	}

	executor.Initialise(*Config.TopicPrefix, byte(*Config.SubscribeQOS), byte(*Config.PublishQOS))
	statusWorker.Initialise(*Config.TopicPrefix)

	// wait for interrupt
	<-signals

	shutdown(client)
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
	token, reconErr := wsRemote.Connect(remoteConnectionLostHandler)
	if reconErr != nil {
		zap.L().Fatal("Error while re-connecting to samsung tv: %s", zap.Error(reconErr))
	}
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
