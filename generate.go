// Package syntrix provides go:generate directives for code generation.
//
// Run "go generate ./..." from project root to regenerate all generated code.
//
// Prerequisites:
//   - protoc: https://grpc.io/docs/protoc-installation/
//   - protoc-gen-go: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
//   - protoc-gen-go-grpc: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
//
//go:generate go run scripts/compile_proto.go
package syntrix
