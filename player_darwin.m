#import <Cocoa/Cocoa.h>
#import <OpenGL/gl3.h>
#include <dlfcn.h>
#include <mpv/client.h>
#include <mpv/render_gl.h>

static void *viplayGetProcAddress(void *ctx, const char *name) {
    (void)ctx;
    return dlsym(RTLD_DEFAULT, name);
}

// ViPlayMpvView renders libmpv frames through the OpenGL render API beneath the transparent webview.
@interface ViPlayMpvView : NSOpenGLView {
    mpv_handle *_mpv;
    mpv_render_context *_render;
}
- (instancetype)initWithFrame:(NSRect)frame mpv:(mpv_handle *)mpv;
@end

static void viplayRenderUpdate(void *ctx) {
    ViPlayMpvView *view = (ViPlayMpvView *)ctx;
    dispatch_async(dispatch_get_main_queue(), ^{
        [view setNeedsDisplay:YES];
    });
}

@implementation ViPlayMpvView

- (instancetype)initWithFrame:(NSRect)frame mpv:(mpv_handle *)mpv {
    NSOpenGLPixelFormatAttribute attributes[] = {
        NSOpenGLPFAOpenGLProfile, NSOpenGLProfileVersion3_2Core,
        NSOpenGLPFADoubleBuffer,
        NSOpenGLPFAAccelerated,
        0,
    };
    NSOpenGLPixelFormat *format = [[[NSOpenGLPixelFormat alloc] initWithAttributes:attributes] autorelease];
    self = [super initWithFrame:frame pixelFormat:format];
    if (self) {
        _mpv = mpv;
        [self setWantsBestResolutionOpenGLSurface:YES];
        [self setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
    }
    return self;
}

- (void)prepareOpenGL {
    [super prepareOpenGL];
    GLint swapInterval = 1;
    [[self openGLContext] setValues:&swapInterval forParameter:NSOpenGLContextParameterSwapInterval];
    if (_render != NULL) {
        return;
    }
    mpv_opengl_init_params glParams = {.get_proc_address = viplayGetProcAddress};
    mpv_render_param params[] = {
        {MPV_RENDER_PARAM_API_TYPE, (void *)MPV_RENDER_API_TYPE_OPENGL},
        {MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &glParams},
        {MPV_RENDER_PARAM_INVALID, NULL},
    };
    if (mpv_render_context_create(&_render, _mpv, params) >= 0) {
        mpv_render_context_set_update_callback(_render, viplayRenderUpdate, self);
    }
}

- (void)drawRect:(NSRect)dirtyRect {
    (void)dirtyRect;
    [[self openGLContext] makeCurrentContext];
    NSSize size = [self convertRectToBacking:[self bounds]].size;
    if (_render == NULL) {
        glClearColor(0, 0, 0, 1);
        glClear(GL_COLOR_BUFFER_BIT);
    } else {
        mpv_render_context_update(_render);
        // Layer-backed OpenGL views render into an offscreen framebuffer whose size the viewport reports.
        GLint framebuffer = 0;
        glGetIntegerv(GL_DRAW_FRAMEBUFFER_BINDING, &framebuffer);
        GLint viewport[4] = {0, 0, (GLint)size.width, (GLint)size.height};
        glGetIntegerv(GL_VIEWPORT, viewport);
        mpv_opengl_fbo fbo = {.fbo = framebuffer, .w = viewport[2], .h = viewport[3]};
        int flipY = 1;
        mpv_render_param params[] = {
            {MPV_RENDER_PARAM_OPENGL_FBO, &fbo},
            {MPV_RENDER_PARAM_FLIP_Y, &flipY},
            {MPV_RENDER_PARAM_INVALID, NULL},
        };
        mpv_render_context_render(_render, params);
    }
    [[self openGLContext] flushBuffer];
}

- (void)reshape {
    [super reshape];
    [self setNeedsDisplay:YES];
}

- (NSView *)hitTest:(NSPoint)point {
    (void)point;
    return nil;
}

@end

static NSView *viplayFindWebView(NSView *view) {
    Class webViewClass = NSClassFromString(@"WKWebView");
    if (webViewClass != nil && [view isKindOfClass:webViewClass]) {
        return view;
    }
    for (NSView *child in [view subviews]) {
        NSView *found = viplayFindWebView(child);
        if (found != nil) {
            return found;
        }
    }
    return nil;
}

void viplayAttachMpvView(void *nsWindow, mpv_handle *mpv) {
    NSWindow *window = (NSWindow *)nsWindow;
    NSView *webView = viplayFindWebView([window contentView]);
    NSView *parent = webView != nil ? [webView superview] : [window contentView];
    ViPlayMpvView *view = [[ViPlayMpvView alloc] initWithFrame:[parent bounds] mpv:mpv];
    [parent addSubview:view positioned:NSWindowBelow relativeTo:webView];
    [view release];
    // Wails enables webview transparency only with the private_mac_apis build tag; macOS has no public switch.
    // shortcut: drawsBackground is private KVC, which blocks Mac App Store distribution; revisit if ViPlay targets the store.
    @try {
        [webView setValue:@NO forKey:@"drawsBackground"];
    } @catch (NSException *exception) {
        NSLog(@"ViPlay: could not make the webview transparent: %@", exception.reason);
    }
}
