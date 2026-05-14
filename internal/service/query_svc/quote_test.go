package query_svc

import (
	"testing"

	"github.com/opskat/opskat/internal/model/entity/asset_entity"
	. "github.com/smartystreets/goconvey/convey"
)

func TestQuoteIdent(t *testing.T) {
	Convey("QuoteIdent", t, func() {
		Convey("MySQL 用反引号", func() {
			So(QuoteIdent("user", asset_entity.DriverMySQL), ShouldEqual, "`user`")
		})
		Convey("PostgreSQL 用双引号", func() {
			So(QuoteIdent("user", asset_entity.DriverPostgreSQL), ShouldEqual, `"user"`)
		})
		Convey("MySQL 反引号转义", func() {
			So(QuoteIdent("a`b", asset_entity.DriverMySQL), ShouldEqual, "`a``b`")
		})
		Convey("PostgreSQL 双引号转义", func() {
			So(QuoteIdent(`a"b`, asset_entity.DriverPostgreSQL), ShouldEqual, `"a""b"`)
		})
	})
}

func TestQuoteTableRef(t *testing.T) {
	Convey("QuoteTableRef", t, func() {
		Convey("MySQL 加 db 前缀", func() {
			So(QuoteTableRef("mydb", "users", asset_entity.DriverMySQL), ShouldEqual, "`mydb`.`users`")
		})
		Convey("PostgreSQL 只引用表名", func() {
			So(QuoteTableRef("mydb", "users", asset_entity.DriverPostgreSQL), ShouldEqual, `"users"`)
		})
		Convey("PostgreSQL 支持 schema.table", func() {
			So(QuoteTableRef("mydb", "public.users", asset_entity.DriverPostgreSQL), ShouldEqual, `"public"."users"`)
		})
	})
}

func TestSQLQuote(t *testing.T) {
	Convey("SQLQuote 转义单引号", t, func() {
		So(SQLQuote("hello"), ShouldEqual, `'hello'`)
		So(SQLQuote("it's"), ShouldEqual, `'it''s'`)
	})
}
