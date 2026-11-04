package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stdinFrom 返回一个内容为 s 的 *os.File(管道读端,不是终端)。
func stdinFrom(t *testing.T, s string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(s); err != nil {
		t.Fatal(err)
	}
	w.Close()
	t.Cleanup(func() { r.Close() })
	return r
}

// runCLI 以给定的标准输入运行命令,返回退出码、标准输出与标准错误。
func runCLI(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = run(args, stdinFrom(t, stdin), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestExitCodes(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	bad := filepath.Join(dir, "bad.yaml")
	write(t, good, "a: 1\n")
	write(t, bad, "a: [\n")

	tests := []struct {
		name  string
		stdin string
		args  []string
		want  int
		msg   string // stderr 中应包含的内容
	}{
		{"成功", "", []string{good}, 0, ""},
		{"转换失败是 1", "", []string{bad}, 1, "1 个输入处理失败"},
		{"文件不存在是 1", "", []string{filepath.Join(dir, "none.yaml")}, 1, "none.yaml"},
		{"未知标志是 2", "", []string{"--bogus"}, 2, "unknown flag"},
		{"stdin 缺少 --from 是 2", "a: 1", nil, 2, "--from"},
		{"缩进过小是 2", "", []string{"--indent", "1", good}, 2, "--indent"},
		{"缩进过大是 2", "", []string{"--indent", "10", good}, 2, "--indent"},
		{"缩进不是数字是 2", "", []string{"--indent", "x", good}, 2, ""},
		{"未知 --from 是 2", "", []string{"--from", "toml", good}, 2, "toml"},
		{"未知 --to 是 2", "", []string{"--to", "toml", good}, 2, "toml"},
		{"stdin 与 -o 同用是 2", "a: 1", []string{"--from", "yaml", "-o", dir}, 2, "-o"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := runCLI(t, tt.stdin, tt.args...)
			if code != tt.want {
				t.Fatalf("退出码 = %d,期望 %d\nstderr: %s", code, tt.want, stderr)
			}
			if !strings.Contains(stderr, tt.msg) {
				t.Errorf("stderr 不包含 %q:\n%s", tt.msg, stderr)
			}
		})
	}
}

func TestStdinToStdout(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"yaml 到 json", "a: 1\n", []string{"--from", "yaml", "--compact"}, "{\"a\":1}\n"},
		{"json 到 yaml", `{"a":[1]}`, []string{"--from", "json"}, "a:\n  - 1\n"},
		{"显式 --to", "a: 1\n", []string{"--from", "yaml", "--to", "yaml"}, "a: 1\n"},
		{"缩进 4", "a: {b: 1}\n", []string{"--from", "yaml", "--indent", "4"}, "{\n    \"a\": {\n        \"b\": 1\n    }\n}\n"},
		{"多文档为 JSON Lines", "a: 1\n---\nb: 2\n", []string{"--from", "yaml"}, "{\"a\":1}\n{\"b\":2}\n"},
		{"- 表示标准输入", "a: 1\n", []string{"--from", "yaml", "--compact", "-"}, "{\"a\":1}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tt.stdin, tt.args...)
			if code != 0 {
				t.Fatalf("退出码 = %d\nstderr: %s", code, stderr)
			}
			if stdout != tt.want {
				t.Fatalf("stdout = %q,期望 %q", stdout, tt.want)
			}
		})
	}
}

func TestStdinStrictFailure(t *testing.T) {
	code, stdout, stderr := runCLI(t, "1: a\n", "--from", "yaml", "--strict")
	if code != 1 {
		t.Fatalf("退出码 = %d,期望 1\n%s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("失败时不应输出:%q", stdout)
	}
	for _, want := range []string{"<stdin>", "第 1 行第 1 列", "非字符串 key"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr 缺少 %q:\n%s", want, stderr)
		}
	}
}

func TestFileConversionAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.yaml")
	out := filepath.Join(dir, "a.json")
	write(t, in, "a: 1\n")

	if code, _, stderr := runCLI(t, "", "--compact", in); code != 0 {
		t.Fatalf("首次转换退出码 = %d\n%s", code, stderr)
	}
	if got := read(t, out); got != "{\"a\":1}\n" {
		t.Fatalf("a.json = %q", got)
	}

	// 目标已存在,stdin 不是终端,未指定 -y:退出码 2,文件不变。
	write(t, in, "a: 2\n")
	code, _, stderr := runCLI(t, "y\n", "--compact", in)
	if code != 2 {
		t.Fatalf("退出码 = %d,期望 2\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Errorf("应提示使用 --yes:%s", stderr)
	}
	if got := read(t, out); got != "{\"a\":1}\n" {
		t.Fatalf("文件不应被改动:%q", got)
	}

	// 指定 -y(以及长选项 --yes)后覆盖。
	for _, flag := range []string{"-y", "--yes"} {
		write(t, in, "a: 3\n")
		if code, _, stderr := runCLI(t, "", flag, "--compact", in); code != 0 {
			t.Fatalf("%s 退出码 = %d\n%s", flag, code, stderr)
		}
		if got := read(t, out); got != "{\"a\":3}\n" {
			t.Fatalf("%s 后 a.json = %q", flag, got)
		}
	}
}

func TestOutputDirectory(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")
	write(t, filepath.Join(src, "a.yaml"), "a: 1\n")
	write(t, filepath.Join(src, "sub", "b.yml"), "b: 2\n---\nc: 3\n")

	code, _, stderr := runCLI(t, "", "-o", dst, "--compact", src)
	if code != 0 {
		t.Fatalf("退出码 = %d\n%s", code, stderr)
	}
	if got := read(t, filepath.Join(dst, "a.json")); got != "{\"a\":1}\n" {
		t.Errorf("a.json = %q", got)
	}
	if got := read(t, filepath.Join(dst, "sub", "b-2.json")); got != "{\"c\":3}\n" {
		t.Errorf("sub/b-2.json = %q", got)
	}
	if _, err := os.Stat(filepath.Join(src, "a.json")); err == nil {
		t.Error("指定 -o 时不应在原目录生成文件")
	}
}

func TestBatchFailureSummary(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ok.yaml"), "a: 1\n")
	write(t, filepath.Join(dir, "bad1.yaml"), "a: [\n")
	write(t, filepath.Join(dir, "bad2.yaml"), "b: {\n")

	code, _, stderr := runCLI(t, "", dir)
	if code != 1 {
		t.Fatalf("退出码 = %d,期望 1", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "ok.json")); err != nil {
		t.Error("成功的文件应已写出")
	}
	for _, want := range []string{"2 个输入处理失败", "bad1.yaml", "bad2.yaml"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("汇总缺少 %q:\n%s", want, stderr)
		}
	}
}

func TestVersionAndHelp(t *testing.T) {
	code, stdout, _ := runCLI(t, "", "--version")
	if code != 0 || !strings.HasPrefix(stdout, "goyj ") {
		t.Fatalf("--version: code=%d stdout=%q", code, stdout)
	}

	old := version
	version = "v1.2.3"
	defer func() { version = old }()
	if _, stdout, _ := runCLI(t, "", "--version"); stdout != "goyj v1.2.3\n" {
		t.Errorf("注入版本后 --version = %q", stdout)
	}

	code, stdout, _ = runCLI(t, "", "--help")
	if code != 0 {
		t.Fatalf("--help 退出码 = %d", code)
	}
	for _, flag := range []string{"--output", "--from", "--to", "--strict", "--indent", "--compact", "--yes", "--version"} {
		if !strings.Contains(stdout, flag) {
			t.Errorf("--help 缺少 %s", flag)
		}
	}
}
