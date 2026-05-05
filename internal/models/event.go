package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type Level string

const (
	LevelInfo   Level = "info"
	LevelWarn   Level = "warn"
	LevelDanger Level = "danger"
)

func NormalizeLevel(s string) Level {
	switch s {
	case "warn", "warning":
		return LevelWarn
	case "danger", "critical", "error":
		return LevelDanger
	default:
		return LevelInfo
	}
}

type Metadata map[string]any

func (m Metadata) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func (m *Metadata) Scan(src any) error {
	if src == nil {
		*m = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return errors.New("metadata: unsupported scan type")
	}
	if len(b) == 0 {
		*m = nil
		return nil
	}
	return json.Unmarshal(b, m)
}

type Event struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	TokenID *uint  `gorm:"index" json:"token_id,omitempty"`
	Token   *Token `gorm:"foreignKey:TokenID" json:"-"`

	Source  string `json:"source"`
	Event   string `json:"event"`
	Title   string `gorm:"not null" json:"title"`
	Message string `json:"message"`

	Level     Level     `gorm:"default:info;index" json:"level"`
	Channel   string    `gorm:"default:email;index" json:"channel"`
	IP        string    `json:"ip"`
	Metadata  Metadata  `gorm:"type:text" json:"metadata,omitempty"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
