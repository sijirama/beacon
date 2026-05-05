package email

import (
	"context"
	"errors"

	"github.com/sijirama/beacon/internal/models"
)

// Notifier abstracts a delivery channel. Email is the V1 implementation;
// new channels (SMS, etc.) implement this interface.
type Notifier interface {
	Channel() string
	Send(ctx context.Context, ev models.Event) (providerMsgID string, err error)
}

type EmailNotifier struct {
	sender   *Sender
	renderer *TemplateRenderer
	to       []string
}

func NewEmailNotifier(sender *Sender, renderer *TemplateRenderer, to []string) *EmailNotifier {
	return &EmailNotifier{sender: sender, renderer: renderer, to: to}
}

func (n *EmailNotifier) Channel() string { return "email" }

func (n *EmailNotifier) Send(ctx context.Context, ev models.Event) (string, error) {
	if len(n.to) == 0 {
		return "", errors.New("no recipients configured")
	}
	subject, html, err := n.renderer.Render(ev)
	if err != nil {
		return "", err
	}
	var lastID string
	for _, addr := range n.to {
		id, err := n.sender.Send(ctx, addr, subject, html)
		if err != nil {
			return "", err
		}
		lastID = id
	}
	return lastID, nil
}
