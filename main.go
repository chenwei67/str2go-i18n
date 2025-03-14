package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"regexp"

	"golang.org/x/tools/go/ast/astutil"
)

var hasChinese = regexp.MustCompile(`\p{Han}`)

func main() {
	if len(os.Args) != 3 {
		println("Usage: transform <input.go> <output.go>")
		return
	}
	inputFile := os.Args[1]
	outputFile := os.Args[2]

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, inputFile, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	transform(file, fset)

	out, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	if err := printer.Fprint(out, fset, file); err != nil {
		panic(err)
	}
}

func transform(file *ast.File, fset *token.FileSet) {
	needsImport := false
	
	pre := func(cursor *astutil.Cursor) bool {
		n := cursor.Node()

		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		if isInStructTag(cursor) {
			return true
		}

		if isWrappedByI18nT(cursor) {
			return true
		}

		if !hasChinese.MatchString(lit.Value) {
			return true
		}

		// 注释中的字符串不应该被处理
		if isInComment(lit, file, fset) {
			return true
		}

		needsImport = true
		
		newNode := &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent("i18n"),
				Sel: ast.NewIdent("T"),
			},
			Args: []ast.Expr{lit},
		}

		cursor.Replace(newNode)
		return true
	}

	astutil.Apply(file, pre, nil)

	if needsImport {
		ensureI18nImport(file, fset)
	}
}

func isInStructTag(cursor *astutil.Cursor) bool {
	parent := cursor.Parent()
	if parent == nil {
		return false
	}

	field, ok := parent.(*ast.Field)
	if !ok {
		return false
	}

	return field.Tag == cursor.Node()
}

func isWrappedByI18nT(cursor *astutil.Cursor) bool {
	parent := cursor.Parent()
	if parent == nil {
		return false
	}

	callExpr, ok := parent.(*ast.CallExpr)
	if !ok {
		return false
	}

	selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	xIdent, ok := selExpr.X.(*ast.Ident)
	if !ok || xIdent.Name != "i18n" {
		return false
	}

	if selExpr.Sel.Name != "T" {
		return false
	}

	for _, arg := range callExpr.Args {
		if arg == cursor.Node() {
			return true
		}
	}

	return false
}

func ensureI18nImport(file *ast.File, fset *token.FileSet) {
	const importPath = "github.com/yourproject/i18n"

	for _, imp := range file.Imports {
		if imp.Path.Value == `"`+importPath+`"` {
			return
		}
	}

	// 使用非命名导入而不是命名导入
	astutil.AddImport(fset, file, importPath)
}

// isInComment 检查给定的节点是否位于注释中
func isInComment(node ast.Node, file *ast.File, fset *token.FileSet) bool {
	// 获取节点的位置信息
	nodePos := fset.Position(node.Pos())
	nodeEnd := fset.Position(node.End())

	// 检查所有注释
	for _, commentGroup := range file.Comments {
		for _, comment := range commentGroup.List {
			commentPos := fset.Position(comment.Pos())
			commentEnd := fset.Position(comment.End())

			// 如果节点位置在注释范围内，则返回true
			if (nodePos.Line > commentPos.Line || (nodePos.Line == commentPos.Line && nodePos.Column >= commentPos.Column)) &&
				(nodeEnd.Line < commentEnd.Line || (nodeEnd.Line == commentEnd.Line && nodeEnd.Column <= commentEnd.Column)) {
				return true
			}
		}
	}
	return false
}
