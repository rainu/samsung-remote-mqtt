package mqtt

import (
	"context"
	"errors"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"sync"
	"time"
)

const (
	StatusOnline  = "Online"
	StatusOffline = "Offline"
)

type StatusWorker struct {
	waitGroup  sync.WaitGroup
	cancelFunc context.CancelFunc

	MqttClient MQTT.Client
}

func (s *StatusWorker) Initialise(topicPrefix string) {

	//generate a context so that we can cancel it later (see Close func)
	var ctx context.Context
	ctx, s.cancelFunc = context.WithCancel(context.Background())

	s.waitGroup.Add(1)
	go s.runStatus(fmt.Sprintf("%s/status", topicPrefix), ctx)
}

func (s *StatusWorker) runStatus(statusTopic string, ctx context.Context) {
	defer s.waitGroup.Done()
	defer func() {
		token := s.MqttClient.Publish(statusTopic, byte(0), false, StatusOffline)

		//we should wait for the last state publish (graceful shutdown dont trigger the mqtt-last-will!)
		token.Wait()
	}()

	//first one
	s.MqttClient.Publish(statusTopic, byte(0), false, StatusOnline)

	ticker := time.Tick(30 * time.Second)
	for {
		//wait until next tick or shutdown
		select {
		case <-ticker:
			s.MqttClient.Publish(statusTopic, byte(0), false, StatusOnline)
		case <-ctx.Done():
			return
		}
	}
}

func (s *StatusWorker) Close() error {
	if s.cancelFunc != nil {
		//close the context to interrupt possible running commands
		s.cancelFunc()
	}

	wgChan := make(chan bool)
	go func() {
		s.waitGroup.Wait()
		wgChan <- true
	}()

	//wait for WaitGroup or Timeout
	select {
	case <-wgChan:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("timeout exceeded")
	}
}
