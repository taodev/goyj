package format

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

// fakeFormat 是仅用于测试注册表的最小实现。
type fakeFormat struct {
	name string
	exts []string
}

func (f fakeFormat) Name() string         { return f.name }
func (f fakeFormat) Extensions() []string { return f.exts }
func (fakeFormat) Decode(io.Reader, *Options) ([]any, error) {
	return nil, nil
}
func (fakeFormat) Encode(io.Writer, any, *Options) error { return nil }

// joinFormat 额外实现了 Joiner。
type joinFormat struct{ fakeFormat }

func (joinFormat) Join(docs [][]byte) ([]byte, error) {
	return bytes.Join(docs, []byte("|")), nil
}

func TestResolveAndValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      *Options
		want    Options
		wantErr bool
	}{
		{"nil 使用默认值", nil, Options{Indent: DefaultIndent}, false},
		{"零值使用默认缩进", &Options{}, Options{Indent: DefaultIndent}, false},
		{"保留显式设置", &Options{Strict: true, Indent: 4, Compact: true}, Options{Strict: true, Indent: 4, Compact: true}, false},
		{"最小缩进", &Options{Indent: MinIndent}, Options{Indent: MinIndent}, false},
		{"最大缩进", &Options{Indent: MaxIndent}, Options{Indent: MaxIndent}, false},
		{"缩进过小", &Options{Indent: 1}, Options{Indent: 1}, true},
		{"缩进过大", &Options{Indent: 10}, Options{Indent: 10}, true},
		{"负数缩进", &Options{Indent: -2}, Options{Indent: -2}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.in)
			if got != tt.want {
				t.Fatalf("Resolve = %+v,期望 %+v", got, tt.want)
			}
			if err := got.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate 错误 = %v,期望出错 = %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveDoesNotMutateInput(t *testing.T) {
	in := &Options{}
	Resolve(in)
	if in.Indent != 0 {
		t.Fatalf("Resolve 修改了入参: %+v", in)
	}
}

func TestRegisterAndLookup(t *testing.T) {
	Register(fakeFormat{name: "FakeA", exts: []string{".FA", "fb"}})

	for _, name := range []string{"fakea", "FAKEA", "FakeA"} {
		if f, ok := Lookup(name); !ok || f.Name() != "FakeA" {
			t.Errorf("Lookup(%q) = %v, %v", name, f, ok)
		}
	}
	for _, ext := range []string{".fa", ".FA", "fa", ".fb", "FB"} {
		if f, ok := ByExt(ext); !ok || f.Name() != "FakeA" {
			t.Errorf("ByExt(%q) = %v, %v", ext, f, ok)
		}
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup 不应找到未注册的格式")
	}
	if _, ok := ByExt(".nope"); ok {
		t.Error("ByExt 不应找到未注册的后缀")
	}
	found := false
	for _, n := range Names() {
		if n == "fakea" {
			found = true
		}
	}
	if !found {
		t.Errorf("Names() = %v,缺少 fakea", Names())
	}
}

func TestRegisterPanics(t *testing.T) {
	Register(fakeFormat{name: "panic1", exts: []string{".p1"}})
	tests := []struct {
		name string
		f    Format
		want string
	}{
		{"nil", nil, "nil"},
		{"空名称", fakeFormat{name: ""}, "名称"},
		{"重复名称", fakeFormat{name: "PANIC1"}, "重复"},
		{"重复后缀", fakeFormat{name: "panic2", exts: []string{"P1"}}, "后缀"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("期望 panic")
				}
				if s, _ := r.(string); !strings.Contains(s, tt.want) {
					t.Fatalf("panic 信息 %q 不包含 %q", s, tt.want)
				}
			}()
			Register(tt.f)
		})
	}
	// 后缀冲突导致的失败注册不应留下半成品。
	if _, ok := Lookup("panic2"); ok {
		t.Error("失败的注册不应写入注册表")
	}
}

func TestJoinDocs(t *testing.T) {
	docs := [][]byte{[]byte("a\n"), []byte("b\n"), []byte("c\n")}
	tests := []struct {
		name string
		f    Format
		in   [][]byte
		want string
	}{
		{"空", fakeFormat{name: "x"}, nil, ""},
		{"单个原样返回", joinFormat{}, docs[:1], "a\n"},
		{"默认直接拼接", fakeFormat{name: "x"}, docs, "a\nb\nc\n"},
		{"使用 Joiner", joinFormat{}, docs, "a\n|b\n|c\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JoinDocs(tt.f, tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("得到 %q,期望 %q", got, tt.want)
			}
		})
	}
}

func TestStrictError(t *testing.T) {
	tests := []struct {
		err  *StrictError
		want string
	}{
		{&StrictError{Line: 3, Column: 5, Path: "a.b", Reason: "原因"}, "严格模式:第 3 行第 5 列(路径 a.b):原因"},
		{&StrictError{Line: 1, Column: 1, Reason: "原因"}, "严格模式:第 1 行第 1 列:原因"},
		{&StrictError{Reason: "原因"}, "严格模式:原因"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q,期望 %q", got, tt.want)
		}
	}
	var wrapped error = &StrictError{Reason: "x"}
	var target *StrictError
	if !errors.As(wrapped, &target) || !reflect.DeepEqual(target, wrapped) {
		t.Error("errors.As 无法提取 *StrictError")
	}
}
