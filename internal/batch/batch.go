// Package batch 实现命令行的转换流程:解析输入(文件、目录、标准输入)、
// 计算输出路径、覆盖确认、写出文件,并汇总失败项。
//
// 流程分为四个阶段,保证"无法确认覆盖"时不会留下写了一半的结果:
//  1. 收集输入文件;
//  2. 在内存中完成全部转换并计算输出路径,检测路径冲突;
//  3. 对已存在的目标文件逐个确认是否覆盖;
//  4. 写出文件。
//
// 单个文件的转换或写入失败不会中断整体流程,失败项记录在 Result.Failed 中。
package batch

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/taodev/goyj"
	"github.com/taodev/goyj/internal/confirm"
)

// stdinName 是标准输入在失败列表中的显示名称。
const stdinName = "<stdin>"

// Confirmer 在覆盖已有文件之前征求同意,*confirm.Prompter 实现了该接口。
type Confirmer interface {
	Overwrite(path string) (bool, error)
}

// Options 描述一次批量转换。
type Options struct {
	// Inputs 是输入文件或目录;为空或仅含 "-" 表示从 Stdin 读取。
	Inputs []string
	// Output 是 -o 指定的输出目录,为空表示输出到输入文件所在目录。
	Output string
	// From、To 是格式名称,为空表示自动推断。
	From, To string
	// Convert 传给 goyj 的转换选项。
	Convert []goyj.Option
	// Confirmer 负责覆盖确认;为 nil 时视为非交互环境,遇到已存在的目标文件即报错。
	Confirmer Confirmer
	// Stdin、Stdout、Stderr 是标准流,可替换以便测试。
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// Failure 是一个失败的输入或输出。
type Failure struct {
	Path string
	Err  error
}

// Result 汇总一次运行的结果。
type Result struct {
	// Written 是成功写出的文件。
	Written []string
	// Skipped 是被跳过的条目(没有文档、用户拒绝覆盖等),附带原因。
	Skipped []string
	// Failed 是失败项。
	Failed []Failure
}

// UsageError 表示参数或环境使用不当(对应退出码 2),而非转换过程中的错误。
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

func usagef(format string, args ...any) error {
	return &UsageError{Msg: fmt.Sprintf(format, args...)}
}

// Run 执行批量转换。
//
// 返回的 error 表示致命问题:*UsageError(参数错误或非交互环境下需要确认覆盖)
// 或 confirm.ErrAborted(用户退出);此时不会写出任何文件。
// 单个文件的失败记录在 Result.Failed 中,error 为 nil。Result 始终非 nil。
func Run(o Options) (*Result, error) {
	res := &Result{}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	if err := checkFormats(o); err != nil {
		return res, err
	}
	if isStdin(o.Inputs) {
		return res, runStdin(o, res)
	}
	for _, in := range o.Inputs {
		if in == "-" {
			return res, usagef("标准输入标记 \"-\" 不能与其他输入同时使用")
		}
	}
	return res, runFiles(o, res)
}

func isStdin(inputs []string) bool {
	return len(inputs) == 0 || (len(inputs) == 1 && inputs[0] == "-")
}

// checkFormats 校验 --from 与 --to 指定的格式名称是否已注册。
func checkFormats(o Options) error {
	for _, n := range []struct{ flag, val string }{{"--from", o.From}, {"--to", o.To}} {
		if n.val == "" {
			continue
		}
		if _, ok := goyj.LookupFormat(n.val); !ok {
			return usagef("%s 指定了未知格式 %q,可用格式:%v", n.flag, n.val, goyj.FormatNames())
		}
	}
	return nil
}

// inferTo 在未指定 --to 时推断目标格式:已注册格式中除 from 之外只有一个时取它。
func inferTo(from string) (string, error) {
	var others []string
	for _, n := range goyj.FormatNames() {
		if n != from {
			others = append(others, n)
		}
	}
	if len(others) != 1 {
		return "", usagef("无法推断目标格式(候选:%v),请使用 --to 指定", others)
	}
	return others[0], nil
}

// extFor 返回格式的默认后缀。
func extFor(name string) string {
	f, _ := goyj.LookupFormat(name)
	if f == nil || len(f.Extensions()) == 0 {
		return "." + name
	}
	return f.Extensions()[0]
}

func runStdin(o Options, res *Result) error {
	if o.From == "" {
		return usagef("从标准输入读取时必须使用 --from 指定输入格式")
	}
	if o.Output != "" {
		return usagef("从标准输入读取时输出到标准输出,不能使用 -o")
	}
	to := o.To
	if to == "" {
		var err error
		if to, err = inferTo(o.From); err != nil {
			return err
		}
	}
	if o.Stdin == nil || o.Stdout == nil {
		return usagef("未提供标准输入或标准输出")
	}
	data, err := io.ReadAll(o.Stdin)
	if err != nil {
		res.Failed = append(res.Failed, Failure{stdinName, fmt.Errorf("读取标准输入失败:%w", err)})
		return nil
	}
	docs, err := goyj.ConvertDocs(data, o.From, to, o.Convert...)
	if err != nil {
		res.Failed = append(res.Failed, Failure{stdinName, err})
		return nil
	}
	out, err := goyj.JoinDocs(to, docs)
	if err != nil {
		res.Failed = append(res.Failed, Failure{stdinName, err})
		return nil
	}
	if _, err := o.Stdout.Write(out); err != nil {
		res.Failed = append(res.Failed, Failure{stdinName, fmt.Errorf("写入标准输出失败:%w", err)})
	}
	return nil
}

// task 是一个待转换的输入文件。
type task struct {
	path string
	root string // 来自输入目录时为该目录,否则为空
}

// output 是一个待写出的文件。
type output struct {
	path string
	data []byte
	src  string
}

func runFiles(o Options, res *Result) error {
	tasks := collect(o, res)

	// 所有输入文件的绝对路径,输出不得覆盖其中任何一个。
	inputs := map[string]bool{}
	for _, t := range tasks {
		inputs[absPath(t.path)] = true
	}

	var outputs []output
	seen := map[string]string{} // 输出绝对路径 → 产生它的输入
	for _, t := range tasks {
		outs, err := convertTask(o, t, res)
		if err != nil {
			res.Failed = append(res.Failed, Failure{t.path, err})
			continue
		}
		if conflict := findConflict(outs, inputs, seen); conflict != nil {
			res.Failed = append(res.Failed, Failure{t.path, conflict})
			continue
		}
		for _, out := range outs {
			seen[absPath(out.path)] = t.path
		}
		outputs = append(outputs, outs...)
	}

	outputs, err := resolveOverwrites(o, outputs, res)
	if err != nil {
		return err
	}
	for _, out := range outputs {
		if err := writeFile(out.path, out.data); err != nil {
			res.Failed = append(res.Failed, Failure{out.path, err})
			continue
		}
		res.Written = append(res.Written, out.path)
		fmt.Fprintf(o.Stderr, "%s -> %s\n", out.src, out.path)
	}
	return nil
}

// collect 展开输入:文件原样保留,目录递归收集其中后缀能识别的文件。
func collect(o Options, res *Result) []task {
	var tasks []task
	for _, in := range o.Inputs {
		info, err := os.Stat(in)
		if err != nil {
			res.Failed = append(res.Failed, Failure{in, err})
			continue
		}
		if !info.IsDir() {
			tasks = append(tasks, task{path: in})
			continue
		}
		before := len(tasks)
		err = filepath.WalkDir(in, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				res.Failed = append(res.Failed, Failure{p, err})
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			f, ok := goyj.FormatByExt(filepath.Ext(p))
			if !ok {
				return nil // 无法识别的后缀:目录扫描时静默忽略
			}
			if o.From != "" && f.Name() != o.From {
				return nil
			}
			if o.To != "" && f.Name() == o.To {
				return nil // 已经是目标格式,无需转换
			}
			tasks = append(tasks, task{path: p, root: in})
			return nil
		})
		if err != nil {
			res.Failed = append(res.Failed, Failure{in, err})
		}
		if len(tasks) == before {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s:目录中没有可转换的文件", in))
			fmt.Fprintf(o.Stderr, "跳过 %s:目录中没有可转换的文件\n", in)
		}
	}
	return tasks
}

// convertTask 读取并转换一个输入文件,返回待写出的文件。没有文档时返回空列表并记入 Skipped。
func convertTask(o Options, t task, res *Result) ([]output, error) {
	from := o.From
	if from == "" {
		f, ok := goyj.FormatByExt(filepath.Ext(t.path))
		if !ok {
			return nil, fmt.Errorf("无法根据后缀 %q 推断输入格式,请使用 --from 指定", filepath.Ext(t.path))
		}
		from = f.Name()
	}
	to := o.To
	if to == "" {
		var err error
		if to, err = inferTo(from); err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return nil, err
	}
	docs, err := goyj.ConvertDocs(data, from, to, o.Convert...)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		res.Skipped = append(res.Skipped, fmt.Sprintf("%s:没有文档", t.path))
		fmt.Fprintf(o.Stderr, "跳过 %s:没有文档\n", t.path)
		return nil, nil
	}
	outs := make([]output, len(docs))
	for i, d := range docs {
		outs[i] = output{
			path: OutputPath(t.path, t.root, o.Output, extFor(to), i+1, len(docs)),
			data: d,
			src:  t.path,
		}
	}
	return outs, nil
}

// findConflict 检查输出是否会覆盖某个输入文件,或与其他输入的输出重名。
func findConflict(outs []output, inputs map[string]bool, seen map[string]string) error {
	for _, out := range outs {
		abs := absPath(out.path)
		if inputs[abs] {
			return fmt.Errorf("输出路径 %s 与某个输入文件相同,拒绝覆盖输入", out.path)
		}
		if other, ok := seen[abs]; ok {
			return fmt.Errorf("输出路径 %s 与 %s 的输出冲突", out.path, other)
		}
	}
	return nil
}

// resolveOverwrites 对已存在的目标文件逐个确认,返回最终需要写出的文件。
// 在写出任何文件之前完成,因此在非交互环境或用户退出时不会留下部分结果。
func resolveOverwrites(o Options, outputs []output, res *Result) ([]output, error) {
	var kept []output
	for _, out := range outputs {
		info, err := os.Lstat(out.path)
		if errors.Is(err, fs.ErrNotExist) {
			kept = append(kept, out)
			continue
		}
		if err != nil {
			res.Failed = append(res.Failed, Failure{out.path, err})
			continue
		}
		if info.IsDir() {
			res.Failed = append(res.Failed, Failure{out.path, errors.New("目标路径是一个目录")})
			continue
		}
		ok, err := confirmOverwrite(o, out.path)
		if err != nil {
			if errors.Is(err, confirm.ErrNonInteractive) {
				return nil, &UsageError{Msg: fmt.Sprintf("%s:%v", out.path, err)}
			}
			return nil, err
		}
		if !ok {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s:已存在,未覆盖", out.path))
			fmt.Fprintf(o.Stderr, "跳过 %s:已存在,未覆盖\n", out.path)
			continue
		}
		kept = append(kept, out)
	}
	return kept, nil
}

func confirmOverwrite(o Options, path string) (bool, error) {
	if o.Confirmer == nil {
		return false, confirm.ErrNonInteractive
	}
	return o.Confirmer.Overwrite(path)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return filepath.Clean(a)
	}
	return filepath.Clean(p)
}
