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

// TestDefaultHostProviderActionCancel exercises the atomic plumbing between
// Plugin.CallAction (which installs a cancellation) and WASM polls via
// host.ActionShouldStop. Without this test the end-to-end cancel path is
// only indirectly exercised by the (skipped-by-default) e2e TCP test.
func TestDefaultHostProviderActionCancel(t *testing.T) {
	Convey("Given a DefaultHostProvider", t, func() {
		h := NewDefaultHostProvider(DefaultHostConfig{})

		Convey("With no cancellation installed, ActionShouldStop is false", func() {
			So(h.ActionShouldStop(), ShouldBeFalse)
		})

		Convey("After installing a cancellation", func() {
			c := NewActionCancellation()
			h.SetActiveCancellation(c)

			Convey("ActionShouldStop remains false until Cancel", func() {
				So(h.ActionShouldStop(), ShouldBeFalse)
			})
			Convey("After Cancel, ActionShouldStop becomes true", func() {
				c.Cancel()
				So(h.ActionShouldStop(), ShouldBeTrue)
			})
			Convey("Clearing the cancellation (nil) resets ShouldStop", func() {
				c.Cancel()
				So(h.ActionShouldStop(), ShouldBeTrue)
				h.SetActiveCancellation(nil)
				So(h.ActionShouldStop(), ShouldBeFalse)
			})
			Convey("Replacing with a fresh cancellation sees fresh state", func() {
				c.Cancel()
				So(h.ActionShouldStop(), ShouldBeTrue)
				h.SetActiveCancellation(NewActionCancellation())
				So(h.ActionShouldStop(), ShouldBeFalse)
			})
		})
	})
}
