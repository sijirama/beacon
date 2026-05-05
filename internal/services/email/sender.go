package email

import (
	"context"

	"github.com/resend/resend-go/v2"
)

type Sender struct {
	client *resend.Client
	from   string
}

func NewSender(apiKey, from string) *Sender {
	return &Sender{
		client: resend.NewClient(apiKey),
		from:   from,
	}
}

func (s *Sender) From() string { return s.from }

// Send delivers a single email and returns the provider message ID.
func (s *Sender) Send(ctx context.Context, to, subject, html string) (string, error) {
	resp, err := s.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{to},
		Subject: subject,
		Html:    html,
	})
	if err != nil {
		return "", err
	}
	return resp.Id, nil
}
