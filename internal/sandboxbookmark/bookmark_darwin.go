//go:build darwin && cgo

package sandboxbookmark

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

static CFURLRef projectdock_resolve_bookmark(const void *bytes, CFIndex length, Boolean *stale, CFErrorRef *error) {
    CFDataRef data = CFDataCreate(kCFAllocatorDefault, bytes, length);
    if (data == NULL) return NULL;
    CFURLRef url = CFURLCreateByResolvingBookmarkData(
        kCFAllocatorDefault,
        data,
        kCFURLBookmarkResolutionWithSecurityScope,
        NULL,
        NULL,
        stale,
        error
    );
    CFRelease(data);
    return url;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

type Handle interface {
	Path() string
	Close()
}

type darwinHandle struct {
	url  C.CFURLRef
	path string
}

func Resolve(data []byte) (Handle, error) {
	if len(data) == 0 {
		return nil, errors.New("安全作用域书签为空")
	}
	var stale C.Boolean
	var cfError C.CFErrorRef
	url := C.projectdock_resolve_bookmark(unsafe.Pointer(&data[0]), C.CFIndex(len(data)), &stale, &cfError)
	if url == 0 {
		if cfError != 0 {
			C.CFRelease(C.CFTypeRef(cfError))
		}
		return nil, errors.New("无法解析安全作用域书签")
	}
	if C.CFURLStartAccessingSecurityScopedResource(url) == 0 {
		C.CFRelease(C.CFTypeRef(url))
		return nil, errors.New("无法启用安全作用域目录权限")
	}
	pathRef := C.CFURLCopyFileSystemPath(url, C.kCFURLPOSIXPathStyle)
	if pathRef == 0 {
		C.CFURLStopAccessingSecurityScopedResource(url)
		C.CFRelease(C.CFTypeRef(url))
		return nil, errors.New("无法读取授权目录路径")
	}
	defer C.CFRelease(C.CFTypeRef(pathRef))
	maximum := C.CFStringGetMaximumSizeForEncoding(C.CFStringGetLength(pathRef), C.kCFStringEncodingUTF8) + 1
	buffer := C.malloc(C.size_t(maximum))
	if buffer == nil {
		C.CFURLStopAccessingSecurityScopedResource(url)
		C.CFRelease(C.CFTypeRef(url))
		return nil, errors.New("为授权目录路径分配内存失败")
	}
	defer C.free(buffer)
	if C.CFStringGetCString(pathRef, (*C.char)(buffer), maximum, C.kCFStringEncodingUTF8) == 0 {
		C.CFURLStopAccessingSecurityScopedResource(url)
		C.CFRelease(C.CFTypeRef(url))
		return nil, fmt.Errorf("无法转换授权目录路径")
	}
	return &darwinHandle{url: url, path: C.GoString((*C.char)(buffer))}, nil
}

func (h *darwinHandle) Path() string { return h.path }

func (h *darwinHandle) Close() {
	if h.url != 0 {
		C.CFURLStopAccessingSecurityScopedResource(h.url)
		C.CFRelease(C.CFTypeRef(h.url))
		h.url = 0
	}
}
