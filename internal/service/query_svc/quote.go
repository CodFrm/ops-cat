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
// MySQL: `db`.`table`(database 是 MySQL 的库名)。
// PostgreSQL: 忽略 database 参数(database 在前端模型里对应 PG 的"数据库连接");
// table 既可以是裸表名(由 search_path 解析,通常落到 public),也可以是 "schema.table"
// 形式——quoteQualified 会按点号拆分并分别加引号,行为与前端 quoteTableRef 一致。
func QuoteTableRef(database, table string, driver asset_entity.DatabaseDriver) string {
	if driver == asset_entity.DriverPostgreSQL {
		return quoteQualified(table, driver)
	}
	return QuoteIdent(database, driver) + "." + QuoteIdent(table, driver)
}

// SQLQuote 把字符串包成 SQL 字符串字面量,只做单引号转义('  -> ”)。
// 主要用于 information_schema 查询里需要字面量比较的字段(如 table_schema、
// table_name)。更安全的做法是参数化查询,但当前 information_schema 拼接
// 路径足够窄、SQL 直接落驱动层,这里用字面量转义即可。注意:只防 SQL 注入,
// 不做语义校验——调用方仍需保证传入的是预期的标识符。
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
