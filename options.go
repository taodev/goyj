package goyj

import "github.com/taodev/goyj/format"

// Options 是转换选项,零值表示默认选项:宽松模式、缩进 2、非紧凑输出。
// 它是 format.Options 的别名,自定义格式的 Decode / Encode 直接接收该类型。
type Options = format.Options

// Option 是函数式选项,用于调整单次转换的行为。
type Option func(*Options)

// WithStrict 设置是否启用严格模式。
//
// 严格模式下,遇到无法无损转换的特性(非字符串 key、锚点与别名、合并键、
// 重复 key、时间戳、无穷大与 NaN、自定义标签等)会返回带位置的 *StrictError,
// 而不是自动降级处理。
func WithStrict(strict bool) Option {
	return func(o *Options) { o.Strict = strict }
}

// WithIndent 设置输出缩进宽度,合法范围为 2 到 9;0 表示使用默认值 2。
// 超出范围的值会使转换函数返回错误。
func WithIndent(n int) Option {
	return func(o *Options) { o.Indent = n }
}

// WithCompact 设置 JSON 输出是否为紧凑单行。对 YAML 输出无效。
func WithCompact(compact bool) Option {
	return func(o *Options) { o.Compact = compact }
}

// resolveOptions 应用全部选项,填充默认值并校验。
func resolveOptions(opts []Option) (Options, error) {
	var o Options
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	o = format.Resolve(&o)
	return o, o.Validate()
}
