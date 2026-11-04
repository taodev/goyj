package goyj_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"encoding/json/jsontext"

	"github.com/taodev/goyj"
)

// validJSONStream 检查 b 是由若干合法 JSON 值组成的流(JSON Lines)。
func validJSONStream(b []byte) error {
	dec := jsontext.NewDecoder(bytes.NewReader(b))
	for {
		if _, err := dec.ReadValue(); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// FuzzYAMLToJSON 检查 YAMLToJSON 对任意输入:
//   - 不会 panic;
//   - 成功时输出一定是合法的 JSON 流;
//   - 严格模式成功时,宽松模式必须成功且结果相同(严格模式只是更挑剔,不会改变结果);
//   - 输出再转回 YAML 后可以被解析。
func FuzzYAMLToJSON(f *testing.F) {
	seeds := []string{
		"",
		"# 注释\n",
		"a: 1\n",
		"a: [1, 2.5, true, null, x]\nb: {c: d}\n",
		"a: &x [1, 2]\nb: *x\n",
		"base: &b {x: 1}\nd:\n  <<: *b\n  y: 2\n",
		"1: a\ntrue: b\nnull: c\n1.5: d\n",
		"a: 1\n---\nb: 2\n---\n",
		"---\n---\n",
		"t: 2001-02-03T04:05:06Z\nd: 2001-02-03\n",
		"x: .inf\ny: -.inf\nz: .nan\n",
		"? [1, 2]\n: v\n",
		"a: !!binary aGk=\nb: !foo bar\n",
		"s: |\n  多行\n  文本\n",
		"a: [\n",
		"a: &x [*x]\n",
		"\"\\u0000\": \"\\ud800\"\n",
		"- - - - - - - - 1\n",
		"a: 0x1F\nb: 0o17\nc: 1e3\nd: 123456789012345678901234567890\n",
		"{a: 1, a: 2}\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// 把卡死转换成可复现的失败:单次执行超过 5 秒视为 bug。
		timer := time.AfterFunc(5*time.Second, func() { panic(fmt.Sprintf("YAMLToJSON 执行超时,输入 %q", data)) })
		defer timer.Stop()

		loose, err := goyj.YAMLToJSON(data)
		if err != nil {
			// 失败时严格模式也必须失败(严格模式不可能比宽松模式更宽容)。
			if _, serr := goyj.YAMLToJSON(data, goyj.WithStrict(true)); serr == nil {
				t.Fatalf("宽松模式失败(%v)但严格模式成功,输入 %q", err, data)
			}
			return
		}
		if err := validJSONStream(loose); err != nil {
			t.Fatalf("输出不是合法 JSON(%v):%q\n输入 %q", err, loose, data)
		}

		strict, serr := goyj.YAMLToJSON(data, goyj.WithStrict(true))
		if serr == nil {
			if !bytes.Equal(strict, loose) {
				t.Fatalf("严格与宽松结果不一致\n严格: %q\n宽松: %q\n输入 %q", strict, loose, data)
			}
		} else {
			var se *goyj.StrictError
			if !errors.As(serr, &se) {
				// 严格模式下除 StrictError 外,只允许与宽松模式相同的普通错误;
				// 宽松模式已成功,因此这里出现其他错误就是 bug。
				t.Fatalf("严格模式返回了非 StrictError 错误:%v,输入 %q", serr, data)
			}
		}

		// 输出的 JSON 必须能再转成 YAML。
		if _, err := goyj.JSONToYAML(loose); err != nil {
			t.Fatalf("输出的 JSON 无法转回 YAML:%v\nJSON: %q\n输入 %q", err, loose, data)
		}
	})
}

// FuzzJSONToYAML 检查 JSONToYAML 对任意输入:
//   - 不会 panic;
//   - 成功时输出的 YAML 能在严格模式下解析回来,并且再转成 JSON 后与直接规范化的 JSON 一致
//     (即 JSON→YAML→JSON 不改变数据)。
func FuzzJSONToYAML(f *testing.F) {
	seeds := []string{
		``,
		`null`,
		`true`,
		`"str"`,
		`42`,
		`-0`,
		`1.5e300`,
		`18446744073709551615`,
		`{"a":1,"b":[true,null,"x"],"c":{"d":2.5}}`,
		`[]`,
		`{}`,
		`{"":""}`,
		`{"a":1}{"b":2}`,
		"{\"a\":1}\n{\"b\":2}\n",
		`{"s":"multi\nline","t":"  spaces  ","u":"#hash","v":"- dash","w":"null","x":"~","y":"yes","z":"1e3"}`,
		`{"k: v":"a: b","- x":"? y","&a":"*b","!t":"|","%":"@"}`,
		`"\u0000\u001f\u007f\u0085\u2028\u2029\ufeff"`,
		`"你好 🌏"`,
		`"\n 0"`,       // 回归:yaml v4 rc.6 的字面块缩进缺陷
		`"\n\t  a\n"`,  // 回归:以空白开头的多行字符串
		`{"\n  a":1}`,  // 回归:多行 key
		`"\u2028\n #"`, // 回归:以 Unicode 换行符开头
		`{"a":`,
		`{"a":1,"a":2}`,
		`[[[[[[[[1]]]]]]]]`,
		`1e999`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		y, err := goyj.JSONToYAML(data)
		if err != nil {
			return
		}
		want, err := goyj.Convert(data, "json", "json", goyj.WithCompact(true))
		if err != nil {
			t.Fatalf("JSONToYAML 成功但 JSON 规范化失败:%v,输入 %q", err, data)
		}
		got, err := goyj.YAMLToJSON(y, goyj.WithCompact(true), goyj.WithStrict(true))
		if err != nil {
			t.Fatalf("生成的 YAML 无法严格解析:%v\nYAML: %q\n输入 %q", err, y, data)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("往返结果不一致\nYAML: %q\n得到: %q\n期望: %q\n输入 %q", y, got, want, data)
		}
	})
}
