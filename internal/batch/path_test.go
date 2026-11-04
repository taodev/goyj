package batch

import (
	"path/filepath"
	"testing"
)

func TestOutputPath(t *testing.T) {
	tests := []struct {
		name         string
		src, root    string
		outDir, ext  string
		index, total int
		want         string
	}{
		// 未指定 -o:输出到输入文件所在目录
		{"原目录换后缀", "a.yaml", "", "", ".json", 1, 1, "a.json"},
		{"原目录带子目录", "conf/a.yml", "", "", ".json", 1, 1, "conf/a.json"},
		{"多文档带序号", "conf/a.yaml", "", "", ".json", 2, 3, "conf/a-2.json"},
		{"文件名含多个点只替换最后一级", "conf/a.b.yaml", "", "", ".json", 1, 1, "conf/a.b.json"},
		{"没有后缀", "conf/a", "", "", ".json", 1, 1, "conf/a.json"},

		// 指定 -o,输入是文件:直接放在输出目录
		{"文件输出到目录", "conf/a.yaml", "", "out", ".json", 1, 1, "out/a.json"},
		{"文件多文档输出到目录", "conf/a.yaml", "", "out", ".json", 1, 2, "out/a-1.json"},

		// 指定 -o,输入来自目录:保持相对结构
		{"目录顶层文件", "conf/a.yaml", "conf", "out", ".json", 1, 1, "out/a.json"},
		{"目录子目录文件", "conf/x/y/a.yaml", "conf", "out", ".json", 1, 1, "out/x/y/a.json"},
		{"目录多文档保持结构", "conf/x/a.yaml", "conf", "out", ".json", 2, 2, "out/x/a-2.json"},
		{"输入目录带末尾斜杠", "conf/x/a.yaml", "conf/", "out", ".json", 1, 1, "out/x/a.json"},
		{"输入目录是当前目录", "x/a.yaml", ".", "out", ".json", 1, 1, "out/x/a.json"},
		{"输出目录在输入目录内", "conf/x/a.yaml", "conf", "conf/gen", ".json", 1, 1, "conf/gen/x/a.json"},

		// 未指定 -o,输入来自目录:仍在原位置
		{"目录输入无 -o", "conf/x/a.yaml", "conf", "", ".json", 1, 1, "conf/x/a.json"},

		// JSON → YAML
		{"json 转 yaml", "conf/a.json", "", "", ".yaml", 1, 1, "conf/a.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OutputPath(filepath.FromSlash(tt.src), filepath.FromSlash(tt.root),
				filepath.FromSlash(tt.outDir), tt.ext, tt.index, tt.total)
			if want := filepath.FromSlash(tt.want); got != want {
				t.Fatalf("OutputPath = %q,期望 %q", got, want)
			}
		})
	}
}

func TestStem(t *testing.T) {
	tests := map[string]string{
		"a.yaml":        "a",
		"dir/a.b.json":  "a.b",
		"dir/noext":     "noext",
		".hidden":       "",
		"dir/.hidden.y": ".hidden",
	}
	for in, want := range tests {
		if got := stem(filepath.FromSlash(in)); got != want {
			t.Errorf("stem(%q) = %q,期望 %q", in, got, want)
		}
	}
}
