#ifndef HEKA_NOTIFY_DARWIN_H
#define HEKA_NOTIFY_DARWIN_H

int notifyAvailable(void);
int notifyPost(const char* title, const char* message);

#endif
