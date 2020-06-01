package hassio

import (
	"encoding/json"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/rainu/samsung-remote-mqtt/internal"
	"github.com/rainu/samsung-remote-mqtt/internal/mqtt"
	"go.uber.org/zap"
	"strings"
)

type generalConfig struct {
	Name                string `json:"name"`
	AvailabilityTopic   string `json:"avty_t,omitempty"`
	PayloadAvailable    string `json:"pl_avail,omitempty"`
	PayloadNotAvailable string `json:"pl_not_avail,omitempty"`
	UniqueId            string `json:"uniq_id"`
	Icon                string `json:"ic,omitempty"`
	Device              device `json:"dev,omitempty"`
}

type sensorConfig struct {
	generalConfig

	StateTopic string `json:"stat_t"`
}

type triggerConfig struct {
	generalConfig

	CommandTopic string `json:"cmd_t"`
	StateTopic   string `json:"stat_t"`
	PayloadStart string `json:"pl_on"`
	PayloadStop  string `json:"pl_off"`
	StateRunning string `json:"stat_on"`
	StateStopped string `json:"stat_off"`
}

type device struct {
	Name         string   `json:"name,omitempty"`
	Ids          []string `json:"ids"`
	Model        string   `json:"mdl,omitempty"`
	Manufacturer string   `json:"mf,omitempty"`
	Version      string   `json:"sw,omitempty"`
}

type AvailabilityConfig struct {
	Topic               string
	AvailablePayload    string
	NotAvailablePayload string
}

type Client struct {
	parsedDeviceInfo deviceInfo
	DeviceInfo       string
	RemoteName       string

	HassioTopicPrefix string
	TopicPrefix       string
	MqttClient        MQTT.Client
}

type deviceInfo struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Device  struct {
		OS              string `json:"OS"`
		FirmwareVersion string `json:"firmwareVersion"`
		Model           string `json:"model"`
		ModelName       string `json:"modelName"`
		Name            string `json:"name"`
		Resolution      string `json:"resolution"`
		SSID            string `json:"ssid"`
		WifiMac         string `json:"wifiMac"`
	} `json:"device"`
}

func (c *Client) PublishDiscoveryConfig() {
	zap.L().Info("Initialise homeassistant config.")

	//try to parse raw device information
	if err := json.Unmarshal([]byte(c.DeviceInfo), &c.parsedDeviceInfo); err != nil {
		zap.L().Debug("Could not parse raw device info: %s", zap.Error(err))
	}

	targetTopic := fmt.Sprintf("%ssensor/%s_status/config", c.HassioTopicPrefix, c.deviceId())
	payload := c.generatePayloadForApplicationStatus()
	c.MqttClient.Publish(targetTopic, byte(0), false, payload)

	targetTopic = fmt.Sprintf("%ssensor/%s_tv_status/config", c.HassioTopicPrefix, c.deviceId())
	payload = c.generatePayloadForTvStatus()
	c.MqttClient.Publish(targetTopic, byte(0), false, payload)

	for topic, payload := range c.generatePayloadsForTvWakeUp() {
		c.MqttClient.Publish(topic, byte(0), false, payload)
	}
	for keyName, description := range SamsungRemoteKeys {
		for topic, payload := range c.generatePayloadsForSendKey(keyName, description) {
			c.MqttClient.Publish(topic, byte(0), false, payload)
		}
	}
}

func (c *Client) generatePayloadForApplicationStatus() []byte {
	conf := sensorConfig{
		generalConfig: generalConfig{
			Name:                "Status",
			PayloadAvailable:    internal.StatusOnline,
			PayloadNotAvailable: internal.StatusOffline,
			UniqueId:            fmt.Sprintf("%s_status", c.deviceId()),
			Device:              c.buildDevice(),
		},
		StateTopic: fmt.Sprintf("%s/status", c.TopicPrefix),
	}

	payload, err := json.Marshal(conf)
	if err != nil {
		//the "marshalling" is relatively safe - it should never appear at runtime
		panic(err)
	}
	return payload
}

func (c *Client) generatePayloadForTvStatus() []byte {
	conf := sensorConfig{
		generalConfig: generalConfig{
			Name:                "TV Status",
			PayloadAvailable:    internal.StatusOnline,
			PayloadNotAvailable: internal.StatusOffline,
			UniqueId:            fmt.Sprintf("%s_tv_status", c.deviceId()),
			Device:              c.buildDevice(),
		},
		StateTopic: fmt.Sprintf("%s/status", c.TopicPrefix),
	}

	payload, err := json.Marshal(conf)
	if err != nil {
		//the "marshalling" is relatively safe - it should never appear at runtime
		panic(err)
	}
	return payload
}

func (c *Client) generatePayloadsForTvWakeUp() map[string][]byte {
	payloads := map[string][]byte{}
	var conf interface{}

	conf = triggerConfig{
		generalConfig: generalConfig{
			Name:                "TV WakeUp",
			Icon:                "mdi:sleep-off",
			UniqueId:            fmt.Sprintf("%s_tv_wake_up", c.deviceId()),
			Device:              c.buildDevice(),
			AvailabilityTopic:   fmt.Sprintf("%s/tv/status", c.TopicPrefix),
			PayloadAvailable:    internal.StatusOffline, //wakeUp is available if the tv is offline
			PayloadNotAvailable: internal.StatusOnline,  //waleUp is unavailable if the tv is online
		},
		CommandTopic: fmt.Sprintf("%s/wake-up", c.TopicPrefix),
		StateTopic:   fmt.Sprintf("%s/wake-up/state", c.TopicPrefix),
		StateRunning: mqtt.PayloadStatusRunning,
		StateStopped: mqtt.PayloadStatusStopped,
	}

	payload, err := json.Marshal(conf)
	if err != nil {
		//the "marshalling" is relatively safe - it should never appear at runtime
		panic(err)
	}
	topic := fmt.Sprintf("%sswitch/%s/wake_up/config", c.HassioTopicPrefix, c.deviceId())
	payloads[topic] = payload

	conf = sensorConfig{
		generalConfig: generalConfig{
			Name:                fmt.Sprintf("TV WakeUp - State"),
			UniqueId:            fmt.Sprintf("%s_wake_up_state", c.deviceId()),
			Device:              c.buildDevice(),
			AvailabilityTopic:   fmt.Sprintf("%s/tv/status", c.TopicPrefix),
			PayloadAvailable:    internal.StatusOnline,
			PayloadNotAvailable: internal.StatusOffline,
		},
		StateTopic: fmt.Sprintf("%s/wake-up/state", c.TopicPrefix),
	}

	payload, err = json.Marshal(conf)
	if err != nil {
		//the "marshalling" is relatively safe - it should never appear at runtime
		panic(err)
	}
	topic = fmt.Sprintf("%ssensor/%s_wake_up/state/config", c.HassioTopicPrefix, c.deviceId())
	payloads[topic] = payload

	conf = sensorConfig{
		generalConfig: generalConfig{
			Name:                fmt.Sprintf("TV WakeUp - Result"),
			UniqueId:            fmt.Sprintf("%s_wake_up_result", c.deviceId()),
			Device:              c.buildDevice(),
			AvailabilityTopic:   fmt.Sprintf("%s/tv/status", c.TopicPrefix),
			PayloadAvailable:    internal.StatusOnline,
			PayloadNotAvailable: internal.StatusOffline,
		},
		StateTopic: fmt.Sprintf("%s/wake-up/result", c.TopicPrefix),
	}

	payload, err = json.Marshal(conf)
	if err != nil {
		//the "marshalling" is relatively safe - it should never appear at runtime
		panic(err)
	}
	topic = fmt.Sprintf("%ssensor/%s_wake_up/result/config", c.HassioTopicPrefix, c.deviceId())
	payloads[topic] = payload

	return payloads
}

func (c *Client) generatePayloadsForSendKey(key, description string) map[string][]byte {
	payloads := map[string][]byte{}
	var conf interface{}

	conf = triggerConfig{
		generalConfig: generalConfig{
			Name:                fmt.Sprintf("%s - Button", description),
			Icon:                "mdi:remote",
			UniqueId:            fmt.Sprintf("%s_%s", c.deviceId(), key),
			Device:              c.buildDevice(),
			AvailabilityTopic:   fmt.Sprintf("%s/tv/status", c.TopicPrefix),
			PayloadAvailable:    internal.StatusOnline,
			PayloadNotAvailable: internal.StatusOffline,
		},
		CommandTopic: fmt.Sprintf("%s/send-key", c.TopicPrefix),
		PayloadStart: key,
		StateTopic:   fmt.Sprintf("%s/send-key/state", c.TopicPrefix),
		StateRunning: mqtt.PayloadStatusRunning,
		StateStopped: mqtt.PayloadStatusStopped,
	}

	payload, err := json.Marshal(conf)
	if err != nil {
		//the "marshalling" is relatively safe - it should never appear at runtime
		panic(err)
	}
	topic := fmt.Sprintf("%sswitch/%s/%s/config", c.HassioTopicPrefix, c.deviceId(), key)
	payloads[topic] = payload

	conf = sensorConfig{
		generalConfig: generalConfig{
			Name:                fmt.Sprintf("%s - State", description),
			UniqueId:            fmt.Sprintf("%s_%s_state", c.deviceId(), key),
			Device:              c.buildDevice(),
			AvailabilityTopic:   fmt.Sprintf("%s/tv/status", c.TopicPrefix),
			PayloadAvailable:    internal.StatusOnline,
			PayloadNotAvailable: internal.StatusOffline,
		},
		StateTopic: fmt.Sprintf("%s/send-key/state", c.TopicPrefix),
	}

	payload, err = json.Marshal(conf)
	if err != nil {
		//the "marshalling" is relatively safe - it should never appear at runtime
		panic(err)
	}
	topic = fmt.Sprintf("%ssensor/%s_%s/state/config", c.HassioTopicPrefix, c.deviceId(), key)
	payloads[topic] = payload

	conf = sensorConfig{
		generalConfig: generalConfig{
			Name:                fmt.Sprintf("%s - Result", description),
			UniqueId:            fmt.Sprintf("%s_%s_result", c.deviceId(), key),
			Device:              c.buildDevice(),
			AvailabilityTopic:   fmt.Sprintf("%s/tv/status", c.TopicPrefix),
			PayloadAvailable:    internal.StatusOnline,
			PayloadNotAvailable: internal.StatusOffline,
		},
		StateTopic: fmt.Sprintf("%s/send-key/result", c.TopicPrefix),
	}

	payload, err = json.Marshal(conf)
	if err != nil {
		//the "marshalling" is relatively safe - it should never appear at runtime
		panic(err)
	}
	topic = fmt.Sprintf("%ssensor/%s_%s/result/config", c.HassioTopicPrefix, c.deviceId(), key)
	payloads[topic] = payload

	return payloads
}

func (c *Client) buildDevice() device {
	d := device{
		Name:         c.parsedDeviceInfo.Name,
		Ids:          []string{c.deviceId()},
		Manufacturer: "Samsung",
		Model:        c.parsedDeviceInfo.Device.ModelName,
		Version:      c.parsedDeviceInfo.Version,
	}

	if d.Name == "" {
		d.Name = c.RemoteName
	}

	return d
}

func (c *Client) deviceId() string {
	if c.parsedDeviceInfo.Device.WifiMac != "" {
		return strings.Replace(c.parsedDeviceInfo.Device.WifiMac, ":", "", -1)
	}
	if c.parsedDeviceInfo.Id != "" {
		return c.parsedDeviceInfo.Id
	}
	if c.parsedDeviceInfo.Name != "" {
		return c.parsedDeviceInfo.Name
	}

	return c.RemoteName
}
