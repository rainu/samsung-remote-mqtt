package main

import (
	"flag"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/rainu/samsung-remote-mqtt/internal/mqtt"
	"go.uber.org/zap"
)

type applicationConfig struct {
	Broker       *string
	SubscribeQOS *int
	PublishQOS   *int
	Username     *string
	Password     *string
	ClientId     *string
	DeviceName   *string
	TopicPrefix  *string

	SamsungTvHost     *string
	SamsungRemoteName *string
	SamsungTokenPath  *string
}

var Config applicationConfig

func LoadConfig() {
	Config = applicationConfig{
		Broker:       flag.String("broker", "", "The broker URI. ex: tcp://127.0.0.1:1883"),
		SubscribeQOS: flag.Int("sub-qos", 0, "The Quality of Service for subscription 0,1,2 (default 0)"),
		PublishQOS:   flag.Int("pub-qos", 0, "The Quality of Service for publishing 0,1,2 (default 0)"),
		Username:     flag.String("user", "", "The User (optional)"),
		Password:     flag.String("password", "", "The password (optional)"),
		ClientId:     flag.String("client-id", "samsung-remote", "The ClientID (default samsung-remote)"),
		DeviceName:   flag.String("device-name", "SamsungRemote", "The name of this remote (default SamsungRemote)"),
		TopicPrefix:  flag.String("topic-prefix", "cmnd/samsung-remote", "The mqtt topic to listen for incoming commands (default cmnd/samsung-remote)"),

		SamsungTvHost:     flag.String("tv-host", "", "The samsung tv host address. ex: 192.168.1.123"),
		SamsungRemoteName: flag.String("tv-remote-name", "samung-remote-mqtt", "The name of the remote. This name will displayed at the TV. (optional)"),
		SamsungTokenPath:  flag.String("tv-remote-token-file", "./samsung-remote.token", "The path to the file were the token will be saved for reuse. (optional)"),
	}
	flag.Parse()

	if *Config.Broker == "" {
		zap.L().Fatal("Broker is missing!")
	}
	if *Config.SamsungTvHost == "" {
		zap.L().Fatal("TV-Host is missing!")
	}
	if *Config.SubscribeQOS != 0 && *Config.SubscribeQOS != 1 && *Config.SubscribeQOS != 2 {
		zap.L().Fatal("Invalid qos level!")
	}
	if *Config.PublishQOS != 0 && *Config.PublishQOS != 1 && *Config.PublishQOS != 2 {
		zap.L().Fatal("Invalid qos level!")
	}
	if *Config.TopicPrefix == "" {
		zap.L().Fatal("Invalid topic prefix!")
	}
}

func (c *applicationConfig) GetMQTTOpts(
	onConn MQTT.OnConnectHandler,
	onLost MQTT.ConnectionLostHandler) *MQTT.ClientOptions {

	opts := MQTT.NewClientOptions()

	opts.AddBroker(*c.Broker)
	if *c.Username != "" {
		opts.SetUsername(*c.Username)
	}
	if *c.Password != "" {
		opts.SetPassword(*c.Password)
	}
	if *c.ClientId != "" {
		opts.SetClientID(*c.ClientId)
	}

	opts.WillEnabled = true
	opts.WillQos = byte(*c.PublishQOS)
	opts.WillPayload = []byte(mqtt.StatusOffline)
	opts.WillTopic = fmt.Sprintf("%s/status", *c.TopicPrefix)

	opts.SetOnConnectHandler(onConn)
	opts.SetConnectionLostHandler(onLost)

	return opts
}
