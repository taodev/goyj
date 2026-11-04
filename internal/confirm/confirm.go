// Package confirm 实现覆盖已有文件之前的确认逻辑。
//
// 三种情形:
//   - 指定了 --yes:不询问,直接允许覆盖;
//   - 标准输入是终端:交互式询问,支持 y / n / a(全部覆盖)/ q(退出);
//   - 标准输入不是终端且未指定 --yes:返回 ErrNonInteractive,绝不阻塞等待输入。
package confirm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

var (
	// ErrNonInteractive 表示需要确认,但当前不是交互环境且未指定 --yes。
	ErrNonInteractive = errors.New("目标文件已存在,但当前不是交互环境;请使用 --yes 或 -y 允许覆盖")
	// ErrAborted 表示用户在提示中选择了退出。
	ErrAborted = errors.New("用户取消了操作")
)

// Prompter 在覆盖文件前征求用户同意。零值不可用,请通过 New 或 NewPrompter 创建。
type Prompter struct {
	in          *bufio.Reader
	out         io.Writer
	interactive bool
	yes         bool
	all         bool // 用户已选择"全部覆盖"
}

// New 创建使用真实终端的 Prompter。
// in 通常是 os.Stdin,是否交互由 in 是否为终端决定;提示写入 out(通常是 os.Stderr)。
func New(in *os.File, out io.Writer, yes bool) *Prompter {
	return NewPrompter(in, out, term.IsTerminal(int(in.Fd())), yes)
}

// NewPrompter 创建 Prompter,交互状态由调用者显式给出,便于测试。
func NewPrompter(in io.Reader, out io.Writer, interactive, yes bool) *Prompter {
	return &Prompter{in: bufio.NewReader(in), out: out, interactive: interactive, yes: yes}
}

// CanPrompt 报告 Overwrite 是否一定能得到答案(--yes 或交互环境)。
func (p *Prompter) CanPrompt() bool { return p.yes || p.interactive }

// Overwrite 询问是否覆盖 path。
//
// 返回 (true, nil) 表示允许覆盖,(false, nil) 表示跳过该文件;
// 非交互环境且未指定 --yes 时返回 ErrNonInteractive,用户选择退出时返回 ErrAborted。
// 在交互环境中输入流提前结束(EOF)按退出处理。
func (p *Prompter) Overwrite(path string) (bool, error) {
	if p.yes || p.all {
		return true, nil
	}
	if !p.interactive {
		return false, ErrNonInteractive
	}
	for {
		fmt.Fprintf(p.out, "文件 %s 已存在,是否覆盖?[y]是 [N]否 [a]全部覆盖 [q]退出: ", path)
		line, err := p.in.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(p.out)
			if errors.Is(err, io.EOF) {
				return false, ErrAborted
			}
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "", "n", "no":
			return false, nil
		case "a", "all":
			p.all = true
			return true, nil
		case "q", "quit":
			return false, ErrAborted
		}
		fmt.Fprintln(p.out, "无法识别的输入,请输入 y、n、a 或 q。")
	}
}
