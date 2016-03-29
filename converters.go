package main

import (
	"fmt"
	"strconv"
	"strings"
)

type AnalyzedType struct {
	Name  string
	Type  string
	Extra string

	// result of Analyze
	IsConst   bool
	IsPointer bool
	IsBlank   bool
	IsArray   bool
	Arity     int
	Prefix    string
	Suffix    string
}

func NewAnalyzedType(name, typ, extra string) AnalyzedType {
	at := AnalyzedType{
		Name:  name,
		Type:  typ,
		Extra: strings.TrimSpace(extra),
	}
	at.Analyze()
	return at
}

func (at *AnalyzedType) Analyze() {
	if at.Extra == "" {
		at.IsBlank = true
		return
	}
	if strings.HasPrefix(at.Extra, "const ") {
		at.IsConst = true
	}
	if strings.HasSuffix(at.Extra, "]") {
		e := at.Extra
		i := strings.LastIndex(e, "[")
		if i == -1 {
			panic("invalid type")
		}
		at.Arity, _ = strconv.Atoi(e[i+1 : len(e)-1])
		at.IsArray = true
	}
	if strings.ContainsAny(at.Extra, "*]") {
		at.IsPointer = true
	}
	at.AssemblePrefixSufix()
}

func (at *AnalyzedType) AssemblePrefixSufix() {
	e := at.Extra
	if strings.HasPrefix(e, "const ") {
		e = strings.TrimPrefix(e, "const ")
		at.Prefix = "const "
	}
	if strings.Contains(e, "[") {
		// there are better ways to replace [] or [2] with *, but let's keep
		// this one for now
		beg := strings.Index(e, "[")
		end := strings.Index(e, "]")
		if beg == -1 || end == -1 {
			panic("invalid type")
		}
		e = e[:beg] + "*" + e[end+1:]
	}
	at.Suffix = e
}

type TypeConverter interface {
	CppToVkArg(at AnalyzedType, src string) string
	CppToVk(at AnalyzedType, src, dst string) string
	VkToCpp(at AnalyzedType, src string) string
}

type CommonConverter struct {
	CppName string
	VkName  string
}

type NopConverter struct{}

func (NopConverter) CppToVkArg(at AnalyzedType, src string) string {
	return src
}

func (NopConverter) CppToVk(at AnalyzedType, src, dst string) string {
	return fmt.Sprintf("%s = %s;", dst, src)
}

func (NopConverter) VkToCpp(at AnalyzedType, src string) string {
	return fmt.Sprintf("return %s;", src)
}

type BitMaskConverter CommonConverter

func (c *BitMaskConverter) CppToVkArg(at AnalyzedType, src string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).CppToVkArg(at, src)
	}
	return fmt.Sprintf("static_cast<%s>(%s)", c.VkName, src)
}

func (c *BitMaskConverter) CppToVk(at AnalyzedType, src, dst string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).CppToVk(at, src, dst)
	}
	return fmt.Sprintf("%s = static_cast<%s>(%s);", dst, c.VkName, src)
}

func (c *BitMaskConverter) VkToCpp(at AnalyzedType, src string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).VkToCpp(at, src)
	}
	return fmt.Sprintf("return %s(%s);", c.CppName, src)
}

type StaticCastConverter CommonConverter

func (c *StaticCastConverter) CppToVkArg(at AnalyzedType, src string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).CppToVkArg(at, src)
	}
	return fmt.Sprintf("static_cast<%s>(%s)", c.VkName, src)
}

func (c *StaticCastConverter) CppToVk(at AnalyzedType, src, dst string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).CppToVk(at, src, dst)
	}
	return fmt.Sprintf("%s = static_cast<%s>(%s);", dst, c.VkName, src)
}

func (c *StaticCastConverter) VkToCpp(at AnalyzedType, src string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).VkToCpp(at, src)
	}
	return fmt.Sprintf("return static_cast<%s>(%s);", c.CppName, src)
}

type ArrayConverter CommonConverter

func (c *ArrayConverter) CppToVkArg(at AnalyzedType, src string) string {
	return src
}

func (c *ArrayConverter) CppToVk(at AnalyzedType, src, dst string) string {
	return fmt.Sprintf("std::memcpy(%s, %s, %d * sizeof(%s));", dst, src, at.Arity, c.VkName)
}

func (c *ArrayConverter) VkToCpp(at AnalyzedType, src string) string {
	return fmt.Sprintf("return reinterpret_cast<const %s*>(%s);", c.CppName, src)
}

type ReinterpretCastConverter CommonConverter

func (c *ReinterpretCastConverter) CppToVkArg(at AnalyzedType, src string) string {
	if at.IsBlank {
		return (*StaticCastConverter)(c).CppToVkArg(at, src)
	}
	return fmt.Sprintf("reinterpret_cast<%s%s%s>(%s)", at.Prefix, c.VkName, at.Suffix, src)
}

func (c *ReinterpretCastConverter) CppToVk(at AnalyzedType, src, dst string) string {
	if at.IsBlank {
		return (*StaticCastConverter)(c).CppToVk(at, src, dst)
	}
	return fmt.Sprintf("%s = reinterpret_cast<%s%s%s>(%s);", dst, at.Prefix, c.VkName, at.Suffix, src)
}

func (c *ReinterpretCastConverter) VkToCpp(at AnalyzedType, src string) string {
	if at.IsBlank {
		return (*StaticCastConverter)(c).VkToCpp(at, src)
	}
	return fmt.Sprintf("return reinterpret_cast<%s%s%s>(%s);", at.Prefix, c.CppName, at.Suffix, src)
}

type HandleConverter CommonConverter

func (c *HandleConverter) CppToVkArg(at AnalyzedType, src string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).CppToVkArg(at, src)
	}
	return fmt.Sprintf("static_cast<%s>(%s)", c.VkName, src)
}

func (c *HandleConverter) CppToVk(at AnalyzedType, src, dst string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).CppToVk(at, src, dst)
	}
	return fmt.Sprintf("%s = static_cast<%s>(%s);", dst, c.VkName, src)
}

func (c *HandleConverter) VkToCpp(at AnalyzedType, src string) string {
	if at.IsPointer {
		return (*ReinterpretCastConverter)(c).VkToCpp(at, src)
	}
	return fmt.Sprintf("return %s(%s);", c.CppName, src)
}
