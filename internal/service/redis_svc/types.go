package redis_svc

const (
	RedisValueFormatRaw    = "raw"
	RedisValueFormatJSON   = "json"
	RedisValueFormatHex    = "hex"
	RedisValueFormatBase64 = "base64"
)

// RedisDatabase describes one logical Redis database from INFO keyspace.
type RedisDatabase struct {
	DB      int   `json:"db"`
	Keys    int64 `json:"keys"`
	Expires int64 `json:"expires"`
	AvgTTL  int64 `json:"avgTtl"`
}

// RedisScanRequest controls bounded key scanning.
type RedisScanRequest struct {
	AssetID int64  `json:"assetId"`
	DB      int    `json:"db"`
	Cursor  string `json:"cursor"`
	Match   string `json:"match"`
	Type    string `json:"type"`
	Count   int64  `json:"count"`
	Exact   bool   `json:"exact"`
}

type RedisScanResponse struct {
	Cursor  string   `json:"cursor"`
	Keys    []string `json:"keys"`
	HasMore bool     `json:"hasMore"`
}

type RedisKeyRequest struct {
	AssetID int64  `json:"assetId"`
	DB      int    `json:"db"`
	Key     string `json:"key"`
	Cursor  string `json:"cursor,omitempty"`
	Offset  int64  `json:"offset,omitempty"`
	Count   int64  `json:"count,omitempty"`
}

type RedisKeyDetail struct {
	Key           string `json:"key"`
	Type          string `json:"type"`
	TTL           int64  `json:"ttl"`
	Size          int64  `json:"size"`
	Total         int64  `json:"total"`
	Value         any    `json:"value"`
	ValueCursor   string `json:"valueCursor"`
	ValueOffset   int64  `json:"valueOffset"`
	HasMoreValues bool   `json:"hasMoreValues"`
}

type RedisHashEntry struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type RedisZSetEntry struct {
	Member string  `json:"member"`
	Score  float64 `json:"score"`
}

type RedisStreamEntry struct {
	ID     string            `json:"id"`
	Fields map[string]string `json:"fields"`
}

type RedisStringSetRequest struct {
	AssetID int64  `json:"assetId"`
	DB      int    `json:"db"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Format  string `json:"format"`
}

type RedisStreamField struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type RedisSlowLogEntry struct {
	ID             int64    `json:"id"`
	Timestamp      int64    `json:"timestamp"`
	DurationMicros int64    `json:"durationMicros"`
	Command        []string `json:"command"`
	Client         string   `json:"client,omitempty"`
	ClientName     string   `json:"clientName,omitempty"`
}

// RedisFormattedValue is a display-only projection of a Redis value.
type RedisFormattedValue struct {
	Format string `json:"format"`
	Value  string `json:"value"`
	Valid  bool   `json:"valid"`
	Error  string `json:"error,omitempty"`
}
