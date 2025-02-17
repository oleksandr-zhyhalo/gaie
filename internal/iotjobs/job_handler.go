package iotjobs

import (
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type JobHandler struct {
	thingName  string
	mqttClient mqtt.Client
}

func NewJobHandler(thingName string, client mqtt.Client) *JobHandler {
	return &JobHandler{
		thingName:  thingName,
		mqttClient: client,
	}
}

func (h *JobHandler) HandleMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received message on topic: %s", msg.Topic())
	log.Printf("Payload: %s", string(msg.Payload()))

	switch {
	case strings.HasSuffix(msg.Topic(), "/jobs/notify"):
		h.handleJobNotification(msg)

	case strings.Contains(msg.Topic(), "/jobs/"+h.thingName+"/jobs/"):
		if strings.Contains(msg.Topic(), "/get/accepted") {
			h.handleJobDocument(msg)
		} else if strings.Contains(msg.Topic(), "/update/accepted") {
			log.Println("Job status update acknowledged by AWS IoT")
		} else {
			log.Printf("Unhandled job-related message: %s", msg.Topic())
		}

	default:
		log.Printf("Unknown message type on topic: %s", msg.Topic())
	}

	msg.Ack()
}

type JobNotification struct {
	Timestamp int64 `json:"timestamp"`
	Jobs      map[string][]struct {
		JobID           string `json:"jobId"`
		QueuedAt        int64  `json:"queuedAt"`
		LastUpdatedAt   int64  `json:"lastUpdatedAt"`
		ExecutionNumber int    `json:"executionNumber"`
		VersionNumber   int    `json:"versionNumber"`
	} `json:"jobs"`
}

func (h *JobHandler) handleJobNotification(msg mqtt.Message) {
	var notification JobNotification
	if err := json.Unmarshal(msg.Payload(), &notification); err != nil {
		log.Printf("Failed to parse job notification: %v", err)
		return
	}

	queuedJobs, exists := notification.Jobs["QUEUED"]
	if !exists || len(queuedJobs) == 0 {
		log.Println("No QUEUED jobs to process")
		return
	}

	for _, job := range queuedJobs {
		log.Printf("Processing QUEUED job: %s", job.JobID)
		h.publishJobStatus(job.JobID, "IN_PROGRESS", "Job started")
		h.requestJobDocument(job.JobID)
	}
}

func (h *JobHandler) requestJobDocument(jobID string) {
	topic := fmt.Sprintf("$aws/things/%s/jobs/%s/get", h.thingName, jobID)
	token := h.mqttClient.Publish(topic, 1, false, "{}")
	if token.Wait() && token.Error() != nil {
		log.Printf("Failed to request job document for %s: %v", jobID, token.Error())
	}
}

type JobDocumentResponse struct {
	Execution struct {
		JobID       string      `json:"jobId"`
		Status      string      `json:"status"`
		JobDocument JobDocument `json:"jobDocument"`
	} `json:"execution"`
}

type JobDocument struct {
	Version string `json:"version"`
	Steps   []struct {
		Action struct {
			Name  string `json:"name"`
			Type  string `json:"type"`
			Input struct {
				Command string `json:"command"`
			} `json:"input"`
			RunAsUser string `json:"runAsUser"`
		} `json:"action"`
	} `json:"steps"`
}

func (h *JobHandler) handleJobDocument(msg mqtt.Message) {
	var response struct {
		Execution struct {
			JobID       string `json:"jobId"`
			Status      string `json:"status"`
			JobDocument struct {
				Version string `json:"version"`
				Steps   []struct {
					Action struct {
						Name  string `json:"name"`
						Type  string `json:"type"`
						Input struct {
							Command string `json:"command"`
						} `json:"input"`
						RunAsUser string `json:"runAsUser"`
					} `json:"action"`
				} `json:"steps"`
			} `json:"jobDocument"`
		} `json:"execution"`
	}

	if err := json.Unmarshal(msg.Payload(), &response); err != nil {
		log.Printf("Failed to parse job document: %v", err)
		return
	}

	jobID := response.Execution.JobID
	log.Printf("Processing job document for: %s", jobID)

	success := true
	var err error

	for _, step := range response.Execution.JobDocument.Steps {
		if step.Action.Type != "runCommand" {
			log.Printf("Skipping action type: %s", step.Action.Type)
			continue
		}

		command := h.resolveParameter(step.Action.Input.Command)
		runAsUser := h.resolveParameter(step.Action.RunAsUser)

		output, execErr := h.executeCommand(command, runAsUser)
		if execErr != nil {
			log.Printf("Command failed: %v, Output: %s", execErr, output)
			success = false
			err = execErr
			break
		}
		log.Printf("Command executed successfully: %s", output)
	}

	status := "SUCCEEDED"
	details := "All steps executed"
	if !success {
		status = "FAILED"
		details = fmt.Sprintf("Error: %v", err)
	}

	h.publishJobStatus(jobID, status, details)
}

func (h *JobHandler) resolveParameter(input string) string {
	re := regexp.MustCompile(`\${aws:iot:parameter:([^}]+)}`)
	matches := re.FindAllStringSubmatch(input, -1)
	resolved := input
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		paramName := strings.ToUpper(match[1])
		resolved = strings.ReplaceAll(resolved, match[0], os.Getenv(paramName))
	}
	return resolved
}

func (h *JobHandler) executeCommand(command, runAsUser string) (string, error) {
	var cmd *exec.Cmd
	if runAsUser != "" {
		cmd = exec.Command("sudo", "-u", runAsUser, "sh", "-c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%v: %s", err, output)
	}
	return string(output), nil
}

func (h *JobHandler) publishJobStatus(jobId, status, details string) {
	topic := fmt.Sprintf("$aws/things/%s/jobs/%s/update", h.thingName, jobId)
	payload := fmt.Sprintf(`{"status":"%s","statusDetails":{"details":"%s"}}`, status, details)
	token := h.mqttClient.Publish(topic, 1, false, payload)
	go func() {
		if token.WaitTimeout(5*time.Second) && token.Error() != nil {
			log.Printf("Failed to publish status: %v", token.Error())
		}
	}()
}
