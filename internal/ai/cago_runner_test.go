package ai

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cago-frame/agents/provider"
	"github.com/cago-frame/agents/provider/providertest"
	. "github.com/smartystreets/goconvey/convey"
)

// collectEvents 是 CagoRunner 的事件 sink，阻塞直到收到 "done" / "error" / "stopped" 之一或超时。
func collectEvents(t *testing.T, runner *CagoRunner, ctx context.Context, prompt string, msgs []Message, timeout time.Duration) []StreamEvent {
	t.Helper()
	var (
		events []StreamEvent
		mu     = make(chan struct{}, 1)
	)
	mu <- struct{}{}
	collect := func(ev StreamEvent) {
		<-mu
		events = append(events, ev)
		mu <- struct{}{}
	}

	if err := runner.Start(ctx, prompt, msgs, collect); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			<-mu
			done := false
			for _, e := range events {
				if e.Type == "done" || e.Type == "error" || e.Type == "stopped" {
					done = true
					break
				}
			}
			mu <- struct{}{}
			if done {
				// 等 goroutine 收尾
				for runner.State() != RunnerIdle {
					time.Sleep(10 * time.Millisecond)
				}
				return events
			}
		case <-deadline.C:
			t.Fatalf("runner 在 %s 内未结束；事件 = %+v", timeout, events)
		}
	}
}

func TestCagoRunner_SimpleTextResponse(t *testing.T) {
	Convey("纯文本回复路径：cago 流 → content + done", t, func() {
		mock := providertest.New().QueueStream(
			provider.StreamChunk{ContentDelta: "hello "},
			provider.StreamChunk{ContentDelta: "world"},
			provider.StreamChunk{FinishReason: provider.FinishStop, Usage: &provider.Usage{PromptTokens: 5, CompletionTokens: 2}},
		)

		runner := NewCagoRunner(CagoRunnerConfig{
			Provider: mock,
			Cwd:      t.TempDir(),
		})

		events := collectEvents(t, runner, context.Background(), "你是 OpsKat 助手。", []Message{
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

func TestCagoRunner_RejectsDoubleStart(t *testing.T) {
	Convey("二次 Start 拒绝", t, func() {
		// 这里给一个会卡住的 stream，让第一个 Start 一直处于 running，便于检查二次 Start
		mock := providertest.New().QueueStreamFunc(func(ctx context.Context) <-chan provider.StreamChunk {
			ch := make(chan provider.StreamChunk)
			go func() {
				<-ctx.Done() // 跟随 ctx 退出
				close(ch)
			}()
			return ch
		})

		runner := NewCagoRunner(CagoRunnerConfig{
			Provider: mock,
			Cwd:      t.TempDir(),
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := runner.Start(ctx, "", []Message{{Role: RoleUser, Content: "hi"}}, func(StreamEvent) {})
		So(err, ShouldBeNil)
		// 等 runner 真正进 Running
		for runner.State() != RunnerRunning {
			time.Sleep(5 * time.Millisecond)
		}

		err = runner.Start(ctx, "", []Message{{Role: RoleUser, Content: "again"}}, func(StreamEvent) {})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "already active")

		runner.Stop()
		So(runner.State(), ShouldEqual, RunnerIdle)
	})
}

func TestCagoRunner_StopCancelsRunningTurn(t *testing.T) {
	Convey("Stop 取消当前 turn，发出 stopped", t, func() {
		mock := providertest.New().QueueStreamFunc(func(ctx context.Context) <-chan provider.StreamChunk {
			ch := make(chan provider.StreamChunk)
			go func() {
				defer close(ch)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					return
				}
			}()
			return ch
		})

		runner := NewCagoRunner(CagoRunnerConfig{
			Provider: mock,
			Cwd:      t.TempDir(),
		})

		var events []StreamEvent
		var emu = make(chan struct{}, 1)
		emu <- struct{}{}
		emit := func(e StreamEvent) {
			<-emu
			events = append(events, e)
			emu <- struct{}{}
		}

		So(runner.Start(context.Background(), "", []Message{{Role: RoleUser, Content: "go"}}, emit), ShouldBeNil)
		for runner.State() != RunnerRunning {
			time.Sleep(5 * time.Millisecond)
		}
		runner.Stop()
		So(runner.State(), ShouldEqual, RunnerIdle)

		<-emu
		hasStopped := false
		for _, e := range events {
			if e.Type == "stopped" {
				hasStopped = true
				break
			}
		}
		emu <- struct{}{}
		So(hasStopped, ShouldBeTrue)
	})
}
