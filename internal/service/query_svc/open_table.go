package query_svc

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

// TableColumnRule 描述一列的约束,用于前端 INSERT 校验。
// 与 frontend/src/components/query/TableDataTab.tsx 现有结构对齐。
type TableColumnRule struct {
	Name          string `json:"name"`
	Nullable      bool   `json:"nullable"`
	HasDefault    bool   `json:"hasDefault"`
	AutoIncrement bool   `json:"autoIncrement"`
}

// OpenTableResult 是 OpenTable 一次返回的全部首屏数据。
// 把原来前端 4 次 ExecuteSQL 合并为一次响应。
type OpenTableResult struct {
	Columns     []string          `json:"columns"`
	ColumnTypes map[string]string `json:"columnTypes"`
	ColumnRules []TableColumnRule `json:"columnRules"`
	PrimaryKeys []string          `json:"primaryKeys"`
	TotalCount  int64             `json:"totalCount"`
	FirstPage   []map[string]any  `json:"firstPage"`
	PageSize    int               `json:"pageSize"`
}

// OpenTable 在同一条 *sql.Conn 上顺序执行 4 条 SQL 并组装结果。
// 调用方负责传入已通过缓存复用的 *sql.DB,本函数自己 db.Conn(ctx) 一次。
//
// 4 条查询:
//  1. primary keys (SHOW KEYS / PG information_schema)
//  2. columns + types + rules (SHOW COLUMNS / PG information_schema)
//  3. SELECT COUNT(*) FROM <table>
//  4. SELECT * FROM <table> LIMIT pageSize OFFSET 0
//
// 任一步失败立即返回错误;不做"部分成功"的容错,前端按整次失败处理。
func OpenTable(ctx context.Context, db *sql.DB, driver asset_entity.DatabaseDriver, database, table string, pageSize int) (*OpenTableResult, error) {
	if pageSize <= 0 {
		pageSize = 1000
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open db conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	pks, err := queryPrimaryKeys(ctx, conn, driver, database, table)
	if err != nil {
		return nil, fmt.Errorf("query primary keys: %w", err)
	}

	cols, types, rules, err := queryColumns(ctx, conn, driver, database, table)
	if err != nil {
		return nil, fmt.Errorf("query columns: %w", err)
	}

	tableRef := QuoteTableRef(database, table, driver)

	var total int64
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tableRef).Scan(&total); err != nil {
		return nil, fmt.Errorf("count: %w", err)
	}

	firstPage, dataCols, err := queryFirstPage(ctx, conn, tableRef, pageSize)
	if err != nil {
		return nil, fmt.Errorf("first page: %w", err)
	}

	// 优先使用 SELECT * 的列顺序(与实际 SELECT 行结果对齐);
	// 退化:若空表/无列,落回 SHOW COLUMNS 的结果。
	finalCols := dataCols
	if len(finalCols) == 0 {
		finalCols = cols
	}

	return &OpenTableResult{
		Columns:     finalCols,
		ColumnTypes: types,
		ColumnRules: rules,
		PrimaryKeys: pks,
		TotalCount:  total,
		FirstPage:   firstPage,
		PageSize:    pageSize,
	}, nil
}

func queryPrimaryKeys(ctx context.Context, conn *sql.Conn, driver asset_entity.DatabaseDriver, database, table string) ([]string, error) {
	var sqlText string
	switch driver {
	case asset_entity.DriverPostgreSQL:
		schema, tbl := splitPGSchemaTable(table)
		sqlText = "SELECT kcu.column_name FROM information_schema.table_constraints tc " +
			"JOIN information_schema.key_column_usage kcu " +
			"ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema " +
			"WHERE tc.table_schema = " + SQLQuote(schema) +
			" AND tc.table_name = " + SQLQuote(tbl) +
			" AND tc.constraint_type = 'PRIMARY KEY' ORDER BY kcu.ordinal_position"
	default:
		sqlText = "SHOW KEYS FROM " + QuoteTableRef(database, table, driver) + " WHERE Key_name = 'PRIMARY'"
	}
	rows, err := conn.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, 4)
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		// 行结果按列名查找,兼容 MySQL("Column_name") 与 PG("column_name")
		row := zipRow(cols, values)
		name := pickString(row, "Column_name", "column_name")
		if name != "" {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

func queryColumns(ctx context.Context, conn *sql.Conn, driver asset_entity.DatabaseDriver, database, table string) ([]string, map[string]string, []TableColumnRule, error) {
	var sqlText string
	switch driver {
	case asset_entity.DriverPostgreSQL:
		schema, tbl := splitPGSchemaTable(table)
		sqlText = "SELECT column_name, data_type, udt_name, is_nullable, column_default " +
			"FROM information_schema.columns WHERE table_schema = " + SQLQuote(schema) +
			" AND table_name = " + SQLQuote(tbl) + " ORDER BY ordinal_position"
	default:
		sqlText = "SHOW COLUMNS FROM " + QuoteTableRef(database, table, driver)
	}
	rows, err := conn.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, nil, err
	}
	var names []string
	types := make(map[string]string)
	var rules []TableColumnRule
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, nil, err
		}
		row := zipRow(cols, values)
		name := pickString(row, "column_name", "Field", "field")
		if name == "" {
			continue
		}
		typeStr := pickString(row, "data_type", "Type", "type", "udt_name")
		nullableRaw := strings.ToUpper(pickString(row, "is_nullable", "Null", "null"))
		extra := strings.ToLower(pickString(row, "Extra", "extra"))
		defaultRaw, hasDefault := row["column_default"]
		if !hasDefault || defaultRaw == nil {
			defaultRaw, hasDefault = row["Default"]
		}
		if !hasDefault || defaultRaw == nil {
			defaultRaw = row["default"]
		}
		names = append(names, name)
		if typeStr != "" {
			types[name] = typeStr
		}
		rules = append(rules, TableColumnRule{
			Name:          name,
			Nullable:      nullableRaw == "YES",
			HasDefault:    defaultRaw != nil,
			AutoIncrement: strings.Contains(extra, "auto_increment"),
		})
	}
	return names, types, rules, rows.Err()
}

func queryFirstPage(ctx context.Context, conn *sql.Conn, tableRef string, pageSize int) ([]map[string]any, []string, error) {
	sqlText := fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET 0", tableRef, pageSize) //nolint:gosec // tableRef 已 quote
	rows, err := conn.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	out := make([]map[string]any, 0, pageSize)
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				val = string(b)
			}
			row[col] = val
		}
		out = append(out, row)
	}
	return out, cols, rows.Err()
}

func zipRow(cols []string, values []any) map[string]any {
	row := make(map[string]any, len(cols))
	for i, col := range cols {
		val := values[i]
		if b, ok := val.([]byte); ok {
			val = string(b)
		}
		row[col] = val
	}
	return row
}

// splitPGSchemaTable 把可能带 schema 前缀的 PG 表名拆成 (schema, table)。
// 输入 "events" → ("public", "events");"reporting.events" → ("reporting", "events");
// 与 QuoteTableRef/quoteQualified 的输入空间保持一致——后者会按点号拆分加引号,
// 这里也按同样规则提取 information_schema 查询所需的两段。
func splitPGSchemaTable(table string) (string, string) {
	parts := strings.Split(table, ".")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) >= 2 {
		return out[0], out[len(out)-1]
	}
	if len(out) == 1 {
		return "public", out[0]
	}
	return "public", ""
}

func pickString(row map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			if s, ok := v.(string); ok {
				return s
			}
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}
