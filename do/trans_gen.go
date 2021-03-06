package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kjk/u"
)

// number of missing translations for a language to be considered
// incomplete (will be excluded from Translations_txt.cpp) as a
// percentage of total string count of that specific file
const INCOMPLETE_MISSING_THRESHOLD = 0.2

type Lang struct {
	desc                      []string
	code                      string // "af"
	name                      string // "Afrikaans"
	ms_lang_id                string
	isRtl                     bool
	code_safe                 string
	c_translations_array_name string
	translations              []string
	c_escaped_lines           []string
	seq                       string
}

func NewLang(desc []string) *Lang {
	panicIf(len(desc) > 4)
	res := &Lang{
		desc:       desc,
		code:       desc[0],
		name:       desc[1],
		ms_lang_id: desc[2],
	}
	if len(desc) > 3 {
		panicIf(desc[3] != "RTL")
		res.isRtl = true
	}
	// code that can be used as part of C identifier i.e.:
	// "ca-xv" => "ca_xv"
	res.code_safe = strings.Replace(res.code, "-", "_", -1)
	res.c_translations_array_name = "gTranslations_" + res.code_safe
	return res
}

func getLangObjects(langs_defs [][]string) []*Lang {
	var res []*Lang
	for _, desc := range langs_defs {
		res = append(res, NewLang(desc))
	}
	return res
}

func get_trans_for_lang(strings_dict map[string][]*Translation, keys []string, lang_arg string) []string {
	if lang_arg == "en" {
		return keys
	}
	var trans []string
	var untrans []string
	for _, k := range keys {
		var found []string
		for _, trans := range strings_dict[k] {
			if trans.Lang == lang_arg {
				found = append(found, trans.Translation)
			}
		}
		if len(found) > 0 {
			panicIf(len(found) != 1)
			// don't include a translation, if it's the same as the default
			if found[0] == k {
				found[0] = ""
			}
			trans = append(trans, found[0])
		} else {
			trans = append(trans, "")
			untrans = append(untrans, k)
		}
	}

	if len(untrans) > int(INCOMPLETE_MISSING_THRESHOLD*float64(len(keys))) {
		return nil
	}
	return trans
}

var g_incomplete_langs []*Lang

func removeLang(langs []*Lang, lang *Lang) []*Lang {
	for idx, el := range langs {
		if el == lang {
			return append(langs[:idx], langs[idx+1:]...)
		}
	}
	panic("didn't find lang in langs")
}

func build_trans_for_langs(langs []*Lang, strings_dict map[string][]*Translation, keys []string) []*Lang {
	g_incomplete_langs = nil
	for _, lang := range langs {
		lang.translations = get_trans_for_lang(strings_dict, keys, lang.code)
		if len(lang.translations) == 0 {
			g_incomplete_langs = append(g_incomplete_langs, lang)
		}
	}
	logf("g_incomplete_langs: %d\n", len(g_incomplete_langs))
	for _, il := range g_incomplete_langs {
		nBefore := len(langs)
		langs = removeLang(langs, il)
		panicIf(len(langs) != nBefore-1)
	}
	return langs
}

const uncompressed_tmpl = `
{{.Translations}}

static const char *gTranslations[LANGS_COUNT] = {
{{.Translations_refs}}
};

const char *GetTranslationsForLang(int langIdx) { return gTranslations[langIdx]; }
`

const compact_c_tmpl = `/*
 DO NOT EDIT MANUALLY !!!
 Generated by scripts\trans_gen.py
*/

#include "utils/BaseUtil.h"

namespace trans {

#define LANGS_COUNT   {{.Langs_count}}
#define STRINGS_COUNT {{.Translations_count}}

const char *gOriginalStrings[STRINGS_COUNT] = {
{{.Orignal_strings}}
};

const char **GetOriginalStrings() { return &gOriginalStrings[0]; }

{{.Translations}}

const char *gLangCodes = {{.Langcodes}} "\0";

const char *gLangNames = {{.Langnames}} "\0";

// from http://msdn.microsoft.com/en-us/library/windows/desktop/dd318693(v=vs.85).aspx
// those definition are not present in 7.0A SDK my VS 2010 uses
#ifndef LANG_CENTRAL_KURDISH
#define LANG_CENTRAL_KURDISH 0x92
#endif

#ifndef SUBLANG_CENTRAL_KURDISH_CENTRAL_KURDISH_IRAQ
#define SUBLANG_CENTRAL_KURDISH_CENTRAL_KURDISH_IRAQ 0x01
#endif

#define _LANGID(lang) MAKELANGID(lang, SUBLANG_NEUTRAL)
const LANGID gLangIds[LANGS_COUNT] = {
{{.Langids}}
};
#undef _LANGID

bool IsLangRtl(int idx)
{
  {{.Islangrtl}}
}

int gLangsCount = LANGS_COUNT;
int gStringsCount = STRINGS_COUNT;

const LANGID *GetLangIds() { return &gLangIds[0]; }

} // namespace trans
`

// escape as octal number for C, as \nnn
func c_oct(c byte) string {
	panicIf(c < 0x80)
	s := strconv.FormatInt(int64(c), 8) // base 8 for octal
	for len(s) < 3 {
		s = "0" + s
	}
	return `\` + s
}

func c_escape(txt string) string {
	if len(txt) == 0 {
		return `"NULL"`
	}
	// escape all quotes
	txt = strings.Replace(txt, `"`, `\"`, -1)
	// and all non-7-bit characters of the UTF-8 encoded string
	res := ""
	n := len(txt)
	for i := 0; i < n; i++ {
		c := txt[i]
		if c < 0x80 {
			res += string(c)
			continue
		}
		res += c_oct(c)
	}
	return `"` + res + `"`
}

func c_escape_for_compact(txt string) string {
	if len(txt) == 0 {
		return `"\0"`
	}
	// escape all quotes
	txt = strings.Replace(txt, `"`, `\"`, -1)
	// and all non-7-bit characters of the UTF-8 encoded string
	var res string
	n := len(txt)
	for i := 0; i < n; i++ {
		c := txt[i]
		if c < 0x80 {
			res += string(c)
			continue
		}
		res += c_oct(c)
	}
	return fmt.Sprintf(`"%s\0"`, res)
}

func file_name_from_dir_name(dir_name string) string {
	// strip "src/"" from dir_name
	s := strings.TrimPrefix(dir_name, "src")
	if len(s) > 0 {
		s = s[1:]
	}
	if s == "" {
		return "Trans_sumatra_txt.cpp"
	}
	return fmt.Sprintf("Trans_%s_txt.cpp", s)
}

func build_translations(langs []*Lang) {
	for _, lang := range langs[1:] {
		var c_escaped []string
		seq := ""
		for _, t := range lang.translations {
			s := fmt.Sprintf("  %s", c_escape_for_compact(t))
			c_escaped = append(c_escaped, s)
			seq += t
			seq += `\0`
		}
		lang.c_escaped_lines = c_escaped
		lang.seq = seq
	}
}

func gen_translations(langs []*Lang) string {
	var lines []string
	for _, lang := range langs[1:] {
		s := strings.Join(lang.c_escaped_lines, "\\\n")
		s = fmt.Sprintf("const char * %s = \n%s;\n", lang.c_translations_array_name, s)
		lines = append(lines, s)
	}
	return strings.Join(lines, "\n")
}

func print_incomplete_langs(dir_name string) {
	var a []string
	for _, lang := range g_incomplete_langs {
		a = append(a, lang.code)
	}
	langs := strings.Join(a, ", ")
	count := fmt.Sprintf("%d out of %d", len(g_incomplete_langs), len(g_langs))
	logf("\nIncomplete langs in %s: %s %s", file_name_from_dir_name(dir_name), count, langs)
}

func gen_c_code_for_dir(strings_dict map[string][]*Translation, keys []string, dir_name string) {
	logf("gen_c_code_for_dir: '%s', %d strings, len(strings_dict): %d\n", dir_name, len(keys), len(strings_dict))

	sort.Slice(g_langs, func(i, j int) bool {
		x := g_langs[i]
		y := g_langs[j]
		if x[0] == "en" {
			return true
		}
		if y[0] == "en" {
			return false
		}
		return x[1] < y[1]
	})
	langs := getLangObjects(g_langs)
	panicIf("en" != langs[0].code)

	langs = build_trans_for_langs(langs, strings_dict, keys)
	logf("langs: %d, g_langs: %d\n", len(langs), len(g_langs))

	var a []string
	for _, lang := range langs {
		s := fmt.Sprintf("  %s", c_escape_for_compact((lang.code)))
		a = append(a, s)
	}
	langcodes := strings.Join(a, " \\\n")
	logf("langcodes: %d bytes\n", len(langcodes))

	a = nil
	for _, lang := range langs {
		s := fmt.Sprintf("  %s", c_escape_for_compact(lang.name))
		a = append(a, s)
	}
	langnames := strings.Join(a, " \\\n")
	logf("langnames: %d bytes\n", len(langnames))

	a = nil
	for _, lang := range langs {
		s := fmt.Sprintf("  %s", lang.ms_lang_id)
		a = append(a, s)
	}
	langids := strings.Join(a, ",\n")
	logf("langids: %d bytes\n", len(langids))

	var rtl_info []string
	n := 0
	for idx, lang := range langs {
		if !lang.isRtl {
			continue
		}
		s := fmt.Sprintf("(%d == idx)", idx)
		rtl_info = append(rtl_info, s)
		n++
	}
	panicIf(len(rtl_info) != 4)

	islangrtl := strings.Join(rtl_info, " || ")
	if len(rtl_info) == 0 {
		islangrtl = "false"
	}
	islangrtl = "return " + islangrtl + ";"
	//logf("islangrtl:\n%s\n", islangrtl)
	build_translations(langs)

	a = nil
	for _, lang := range langs[1:] {
		s := fmt.Sprintf("  %s", lang.c_translations_array_name)
		a = append(a, s)
	}

	translations_refs := "  NULL,\n" + strings.Join(a, ", \n")
	logf("translations_refs: %d bytes\n", len(translations_refs))

	translations := gen_translations(langs)
	logf("translations: %d bytes\n", len(translations))

	v := struct {
		Translations_refs string
		Translations      string
	}{
		Translations_refs: translations_refs,
		Translations:      translations,
	}
	translations = evalTmpl(uncompressed_tmpl, v)
	logf("translations: %d bytes\n", len(translations))

	var lines []string
	for _, t := range langs[0].translations {
		s := fmt.Sprintf("  %s", c_escape(t))
		lines = append(lines, s)
	}
	original_strings := strings.Join(lines, ",\n")
	logf("orignal_strings: %d bytes\n", len(original_strings))
	langs_count := len(langs)
	translations_count := len(keys)

	v2 := struct {
		Orignal_strings    string
		Langs_count        int
		Translations_count int
		Translations       string
		Langcodes          string
		Langnames          string
		Langids            string
		Islangrtl          string
	}{
		Orignal_strings:    original_strings,
		Langs_count:        langs_count,
		Translations_count: translations_count,
		Translations:       translations,
		Langcodes:          langcodes,
		Langnames:          langnames,
		Langids:            langids,
		Islangrtl:          islangrtl,
	}
	path := filepath.Join(dir_name, file_name_from_dir_name(dir_name))
	file_content := evalTmpl(compact_c_tmpl, v2)
	logf("file_content: path: %s, file size: %d\n", path, len(file_content))
	u.WriteFileMust(path, []byte(file_content))
	print_incomplete_langs(dir_name)
	// print_stats(langs)
}

func gen_c_code(strings_dict map[string][]*Translation, strings2 []*StringWithPath) {
	for _, dir := range dirsToProcess {
		dirToCheck := filepath.Base(dir)
		var keys []string
		for _, el := range strings2 {
			if el.Dir == dirToCheck {
				s := el.Text
				if _, ok := strings_dict[s]; ok {
					keys = append(keys, s)
				}
			}
		}
		keys = uniquifyStrings(keys)
		sort.Slice(keys, func(i, j int) bool {
			a := strings.Replace(keys[i], `\t`, "\t", -1)
			b := strings.Replace(keys[j], `\t`, "\t", -1)
			return a < b
		})
		gen_c_code_for_dir(strings_dict, keys, dir)
	}
}

func testEscape() {
	d := u.ReadFileMust("t.txt")
	s := string(d)
	logf("len(d): %d, len(s): %d, d: '%s'\n", len(d), len(s), string(d))
	s2 := c_escape(string(d))
	logf("s2: '%s'\n", s2)
}
