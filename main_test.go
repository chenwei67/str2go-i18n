package main

import (
	"go/parser"
	"go/printer" // 添加这一行导入 printer 包
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransform(t *testing.T) {
	// 测试用例
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "基本中文字符串转换",
			input: `package test
func main() {
	s := "你好，世界"
}`,
			expected: `package test

import "github.com/yourproject/i18n"

func main() {
	s := i18n.T("你好，世界")
}`,
		},
		{
			name: "忽略非中文字符串",
			input: `package test
func main() {
	s1 := "Hello, World"
	s2 := "你好，世界"
}`,
			expected: `package test

import "github.com/yourproject/i18n"

func main() {
	s1 := "Hello, World"
	s2 := i18n.T("你好，世界")
}`,
		},
		{
			name: "忽略结构体标签",
			input: `package test
type Person struct {
	Name string ` + "`json:\"姓名\"`" + `
}`,
			expected: `package test

type Person struct {
	Name string ` + "`json:\"姓名\"`" + `
}`,
		},
		{
			name: "忽略已包装的字符串",
			input: `package test

import "github.com/yourproject/i18n"

func main() {
	s := i18n.T("你好，世界")
}`,
			expected: `package test

import "github.com/yourproject/i18n"

func main() {
	s := i18n.T("你好，世界")
}`,
		},
		{
			name: "忽略注释中的中文",
			input: `package test
// 这是一个中文注释
func main() {
	// 另一个中文注释
	s := "Hello"
}`,
			expected: `package test

// 这是一个中文注释
func main() {
	// 另一个中文注释
	s := "Hello"
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建临时目录
			tempDir, err := os.MkdirTemp("", "str2go-i18n-test")
			if err != nil {
				t.Fatalf("创建临时目录失败: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// 创建输入文件
			inputFile := filepath.Join(tempDir, "input.go")
			if err := os.WriteFile(inputFile, []byte(tt.input), 0644); err != nil {
				t.Fatalf("写入输入文件失败: %v", err)
			}

			// 创建输出文件
			outputFile := filepath.Join(tempDir, "output.go")

			// 解析输入文件
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, inputFile, nil, parser.ParseComments)
			if err != nil {
				t.Fatalf("解析输入文件失败: %v", err)
			}

			// 转换
			transform(file, fset)

			// 写入输出文件
			out, err := os.Create(outputFile)
			if err != nil {
				t.Fatalf("创建输出文件失败: %v", err)
			}
			if err := printer.Fprint(out, fset, file); err != nil {
				t.Fatalf("写入输出文件失败: %v", err)
			}
			out.Close()

			// 读取输出文件
			output, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatalf("读取输出文件失败: %v", err)
			}

			// 规范化空白字符进行比较
			normalizedOutput := strings.ReplaceAll(strings.TrimSpace(string(output)), "\r\n", "\n")
			normalizedExpected := strings.ReplaceAll(strings.TrimSpace(tt.expected), "\r\n", "\n")

			if normalizedOutput != normalizedExpected {
				t.Errorf("输出与预期不符\n期望:\n%s\n\n实际:\n%s", normalizedExpected, normalizedOutput)
			}
		})
	}
}

// 测试命令行参数处理
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
	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("读取输出文件失败: %v", err)
	}

	// 验证输出内容
	if !strings.Contains(string(output), "i18n.T(\"你好，世界\")") {
		t.Errorf("输出文件内容不正确，未找到 i18n.T 调用")
	}
}
