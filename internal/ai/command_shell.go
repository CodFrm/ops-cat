package ai

import (
	"fmt"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"mvdan.cc/sh/v3/syntax"
)

// --- Shell AST 解析 ---

// ExtractSubCommands 从 shell 命令中提取所有可执行子命令。
//
// 处理：
//   - `&&` `||` `;` `|` 等 BinaryCmd 拆分
//   - CallExpr：Args 作为命令本体；Assigns 的右值仍要扫，因为 `PAYLOAD=$(rm -rf /) echo`
//     里的 CmdSubst 会先于命令执行
//   - Stmt.Redirs：操作符本身忽略，但目标 word 仍要扫，因为 `> $(rm -rf /)` 会先求值
//   - `$()`、反引号、`<(...)`/`>(...)` 进程替换、`${VAR:-$(...)}` 参数展开默认值都会执行
//   - 双引号内的 `$()` 同样递归；单引号内不展开
//   - 其他控制结构（Subshell、Block、IfClause、ForClause…）走 Walk 兜底
func ExtractSubCommands(command string) ([]string, error) {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, fmt.Errorf("shell parse failed: %w", err)
	}

	var cmds []string
	printer := syntax.NewPrinter()

	var extractFromStmt func(stmt *syntax.Stmt)
	var extractFromWord func(w *syntax.Word)

	// 通用 word 扫描：递归到 part，发现 CmdSubst/ProcSubst 都把内部 stmts 提取出来；
	// DblQuoted、ParamExp 的 default value 也要继续往下走。
	extractFromWord = func(w *syntax.Word) {
		if w == nil {
			return
		}
		for _, part := range w.Parts {
			extractFromWordPart(part, extractFromStmt, extractFromWord)
		}
	}

	printWords := func(words []*syntax.Word) string {
		var buf strings.Builder
		for i, w := range words {
			if i > 0 {
				buf.WriteByte(' ')
			}
			if err := printer.Print(&buf, w); err != nil {
				logger.Default().Warn("print shell word", zap.Error(err))
			}
		}
		return strings.TrimSpace(buf.String())
	}

	extractFromStmt = func(stmt *syntax.Stmt) {
		if stmt == nil {
			return
		}
		// 重定向操作符本身（>、2>&1 等）从命令文本里剥掉，但目标 word 里若有
		// CmdSubst/ProcSubst 仍会在 shell 求值阶段执行，必须按子命令检查。
		for _, r := range stmt.Redirs {
			extractFromWord(r.Word)
		}
		if stmt.Cmd == nil {
			return
		}
		switch cmd := stmt.Cmd.(type) {
		case *syntax.BinaryCmd:
			extractFromStmt(cmd.X)
			extractFromStmt(cmd.Y)
		case *syntax.CallExpr:
			// 环境变量前缀的 RHS 在命令执行前求值，含命令替换时必须递归
			for _, a := range cmd.Assigns {
				extractFromWord(a.Value)
			}
			if len(cmd.Args) > 0 {
				if s := printWords(cmd.Args); s != "" {
					cmds = append(cmds, s)
				}
				for _, w := range cmd.Args {
					extractFromWord(w)
				}
			}
		default:
			// 其他控制结构里有内嵌 Stmt，用 Walk 找出来递归
			syntax.Walk(stmt.Cmd, func(node syntax.Node) bool {
				if s, ok := node.(*syntax.Stmt); ok {
					extractFromStmt(s)
					return false
				}
				return true
			})
		}
	}

	for _, stmt := range file.Stmts {
		extractFromStmt(stmt)
	}

	return cmds, nil
}

// extractFromWordPart 递归处理单个 WordPart，把内部可执行单元交回 extractFromStmt。
// 拆出来是为了让 ExtractSubCommands 主循环聚焦语句级控制流。
func extractFromWordPart(
	part syntax.WordPart,
	extractFromStmt func(*syntax.Stmt),
	extractFromWord func(*syntax.Word),
) {
	switch p := part.(type) {
	case *syntax.CmdSubst:
		// $(...) 与反引号
		for _, s := range p.Stmts {
			extractFromStmt(s)
		}
	case *syntax.ProcSubst:
		// <(...) / >(...) bash 进程替换，子 shell 里的命令也会被执行
		for _, s := range p.Stmts {
			extractFromStmt(s)
		}
	case *syntax.DblQuoted:
		for _, sp := range p.Parts {
			extractFromWordPart(sp, extractFromStmt, extractFromWord)
		}
	case *syntax.ParamExp, *syntax.ArithmExp:
		// ${VAR:-$(rm -rf /)} 的默认值 / $((expr)) 内的子表达式都可能藏 CmdSubst，
		// 走 Walk 把内部 Stmt 全部找出来递归
		syntax.Walk(p, func(node syntax.Node) bool {
			if s, ok := node.(*syntax.Stmt); ok {
				extractFromStmt(s)
				return false
			}
			return true
		})
		// SglQuoted/Lit：单引号/字面量不展开，跳过
	}
}
