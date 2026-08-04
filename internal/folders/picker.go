package folders

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type Picker interface {
	Pick(context.Context) ([]string, error)
}

type NativePicker struct{}

type DirectoryPicker interface {
	PickOne(context.Context, string) (string, error)
}

func NewPicker() Picker {
	return NativePicker{}
}

func NewDirectoryPicker() DirectoryPicker {
	return NativePicker{}
}

func (NativePicker) Pick(ctx context.Context) ([]string, error) {
	path, err := NativePicker{}.PickOne(ctx, "选择要添加到 ProjectDock 的项目文件夹")
	if err != nil {
		return nil, err
	}
	return []string{path}, nil
}

func (NativePicker) PickOne(ctx context.Context, prompt string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("原生文件夹选择器当前只支持 macOS")
	}
	prompt = strings.ReplaceAll(strings.TrimSpace(prompt), `"`, `\"`)
	if prompt == "" {
		prompt = "选择文件夹"
	}
	script := `set chosenFolder to choose folder with prompt "` + prompt + `"
POSIX path of chosenFolder`
	output, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if strings.Contains(strings.ToLower(message), "canceled") || strings.Contains(message, "已取消") {
			return "", errors.New("已取消选择文件夹")
		}
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("打开文件夹选择器: %s", message)
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		return "", errors.New("文件夹选择器没有返回路径")
	}
	return path, nil
}
