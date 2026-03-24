package ai

import (
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
)

// StatementInfo 解析后的 SQL 语句分类信息
type StatementInfo struct {
	Type      string // SELECT, INSERT, UPDATE, DELETE, DROP TABLE, TRUNCATE, ...
	Raw       string
	Dangerous bool
	Reason    string // no_where_delete, no_where_update, prepare, call
}

// ClassifyStatements 解析 SQL 并返回每条语句的类型
func ClassifyStatements(sqlText string) ([]StatementInfo, error) {
	p := parser.New()
	stmts, _, err := p.Parse(sqlText, "", "")
	if err != nil {
		return nil, fmt.Errorf("SQL 解析失败: %w", err)
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("SQL 为空")
	}

	results := make([]StatementInfo, 0, len(stmts))
	for _, stmt := range stmts {
		info := classifyStmt(stmt)
		results = append(results, info)
	}
	return results, nil
}

func classifyStmt(stmt ast.StmtNode) StatementInfo {
	info := StatementInfo{Raw: stmt.Text()}
	switch s := stmt.(type) {
	case *ast.SelectStmt:
		info.Type = "SELECT"
	case *ast.InsertStmt:
		info.Type = "INSERT"
	case *ast.UpdateStmt:
		info.Type = "UPDATE"
		if s.Where == nil {
			info.Dangerous = true
			info.Reason = "no_where_update"
		}
	case *ast.DeleteStmt:
		info.Type = "DELETE"
		if s.Where == nil {
			info.Dangerous = true
			info.Reason = "no_where_delete"
		}
	case *ast.DropTableStmt:
		info.Type = "DROP TABLE"
	case *ast.DropDatabaseStmt:
		info.Type = "DROP DATABASE"
	case *ast.TruncateTableStmt:
		info.Type = "TRUNCATE"
	case *ast.CreateTableStmt:
		info.Type = "CREATE TABLE"
	case *ast.CreateDatabaseStmt:
		info.Type = "CREATE DATABASE"
	case *ast.AlterTableStmt:
		info.Type = "ALTER TABLE"
	case *ast.GrantStmt:
		info.Type = "GRANT"
	case *ast.RevokeStmt:
		info.Type = "REVOKE"
	case *ast.CreateUserStmt:
		info.Type = "CREATE USER"
	case *ast.DropUserStmt:
		info.Type = "DROP USER"
	case *ast.AlterUserStmt:
		info.Type = "ALTER USER"
	case *ast.PrepareStmt:
		info.Type = "PREPARE"
		info.Dangerous = true
		info.Reason = "prepare"
	case *ast.ExecuteStmt:
		info.Type = "EXECUTE"
		info.Dangerous = true
		info.Reason = "prepare"
	case *ast.CallStmt:
		info.Type = "CALL"
		info.Dangerous = true
		info.Reason = "call"
	case *ast.ShowStmt:
		info.Type = "SHOW"
	case *ast.ExplainStmt:
		info.Type = "EXPLAIN"
	case *ast.UseStmt:
		info.Type = "USE"
	default:
		info.Type = "OTHER"
	}
	return info
}

// CheckQueryPolicy 检查 SQL 语句是否符合策略
func CheckQueryPolicy(policy *asset_entity.QueryPolicy, stmts []StatementInfo) CheckResult {
	merged := mergeQueryPolicy(policy, asset_entity.DefaultQueryPolicy())

	for _, stmt := range stmts {
		// deny_types 检查
		for _, denied := range merged.DenyTypes {
			if strings.EqualFold(stmt.Type, denied) {
				return CheckResult{
					Decision:       Deny,
					Message:        fmt.Sprintf("SQL 语句类型 %s 被策略禁止", stmt.Type),
					DecisionSource: SourcePolicyDeny,
					MatchedPattern: denied,
				}
			}
		}
		// deny_flags 检查
		if stmt.Dangerous && containsStr(merged.DenyFlags, stmt.Reason) {
			return CheckResult{
				Decision:       Deny,
				Message:        fmt.Sprintf("SQL 语句被策略禁止: %s (%s)", stmt.Reason, stmt.Raw),
				DecisionSource: SourcePolicyDeny,
				MatchedPattern: stmt.Reason,
			}
		}
		// allow_types 白名单
		if len(merged.AllowTypes) > 0 && !containsStrFold(merged.AllowTypes, stmt.Type) {
			return CheckResult{Decision: NeedConfirm}
		}
	}
	return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
}

// CheckQueryPolicyOnly 只检查策略，不触发确认回调
func CheckQueryPolicyOnly(policy *asset_entity.QueryPolicy, sqlText string) CheckResult {
	stmts, err := ClassifyStatements(sqlText)
	if err != nil {
		return CheckResult{Decision: Deny, Message: fmt.Sprintf("SQL 解析失败，拒绝执行: %v", err)}
	}
	return CheckQueryPolicy(policy, stmts)
}

func mergeQueryPolicy(custom, defaults *asset_entity.QueryPolicy) *asset_entity.QueryPolicy {
	result := &asset_entity.QueryPolicy{}
	if custom != nil {
		result.AllowTypes = custom.AllowTypes
		result.DenyTypes = append(result.DenyTypes, custom.DenyTypes...)
		result.DenyFlags = append(result.DenyFlags, custom.DenyFlags...)
	}
	if defaults != nil {
		result.DenyTypes = appendUnique(result.DenyTypes, defaults.DenyTypes...)
		result.DenyFlags = appendUnique(result.DenyFlags, defaults.DenyFlags...)
	}
	return result
}

func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func containsStrFold(slice []string, s string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}

func appendUnique(base []string, items ...string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[strings.ToUpper(s)] = true
	}
	for _, s := range items {
		if !seen[strings.ToUpper(s)] {
			base = append(base, s)
			seen[strings.ToUpper(s)] = true
		}
	}
	return base
}
