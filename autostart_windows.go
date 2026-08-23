//go:build windows && amd64

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// 开机自启：写/删当前用户 HKCU 的 Run 注册表键（无需管理员），
// 与把快捷方式放进 shell:startup 等效，由托盘菜单一键开关。

var advapi32 = syscall.NewLazyDLL("advapi32.dll")

var (
	pRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	pRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	pRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	pRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

const (
	hkeyCurrentUser uintptr = 0x80000001

	regSz               = 1
	regOptionNonVolatile = 0

	keyQueryValue = 0x0001
	keySetValue   = 0x0002

	errSuccessWin      = 0
	errFileNotFoundWin = 2
)

const (
	autoStartRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartValue  = "Fastype"
)

// autoStartEnabled 查询开机自启是否已开启。
func autoStartEnabled() bool {
	sub := mustUTF16(autoStartRunKey)
	var h uintptr
	r, _, _ := pRegOpenKeyExW.Call(hkeyCurrentUser,
		uintptr(unsafe.Pointer(&sub[0])), 0, keyQueryValue, uintptr(unsafe.Pointer(&h)))
	if r != errSuccessWin {
		return false
	}
	defer pRegCloseKey.Call(h)
	name := mustUTF16(autoStartValue)
	var typ, size uint32
	r, _, _ = pRegQueryValueExW.Call(h,
		uintptr(unsafe.Pointer(&name[0])), 0,
		uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)))
	return r == errSuccessWin
}

// autoStartSet 开启/关闭开机自启，注册当前正在运行的这个 exe。
func autoStartSet(enable bool) error {
	if !enable {
		return autoStartDelete()
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取程序路径失败: %w", err)
	}
	cmd := mustUTF16(`"` + exe + `"`)
	sub := mustUTF16(autoStartRunKey)
	var h uintptr
	r, _, callErr := pRegCreateKeyExW.Call(hkeyCurrentUser,
		uintptr(unsafe.Pointer(&sub[0])), 0, 0,
		regOptionNonVolatile, keySetValue, 0,
		uintptr(unsafe.Pointer(&h)), 0)
	if r != errSuccessWin {
		return fmt.Errorf("打开注册表 Run 键失败: %v %v", r, callErr)
	}
	defer pRegCloseKey.Call(h)
	name := mustUTF16(autoStartValue)
	r, _, callErr = pRegSetValueExW.Call(h,
		uintptr(unsafe.Pointer(&name[0])), 0, regSz,
		uintptr(unsafe.Pointer(&cmd[0])), uintptr(len(cmd)*2)) // 含结尾 NUL 的 UTF-16 字节数
	if r != errSuccessWin {
		return fmt.Errorf("写入注册表失败: %v %v", r, callErr)
	}
	return nil
}

func autoStartDelete() error {
	sub := mustUTF16(autoStartRunKey)
	var h uintptr
	r, _, _ := pRegOpenKeyExW.Call(hkeyCurrentUser,
		uintptr(unsafe.Pointer(&sub[0])), 0, keySetValue, uintptr(unsafe.Pointer(&h)))
	if r == errFileNotFoundWin {
		return nil // 键不存在视为已关闭
	}
	if r != errSuccessWin {
		return fmt.Errorf("打开注册表 Run 键失败: %d", r)
	}
	defer pRegCloseKey.Call(h)
	name := mustUTF16(autoStartValue)
	r, _, _ = pRegDeleteValueW.Call(h, uintptr(unsafe.Pointer(&name[0])))
	if r == errFileNotFoundWin {
		return nil // 值不存在也视为成功
	}
	if r != errSuccessWin {
		return fmt.Errorf("删除注册表值失败: %d", r)
	}
	return nil
}
