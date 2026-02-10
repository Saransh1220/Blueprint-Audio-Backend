package dto

type AnalyticsOverviewResponse struct {
	TotalPlays       int                `json:"total_plays"`
	TotalFavorites   int                `json:"total_favorites"`
	TotalRevenue     float64            `json:"total_revenue"`
	TotalDownloads   int                `json:"total_downloads"`
	PlaysByDay       []DailyStat        `json:"plays_by_day"`
	DownloadsByDay   []DailyStat        `json:"downloads_by_day"`
	RevenueByDay     []DailyRevenueStat `json:"revenue_by_day"`
	TopSpecs         []TopSpecStat      `json:"top_specs"`
	RevenueByLicense map[string]float64 `json:"revenue_by_license"`
}

type DailyStat struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type DailyRevenueStat struct {
	Date    string  `json:"date"`
	Revenue float64 `json:"revenue"`
}

type TopSpecStat struct {
	SpecID string `json:"spec_id"`
	Title  string `json:"title"`
	Plays  int    `json:"plays"`
}
