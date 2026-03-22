package syntax

import (
	"strings"
	"unicode"
)

// TokenType はシンタックストークンの種別
type TokenType int

const (
	TokenPlain TokenType = iota
	TokenKeyword
	TokenType_
	TokenString
	TokenComment
	TokenNumber
	TokenFunction
	TokenOperator
	TokenPunctuation
	TokenBuiltin
)

// Span はハイライトされたテキスト断片
type Span struct {
	Text string
	Type TokenType
}

// Language は言語固有のシンタックス定義
type Language struct {
	Name              string
	Keywords          map[string]bool
	Types             map[string]bool
	Builtins          map[string]bool
	LineComment       string
	BlockCommentStart string
	BlockCommentEnd   string
	StringDelimiters  []byte
}

// Highlighter はシンタックスハイライトエンジン
type Highlighter struct {
	languages map[string]*Language
	TS        *TSEngine
}

// NewHighlighter は新しいハイライターを生成する
func NewHighlighter() *Highlighter {
	h := &Highlighter{
		languages: make(map[string]*Language),
		TS:        NewTSEngine(),
	}
	h.registerGo()
	h.registerRust()
	h.registerPython()
	h.registerTypeScript()
	return h
}

// DetectLanguage はファイル名から言語を判定する
func (h *Highlighter) DetectLanguage(filename string) string {
	ext := strings.ToLower(filename)
	if idx := strings.LastIndex(ext, "."); idx >= 0 {
		ext = ext[idx:]
	}
	switch ext {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	default:
		return "plain"
	}
}

// HighlightLine は1行のテキストをハイライトする
func (h *Highlighter) HighlightLine(line string, lang string) []Span {
	language, ok := h.languages[lang]
	if !ok {
		// JavaScript は TypeScript の定義を流用
		if lang == "javascript" {
			language, ok = h.languages["typescript"]
		}
	}
	if !ok {
		return []Span{{Text: line, Type: TokenPlain}}
	}
	return h.tokenizeLine(line, language)
}

func (h *Highlighter) tokenizeLine(line string, lang *Language) []Span {
	var spans []Span
	i := 0

	for i < len(line) {
		// 行コメントの判定
		if lang.LineComment != "" && strings.HasPrefix(line[i:], lang.LineComment) {
			spans = append(spans, Span{Text: line[i:], Type: TokenComment})
			return spans
		}

		// ブロックコメント開始の判定（行内のみ）
		if lang.BlockCommentStart != "" && strings.HasPrefix(line[i:], lang.BlockCommentStart) {
			endIdx := strings.Index(line[i+len(lang.BlockCommentStart):], lang.BlockCommentEnd)
			if endIdx >= 0 {
				end := i + len(lang.BlockCommentStart) + endIdx + len(lang.BlockCommentEnd)
				spans = append(spans, Span{Text: line[i:end], Type: TokenComment})
				i = end
				continue
			}
			// 閉じがなければ行末まで
			spans = append(spans, Span{Text: line[i:], Type: TokenComment})
			return spans
		}

		// 文字列リテラルの判定
		if isStringDelimiter(line[i], lang.StringDelimiters) {
			delim := line[i]
			j := i + 1
			for j < len(line) {
				if line[j] == '\\' && j+1 < len(line) {
					j += 2 // エスケープシーケンスをスキップ
					continue
				}
				if line[j] == delim {
					j++
					break
				}
				j++
			}
			spans = append(spans, Span{Text: line[i:j], Type: TokenString})
			i = j
			continue
		}

		// バッククォート文字列（GoのRaw Stringなど）
		if line[i] == '`' {
			j := i + 1
			for j < len(line) && line[j] != '`' {
				j++
			}
			if j < len(line) {
				j++
			}
			spans = append(spans, Span{Text: line[i:j], Type: TokenString})
			i = j
			continue
		}

		// 数値リテラルの判定
		if isDigitStart(line, i) {
			j := i
			if j < len(line) && (line[j] == '-' || line[j] == '+') {
				j++
			}
			for j < len(line) && (isDigitChar(line[j]) || line[j] == '.' || line[j] == 'x' || line[j] == 'o' || line[j] == 'b' || line[j] == '_') {
				j++
			}
			spans = append(spans, Span{Text: line[i:j], Type: TokenNumber})
			i = j
			continue
		}

		// 識別子/キーワードの判定
		if isIdentStart(line[i]) {
			j := i + 1
			for j < len(line) && isIdentChar(line[j]) {
				j++
			}
			word := line[i:j]
			tokenType := TokenPlain
			if lang.Keywords[word] {
				tokenType = TokenKeyword
			} else if lang.Types[word] {
				tokenType = TokenType_
			} else if lang.Builtins[word] {
				tokenType = TokenBuiltin
			} else if j < len(line) && line[j] == '(' {
				tokenType = TokenFunction
			}
			spans = append(spans, Span{Text: word, Type: tokenType})
			i = j
			continue
		}

		// 演算子の判定
		if isOperator(line[i]) {
			j := i + 1
			// 複数文字演算子（::, ->, =>, <=, >=, !=, ==, +=, -=）
			for j < len(line) && isOperator(line[j]) && j-i < 3 {
				j++
			}
			spans = append(spans, Span{Text: line[i:j], Type: TokenOperator})
			i = j
			continue
		}

		// 括弧・区切り文字
		if isPunctuation(line[i]) {
			spans = append(spans, Span{Text: string(line[i]), Type: TokenPunctuation})
			i++
			continue
		}

		// 空白やその他
		j := i + 1
		for j < len(line) && !isIdentStart(line[j]) && !isOperator(line[j]) && !isPunctuation(line[j]) &&
			!isStringDelimiter(line[j], lang.StringDelimiters) && line[j] != '`' &&
			!(lang.LineComment != "" && strings.HasPrefix(line[j:], lang.LineComment)) {
			if unicode.IsDigit(rune(line[j])) {
				break
			}
			j++
		}
		spans = append(spans, Span{Text: line[i:j], Type: TokenPlain})
		i = j
	}

	if len(spans) == 0 {
		spans = append(spans, Span{Text: "", Type: TokenPlain})
	}
	return spans
}

func isStringDelimiter(b byte, delims []byte) bool {
	for _, d := range delims {
		if b == d {
			return true
		}
	}
	return false
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentChar(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

func isDigitStart(line string, i int) bool {
	if i >= len(line) {
		return false
	}
	return line[i] >= '0' && line[i] <= '9'
}

func isDigitChar(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func isOperator(b byte) bool {
	return b == '+' || b == '-' || b == '*' || b == '/' || b == '=' || b == '<' || b == '>' ||
		b == '!' || b == '&' || b == '|' || b == '^' || b == '%' || b == '~' || b == ':' || b == '?'
}

func isPunctuation(b byte) bool {
	return b == '(' || b == ')' || b == '{' || b == '}' || b == '[' || b == ']' ||
		b == ';' || b == ',' || b == '.' || b == '#' || b == '@'
}

// toSet はスライスをマップに変換する
func toSet(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, item := range items {
		m[item] = true
	}
	return m
}

// --- 言語定義 ---

func (h *Highlighter) registerGo() {
	h.languages["go"] = &Language{
		Name: "Go",
		Keywords: toSet([]string{
			"break", "case", "chan", "const", "continue", "default", "defer",
			"else", "fallthrough", "for", "func", "go", "goto", "if",
			"import", "interface", "map", "package", "range", "return",
			"select", "struct", "switch", "type", "var",
		}),
		Types: toSet([]string{
			"bool", "byte", "complex64", "complex128", "error", "float32", "float64",
			"int", "int8", "int16", "int32", "int64", "rune", "string",
			"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
			"any", "comparable",
		}),
		Builtins: toSet([]string{
			"append", "cap", "close", "complex", "copy", "delete", "imag",
			"len", "make", "new", "panic", "print", "println", "real", "recover",
			"true", "false", "nil", "iota",
		}),
		LineComment:       "//",
		BlockCommentStart: "/*",
		BlockCommentEnd:   "*/",
		StringDelimiters:  []byte{'"', '\''},
	}
}

func (h *Highlighter) registerRust() {
	h.languages["rust"] = &Language{
		Name: "Rust",
		Keywords: toSet([]string{
			"as", "async", "await", "break", "const", "continue", "crate",
			"dyn", "else", "enum", "extern", "false", "fn", "for", "if",
			"impl", "in", "let", "loop", "match", "mod", "move", "mut",
			"pub", "ref", "return", "self", "Self", "static", "struct",
			"super", "trait", "true", "type", "unsafe", "use", "where", "while",
			"yield",
		}),
		Types: toSet([]string{
			"i8", "i16", "i32", "i64", "i128", "isize",
			"u8", "u16", "u32", "u64", "u128", "usize",
			"f32", "f64", "bool", "char", "str",
			"String", "Vec", "Box", "Rc", "Arc", "Option", "Result",
			"HashMap", "HashSet", "Mutex", "RwLock",
		}),
		Builtins: toSet([]string{
			"Some", "None", "Ok", "Err", "println", "print", "eprintln",
			"format", "vec", "todo", "unimplemented", "unreachable",
			"assert", "assert_eq", "assert_ne", "debug_assert",
		}),
		LineComment:       "//",
		BlockCommentStart: "/*",
		BlockCommentEnd:   "*/",
		StringDelimiters:  []byte{'"'},
	}
}

func (h *Highlighter) registerPython() {
	h.languages["python"] = &Language{
		Name: "Python",
		Keywords: toSet([]string{
			"False", "None", "True", "and", "as", "assert", "async", "await",
			"break", "class", "continue", "def", "del", "elif", "else",
			"except", "finally", "for", "from", "global", "if", "import",
			"in", "is", "lambda", "nonlocal", "not", "or", "pass", "raise",
			"return", "try", "while", "with", "yield",
		}),
		Types: toSet([]string{
			"int", "float", "str", "bool", "list", "dict", "tuple", "set",
			"bytes", "bytearray", "complex", "frozenset", "type",
		}),
		Builtins: toSet([]string{
			"abs", "all", "any", "bin", "chr", "dir", "divmod", "enumerate",
			"eval", "filter", "format", "getattr", "globals", "hasattr",
			"hash", "hex", "id", "input", "isinstance", "issubclass",
			"iter", "len", "locals", "map", "max", "min", "next", "oct",
			"open", "ord", "pow", "print", "range", "repr", "reversed",
			"round", "setattr", "slice", "sorted", "staticmethod", "sum",
			"super", "vars", "zip", "self",
		}),
		LineComment:       "#",
		BlockCommentStart: "",
		BlockCommentEnd:   "",
		StringDelimiters:  []byte{'"', '\''},
	}
}

func (h *Highlighter) registerTypeScript() {
	keywords := toSet([]string{
		"abstract", "as", "async", "await", "break", "case", "catch",
		"class", "const", "continue", "debugger", "default", "delete",
		"do", "else", "enum", "export", "extends", "false", "finally",
		"for", "from", "function", "get", "if", "implements", "import",
		"in", "instanceof", "interface", "let", "module", "namespace",
		"new", "null", "of", "package", "private", "protected", "public",
		"readonly", "return", "require", "set", "static", "super",
		"switch", "this", "throw", "true", "try", "type", "typeof",
		"undefined", "var", "void", "while", "with", "yield",
	})

	types := toSet([]string{
		"any", "boolean", "number", "string", "symbol", "object",
		"never", "unknown", "void", "null", "undefined",
		"Array", "Map", "Set", "Promise", "Record", "Partial",
		"Required", "Readonly", "Pick", "Omit",
	})

	lang := &Language{
		Name:              "TypeScript",
		Keywords:          keywords,
		Types:             types,
		Builtins:          toSet([]string{"console", "Math", "JSON", "Date", "Error", "RegExp", "parseInt", "parseFloat", "isNaN", "isFinite"}),
		LineComment:       "//",
		BlockCommentStart: "/*",
		BlockCommentEnd:   "*/",
		StringDelimiters:  []byte{'"', '\'', '`'},
	}
	h.languages["typescript"] = lang
	h.languages["javascript"] = lang
}
