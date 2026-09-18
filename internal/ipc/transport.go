// Package ipc is Heka's local API (SPEC-07): HTTP/1.1 over a named pipe
// (Windows) or unix socket (POSIX). The transport moved here from the daemon
// so the endpoint is owned by the contract layer, not the runtime.
package ipc
