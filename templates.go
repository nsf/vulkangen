package main

import (
	"strings"
	"text/template"
)

func line(s string) string {
	if s != "" {
		return s + "\n"
	}
	return ""
}

var tpl = template.Must(template.New("").Funcs(template.FuncMap{
	"hasPrefix": strings.HasPrefix,
	"hasSuffix": strings.HasSuffix,
	"line":      line,
}).Parse(`








{{ define "header" }}

{{- .GuardBegin }}

#include <cstdint>
#include <cstddef>
#include <cstring>
#include <vulkan/vulkan.h>

namespace {{ .Namespace }} {

template <typename EnumType, typename T = uint32_t>
class Flags {
	T m_mask;

public:
	Flags(): m_mask(0) {}
	Flags(EnumType bit): m_mask(static_cast<uint32_t>(bit)) {}
	explicit Flags(T mask): m_mask(mask) {}
	Flags(const Flags &rhs): m_mask(rhs.m_mask) {}

	Flags &operator=(const Flags &rhs) { m_mask = rhs.m_mask; return *this; }

	Flags &operator|=(const Flags &rhs) { m_mask |= rhs.m_mask; return *this; }
	Flags &operator&=(const Flags &rhs) { m_mask &= rhs.m_mask; return *this; }
	Flags &operator^=(const Flags &rhs) { m_mask ^= rhs.m_mask; return *this; }

	Flags operator|(const Flags &rhs) const { return Flags(m_mask | rhs.m_mask); }
	Flags operator&(const Flags &rhs) const { return Flags(m_mask & rhs.m_mask); }
	Flags operator^(const Flags &rhs) const { return Flags(m_mask ^ rhs.m_mask); }

	Flags operator~() const { return Flags(~m_mask); }

	bool operator==(const Flags &rhs) const { return m_mask == rhs.m_mask; }
	bool operator!=(const Flags &rhs) const { return m_mask != rhs.m_mask; }

	operator bool() const { return m_mask != 0; }
	explicit operator T() const { return m_mask; }
};

template <typename EnumType, typename T>
inline Flags<EnumType, T> operator|(EnumType bit, const Flags<EnumType, T> &flags)
{
	return flags | bit;
}
template <typename EnumType, typename T>
inline Flags<EnumType, T> operator&(EnumType bit, const Flags<EnumType, T> &flags)
{
	return flags & bit;
}
template <typename EnumType, typename T>
inline Flags<EnumType, T> operator^(EnumType bit, const Flags<EnumType, T> &flags)
{
	return flags ^ bit;
}

typedef uint32_t SampleMask;
typedef uint32_t Bool32;
typedef uint64_t DeviceSize;

#if defined(__LP64__) || defined(_WIN64) || defined(__x86_64__) || defined(_M_X64) || defined(__ia64) || defined (_M_IA64) || defined(__aarch64__) || defined(__powerpc64__)
#define VK_EXPLICIT_HANDLE
#else
#define VK_EXPLICIT_HANDLE explicit
#endif

struct NullHandle {};
constexpr NullHandle nullHandle = {};

{{ end }}










{{ define "footer" }}

} // namespace {{ .Namespace }}
{{ .GuardEnd -}}

{{ end }}





{{ define "handle" -}}
{{- "\n\n" -}}

class {{ .Name }} {
	{{ .VkName }} m_handle;
public:
	{{ .Name }}(): m_handle(VK_NULL_HANDLE) {}
	{{ .Name }}(NullHandle): m_handle(VK_NULL_HANDLE) {}
	{{ if not .TypeSafe }}VK_EXPLICIT_HANDLE {{ end }}{{ .Name }}({{ .VkName }} handle): m_handle(handle) {}
	{{ if not .TypeSafe }}VK_EXPLICIT_HANDLE {{ end }}operator {{ .VkName }}() const { return m_handle; }

	{{ .VkName }} handle() const { return m_handle; }
	{{ .VkName }} *c_ptr() { return &m_handle; }
	const {{ .VkName }} *c_ptr() const { return &m_handle; }
};

inline bool operator==(const {{ .Name }} &lhs, NullHandle) { return lhs.handle() == VK_NULL_HANDLE; }
inline bool operator==(NullHandle, const {{ .Name }} &rhs) { return rhs.handle() == VK_NULL_HANDLE; }
inline bool operator!=(const {{ .Name }} &lhs, NullHandle) { return lhs.handle() != VK_NULL_HANDLE; }
inline bool operator!=(NullHandle, const {{ .Name }} &rhs) { return rhs.handle() != VK_NULL_HANDLE; }

{{- end }}










{{ define "enum" }}
{{- "\n" -}}

{{ line .Protect.Begin -}}
enum class {{ .Name }} {
{{- range .Values }}
	{{ .Name }} = {{ .VkName }},
{{- end }}
};

{{ with $e := . -}}
inline const char *getEnumString({{ $e.Name }} e)
{
	switch (e) {
	{{ range .Values -}}
	case {{$e.Name}}::{{.Name}}: return "{{$e.Name}}::{{.Name}}";
	{{ end -}}
	default: return "<invalid enum>";
	}
}
{{- end }}

{{ line .Protect.End -}}

{{ end }}










{{ define "bitmask" }}
{{- "\n" -}}

{{ line .Protect.Begin -}}
{{ template "enum" .Enum }}
using {{ .Name }} = Flags<{{ .Enum.Name }}, {{ .VkName }}>;

inline {{ .Name }} operator|({{ .Enum.Name }} bit0, {{ .Enum.Name }} bit1)
{
	return {{ .Name }}(bit0) | bit1;
}
{{ line .Protect.End -}}

{{ end }}








{{ define "struct" }}
{{- "\n" -}}

{{ line .Protect.Begin -}}
{{ with $s := . -}}
class {{ .Name }} {
	{{ .VkName }} m_struct;
public:
	{{ .Name }}()
	{
		std::memset(&m_struct, 0, sizeof({{ .VkName }}));
		{{ if .HasSType -}}
		m_struct.sType = {{ .TypeName }};
		{{- end }}
	}
	{{ .Name }}(const {{ .VkName }} &r): m_struct(r) {}

	{{ range $m := .Members }}
	{{ if and (not (hasPrefix $m.Type "const ")) $m.AnalyzedType.IsPointer }}const {{ end -}}
	{{ $m.Type }} {{ $m.Name }}() const
	{
		{{ $m.Converter.VkToCpp $m.AnalyzedType (print "m_struct." $m.Name) }}
	}
	{{ if not $s.ReadOnly -}}
	{{ $s.Name }} &{{ $m.Name }}({{ $m.Type }} {{ $m.Name }})
	{
		{{ $m.Converter.CppToVk $m.AnalyzedType $m.Name (print "m_struct." $m.Name) }}
		return *this;
	}
	{{- end -}}
	{{ end }}

	{{ .VkName }} *c_ptr() { return &m_struct; }
	const {{ .VkName }} *c_ptr() const { return &m_struct; }

	operator const {{ .VkName }}&() const { return m_struct; }
};
{{- end }}
{{ .Protect.End -}}

{{ end }}










{{ define "command" }}
{{- "\n" -}}

{{ line .Protect.Begin -}}
inline {{ .RetType }} {{ .Name }}(
	{{- range $i, $p := .Parameters -}}
		{{if $i}}, {{end}}{{$p.Type}} {{$p.Name}}
	{{- end -}}
)
{
	{{if ne .RetType "void"}}return {{end -}}
	{{if eq .RetType "Result"}}Result({{end -}}
	{{ .VkName }}(
		{{- range $i, $p := .Parameters -}}
		{{if $i}}, {{end}}{{ $p.Converter.CppToVkArg $p.AnalyzedType $p.Name }}
		{{- end -}}
	)
	{{- if eq .RetType "Result"}}){{end -}}
	;
}
{{ line .Protect.End -}}

{{ end }}












{{ define "body" }}

{{ range .Handles -}}
{{ template "handle" . }}
{{- end }}

{{ range .Enums -}}
{{ template "enum" . }}
{{- end }}

{{ range .BitMasks -}}
{{ template "bitmask" . }}
{{- end }}

{{ range .Structs -}}
{{ template "struct" . }}
{{- end }}

{{ range .Commands -}}
{{ template "command" . }}
{{- end }}

{{ end }}
`))
