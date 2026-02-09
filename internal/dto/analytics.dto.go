package dto

type AnalyticsOverviewResponse struct {
	TotalPlays       int                `json:"total_plays"`
	TotalFavorites   int                `json:"total_favorites"`
	TotalRevenue     float64            `json:"total_revenue"`
	TotalDownloads   int                `json:"total_downloads"`
	PlaysByDay       []DailyStat        `json:"plays_by_day"`
	TopSpecs         []TopSpecStat      `json:"top_specs"`
	RevenueByLicense map[string]float64 `json:"revenue_by_license"`
}

type DailyStat struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type TopSpecStat struct {
	SpecID string `json:"spec_id"`
	Title  string `json:"title"`
	Plays  int    `json:"plays"`
}
