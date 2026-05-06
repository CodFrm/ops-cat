package query_svc

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
)

type fakeSQLResult int64

func (r fakeSQLResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeSQLResult) RowsAffected() (int64, error) { return int64(r), nil }

type fakeSQLSession struct {
	operations []string
	failOn     string
	tx         *fakeSQLTx
}

func (s *fakeSQLSession) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	s.operations = append(s.operations, "conn:"+query)
	if s.failOn != "" && strings.Contains(query, s.failOn) {
		return nil, errors.New("statement failed")
	}
	return fakeSQLResult(1), nil
}

func (s *fakeSQLSession) BeginTx(_ context.Context, _ *sql.TxOptions) (SQLTx, error) {
	s.operations = append(s.operations, "begin")
	s.tx = &fakeSQLTx{session: s}
	return s.tx, nil
}

type fakeSQLTx struct {
	session *fakeSQLSession
}

func (tx *fakeSQLTx) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	tx.session.operations = append(tx.session.operations, "tx:"+query)
	if tx.session.failOn != "" && strings.Contains(query, tx.session.failOn) {
		return nil, errors.New("statement failed")
	}
	return fakeSQLResult(1), nil
}

func (tx *fakeSQLTx) Commit() error {
	tx.session.operations = append(tx.session.operations, "commit")
	return nil
}

func (tx *fakeSQLTx) Rollback() error {
	tx.session.operations = append(tx.session.operations, "rollback")
	return nil
}

func TestRunTableImportBatchRollsBackCopyAndRestoresForeignKeyChecks(t *testing.T) {
	session := &fakeSQLSession{failOn: "INSERT"}
	result, err := RunTableImportBatch(context.Background(), session, asset_entity.DriverMySQL, TableImportBatchRequest{
		Mode:                    "copy",
		ContinueOnError:         true,
		DisableForeignKeyChecks: true,
		Statements: []string{
			"DELETE FROM `appdb`.`users`;",
			"INSERT INTO `appdb`.`users` (`id`) VALUES (1);",
		},
	})
	if err != nil {
		t.Fatalf("RunTableImportBatch() error = %v", err)
	}

	if !result.RolledBack {
		t.Fatalf("expected copy import to roll back on statement failure")
	}
	if result.Error != 1 || result.Processed != 2 {
		t.Fatalf("unexpected result counters: %+v", result)
	}
	if result.Added != 0 || result.Updated != 0 || result.Deleted != 0 {
		t.Fatalf("rolled back import should not report persisted row counts: %+v", result)
	}
	wantOps := []string{
		"conn:SET FOREIGN_KEY_CHECKS = 0",
		"begin",
		"tx:DELETE FROM `appdb`.`users`;",
		"tx:INSERT INTO `appdb`.`users` (`id`) VALUES (1);",
		"rollback",
		"conn:SET FOREIGN_KEY_CHECKS = 1",
	}
	if strings.Join(session.operations, "\n") != strings.Join(wantOps, "\n") {
		t.Fatalf("unexpected operation order:\n got: %v\nwant: %v", session.operations, wantOps)
	}
}

func TestRunTableImportBatchContinuesNonAtomicImportAndRestoresForeignKeyChecks(t *testing.T) {
	session := &fakeSQLSession{failOn: "bad"}
	result, err := RunTableImportBatch(context.Background(), session, asset_entity.DriverMySQL, TableImportBatchRequest{
		Mode:                    "append",
		ContinueOnError:         true,
		DisableForeignKeyChecks: true,
		Statements: []string{
			"INSERT INTO `appdb`.`users` (`id`) VALUES ('bad');",
			"INSERT INTO `appdb`.`users` (`id`) VALUES (2);",
		},
	})
	if err != nil {
		t.Fatalf("RunTableImportBatch() error = %v", err)
	}

	if result.RolledBack {
		t.Fatalf("append import with continueOnError should not be atomic")
	}
	if result.Error != 1 || result.Processed != 2 || result.Added != 1 {
		t.Fatalf("unexpected result counters: %+v", result)
	}
	wantOps := []string{
		"conn:SET FOREIGN_KEY_CHECKS = 0",
		"conn:INSERT INTO `appdb`.`users` (`id`) VALUES ('bad');",
		"conn:INSERT INTO `appdb`.`users` (`id`) VALUES (2);",
		"conn:SET FOREIGN_KEY_CHECKS = 1",
	}
	if strings.Join(session.operations, "\n") != strings.Join(wantOps, "\n") {
		t.Fatalf("unexpected operation order:\n got: %v\nwant: %v", session.operations, wantOps)
	}
}
