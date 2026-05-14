package ai

import (
	"fmt"
	"strings"

	"github.com/cago-frame/cago/pkg/logger"
	"go.uber.org/zap"

	"mvdan.cc/sh/v3/syntax"
)

// --- Shell AST 解析 ---

// safeRedirTargets 列举公认安全的输出重定向目标。这些目标不会产生持久副作用，
// 跳过它们能保留 `cmd 2>/dev/null` 等常见模式与现有 allow 规则的兼容；
// 任何其他目标都得作为合成 sub-command 强制走策略匹配。
var safeRedirTargets = map[string]struct{}{
	"/dev/null":   {},
	"/dev/stderr": {},
	"/dev/stdout": {},
}

// isOutputWriteRedir 返回 true 表示该重定向产生新的 I/O 写入，应作为合成 sub-command。
// fd 复制（>&、<&）与输入重定向（<、<<、<<-、<<<）不产生新写副作用，跳过。
func isOutputWriteRedir(op syntax.RedirOperator) bool {
	switch op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrInOut,
		syntax.RdrClob, syntax.AppClob,
		syntax.RdrAll, syntax.RdrAllClob, syntax.AppAll, syntax.AppAllClob:
		return true
	}
	return false
}

// ExtractSubCommands 从 shell 命令中提取所有可执行子命令。
//
// 处理：
//   - `&&` `||` `;` `|` 等 BinaryCmd 拆分
//   - CallExpr：Args 作为命令本体；Assigns 的右值仍要扫，因为 `PAYLOAD=$(rm -rf /) echo`
//     里的 CmdSubst 会先于命令执行
//   - Stmt.Redirs：目标 word 里若有 CmdSubst/ProcSubst 要递归提取；输出重定向（>、>>、&>…）
//     除目标是 /dev/null 等公认安全目标外，还要作为合成 sub-command 强制策略覆盖，
//     否则 `echo pwned > /etc/cron.d/x` 会被 `echo *` 静默放行写文件
//   - `$()`、反引号、`<(...)`/`>(...)` 进程替换、`${VAR:-$(...)}` 参数展开默认值都会执行
//   - 双引号内的 `$()` 同样递归；单引号内不展开
//   - FuncDecl：函数声明体不触发执行，跳过，避免 `cleanup() { rm -rf /tmp/foo; }; echo ok`
//     这种仅定义不调用的脚本被误判
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

	printWord := func(w *syntax.Word) string {
		if w == nil {
			return ""
		}
		var buf strings.Builder
		if err := printer.Print(&buf, w); err != nil {
			logger.Default().Warn("print shell word", zap.Error(err))
		}
		return strings.TrimSpace(buf.String())
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
		// 重定向：1) 扫目标 word 里的 CmdSubst/ProcSubst；2) 输出写重定向额外
		// 作为合成 sub-command 入 cmds，让普通 allow 规则无法静默放行写文件。
		for _, r := range stmt.Redirs {
			extractFromWord(r.Word)
			if !isOutputWriteRedir(r.Op) {
				continue
			}
			target := printWord(r.Word)
			if target == "" {
				continue
			}
			if _, safe := safeRedirTargets[target]; safe {
				continue
			}
			cmds = append(cmds, r.Op.String()+" "+target)
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
				// 把 Assigns 与 Args 拼回原始文本，让 ParseActualCommand 的 stripLeadingAssigns
				// 决定哪些可安全剥离（DEBIAN_FRONTEND=…）哪些必须保留参与匹配（PATH=…/LD_PRELOAD=…）。
				// 直接丢弃 Assigns 会让 `PATH=/tmp/evil ls` 与 `ls *` 误匹配。
				s := printCall(printer, cmd, printWords)
				if s != "" {
					cmds = append(cmds, s)
				}
				for _, w := range cmd.Args {
					extractFromWord(w)
				}
			}
		case *syntax.FuncDecl:
			// 函数声明本身不触发执行，跳过 Body；调用点会走 CallExpr 路径
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

// printCall 拼回 CallExpr 的原始文本：先环境变量赋值（NAME=VALUE），再命令本体。
// 保留赋值是为了让下游 stripLeadingAssigns 自行决定哪些可剥离哪些是 PATH 等危险变量。
func printCall(printer *syntax.Printer, cmd *syntax.CallExpr, printWords func([]*syntax.Word) string) string {
	var buf strings.Builder
	for i, a := range cmd.Assigns {
		if i > 0 {
			buf.WriteByte(' ')
		}
		if a.Name != nil {
			buf.WriteString(a.Name.Value)
		}
		buf.WriteByte('=')
		if a.Value != nil {
			if err := printer.Print(&buf, a.Value); err != nil {
				logger.Default().Warn("print shell assign value", zap.Error(err))
			}
		}
	}
	args := printWords(cmd.Args)
	if args == "" {
		return strings.TrimSpace(buf.String())
	}
	if buf.Len() > 0 {
		buf.WriteByte(' ')
	}
	buf.WriteString(args)
	return strings.TrimSpace(buf.String())
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
