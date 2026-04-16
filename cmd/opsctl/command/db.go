package command

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/opskat/opskat/internal/ai"
	"github.com/opskat/opskat/internal/approval"
)

func cmdSQL(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printSQLUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	asset, err := resolveAsset(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("sql", flag.ContinueOnError)
	file := fs.String("f", "", "Read SQL from file")
	database := fs.String("d", "", "Override default database")
	fs.Usage = func() { printSQLUsage() }
	_ = fs.Parse(args[1:])

	var sqlText string
	if *file != "" {
		data, readErr := os.ReadFile(*file)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", readErr)
			return 1
		}
		sqlText = string(data)
	} else {
		sqlText = strings.Join(fs.Args(), " ")
	}

	if sqlText == "" {
		fmt.Fprintln(os.Stderr, "Error: SQL statement is required")
		printSQLUsage()
		return 1
	}

	// Require approval
	argsJSON := fmt.Sprintf(`{"asset_id":%d,"sql":%q}`, asset.ID, truncateStr(sqlText, 200))
	approvalResult, approvalErr := requireApproval(ctx, approval.ApprovalRequest{
		Type:      "sql",
		AssetID:   asset.ID,
		AssetName: asset.Name,
		Command:   sqlText,
		Detail:    fmt.Sprintf("opsctl sql %s %q", args[0], truncateStr(sqlText, 100)),
		SessionID: session,
	})
	auditCtx := ai.WithSessionID(ctx, approvalResult.SessionID)
	if approvalErr != nil {
		writeOpsctlAudit(auditCtx, "exec_sql", argsJSON, "", approvalErr, approvalResult.ToCheckResult())
		fmt.Fprintf(os.Stderr, "Error: %v\n", approvalErr)
		return 1
	}

	params := map[string]any{
		"asset_id": float64(asset.ID),
		"sql":      sqlText,
	}
	if *database != "" {
		params["database"] = *database
	}
	return callHandler(auditCtx, handlers, "exec_sql", params, approvalResult.ToCheckResult())
}

func cmdRedisCmd(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printRedisUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	asset, err := resolveAsset(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("redis", flag.ContinueOnError)
	db := fs.Int("n", -1, "Redis database number (0-15)")
	fs.Usage = func() { printRedisUsage() }
	_ = fs.Parse(args[1:])

	command := strings.Join(fs.Args(), " ")
	if command == "" {
		fmt.Fprintln(os.Stderr, "Error: Redis command is required")
		printRedisUsage()
		return 1
	}

	// Require approval
	argsJSON := fmt.Sprintf(`{"asset_id":%d,"command":%q}`, asset.ID, truncateStr(command, 200))
	approvalResult, approvalErr := requireApproval(ctx, approval.ApprovalRequest{
		Type:      "redis",
		AssetID:   asset.ID,
		AssetName: asset.Name,
		Command:   command,
		Detail:    fmt.Sprintf("opsctl redis %s %q", args[0], truncateStr(command, 100)),
		SessionID: session,
	})
	auditCtx := ai.WithSessionID(ctx, approvalResult.SessionID)
	if approvalErr != nil {
		writeOpsctlAudit(auditCtx, "exec_redis", argsJSON, "", approvalErr, approvalResult.ToCheckResult())
		fmt.Fprintf(os.Stderr, "Error: %v\n", approvalErr)
		return 1
	}

	params := map[string]any{
		"asset_id": float64(asset.ID),
		"command":  command,
	}
	if *db >= 0 {
		params["db"] = float64(*db)
	}
	return callHandler(auditCtx, handlers, "exec_redis", params, approvalResult.ToCheckResult())
}

func cmdMongo(ctx context.Context, handlers map[string]ai.ToolHandlerFunc, args []string, session string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printMongoUsage()
		if len(args) > 0 {
			return 0
		}
		return 1
	}

	asset, err := resolveAsset(ctx, args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fs := flag.NewFlagSet("mongo", flag.ContinueOnError)
	database := fs.String("d", "", "Database name (required)")
	collection := fs.String("c", "", "Collection name (required)")
	operation := fs.String("o", "find", "Operation (e.g. find, insertOne, updateOne, deleteOne, aggregate)")
	fs.Usage = func() { printMongoUsage() }
	_ = fs.Parse(args[1:])

	if *database == "" {
		fmt.Fprintln(os.Stderr, "Error: -d (database) is required")
		printMongoUsage()
		return 1
	}
	if *collection == "" {
		fmt.Fprintln(os.Stderr, "Error: -c (collection) is required")
		printMongoUsage()
		return 1
	}

	query := strings.Join(fs.Args(), " ")
	if query == "" {
		query = "{}"
	}

	// Require approval
	argsJSON := fmt.Sprintf(`{"asset_id":%d,"database":%q,"collection":%q,"operation":%q,"query":%q}`,
		asset.ID, *database, *collection, *operation, truncateStr(query, 200))
	approvalResult, approvalErr := requireApproval(ctx, approval.ApprovalRequest{
		Type:      "mongo",
		AssetID:   asset.ID,
		AssetName: asset.Name,
		Command:   fmt.Sprintf("%s.%s.%s(%s)", *database, *collection, *operation, truncateStr(query, 100)),
		Detail:    fmt.Sprintf("opsctl mongo %s -d %s -c %s -o %s %s", args[0], *database, *collection, *operation, truncateStr(query, 100)),
		SessionID: session,
	})
	auditCtx := ai.WithSessionID(ctx, approvalResult.SessionID)
	if approvalErr != nil {
		writeOpsctlAudit(auditCtx, "exec_mongo", argsJSON, "", approvalErr, approvalResult.ToCheckResult())
		fmt.Fprintf(os.Stderr, "Error: %v\n", approvalErr)
		return 1
	}

	params := map[string]any{
		"asset_id":   float64(asset.ID),
		"database":   *database,
		"collection": *collection,
		"operation":  *operation,
		"query":      query,
	}
	return callHandler(auditCtx, handlers, "exec_mongo", params, approvalResult.ToCheckResult())
}

func printSQLUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] sql <asset> [flags] "<SQL>"

Arguments:
  asset     Database asset name or numeric ID

Flags:
  -f <file>     Read SQL from file instead of argument
  -d <database> Override the default database for this execution

Approval:
  SQL statements are checked against the asset's query policy:
  - Allowed types (e.g. SELECT) execute without approval
  - Denied types (e.g. DROP TABLE) are rejected
  - Other statements require user confirmation (desktop app) or are rejected (offline)

Examples:
  opsctl sql prod-db "SELECT * FROM users LIMIT 10"
  opsctl sql prod-db "INSERT INTO logs (msg) VALUES ('test')"
  opsctl sql prod-db -f migration.sql
  opsctl sql prod-db -d other_db "SHOW TABLES"
`)
}

func printRedisUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] redis <asset> [flags] "<command>"

Arguments:
  asset     Redis asset name or numeric ID
  command   Redis command (e.g. "GET mykey", "HGETALL user:1")

Flags:
  -n <db>   Override the default database number (0-15)

Approval:
  Commands are checked against the asset's Redis policy:
  - Dangerous commands (FLUSHDB, CONFIG SET, etc.) are rejected by default
  - Other commands require user confirmation (desktop app) or are rejected (offline)

Examples:
  opsctl redis cache "GET session:abc123"
  opsctl redis cache "HGETALL user:1"
  opsctl redis cache -n 2 "KEYS user:*"
  opsctl redis cache "SET key value EX 3600"
`)
}

func printMongoUsage() {
	fmt.Fprint(os.Stderr, `Usage:
  opsctl [--session <id>] mongo <asset> -d <database> -c <collection> [flags] "<query>"

Arguments:
  asset     MongoDB asset name or numeric ID
  query     Query document as JSON (default "{}")

Flags:
  -d <database>    Database name (required)
  -c <collection>  Collection name (required)
  -o <operation>   Operation to run (default: find)
                   e.g. find, insertOne, updateOne, deleteOne, aggregate

Approval:
  Operations are checked against the asset's MongoDB policy:
  - Read operations (find, aggregate) may be auto-approved depending on policy
  - Write operations (insertOne, updateOne, deleteOne) require user confirmation (desktop app) or are rejected (offline)

Examples:
  opsctl mongo prod-mongo -d mydb -c users '{}'
  opsctl mongo prod-mongo -d mydb -c users -o find '{"filter":{"status":"active"}}'
  opsctl mongo prod-mongo -d mydb -c logs -o insertOne '{"document":{"msg":"hello"}}'
  opsctl mongo prod-mongo -d mydb -c orders -o aggregate '{"pipeline":[{"$match":{"status":"pending"}}]}'
`)
}
