package query_svc

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestOpenTable_MySQL(t *testing.T) {
	Convey("OpenTable MySQL 一次返回 4 部分数据", t, func() {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(false))
		So(err, ShouldBeNil)
		defer func() { _ = db.Close() }()
		mock.MatchExpectationsInOrder(true)

		// 1. primary keys
		mock.ExpectQuery("SHOW KEYS FROM `mydb`.`users` WHERE Key_name = 'PRIMARY'").
			WillReturnRows(sqlmock.NewRows([]string{"Table", "Column_name"}).
				AddRow("users", "id"))

		// 2. columns
		mock.ExpectQuery("SHOW COLUMNS FROM `mydb`.`users`").
			WillReturnRows(sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
				AddRow("id", "int", "NO", "PRI", nil, "auto_increment").
				AddRow("name", "varchar(255)", "YES", "", "anon", ""))

		// 3. count
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM `mydb`.`users`").
			WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(42))

		// 4. first page
		mock.ExpectQuery("SELECT \\* FROM `mydb`.`users` LIMIT 10 OFFSET 0").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
				AddRow(1, "alice").
				AddRow(2, "bob"))

		res, err := OpenTable(context.Background(), db, asset_entity.DriverMySQL, "mydb", "users", 10)
		So(err, ShouldBeNil)
		So(res, ShouldNotBeNil)
		So(res.PrimaryKeys, ShouldResemble, []string{"id"})
		So(res.Columns, ShouldResemble, []string{"id", "name"})
		So(res.ColumnTypes["id"], ShouldEqual, "int")
		So(res.ColumnTypes["name"], ShouldEqual, "varchar(255)")
		So(res.TotalCount, ShouldEqual, 42)
		So(res.PageSize, ShouldEqual, 10)
		So(len(res.FirstPage), ShouldEqual, 2)
		So(res.FirstPage[0]["name"], ShouldEqual, "alice")

		// rules
		var idRule, nameRule TableColumnRule
		for _, r := range res.ColumnRules {
			if r.Name == "id" {
				idRule = r
			}
			if r.Name == "name" {
				nameRule = r
			}
		}
		So(idRule.Nullable, ShouldBeFalse)
		So(idRule.AutoIncrement, ShouldBeTrue)
		So(nameRule.Nullable, ShouldBeTrue)
		So(nameRule.HasDefault, ShouldBeTrue) // "anon"
		So(nameRule.AutoIncrement, ShouldBeFalse)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestOpenTable_PostgreSQL(t *testing.T) {
	Convey("OpenTable PG 4 条 SQL 顺序正确", t, func() {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(false))
		So(err, ShouldBeNil)
		defer func() { _ = db.Close() }()
		mock.MatchExpectationsInOrder(true)

		mock.ExpectQuery("information_schema.table_constraints").
			WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("id"))

		mock.ExpectQuery("information_schema.columns").
			WillReturnRows(sqlmock.NewRows([]string{"column_name", "data_type", "udt_name", "is_nullable", "column_default"}).
				AddRow("id", "integer", "int4", "NO", "nextval('seq')").
				AddRow("name", "text", "text", "YES", nil))

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM "users"`).
			WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(5))

		mock.ExpectQuery(`SELECT \* FROM "users" LIMIT 20 OFFSET 0`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "alice"))

		res, err := OpenTable(context.Background(), db, asset_entity.DriverPostgreSQL, "mydb", "users", 20)
		So(err, ShouldBeNil)
		So(res.PrimaryKeys, ShouldResemble, []string{"id"})
		So(res.TotalCount, ShouldEqual, 5)
		So(res.ColumnTypes["name"], ShouldEqual, "text")
		So(len(res.FirstPage), ShouldEqual, 1)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestOpenTable_PageSizeDefault(t *testing.T) {
	Convey("pageSize<=0 时回退到 1000", t, func() {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(false))
		So(err, ShouldBeNil)
		defer func() { _ = db.Close() }()
		mock.MatchExpectationsInOrder(true)

		mock.ExpectQuery("SHOW KEYS").WillReturnRows(sqlmock.NewRows([]string{"Column_name"}))
		mock.ExpectQuery("SHOW COLUMNS").WillReturnRows(sqlmock.NewRows([]string{"Field", "Type", "Null", "Default", "Extra"}))
		mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(0))
		mock.ExpectQuery("LIMIT 1000 OFFSET 0").WillReturnRows(sqlmock.NewRows([]string{"id"}))

		res, err := OpenTable(context.Background(), db, asset_entity.DriverMySQL, "mydb", "users", 0)
		So(err, ShouldBeNil)
		So(res.PageSize, ShouldEqual, 1000)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
