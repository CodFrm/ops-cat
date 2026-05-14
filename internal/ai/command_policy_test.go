package ai

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseCommandRule(t *testing.T) {
	Convey("ParseCommandRule", t, func() {
		Convey("简单命令名", func() {
			r := ParseCommandRule("ls")
			So(r.Program, ShouldEqual, "ls")
			So(r.SubCommands, ShouldBeEmpty)
			So(r.Flags, ShouldBeEmpty)
			So(r.Wildcard, ShouldBeFalse)
		})

		Convey("命令 + 子命令", func() {
			r := ParseCommandRule("kubectl get po")
			So(r.Program, ShouldEqual, "kubectl")
			So(r.SubCommands, ShouldResemble, []string{"get", "po"})
			So(r.Flags, ShouldBeEmpty)
		})

		Convey("命令 + flag + value", func() {
			r := ParseCommandRule("kubectl get po -n app")
			So(r.Program, ShouldEqual, "kubectl")
			So(r.SubCommands, ShouldResemble, []string{"get", "po"})
			So(r.Flags, ShouldResemble, map[string]string{"-n": "app"})
		})

		Convey("长 flag=value 格式", func() {
			r := ParseCommandRule("kubectl get po --namespace=app")
			So(r.Flags, ShouldResemble, map[string]string{"--namespace": "app"})
		})

		Convey("通配符", func() {
			r := ParseCommandRule("kubectl get *")
			So(r.Program, ShouldEqual, "kubectl")
			So(r.SubCommands, ShouldResemble, []string{"get"})
			So(r.Wildcard, ShouldBeTrue)
		})

		Convey("flag 值为通配符", func() {
			r := ParseCommandRule("kubectl get po -n *")
			So(r.Flags, ShouldResemble, map[string]string{"-n": "*"})
			So(r.Wildcard, ShouldBeFalse)
		})

		Convey("末尾通配符 + flag 值通配符", func() {
			r := ParseCommandRule("kubectl get * -n * *")
			So(r.SubCommands, ShouldResemble, []string{"get"})
			So(r.Flags, ShouldResemble, map[string]string{"-n": "*"})
			So(r.Wildcard, ShouldBeTrue)
		})

		Convey("空字符串", func() {
			r := ParseCommandRule("")
			So(r.Program, ShouldBeEmpty)
		})
	})
}

func TestMatchCommandRule(t *testing.T) {
	Convey("MatchCommandRule", t, func() {
		Convey("简单命令名匹配", func() {
			So(MatchCommandRule("ls", "ls"), ShouldBeTrue)
			So(MatchCommandRule("ls", "cat"), ShouldBeFalse)
		})

		Convey("命令名匹配不允许额外子命令（无通配符）", func() {
			So(MatchCommandRule("ls", "ls -la"), ShouldBeFalse)
		})

		Convey("带通配符允许额外参数", func() {
			So(MatchCommandRule("ls *", "ls -la /tmp"), ShouldBeTrue)
			So(MatchCommandRule("ls *", "ls"), ShouldBeTrue)
		})

		Convey("单独 * 匹配任意命令", func() {
			So(MatchCommandRule("*", "ls -la /tmp"), ShouldBeTrue)
			So(MatchCommandRule("*", "DEBIAN_FRONTEND=noninteractive apt-get update -qq"), ShouldBeTrue)
			So(MatchCommandRule("*", "rm -rf /"), ShouldBeTrue)
		})

		Convey("环境变量前缀不影响实际程序匹配", func() {
			So(MatchCommandRule("apt-get *", "DEBIAN_FRONTEND=noninteractive apt-get update -qq"), ShouldBeTrue)
		})

		Convey("PATH=... 等危险前缀不能让命令逃过 program 检查", func() {
			// PATH/LD_PRELOAD/IFS/BASH_ENV 等会改变命令解析或解释器行为，
			// 不能像 DEBIAN_FRONTEND 那样被静默剥离，否则 `apt-get *` 等规则被绕过
			So(MatchCommandRule("ls *", "PATH=/tmp/evil ls"), ShouldBeFalse)
			So(MatchCommandRule("ls *", "LD_PRELOAD=/tmp/x.so ls -la"), ShouldBeFalse)
			So(MatchCommandRule("apt-get *", "BASH_ENV=/tmp/x apt-get update"), ShouldBeFalse)
			So(MatchCommandRule("cat /etc/hosts", "IFS=$'\\n' cat /etc/hosts"), ShouldBeFalse)
		})

		Convey("子命令匹配", func() {
			So(MatchCommandRule("kubectl get *", "kubectl get po"), ShouldBeTrue)
			So(MatchCommandRule("kubectl get *", "kubectl delete po"), ShouldBeFalse)
		})

		Convey("flag 匹配 - 相同位置", func() {
			So(MatchCommandRule("kubectl get po -n app", "kubectl get po -n app"), ShouldBeTrue)
		})

		Convey("flag 匹配 - 不同位置（顺序无关）", func() {
			So(MatchCommandRule("kubectl get po -n app", "kubectl -n app get po"), ShouldBeTrue)
		})

		Convey("flag 值不匹配", func() {
			So(MatchCommandRule("kubectl get po -n app", "kubectl get po -n production"), ShouldBeFalse)
		})

		Convey("flag 值通配符", func() {
			So(MatchCommandRule("kubectl get po -n *", "kubectl get po -n production"), ShouldBeTrue)
			So(MatchCommandRule("kubectl get po -n *", "kubectl get po -n app"), ShouldBeTrue)
		})

		Convey("长 flag 格式", func() {
			So(MatchCommandRule("kubectl get po --namespace=app", "kubectl get po --namespace=app"), ShouldBeTrue)
			So(MatchCommandRule("kubectl get po --namespace=app", "kubectl get po --namespace=production"), ShouldBeFalse)
		})

		Convey("路径 glob 匹配", func() {
			So(MatchCommandRule("cat /var/log/*", "cat /var/log/nginx.log"), ShouldBeTrue)
			So(MatchCommandRule("cat /var/log/*", "cat /etc/passwd"), ShouldBeFalse)
			So(MatchCommandRule("cat /var/log/*", "cat /var/log/nginx/access.log"), ShouldBeFalse)
		})

		Convey("多余子命令 - 无通配符拒绝", func() {
			So(MatchCommandRule("systemctl status", "systemctl status nginx"), ShouldBeFalse)
		})

		Convey("多余子命令 - 有通配符允许", func() {
			So(MatchCommandRule("systemctl status *", "systemctl status nginx"), ShouldBeTrue)
		})

		Convey("布尔 flag 不影响匹配", func() {
			So(MatchCommandRule("kubectl get po -n app *", "kubectl -v -n app get po"), ShouldBeTrue)
		})

		Convey("缺少规则要求的 flag", func() {
			So(MatchCommandRule("kubectl get po -n app", "kubectl get po"), ShouldBeFalse)
		})

		Convey("rm -rf 危险命令匹配", func() {
			Convey("rm -rf /* * 匹配 rm -rf /", func() {
				// /* 作为 -rf 的 flag 值，filepath.Match("/*", "/") 匹配成功
				So(MatchCommandRule("rm -rf /* *", "rm -rf /"), ShouldBeTrue)
			})

			Convey("rm -rf / * 匹配 rm -rf /", func() {
				// / 作为 -rf 的 flag 值，精确匹配
				So(MatchCommandRule("rm -rf / *", "rm -rf /"), ShouldBeTrue)
			})

			Convey("rm -rf /* * 匹配 rm -rf /tmp", func() {
				So(MatchCommandRule("rm -rf /* *", "rm -rf /tmp"), ShouldBeTrue)
			})

			Convey("rm -rf /* * 不匹配 rm -rf /tmp/sub（跨路径分隔符）", func() {
				// filepath.Match 的 * 不匹配路径分隔符
				So(MatchCommandRule("rm -rf /* *", "rm -rf /tmp/sub"), ShouldBeFalse)
			})

			Convey("rm -rf / * 不匹配 rm -rf /tmp（精确值不匹配）", func() {
				So(MatchCommandRule("rm -rf / *", "rm -rf /tmp"), ShouldBeFalse)
			})

			Convey("rm -rf / 精确匹配 rm -rf /", func() {
				So(MatchCommandRule("rm -rf /", "rm -rf /"), ShouldBeTrue)
			})

			Convey("rm -rf / 不匹配 rm -rf /tmp", func() {
				So(MatchCommandRule("rm -rf /", "rm -rf /tmp"), ShouldBeFalse)
			})

			Convey("rm -rf /* 无尾部通配符也能匹配 rm -rf /", func() {
				// -rf 的值为 /*，匹配 /；无尾部 * 所以不允许多余参数
				So(MatchCommandRule("rm -rf /*", "rm -rf /"), ShouldBeTrue)
				So(MatchCommandRule("rm -rf /*", "rm -rf /tmp"), ShouldBeTrue)
			})

			Convey("rm -rf /* 不匹配有额外 flag 的命令（无尾部通配符）", func() {
				So(MatchCommandRule("rm -rf /*", "rm -rf --no-preserve-root /"), ShouldBeFalse)
			})
		})

		Convey("组合 flag 自动展开（-rf 等价 -r -f）", func() {
			Convey("-rf 规则匹配 -r -f 命令", func() {
				So(MatchCommandRule("rm -rf /* *", "rm -r -f /"), ShouldBeTrue)
			})

			Convey("-r -f 规则匹配 -rf 命令", func() {
				So(MatchCommandRule("rm -r -f /* *", "rm -rf /"), ShouldBeTrue)
			})

			Convey("-r -f 规则匹配 -r -f 命令", func() {
				So(MatchCommandRule("rm -r -f /* *", "rm -r -f /"), ShouldBeTrue)
			})

			Convey("-rf 规则匹配 -rf 命令", func() {
				So(MatchCommandRule("rm -rf /* *", "rm -rf /"), ShouldBeTrue)
			})

			Convey("长 flag 不展开", func() {
				So(MatchCommandRule("rm --recursive --force /* *", "rm -r -f /"), ShouldBeFalse)
			})
		})
	})
}

func TestExtractSubCommands(t *testing.T) {
	Convey("ExtractSubCommands", t, func() {
		Convey("简单命令", func() {
			cmds, err := ExtractSubCommands("ls -la")
			So(err, ShouldBeNil)
			So(cmds, ShouldHaveLength, 1)
			So(cmds[0], ShouldEqual, "ls -la")
		})

		Convey("&& 组合", func() {
			cmds, err := ExtractSubCommands("ls /tmp && cat /etc/passwd")
			So(err, ShouldBeNil)
			So(cmds, ShouldHaveLength, 2)
			So(cmds[0], ShouldEqual, "ls /tmp")
			So(cmds[1], ShouldEqual, "cat /etc/passwd")
		})

		Convey("|| 组合", func() {
			cmds, err := ExtractSubCommands("ls /tmp || echo fail")
			So(err, ShouldBeNil)
			So(cmds, ShouldResemble, []string{"ls /tmp", "echo fail"})
		})

		Convey("; 分隔", func() {
			cmds, err := ExtractSubCommands("ls; pwd; whoami")
			So(err, ShouldBeNil)
			So(cmds, ShouldResemble, []string{"ls", "pwd", "whoami"})
		})

		Convey("管道", func() {
			cmds, err := ExtractSubCommands("cat file | grep error")
			So(err, ShouldBeNil)
			So(cmds, ShouldResemble, []string{"cat file", "grep error"})
		})

		Convey("命令替换", func() {
			cmds, err := ExtractSubCommands("echo $(whoami)")
			So(err, ShouldBeNil)
			So(cmds, ShouldResemble, []string{"echo $(whoami)", "whoami"})
		})

		Convey("环境变量前缀随命令一起进入子命令文本，由匹配阶段决定是否剥离", func() {
			// 旧实现在抽取阶段就丢弃了所有 Assigns，导致 `PATH=/tmp/evil ls` 与 `ls *` 误匹配。
			// 现在保留 Assigns，让 ParseActualCommand 的 stripLeadingAssigns 决定哪些可剥离。
			cmds, err := ExtractSubCommands("DEBIAN_FRONTEND=noninteractive apt-get update -qq && systemctl stop nginx")
			So(err, ShouldBeNil)
			So(cmds, ShouldResemble, []string{"DEBIAN_FRONTEND=noninteractive apt-get update -qq", "systemctl stop nginx"})
		})

		Convey("反引号命令替换也会提取内部命令", func() {
			cmds, err := ExtractSubCommands("echo `whoami`")
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "whoami")
		})

		Convey("双引号内命令替换会执行并提取", func() {
			cmds, err := ExtractSubCommands(`echo "$(uname -a)"`)
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "uname -a")
		})

		Convey("单引号内命令替换不会被当作执行单元", func() {
			cmds, err := ExtractSubCommands(`echo '$(rm -rf /)'`)
			So(err, ShouldBeNil)
			So(cmds, ShouldHaveLength, 1)
			So(cmds[0], ShouldEqual, `echo '$(rm -rf /)'`)
		})

		Convey("嵌套命令替换递归提取", func() {
			cmds, err := ExtractSubCommands(`echo "$(printf '%s' "$(whoami)")"`)
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "whoami")
			So(cmds, ShouldContain, `printf '%s' "$(whoami)"`)
		})

		Convey("复杂组合命令覆盖环境变量、连接符、管道、命令替换和引用差异", func() {
			command := "cd /tmp && DEBIAN_FRONTEND=noninteractive apt-get update -qq; echo \"$(printf '%s' \"$(whoami)\")\" | grep \"$(hostname)\" || echo '$(rm -rf /)' && printf %s `uname -s`"

			cmds, err := ExtractSubCommands(command)
			So(err, ShouldBeNil)

			So(cmds, ShouldContain, "cd /tmp")
			So(cmds, ShouldContain, "DEBIAN_FRONTEND=noninteractive apt-get update -qq")
			So(cmds, ShouldContain, `echo "$(printf '%s' "$(whoami)")"`)
			So(cmds, ShouldContain, `printf '%s' "$(whoami)"`)
			So(cmds, ShouldContain, "whoami")
			So(cmds, ShouldContain, `grep "$(hostname)"`)
			So(cmds, ShouldContain, "hostname")
			So(cmds, ShouldContain, `echo '$(rm -rf /)'`)
			So(cmds, ShouldContain, "uname -s")
			So(cmds, ShouldNotContain, "rm -rf /")
		})

		Convey("/dev/null 重定向不被作为合成执行单元，其他写入目标仍要作为单独 unit", func() {
			// 2>/dev/null 是公认安全目标，剥离不影响匹配；
			// `>file` 写入任意路径必须作为额外 sub-command 强制走策略匹配，
			// 否则 allow `echo *` 会让 `echo pwned > /etc/cron.d/x` 静默写文件。
			cmds, err := ExtractSubCommands("systemctl stop nginx 2>/dev/null && systemctl disable nginx 2>/dev/null; echo done > /tmp/out.log")
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "systemctl stop nginx")
			So(cmds, ShouldContain, "systemctl disable nginx")
			So(cmds, ShouldContain, "echo done")
			So(cmds, ShouldContain, "> /tmp/out.log")
		})

		Convey("git clone 2>&1 也被剥离（fd 复制不产生新 I/O）", func() {
			cmds, err := ExtractSubCommands("cd /tmp && git clone --depth 1 https://example.com/x.git x 2>&1")
			So(err, ShouldBeNil)
			So(cmds, ShouldResemble, []string{
				"cd /tmp",
				"git clone --depth 1 https://example.com/x.git x",
			})
		})

		Convey("`>file` 输出重定向作为合成 sub-command", func() {
			// echo pwned > /etc/cron.d/x：echo 本体允许时，写文件也得被策略覆盖
			cmds, err := ExtractSubCommands("echo pwned > /etc/cron.d/x")
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "echo pwned")
			So(cmds, ShouldContain, "> /etc/cron.d/x")
		})

		Convey("`>>file` 追加重定向也作为合成 sub-command", func() {
			cmds, err := ExtractSubCommands("echo line >> /etc/hosts")
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, ">> /etc/hosts")
		})

		Convey("函数声明体不被当作执行单元提取", func() {
			// 函数声明不触发执行 — `cleanup() { rm -rf /tmp/foo; }; echo ok` 里 rm 不应进 cmds
			cmds, err := ExtractSubCommands("cleanup() { rm -rf /tmp/foo; }; echo ok")
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "echo ok")
			So(cmds, ShouldNotContain, "rm -rf /tmp/foo")
			So(cmds, ShouldNotContain, "rm -rf /tmp/foo;")
		})

		Convey("$$ 是 PID 展开，不是命令分隔符", func() {
			// `$$` 是 ParamExp（shell 进程 PID），不应被当作 && 之类的执行分隔符
			cmds, err := ExtractSubCommands("echo $$")
			So(err, ShouldBeNil)
			So(cmds, ShouldHaveLength, 1)
			So(cmds[0], ShouldEqual, "echo $$")

			cmds, err = ExtractSubCommands("kill -9 $$ && echo done")
			So(err, ShouldBeNil)
			So(cmds, ShouldResemble, []string{"kill -9 $$", "echo done"})
		})

		Convey("解析失败返回错误", func() {
			// 未闭合的命令替换：parser 应当报错，让上层走 NeedConfirm 兜底
			_, err := ExtractSubCommands("echo $(")
			So(err, ShouldNotBeNil)
		})

		Convey("重定向目标里的命令替换会被提取", func() {
			// `> $(rm -rf /)`：redirect target word 含 CmdSubst，必须把内部命令拿出来
			cmds, err := ExtractSubCommands("echo ok > $(rm -rf /)")
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "rm -rf /")
		})

		Convey("命令前的环境变量赋值里的命令替换会被提取", func() {
			// CallExpr.Assigns 的 RHS 可执行 CmdSubst：PAYLOAD=$(rm -rf /) echo ok
			// 主命令以 "PAYLOAD=$(rm -rf /) echo ok" 完整形式入 cmds，匹配阶段由
			// stripLeadingAssigns 决定 PAYLOAD 是否安全可剥离；CmdSubst 内部命令照常单独提取。
			cmds, err := ExtractSubCommands("PAYLOAD=$(rm -rf /) echo ok")
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "rm -rf /")
			So(cmds, ShouldContain, "PAYLOAD=$(rm -rf /) echo ok")
		})

		Convey("进程替换 <(...) 内的命令会被提取", func() {
			cmds, err := ExtractSubCommands("cat <(rm -rf /)")
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "rm -rf /")
		})

		Convey("参数展开默认值里的命令替换会被提取", func() {
			// ${VAR:-$(rm -rf /)} 的 default value 是 word，含 CmdSubst
			cmds, err := ExtractSubCommands(`echo ${VAR:-$(rm -rf /)}`)
			So(err, ShouldBeNil)
			So(cmds, ShouldContain, "rm -rf /")
		})

		Convey("仅注释/空白时返回空切片", func() {
			// parser 成功但没有可执行 Stmt — 让上层按 NeedConfirm 兜底
			cmds, err := ExtractSubCommands("   ")
			So(err, ShouldBeNil)
			So(cmds, ShouldBeEmpty)
		})
	})
}

func TestFindHintRules(t *testing.T) {
	Convey("findHintRules", t, func() {
		allowRules := []string{
			"kubectl get po -n app *",
			"kubectl get svc -n app *",
			"ls *",
			"docker ps *",
		}

		Convey("找到同程序名的提示", func() {
			hints := findHintRules("kubectl get po --namespace app", allowRules)
			So(hints, ShouldHaveLength, 2)
			So(hints[0], ShouldEqual, "kubectl get po -n app *")
			So(hints[1], ShouldEqual, "kubectl get svc -n app *")
		})

		Convey("没有匹配的程序名", func() {
			hints := findHintRules("rm -rf /", allowRules)
			So(hints, ShouldBeEmpty)
		})
	})
}

func TestAllSubCommandsAllowed(t *testing.T) {
	Convey("allSubCommandsAllowed", t, func() {
		rules := []string{"ls *", "cat *", "grep *"}

		Convey("全部允许", func() {
			ok, matched := allSubCommandsAllowed([]string{"ls -la", "cat /etc/passwd"}, rules)
			So(ok, ShouldBeTrue)
			So(matched, ShouldNotBeEmpty)
		})

		Convey("部分不允许", func() {
			ok, _ := allSubCommandsAllowed([]string{"ls -la", "rm -rf /"}, rules)
			So(ok, ShouldBeFalse)
		})

		Convey("空规则", func() {
			ok, _ := allSubCommandsAllowed([]string{"ls"}, nil)
			So(ok, ShouldBeFalse)
		})
	})
}
