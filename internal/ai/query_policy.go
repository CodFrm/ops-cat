package ai

import (
	"context"
	"fmt"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
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
		return nil, fmt.Errorf("SQL parse failed: %w", err)
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("empty SQL")
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

// CheckQueryPolicy 检查 SQL 语句是否符合策略（合并默认策略后检查）
func CheckQueryPolicy(ctx context.Context, policy *asset_entity.QueryPolicy, stmts []StatementInfo) CheckResult {
	merged := effectiveQueryPolicy(ctx, policy)
	return checkQueryPolicyRules(ctx, merged, stmts)
}

// checkQueryPolicyRules 检查 SQL 语句是否符合给定策略（不合并默认策略）
func checkQueryPolicyRules(ctx context.Context, policy *asset_entity.QueryPolicy, stmts []StatementInfo) CheckResult {
	if policy == nil {
		return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
	}
	for _, stmt := range stmts {
		// deny_types 检查
		for _, denied := range policy.DenyTypes {
			if policyValueMatches(denied, stmt.Type) {
				return CheckResult{
					Decision:       Deny,
					Message:        policyFmt(ctx, "SQL statement type %s denied by policy", "SQL 语句类型 %s 被策略禁止", stmt.Type),
					DecisionSource: SourcePolicyDeny,
					MatchedPattern: denied,
				}
			}
		}
		// deny_flags 检查
		if stmt.Dangerous && containsStr(policy.DenyFlags, stmt.Reason) {
			return CheckResult{
				Decision:       Deny,
				Message:        policyFmt(ctx, "SQL statement denied by policy: %s (%s)", "SQL 语句被策略禁止: %s (%s)", stmt.Reason, stmt.Raw),
				DecisionSource: SourcePolicyDeny,
				MatchedPattern: stmt.Reason,
			}
		}
		// allow_types 白名单
		if len(policy.AllowTypes) > 0 && !containsPolicyValue(policy.AllowTypes, stmt.Type) {
			return CheckResult{Decision: NeedConfirm}
		}
	}
	return CheckResult{Decision: Allow, DecisionSource: SourcePolicyAllow}
}

func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// appendUnique 按精确字符串去重。
// 不能按 ToUpper 折叠 —— Redis key、Kafka resource、shell 命令模式都是大小写敏感的，
// `GET User:*` 与 `GET user:*` 是两条不同的规则，合并会让其中一条静默失效。
func appendUnique(base []string, items ...string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	for _, s := range items {
		if !seen[s] {
			base = append(base, s)
			seen[s] = true
		}
	}
	return base
}
