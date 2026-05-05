package queue

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

func EnqueueEmail(client *asynq.Client, eventID uint, retries int) error {
	payload, err := json.Marshal(SendEmailPayload{EventID: eventID})
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskSendEmail, payload)
	_, err = client.Enqueue(task,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(retries),
	)
	return err
}
