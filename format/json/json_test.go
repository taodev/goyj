package json

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/taodev/goyj/format"
)

func decode(t *testing.T, in string) []any {
	t.Helper()
	docs, err := Format{}.Decode(strings.NewReader(in), nil)
	if err != nil {
		t.Fatalf("Decode(%q) 失败:%v", in, err)
	}
	return docs
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []any
	}{
		{"空输入没有文档", "", []any{}},
		{"只有空白", " \n\t ", []any{}},
		{"null", "null", []any{nil}},
		{"布尔", "true false", []any{true, false}},
		{"字符串", `"hi\n"`, []any{"hi\n"}},
		{"整数保持为 int64", "42", []any{int64(42)}},
		{"负整数", "-7", []any{int64(-7)}},
		{"超出 int64 的整数保持为 uint64", "18446744073709551615", []any{uint64(math.MaxUint64)}},
		{"浮点数", "2.5", []any{2.5}},
		{"指数记法", "1e3", []any{1000.0}},
		{"空对象与空数组", "{} []", []any{map[string]any{}, []any{}}},
		{"嵌套结构", `{"a":[1,{"b":null}],"c":"d"}`, []any{map[string]any{
			"a": []any{int64(1), map[string]any{"b": nil}},
			"c": "d",
		}}},
		{"多个顶层值(JSON Lines)", "{\"a\":1}\n{\"a\":2}\n", []any{
			map[string]any{"a": int64(1)}, map[string]any{"a": int64(2)},
		}},
		{"带转义的键", `{"a\"b":1}`, []any{map[string]any{`a"b`: int64(1)}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decode(t, tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("得到 %#v,期望 %#v", got, tt.want)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"重复键", `{"a":1,"a":2}`},
		{"对象未闭合", `{"a":`},
		{"数组未闭合", `[1,2`},
		{"尾部多余内容", `{"a":1} x`},
		{"非法字面量", `tru`},
		{"数字超出 float64", `1e999`},
		{"非法 UTF-8", "\"\xff\""},
		{"键后缺少值", `{"a"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if docs, err := (Format{}).Decode(strings.NewReader(tt.in), nil); err == nil {
				t.Fatalf("期望报错,得到 %#v", docs)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	doc := map[string]any{"b": []any{int64(1), "x"}, "a": map[string]any{"z": nil}}
	tests := []struct {
		name string
		opts *format.Options
		want string
	}{
		{"默认缩进 2 且键有序", nil, "{\n  \"a\": {\n    \"z\": null\n  },\n  \"b\": [\n    1,\n    \"x\"\n  ]\n}\n"},
		{"缩进 4", &format.Options{Indent: 4}, "{\n    \"a\": {\n        \"z\": null\n    },\n    \"b\": [\n        1,\n        \"x\"\n    ]\n}\n"},
		{"紧凑", &format.Options{Compact: true}, "{\"a\":{\"z\":null},\"b\":[1,\"x\"]}\n"},
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

func TestEncodeScalarsAndEscaping(t *testing.T) {
	tests := []struct {
		name string
		doc  any
		want string
	}{
		{"nil", nil, "null\n"},
		{"字符串不转义 HTML", "<a&b>", "\"<a&b>\"\n"},
		{"中文原样输出", "你好", "\"你好\"\n"},
		{"uint64", uint64(math.MaxUint64), "18446744073709551615\n"},
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

func TestEncodeErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := (Format{}).Encode(&buf, math.NaN(), nil); err == nil {
		t.Error("NaN 无法编码为 JSON,期望报错")
	}
	if err := (Format{}).Encode(&buf, 1, &format.Options{Indent: 1}); err == nil {
		t.Error("非法缩进期望报错")
	}
}

func TestJoin(t *testing.T) {
	docs := [][]byte{[]byte("{\n  \"a\": 1\n}\n"), []byte("[1, 2]\n"), []byte("null\n")}
	got, err := (Format{}).Join(docs)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\"a\":1}\n[1,2]\nnull\n"; string(got) != want {
		t.Fatalf("得到 %q,期望 %q", got, want)
	}
	if _, err := (Format{}).Join([][]byte{[]byte("{bad")}); err == nil {
		t.Error("非法 JSON 合并应报错")
	}
}

func TestRegistered(t *testing.T) {
	f, ok := format.Lookup("json")
	if !ok || f.Name() != "json" {
		t.Fatalf("json 未注册:%v %v", f, ok)
	}
	if g, ok := format.ByExt(".JSON"); !ok || g.Name() != "json" {
		t.Fatalf("后缀 .json 未注册:%v %v", g, ok)
	}
}

func TestRoundTrip(t *testing.T) {
	in := `{"s":"x","n":[1,2.5,-3],"o":{"t":true,"f":false,"z":null},"big":18446744073709551615}`
	docs := decode(t, in)
	var buf bytes.Buffer
	if err := (Format{}).Encode(&buf, docs[0], &format.Options{Compact: true}); err != nil {
		t.Fatal(err)
	}
	want := `{"big":18446744073709551615,"n":[1,2.5,-3],"o":{"f":false,"t":true,"z":null},"s":"x"}` + "\n"
	if buf.String() != want {
		t.Fatalf("得到 %q,期望 %q", buf.String(), want)
	}
}
