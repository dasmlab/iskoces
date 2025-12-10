.PHONY: proto deps build test clean run install-protoc

# Generate proto stubs for iskoces server
proto:
	@echo "Generating Go code from proto files..."
	@mkdir -p pkg/proto/v1
	@protoc \
		--go_out=pkg/proto/v1 \
		--go_opt=paths=source_relative \
		--go-grpc_out=pkg/proto/v1 \
		--go-grpc_opt=paths=source_relative \
		--proto_path=proto \
		proto/translation.proto
	@echo "Proto code generated successfully!"

# Install dependencies
deps:
	@echo "Downloading Go dependencies..."
	@go mod download
	@go mod tidy

# Install protoc and plugins
install-protoc:
	@echo "Installing protoc and plugins..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Install protoc compiler: sudo apt install protobuf-compiler"

# Build the binary
build: proto
	@echo "Building iskoces server..."
	@go build -o bin/iskoces-server ./cmd/server
	@echo "Build complete: bin/iskoces-server"

# Build test client
build-test: proto
	@echo "Building test client..."
	@go build -o bin/test-client ./cmd/testclient
	@echo "Build complete: bin/test-client"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -rf pkg/proto/v1/*.pb.go
	@rm -rf pkg/proto/v1/*_grpc.pb.go

# Run the server locally (development)
run: build
	@echo "Running iskoces server..."
	@./bin/iskoces-server

