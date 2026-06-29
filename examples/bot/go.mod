// Separate module so the example's mellium.im dependency doesn't bloat the
// main API/worker binaries. Build by running `go run .` inside this folder
// (a `go work` file at the repo root would also work).
module github.com/krovara/krovara/examples/bot

go 1.26

require mellium.im/xmpp v0.22.0
