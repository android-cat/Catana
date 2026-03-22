package syntax

import (
	"context"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	tsTypescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

// TSEngine はTree-sitterベースのシンタックス解析エンジン
type TSEngine struct {
	languages map[string]*sitter.Language
	queries   map[string]*sitter.Query
}

// TSTree はTree-sitterのパースツリー
type TSTree struct {
	Tree *sitter.Tree
	Lang string
}

// NewTSEngine は新しいTree-sitterエンジンを生成する
func NewTSEngine() *TSEngine {
	e := &TSEngine{
		languages: make(map[string]*sitter.Language),
		queries:   make(map[string]*sitter.Query),
	}
	e.registerLanguages()
	return e
}

// Supports は指定言語がTree-sitterに対応しているか返す
func (e *TSEngine) Supports(lang string) bool {
	_, ok := e.queries[lang]
	return ok
}

func (e *TSEngine) registerLanguages() {
	type langDef struct {
		lang  *sitter.Language
		query string
	}

	defs := map[string]langDef{
		"go":         {golang.GetLanguage(), goQuery},
		"rust":       {rust.GetLanguage(), rustQuery},
		"python":     {python.GetLanguage(), pythonQuery},
		"typescript": {tsTypescript.GetLanguage(), tsQuery},
		"javascript": {javascript.GetLanguage(), jsQuery},
	}

	for name, def := range defs {
		e.languages[name] = def.lang
		q, err := sitter.NewQuery([]byte(def.query), def.lang)
		if err == nil {
			e.queries[name] = q
		}
	}
}

// Parse はソースコードを解析しパースツリーを返す
func (e *TSEngine) Parse(source []byte, lang string, oldTree *TSTree) *TSTree {
	sitterLang, ok := e.languages[lang]
	if !ok {
		return nil
	}
	parser := sitter.NewParser()
	parser.SetLanguage(sitterLang)

	var old *sitter.Tree
	if oldTree != nil && oldTree.Tree != nil && oldTree.Lang == lang {
		old = oldTree.Tree
	}

	tree, err := parser.ParseCtx(context.Background(), old, source)
	if err != nil {
		return nil
	}
	return &TSTree{Tree: tree, Lang: lang}
}

// EditTree はインクリメンタルパース用にツリーのEdit情報を反映する
func (e *TSEngine) EditTree(tree *TSTree, edit sitter.EditInput) {
	if tree != nil && tree.Tree != nil {
		tree.Tree.Edit(edit)
	}
}

// HighlightLines はパースツリーからハイライトスパンを行ごとに生成する
func (e *TSEngine) HighlightLines(tree *TSTree, source []byte, lineCount int) [][]Span {
	if tree == nil || tree.Tree == nil {
		return nil
	}
	query, ok := e.queries[tree.Lang]
	if !ok {
		return nil
	}

	root := tree.Tree.RootNode()
	qc := sitter.NewQueryCursor()
	qc.Exec(query, root)

	type capture struct {
		startByte uint32
		endByte   uint32
		tokenType TokenType
	}

	var captures []capture
	for {
		m, ok := qc.NextMatch()
		if !ok {
			break
		}
		for _, c := range m.Captures {
			name := query.CaptureNameForId(c.Index)
			tt := captureToToken(name)
			captures = append(captures, capture{
				startByte: c.Node.StartByte(),
				endByte:   c.Node.EndByte(),
				tokenType: tt,
			})
		}
	}

	// startByte順でソート、同位置なら短いスパン（具体的なキャプチャ）優先
	sort.Slice(captures, func(i, j int) bool {
		if captures[i].startByte != captures[j].startByte {
			return captures[i].startByte < captures[j].startByte
		}
		return (captures[i].endByte - captures[i].startByte) < (captures[j].endByte - captures[j].startByte)
	})

	// 行の開始バイトオフセットを計算
	lineStarts := make([]uint32, lineCount+1)
	lineIdx := 0
	lineStarts[0] = 0
	for i, b := range source {
		if b == '\n' {
			lineIdx++
			if lineIdx < lineCount {
				lineStarts[lineIdx] = uint32(i) + 1
			}
		}
	}
	lineStarts[lineCount] = uint32(len(source))

	// 各行のスパンを生成
	result := make([][]Span, lineCount)
	for line := 0; line < lineCount; line++ {
		lineStart := lineStarts[line]
		lineEnd := lineStarts[line+1]
		if lineEnd > lineStart && lineEnd <= uint32(len(source)) && source[lineEnd-1] == '\n' {
			lineEnd--
		}
		if lineStart >= lineEnd {
			result[line] = []Span{{Text: "", Type: TokenPlain}}
			continue
		}

		var spans []Span
		pos := lineStart

		for i := range captures {
			cap := &captures[i]
			if cap.endByte <= lineStart {
				continue
			}
			if cap.startByte >= lineEnd {
				break
			}

			cStart := cap.startByte
			cEnd := cap.endByte
			if cStart < lineStart {
				cStart = lineStart
			}
			if cEnd > lineEnd {
				cEnd = lineEnd
			}

			// キャプチャ前の未ハイライト部分
			if cStart > pos {
				spans = append(spans, Span{
					Text: string(source[pos:cStart]),
					Type: TokenPlain,
				})
			}
			if cEnd > cStart && cEnd > pos {
				actualStart := cStart
				if pos > actualStart {
					actualStart = pos
				}
				spans = append(spans, Span{
					Text: string(source[actualStart:cEnd]),
					Type: cap.tokenType,
				})
			}
			if cEnd > pos {
				pos = cEnd
			}
		}

		// 行末の残り
		if pos < lineEnd {
			spans = append(spans, Span{
				Text: string(source[pos:lineEnd]),
				Type: TokenPlain,
			})
		}
		if len(spans) == 0 {
			spans = append(spans, Span{Text: string(source[lineStart:lineEnd]), Type: TokenPlain})
		}
		result[line] = spans
	}
	return result
}

// captureToToken はキャプチャ名をTokenTypeに変換する
func captureToToken(name string) TokenType {
	switch {
	case name == "comment":
		return TokenComment
	case name == "string":
		return TokenString
	case name == "number":
		return TokenNumber
	case name == "keyword":
		return TokenKeyword
	case strings.HasPrefix(name, "type"):
		return TokenType_
	case strings.HasPrefix(name, "function"):
		return TokenFunction
	case name == "builtin" || name == "constant.builtin":
		return TokenBuiltin
	case name == "operator":
		return TokenOperator
	case name == "punctuation":
		return TokenPunctuation
	default:
		return TokenPlain
	}
}

// --- 言語ごとのハイライトクエリ ---

var goQuery = `
(comment) @comment
(interpreted_string_literal) @string
(raw_string_literal) @string
(rune_literal) @string
(int_literal) @number
(float_literal) @number
(imaginary_literal) @number
(type_identifier) @type
(function_declaration name: (identifier) @function)
(method_declaration name: (field_identifier) @function)
(call_expression function: (identifier) @function)
(call_expression function: (selector_expression field: (field_identifier) @function))
(true) @builtin
(false) @builtin
(nil) @builtin
(iota) @builtin
["package" "import" "func" "return" "if" "else" "for" "range" "var" "const" "type" "struct" "interface" "map" "chan" "go" "defer" "select" "switch" "case" "default" "break" "continue" "fallthrough" "goto"] @keyword`

var rustQuery = `
(line_comment) @comment
(block_comment) @comment
(string_literal) @string
(raw_string_literal) @string
(char_literal) @string
(integer_literal) @number
(float_literal) @number
(boolean_literal) @builtin
(type_identifier) @type
(primitive_type) @type
(mutable_specifier) @keyword
(function_item name: (identifier) @function)
(call_expression function: (identifier) @function)
(call_expression function: (field_expression field: (field_identifier) @function))
["fn" "let" "const" "static" "pub" "mod" "use" "struct" "enum" "trait" "impl" "for" "while" "loop" "if" "else" "match" "return" "break" "continue" "as" "in" "ref" "move" "async" "await" "dyn" "type" "where" "extern" "unsafe"] @keyword
(self) @builtin
(crate) @builtin`

var pythonQuery = `
(comment) @comment
(string) @string
(integer) @number
(float) @number
(true) @builtin
(false) @builtin
(none) @builtin
(identifier) @variable
(function_definition name: (identifier) @function)
(class_definition name: (identifier) @type)
(call function: (identifier) @function)
(call function: (attribute attribute: (identifier) @function))
["def" "class" "return" "if" "elif" "else" "for" "while" "break" "continue" "pass" "try" "except" "finally" "with" "as" "import" "from" "raise" "yield" "lambda" "global" "nonlocal" "del" "assert" "and" "or" "not" "in" "is" "async" "await"] @keyword`

var tsQuery = `
(comment) @comment
(string) @string
(template_string) @string
(number) @number
(true) @builtin
(false) @builtin
(null) @builtin
(undefined) @builtin
(type_identifier) @type
(predefined_type) @type
(function_declaration name: (identifier) @function)
(method_definition name: (property_identifier) @function)
(call_expression function: (identifier) @function)
(call_expression function: (member_expression property: (property_identifier) @function))
["function" "return" "if" "else" "for" "while" "do" "switch" "case" "default" "break" "continue" "throw" "try" "catch" "finally" "new" "delete" "typeof" "instanceof" "in" "of" "var" "let" "const" "class" "extends" "import" "export" "from" "as" "async" "await" "yield" "void"] @keyword`

var jsQuery = `
(comment) @comment
(string) @string
(template_string) @string
(number) @number
(true) @builtin
(false) @builtin
(null) @builtin
(undefined) @builtin
(function_declaration name: (identifier) @function)
(method_definition name: (property_identifier) @function)
(call_expression function: (identifier) @function)
(call_expression function: (member_expression property: (property_identifier) @function))
["function" "return" "if" "else" "for" "while" "do" "switch" "case" "default" "break" "continue" "throw" "try" "catch" "finally" "new" "delete" "typeof" "instanceof" "in" "of" "var" "let" "const" "class" "extends" "import" "export" "from" "as" "async" "await" "yield" "void"] @keyword`
