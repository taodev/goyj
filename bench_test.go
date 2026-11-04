package goyj_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/taodev/goyj"
)

// genYAML 生成包含 n 个服务条目的 YAML 文档:嵌套映射、序列、各类标量和一个被引用的锚点。
func genYAML(n int) []byte {
	var b strings.Builder
	b.WriteString("defaults: &defaults\n  retries: 3\n  timeout: 2.5\n  tls: true\n")
	b.WriteString("services:\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  - name: service-%d\n", i)
		fmt.Fprintf(&b, "    port: %d\n", 8000+i)
		b.WriteString("    enabled: true\n")
		b.WriteString("    ratio: 0.75\n")
		b.WriteString("    note: null\n")
		b.WriteString("    options:\n      <<: *defaults\n      region: us-east-1\n")
		b.WriteString("    tags: [web, internal, \"v2\"]\n")
		b.WriteString("    description: |\n      第一行\n      第二行\n")
	}
	return []byte(b.String())
}

// genYAMLNoAlias 与 genYAML 相同,但不使用锚点、别名和合并键,可用于严格模式。
func genYAMLNoAlias(n int) []byte {
	var b strings.Builder
	b.WriteString("services:\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  - name: service-%d\n", i)
		fmt.Fprintf(&b, "    port: %d\n", 8000+i)
		b.WriteString("    enabled: true\n    ratio: 0.75\n    note: null\n")
		b.WriteString("    options: {retries: 3, timeout: 2.5, tls: true, region: us-east-1}\n")
		b.WriteString("    tags: [web, internal, \"v2\"]\n")
	}
	return []byte(b.String())
}

// genJSON 把 genYAMLNoAlias 的内容转换为 JSON,作为 JSON→YAML 基准的输入。
func genJSON(b *testing.B, n int) []byte {
	b.Helper()
	out, err := goyj.YAMLToJSON(genYAMLNoAlias(n), goyj.WithCompact(true))
	if err != nil {
		b.Fatal(err)
	}
	return out
}

var sizes = []struct {
	name string
	n    int
}{
	{"小(10)", 10},
	{"中(1000)", 1000},
	{"大(20000)", 20000},
}

func BenchmarkYAMLToJSON(b *testing.B) {
	for _, s := range sizes {
		data := genYAML(s.n)
		b.Run(s.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := goyj.YAMLToJSON(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkYAMLToJSONCompact(b *testing.B) {
	data := genYAML(1000)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := goyj.YAMLToJSON(data, goyj.WithCompact(true)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkYAMLToJSONStrict 比较严格模式与宽松模式的开销(输入不含别名,严格模式可通过)。
func BenchmarkYAMLToJSONStrict(b *testing.B) {
	data := genYAMLNoAlias(1000)
	for _, strict := range []bool{false, true} {
		b.Run(fmt.Sprintf("strict=%v", strict), func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := goyj.YAMLToJSON(data, goyj.WithStrict(strict)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkJSONToYAML(b *testing.B) {
	for _, s := range sizes {
		data := genJSON(b, s.n)
		b.Run(s.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := goyj.JSONToYAML(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkYAMLToJSONMultiDoc 测量多文档输入(每个文档较小)的转换开销,包含 JSON Lines 合并。
func BenchmarkYAMLToJSONMultiDoc(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&sb, "---\nid: %d\nname: doc-%d\ntags: [a, b]\n", i, i)
	}
	data := []byte(sb.String())
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := goyj.YAMLToJSON(data); err != nil {
			b.Fatal(err)
		}
	}
}
