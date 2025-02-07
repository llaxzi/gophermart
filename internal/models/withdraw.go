package models

import "time"

type Withdrawal struct {
	Order       string    `json:"order"`
	Login       string    `json:"login,omitempty"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at,omitempty"`
}

type WithdrawalResponse struct {
	Order       string  `json:"order"`
	Login       string  `json:"login,omitempty"`
	Sum         float64 `json:"sum"`
	ProcessedAt string  `json:"processed_at"`
}
