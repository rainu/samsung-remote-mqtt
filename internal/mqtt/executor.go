package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/linde12/gowol"
	samsungRemoteHTTP "github.com/rainu/samsung-remote/http"
	samsungRemoteWS "github.com/rainu/samsung-remote/ws"
	"go.uber.org/zap"
	"strconv"
	"strings"
)

const (
	PayloadStatusRunning = "RUNNING"
	PayloadStatusStopped = "STOPPED"
)

type Executor struct {
	initialised  bool
	subscribeQOS byte
	publishQOS   byte
	topics       map[string]MQTT.MessageHandler

	ctx       context.Context
	ctxCancel context.CancelFunc

	MqttClient        MQTT.Client
	SamsungTvMac      string
	SamsungRemoteWS   samsungRemoteWS.SamsungRemote
	SamsungRemoteHttp samsungRemoteHTTP.SamsungRemote
}

func (e *Executor) Initialise(topicPrefix string, subscribeQOS, publishQOS byte) {
	e.ctx, e.ctxCancel = context.WithCancel(context.Background())
	e.subscribeQOS = subscribeQOS
	e.publishQOS = publishQOS

	e.topics = map[string]MQTT.MessageHandler{
		fmt.Sprintf("%s/wake-up", topicPrefix):       e.stateWrapper(e.handleWakeUp),
		fmt.Sprintf("%s/info", topicPrefix):          e.stateWrapper(e.handleInfo),
		fmt.Sprintf("%s/send-key", topicPrefix):      e.stateWrapper(e.handleSendKey),
		fmt.Sprintf("%s/send-text", topicPrefix):     e.stateWrapper(e.handleSendText),
		fmt.Sprintf("%s/move", topicPrefix):          e.stateWrapper(e.handleMove),
		fmt.Sprintf("%s/left-click", topicPrefix):    e.stateWrapper(e.handleLeftClick),
		fmt.Sprintf("%s/right-click", topicPrefix):   e.stateWrapper(e.handleRightClick),
		fmt.Sprintf("%s/browser", topicPrefix):       e.stateWrapper(e.handleBrowser),
		fmt.Sprintf("%s/app/start", topicPrefix):     e.stateWrapper(e.handleAppStart),
		fmt.Sprintf("%s/app/start/+", topicPrefix):   e.stateWrapper(e.handleAppStartByName),
		fmt.Sprintf("%s/app/stop", topicPrefix):      e.stateWrapper(e.handleAppStop),
		fmt.Sprintf("%s/app/status", topicPrefix):    e.stateWrapper(e.handleAppStatus),
		fmt.Sprintf("%s/app/installed", topicPrefix): e.stateWrapper(e.handleAppInstalled),
	}

	for topic, handler := range e.topics {
		e.MqttClient.Subscribe(topic, subscribeQOS, handler)
	}

	e.initialised = true
}

func (e *Executor) IsInitialised() bool {
	return e.initialised
}

func (e *Executor) ReInitialise() {
	for topic, handler := range e.topics {
		e.MqttClient.Subscribe(topic, e.subscribeQOS, handler)
	}
}

func (e *Executor) handleWakeUp(client MQTT.Client, message MQTT.Message) {
	if e.SamsungTvMac == "" {
		e.sendAnswer(client, message, nil, errors.New("TV Mac-Address is not given"))
		return
	}

	packet, err := gowol.NewMagicPacket(e.SamsungTvMac)
	if err != nil {
		e.sendAnswer(client, message, nil, fmt.Errorf("failed to create magic packet: %w", err))
		return
	}

	err = packet.Send("255.255.255.255")
	if err != nil {
		e.sendAnswer(client, message, nil, fmt.Errorf("failed to send magic packet: %w", err))
		return
	}
	e.sendAnswer(client, message, nil, nil)
}

func (e *Executor) handleInfo(client MQTT.Client, message MQTT.Message) {
	info, err := e.SamsungRemoteHttp.GetInformationWithContext(e.ctx)
	e.sendAnswer(client, message, []byte(info), err)
}

func (e *Executor) handleSendKey(client MQTT.Client, message MQTT.Message) {
	err := e.SamsungRemoteWS.SendKey(string(message.Payload()))
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleSendText(client MQTT.Client, message MQTT.Message) {
	err := e.SamsungRemoteWS.SendText(message.Payload())
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleMove(client MQTT.Client, message MQTT.Message) {
	split := strings.Split(string(message.Payload()), " ")
	if len(split) != 2 {
		e.sendAnswer(client, message, nil, errors.New("invalid payload"))
		return
	}
	x, err := strconv.Atoi(split[0])
	if err != nil {
		e.sendAnswer(client, message, nil, fmt.Errorf("invalid x position: %w", err))
		return
	}
	y, err := strconv.Atoi(split[1])
	if err != nil {
		e.sendAnswer(client, message, nil, fmt.Errorf("invalid y position: %w", err))
		return
	}

	err = e.SamsungRemoteWS.Move(x, y)
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleLeftClick(client MQTT.Client, message MQTT.Message) {
	err := e.SamsungRemoteWS.LeftClick()
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleRightClick(client MQTT.Client, message MQTT.Message) {
	err := e.SamsungRemoteWS.RightClick()
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleBrowser(client MQTT.Client, message MQTT.Message) {
	err := e.SamsungRemoteWS.OpenBrowser(string(message.Payload()))
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleAppStart(client MQTT.Client, message MQTT.Message) {
	err := e.SamsungRemoteWS.StartAppWithContext(string(message.Payload()), e.ctx)
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleAppStartByName(client MQTT.Client, message MQTT.Message) {
	topicSegments := strings.Split(message.Topic(), "/")
	appName := topicSegments[len(topicSegments)-1]

	err := e.SamsungRemoteHttp.StartAppWithContext(appName, message.Payload(), e.ctx)
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleAppStop(client MQTT.Client, message MQTT.Message) {
	err := e.SamsungRemoteWS.CloseAppWithContext(string(message.Payload()), e.ctx)
	e.sendAnswer(client, message, nil, err)
}

func (e *Executor) handleAppStatus(client MQTT.Client, message MQTT.Message) {
	status, err := e.SamsungRemoteWS.GetAppStatusWithContext(string(message.Payload()), e.ctx)
	if err != nil {
		e.sendAnswer(client, message, nil, err)
	} else {
		answer, err := json.Marshal(status)
		if err != nil {
			zap.L().Warn("Error while marshal application status: %s", zap.Error(err))
		} else {
			e.sendAnswer(client, message, answer, err)
		}
	}
}

func (e *Executor) handleAppInstalled(client MQTT.Client, message MQTT.Message) {
	apps, err := e.SamsungRemoteWS.GetInstalledAppsWithContext(e.ctx)
	if err != nil {
		e.sendAnswer(client, message, nil, err)
	} else {
		answer, err := json.Marshal(apps)
		if err != nil {
			zap.L().Warn("Error while marshal application status: %s", zap.Error(err))
		} else {
			e.sendAnswer(client, message, answer, err)
		}
	}
}

func (e *Executor) stateWrapper(delegate MQTT.MessageHandler) MQTT.MessageHandler {
	return func(client MQTT.Client, message MQTT.Message) {
		e.sendStatus(client, message, []byte(PayloadStatusRunning))
		delegate(client, message)
		e.sendStatus(client, message, []byte(PayloadStatusStopped))
	}
}

func (e *Executor) sendStatus(client MQTT.Client, message MQTT.Message, state []byte) {
	client.Publish(fmt.Sprintf("%s/state", message.Topic()), e.publishQOS, false, state)
}

func (e *Executor) sendAnswer(client MQTT.Client, message MQTT.Message, answer []byte, err error) {
	if err != nil {
		answer = []byte(fmt.Sprintf(`ERROR %s`, err.Error()))
	} else if answer == nil {
		answer = []byte(`OK`)
	}

	client.Publish(fmt.Sprintf("%s/result", message.Topic()), e.publishQOS, false, answer)
}

func (e *Executor) Close() error {
	e.ctxCancel()

	//unsubscribe to all mqtt-topics
	for topic := range e.topics {
		e.MqttClient.Unsubscribe(topic)
	}

	return nil
}
