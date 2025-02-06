package models

import (
	"time"
)

type Order struct {
	Number     int
	Login      string
	Status     string
	Accrual    *float64
	UploadedAt time.Time
}

type OrderResponse struct {
	Number     int      `json:"number"`
	Login      string   `json:"login,omitempty"`
	Status     string   `json:"status"`
	Accrual    *float64 `json:"accrual,omitempty"`
	UploadedAt string   `json:"uploaded_at"`
}
