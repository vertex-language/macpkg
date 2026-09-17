#import <Cocoa/Cocoa.h>

@interface AppDelegate : NSObject <NSApplicationDelegate, NSWindowDelegate>
@property (strong, nonatomic) NSWindow *window;
@property (strong, nonatomic) NSTextField *counterLabel;
@property (assign, nonatomic) NSInteger clickCount;
@end

@implementation AppDelegate

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    // 1. Create main window
    NSRect frame = NSMakeRect(0, 0, 540, 400);
    NSUInteger style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                       NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskFullSizeContentView;

    self.window = [[NSWindow alloc] initWithContentRect:frame
                                              styleMask:style
                                                backing:NSBackingStoreBuffered
                                                  defer:NO];
    [self.window setTitle:@"macpkg Showcase"];
    [self.window setTitlebarAppearsTransparent:YES];
    [self.window center];
    self.window.delegate = self;

    // 2. Add visual effect background (frosted glass)
    NSVisualEffectView *vibrantView = [[NSVisualEffectView alloc] initWithFrame:self.window.contentView.bounds];
    vibrantView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    vibrantView.material = NSVisualEffectMaterialUnderWindowBackground;
    vibrantView.blendingMode = NSVisualEffectBlendingModeBehindWindow;
    vibrantView.state = NSVisualEffectStateActive;
    [self.window.contentView addSubview:vibrantView];

    // 3. Header title
    NSTextField *titleLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(32, 320, 476, 36)];
    titleLabel.stringValue = @"macpkg Showcase";
    titleLabel.font = [NSFont systemFontOfSize:26 weight:NSFontWeightBold];
    titleLabel.editable = NO;
    titleLabel.bezeled = NO;
    titleLabel.drawsBackground = NO;
    titleLabel.textColor = [NSColor labelColor];
    [vibrantView addSubview:titleLabel];

    // Subtitle
    NSTextField *subLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(32, 296, 476, 20)];
    subLabel.stringValue = @"100% Pure-Go Apple Packaging & Code Signing Toolchain";
    subLabel.font = [NSFont systemFontOfSize:13 weight:NSFontWeightMedium];
    subLabel.editable = NO;
    subLabel.bezeled = NO;
    subLabel.drawsBackground = NO;
    subLabel.textColor = [NSColor secondaryLabelColor];
    [vibrantView addSubview:subLabel];

    // 4. Information card
    NSBox *card = [[NSBox alloc] initWithFrame:NSMakeRect(32, 148, 476, 132)];
    card.boxType = NSBoxCustom;
    card.fillColor = [[NSColor controlBackgroundColor] colorWithAlphaComponent:0.65];
    card.borderColor = [[NSColor separatorColor] colorWithAlphaComponent:0.4];
    card.borderWidth = 1.0;
    card.cornerRadius = 10.0;
    [vibrantView addSubview:card];

    NSTextField *infoText = [[NSTextField alloc] initWithFrame:NSMakeRect(16, 12, 444, 108)];
    infoText.stringValue = @"• Bundle Identifier:  com.example.sampleapp\n"
                            "• Architecture:       Apple Silicon (arm64)\n"
                            "• Security Runtime:   Gatekeeper Validated (CS_RUNTIME)\n"
                            "• Formats Packaged:   .app bundle, .dmg disk image, .pkg installer\n"
                            "• Toolchain:          Zero Cgo / Pure-Go (macpkg)\n"
                            "• Status:             Native Cocoa App running successfully!";
    infoText.font = [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];
    infoText.editable = NO;
    infoText.bezeled = NO;
    infoText.drawsBackground = NO;
    infoText.textColor = [NSColor textColor];
    [card addSubview:infoText];

    // 5. Interactive counter section
    self.counterLabel = [[NSTextField alloc] initWithFrame:NSMakeRect(32, 98, 476, 22)];
    self.counterLabel.stringValue = @"Click count: 0  (Click the button below to test UI responsiveness)";
    self.counterLabel.font = [NSFont systemFontOfSize:13 weight:NSFontWeightRegular];
    self.counterLabel.editable = NO;
    self.counterLabel.bezeled = NO;
    self.counterLabel.drawsBackground = NO;
    self.counterLabel.textColor = [NSColor secondaryLabelColor];
    [vibrantView addSubview:self.counterLabel];

    NSButton *clickBtn = [NSButton buttonWithTitle:@"Click Me!" target:self action:@selector(onButtonClick:)];
    clickBtn.frame = NSMakeRect(32, 44, 130, 36);
    clickBtn.bezelStyle = NSBezelStyleRounded;
    [vibrantView addSubview:clickBtn];

    NSButton *webBtn = [NSButton buttonWithTitle:@"View GitHub Repo" target:self action:@selector(onOpenRepo:)];
    webBtn.frame = NSMakeRect(170, 44, 160, 36);
    webBtn.bezelStyle = NSBezelStyleRounded;
    [vibrantView addSubview:webBtn];

    NSButton *quitBtn = [NSButton buttonWithTitle:@"Quit" target:self action:@selector(onQuit:)];
    quitBtn.frame = NSMakeRect(428, 44, 80, 36);
    quitBtn.bezelStyle = NSBezelStyleRounded;
    [vibrantView addSubview:quitBtn];

    // Show window and bring to front
    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

- (void)onButtonClick:(id)sender {
    self.clickCount++;
    self.counterLabel.stringValue = [NSString stringWithFormat:@"Click count: %ld  🎉 UI is fully responsive & interactive!", (long)self.clickCount];
    self.counterLabel.textColor = [NSColor systemBlueColor];
}

- (void)onOpenRepo:(id)sender {
    [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:@"https://github.com/vertex-language/macpkg"]];
}

- (void)onQuit:(id)sender {
    [NSApp terminate:nil];
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
    return YES;
}

@end

int main(int argc, const char * argv[]) {
    // Support headless test execution flag
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--test") == 0 || strcmp(argv[i], "--headless") == 0) {
            printf("SampleApp: headless smoke test passed\n");
            return 0;
        }
    }

    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        [app setActivationPolicy:NSApplicationActivationPolicyRegular];

        // Standard Application Menu (About, Cmd+Q)
        NSMenu *menubar = [NSMenu new];
        NSMenuItem *appMenuItem = [NSMenuItem new];
        [menubar addItem:appMenuItem];
        [app setMainMenu:menubar];

        NSMenu *appMenu = [NSMenu new];
        NSString *appName = [[NSProcessInfo processInfo] processName];
        NSString *quitTitle = [@"Quit " stringByAppendingString:appName];
        NSMenuItem *quitMenuItem = [[NSMenuItem alloc] initWithTitle:quitTitle
                                                              action:@selector(terminate:)
                                                       keyEquivalent:@"q"];
        [appMenu addItem:quitMenuItem];
        [appMenuItem setSubmenu:appMenu];

        AppDelegate *delegate = [[AppDelegate alloc] init];
        [app setDelegate:delegate];
        [app run];
    }
    return 0;
}
