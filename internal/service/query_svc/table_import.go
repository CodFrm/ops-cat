package query_svc

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

// SQLTx is the small transaction surface needed by table imports.
type SQLTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	Commit() error
	Rollback() error
}

// SQLSession is a single database session. MySQL FOREIGN_KEY_CHECKS is session-scoped,
// so imports that toggle it must use one SQL session from start to restore.
type SQLSession interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (SQLTx, error)
}

type sqlConnSession struct {
	conn *sql.Conn
}

// NewSQLSession adapts database/sql's Conn to the import service interface.
func NewSQLSession(conn *sql.Conn) SQLSession {
	return sqlConnSession{conn: conn}
}

func (s sqlConnSession) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.conn.ExecContext(ctx, query, args...)
}

func (s sqlConnSession) BeginTx(ctx context.Context, opts *sql.TxOptions) (SQLTx, error) {
	return s.conn.BeginTx(ctx, opts)
}

// TableImportBatchRequest describes a prepared table import batch.
type TableImportBatchRequest struct {
	Statements              []string `json:"statements"`
	Mode                    string   `json:"mode"`
	ContinueOnError         bool     `json:"continueOnError"`
	DisableForeignKeyChecks bool     `json:"disableForeignKeyChecks"`
}

// TableImportBatchError captures a failed statement without leaking connection setup failures into row logs.
type TableImportBatchError struct {
	Index     int    `json:"index"`
	Statement string `json:"statement"`
	Message   string `json:"message"`
}

// TableImportBatchResult returns aggregate counters for the import summary.
type TableImportBatchResult struct {
	Processed  int                     `json:"processed"`
	Added      int64                   `json:"added"`
	Updated    int64                   `json:"updated"`
	Deleted    int64                   `json:"deleted"`
	Error      int                     `json:"error"`
	RolledBack bool                    `json:"rolledBack"`
	Errors     []TableImportBatchError `json:"errors"`
}

// RunTableImportBatch executes an import batch on one database session.
func RunTableImportBatch(
	ctx context.Context,
	session SQLSession,
	driver asset_entity.DatabaseDriver,
	request TableImportBatchRequest,
) (result *TableImportBatchResult, err error) {
	result = &TableImportBatchResult{}
	if session == nil {
		return nil, fmt.Errorf("SQL session is nil")
	}

	disableFK := request.DisableForeignKeyChecks && driver == asset_entity.DriverMySQL
	if disableFK {
		if _, execErr := session.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); execErr != nil {
			return nil, fmt.Errorf("disable foreign key checks: %w", execErr)
		}
		defer func() {
			if _, restoreErr := session.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); restoreErr != nil {
				if err == nil && len(result.Errors) == 0 {
					err = fmt.Errorf("restore foreign key checks: %w", restoreErr)
					return
				}
				result.Error++
				result.Errors = append(result.Errors, TableImportBatchError{
					Index:   -1,
					Message: fmt.Sprintf("restore foreign key checks: %v", restoreErr),
				})
			}
		}()
	}

	atomic := shouldRunAtomicImport(request)
	var execer interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	} = session
	var tx SQLTx
	if atomic {
		tx, err = session.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("begin import transaction: %w", err)
		}
		execer = tx
	}

	committed := false
	defer func() {
		if tx == nil || committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && err == nil {
			err = fmt.Errorf("rollback import transaction: %w", rbErr)
		}
	}()

	for index, statement := range request.Statements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		result.Processed++

		execResult, execErr := execer.ExecContext(ctx, statement)
		if execErr != nil {
			result.Error++
			result.Errors = append(result.Errors, TableImportBatchError{
				Index:     index,
				Statement: statement,
				Message:   execErr.Error(),
			})
			if atomic || !request.ContinueOnError {
				break
			}
			continue
		}

		affected, rowsErr := execResult.RowsAffected()
		if rowsErr != nil {
			affected = 0
		}
		addAffectedRows(result, statement, request.Mode, affected)
	}

	if len(result.Errors) > 0 {
		if atomic {
			result.Added = 0
			result.Updated = 0
			result.Deleted = 0
			result.RolledBack = true
		}
		return result, nil
	}

	if tx != nil {
		if err = tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit import transaction: %w", err)
		}
		committed = true
	}
	return result, nil
}

func shouldRunAtomicImport(request TableImportBatchRequest) bool {
	return request.Mode == "copy" || !request.ContinueOnError
}

func addAffectedRows(result *TableImportBatchResult, statement string, mode string, affected int64) {
	kind := leadingSQLKeyword(statement)
	switch kind {
	case "DELETE":
		result.Deleted += affected
	case "UPDATE":
		result.Updated += affected
	case "INSERT", "REPLACE":
		result.Added += affected
	default:
		switch mode {
		case "update":
			result.Updated += affected
		case "delete":
			result.Deleted += affected
		default:
			result.Added += affected
		}
	}
}

func leadingSQLKeyword(statement string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return ""
	}
	fields := strings.Fields(statement)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(fields[0])
}
