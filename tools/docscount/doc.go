// Package docscount holds the gate that keeps the derived numbers in
// docs/harness.md from being typed from memory. It has no non-test behaviour;
// doc.go exists so that `go build ./...` sees a real package rather than a
// test-only directory.
//
// IT LIVES HERE, AND NOT IN THE ROOT PACKAGE, FOR A MEASURED REASON. The first
// version of this gate was a _test.go file in package main. On Windows it
// passed; on CI it failed the whole `go test ./...` run with
//
//	glfw: X11: The DISPLAY environment variable is missing
//	panic: glfw: The GLFW library is not initialized
//
// because a test file in package main makes Go build and RUN a test binary for
// package main, whose init chain reaches ebiten's internal/ui.init. The
// project's standing rule is that no test binary may link ebiten; the gate that
// enforces it for d2world, d2input and d2maprenderer does not cover the root
// package, because nothing had ever put a test there. A gate that cannot run on
// the machine that runs the gate is not a gate.
package docscount
