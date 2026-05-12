package ai

import (
	"testing"

	"github.com/opskat/opskat/pkg/extension"
	. "github.com/smartystreets/goconvey/convey"
)

// mockExtToolExecutor implements ExtensionToolExecutor for testing.
type mockExtToolExecutor struct {
	ext *extension.Extension
}

func (m *mockExtToolExecutor) FindExtensionByTool(extName, toolName string) *extension.Extension {
	return m.ext
}

func (m *mockExtToolExecutor) GetExtensionPolicyGroups(extName, assetType string, assetID int64) []string {
	return nil
}

// Compile-time interface check.
var _ ExtensionToolExecutor = (*mockExtToolExecutor)(nil)

func TestExecToolHandler(t *testing.T) {
	Convey("handleExecTool", t, func() {
		// Reset global state
		origExecutor := execToolExecutor
		t.Cleanup(func() { execToolExecutor = origExecutor })

		Convey("should return error when no executor configured", func() {
			execToolExecutor = nil
			_, err := handleExecTool(t.Context(), map[string]any{
				"extension": "nonexistent",
				"tool":      "some_tool",
				"args":      map[string]any{},
			})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")
		})

		Convey("should return error when missing extension arg", func() {
			_, err := handleExecTool(t.Context(), map[string]any{
				"tool": "some_tool",
				"args": map[string]any{},
			})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "extension")
		})

		Convey("should return error when missing tool arg", func() {
			_, err := handleExecTool(t.Context(), map[string]any{
				"extension": "oss",
				"args":      map[string]any{},
			})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "tool")
		})

		Convey("should return error when tool not found in extension", func() {
			execToolExecutor = &mockExtToolExecutor{ext: nil}
			_, err := handleExecTool(t.Context(), map[string]any{
				"extension": "oss",
				"tool":      "nonexistent",
				"args":      map[string]any{},
			})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")
			So(err.Error(), ShouldContainSubstring, "nonexistent")
		})
	})
}

func TestPromptBuilderExtensionSkillMD(t *testing.T) {
	Convey("PromptBuilder with extension SKILL.md", t, func() {
		builder := NewPromptBuilder("en", AIContext{})

		Convey("should not include extension content by default", func() {
			prompt := builder.Build()
			So(prompt, ShouldNotContainSubstring, "exec_tool")
		})

		Convey("should include SKILL.md when set", func() {
			builder.SetExtensionSkillMDs(map[string]string{
				"oss": "# OSS Tools\nUse exec_tool to call OSS tools.",
			})
			prompt := builder.Build()
			So(prompt, ShouldContainSubstring, "OSS Tools")
			So(prompt, ShouldContainSubstring, "exec_tool")
		})
	})
}
