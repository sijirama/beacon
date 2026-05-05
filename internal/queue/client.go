package queue

import (
	"github.com/hibiken/asynq"
)

const (
	TaskSendEmail = "email:send"
	QueueDefault  = "default"
)

type SendEmailPayload struct {
	EventID uint `json:"event_id"`
}

func RedisOpt(redisURL string) (asynq.RedisConnOpt, error) {
	return asynq.ParseRedisURI(redisURL)
}

func NewClient(redisURL string) (*asynq.Client, error) {
	opt, err := RedisOpt(redisURL)
	if err != nil {
		return nil, err
	}
	return asynq.NewClient(opt), nil
}

func NewInspector(redisURL string) (*asynq.Inspector, error) {
	opt, err := RedisOpt(redisURL)
	if err != nil {
		return nil, err
	}
	return asynq.NewInspector(opt), nil
}
