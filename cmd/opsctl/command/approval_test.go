package command

import (
	"testing"

	"github.com/opskat/opskat/internal/ai"

	. "github.com/smartystreets/goconvey/convey"
)

// CheckPermission delegation is tested in internal/ai/permission_test.go.
// matchGrantItem was removed — grant matching is now unified inside CheckPermission.

func TestFormatOfflineDenyMessage(t *testing.T) {
	Convey("formatOfflineDenyMessage", t, func() {
		Convey("exec with hints", func() {
			msg := formatOfflineDenyMessage("exec", "systemctl restart nginx", []string{"ls *", "systemctl status *"})
			So(msg, ShouldContainSubstring, "desktop app is not running")
			So(msg, ShouldContainSubstring, "command did not match")
			So(msg, ShouldContainSubstring, "Command: systemctl restart nginx")
			So(msg, ShouldContainSubstring, "Allowed commands")
			So(msg, ShouldContainSubstring, "ls *")
			So(msg, ShouldContainSubstring, "systemctl status *")
			So(msg, ShouldContainSubstring, "Please adjust")
		})

		Convey("sql with hints", func() {
			msg := formatOfflineDenyMessage("sql", "INSERT INTO users VALUES (1)", []string{"SELECT", "SHOW"})
			So(msg, ShouldContainSubstring, "SQL statement did not match")
			So(msg, ShouldContainSubstring, "SQL: INSERT INTO users VALUES (1)")
			So(msg, ShouldContainSubstring, "Allowed SQL types")
			So(msg, ShouldContainSubstring, "SELECT")
			So(msg, ShouldContainSubstring, "SHOW")
		})

		Convey("redis with hints", func() {
			msg := formatOfflineDenyMessage("redis", "SET key val", []string{"GET *", "HGETALL *"})
			So(msg, ShouldContainSubstring, "Redis command did not match")
			So(msg, ShouldContainSubstring, "Redis command: SET key val")
			So(msg, ShouldContainSubstring, "Allowed Redis commands")
			So(msg, ShouldContainSubstring, "GET *")
			So(msg, ShouldContainSubstring, "HGETALL *")
		})

		Convey("exec without hints", func() {
			msg := formatOfflineDenyMessage("exec", "rm -rf /", nil)
			So(msg, ShouldContainSubstring, "desktop app is not running")
			So(msg, ShouldContainSubstring, "command did not match")
			So(msg, ShouldNotContainSubstring, "Allowed commands")
			So(msg, ShouldContainSubstring, "Please adjust")
		})

		Convey("empty hints slice", func() {
			msg := formatOfflineDenyMessage("exec", "ls", []string{})
			So(msg, ShouldNotContainSubstring, "Allowed commands")
		})
	})
}

func TestApprovalResultToCheckResult(t *testing.T) {
	Convey("ApprovalResult.ToCheckResult", t, func() {
		ar := ApprovalResult{
			Decision:       ai.Allow,
			DecisionSource: ai.SourcePolicyAllow,
			MatchedPattern: "ls *",
			SessionID:      "sess-123",
		}
		cr := ar.ToCheckResult()
		So(cr.Decision, ShouldEqual, ai.Allow)
		So(cr.DecisionSource, ShouldEqual, ai.SourcePolicyAllow)
		So(cr.MatchedPattern, ShouldEqual, "ls *")
	})
}
