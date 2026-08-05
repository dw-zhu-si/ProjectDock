//go:build !darwin || !cgo

package sandboxbookmark

import "errors"

type Handle interface {
	Path() string
	Close()
}

func Resolve([]byte) (Handle, error) {
	return nil, errors.New("当前平台不支持 macOS 安全作用域书签")
}
