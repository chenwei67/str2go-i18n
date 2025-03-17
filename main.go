package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"regexp"
	"strings"

	"github.com/mozillazg/go-pinyin"
	"golang.org/x/tools/go/ast/astutil"
	"unicode"
)

var hasChinese = regexp.MustCompile(`\p{Han}`)

// 添加一个函数用于收集并输出中文字符串
func collectAndPrintChineseStrings(file *ast.File) []string {
	// 初始化为空切片而不是 nil
	chineseStrings := []string{}
	
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			// 检查是否是中文字符串
			if containsChinese(lit.Value) && !isInComment(lit, file, token.NewFileSet()) && !isInStructTagBasicLit(lit, file) {
				// 去除引号
				strValue := strings.Trim(lit.Value, "`\"")
				chineseStrings = append(chineseStrings, strValue)
			}
		}
		return true
	})
	
	// 输出找到的中文字符串
	if len(chineseStrings) > 0 {
		fmt.Println("找到以下中文字符串:")
		for i, str := range chineseStrings {
			fmt.Printf("%d. %s\n", i+1, str)
		}
	} else {
		fmt.Println("未找到中文字符串")
	}
	
	return chineseStrings
}

// 修改 main 函数，在转换前输出中文字段
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
		fmt.Printf("解析文件失败: %v\n", err)
		return
	}
	
	// 在转换前收集并输出中文字符串
	fmt.Printf("正在分析文件: %s\n", inputFile)
	collectAndPrintChineseStrings(file)
	
	// 转换文件
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

		// 生成消息ID
		msgID := generateMessageID(lit.Value)

		// 创建符合 go-i18n 格式的调用
		// 使用 i18n.Localizer.MustLocalize 和 &i18n.LocalizeConfig
		newNode := &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent("i18n"),
					Sel: ast.NewIdent("Localizer"),
				},
				Sel: ast.NewIdent("MustLocalize"),
			},
			Args: []ast.Expr{
				&ast.UnaryExpr{
					Op: token.AND,
					X: &ast.CompositeLit{
						Type: &ast.SelectorExpr{
							X:   ast.NewIdent("i18n"),
							Sel: ast.NewIdent("LocalizeConfig"),
						},
						Elts: []ast.Expr{
							&ast.KeyValueExpr{
								Key:   ast.NewIdent("MessageID"),
								Value: &ast.BasicLit{Kind: token.STRING, Value: `"` + msgID + `"`},
							},
							&ast.KeyValueExpr{
								Key: ast.NewIdent("DefaultMessage"),
								Value: &ast.UnaryExpr{
									Op: token.AND,
									X: &ast.CompositeLit{
										Type: &ast.SelectorExpr{
											X:   ast.NewIdent("i18n"),
											Sel: ast.NewIdent("Message"),
										},
										Elts: []ast.Expr{
											&ast.KeyValueExpr{
												Key:   ast.NewIdent("ID"),
												Value: &ast.BasicLit{Kind: token.STRING, Value: `"` + msgID + `"`},
											},
											&ast.KeyValueExpr{
												Key:   ast.NewIdent("Other"),
												Value: lit,
											},
										},
									},
								},
							},
						},
					},
				},
			},
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
	// 检查当前节点是否是字符串字面量
	_, ok := cursor.Node().(*ast.BasicLit)
	if !ok {
		return false
	}
	
	// 检查父节点是否是 KeyValueExpr，且 Key 是 "Other"
	parent := cursor.Parent()
	kv, ok := parent.(*ast.KeyValueExpr)
	if !ok {
		return false
	}
	
	key, ok := kv.Key.(*ast.Ident)
	if !ok || key.Name != "Other" {
		return false
	}
	
	// 简化处理：如果是 Other 字段，假设它在 i18n.Message 中
	return true
}

func ensureI18nImport(file *ast.File, fset *token.FileSet) {
	const importPath = "github.com/nicksnyder/go-i18n/v2/i18n"

	for _, imp := range file.Imports {
		if imp.Path.Value == `"`+importPath+`"` {
			return
		}
	}

	// 添加 go-i18n 导入
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

// // generateMessageID 根据中文消息生成唯一ID
// func generateMessageID(message string) string {
// 	// 去除引号
// 	message = strings.Trim(message, `"`)

// 	// 提取前几个字符作为前缀，转为拼音
// 	prefix := extractPinyinPrefix(message, 5)

// 	// 计算消息的哈希值作为后缀，确保唯一性
// 	hash := md5.Sum([]byte(message))
// 	hashStr := fmt.Sprintf("%x", hash)[:8] // 取前8位

// 	// 组合前缀和哈希
// 	return prefix + "_" + hashStr
// }

// generateMessageID 根据中文消息生成唯一ID
func generateMessageID(message string) string {
	// 去除引号
	message = strings.Trim(message, `"`)

	// 提取前几个字符作为前缀，转为拼音
	prefix := extractPinyinPrefix(message, 5)
	// 组合前缀和哈希
	return prefix
}

// extractPinyinPrefix 从中文消息中提取拼音首字母作为前缀
func extractPinyinPrefix(message string, maxChars int) string {
	if len(message) == 0 {
		return "msg"
	}

	// 去除引号
	message = strings.Trim(message, `"`)
	
	// 检查是否包含中文字符
	if hasChinese.MatchString(message) {
		// 如果包含中文，只提取中文字符的拼音
		var result strings.Builder
		count := 0
		
		for _, char := range []rune(message) {
			if hasChinese.MatchString(string(char)) {
				args := pinyin.NewArgs()
				args.Style = pinyin.FirstLetter
				pys := pinyin.Pinyin(string(char), args)
				if len(pys) > 0 && len(pys[0]) > 0 {
					result.WriteString(pys[0][0])
					count++
					if count >= maxChars {
						break
					}
				}
			}
		}
		
		id := result.String()
		if id != "" && regexp.MustCompile(`^[a-zA-Z]`).MatchString(id) {
			return id
		}
		return "msg"
	} else {
		// 如果不包含中文，处理英文和数字
		var result strings.Builder
		count := 0
		
		for _, char := range []rune(message) {
			if regexp.MustCompile(`[a-zA-Z0-9]`).MatchString(string(char)) {
				result.WriteString(strings.ToLower(string(char)))
				count++
				if count >= maxChars {
					break
				}
			}
		}
		
		id := result.String()
		if id != "" && regexp.MustCompile(`^[a-zA-Z]`).MatchString(id) {
			return id
		}
		return "msg"
	}
}

// containsChinese 检查字符串是否包含中文字符
func containsChinese(s string) bool {
	// 去除字符串两端的引号
	s = strings.Trim(s, "`\"")
	
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// isInStructTagBasicLit 检查给定的 BasicLit 是否位于结构体标签中
func isInStructTagBasicLit(lit *ast.BasicLit, file *ast.File) bool {
	// 遍历所有结构体字段
	var result bool
	ast.Inspect(file, func(n ast.Node) bool {
		if field, ok := n.(*ast.Field); ok && field.Tag != nil {
			// 检查标签是否就是当前的字符串字面量
			if field.Tag == lit {
				result = true
				return false
			}
		}
		return true
	})
	return result
}
