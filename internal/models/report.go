package models

// DTO структуры для отчетов.

type FunnelRow struct {
	Status string `db:"status" json:"status"`
	Count  int64  `db:"count" json:"count"`
}

type RevenueRow struct {
	Period      string  `db:"period" json:"period"`
	TotalAmount float64 `db:"total_amount" json:"total_amount"`
	Currency    string  `db:"currency" json:"currency"`
}

type TopClientRow struct {
	ClientID    int     `db:"client_id" json:"client_id"`
	ClientName  string  `db:"client_name" json:"client_name"`
	TotalAmount float64 `db:"total_amount" json:"total_amount"`
	Currency    string  `db:"currency" json:"currency"`
}

type LeadSummaryRow struct {
	Status string `db:"status" json:"status"`
	Source string `db:"source" json:"source"`
	Count  int64  `db:"count" json:"count"`
}
