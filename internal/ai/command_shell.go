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
//   - CallExpr 仅取 Args，剥掉 Assigns（如 DEBIAN_FRONTEND=x apt-get … → apt-get …）
//   - Stmt.Redirs（如 2>/dev/null、>file）被忽略
//   - `$()` 和反引号命令替换递归提取内部命令
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

	extractFromWord := func(w *syntax.Word) {
		if w == nil {
			return
		}
		for _, part := range w.Parts {
			switch p := part.(type) {
			case *syntax.CmdSubst:
				for _, s := range p.Stmts {
					extractFromStmt(s)
				}
			case *syntax.DblQuoted:
				for _, sp := range p.Parts {
					if cs, ok := sp.(*syntax.CmdSubst); ok {
						for _, s := range cs.Stmts {
							extractFromStmt(s)
						}
					}
				}
				// SglQuoted: 单引号内不展开，留给上层原样输出
			}
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
		if stmt == nil || stmt.Cmd == nil {
			return
		}
		switch cmd := stmt.Cmd.(type) {
		case *syntax.BinaryCmd:
			extractFromStmt(cmd.X)
			extractFromStmt(cmd.Y)
		case *syntax.CallExpr:
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
