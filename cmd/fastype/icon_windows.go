//go:build windows && amd64

package main

import (
	"sort"
	"unsafe"
)

// 程序化绘制托盘图标：白底圆角方块 + 居中 ⚡，避免引入资源文件/第三方库。

var pCreateIcon = user32.NewProc("CreateIcon")

type rgb struct{ r, g, b byte }

// setPix 写入自顶向下的 RGBA 缓冲
func setPix(buf []byte, w, x, y int, c rgb, a byte) {
	i := (y*w + x) * 4
	buf[i], buf[i+1], buf[i+2], buf[i+3] = c.r, c.g, c.b, a
}

// inRoundRect 判断点是否在圆角矩形内（x0,y0 含，x1,y1 不含）
func inRoundRect(x, y, x0, y0, x1, y1, r int) bool {
	if x < x0 || x >= x1 || y < y0 || y >= y1 {
		return false
	}
	dx, dy := 0, 0
	if x < x0+r {
		dx = x0 + r - x
	}
	if x >= x1-r {
		dx = x - (x1 - 1 - r)
	}
	if y < y0+r {
		dy = y0 + r - y
	}
	if y >= y1-r {
		dy = y - (y1 - 1 - r)
	}
	return dx*dx+dy*dy <= r*r
}

// fillPoly 简易多边形填充（扫描奇偶规则）
func fillPoly(buf []byte, w, h int, poly [][2]float64, c rgb) {
	minY, maxY := 1e9, -1e9
	for _, p := range poly {
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	n := len(poly)
	for y := 0; y < h; y++ {
		fy := float64(y) + 0.5
		if fy < minY || fy > maxY {
			continue
		}
		var xs []float64
		for i := 0; i < n; i++ {
			a, b := poly[i], poly[(i+1)%n]
			if (a[1] <= fy && b[1] > fy) || (b[1] <= fy && a[1] > fy) {
				t := (fy - a[1]) / (b[1] - a[1])
				xs = append(xs, a[0]+t*(b[0]-a[0]))
			}
		}
		sort.Float64s(xs) // 交点必须有序，否则奇偶配对会得到负宽度的空区间
		for i := 0; i+1 < len(xs); i += 2 {
			for x := int(xs[i] + 0.5); x <= int(xs[i+1]+0.5); x++ {
				if x >= 0 && x < w {
					setPix(buf, w, x, y, c, 255)
				}
			}
		}
	}
}

// drawKeycap 在 32x32 RGBA 上画图标：白底圆角方块（近满幅）+ 居中 ⚡
func drawKeycap(buf []byte) {
	const w, h = 32, 32
	border := rgb{0xd8, 0xd8, 0xd8} // 1px 浅灰描边（浅色任务栏上可辨）
	face := rgb{0xff, 0xff, 0xff}   // 白色底
	bolt := rgb{0xf5, 0x9e, 0x0b}   // 琥珀色闪电

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			switch {
			case inRoundRect(x, y, 1, 1, 31, 31, 4): // 描边层
				setPix(buf, w, x, y, border, 255)
			case inRoundRect(x, y, 2, 2, 30, 30, 3): // 白色底（28px）
				setPix(buf, w, x, y, face, 255)
			}
		}
	}
	// ⚡ 闪电：标准六边形闪电（上尖→左斜边→中台阶→下尖→右斜边→上台阶），质心约 16,16
	fillPoly(buf, w, h, [][2]float64{
		{16.98, 7.15}, {9.79, 17.27}, {14.46, 17.27},
		{13.39, 25.55}, {21.29, 14.51}, {16.62, 14.51}}, bolt)
}

// rgbaToDIB32 把自顶向下的 RGBA 转成 Win32 需要的自底向上 BGRA
func rgbaToDIB32(rgba []byte, w, h int) []byte {
	out := make([]byte, len(rgba))
	for y := 0; y < h; y++ {
		src := y * w * 4
		dst := (h - 1 - y) * w * 4
		for x := 0; x < w*4; x += 4 {
			out[dst+x] = rgba[src+x+2]
			out[dst+x+1] = rgba[src+x+1]
			out[dst+x+2] = rgba[src+x]
			out[dst+x+3] = rgba[src+x+3]
		}
	}
	return out
}

// buildKeycapIcon 生成 32x32 带透明通道的托盘 HICON。
func buildKeycapIcon() uintptr {
	const w, h = 32, 32
	rgba := make([]byte, w*h*4)
	drawKeycap(rgba)
	xor := rgbaToDIB32(rgba, w, h)
	and := make([]byte, ((w+31)/32*4)*h) // 全 0 = 不透明（透明由 alpha 决定）
	ico, _, _ := pCreateIcon.Call(0, w, h, 1, 32,
		uintptr(unsafe.Pointer(&and[0])), uintptr(unsafe.Pointer(&xor[0])))
	return ico
}
