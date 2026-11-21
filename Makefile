.PHONY: all build test integration generate_proto install run
all: build
build:
	go build -o gosync ./cmd/server
test:
	CGO_ENABLED=1 go test -race ./...
integration:
	python3 integration/smoke.py
install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.1
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
generate_proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/filesync.proto
run: build
	./gosync --sync-folder ./sync_folder --insecure
benchmark:
	go test ./internal/servers -run '^$$' -bench BenchmarkTransferWorkloads -benchtime=1x -count=1
