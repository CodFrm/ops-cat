package status

import (
	"testing"

	cv "github.com/smartystreets/goconvey/convey"
)

func TestStatusRegistry(t *testing.T) {
	cv.Convey("StatusRegistry", t, func() {
		Reset()

		cv.Convey("initially empty", func() {
			cv.So(List(), cv.ShouldBeEmpty)
			cv.So(HasProblems(), cv.ShouldBeFalse)
		})

		cv.Convey("Add and List", func() {
			Add(Entry{
				Level:   LevelWarn,
				Source:  "migration",
				Message: "test warning",
				Detail:  "some detail",
			})
			entries := List()
			cv.So(entries, cv.ShouldHaveLength, 1)
			cv.So(entries[0].Level, cv.ShouldEqual, LevelWarn)
			cv.So(entries[0].Source, cv.ShouldEqual, "migration")
			cv.So(entries[0].Message, cv.ShouldEqual, "test warning")
			cv.So(entries[0].Detail, cv.ShouldEqual, "some detail")
			cv.So(entries[0].Time.IsZero(), cv.ShouldBeFalse)
		})

		cv.Convey("HasProblems", func() {
			cv.Convey("info only — no problems", func() {
				Add(Entry{Level: LevelInfo, Source: "test", Message: "info"})
				cv.So(HasProblems(), cv.ShouldBeFalse)
			})
			cv.Convey("warn — has problems", func() {
				Add(Entry{Level: LevelWarn, Source: "test", Message: "warn"})
				cv.So(HasProblems(), cv.ShouldBeTrue)
			})
			cv.Convey("error — has problems", func() {
				Add(Entry{Level: LevelError, Source: "test", Message: "err"})
				cv.So(HasProblems(), cv.ShouldBeTrue)
			})
		})

		cv.Convey("List returns a copy", func() {
			Add(Entry{Level: LevelWarn, Source: "test", Message: "a"})
			list := List()
			list[0].Message = "mutated"
			cv.So(List()[0].Message, cv.ShouldEqual, "a")
		})

		cv.Convey("Reset clears all entries", func() {
			Add(Entry{Level: LevelError, Source: "test", Message: "err"})
			cv.So(List(), cv.ShouldHaveLength, 1)
			Reset()
			cv.So(List(), cv.ShouldBeEmpty)
		})
	})
}
