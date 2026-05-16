package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cago-frame/agents/agent"
	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"
	"github.com/opskat/opskat/internal/model/entity/ai_provider_entity"
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

// TestRunner_ReplaysHistoryToLLM 验证回放的历史会真正进入 LLM 请求。
//
// 回归：ToAgentMessages 早期用 &agent.TextBlock{} 之类的指针，cago 的
// BuildRequest 用值类型 type switch（case TextBlock:），指针不匹配会被静默
// 丢弃，导致 LLM 端看不到任何历史。
func TestRunner_ReplaysHistoryToLLM(t *testing.T) {
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

// TestRunner_SystemPromptHasOpsKatIntro 验证实际发出的 system message
// 用的是 OpsKat 模板，而不是 cago 默认 "lead Cago coding agent" 那段。
// 这是 WithSystemTemplate(opskatSystemTemplate) 接线的端到端断言。
func TestRunner_SystemPromptHasOpsKatIntro(t *testing.T) {
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

// TestRunner_RetriesOn503ProviderError 验证 BuildSystem 注入的 agent.RetryPolicy
// 在面对 503 时确实走 cago retry 路径：第一次 ChatStream chunk.Err 命中
// defaultShouldRetry 的 408/425/429/500/502/503/504 白名单 → handleRetry 发出
// EventRetry（被翻译为前端 type:"retry" StreamEvent）+ sleep InitialDelay →
// 第二次 ChatStream 正常吐出 content + finish_reason → 整轮成功。
//
// 这是 OpsKat 端到端的回归 —— 当 cago 升级 / 配置漂移导致 RetryPolicy 失效时
// 该测试会先于线上挂掉。
func TestRunner_RetriesOn503ProviderError(t *testing.T) {
	Convey("503 ProviderError 触发 cago retry → 收到 retry 事件 + 续传完成", t, func() {
		// 两个 QueueStream entry：FIFO 消费，第一次 503 触发 retry，第二次正常返回。
		mock := providertest.New().
			QueueStream(provider.StreamChunk{Err: &provider.ProviderError{
				Err:        errors.New("503 service unavailable"),
				StatusCode: 503,
			}}).
			QueueStream(
				provider.StreamChunk{ContentDelta: "recovered"},
				provider.StreamChunk{FinishReason: provider.FinishStop},
			)

		// timeout 大于 InitialDelay(1s) 即可。
		out := runOneTurn(t, mock, "", []Message{
			{Role: RoleUser, Content: "hello"},
		}, 8*time.Second)

		var retryCount, contentCount, doneCount, errorCount int
		var retryEv *StreamEvent
		for i := range out {
			e := &out[i]
			switch e.Type {
			case "retry":
				retryCount++
				retryEv = e
			case "content":
				contentCount++
			case "done":
				doneCount++
			case "error":
				errorCount++
			}
		}
		So(retryCount, ShouldEqual, 1)
		So(retryEv, ShouldNotBeNil)
		// Attempt 序号放在 Content 字段；RetryDelayMs 透传 cago Delay。
		So(retryEv.Content, ShouldEqual, "1")
		So(retryEv.RetryDelayMs, ShouldBeGreaterThan, 0)
		So(retryEv.Error, ShouldContainSubstring, "503")
		So(contentCount, ShouldBeGreaterThanOrEqualTo, 1)
		So(doneCount, ShouldEqual, 1)
		So(errorCount, ShouldEqual, 0)
		// ChatStream 被调用 2 次（首次失败 + 重试）。
		So(len(mock.Received()), ShouldEqual, 2)
	})
}

// TestRunner_EndToEnd_RealOpenAIProvider_503TriggersRetry 是更彻底的端到端回归：
// 起一个真 httptest server，前 N 次返 503，最后一次正常返回 SSE 流。
// 走的链路完全是生产路径：BuildProvider(*openai.AIProvider, apiKey) →
// cago openai.NewProvider → BuildSystem (注入 RetryPolicy) → Runner.Send。
//
// 这条路径覆盖的关键 bug:
//   - cago openai provider 必须把 *openai.APIError 包装成 *provider.ProviderError
//     (status_code 走 defaultShouldRetry 白名单)
//   - 如果包装失效，503 直接走 EventError，前端只看到 ErrorBlock 没有 RetryBanner
//
// 跟 TestRunner_RetriesOn503ProviderError 的区别：那个测试用 providertest
// 直接注入 chunk.Err{ProviderError}，跳过了 OpenAI provider 的 HTTP 错误转换层。
func TestRunner_EndToEnd_RealOpenAIProvider_503TriggersRetry(t *testing.T) {
	Convey("真 OpenAI provider + 503 HTTP → cago retry 链路", t, func() {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			n := hits.Add(1)
			if n < 2 {
				// 第一次返 503，模拟用户截图里的错误。
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"error":{"message":"No available channel for model gpt-5.51"}}`)
				return
			}
			// 第二次返一段正常 SSE 流。
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			frames := []string{
				`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"recovered"}}]}`,
				`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			}
			for _, f := range frames {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", f)
				flusher.Flush()
			}
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
		defer srv.Close()

		// 走生产路径：BuildProvider 从 entity 构造 cago openai provider。
		entity := &ai_provider_entity.AIProvider{
			Type:    "openai",
			APIBase: srv.URL,
			Model:   "gpt-4o",
		}
		prov, err := BuildProvider(entity, "test-key")
		So(err, ShouldBeNil)

		cfg := SystemConfig{
			Provider: prov,
			Cwd:      t.TempDir(),
			Model:    "gpt-4o",
		}
		sys, err := BuildSystem(context.Background(), cfg)
		So(err, ShouldBeNil)
		defer func() { _ = sys.Close(context.Background()) }()

		conv := agent.LoadConversation(fmt.Sprintf("e2e-%d", time.Now().UnixNano()), nil)
		runner := sys.Agent().Runner(conv)
		defer func() { _ = runner.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		events, err := runner.Send(ctx, "hello")
		So(err, ShouldBeNil)

		var retryCount, contentCount, doneCount, errorCount int
		var retryEv *StreamEvent
		translator := NewStreamTranslator()
		for ev := range events {
			translator.Translate(ev, func(se StreamEvent) {
				switch se.Type {
				case "retry":
					retryCount++
					sc := se
					retryEv = &sc
				case "content":
					contentCount++
				case "done":
					doneCount++
				case "error":
					errorCount++
				}
			})
		}

		// 第一次 503 必须命中 cago retry：
		//   - 后端 emit 一次 EventRetry → 前端 type:"retry" StreamEvent
		//   - RetryDelayMs 应该 > 0 (cago InitialDelay=1s)
		//   - Error 文本包含 503 状态码
		So(retryCount, ShouldEqual, 1)
		So(retryEv, ShouldNotBeNil)
		So(retryEv.RetryDelayMs, ShouldBeGreaterThan, 0)
		So(retryEv.Error, ShouldContainSubstring, "503")
		// 第二次正常返回 → content + done，没有 error。
		So(contentCount, ShouldBeGreaterThanOrEqualTo, 1)
		So(doneCount, ShouldEqual, 1)
		So(errorCount, ShouldEqual, 0)
		// 服务器被命中 2 次。
		So(hits.Load(), ShouldEqual, int32(2))
	})
}

func TestRunner_ProviderEntityReasoningConfig(t *testing.T) {
	Convey("ProviderEntity 的 reasoning 设置会注入 cago 请求", t, func() {
		mock := providertest.New().QueueStream(
			provider.StreamChunk{ContentDelta: "ok"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)

		cfg := SystemConfig{
			Provider: mock,
			ProviderEntity: &ai_provider_entity.AIProvider{
				Type:             "openai",
				Model:            "deepseek-v4-pro",
				ReasoningEnabled: true,
				ReasoningEffort:  "high",
			},
			Cwd: t.TempDir(),
		}
		sys, err := BuildSystem(context.Background(), cfg)
		So(err, ShouldBeNil)
		t.Cleanup(func() { _ = sys.Close(context.Background()) })

		runner := sys.Agent().Runner(agent.NewConversation())
		t.Cleanup(func() { _ = runner.Close() })
		events, err := runner.Send(context.Background(), "hi")
		So(err, ShouldBeNil)
		for range events {
		}

		recv := mock.Received()
		So(len(recv), ShouldEqual, 1)
		So(recv[0].Thinking, ShouldNotBeNil)
		So(recv[0].Thinking.Effort, ShouldEqual, provider.ThinkingHigh)
	})
}

func TestRunner_SimpleTextResponse(t *testing.T) {
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

// TestBuildGeneralPurposeTools_AllLocalToolsPrefixed 跑真 cago 构造器，断言 GP subagent
// 工具集里本地 7 件套全部带 local_ 前缀，且没有任何裸名 bash/write/edit/read/grep/find/ls 漏网。
//
// 同时覆盖两条回归：
//   - WrapLocalTool 依赖 cago 内置工具是 *tool.RawTool。若 cago 升级换成别的 Tool
//     实现，decorator 会静默 pass-through，本测试用真 cago bash.New/grep.New 等构造器
//     而非合成 RawTool，能抓到此类依赖漂移。
//   - 早期实现把 grep/find/ls 追加在 WrapLocalTool 循环之后，导致子 agent 看到的是
//     裸 grep/find/ls 而不是 local_*。本测试断言全部 7 个名字都已带前缀。
func TestBuildGeneralPurposeTools_AllLocalToolsPrefixed(t *testing.T) {
	Convey("GP subagent 工具集里 7 件本地工具全部 local_ 前缀，无裸名漏网", t, func() {
		tools := buildGeneralPurposeTools(t.TempDir())
		names := make(map[string]bool, len(tools))
		for _, tl := range tools {
			names[tl.Name()] = true
		}

		// 7 件套必须全部以 local_ 前缀出现
		for _, want := range []string{
			"local_bash", "local_write", "local_edit",
			"local_read", "local_grep", "local_find", "local_ls",
		} {
			So(names[want], ShouldBeTrue)
		}

		// 裸名一个都不能有
		for _, banned := range []string{
			"bash", "write", "edit", "read", "grep", "find", "ls",
		} {
			So(names[banned], ShouldBeFalse)
		}

		// 不在 localRenames 表里的本地工具保持原名（cago runtime 文案依赖）
		So(names["bash_output"], ShouldBeTrue)
		So(names["kill_shell"], ShouldBeTrue)
	})
}

// 回归：subagent 调出的 general-purpose 子 agent 工具集含
// local_bash/local_write/local_edit（cago 默认 bash/write/edit 经 WrapLocalTool 改名）。
// 旧实现只把 LocalToolGate 挂在父 agent 上，子 agent 调 local_bash 时绕过审批。
// 这里通过 providertest 串起一条 parent → subagent → child local_bash 的端到端流，
// 断言 LocalToolGate.confirm 一定被触发。
func TestRunner_GPSubagentInheritsLocalToolGate(t *testing.T) {
	Convey("subagent 调出的 general-purpose 子 agent 调 local_bash 时也走 LocalToolGate", t, func() {
		var confirmCalls int32
		var seenTool, seenCmd string
		gate := NewLocalToolGate(func(_ context.Context, req LocalToolApprovalRequest) ApprovalResponse {
			atomic.AddInt32(&confirmCalls, 1)
			seenTool = req.ToolName
			seenCmd = req.Command
			return ApprovalResponse{Decision: "deny"}
		})

		mock := providertest.New()
		// 1) 父 agent: subagent → general-purpose
		mock.QueueStream(
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "d1", Name: "subagent"}},
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ArgsDelta: `{"type":"general-purpose","prompt":"run echo"}`}},
			provider.StreamChunk{FinishReason: provider.FinishToolCalls},
		)
		// 2) 子 agent: local_bash 调用 —— 期望被 gate 拦截
		mock.QueueStream(
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "b1", Name: "local_bash"}},
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ArgsDelta: `{"command":"echo hi"}`}},
			provider.StreamChunk{FinishReason: provider.FinishToolCalls},
		)
		// 3) 子 agent: 看到 deny 后总结收尾
		mock.QueueStream(
			provider.StreamChunk{ContentDelta: "denied"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)
		// 4) 父 agent: 拿到 subagent result 后收尾
		mock.QueueStream(
			provider.StreamChunk{ContentDelta: "ok"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)

		cfg := SystemConfig{
			Provider:      mock,
			Cwd:           t.TempDir(),
			LocalToolGate: gate,
		}
		sys, err := BuildSystem(context.Background(), cfg)
		So(err, ShouldBeNil)
		defer func() { _ = sys.Close(context.Background()) }()

		conv := agent.LoadConversation("opskat-gp-gate-test", nil)
		runner := sys.Agent().Runner(conv)
		defer func() { _ = runner.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		events, err := runner.Send(ctx, "dispatch please")
		So(err, ShouldBeNil)
		for range events { // drain
		}

		So(atomic.LoadInt32(&confirmCalls), ShouldEqual, 1)
		So(seenTool, ShouldEqual, "local_bash")
		So(seenCmd, ShouldEqual, "echo hi")
	})
}

func TestRunner_CancelEmitsStopped(t *testing.T) {
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

// queueParentDispatch 把"父 agent 调 subagent(type=explore, prompt=go)"
// 那一步流压进 mock 队列。providertest 是 FIFO 共享队列，调用方需自己按
// 父→子→...→子→父 的顺序把各步压进去（不能用 defer 把父收尾流后置，
// 否则 child 会拉到 "ok" 那条）。
func queueParentDispatch(mock *providertest.Mock, toolUseID string) {
	mock.QueueStream(
		provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: toolUseID, Name: "subagent"}},
		provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ArgsDelta: `{"type":"explore","prompt":"go"}`}},
		provider.StreamChunk{FinishReason: provider.FinishToolCalls},
	)
}

// queueParentClose 父 agent 拿到 subagent tool 结果后的收尾流（一段 text + FinishStop）。
func queueParentClose(mock *providertest.Mock, text string) {
	mock.QueueStream(
		provider.StreamChunk{ContentDelta: text},
		provider.StreamChunk{FinishReason: provider.FinishStop},
	)
}

// captureSubagentResultText 跑一遍 parent.Runner，抓 subagent tool result 的文本。
func captureSubagentResultText(t *testing.T, mock provider.Provider, toolUseID string) string {
	t.Helper()
	cfg := SystemConfig{Provider: mock, Cwd: t.TempDir()}
	sys, err := BuildSystem(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BuildSystem: %v", err)
	}
	t.Cleanup(func() { _ = sys.Close(context.Background()) })

	conv := agent.LoadConversation(fmt.Sprintf("opskat-subagent-test-%d", time.Now().UnixNano()), nil)
	runner := sys.Agent().Runner(conv)
	t.Cleanup(func() { _ = runner.Close() })

	var mu sync.Mutex
	var captured *agent.ToolResultBlock
	unsub := runner.OnEvent(agent.OnlyKinds(agent.EventPostToolUse), func(_ context.Context, ev agent.Event) {
		if ev.Tool == nil || ev.Tool.ToolUseID != toolUseID {
			return
		}
		mu.Lock()
		captured = ev.Tool.Output
		mu.Unlock()
	})
	defer unsub()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := runner.Send(ctx, "explore please")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	for range events { // drain
	}

	mu.Lock()
	rb := captured
	mu.Unlock()
	return toolResultText(rb)
}

// toolResultText 抽出 *ToolResultBlock 里的 TextBlock 文本。
func toolResultText(rb *agent.ToolResultBlock) string {
	if rb == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range rb.Content {
		switch v := c.(type) {
		case agent.TextBlock:
			b.WriteString(v.Text)
		case *agent.TextBlock:
			if v != nil {
				b.WriteString(v.Text)
			}
		}
	}
	return b.String()
}

// TestRunner_SubagentExplore_TextPath：sanity——子 agent 正常出文本时，
// 父侧 subagent tool 结果就是子 assistant 的文本。
// 防 opskat audit middleware / GP 替换之类的接线把正常路径搞坏。
func TestRunner_SubagentExplore_TextPath(t *testing.T) {
	Convey("explore 子 agent 出文本时，父 tool 结果原文回传", t, func() {
		mock := providertest.New()
		// 父 step 1: dispatch
		queueParentDispatch(mock, "tx1")
		// 子: 直接产文本收尾
		mock.QueueStream(
			provider.StreamChunk{ContentDelta: "found three config files"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)
		// 父 step 2: close
		queueParentClose(mock, "ok")

		text := captureSubagentResultText(t, mock, "tx1")
		So(text, ShouldEqual, "found three config files")
	})
}

// TestRunner_SubagentExplore_ThinkingOnlyFallback：端到端验证 cago 那侧的
// thinking 回退 fix 通过 opskat BuildSystem（含 audit middleware、GP 替换、
// LocalToolGate 中间件链）依然生效。
//
// 如果再看到 "sub-agent returned no content"，说明：
//   - cago 改没生效（旧二进制 / replace 没指对）→ 这条 test 会红
//   - 或 opskat 加的中间件吃了 thinking 块
func TestRunner_SubagentExplore_ThinkingOnlyFallback(t *testing.T) {
	Convey("explore 子 agent 只产 thinking 时，父 tool 结果落到 thinking 文本", t, func() {
		mock := providertest.New()
		queueParentDispatch(mock, "tt1")
		// 子: 全程 thinking，无 ContentDelta
		mock.QueueStream(
			provider.StreamChunk{ThinkingDelta: &provider.ThinkingDelta{Text: "looked at ~/.opskat — three configs found"}},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)
		queueParentClose(mock, "ok")

		text := captureSubagentResultText(t, mock, "tt1")
		So(text, ShouldNotEqual, "sub-agent returned no content")
		So(text, ShouldContainSubstring, "three configs found")
	})
}

// TestRunner_SubagentExplore_ChildInheritsParentModel：核心猜测——
// 用户那边 explore 子 agent 调出去看似"没产内容"，真实根因可能是 child
// 的请求 Model 字段为空：opskat 把 cfg.Model 透给 coding.WithModel 只设了
// **父** agent 的 model，cago 的 Explore/Plan/GP 默认 entry 在不显式
// SubagentWithModel 时不会继承父 model → 子 agent 请求里 Model=""，
// openai/anthropic 这类 API 直接 400 → cago 流出 chunk.Err → conv 空 →
// runChild 默认分支返回 "sub-agent returned no content"，真实错误被吞。
//
// 这条 test 抓的是"child request 是否带上了 parent 的 model"，绿了说明
// 子 agent 跑的是和父相同的模型，红了就是上面这条假设成真。
func TestRunner_SubagentExplore_ChildInheritsParentModel(t *testing.T) {
	Convey("子 explore agent 的请求里 Model 字段必须与父 agent 一致", t, func() {
		mock := providertest.New()
		queueParentDispatch(mock, "im1")
		mock.QueueStream(
			provider.StreamChunk{ContentDelta: "child ok"},
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)
		queueParentClose(mock, "done")

		cfg := SystemConfig{Provider: mock, Cwd: t.TempDir(), Model: "test-model"}
		sys, err := BuildSystem(context.Background(), cfg)
		So(err, ShouldBeNil)
		t.Cleanup(func() { _ = sys.Close(context.Background()) })

		conv := agent.LoadConversation(fmt.Sprintf("opskat-im-%d", time.Now().UnixNano()), nil)
		runner := sys.Agent().Runner(conv)
		t.Cleanup(func() { _ = runner.Close() })

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		events, err := runner.Send(ctx, "explore please")
		So(err, ShouldBeNil)
		for range events { // drain
		}

		recv := mock.Received()
		// parent step1 (dispatch) → child step1 (text) → parent step2 (close)
		So(len(recv), ShouldEqual, 3)

		// 第二条请求是 child 的（subagent dispatch 出去后立刻发的）。
		childReq := recv[1]
		So(childReq.Model, ShouldEqual, "test-model")
	})
}

// TestRunner_SubagentExplore_StreamErrorSurfacesAsToolError：用户那条
// "no content" 的真实根因——子 agent provider stream 出错（API 403 / 限流 /
// 网络 / 协议异常等）时，cago 的 subagent.runChild 之前会兜底成
// "sub-agent returned no content"，把真错吞掉，父 agent 完全看不到。
//
// 修复后行为：tool 结果 IsError=true，文本里带原始错误，父 agent 拿到的是
// 正常的 tool error 信号，可以判断是否重试/转人工。
func TestRunner_SubagentExplore_StreamErrorSurfacesAsToolError(t *testing.T) {
	Convey("child stream 错误必须冒泡成 tool error，而不是被吞成 no content", t, func() {
		mock := providertest.New()
		queueParentDispatch(mock, "es1")
		// 子 agent: stream 第一个 chunk 直接报错
		mock.QueueStream(
			provider.StreamChunk{Err: errors.New("upstream 429 rate limit")},
		)
		queueParentClose(mock, "done")

		cfg := SystemConfig{Provider: mock, Cwd: t.TempDir(), Model: "m"}
		sys, err := BuildSystem(context.Background(), cfg)
		So(err, ShouldBeNil)
		t.Cleanup(func() { _ = sys.Close(context.Background()) })

		conv := agent.LoadConversation(fmt.Sprintf("opskat-es-%d", time.Now().UnixNano()), nil)
		runner := sys.Agent().Runner(conv)
		t.Cleanup(func() { _ = runner.Close() })

		var mu sync.Mutex
		var rb *agent.ToolResultBlock
		unsub := runner.OnEvent(agent.OnlyKinds(agent.EventPostToolUse), func(_ context.Context, ev agent.Event) {
			if ev.Tool == nil || ev.Tool.ToolUseID != "es1" {
				return
			}
			mu.Lock()
			rb = ev.Tool.Output
			mu.Unlock()
		})
		defer unsub()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		events, err := runner.Send(ctx, "explore please")
		So(err, ShouldBeNil)
		for range events { // drain
		}

		mu.Lock()
		got := rb
		mu.Unlock()
		So(got, ShouldNotBeNil)
		So(got.IsError, ShouldBeTrue)
		text := toolResultText(got)
		So(text, ShouldContainSubstring, "sub-agent error")
		So(text, ShouldContainSubstring, "429 rate limit")
		So(text, ShouldNotContainSubstring, "sub-agent returned no content")
	})
}

// TestRunner_SubagentExplore_NoContentRegression：场景 A 兜底——子 agent
// 从头到尾只调工具不出任何文本/思考时，按之前讨论保留 "sub-agent returned
// no content" 的行为，作为回归保护。改 fallback 语义时这条会红，强制重新
// 评估。
func TestRunner_SubagentExplore_NoContentRegression(t *testing.T) {
	Convey("explore 子 agent 全程无文本无思考时，保留 'no content' 兜底", t, func() {
		mock := providertest.New()
		queueParentDispatch(mock, "ta1")
		// 子 step 1: 出一个 ls 工具调用（ReadOnly 工具集里有 ls，cwd=tempdir，
		// 默认 path="." 会列出空目录——不会报错）
		mock.QueueStream(
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ID: "ls1", Name: "ls"}},
			provider.StreamChunk{ToolCallDelta: &provider.ToolCallDelta{Index: 0, ArgsDelta: `{}`}},
			provider.StreamChunk{FinishReason: provider.FinishToolCalls},
		)
		// 子 step 2: 啥也不产，直接 FinishStop
		mock.QueueStream(
			provider.StreamChunk{FinishReason: provider.FinishStop},
		)
		queueParentClose(mock, "ok")

		text := captureSubagentResultText(t, mock, "ta1")
		So(text, ShouldEqual, "sub-agent returned no content")
	})
}
