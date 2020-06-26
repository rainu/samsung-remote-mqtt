package internal

import (
	"context"
	"errors"
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"net"
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

func (s *StatusWorker) Initialise(topicPrefix, tvHost string) {

	//generate a context so that we can cancel it later (see Close func)
	var ctx context.Context
	ctx, s.cancelFunc = context.WithCancel(context.Background())

	s.waitGroup.Add(1)
	go s.runApplicationStatus(fmt.Sprintf("%s/status", topicPrefix), ctx)

	s.waitGroup.Add(1)
	go s.runTvStatus(fmt.Sprintf("%s/tv/status", topicPrefix), tvHost, ctx)
}

func (s *StatusWorker) runApplicationStatus(statusTopic string, ctx context.Context) {
	defer s.waitGroup.Done()
	defer func() {
		token := s.sendStatus(statusTopic, false)

		//we should wait for the last state publish (graceful shutdown dont trigger the mqtt-last-will!)
		token.Wait()
	}()

	//first one
	s.sendStatus(statusTopic, true)

	//wait until shutdown
	<-ctx.Done()
}

func (s *StatusWorker) runTvStatus(statusTopic, tvHost string, ctx context.Context) {
	defer s.waitGroup.Done()

	//first one
	s.sendStatus(statusTopic, isPortOpen(tvHost))

	ticker := time.Tick(10 * time.Second)
	for {
		//wait until next tick or shutdown
		select {
		case <-ticker:
			s.sendStatus(statusTopic, isPortOpen(tvHost))
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

func (s *StatusWorker) sendStatus(statusTopic string, status bool) MQTT.Token {
	payload := StatusOnline
	if !status {
		payload = StatusOffline
	}

	return s.MqttClient.Publish(statusTopic, byte(1), true, payload)
}

func isPortOpen(tvHost string) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:8002", tvHost), 1*time.Second)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	return err == nil
}
