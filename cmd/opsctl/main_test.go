package main

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseRemotePath(t *testing.T) {
	Convey("parseRemotePath", t, func() {
		Convey("should parse valid remote paths", func() {
			id, path := parseRemotePath("1:/etc/hosts")
			So(id, ShouldEqual, 1)
			So(path, ShouldEqual, "/etc/hosts")

			id, path = parseRemotePath("42:/var/log/app.log")
			So(id, ShouldEqual, 42)
			So(path, ShouldEqual, "/var/log/app.log")

			id, path = parseRemotePath("100:/tmp/file with spaces.txt")
			So(id, ShouldEqual, 100)
			So(path, ShouldEqual, "/tmp/file with spaces.txt")
		})

		Convey("should return 0 for local paths", func() {
			id, path := parseRemotePath("./local-file.txt")
			So(id, ShouldEqual, 0)
			So(path, ShouldEqual, "./local-file.txt")

			id, path = parseRemotePath("/absolute/path")
			So(id, ShouldEqual, 0)
			So(path, ShouldEqual, "/absolute/path")

			id, path = parseRemotePath("relative.txt")
			So(id, ShouldEqual, 0)
			So(path, ShouldEqual, "relative.txt")
		})

		Convey("should handle edge cases", func() {
			// Colon at start (no ID)
			id, path := parseRemotePath(":/path")
			So(id, ShouldEqual, 0)
			So(path, ShouldEqual, ":/path")

			// Non-numeric before colon
			id, path = parseRemotePath("abc:/path")
			So(id, ShouldEqual, 0)
			So(path, ShouldEqual, "abc:/path")

			// Empty string
			id, path = parseRemotePath("")
			So(id, ShouldEqual, 0)
			So(path, ShouldEqual, "")

			// Windows-like path (C:\path) should not parse as remote
			id, path = parseRemotePath("C:\\Users\\file")
			So(id, ShouldEqual, 0)
			So(path, ShouldEqual, "C:\\Users\\file")
		})
	})
}

func TestExtractCommand(t *testing.T) {
	Convey("extractCommand", t, func() {
		Convey("should extract command after --", func() {
			cmd := extractCommand([]string{"--", "uptime"})
			So(cmd, ShouldEqual, "uptime")

			cmd = extractCommand([]string{"--", "ls", "-la", "/var/log"})
			So(cmd, ShouldEqual, "ls -la /var/log")

			cmd = extractCommand([]string{"--", "cat", "/etc/hosts"})
			So(cmd, ShouldEqual, "cat /etc/hosts")
		})

		Convey("should join all args without --", func() {
			cmd := extractCommand([]string{"uptime"})
			So(cmd, ShouldEqual, "uptime")

			cmd = extractCommand([]string{"ls", "-la"})
			So(cmd, ShouldEqual, "ls -la")
		})

		Convey("should return empty for no command", func() {
			cmd := extractCommand([]string{})
			So(cmd, ShouldEqual, "")

			cmd = extractCommand([]string{"--"})
			So(cmd, ShouldEqual, "")
		})

		Convey("should use first -- only", func() {
			cmd := extractCommand([]string{"--", "echo", "--", "hello"})
			So(cmd, ShouldEqual, "echo -- hello")
		})
	})
}

func TestPlanSessionFlagParsing(t *testing.T) {
	Convey("--plan-session flag 解析", t, func() {
		Convey("有 --plan-session 时正确提取", func() {
			args := []string{"--plan-session", "abc-123", "web-server", "--", "uptime"}
			var planSession string
			remaining := args
			if len(remaining) >= 2 && remaining[0] == "--plan-session" {
				planSession = remaining[1]
				remaining = remaining[2:]
			}
			So(planSession, ShouldEqual, "abc-123")
			So(remaining, ShouldResemble, []string{"web-server", "--", "uptime"})
		})

		Convey("无 --plan-session 时不影响解析", func() {
			args := []string{"web-server", "--", "uptime"}
			var planSession string
			remaining := args
			if len(remaining) >= 2 && remaining[0] == "--plan-session" {
				planSession = remaining[1]
				remaining = remaining[2:]
			}
			So(planSession, ShouldEqual, "")
			So(remaining, ShouldResemble, []string{"web-server", "--", "uptime"})
		})
	})
}

func TestPlanInputParsing(t *testing.T) {
	Convey("plan JSON 输入解析", t, func() {
		Convey("有效 JSON", func() {
			input := `{"description":"test plan","items":[{"type":"exec","asset":"web-01","command":"uptime"}]}`
			var plan planInput
			err := json.Unmarshal([]byte(input), &plan)
			So(err, ShouldBeNil)
			So(plan.Description, ShouldEqual, "test plan")
			So(len(plan.Items), ShouldEqual, 1)
			So(plan.Items[0].Type, ShouldEqual, "exec")
			So(plan.Items[0].Asset, ShouldEqual, "web-01")
			So(plan.Items[0].Command, ShouldEqual, "uptime")
		})

		Convey("多项计划", func() {
			input := `{"description":"deploy","items":[
				{"type":"exec","asset":"web-01","command":"systemctl stop app"},
				{"type":"cp","asset":"web-01","detail":"upload config"},
				{"type":"exec","asset":"web-01","command":"systemctl start app"}
			]}`
			var plan planInput
			err := json.Unmarshal([]byte(input), &plan)
			So(err, ShouldBeNil)
			So(len(plan.Items), ShouldEqual, 3)
			So(plan.Items[0].Type, ShouldEqual, "exec")
			So(plan.Items[1].Type, ShouldEqual, "cp")
			So(plan.Items[2].Command, ShouldEqual, "systemctl start app")
		})

		Convey("空 items", func() {
			input := `{"description":"empty","items":[]}`
			var plan planInput
			err := json.Unmarshal([]byte(input), &plan)
			So(err, ShouldBeNil)
			So(len(plan.Items), ShouldEqual, 0)
		})
	})
}

func TestCpPathParsing(t *testing.T) {
	Convey("cp path classification", t, func() {
		Convey("upload: local -> remote", func() {
			srcID, _ := parseRemotePath("./file.txt")
			dstID, dstPath := parseRemotePath("1:/tmp/file.txt")
			So(srcID, ShouldEqual, 0)
			So(dstID, ShouldEqual, 1)
			So(dstPath, ShouldEqual, "/tmp/file.txt")
		})

		Convey("download: remote -> local", func() {
			srcID, srcPath := parseRemotePath("1:/tmp/file.txt")
			dstID, _ := parseRemotePath("./file.txt")
			So(srcID, ShouldEqual, 1)
			So(srcPath, ShouldEqual, "/tmp/file.txt")
			So(dstID, ShouldEqual, 0)
		})

		Convey("asset-to-asset: remote -> remote", func() {
			srcID, srcPath := parseRemotePath("1:/etc/config")
			dstID, dstPath := parseRemotePath("2:/tmp/config")
			So(srcID, ShouldEqual, 1)
			So(srcPath, ShouldEqual, "/etc/config")
			So(dstID, ShouldEqual, 2)
			So(dstPath, ShouldEqual, "/tmp/config")
		})

		Convey("error: local -> local", func() {
			srcID, _ := parseRemotePath("./a.txt")
			dstID, _ := parseRemotePath("./b.txt")
			So(srcID, ShouldEqual, 0)
			So(dstID, ShouldEqual, 0)
			// Both are local → should error in cmdCp
		})
	})
}
