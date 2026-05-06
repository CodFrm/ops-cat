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
			So(cmds, ShouldHaveLength, 2)
		})

		Convey("; 分隔", func() {
			cmds, err := ExtractSubCommands("ls; pwd; whoami")
			So(err, ShouldBeNil)
			So(cmds, ShouldHaveLength, 3)
		})

		Convey("管道", func() {
			cmds, err := ExtractSubCommands("cat file | grep error")
			So(err, ShouldBeNil)
			So(cmds, ShouldHaveLength, 2)
		})

		Convey("命令替换", func() {
			cmds, err := ExtractSubCommands("echo $(whoami)")
			So(err, ShouldBeNil)
			So(len(cmds), ShouldBeGreaterThanOrEqualTo, 1)
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
