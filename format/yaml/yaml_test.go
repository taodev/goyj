package yaml

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taodev/goyj/format"
)

func decode(t *testing.T, in string, strict bool) ([]any, error) {
	t.Helper()
	return Format{}.Decode(strings.NewReader(in), &format.Options{Strict: strict})
}

func mustDecode(t *testing.T, in string) []any {
	t.Helper()
	docs, err := decode(t, in, false)
	if err != nil {
		t.Fatalf("Decode(%q) 失败:%v", in, err)
	}
	return docs
}

// TestDecodeLoose 覆盖宽松模式下的各类 YAML 边界用例。
func TestDecodeLoose(t *testing.T) {
	m := func(kv ...any) map[string]any {
		out := map[string]any{}
		for i := 0; i < len(kv); i += 2 {
			out[kv[i].(string)] = kv[i+1]
		}
		return out
	}
	tests := []struct {
		name string
		in   string
		want []any
	}{
		// 空文档
		{"空输入", "", []any{}},
		{"只有注释", "# 仅有注释\n", []any{}},
		{"显式空文档", "---\n", []any{nil}},
		{"尾部空文档", "a: 1\n---\n", []any{m("a", 1), nil}},
		{"连续空文档", "---\n---\n", []any{nil, nil}},

		// 标量类型
		{"标量类型", "s: x\ni: 3\nf: 1.5\nt: true\nn: null\ntilde: ~\nempty:\n",
			[]any{m("s", "x", "i", 3, "f", 1.5, "t", true, "n", nil, "tilde", nil, "empty", nil)}},
		{"顶层标量", "42\n", []any{42}},
		{"引号保持字符串", `a: "1"` + "\n" + `b: 'true'`, []any{m("a", "1", "b", "true")}},
		{"YAML 1.2 中 yes 是字符串", "a: yes\nb: no\n", []any{m("a", "yes", "b", "no")}},
		{"多行字符串", "a: |\n  x\n  y\nb: >\n  p\n  q\n", []any{m("a", "x\ny\n", "b", "p q\n")}},
		{"时间戳", "t: 2001-02-03T04:05:06Z\n", []any{m("t", time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC))}},

		// 嵌套结构
		{"嵌套映射与序列", "a:\n  - b: 1\n    c: [x, y]\n  - {d: {e: null}}\n",
			[]any{m("a", []any{m("b", 1, "c", []any{"x", "y"}), m("d", m("e", nil))})}},
		{"空集合", "a: []\nb: {}\n", []any{m("a", []any{}, "b", m())}},

		// 注释
		{"注释被丢弃", "# 头部\na: 1 # 行尾\n# 脚注\nb: 2\n", []any{m("a", 1, "b", 2)}},

		// 多文档
		{"多文档", "a: 1\n---\nb: 2\n---\n- 3\n", []any{m("a", 1), m("b", 2), []any{3}}},

		// 非字符串 key
		{"非字符串 key 转为字符串", "1: a\ntrue: b\n1.5: c\nnull: d\n",
			[]any{m("1", "a", "true", "b", "1.5", "c", "null", "d")}},
		{"十六进制 key 使用规范化值", "0x10: a\n", []any{m("16", "a")}},

		// 锚点、别名、合并键
		{"锚点与别名展开", "a: &x [1, 2]\nb: *x\n", []any{m("a", []any{1, 2}, "b", []any{1, 2})}},
		{"别名标量", "a: &v hello\nb: *v\n", []any{m("a", "hello", "b", "hello")}},
		{"别名作为 key", "&k a: 1\n*k : 2\n", []any{m("a", 2)}},
		{"合并键", "base: &b {x: 1, y: 2}\nd:\n  <<: *b\n  y: 3\n", []any{m(
			"base", m("x", 1, "y", 2), "d", m("x", 1, "y", 3))}},
		{"合并键显式键优先,与书写顺序无关", "base: &b {y: 2}\nd:\n  y: 3\n  <<: *b\n", []any{m(
			"base", m("y", 2), "d", m("y", 3))}},
		{"合并键序列中靠前者优先", "a: &a {k: 1}\nb: &b {k: 2, z: 9}\nc:\n  <<: [*a, *b]\n", []any{m(
			"a", m("k", 1), "b", m("k", 2, "z", 9), "c", m("k", 1, "z", 9))}},

		// 重复 key
		{"重复 key 后者生效", "a: 1\na: 2\n", []any{m("a", 2)}},
		{"归一化后冲突的 key 后者生效", "1: x\n\"1\": y\n", []any{m("1", "y")}},

		// 特殊浮点数
		{"无穷大与 NaN 保留为文本", "a: .inf\nb: -.inf\nc: .nan\n", []any{m("a", ".inf", "b", "-.inf", "c", ".nan")}},

		// 自定义标签
		{"自定义标签被忽略", "a: !foo bar\nb: !bar [1]\n", []any{m("a", "bar", "b", []any{1})}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustDecode(t, tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("得到 %#v\n期望 %#v", got, tt.want)
			}
		})
	}
}

func TestDecodeLooseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"语法错误", "a: [", "YAML 解析失败"},
		{"递归别名", "a: &x [*x]\n", "递归"},
		{"复合 key", "? [1, 2]\n: v\n", "复合类型"},
		{"合并键的值不是映射", "a:\n  <<: 5\n", "合并键"},
		{"合并键序列含非映射", "a:\n  <<: [1]\n", "合并键"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := decode(t, tt.in, false)
			if err == nil {
				t.Fatalf("期望报错,得到 %#v", docs)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("错误 %q 不包含 %q", err, tt.want)
			}
		})
	}
}

// TestDecodeStrict 检查严格模式的报错内容、行列号与路径。
func TestDecodeStrict(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		line, col  int
		path       string
		wantReason string
	}{
		{"非字符串 key(整数)", "a: 1\n2: b\n", 2, 1, "", "非字符串 key"},
		{"非字符串 key(布尔)", "x:\n  true: v\n", 2, 3, "x", "无法无损转换为 JSON 字符串 key"},
		{"非字符串 key(null)", "~: v\n", 1, 1, "", "!!null"},
		{"锚点", "a: &x 1\n", 1, 4, "a", "锚点 &x"},
		{"别名", "a: &x 1\nb: *x\n", 1, 4, "a", "锚点 &x"}, // 先遇到锚点
		{"合并键", "<<: {a: 1}\n", 1, 1, "", "合并键"},
		{"重复 key", "a: 1\nb: 2\na: 3\n", 3, 1, "a", `重复的 key "a"`},
		{"归一化后冲突的重复 key", "1: x\n\"1\": y\n", 1, 1, "", "非字符串 key"},
		{"时间戳", "t: 2001-02-03\n", 1, 4, "t", "时间戳"},
		{"无穷大", "l:\n  - 1\n  - .inf\n", 3, 5, "l[1]", "浮点数"},
		{"NaN", "a: .nan\n", 1, 4, "a", "浮点数"},
		{"二进制", "a: !!binary aGk=\n", 1, 4, "a", "二进制"},
		{"自定义标量标签", "a: !foo bar\n", 1, 4, "a", "标签 !foo"},
		{"自定义序列标签", "a: !foo [1]\n", 1, 4, "a", "标签 !foo"},
		{"嵌套路径", "a:\n  b:\n    - c: 1\n      7: x\n", 4, 7, "a.b[0]", "非字符串 key"},
		{"后续文档中的问题", "a: 1\n---\n3: x\n", 3, 1, "", "非字符串 key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decode(t, tt.in, true)
			var se *format.StrictError
			if !errors.As(err, &se) {
				t.Fatalf("期望 *StrictError,得到 %v", err)
			}
			if se.Line != tt.line || se.Column != tt.col {
				t.Errorf("位置 = %d:%d,期望 %d:%d(%v)", se.Line, se.Column, tt.line, tt.col, err)
			}
			if se.Path != tt.path {
				t.Errorf("路径 = %q,期望 %q", se.Path, tt.path)
			}
			if !strings.Contains(se.Reason, tt.wantReason) {
				t.Errorf("原因 %q 不包含 %q", se.Reason, tt.wantReason)
			}
		})
	}
}

// TestDecodeStrictAccepts 确认严格模式不会误伤可以无损转换的输入。
func TestDecodeStrictAccepts(t *testing.T) {
	inputs := []string{
		"",
		"# 注释不算损失\na: 1 # 行尾注释\n",
		"a: [1, 2.5, true, null, x]\nb: {c: \"1\"}\n",
		"a: 1\n---\nb: 2\n",
		"\"1\": a\n\"true\": b\n",
		"a: !!str 123\nb: !!int \"7\"\n",
		"a: |\n  文本\n",
	}
	for _, in := range inputs {
		if _, err := decode(t, in, true); err != nil {
			t.Errorf("严格模式不应拒绝 %q:%v", in, err)
		}
	}
}

func TestAliasBomb(t *testing.T) {
	// 每层把上一层引用 9 次,共 9 层:展开后节点数远超上限。
	var b strings.Builder
	b.WriteString("a0: &a0 [x, x, x, x, x, x, x, x, x]\n")
	prev := "a0"
	for i := 1; i <= 9; i++ {
		name := "a" + string(rune('0'+i))
		b.WriteString(name + ": &" + name + " [")
		b.WriteString(strings.TrimSuffix(strings.Repeat("*"+prev+", ", 9), ", "))
		b.WriteString("]\n")
		prev = name
	}
	_, err := decode(t, b.String(), false)
	if err == nil || !strings.Contains(err.Error(), "别名炸弹") {
		t.Fatalf("期望别名炸弹错误,得到 %v", err)
	}
}

func TestEncode(t *testing.T) {
	doc := map[string]any{
		"b": []any{int64(1), "x", map[string]any{"k": nil}},
		"a": map[string]any{"z": true},
	}
	tests := []struct {
		name string
		opts *format.Options
		want string
	}{
		{"默认缩进 2 且键有序", nil, "a:\n  z: true\nb:\n  - 1\n  - x\n  - k: null\n"},
		{"Compact 对 YAML 无效", &format.Options{Compact: true}, "a:\n  z: true\nb:\n  - 1\n  - x\n  - k: null\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := (Format{}).Encode(&buf, doc, tt.opts); err != nil {
				t.Fatal(err)
			}
			if buf.String() != tt.want {
				t.Fatalf("得到:\n%q\n期望:\n%q", buf.String(), tt.want)
			}
		})
	}
}

func TestEncodeIndent(t *testing.T) {
	doc := map[string]any{"a": map[string]any{"b": map[string]any{"c": 1}}}
	for _, n := range []int{2, 3, 4, 9} {
		var buf bytes.Buffer
		if err := (Format{}).Encode(&buf, doc, &format.Options{Indent: n}); err != nil {
			t.Fatal(err)
		}
		want := "a:\n" + strings.Repeat(" ", n) + "b:\n" + strings.Repeat(" ", 2*n) + "c: 1\n"
		if buf.String() != want {
			t.Errorf("缩进 %d:得到 %q,期望 %q", n, buf.String(), want)
		}
	}
	for _, n := range []int{1, 10, -1} {
		if err := (Format{}).Encode(&bytes.Buffer{}, doc, &format.Options{Indent: n}); err == nil {
			t.Errorf("缩进 %d 应报错", n)
		}
	}
}

func TestEncodeScalars(t *testing.T) {
	tests := []struct {
		name string
		doc  any
		want string
	}{
		{"nil", nil, "null\n"},
		{"整数", int64(7), "7\n"},
		{"uint64 最大值", uint64(18446744073709551615), "18446744073709551615\n"},
		{"浮点数", 2.5, "2.5\n"},
		{"数字形态的字符串会加引号", "123", "'123'\n"},
		{"布尔形态的字符串会加引号", "true", "'true'\n"},
		{"多行字符串", "a\nb\n", "|\n  a\n  b\n"},
		{"空切片", []any{}, "[]\n"},
		{"空映射", map[string]any{}, "{}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := (Format{}).Encode(&buf, tt.doc, nil); err != nil {
				t.Fatal(err)
			}
			if buf.String() != tt.want {
				t.Fatalf("得到 %q,期望 %q", buf.String(), tt.want)
			}
		})
	}
}

func TestJoin(t *testing.T) {
	got, err := (Format{}).Join([][]byte{[]byte("a: 1\n"), []byte("b: 2"), []byte("- 3\n")})
	if err != nil {
		t.Fatal(err)
	}
	if want := "a: 1\n---\nb: 2\n---\n- 3\n"; string(got) != want {
		t.Fatalf("得到 %q,期望 %q", got, want)
	}
}

func TestRegistered(t *testing.T) {
	for _, ext := range []string{".yaml", ".yml", ".YML"} {
		f, ok := format.ByExt(ext)
		if !ok || f.Name() != "yaml" {
			t.Errorf("后缀 %s 未注册为 yaml:%v %v", ext, f, ok)
		}
	}
	if _, ok := format.Lookup("yaml"); !ok {
		t.Error("yaml 未注册")
	}
}

func TestEncodeSequenceIndent(t *testing.T) {
	doc := map[string]any{"a": []any{map[string]any{"b": []any{int64(1)}}}}
	var buf bytes.Buffer
	if err := (Format{}).Encode(&buf, doc, &format.Options{Indent: 4}); err != nil {
		t.Fatal(err)
	}
	if want := "a:\n    - b:\n        - 1\n"; buf.String() != want {
		t.Fatalf("得到 %q,期望 %q", buf.String(), want)
	}
}

// TestEncodeRoundTripsRiskyStrings 是回归测试:yaml v4 rc.6 对以空白或换行开头的多行字符串
// 默认输出缺少缩进指示符的 "|" 字面块,读回来会丢失空白甚至解析失败。
// Encode 必须保证这类字符串(作为值或 key)都能无损往返。
func TestEncodeRoundTripsRiskyStrings(t *testing.T) {
	risky := []string{
		"\n 0",
		"\n  a",
		"\n\n  a\n",
		" a\nb",
		"\ta\nb",
		"\t  a\n",
		" \n #",
		" \n\ta",
		"  leading\nsecond",
		"\n",
		"\n\n",
		// 这些不受影响,但应保持正常往返。
		"a\nb",
		"a\nb\n",
		"a\n b",
		"",
		" ",
		"plain",
	}
	contexts := map[string]func(s string) any{
		"顶层":  func(s string) any { return s },
		"映射值": func(s string) any { return map[string]any{"k": map[string]any{"k": s}} },
		"序列":  func(s string) any { return []any{[]any{s}} },
		"映射键": func(s string) any { return map[string]any{s: int64(1), "other": "x"} },
	}
	for _, s := range risky {
		for name, build := range contexts {
			doc := build(s)
			var buf bytes.Buffer
			if err := (Format{}).Encode(&buf, doc, nil); err != nil {
				t.Fatalf("%s %q: 编码失败:%v", name, s, err)
			}
			back, err := Format{}.Decode(&buf, nil)
			if err != nil {
				t.Fatalf("%s %q: 读回失败:%v\nYAML: %q", name, s, err, buf.String())
			}
			if len(back) != 1 || !reflect.DeepEqual(normalize(back[0]), normalize(doc)) {
				t.Errorf("%s %q: 往返不一致\n得到 %#v\n期望 %#v", name, s, back, doc)
			}
		}
	}
}

// normalize 把整数统一为 int64,便于比较 Decode(int)与手写文档(int64)。
func normalize(v any) any {
	switch x := v.(type) {
	case int:
		return int64(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalize(e)
		}
		return out
	}
	return v
}

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"abc", false},
		{"a\nb", false},
		{"a\n b", false},
		{" a", false}, // 单行字符串由 Dumper 自行加引号,不会成为字面块
		{"\n", true},
		{"\na", true},
		{" a\nb", true},
		{"\ta\nb", true},
		{" \na", true},
		{" a\nb", true},
	}
	for _, tt := range tests {
		if got := needsQuoting(tt.in); got != tt.want {
			t.Errorf("needsQuoting(%q) = %v,期望 %v", tt.in, got, tt.want)
		}
	}
}
