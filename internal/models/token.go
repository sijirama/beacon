package models

import "time"

type Token struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	Token     string    `gorm:"uniqueIndex;not null" json:"token"`
	CreatedAt time.Time `json:"created_at"`
}
