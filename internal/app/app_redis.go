package app

import "github.com/opskat/opskat/internal/service/redis_svc"

func (a *App) redisSvc() *redis_svc.Service {
	if a.redisService == nil {
		a.redisService = redis_svc.New(a.sshPool)
	}
	return a.redisService
}

func (a *App) RedisListDatabases(assetID int64) ([]redis_svc.RedisDatabase, error) {
	return a.redisSvc().ListDatabases(a.langCtx(), assetID)
}

func (a *App) RedisScanKeys(req redis_svc.RedisScanRequest) (redis_svc.RedisScanResponse, error) {
	return a.redisSvc().ScanKeys(a.langCtx(), req)
}

func (a *App) RedisGetKeyDetail(req redis_svc.RedisKeyRequest) (redis_svc.RedisKeyDetail, error) {
	return a.redisSvc().GetKeyDetail(a.langCtx(), req)
}

func (a *App) RedisSetKeyTTL(assetID int64, db int, key string, seconds int64) error {
	return a.redisSvc().SetKeyTTL(a.langCtx(), assetID, db, key, seconds)
}

func (a *App) RedisPersistKey(assetID int64, db int, key string) error {
	return a.redisSvc().PersistKey(a.langCtx(), assetID, db, key)
}

func (a *App) RedisRenameKey(assetID int64, db int, oldKey string, newKey string) error {
	return a.redisSvc().RenameKey(a.langCtx(), assetID, db, oldKey, newKey)
}

func (a *App) RedisDeleteKeys(assetID int64, db int, keys []string) error {
	return a.redisSvc().DeleteKeys(a.langCtx(), assetID, db, keys)
}

func (a *App) RedisSetStringValue(req redis_svc.RedisStringSetRequest) error {
	return a.redisSvc().SetStringValue(a.langCtx(), req)
}

func (a *App) RedisHashSet(assetID int64, db int, key string, field string, value string) error {
	return a.redisSvc().HashSet(a.langCtx(), assetID, db, key, field, value)
}

func (a *App) RedisHashDelete(assetID int64, db int, key string, field string) error {
	return a.redisSvc().HashDelete(a.langCtx(), assetID, db, key, field)
}

func (a *App) RedisListPush(assetID int64, db int, key string, value string) error {
	return a.redisSvc().ListPush(a.langCtx(), assetID, db, key, value)
}

func (a *App) RedisListSet(assetID int64, db int, key string, index int64, value string) error {
	return a.redisSvc().ListSet(a.langCtx(), assetID, db, key, index, value)
}

func (a *App) RedisListDelete(assetID int64, db int, key string, index int64) error {
	return a.redisSvc().ListDelete(a.langCtx(), assetID, db, key, index)
}

func (a *App) RedisSetAdd(assetID int64, db int, key string, member string) error {
	return a.redisSvc().SetAdd(a.langCtx(), assetID, db, key, member)
}

func (a *App) RedisSetRemove(assetID int64, db int, key string, member string) error {
	return a.redisSvc().SetRemove(a.langCtx(), assetID, db, key, member)
}

func (a *App) RedisZSetAdd(assetID int64, db int, key string, member string, score float64) error {
	return a.redisSvc().ZSetAdd(a.langCtx(), assetID, db, key, member, score)
}

func (a *App) RedisZSetRemove(assetID int64, db int, key string, member string) error {
	return a.redisSvc().ZSetRemove(a.langCtx(), assetID, db, key, member)
}

func (a *App) RedisStreamAdd(assetID int64, db int, key string, id string, fields []redis_svc.RedisStreamField) error {
	return a.redisSvc().StreamAdd(a.langCtx(), assetID, db, key, id, fields)
}

func (a *App) RedisStreamDelete(assetID int64, db int, key string, ids []string) error {
	return a.redisSvc().StreamDelete(a.langCtx(), assetID, db, key, ids)
}

func (a *App) RedisClientList(assetID int64) (string, error) {
	return a.redisSvc().ClientList(a.langCtx(), assetID)
}

func (a *App) RedisSlowLog(assetID int64, limit int64) ([]redis_svc.RedisSlowLogEntry, error) {
	return a.redisSvc().SlowLog(a.langCtx(), assetID, limit)
}

func (a *App) RedisCommandHistory(assetID int64, limit int) []redis_svc.CommandHistoryEntry {
	return a.redisSvc().CommandHistory(assetID, limit)
}

func (a *App) RedisFormatValue(value string, format string) redis_svc.RedisFormattedValue {
	return redis_svc.FormatDisplayValue(value, format)
}

func (a *App) RedisEncodeValue(value string, format string) (string, error) {
	return redis_svc.EncodeValueForStorage(value, format)
}
