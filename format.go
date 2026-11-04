package goyj

import (
	"github.com/taodev/goyj/format"
	// 导入内置格式实现,触发它们在 init 中的自注册。
	_ "github.com/taodev/goyj/format/json"
	_ "github.com/taodev/goyj/format/yaml"
)

// Format 描述一种可互转的数据格式,详见 format.Format。
type Format = format.Format

// StrictError 是严格模式下发现无法无损转换的特性时返回的错误,携带行列号、数据路径和原因。
// 可以用 errors.As 提取。
type StrictError = format.StrictError

// Register 把自定义格式加入全局注册表,之后可通过 Convert 的 from / to 参数按名称使用。
// 名称或后缀与已注册格式冲突时会 panic。内置的 yaml 和 json 已自动注册。
func Register(f Format) { format.Register(f) }

// LookupFormat 按名称(不区分大小写)查找已注册的格式。
func LookupFormat(name string) (Format, bool) { return format.Lookup(name) }

// FormatByExt 按文件后缀查找已注册的格式,例如 ".yml" 或 "json"。
func FormatByExt(ext string) (Format, bool) { return format.ByExt(ext) }

// FormatNames 返回所有已注册格式的名称,按字典序排列。
func FormatNames() []string { return format.Names() }
