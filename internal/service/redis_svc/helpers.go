package redis_svc

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultRedisScanCount = int64(200)
	maxRedisScanCount     = int64(2000)
)

var keyspaceLineRE = regexp.MustCompile(`^db(\d+):(.+)$`)

func ParseKeyspaceInfo(info string) []RedisDatabase {
	lines := strings.Split(strings.ReplaceAll(info, "\r\n", "\n"), "\n")
	dbs := make([]RedisDatabase, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		m := keyspaceLineRE.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		dbIndex, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		item := RedisDatabase{DB: dbIndex}
		for _, part := range strings.Split(m[2], ",") {
			key, val, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				continue
			}
			switch key {
			case "keys":
				item.Keys = n
			case "expires":
				item.Expires = n
			case "avg_ttl":
				item.AvgTTL = n
			}
		}
		dbs = append(dbs, item)
	}
	sort.Slice(dbs, func(i, j int) bool { return dbs[i].DB < dbs[j].DB })
	return dbs
}

func NormalizeScanOptions(req RedisScanRequest) RedisScanRequest {
	req.Match = strings.TrimSpace(req.Match)
	if req.Match == "" {
		req.Match = "*"
	}
	req.Cursor = strings.TrimSpace(req.Cursor)
	if req.Cursor == "" {
		req.Cursor = "0"
	}
	if req.Count <= 0 {
		req.Count = defaultRedisScanCount
	}
	if req.Count > maxRedisScanCount {
		req.Count = maxRedisScanCount
	}
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	return req
}

func ValidatePatternDelete(pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	if strings.Trim(pattern, "*") == "" {
		return fmt.Errorf("refusing to delete the entire database")
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return fmt.Errorf("pattern delete requires a wildcard")
	}
	return nil
}
