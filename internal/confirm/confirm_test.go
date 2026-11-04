package confirm

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestOverwrite(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		yes         bool
		input       string
		want        bool
		wantErr     error
	}{
		{"yes 跳过确认(交互)", true, true, "", true, nil},
		{"yes 跳过确认(非交互)", false, true, "", true, nil},
		{"非交互且无 yes 报错", false, false, "y\n", false, ErrNonInteractive},
		{"输入 y", true, false, "y\n", true, nil},
		{"输入 YES(忽略大小写与空白)", true, false, "  YES \n", true, nil},
		{"输入 n", true, false, "n\n", false, nil},
		{"直接回车默认为否", true, false, "\n", false, nil},
		{"输入 no", true, false, "no\n", false, nil},
		{"输入 a 视为同意", true, false, "a\n", true, nil},
		{"输入 q 退出", true, false, "q\n", false, ErrAborted},
		{"输入流结束视为退出", true, false, "", false, ErrAborted},
		{"最后一行没有换行也能识别", true, false, "y", true, nil},
		{"无法识别后重新询问", true, false, "maybe\ny\n", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrompter(strings.NewReader(tt.input), &out, tt.interactive, tt.yes)
			got, err := p.Overwrite("a.json")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("错误 = %v,期望 %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("结果 = %v,期望 %v", got, tt.want)
			}
		})
	}
}

func TestNonInteractiveDoesNotReadInput(t *testing.T) {
	// 非交互环境下不能读取输入,否则会在管道上挂起;用一个读取即失败的 Reader 验证。
	p := NewPrompter(iotest.ErrReader(errors.New("不应读取输入")), io.Discard, false, false)
	if _, err := p.Overwrite("x"); !errors.Is(err, ErrNonInteractive) {
		t.Fatalf("错误 = %v,期望 ErrNonInteractive", err)
	}
}

func TestPromptMentionsPath(t *testing.T) {
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("n\n"), &out, true, false)
	if _, err := p.Overwrite("dir/a.json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dir/a.json") {
		t.Fatalf("提示中应包含文件路径:%q", out.String())
	}
}

func TestAllRemembered(t *testing.T) {
	// 回答一次 a 之后不再询问:第二次调用时输入流已耗尽,仍应返回 true。
	var out bytes.Buffer
	p := NewPrompter(strings.NewReader("a\n"), &out, true, false)
	for i := 0; i < 3; i++ {
		ok, err := p.Overwrite("f")
		if err != nil || !ok {
			t.Fatalf("第 %d 次:%v, %v", i+1, ok, err)
		}
	}
	if n := strings.Count(out.String(), "是否覆盖"); n != 1 {
		t.Fatalf("应只询问 1 次,实际 %d 次", n)
	}
}

func TestReadError(t *testing.T) {
	boom := errors.New("读取失败")
	p := NewPrompter(iotest.ErrReader(boom), io.Discard, true, false)
	if _, err := p.Overwrite("f"); !errors.Is(err, boom) {
		t.Fatalf("错误 = %v,期望 %v", err, boom)
	}
}

func TestSequentialAnswers(t *testing.T) {
	p := NewPrompter(strings.NewReader("y\nn\ny\n"), io.Discard, true, false)
	want := []bool{true, false, true}
	for i, w := range want {
		got, err := p.Overwrite("f")
		if err != nil || got != w {
			t.Fatalf("第 %d 次 = %v, %v,期望 %v", i+1, got, err, w)
		}
	}
}

func TestCanPrompt(t *testing.T) {
	tests := []struct {
		interactive, yes, want bool
	}{
		{false, false, false}, {true, false, true}, {false, true, true}, {true, true, true},
	}
	for _, tt := range tests {
		p := NewPrompter(strings.NewReader(""), io.Discard, tt.interactive, tt.yes)
		if p.CanPrompt() != tt.want {
			t.Errorf("interactive=%v yes=%v: CanPrompt = %v", tt.interactive, tt.yes, p.CanPrompt())
		}
	}
}
