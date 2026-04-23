package extension

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestActionCancellation(t *testing.T) {
	Convey("Given a new cancellation", t, func() {
		c := NewActionCancellation()
		Convey("ShouldStop is initially false", func() {
			So(c.ShouldStop(), ShouldBeFalse)
		})
		Convey("After Cancel, ShouldStop is true", func() {
			c.Cancel()
			So(c.ShouldStop(), ShouldBeTrue)
		})
		Convey("Cancel is idempotent", func() {
			c.Cancel()
			c.Cancel()
			So(c.ShouldStop(), ShouldBeTrue)
		})
	})
}
