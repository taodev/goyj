package batch

import (
	"fmt"
	"path/filepath"
	"strings"
)

// OutputPath 计算单个输出文件的路径。
//
//   - src:输入文件路径;
//   - root:输入文件所属的输入目录,src 是命令行直接给出的文件时为空;
//   - outDir:-o 指定的输出目录,为空表示输出到输入文件所在目录;
//   - ext:目标格式的后缀(含前导点);
//   - index、total:文档序号(从 1 开始)与文档总数,total 大于 1 时文件名带序号。
//
// 指定 outDir 且 src 来自输入目录 root 时,保持相对于 root 的目录结构;
// 命令行直接给出的文件直接放在 outDir 下。
func OutputPath(src, root, outDir, ext string, index, total int) string {
	name := stem(src)
	if total > 1 {
		name = fmt.Sprintf("%s-%d", name, index)
	}
	name += ext

	if outDir == "" {
		return filepath.Join(filepath.Dir(src), name)
	}
	dir := outDir
	if root != "" {
		if rel, err := filepath.Rel(root, filepath.Dir(src)); err == nil && !strings.HasPrefix(rel, "..") {
			dir = filepath.Join(outDir, rel)
		}
	}
	return filepath.Join(dir, name)
}

// stem 返回去掉目录和最后一级后缀的文件名,例如 "a/b.c.yaml" → "b.c"。
func stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
