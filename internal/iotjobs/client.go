package iotjobs

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"gaie/internal/config"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"os"
	"time"
)

type IoTClient struct {
	cfg        *config.Environment
	MqttClient mqtt.Client
}

func NewIoTClient(cfg *config.Environment) (*IoTClient, error) {
	log.Printf("Using certificates:\nRoot CA: %s\nDevice Cert: %s\nPrivate Key: %s",
		cfg.RootCAPath,
		cfg.CertPath,
		cfg.KeyPath,
	)
	certpool := x509.NewCertPool()
	pemCert, err := os.ReadFile(cfg.RootCAPath) // Changed from CertPath to RootCAPath
	if err != nil {
		return nil, fmt.Errorf("failed to read root CA: %w", err)
	}
	if !certpool.AppendCertsFromPEM(pemCert) {
		return nil, fmt.Errorf("failed to parse root CA")
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		RootCAs:      certpool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("ssl://%s:8883", cfg.IoTEndpoint))
	opts.SetClientID(cfg.ThingName)
	opts.SetTLSConfig(tlsConfig)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetAutoReconnect(true)

	// Create closure to capture cfg
	thingName := cfg.ThingName
	opts.OnConnect = func(client mqtt.Client) {
		log.Println("Connected to AWS IoT Core")
		topic := fmt.Sprintf("$aws/things/%s/jobs/#", thingName)
		log.Printf("Subscribing to %s", topic)
		if token := client.Subscribe(topic, 1, messageHandler); token.Wait() && token.Error() != nil {
			log.Printf("Error subscribing to topic: %v", token.Error())
		}
	}

	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("Connection lost: %v", err)
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect: %w", token.Error())
	}

	return &IoTClient{
		cfg:        cfg,
		MqttClient: client,
	}, nil
}

func messageHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received message on topic: %s", msg.Topic())
	log.Printf("Message ID: %d", msg.MessageID())
	log.Printf("Payload: %s", string(msg.Payload()))

	// Add message acknowledgement for QoS 1
	msg.Ack()
}

func (c *IoTClient) Close() {
	if c.MqttClient != nil && c.MqttClient.IsConnected() {
		c.MqttClient.Disconnect(250)
		log.Println("Disconnected from MQTT broker")
	}
}
