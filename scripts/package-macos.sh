#!/bin/bash
# 打包 macOS 应用：universal .app + DMG 安装包
#
# 用法: scripts/package-macos.sh [输出版本号]
# 环境变量:
#   FASTYPE_SIGN_IDENTITY  指定时用该证书签名 (默认 ad-hoc: "-")
#
# 产物: dist/Fastype-<版本>-macos.dmg, dist/fastype-macos-universal

set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-$(sed -n 's/^var version = "\(.*\)"$/\1/p' cmd/fastype/app.go | head -1)}"
SIGN_IDENTITY="${FASTYPE_SIGN_IDENTITY:--}"
BUILD=build/dmg
STAGE="$BUILD/stage"

rm -rf "$BUILD"
mkdir -p "$STAGE" dist

echo "==> 构建 universal binary (arm64 + amd64)"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="clang -arch arm64" \
	go build -ldflags "-s -w" -o "$BUILD/fastype-arm64" ./cmd/fastype
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="clang -arch x86_64" \
	go build -ldflags "-s -w" -o "$BUILD/fastype-amd64" ./cmd/fastype
lipo -create -output "$BUILD/fastype" "$BUILD/fastype-arm64" "$BUILD/fastype-amd64"
lipo -info "$BUILD/fastype"

echo "==> 组装 Fastype.app"
APP="$STAGE/Fastype.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BUILD/fastype" "$APP/Contents/MacOS/fastype"
cp assets/AppIcon.icns "$APP/Contents/Resources/AppIcon.icns"
cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key><string>Fastype</string>
    <key>CFBundleDisplayName</key><string>Fastype</string>
    <key>CFBundleIdentifier</key><string>com.xiongyihui.fastype</string>
    <key>CFBundleVersion</key><string>$VERSION</string>
    <key>CFBundleShortVersionString</key><string>$VERSION</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleExecutable</key><string>fastype</string>
    <key>CFBundleIconFile</key><string>AppIcon</string>
    <key>LSUIElement</key><true/>
    <key>LSMinimumSystemVersion</key><string>13.0</string>
    <key>NSHighResolutionCapable</key><true/>
    <key>CFBundleURLTypes</key>
    <array>
        <dict>
            <key>CFBundleURLName</key><string>com.xiongyihui.fastype.config</string>
            <key>CFBundleURLSchemes</key><array><string>fastype</string></array>
        </dict>
    </array>
</dict>
</plist>
EOF

echo "==> 签名 ($SIGN_IDENTITY)"
if [ "$SIGN_IDENTITY" = "-" ]; then
  # ad-hoc 签名时把授权要求(designated requirement)固定为 bundle identifier:
  # 默认的 cdhash 绑定会导致每次重新构建后「辅助功能」授权失效。
  codesign --force --sign "$SIGN_IDENTITY" \
    -r="designated => identifier \"com.xiongyihui.fastype\"" "$APP"
else
  codesign --force --sign "$SIGN_IDENTITY" "$APP"
fi
codesign --verify --verbose=1 "$APP"

echo "==> 生成 DMG"
ln -sfn /Applications "$STAGE/Applications"
DMG="dist/Fastype-$VERSION-macos.dmg"
rm -f "$DMG"
hdiutil create -volname "Fastype" -srcfolder "$STAGE" -ov -format UDZO "$DMG" -quiet
hdiutil verify "$DMG" -quiet && echo "DMG 校验通过"

cp "$BUILD/fastype" "dist/fastype-macos-universal"
echo "==> 完成: $DMG"
