package models

import "time"

type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySent    DeliveryStatus = "sent"
	DeliveryFailed  DeliveryStatus = "failed"
)

type Delivery struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	EventID       uint           `gorm:"index;not null" json:"event_id"`
	Event         *Event         `gorm:"foreignKey:EventID" json:"-"`
	Channel       string         `gorm:"index" json:"channel"`
	Status        DeliveryStatus `gorm:"index" json:"status"`
	Provider      string         `json:"provider"`
	ProviderMsgID string         `json:"provider_msg_id"`
	Error         string         `json:"error,omitempty"`
	Attempts      int            `json:"attempts"`
	CreatedAt     time.Time      `json:"created_at"`
	DeliveredAt   *time.Time     `json:"delivered_at,omitempty"`
}
