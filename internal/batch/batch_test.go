package batch

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/taodev/goyj"
	"github.com/taodev/goyj/internal/confirm"
)

// writeTree 在 dir 下按 "相对路径 → 内容" 创建文件。
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// listFiles 返回 dir 下所有文件相对路径(斜杠分隔,已排序)。
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func eq(a, b []string) bool { return strings.Join(a, "\n") == strings.Join(b, "\n") }

// stubConfirmer 按预设顺序回答,并记录被询问的路径。
type stubConfirmer struct {
	answers []bool
	err     error
	asked   []string
}

func (s *stubConfirmer) Overwrite(path string) (bool, error) {
	s.asked = append(s.asked, path)
	if s.err != nil {
		return false, s.err
	}
	if len(s.answers) == 0 {
		return false, nil
	}
	a := s.answers[0]
	s.answers = s.answers[1:]
	return a, nil
}

var compact = []goyj.Option{goyj.WithCompact(true)}

func TestSingleFileAlongside(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.yaml": "x: 1\n"})
	in := filepath.Join(dir, "a.yaml")

	res, err := Run(Options{Inputs: []string{in}, Convert: compact})
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}
	if got := readFile(t, filepath.Join(dir, "a.json")); got != "{\"x\":1}\n" {
		t.Fatalf("输出内容 = %q", got)
	}
	if readFile(t, in) != "x: 1\n" {
		t.Error("输入文件不应被修改")
	}
	if len(res.Written) != 1 {
		t.Errorf("Written = %v", res.Written)
	}
}

func TestJSONToYAMLInferred(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.json": `{"k":[1,2]}`})
	res, err := Run(Options{Inputs: []string{filepath.Join(dir, "a.json")}})
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}
	if got := readFile(t, filepath.Join(dir, "a.yaml")); got != "k:\n  - 1\n  - 2\n" {
		t.Fatalf("输出内容 = %q", got)
	}
}

func TestOutputDirKeepsStructure(t *testing.T) {
	src, out := t.TempDir(), filepath.Join(t.TempDir(), "out")
	writeTree(t, src, map[string]string{
		"a.yaml":          "a: 1\n",
		"sub/b.yml":       "b: 2\n",
		"sub/deep/c.js":   "ignored", // 无法识别的后缀
		"sub/deep/d.yaml": "d: 3\n",
		"e.json":          `{"e":4}`, // 目录扫描且未指定 --to 时,json 转 yaml
	})
	res, err := Run(Options{Inputs: []string{src}, Output: out, Convert: compact})
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}
	want := []string{"a.json", "e.yaml", "sub/b.json", "sub/deep/d.json"}
	if got := listFiles(t, out); !eq(got, want) {
		t.Fatalf("输出文件 = %v,期望 %v", got, want)
	}
	if got := listFiles(t, src); len(got) != 5 {
		t.Errorf("输入目录不应有变化:%v", got)
	}
	if got := readFile(t, filepath.Join(out, "sub", "b.json")); got != "{\"b\":2}\n" {
		t.Errorf("b.json = %q", got)
	}
}

func TestDirWithExplicitToSkipsTargetFormat(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"a.yaml": "a: 1\n", "b.json": `{"b":2}`})
	res, err := Run(Options{Inputs: []string{src}, To: "json", Convert: compact})
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}
	// b.json 已是目标格式,不应处理(否则会与自身冲突)。
	if got := listFiles(t, src); !eq(got, []string{"a.json", "a.yaml", "b.json"}) {
		t.Fatalf("文件 = %v", got)
	}
}

func TestDirWithFromFilter(t *testing.T) {
	src, out := t.TempDir(), t.TempDir()
	writeTree(t, src, map[string]string{"a.yaml": "a: 1\n", "b.json": `{"b":2}`})
	res, err := Run(Options{Inputs: []string{src}, Output: out, From: "json", Convert: compact})
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}
	if got := listFiles(t, out); !eq(got, []string{"b.yaml"}) {
		t.Fatalf("文件 = %v", got)
	}
}

func TestEmptyDirIsSkipped(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, map[string]string{"readme.txt": "x"})
	var stderr bytes.Buffer
	res, err := Run(Options{Inputs: []string{src}, Stderr: &stderr})
	if err != nil || len(res.Failed) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("err=%v failed=%v skipped=%v", err, res.Failed, res.Skipped)
	}
	if !strings.Contains(stderr.String(), "没有可转换的文件") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestMultiDocWritesNumberedFiles(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.yaml": "x: 1\n---\ny: 2\n---\nz: 3\n"})
	res, err := Run(Options{Inputs: []string{filepath.Join(dir, "a.yaml")}, Convert: compact})
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}
	if got := listFiles(t, dir); !eq(got, []string{"a-1.json", "a-2.json", "a-3.json", "a.yaml"}) {
		t.Fatalf("文件 = %v", got)
	}
	if got := readFile(t, filepath.Join(dir, "a-2.json")); got != "{\"y\":2}\n" {
		t.Errorf("a-2.json = %q", got)
	}
}

func TestEmptyDocumentsAreSkipped(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"empty.yaml": "", "comment.yaml": "# nothing\n"})
	res, err := Run(Options{Inputs: []string{dir}})
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}
	if len(res.Skipped) != 2 || len(res.Written) != 0 {
		t.Fatalf("skipped=%v written=%v", res.Skipped, res.Written)
	}
}

func TestFailureDoesNotStopBatch(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.yaml":      "a: 1\n",
		"bad.yaml":    "a: [\n",
		"c.yaml":      "c: 3\n",
		"strict.yaml": "1: x\n",
	})
	missing := filepath.Join(dir, "nope.yaml")
	res, err := Run(Options{
		Inputs:  []string{filepath.Join(dir, "a.yaml"), filepath.Join(dir, "bad.yaml"), missing, filepath.Join(dir, "strict.yaml"), filepath.Join(dir, "c.yaml")},
		Convert: []goyj.Option{goyj.WithStrict(true)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) != 3 {
		t.Fatalf("失败项 = %v", res.Failed)
	}
	var se *goyj.StrictError
	found := false
	for _, f := range res.Failed {
		if errors.As(f.Err, &se) {
			found = true
		}
	}
	if !found {
		t.Error("失败项中应包含 StrictError")
	}
	if len(res.Written) != 2 || !exists(filepath.Join(dir, "a.json")) || !exists(filepath.Join(dir, "c.json")) {
		t.Fatalf("其余文件应转换成功:%v", res.Written)
	}
}

func TestUnknownExtensionNeedsFrom(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"data.txt": "a: 1\n"})
	in := filepath.Join(dir, "data.txt")

	res, err := Run(Options{Inputs: []string{in}})
	if err != nil || len(res.Failed) != 1 || !strings.Contains(res.Failed[0].Err.Error(), "--from") {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}

	res, err = Run(Options{Inputs: []string{in}, From: "yaml", Convert: compact})
	if err != nil || len(res.Failed) != 0 {
		t.Fatalf("err=%v failed=%v", err, res.Failed)
	}
	if got := readFile(t, filepath.Join(dir, "data.json")); got != "{\"a\":1}\n" {
		t.Errorf("data.json = %q", got)
	}
}

func TestOverwrite(t *testing.T) {
	setup := func(t *testing.T) (dir, in, outPath string) {
		dir = t.TempDir()
		writeTree(t, dir, map[string]string{"a.yaml": "x: 1\n", "a.json": "OLD"})
		return dir, filepath.Join(dir, "a.yaml"), filepath.Join(dir, "a.json")
	}

	t.Run("同意覆盖", func(t *testing.T) {
		_, in, out := setup(t)
		c := &stubConfirmer{answers: []bool{true}}
		res, err := Run(Options{Inputs: []string{in}, Confirmer: c, Convert: compact})
		if err != nil || len(res.Failed) != 0 {
			t.Fatalf("err=%v failed=%v", err, res.Failed)
		}
		if len(c.asked) != 1 || c.asked[0] != out {
			t.Errorf("询问记录 = %v", c.asked)
		}
		if readFile(t, out) != "{\"x\":1}\n" {
			t.Error("文件应被覆盖")
		}
	})

	t.Run("拒绝覆盖", func(t *testing.T) {
		_, in, out := setup(t)
		res, err := Run(Options{Inputs: []string{in}, Confirmer: &stubConfirmer{answers: []bool{false}}})
		if err != nil || len(res.Failed) != 0 {
			t.Fatalf("err=%v failed=%v", err, res.Failed)
		}
		if readFile(t, out) != "OLD" || len(res.Skipped) != 1 || len(res.Written) != 0 {
			t.Errorf("文件不应被修改,skipped=%v written=%v", res.Skipped, res.Written)
		}
	})

	t.Run("非交互且无 yes 报用法错误", func(t *testing.T) {
		_, in, out := setup(t)
		p := confirm.NewPrompter(strings.NewReader(""), &bytes.Buffer{}, false, false)
		res, err := Run(Options{Inputs: []string{in}, Confirmer: p})
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("期望 UsageError,得到 %v", err)
		}
		if !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), out) {
			t.Errorf("错误信息应提示 --yes 并包含路径:%v", err)
		}
		if readFile(t, out) != "OLD" || len(res.Written) != 0 {
			t.Error("不应写出任何文件")
		}
	})

	t.Run("没有 Confirmer 等同非交互", func(t *testing.T) {
		_, in, _ := setup(t)
		_, err := Run(Options{Inputs: []string{in}})
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("期望 UsageError,得到 %v", err)
		}
	})

	t.Run("yes 直接覆盖", func(t *testing.T) {
		_, in, out := setup(t)
		p := confirm.NewPrompter(strings.NewReader(""), &bytes.Buffer{}, false, true)
		res, err := Run(Options{Inputs: []string{in}, Confirmer: p, Convert: compact})
		if err != nil || len(res.Failed) != 0 {
			t.Fatalf("err=%v failed=%v", err, res.Failed)
		}
		if readFile(t, out) != "{\"x\":1}\n" {
			t.Error("文件应被覆盖")
		}
	})

	t.Run("用户退出时不写出任何文件", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"a.yaml": "a: 1\n", "b.yaml": "b: 2\n", "b.json": "OLD"})
		c := &stubConfirmer{err: confirm.ErrAborted}
		_, err := Run(Options{Inputs: []string{filepath.Join(dir, "a.yaml"), filepath.Join(dir, "b.yaml")}, Confirmer: c})
		if !errors.Is(err, confirm.ErrAborted) {
			t.Fatalf("err = %v", err)
		}
		if exists(filepath.Join(dir, "a.json")) {
			t.Error("退出前不应写出 a.json")
		}
	})

	t.Run("非交互时不应写出其他文件", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"a.yaml": "a: 1\n", "b.yaml": "b: 2\n", "b.json": "OLD"})
		_, err := Run(Options{Inputs: []string{filepath.Join(dir, "a.yaml"), filepath.Join(dir, "b.yaml")}})
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("err = %v", err)
		}
		if exists(filepath.Join(dir, "a.json")) {
			t.Error("应先确认全部覆盖再写出,a.json 不应存在")
		}
	})

	t.Run("目标是目录", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"a.yaml": "a: 1\n", "a.json/keep": "x"})
		res, err := Run(Options{Inputs: []string{filepath.Join(dir, "a.yaml")}})
		if err != nil || len(res.Failed) != 1 {
			t.Fatalf("err=%v failed=%v", err, res.Failed)
		}
	})
}

func TestOutputsNeverOverwriteInputs(t *testing.T) {
	t.Run("同格式转换到同一路径", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"a.json": `{"a":1}`})
		in := filepath.Join(dir, "a.json")
		res, err := Run(Options{Inputs: []string{in}, To: "json", Confirmer: &stubConfirmer{answers: []bool{true}}})
		if err != nil || len(res.Failed) != 1 || !strings.Contains(res.Failed[0].Err.Error(), "输入") {
			t.Fatalf("err=%v failed=%v", err, res.Failed)
		}
		if readFile(t, in) != `{"a":1}` {
			t.Error("输入文件被修改")
		}
	})

	t.Run("互为输出的两个输入", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"a.yaml": "a: 1\n", "a.json": `{"a":1}`})
		res, err := Run(Options{Inputs: []string{dir}, Confirmer: &stubConfirmer{answers: []bool{true, true}}})
		if err != nil || len(res.Failed) != 2 || len(res.Written) != 0 {
			t.Fatalf("err=%v failed=%v written=%v", err, res.Failed, res.Written)
		}
	})

	t.Run("两个输入产生同一输出", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"a.yaml": "a: 1\n", "a.yml": "b: 2\n"})
		res, err := Run(Options{Inputs: []string{dir}, Convert: compact})
		if err != nil || len(res.Failed) != 1 || len(res.Written) != 1 {
			t.Fatalf("err=%v failed=%v written=%v", err, res.Failed, res.Written)
		}
		if !strings.Contains(res.Failed[0].Err.Error(), "冲突") {
			t.Errorf("失败原因 = %v", res.Failed[0].Err)
		}
	})
}

func TestStdin(t *testing.T) {
	run := func(o Options) (*Result, string, error) {
		var stdout bytes.Buffer
		o.Stdout = &stdout
		res, err := Run(o)
		return res, stdout.String(), err
	}

	t.Run("缺少 --from 是用法错误", func(t *testing.T) {
		_, _, err := run(Options{Stdin: strings.NewReader("a: 1")})
		var ue *UsageError
		if !errors.As(err, &ue) || !strings.Contains(err.Error(), "--from") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("显式 - 也表示标准输入", func(t *testing.T) {
		_, out, err := run(Options{Inputs: []string{"-"}, From: "yaml", Stdin: strings.NewReader("a: 1"), Convert: compact})
		if err != nil || out != "{\"a\":1}\n" {
			t.Fatalf("err=%v out=%q", err, out)
		}
	})

	t.Run("未指定 --to 时取另一种格式", func(t *testing.T) {
		_, out, err := run(Options{From: "json", Stdin: strings.NewReader(`{"a":[1]}`)})
		if err != nil || out != "a:\n  - 1\n" {
			t.Fatalf("err=%v out=%q", err, out)
		}
	})

	t.Run("多文档输出 JSON Lines", func(t *testing.T) {
		_, out, err := run(Options{From: "yaml", Stdin: strings.NewReader("a: 1\n---\nb: 2\n")})
		if err != nil || out != "{\"a\":1}\n{\"b\":2}\n" {
			t.Fatalf("err=%v out=%q", err, out)
		}
	})

	t.Run("-o 与标准输入冲突", func(t *testing.T) {
		_, _, err := run(Options{From: "yaml", Output: "out", Stdin: strings.NewReader("a: 1")})
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("转换失败记入 Failed", func(t *testing.T) {
		res, out, err := run(Options{From: "yaml", Stdin: strings.NewReader("a: ["), Convert: compact})
		if err != nil || len(res.Failed) != 1 || res.Failed[0].Path != "<stdin>" || out != "" {
			t.Fatalf("err=%v failed=%v out=%q", err, res.Failed, out)
		}
	})

	t.Run("严格模式错误", func(t *testing.T) {
		res, _, _ := run(Options{From: "yaml", Stdin: strings.NewReader("1: a"), Convert: []goyj.Option{goyj.WithStrict(true)}})
		var se *goyj.StrictError
		if len(res.Failed) != 1 || !errors.As(res.Failed[0].Err, &se) {
			t.Fatalf("failed = %v", res.Failed)
		}
	})
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		o    Options
	}{
		{"未知 --from", Options{Inputs: []string{"a.yaml"}, From: "toml"}},
		{"未知 --to", Options{Inputs: []string{"a.yaml"}, To: "toml"}},
		{"- 与其他输入混用", Options{Inputs: []string{"-", "a.yaml"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Run(tt.o)
			var ue *UsageError
			if !errors.As(err, &ue) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
