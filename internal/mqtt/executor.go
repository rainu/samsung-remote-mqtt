package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	samsungRemoteHTTP "github.com/rainu/samsung-remote/http"
	samsungRemoteWS "github.com/rainu/samsung-remote/ws"
	"go.uber.org/zap"
	"strconv"
	"strings"
)

type Executor struct {
	initialised  bool
	subscribeQOS byte
	publishQOS   byte
	topics       map[string]MQTT.MessageHandler

	ctx       context.Context
	ctxCancel context.CancelFunc

	MqttClient        MQTT.Client
	SamsungRemoteWS   samsungRemoteWS.SamsungRemote
	SamsungRemoteHttp samsungRemoteHTTP.SamsungRemote
}

func (e *Executor) Initialise(topicPrefix string, subscribeQOS, publishQOS byte) {
	e.ctx, e.ctxCancel = context.WithCancel(context.Background())
	e.subscribeQOS = subscribeQOS
	e.publishQOS = publishQOS

	e.topics = map[string]MQTT.MessageHandler{
		fmt.Sprintf("%s/info", topicPrefix):          e.handleInfo,
		fmt.Sprintf("%s/send-key", topicPrefix):      e.handleSendKey,
		fmt.Sprintf("%s/send-text", topicPrefix):     e.handleSendText,
		fmt.Sprintf("%s/move", topicPrefix):          e.handleMove,
		fmt.Sprintf("%s/left-click", topicPrefix):    e.handleLeftClick,
		fmt.Sprintf("%s/right-click", topicPrefix):   e.handleRightClick,
		fmt.Sprintf("%s/browser", topicPrefix):       e.handleBrowser,
		fmt.Sprintf("%s/app/start", topicPrefix):     e.handleAppStart,
		fmt.Sprintf("%s/app/start/+", topicPrefix):   e.handleAppStartByName,
		fmt.Sprintf("%s/app/stop", topicPrefix):      e.handleAppStop,
		fmt.Sprintf("%s/app/status", topicPrefix):    e.handleAppStatus,
		fmt.Sprintf("%s/app/installed", topicPrefix): e.handleAppInstalled,
	}

	for topic, handler := range e.topics {
		e.MqttClient.Subscribe(topic, subscribeQOS, handler)
	}

	e.initialised = true
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
