package goyj_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/taodev/goyj"
)

func TestYAMLToJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		opts []goyj.Option
		want string
	}{
		{"简单映射", "a: 1\nb: [x, y]\n", nil, "{\n  \"a\": 1,\n  \"b\": [\n    \"x\",\n    \"y\"\n  ]\n}\n"},
		{"紧凑输出", "a: 1\nb: [x, y]\n", []goyj.Option{goyj.WithCompact(true)}, "{\"a\":1,\"b\":[\"x\",\"y\"]}\n"},
		{"缩进 4", "a: {b: 1}\n", []goyj.Option{goyj.WithIndent(4)}, "{\n    \"a\": {\n        \"b\": 1\n    }\n}\n"},
		{"锚点与别名展开", "a: &x {k: v}\nb: *x\n", []goyj.Option{goyj.WithCompact(true)}, "{\"a\":{\"k\":\"v\"},\"b\":{\"k\":\"v\"}}\n"},
		{"非字符串 key 转字符串", "1: a\ntrue: b\n", []goyj.Option{goyj.WithCompact(true)}, "{\"1\":\"a\",\"true\":\"b\"}\n"},
		{"注释丢弃", "# c\na: 1 # d\n", []goyj.Option{goyj.WithCompact(true)}, "{\"a\":1}\n"},
		{"顶层标量", "hello\n", nil, "\"hello\"\n"},
		{"空输入", "", nil, ""},
		{"只有注释", "# c\n", nil, ""},
		{"显式空文档", "---\n", nil, "null\n"},
		{"多文档输出 JSON Lines", "a: 1\n---\nb: [1, 2]\n---\n", nil, "{\"a\":1}\n{\"b\":[1,2]}\nnull\n"},
		{"多文档忽略 indent 并保持单行", "a: 1\n---\nb: 2\n", []goyj.Option{goyj.WithIndent(4)}, "{\"a\":1}\n{\"b\":2}\n"},
		{"严格模式接受可无损转换的输入", "a: 1\n", []goyj.Option{goyj.WithStrict(true), goyj.WithCompact(true)}, "{\"a\":1}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := goyj.YAMLToJSON([]byte(tt.in), tt.opts...)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("得到 %q\n期望 %q", got, tt.want)
			}
		})
	}
}

func TestJSONToYAML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		opts []goyj.Option
		want string
	}{
		{"简单对象", `{"b":[1,2],"a":"x"}`, nil, "a: x\nb:\n  - 1\n  - 2\n"},
		{"缩进 4", `{"a":{"b":1}}`, []goyj.Option{goyj.WithIndent(4)}, "a:\n    b: 1\n"},
		{"Compact 对 YAML 无效", `{"a":1}`, []goyj.Option{goyj.WithCompact(true)}, "a: 1\n"},
		{"大整数不丢精度", `{"num":18446744073709551615}`, nil, "num: 18446744073709551615\n"},
		{"需要引用的字符串", `{"a":"123","b":"true","c":"x: y"}`, nil, "a: '123'\nb: 'true'\nc: 'x: y'\n"},
		{"null", `null`, nil, "null\n"},
		{"空输入", ``, nil, ""},
		{"多个 JSON 值输出多文档", "{\"a\":1}\n{\"b\":2}\n", nil, "a: 1\n---\nb: 2\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := goyj.JSONToYAML([]byte(tt.in), tt.opts...)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("得到 %q\n期望 %q", got, tt.want)
			}
		})
	}
}

func TestStrictErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"非字符串 key", "a: 1\n2: x\n", "第 2 行第 1 列"},
		{"锚点", "a: &x 1\n", "锚点 &x"},
		{"重复 key", "a: 1\na: 2\n", "重复的 key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := goyj.YAMLToJSON([]byte(tt.in), goyj.WithStrict(true))
			var se *goyj.StrictError
			if !errors.As(err, &se) {
				t.Fatalf("期望 *StrictError,得到 %v", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("错误 %q 不包含 %q", err, tt.want)
			}
			// 宽松模式下同一输入必须能成功转换。
			if _, err := goyj.YAMLToJSON([]byte(tt.in)); err != nil {
				t.Fatalf("宽松模式不应失败:%v", err)
			}
		})
	}
}

func TestInvalidInput(t *testing.T) {
	if _, err := goyj.YAMLToJSON([]byte("a: [")); err == nil {
		t.Error("非法 YAML 应报错")
	}
	if _, err := goyj.JSONToYAML([]byte(`{"a":`)); err == nil {
		t.Error("非法 JSON 应报错")
	}
	if _, err := goyj.JSONToYAML([]byte(`{"a":1,"a":2}`)); err == nil {
		t.Error("JSON 重复键应报错")
	}
}

func TestInvalidIndent(t *testing.T) {
	for _, n := range []int{-1, 1, 10, 100} {
		if _, err := goyj.YAMLToJSON([]byte("a: 1"), goyj.WithIndent(n)); err == nil {
			t.Errorf("YAMLToJSON 缩进 %d 应报错", n)
		}
		if _, err := goyj.JSONToYAML([]byte(`{"a":1}`), goyj.WithIndent(n)); err == nil {
			t.Errorf("JSONToYAML 缩进 %d 应报错", n)
		}
	}
	// 0 表示默认值。
	if _, err := goyj.YAMLToJSON([]byte("a: 1"), goyj.WithIndent(0)); err != nil {
		t.Errorf("缩进 0 应使用默认值:%v", err)
	}
}

func TestNilOption(t *testing.T) {
	if _, err := goyj.YAMLToJSON([]byte("a: 1"), nil); err != nil {
		t.Fatalf("nil 选项应被忽略:%v", err)
	}
}

func TestLaterOptionWins(t *testing.T) {
	got, err := goyj.YAMLToJSON([]byte("a: 1"), goyj.WithCompact(true), goyj.WithCompact(false))
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"a\": 1\n}\n"; string(got) != want {
		t.Fatalf("得到 %q,期望 %q", got, want)
	}
}

func TestConvertUnknownFormat(t *testing.T) {
	for _, c := range [][2]string{{"toml", "json"}, {"yaml", "toml"}, {"", "json"}} {
		_, err := goyj.Convert([]byte("a: 1"), c[0], c[1])
		if err == nil || !strings.Contains(err.Error(), "未知格式") {
			t.Errorf("Convert(%q→%q) 错误 = %v", c[0], c[1], err)
		}
	}
	// 未知格式的错误里应列出可用格式。
	_, err := goyj.Convert(nil, "toml", "json")
	if err == nil || !strings.Contains(err.Error(), "json") || !strings.Contains(err.Error(), "yaml") {
		t.Errorf("错误信息应列出可用格式:%v", err)
	}
	if _, err := goyj.JoinDocs("toml", nil); err == nil {
		t.Error("JoinDocs 对未知格式应报错")
	}
}

func TestConvertDocs(t *testing.T) {
	docs, err := goyj.ConvertDocs([]byte("a: 1\n---\nb: 2\n"), "yaml", "json", goyj.WithCompact(true))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(docs))
	for i, d := range docs {
		got[i] = string(d)
	}
	if want := []string{"{\"a\":1}\n", "{\"b\":2}\n"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("得到 %q,期望 %q", got, want)
	}

	// 逐文档结果保持原缩进,合并时才压缩为 JSON Lines。
	docs, err = goyj.ConvertDocs([]byte("a: 1\n---\nb: 2\n"), "yaml", "json")
	if err != nil {
		t.Fatal(err)
	}
	if string(docs[0]) != "{\n  \"a\": 1\n}\n" {
		t.Errorf("逐文档输出应保持缩进:%q", docs[0])
	}
	joined, err := goyj.JoinDocs("json", docs)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\"a\":1}\n{\"b\":2}\n"; string(joined) != want {
		t.Errorf("JoinDocs = %q,期望 %q", joined, want)
	}

	empty, err := goyj.ConvertDocs(nil, "yaml", "json")
	if err != nil || len(empty) != 0 {
		t.Errorf("空输入应返回空切片,得到 %v, %v", empty, err)
	}
}

// TestRoundTrip 检查 JSON→YAML→JSON 与 YAML→JSON→YAML 的值保持不变。
func TestRoundTrip(t *testing.T) {
	jsonDocs := []string{
		`{"a":1,"b":[true,false,null],"c":{"d":"e","f":2.5},"g":"","h":[]}`,
		`[1,"2",3.5,{"k":"v"}]`,
		`{"s":"multi\nline","t":"  spaces  ","u":"#hash","v":"- dash","w":"null","x":"~"}`,
		`{"unicode":"你好 🌏","empty":{}}`,
		`"just a string"`,
		`42`,
	}
	for _, in := range jsonDocs {
		y, err := goyj.JSONToYAML([]byte(in))
		if err != nil {
			t.Fatalf("JSONToYAML(%s):%v", in, err)
		}
		back, err := goyj.YAMLToJSON(y, goyj.WithCompact(true), goyj.WithStrict(true))
		if err != nil {
			t.Fatalf("YAMLToJSON(%q):%v", y, err)
		}
		want, err := goyj.Convert([]byte(in), "json", "json", goyj.WithCompact(true))
		if err != nil {
			t.Fatal(err)
		}
		if string(back) != string(want) {
			t.Errorf("往返不一致\n输入: %s\nYAML: %q\n得到: %s期望: %s", in, y, back, want)
		}
	}
}

// lineFormat 是一个玩具格式:每行一个字符串文档,用来验证新增格式无需修改已有代码。
type lineFormat struct{}

func (lineFormat) Name() string         { return "lines" }
func (lineFormat) Extensions() []string { return []string{".lines", ".txt"} }
func (lineFormat) Decode(r io.Reader, _ *goyj.Options) ([]any, error) {
	docs := []any{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		docs = append(docs, sc.Text())
	}
	return docs, sc.Err()
}
func (lineFormat) Encode(w io.Writer, doc any, _ *goyj.Options) error {
	_, err := fmt.Fprintln(w, doc)
	return err
}

func TestRegisterCustomFormat(t *testing.T) {
	goyj.Register(lineFormat{})

	if f, ok := goyj.LookupFormat("LINES"); !ok || f.Name() != "lines" {
		t.Fatalf("LookupFormat 失败:%v %v", f, ok)
	}
	if f, ok := goyj.FormatByExt("txt"); !ok || f.Name() != "lines" {
		t.Fatalf("FormatByExt 失败:%v %v", f, ok)
	}

	got, err := goyj.Convert([]byte("one\ntwo\n"), "lines", "json", goyj.WithCompact(true))
	if err != nil {
		t.Fatal(err)
	}
	if want := "\"one\"\n\"two\"\n"; string(got) != want {
		t.Fatalf("lines→json = %q,期望 %q", got, want)
	}
	got, err = goyj.Convert([]byte(`["a","b"]`), "json", "lines")
	if err != nil {
		t.Fatal(err)
	}
	if want := "[a b]\n"; string(got) != want {
		t.Fatalf("json→lines = %q,期望 %q", got, want)
	}

	names := goyj.FormatNames()
	if !reflect.DeepEqual(names, []string{"json", "lines", "yaml"}) {
		t.Fatalf("FormatNames = %v", names)
	}
}

func TestBuiltinFormatsRegistered(t *testing.T) {
	for ext, name := range map[string]string{".yaml": "yaml", ".yml": "yaml", ".json": "json", ".JSON": "json"} {
		f, ok := goyj.FormatByExt(ext)
		if !ok || f.Name() != name {
			t.Errorf("后缀 %s → %v %v,期望 %s", ext, f, ok, name)
		}
	}
}

func ExampleYAMLToJSON() {
	out, err := goyj.YAMLToJSON([]byte("name: goyj\ntags: [yaml, json]\n"), goyj.WithCompact(true))
	if err != nil {
		panic(err)
	}
	fmt.Print(string(out))
	// Output: {"name":"goyj","tags":["yaml","json"]}
}

func ExampleJSONToYAML() {
	out, err := goyj.JSONToYAML([]byte(`{"name":"goyj","tags":["yaml","json"]}`))
	if err != nil {
		panic(err)
	}
	fmt.Print(string(out))
	// Output:
	// name: goyj
	// tags:
	//   - yaml
	//   - json
}

func ExampleWithStrict() {
	_, err := goyj.YAMLToJSON([]byte("1: a\n"), goyj.WithStrict(true))
	var se *goyj.StrictError
	if errors.As(err, &se) {
		fmt.Println("行", se.Line, "列", se.Column)
	}
	// Output: 行 1 列 1
}
