package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"
	"unicode"
)

const helpText = `
usage: vk_cpp_generator <spec_file> [-o <output_file>]

Convert XML specification into C++ header. Writes to STDOUT, unless
<output_file> is specified.

Options:
`

var outputFile = flag.String("o", "", "Write output to file instead of STDOUT")

func panicIfError(err error) {
	if err != nil {
		panic(err)
	}
}

func convertEnumValueName(expand, enum, name string) string {
	// strip prefix
	if expand != "" && strings.HasPrefix(name, expand) {
		name = strings.TrimPrefix(name, expand)
	} else {
		senum, _ := trimTagSuffix(enum)
		senum = toSnakeCase(strings.TrimSuffix(senum, "FlagBits"))
		if strings.HasPrefix(name, senum) {
			name = name[len(senum)+1:]
		}
	}

	if enum == "VkResult" {
		// special case
		name = strings.TrimPrefix(name, "VK_")
	}

	name, _ = trimTagSuffix(name)
	name = strings.TrimSuffix(name, "_BIT")
	name = toCamelCase(name)
	return "e" + name
}

func convertVkName(name string) string {
	return strings.TrimPrefix(name, "Vk")
}

func convertEnumName(name string) string    { return convertVkName(name) }
func convertHandleName(name string) string  { return convertVkName(name) }
func convertBitMaskName(name string) string { return convertVkName(name) }
func convertStructName(name string) string  { return convertVkName(name) }
func convertCommandName(name string) string {
	name = strings.TrimPrefix(name, "vk")
	return strings.ToLower(name[0:1]) + name[1:]
}

// PIPELINE_DEPTH_STENCIL_STATE_CREATE_INFO -> PipelineDepthStencilStateCreateInfo
func toCamelCase(s string) string {
	if len(s) <= 1 {
		return s
	}
	var b bytes.Buffer
	var prev rune
	for i, r := range s {
		if r != '_' {
			if i == 0 || prev == '_' || unicode.IsDigit(prev) {
				b.WriteRune(r)
			} else {
				b.WriteRune(unicode.ToLower(r))
			}
		}
		prev = r
	}
	return b.String()
}

// PipelineDepthStencilStateCreateInfo -> PIPELINE_DEPTH_STENCIL_STATE_CREATE_INFO
func toSnakeCase(s string) string {
	if len(s) <= 1 {
		return s
	}
	var b bytes.Buffer
	var prev rune
	for i, r := range s {
		if i != 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			b.WriteRune('_')
		}
		b.WriteRune(unicode.ToUpper(r))
		prev = r
	}
	return b.String()
}

var knownTags = []string{
	"KHR",
	"EXT",
}

func trimTagSuffix(s string) (string, string) {
	for _, tag := range knownTags {
		stag := "_" + tag
		if strings.HasSuffix(s, stag) {
			return strings.TrimSuffix(s, stag), tag
		}
		if strings.HasSuffix(s, tag) {
			return strings.TrimSuffix(s, tag), tag
		}
	}
	return s, ""
}

func bitMaskNameToEnumName(s string) string {
	s, tagUsed := trimTagSuffix(s)
	if strings.HasSuffix(s, "Flags") {
		s = strings.TrimSuffix(s, "Flags") + "FlagBits"
	}
	return s + tagUsed
}

func structToTypeName(s string) string {
	return "VK_STRUCTURE_TYPE_" + toSnakeCase(s)
}

func nameExtraArrayFix(name, extra *string) {
	// hack to fix the vk.xml, some names end with [2], let's move it to extra
	if strings.HasSuffix(*name, "[2]") {
		*name = strings.TrimSuffix(*name, "[2]")
		*extra += "[2]"
	}
}

type xmlRegistry struct {
	XMLName string `xml:"registry"`
	Types   struct {
		Type []xmlType `xml:"type"`
	} `xml:"types"`
	Enums    []xmlEnums `xml:"enums"`
	Commands struct {
		Command []xmlCommand `xml:"command"`
	} `xml:"commands"`
	Extensions struct {
		Extension []xmlExtension `xml:"extension"`
	} `xml:"extensions"`
}

type xmlExtension struct {
	Protect string `xml:"protect,attr"`
	Require struct {
		Types []struct {
			Name string `xml:"name,attr"`
		} `xml:"type"`
		Commands []struct {
			Name string `xml:"name,attr"`
		} `xml:"command"`
	} `xml:"require"`
}

type xmlCommand struct {
	Proto  xmlTypeName   `xml:"proto"`
	Params []xmlTypeName `xml:"param"`
}

type xmlType struct {
	Name         string        `xml:"name,attr"`
	Requires     string        `xml:"requires,attr"`
	Category     string        `xml:"category,attr"`
	ReturnedOnly bool          `xml:"returnedonly,attr"`
	Members      []xmlTypeName `xml:"member"`
	InnerName    string        `xml:"name"`
	InnerType    string        `xml:"type"`
}

type xmlTypeName struct {
	Type  string `xml:"type"`
	Name  string `xml:"name"`
	Extra string `xml:",chardata"`
}

type xmlEnums struct {
	Name   string    `xml:"name,attr"`
	Type   string    `xml:"type,attr"`
	Expand string    `xml:"expand,attr"`
	Values []xmlEnum `xml:"enum"`
}

type xmlEnum struct {
	Name   string `xml:"name,attr"`
	Value  int    `xml:"value,attr"`
	BitPos int    `xml:"bitpos,attr"`
}

type HeaderParams struct {
	GuardBegin string
	GuardEnd   string
	Namespace  string
}

type Handle struct {
	Name     string
	VkName   string
	TypeSafe bool
}

type EnumValue struct {
	Name   string
	VkName string
}

type Protect struct {
	Begin string
	End   string
}

type Enum struct {
	Protect Protect
	Name    string
	Values  []EnumValue
	used    bool
}

type BitMask struct {
	Protect Protect
	Name    string
	VkName  string
	Enum    *Enum
}

type Command struct {
	Protect    Protect
	Name       string
	VkName     string
	RetType    string
	RetVkType  string
	Parameters []CommandParameter
}

type CommandParameter struct {
	Name         string
	Type         string
	VkType       string
	AnalyzedType AnalyzedType
	Converter    TypeConverter
}

type Struct struct {
	Protect  Protect
	Name     string
	VkName   string
	TypeName string
	HasSType bool
	Members  []StructMember
	ReadOnly bool
}

type StructMember struct {
	Name         string
	Type         string
	VkType       string
	AnalyzedType AnalyzedType
	Converter    TypeConverter
}

type Context struct {
	Handles    []Handle
	BitMasks   []BitMask
	Enums      []Enum
	Structs    []Struct
	Commands   []Command
	converters map[string]TypeConverter
}

type StructsSort []Struct

func (s StructsSort) Len() int           { return len(s) }
func (s StructsSort) Less(i, j int) bool { return s[i].VkName < s[j].VkName }
func (s StructsSort) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (ctx *Context) SortStructsByDeps() {
	// we need to sort struct by deps
	set := map[string]*Struct{}
	for i, s := range ctx.Structs {
		set[s.VkName] = &ctx.Structs[i]
	}

	// now we just go over structs many times, if a struct has no deps in set,
	// we remove it from set and add it to array, then repeat, note that this
	// process will not break cycles, but I've added protection against cycles
	lastOutLen := 0
	out := make([]Struct, 0, len(ctx.Structs))
	for len(set) > 0 {
		for _, s := range set {
			hasDeps := false
			for _, m := range s.Members {
				if _, ok := set[m.AnalyzedType.Type]; ok {
					hasDeps = true
					break
				}
			}
			if !hasDeps {
				out = append(out, *s)
			}
		}
		if len(out) == lastOutLen {
			panic("circular dependencies detected")
		}
		for i := lastOutLen; i < len(out); i++ {
			delete(set, out[i].VkName)
		}
		sort.Sort(StructsSort(out[lastOutLen:]))
		lastOutLen = len(out)
	}
	ctx.Structs = out
}

func (ctx *Context) ResolveStructMemberConverters() {
	for _, s := range ctx.Structs {
		for i := range s.Members {
			m := &s.Members[i]
			if m.AnalyzedType.IsArray {
				// hacky way to handle array types
				m.Converter = &ArrayConverter{
					VkName:  m.AnalyzedType.Type,
					CppName: convertVkName(m.AnalyzedType.Type),
				}
				continue
			}

			// try to get a type-specific converter
			conv, ok := ctx.converters[m.AnalyzedType.Type]
			if ok {
				m.Converter = conv
			}
		}
	}
}

func (ctx *Context) ResolveCommandParameterConverters() {
	for _, c := range ctx.Commands {
		for i := range c.Parameters {
			p := &c.Parameters[i]
			conv, ok := ctx.converters[p.AnalyzedType.Type]
			if ok {
				p.Converter = conv
			}
		}
	}
}

func assembleType(typ, extra string) string {
	extra = strings.TrimSpace(extra)
	out := typ
	if strings.HasPrefix(extra, "const ") {
		out = "const " + out
		extra = strings.TrimSpace(strings.TrimPrefix(extra, "const "))
	}
	if len(extra) > 0 && extra[0] == '[' && extra[len(extra)-1] == ']' {
		extra = "*"
	}
	return out + extra
}

func newContext(registry *xmlRegistry) Context {
	var ctx Context
	ctx.converters = map[string]TypeConverter{}
	enumMap := map[string]*Enum{}      // vk enum name -> Enum
	protectMap := map[string]Protect{} // vk type name -> protect string
	for _, e := range registry.Extensions.Extension {
		if e.Protect == "" {
			continue
		}
		for _, t := range e.Require.Types {
			protectMap[t.Name] = Protect{
				Begin: "#ifdef " + e.Protect,
				End:   "#endif",
			}
		}
		for _, c := range e.Require.Commands {
			protectMap[c.Name] = Protect{
				Begin: "#ifdef " + e.Protect,
				End:   "#endif",
			}
		}
	}
	for _, xe := range registry.Enums {
		e := &Enum{
			Protect: protectMap[xe.Name],
			Name:    convertEnumName(xe.Name),
		}
		for _, v := range xe.Values {
			e.Values = append(e.Values, EnumValue{
				Name:   convertEnumValueName(xe.Expand, xe.Name, v.Name),
				VkName: v.Name,
			})
		}
		enumMap[xe.Name] = e
		ctx.converters[xe.Name] = &StaticCastConverter{
			CppName: e.Name,
			VkName:  xe.Name,
		}
	}
	// Separate pass on bitmasks, so that we know which enums are used.
	// Technically bitmasks are placed before enums in vk.xml, but who
	// guaranees that.
	for _, t := range registry.Types.Type {
		switch t.Category {
		case "bitmask":
			if t.InnerType != "VkFlags" {
				log.Printf("unrecognized bitmask type: %s", t.InnerType)
				continue
			}

			enumName := bitMaskNameToEnumName(t.InnerName)
			enum, ok := enumMap[enumName]
			if !ok {
				// broken xml, some enums are missing, let's just create them
				enum = &Enum{Name: convertEnumName(enumName)}
				enumMap[enumName] = enum
			}
			// we also clear protect, because in all cases bit mask is already
			// wrapped
			enum.Protect = Protect{}
			enum.used = true

			bm := BitMask{
				Protect: protectMap[t.InnerName],
				Name:    convertBitMaskName(t.InnerName),
				VkName:  t.InnerName,
				Enum:    enum,
			}
			ctx.BitMasks = append(ctx.BitMasks, bm)
			ctx.converters[t.InnerName] = &BitMaskConverter{
				CppName: bm.Name,
				VkName:  bm.VkName,
			}
		}
	}
	for _, t := range registry.Types.Type {
		switch t.Category {
		case "handle":
			h := Handle{
				Name:     convertHandleName(t.InnerName),
				VkName:   t.InnerName,
				TypeSafe: t.InnerType == "VK_DEFINE_HANDLE",
			}
			ctx.Handles = append(ctx.Handles, h)
			ctx.converters[t.InnerName] = &HandleConverter{
				CppName: h.Name,
				VkName:  h.VkName,
			}
		case "enum":
			enum, ok := enumMap[t.Name]
			if !ok {
				enum = &Enum{Name: convertEnumName(t.Name)}
			}
			if enum.used {
				continue
			}
			ctx.Enums = append(ctx.Enums, *enum)
		case "struct", "union":
			if t.Name == "VkRect3D" { // TODO: vulkan/vulkan.h contains no such thing
				continue
			}
			name := convertStructName(t.Name)
			s := Struct{
				Protect:  protectMap[t.Name],
				Name:     name,
				VkName:   t.Name,
				TypeName: structToTypeName(name),
				ReadOnly: t.ReturnedOnly,
			}
			for _, m := range t.Members {
				if m.Name == "sType" {
					s.HasSType = true
				}
				nameExtraArrayFix(&m.Name, &m.Extra)
				s.Members = append(s.Members, StructMember{
					Name:         m.Name,
					Type:         assembleType(convertVkName(m.Type), m.Extra),
					VkType:       assembleType(m.Type, m.Extra),
					AnalyzedType: NewAnalyzedType(m.Name, m.Type, m.Extra),
					Converter:    NopConverter{},
				})
			}
			ctx.Structs = append(ctx.Structs, s)
			ctx.converters[t.Name] = &ReinterpretCastConverter{
				CppName: s.Name,
				VkName:  s.VkName,
			}
		}
	}
	for _, c := range registry.Commands.Command {
		cmd := Command{
			Protect:   protectMap[c.Proto.Name],
			Name:      convertCommandName(c.Proto.Name),
			VkName:    c.Proto.Name,
			RetType:   assembleType(convertVkName(c.Proto.Type), c.Proto.Extra),
			RetVkType: assembleType(c.Proto.Type, c.Proto.Extra),
		}
		for _, p := range c.Params {
			cp := CommandParameter{
				Name:         p.Name,
				Type:         assembleType(convertVkName(p.Type), p.Extra),
				VkType:       assembleType(p.Type, p.Extra),
				AnalyzedType: NewAnalyzedType(p.Name, p.Type, p.Extra),
				Converter:    NopConverter{},
			}
			cmd.Parameters = append(cmd.Parameters, cp)
		}
		ctx.Commands = append(ctx.Commands, cmd)
	}
	ctx.SortStructsByDeps()
	ctx.ResolveStructMemberConverters()
	ctx.ResolveCommandParameterConverters()
	return ctx
}

func main() {
	flag.Usage = func() {
		fmt.Printf(helpText[1:])
		flag.PrintDefaults()
	}
	flag.Parse()
	nargs := flag.NArg()
	if nargs != 1 {
		flag.Usage()
		os.Exit(1)
	}

	var output io.Writer
	if *outputFile != "" {
		f, err := os.Create(*outputFile)
		panicIfError(err)
		defer f.Close()
		output = f
	} else {
		output = os.Stdout
	}

	specfile := flag.Arg(0)
	specxml, err := ioutil.ReadFile(specfile)
	panicIfError(err)

	var registry xmlRegistry
	panicIfError(xml.Unmarshal(specxml, &registry))
	headerParams := HeaderParams{
		GuardBegin: "#pragma once",
		GuardEnd:   "",
		Namespace:  "vk",
	}
	ctx := newContext(&registry)
	panicIfError(tpl.ExecuteTemplate(output, "header", &headerParams))
	panicIfError(tpl.ExecuteTemplate(output, "body", &ctx))
	panicIfError(tpl.ExecuteTemplate(output, "footer", &headerParams))
}
