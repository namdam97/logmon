package domain

// HealthStatus là tình trạng các thành phần pipeline (read model cho /status).
// Giá trị: "up" | "down" | "unknown".
type HealthStatus struct {
	Elasticsearch string
	Collector     string
	Kafka         string
}

// DataStreamStat là thống kê một data stream ES (read model cho /datastreams).
type DataStreamStat struct {
	Name           string
	SizeBytes      int64
	DocCount       int64
	BackingIndices int
}
