// 命令 goyj 在 YAML 与 JSON 之间互相转换。
//
// 本文件只负责参数解析、退出码映射和调用库,转换逻辑位于 internal/batch。
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/taodev/goyj"
	"github.com/taodev/goyj/internal/batch"
	"github.com/taodev/goyj/internal/confirm"
)

// 退出码。
const (
	exitOK       = 0 // 全部成功
	exitFailure  = 1 // 转换或 I/O 错误
	exitUsageErr = 2 // 参数错误
)

// version 由构建时通过 -ldflags "-X main.version=..." 注入;未注入时尝试读取模块版本。
var version = ""

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// usageError 标记命令行参数层面的错误,对应退出码 2。
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

// cliOptions 保存命令行标志的取值。
type cliOptions struct {
	output  string
	from    string
	to      string
	strict  bool
	indent  int
	compact bool
	yes     bool
}

// run 执行命令并返回退出码,与 os 解耦以便测试。
func run(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	var code int
	cmd := newRootCmd(stdin, stdout, stderr, &code)
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(stderr, "错误:"+err.Error())
		var ue *usageError
		if errors.As(err, &ue) {
			fmt.Fprintln(stderr, "使用 goyj --help 查看用法。")
			return exitUsageErr
		}
		return exitFailure
	}
	return code
}

func newRootCmd(stdin *os.File, stdout, stderr io.Writer, code *int) *cobra.Command {
	var o cliOptions
	cmd := &cobra.Command{
		Use:   "goyj [flags] [<input>...]",
		Short: "在 YAML 与 JSON 之间互相转换",
		Long: `goyj 在 YAML 与 JSON 之间互相转换。

输入可以是文件、目录(递归处理)或标准输入。输入格式默认由后缀推断
(.yaml/.yml 为 YAML,.json 为 JSON),可用 --from 覆盖。

未指定 -o 时,在原目录生成同名新后缀文件;指定 -o 时,保持相对目录结构输出到该目录。
不带输入参数(或输入为 -)时从标准输入读取并写到标准输出,此时必须使用 --from。

退出码:0 全部成功;1 转换或 I/O 错误;2 参数错误。`,
		Example: `  goyj config.yaml                  # 生成 config.json
  goyj -o out/ conf/                # 转换 conf 目录,结果放到 out/
  goyj --strict --indent 4 a.json   # 严格模式,缩进 4
  cat a.yaml | goyj --from yaml     # 标准输入 → 标准输出
  goyj -y *.yaml                    # 覆盖已存在的文件时不询问`,
		Version:       resolveVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			*code = execute(o, args, stdin, stdout, stderr)
			return nil
		},
	}
	cmd.SetVersionTemplate("goyj {{.Version}}\n")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &usageError{err} })

	f := cmd.Flags()
	f.StringVarP(&o.output, "output", "o", "", "输出目录;不指定则在原目录生成新后缀文件")
	f.StringVar(&o.from, "from", "", "强制指定输入格式(yaml|json),从标准输入读取时必须指定")
	f.StringVar(&o.to, "to", "", "强制指定输出格式(yaml|json)")
	f.BoolVar(&o.strict, "strict", false, "严格模式:遇到无法无损转换的特性时报错")
	f.IntVar(&o.indent, "indent", 2, "缩进宽度(2 到 9)")
	f.BoolVar(&o.compact, "compact", false, "JSON 紧凑单行输出")
	f.BoolVarP(&o.yes, "yes", "y", false, "覆盖已存在的文件时跳过确认")
	return cmd
}

// execute 运行转换并返回退出码。
func execute(o cliOptions, args []string, stdin *os.File, stdout, stderr io.Writer) int {
	if o.indent < 2 || o.indent > 9 {
		fmt.Fprintf(stderr, "错误:--indent 必须在 2 到 9 之间,实际为 %d\n", o.indent)
		return exitUsageErr
	}

	res, err := batch.Run(batch.Options{
		Inputs: args,
		Output: o.output,
		From:   o.from,
		To:     o.to,
		Convert: []goyj.Option{
			goyj.WithStrict(o.strict),
			goyj.WithIndent(o.indent),
			goyj.WithCompact(o.compact),
		},
		Confirmer: confirm.New(stdin, stderr, o.yes),
		Stdin:     stdin,
		Stdout:    stdout,
		Stderr:    stderr,
	})
	if err != nil {
		fmt.Fprintln(stderr, "错误:"+err.Error())
		var ue *batch.UsageError
		if errors.As(err, &ue) {
			return exitUsageErr
		}
		return exitFailure
	}

	if len(res.Failed) > 0 {
		fmt.Fprintf(stderr, "\n%d 个输入处理失败:\n", len(res.Failed))
		for _, f := range res.Failed {
			fmt.Fprintf(stderr, "  %s: %v\n", f.Path, f.Err)
		}
		return exitFailure
	}
	return exitOK
}

// resolveVersion 返回版本号:优先使用构建时注入的值,其次读取 go install 记录的模块版本。
func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
