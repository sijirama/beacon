package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/sijirama/beacon/internal/models"
	"github.com/sijirama/beacon/internal/services/email"
)

type WorkerDeps struct {
	DB           *gorm.DB
	Notifier     email.Notifier
	Concurrency  int
}

func NewWorker(redisURL string, deps WorkerDeps) (*asynq.Server, *asynq.ServeMux, error) {
	opt, err := RedisOpt(redisURL)
	if err != nil {
		return nil, nil, err
	}
	conc := deps.Concurrency
	if conc <= 0 {
		conc = 5
	}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: conc,
		Queues:      map[string]int{QueueDefault: 1},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskSendEmail, makeEmailHandler(deps))
	return srv, mux, nil
}

func makeEmailHandler(deps WorkerDeps) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var p SendEmailPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			// Permanent failure: bad payload, don't retry.
			return fmt.Errorf("bad payload: %w: %w", err, asynq.SkipRetry)
		}

		var ev models.Event
		if err := deps.DB.First(&ev, p.EventID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("event %d not found: %w", p.EventID, asynq.SkipRetry)
			}
			return err
		}

		// Track attempt count: asynq exposes the current retry count via the context.
		retried, _ := asynq.GetRetryCount(ctx)
		attempts := retried + 1

		msgID, err := deps.Notifier.Send(ctx, ev)
		now := time.Now().UTC()
		if err != nil {
			// Persist a failed delivery row each attempt? No — just upsert latest.
			// Simplest: insert a row reflecting the current outcome.
			deliv := models.Delivery{
				EventID:  ev.ID,
				Channel:  deps.Notifier.Channel(),
				Status:   models.DeliveryFailed,
				Provider: providerName(deps.Notifier),
				Error:    err.Error(),
				Attempts: attempts,
			}
			deps.DB.Create(&deliv)
			return err
		}

		deliv := models.Delivery{
			EventID:       ev.ID,
			Channel:       deps.Notifier.Channel(),
			Status:        models.DeliverySent,
			Provider:      providerName(deps.Notifier),
			ProviderMsgID: msgID,
			Attempts:      attempts,
			DeliveredAt:   &now,
		}
		deps.DB.Create(&deliv)
		return nil
	}
}

func providerName(n email.Notifier) string {
	if n.Channel() == "email" {
		return "resend"
	}
	return n.Channel()
}
