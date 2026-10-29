#import <Cocoa/Cocoa.h>

void viplaySetTitleVisible(void *nsWindow, bool visible) {
    ((NSWindow *)nsWindow).titleVisibility = visible ? NSWindowTitleVisible : NSWindowTitleHidden;
}
