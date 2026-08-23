#import <AppKit/AppKit.h>
#import <stdlib.h>
#include "_cgo_export.h" // goTrayCmd

static NSStatusItem* gItem;
static NSMenu* gMenu;
static NSMenuItem* gPauseItem;
static NSMenuItem* gAutoItem;
static id gDelegate;

@interface FastypeDelegate : NSObject
- (void)menuAction:(id)sender;
- (void)applyPauseTitle:(id)s;
- (void)applyTip:(id)s;
- (void)teardown:(id)_;
@end

@implementation FastypeDelegate
- (void)menuAction:(id)sender {
	goTrayCmd((int)[sender tag]);
}
- (void)applyPauseTitle:(id)s {
	[gPauseItem setTitle:(NSString*)s];
}
- (void)applyTip:(id)s {
	[[gItem button] setToolTip:(NSString*)s];
}
// 摘除图标并停止 AppKit 事件循环；投递到主线程执行
- (void)teardown:(id)_ {
	@autoreleasepool {
		if (gItem) {
			[[NSStatusBar systemStatusBar] removeStatusItem:gItem];
			gItem = nil;
		}
	}
	// stop: 只置标志，循环空闲时不会醒来检查——补发一个哑事件唤醒它
	[NSApp stop:nil];
	NSEvent* ev = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
	                                location:NSMakePoint(0, 0)
	                           modifierFlags:0
	                               timestamp:0
		                    windowNumber:-1
		                         context:nil
		                          subtype:0
		                            data1:0
		                            data2:0];
	[NSApp postEvent:ev atStart:YES];
}
@end

static void addMenuItem(const char* title, int tag) {
	NSMenuItem* it = [gMenu addItemWithTitle:[NSString stringWithUTF8String:title]
	                                  action:@selector(menuAction:)
	                           keyEquivalent:@""];
	[it setTarget:gDelegate];
	[it setTag:tag];
	[it setEnabled:YES];
}

// 模板图（仅 alpha 起效，自动适配菜单栏深浅色）：
// 不透明圆角方块 + 镂空（透视）闪电。提供 1x/2x 两个位图表示保证 Retina 清晰。
// 注意：必须用纯 CoreGraphics 位图上下文绘制——经 NSGraphicsContext+
// NSBitmapImageRep 的路线在 2x 尺寸下会产生残缺位图（实测底部被裁、镂空丢失）。

// 闪电六边形（16pt 坐标，CG y 轴向上，顶部尖端 y 值大）
static const CGFloat kBolt[6][2] = {
    {9, 13}, {4, 7.5}, {7.25, 7.5}, {6.5, 3}, {12, 9}, {8.75, 9}};

static NSBitmapImageRep* drawIconRep(NSInteger px) {
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef cg = CGBitmapContextCreate(NULL, px, px, 8, 0, cs, kCGImageAlphaPremultipliedLast);
	CGColorSpaceRelease(cs);
	CGFloat s = (CGFloat)px / 16.0;
	// 圆角方块（占 14pt，四边留 1pt 呼吸空间）
	CGPathRef box = CGPathCreateWithRoundedRect(
	    CGRectMake(1.0 * s, 1.0 * s, 14.0 * s, 14.0 * s), 3.5 * s, 3.5 * s, NULL);
	CGMutablePathRef boltPath = CGPathCreateMutable();
	CGPathMoveToPoint(boltPath, NULL, kBolt[0][0] * s, kBolt[0][1] * s);
	for (int i = 1; i < 6; i++) {
		CGPathAddLineToPoint(boltPath, NULL, kBolt[i][0] * s, kBolt[i][1] * s);
	}
	CGPathCloseSubpath(boltPath);
	// 奇偶填充：方块实心、闪电镂空（透视）
	CGContextAddPath(cg, box);
	CGContextAddPath(cg, boltPath);
	CGContextSetRGBFillColor(cg, 0, 0, 0, 1);
	CGContextEOFillPath(cg);
	CGPathRelease(box);
	CGPathRelease(boltPath);
	CGImageRef img = CGBitmapContextCreateImage(cg);
	NSBitmapImageRep* rep = [[NSBitmapImageRep alloc] initWithCGImage:img];
	[rep setSize:NSMakeSize(16, 16)];
	CGImageRelease(img);
	CGContextRelease(cg);
	return rep;
}

static NSImage* makeMenuBarIcon(void) {
	NSImage* img = [[NSImage alloc] initWithSize:NSMakeSize(16, 16)];
	[img addRepresentation:drawIconRep(16)];
	[img addRepresentation:drawIconRep(32)];
	[img setTemplate:YES];
	return img;
}

void traySetup(const char* tip, const char* open,
               const char* pause, const char* autostart, const char* quit) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[[NSApplication sharedApplication]
		    setActivationPolicy:NSApplicationActivationPolicyAccessory];

		gDelegate = [[FastypeDelegate alloc] init];
		gMenu = [[NSMenu alloc] init];
		[gMenu setAutoenablesItems:NO];

		addMenuItem(open, 1);
		addMenuItem(pause, 2);
		addMenuItem(autostart, 5);
		gPauseItem = [gMenu itemWithTag:2];
		gAutoItem = [gMenu itemWithTag:5];
		[gMenu addItem:[NSMenuItem separatorItem]];
		addMenuItem(quit, 3);

		gItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
		NSButton* btn = [gItem button];
		[btn setImage:makeMenuBarIcon()];
		[btn setToolTip:[NSString stringWithUTF8String:tip]];
		[gItem setMenu:gMenu];
	}
}

void trayRun(void) {
	@autoreleasepool {
		[NSApp run];
	}
}

// 诊断：bit0=item 非 nil, bit1=button 非 nil, bit2=已设图标, bit3=已设文字
int trayDiag(void) {
	int r = 0;
	if (gItem) r |= 1;
	NSButton* btn = [gItem button];
	if (btn) r |= 2;
	if ([btn image]) r |= 4;
	if ([btn title]) r |= 8;
	return r;
}

void trayStopAsync(void) {
	[gDelegate performSelectorOnMainThread:@selector(teardown:)
	                             withObject:nil
	                          waitUntilDone:NO];
}

void traySetPauseTitle(const char* s) {
	@autoreleasepool {
		NSString* title = [NSString stringWithUTF8String:s];
		[gDelegate performSelectorOnMainThread:@selector(applyPauseTitle:)
		                             withObject:title
		                          waitUntilDone:NO];
	}
}

void traySetAutoState(int on) {
	[gAutoItem setState:(on ? NSControlStateValueOn : NSControlStateValueOff)];
}

void traySetTip(const char* s) {
	@autoreleasepool {
		NSString* tip = [NSString stringWithUTF8String:s];
		[gDelegate performSelectorOnMainThread:@selector(applyTip:)
		                             withObject:tip
		                          waitUntilDone:NO];
	}
}
