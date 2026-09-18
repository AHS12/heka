#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>

// Native macOS notifications for Heka (SPEC-17 §4.13). The system derives
// the toast icon from the notifying process's bundle — the daemon runs from
// Heka.app, so toasts carry the Heka icon with no icon plumbing. beeep's
// osascript fallback (used when this shim declines) attributes to Script
// Editor and cannot show the icon at all.

// notifyAvailable reports whether posting is possible right now: the
// UNUserNotificationCenter framework exists and an NSApplication is running
// (the tray owns one; HEKA_NO_TRAY daemons have none and fall back).
int notifyAvailable(void) {
    if (![UNUserNotificationCenter class]) {
        return 0;
    }
    return NSApp != nil ? 1 : 0;
}

// notifyPost requests authorization (first call shows the one-time macOS
// prompt) and posts the toast. Returns 0 on success, nonzero otherwise:
//   1 — center unavailable
//   2 — permission not granted (prompt declined or timed out)
//   3 — the system rejected the request
// No sound is attached: Heka's own sound pipeline (PlaySound) owns audio so
// presets and the silent setting are honored.
int notifyPost(const char* title, const char* message) {
    if (!NSApp) {
        return 1;
    }
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
    if (!center) {
        return 1;
    }

    dispatch_semaphore_t authSem = dispatch_semaphore_create(0);
    __block BOOL granted = NO;
    [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                          completionHandler:^(BOOL ok, NSError *error) {
        granted = ok;
        dispatch_semaphore_signal(authSem);
    }];
    // The very first call surfaces the system permission prompt; the user
    // may take a moment, so give the answer a generous window. If it lapses,
    // this toast falls back to osascript and later toasts go native.
    if (dispatch_semaphore_wait(authSem, dispatch_time(DISPATCH_TIME_NOW, 30LL * NSEC_PER_SEC)) != 0) {
        return 2;
    }
    if (!granted) {
        return 2;
    }

    UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
    content.title = [NSString stringWithUTF8String:title];
    content.body = [NSString stringWithUTF8String:message];
    // Sound stays off — PlaySound owns audio (presets, silent setting).
    content.sound = nil;

    UNNotificationRequest *request =
        [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
                                             content:content
                                             trigger:nil];

    dispatch_semaphore_t postSem = dispatch_semaphore_create(0);
    __block BOOL posted = NO;
    [center addNotificationRequest:request
              withCompletionHandler:^(NSError *error) {
        posted = (error == nil);
        dispatch_semaphore_signal(postSem);
    }];
    if (dispatch_semaphore_wait(postSem, dispatch_time(DISPATCH_TIME_NOW, 10LL * NSEC_PER_SEC)) != 0) {
        return 3;
    }
    return posted ? 0 : 3;
}
