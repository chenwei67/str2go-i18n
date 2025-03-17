package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io" // 添加这一行导入 io 包
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 测试命令行参数处理
// 修改测试断言，检查输出文件中是否包含 i18n.Localizer.MustLocalize 调用
// 而不是检查 i18n.T 调用
func TestMainWithArgs(t *testing.T) {
	// 保存原始参数
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "str2go-i18n-test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建测试文件
	inputFile := filepath.Join(tempDir, "input.go")
	content := `package test
func main() {
	s := "你好，世界"
}`
	if err := os.WriteFile(inputFile, []byte(content), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	outputFile := filepath.Join(tempDir, "output.go")

	// 设置命令行参数
	os.Args = []string{"cmd", inputFile, outputFile}

	// 执行 main 函数
	main()

	// 验证输出文件是否存在
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Errorf("输出文件未创建")
	}

	// 读取输出文件内容
	outputContent, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("无法读取输出文件: %v", err)
	}

	// 检查输出文件是否包含 i18n.Localizer.MustLocalize 调用
	// 而不是检查 i18n.T 调用
	if !strings.Contains(string(outputContent), "i18n.Localizer.MustLocalize") {
		t.Error("输出文件内容不正确，未找到 i18n.Localizer.MustLocalize 调用")
	}
}

func TestTransform(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "transform Chinese string",
			input: `package main

import "github.com/nicksnyder/go-i18n/v2/i18n"

func example() {
    s := "你好世界"
}`,
			expected: `package main

import "github.com/nicksnyder/go-i18n/v2/i18n"

func example() {
	s := i18n.Localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "nhsj", DefaultMessage: &i18n.Message{ID: "nhsj", Other: "你好世界"}})
}`,
		},
		{
			name: "ignore English string",
			input: `package main

func example() {
	s := "Hello World"
}`,
			expected: `package main

func example() {
	s := "Hello World"
}`,
		},
		{
			name: "ignore struct tags",
			input: `package main

type Person struct {
	Name string ` + "`json:\"姓名\"`" + `
}`,
			expected: `package main

type Person struct {
	Name string ` + "`json:\"姓名\"`" + `
}`,
		},
		{
			name: "ignore wrapped string",
			input: `package main

import "github.com/nicksnyder/go-i18n/v2/i18n"

func example() {
	s := i18n.Localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "nhsj", DefaultMessage: &i18n.Message{ID: "nhsj", Other: "你好世界"}})
}`,
			expected: `package main

import "github.com/nicksnyder/go-i18n/v2/i18n"

func example() {
	s := i18n.Localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "nhsj", DefaultMessage: &i18n.Message{ID: "nhsj", Other: "你好世界"}})
}`,
		},
		{
			name: "ignore Chinese in comments",
			input: `package main

// 这是一个中文注释
func example() {
	// 另一个中文注释
	s := "Hello"
	/* 这也是中文注释 */
}`,
			expected: `package main

// 这是一个中文注释
func example() {
	// 另一个中文注释
	s := "Hello"
	/* 这也是中文注释 */
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.input, parser.ParseComments)
			assert.NoError(t, err)

			transform(file, fset)

			// 将转换后的 AST 转换回字符串
			var buf strings.Builder
			err = printer.Fprint(&buf, fset, file)
			assert.NoError(t, err)

			// 规范化字符串（移除多余的空白字符）
			normalizedResult := strings.TrimSpace(buf.String())
			normalizedExpected := strings.TrimSpace(tt.expected)

			assert.Equal(t, normalizedExpected, normalizedResult)
		})
	}
}

func TestGenerateMessageID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Chinese characters",
			input:    `"你好世界"`,
			expected: "nhsj",
		},
		{
			name:     "Mixed content",
			input:    `"Hello 世界"`,
			expected: "sj",
		},
		{
			name:     "Empty string",
			input:    `""`,
			expected: "msg",
		},
		{
			name:     "Non-Chinese string",
			input:    `"Hello"`,
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateMessageID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsInComment(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{
			name: "string in line comment",
			code: `package main
// This is a "测试" comment
func main() {}`,
			expected: true,
		},
		{
			name: "string in block comment",
			code: `package main
/* This is a "测试" comment */
func main() {}`,
			expected: true,
		},
		{
			name: "string not in comment",
			code: `package main
func main() {
    s := "测试"
}`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			assert.NoError(t, err)

			// 找到第一个字符串字面量
			var stringLit *ast.BasicLit
			ast.Inspect(file, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					stringLit = lit
					return false
				}
				return true
			})

			if stringLit != nil {
				result := isInComment(stringLit, file, fset)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestCollectAndPrintChineseStrings(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedCount   int
		expectedStrings []string
	}{
		{
			name: "collect Chinese strings",
			input: `package main

func example() {
    s1 := "你好世界"
    s2 := "Hello World"
    s3 := "中文字符串"
	s3 := "有占位符的中文串%s"
	s4 := "ff混合23"
}`,
			expectedCount:   4,
			expectedStrings: []string{"你好世界", "中文字符串", "有占位符的中文串%s", "ff混合23"},
		},
		{
			name: "ignore Chinese in comments",
			input: `package main

// 这是一个中文注释
func example() {
    s := "Hello"
    /* 这也是中文注释 */
}`,
			expectedCount:   0,
			expectedStrings: []string{},
		},
		{
			name: "ignore Chinese in struct tags",
			input: `package main

type Person struct {
    Name string ` + "`json:\"姓名\"`" + `
}`,
			expectedCount:   0,
			expectedStrings: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.input, parser.ParseComments)
			assert.NoError(t, err)

			// 重定向标准输出以捕获打印内容
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// 调用函数
			result := collectAndPrintChineseStrings(file)

			// 恢复标准输出
			w.Close()
			os.Stdout = oldStdout

			// 读取捕获的输出
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// 验证结果
			assert.Equal(t, tt.expectedCount, len(result), "收集到的中文字符串数量不匹配")
			assert.Equal(t, tt.expectedStrings, result, "收集到的中文字符串不匹配")

			// 验证输出包含预期信息
			if tt.expectedCount > 0 {
				assert.Contains(t, output, "找到以下中文字符串:", "输出应包含提示信息")
				t.Logf("%v", output)
				for _, str := range tt.expectedStrings {
					assert.Contains(t, output, str, "输出应包含中文字符串")
				}
			} else {
				assert.Contains(t, output, "未找到中文字符串", "输出应包含未找到的提示")
			}
		})
	}
}
