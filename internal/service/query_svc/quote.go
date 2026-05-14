package query_svc

import (
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

// QuoteIdent 对单个 SQL 标识符按 driver 加引号。
// MySQL 用反引号,反引号转义为两个反引号。
// PostgreSQL 用双引号,内部双引号转义为两个双引号。
//
// 行为与前端 frontend/src/lib/tableSql.ts:quoteIdent 等价,移到后端是为了
// OpenTable 等服务端拼装 SQL 时复用,不再依赖前端传 SQL 字符串。
func QuoteIdent(name string, driver asset_entity.DatabaseDriver) string {
	if driver == asset_entity.DriverPostgreSQL {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// QuoteTableRef 把 db + table 拼成限定表引用。
// MySQL: `db`.`table`
// PostgreSQL: 仅 "table"(database 在前端模型里对应 PG 的"数据库连接",
// 表所在 schema 由 search_path 解析,这里与前端 quoteTableRef 行为一致)。
func QuoteTableRef(database, table string, driver asset_entity.DatabaseDriver) string {
	if driver == asset_entity.DriverPostgreSQL {
		return quoteQualified(table, driver)
	}
	return QuoteIdent(database, driver) + "." + QuoteIdent(table, driver)
}

// SQLQuote 把任意值包成 SQL 字面量,主要用于 information_schema 查询里的
// 字符串参数(更安全的做法当然是参数化查询,但 SHOW/SELECT 字面量场景下
// 这里只接受调用方已经清洗过的 string,因此可控)。
func SQLQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func quoteQualified(name string, driver asset_entity.DatabaseDriver) string {
	parts := strings.Split(name, ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, QuoteIdent(p, driver))
	}
	return strings.Join(out, ".")
}
