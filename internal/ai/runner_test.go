package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"
	. "github.com/smartystreets/goconvey/convey"
)

// runOneTurn 用 BuildSystem 直接拉一个 cago Runner，按 messages 注入历史 + 发末尾 user
// 文本，把翻译后的 StreamEvent 串聚回数组。
func runOneTurn(t *testing.T, mock provider.Provider, systemPrompt string, messages []Message, timeout time.Duration) []StreamEvent {
	t.Helper()
	cfg := SystemConfig{
		Provider:     mock,
		Cwd:          t.TempDir(),
		SystemPrompt: systemPrompt,
	}
	sys, err := BuildSystem(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BuildSystem: %v", err)
	}
	t.Cleanup(func() { _ = sys.Close(context.Background()) })

	history, lastUserText := SplitForReplay(messages)
	conv := agent.LoadConversation(fmt.Sprintf("opskat-test-%d", time.Now().UnixNano()), ToAgentMessages(history))
	runner := sys.Agent().Runner(conv)
	t.Cleanup(func() { _ = runner.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	events, err := runner.Send(ctx, lastUserText)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var out []StreamEvent
	translator := NewStreamTranslator()
	for ev := range events {
		translator.Translate(ev, func(se StreamEvent) {
			out = append(out, se)
		})
	}
	return out
}

// TestCagoRunner_ReplaysHistoryToLLM 验证回放的历史会真正进入 LLM 请求。
//
// 回归：ToAgentMessages 早期用 &agent.TextBlock{} 之类的指针，cago 的
// BuildRequest 用值类型 type switch（case TextBlock:），指针不匹配会被静默
// 丢弃，导致 LLM 端看不到任何历史。
func TestCagoRunner_ReplaysHistoryToLLM(t *testing.T) {
	Convey("history 必须出现在 LLM 请求里", t, func() {
		mock := providertest.New().QueueStream(
			provider.StreamChunk{ContentDelta: "ok"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)

		_ = runOneTurn(t, mock, "system prompt", []Message{
			{Role: RoleUser, Content: "first question"},
			{Role: RoleAssistant, Content: "first answer"},
			{Role: RoleUser, Content: "what did I just say"},
		}, 5*time.Second)

		recv := mock.Received()
		So(len(recv), ShouldEqual, 1)
		req := recv[0]

		var nonSys []provider.Message
		for _, m := range req.Messages {
			if m.Role == provider.RoleSystem {
				continue
			}
			nonSys = append(nonSys, m)
		}
		So(nonSys, ShouldHaveLength, 3)
		So(nonSys[0].Role, ShouldEqual, provider.RoleUser)
		So(nonSys[0].Content, ShouldEqual, "first question")
		So(nonSys[1].Role, ShouldEqual, provider.RoleAssistant)
		So(nonSys[1].Content, ShouldEqual, "first answer")
		So(nonSys[2].Role, ShouldEqual, provider.RoleUser)
		So(nonSys[2].Content, ShouldEqual, "what did I just say")
	})
}

// TestCagoRunner_SystemPromptHasOpsKatIntro 验证实际发出的 system message
// 用的是 OpsKat 模板，而不是 cago 默认 "lead Cago coding agent" 那段。
// 这是 WithSystemTemplate(opskatSystemTemplate) 接线的端到端断言。
func TestCagoRunner_SystemPromptHasOpsKatIntro(t *testing.T) {
	Convey("system prompt 开头是 OpsKat 身份，不是 cago 默认 intro", t, func() {
		mock := providertest.New().QueueStream(
			provider.StreamChunk{ContentDelta: "ok"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)

		_ = runOneTurn(t, mock, "", []Message{
			{Role: RoleUser, Content: "hi"},
		}, 5*time.Second)

		recv := mock.Received()
		So(len(recv), ShouldEqual, 1)

		var sys strings.Builder
		for _, m := range recv[0].Messages {
			if m.Role == provider.RoleSystem {
				sys.WriteString(m.Content)
			}
		}
		text := sys.String()
		So(text, ShouldContainSubstring, "OpsKat AI assistant")
		So(text, ShouldNotContainSubstring, "lead Cago coding agent")
		So(text, ShouldContainSubstring, "## Available tools")
		So(text, ShouldContainSubstring, "## Guidelines")
	})
}

func TestCagoRunner_SimpleTextResponse(t *testing.T) {
	Convey("纯文本回复路径：cago 流 → content + done", t, func() {
		mock := providertest.New().QueueStream(
			provider.StreamChunk{ContentDelta: "hello "},
			provider.StreamChunk{ContentDelta: "world"},
			provider.StreamChunk{FinishReason: provider.FinishStop, Usage: &provider.Usage{PromptTokens: 5, CompletionTokens: 2}},
		)

		events := runOneTurn(t, mock, "你是 OpsKat 助手。", []Message{
			{Role: RoleUser, Content: "say hi"},
		}, 5*time.Second)

		var (
			content strings.Builder
			hasDone bool
			hasUsg  bool
		)
		for _, e := range events {
			switch e.Type {
			case "content":
				content.WriteString(e.Content)
			case "done":
				hasDone = true
			case "usage":
				hasUsg = true
				So(e.Usage.InputTokens, ShouldEqual, 5)
				So(e.Usage.OutputTokens, ShouldEqual, 2)
			}
		}
		So(content.String(), ShouldEqual, "hello world")
		So(hasDone, ShouldBeTrue)
		So(hasUsg, ShouldBeTrue)
	})
}

func TestCagoRunner_CancelEmitsStopped(t *testing.T) {
	Convey("Runner.Cancel 后翻译出 stopped 事件", t, func() {
		mock := providertest.New().QueueStreamFunc(func(ctx context.Context) <-chan provider.StreamChunk {
			ch := make(chan provider.StreamChunk)
			go func() {
				defer close(ch)
				select {
				case <-ctx.Done():
				case <-time.After(5 * time.Second):
				}
			}()
			return ch
		})

		cfg := SystemConfig{Provider: mock, Cwd: t.TempDir()}
		sys, err := BuildSystem(context.Background(), cfg)
		So(err, ShouldBeNil)
		defer func() { _ = sys.Close(context.Background()) }()

		conv := agent.LoadConversation("opskat-cancel-test", nil)
		runner := sys.Agent().Runner(conv)
		defer func() { _ = runner.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		events, err := runner.Send(ctx, "go")
		So(err, ShouldBeNil)

		// 等到 turn 真正在跑（至少一次进入 stream 处理），然后 Cancel
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = runner.Cancel("user_stop")
		}()

		var seenStopped bool
		translator := NewStreamTranslator()
		for ev := range events {
			translator.Translate(ev, func(se StreamEvent) {
				if se.Type == "stopped" {
					seenStopped = true
				}
			})
		}
		So(seenStopped, ShouldBeTrue)
	})
}
