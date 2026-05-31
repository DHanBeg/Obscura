# Obscura Protobuf Schemas

`obscura.proto` defines the canonical Spec EK A wire format.

## Current status

The network currently uses **JSON** for all message encoding.
This file documents the target schema for the protobuf migration.

## Generating Go code

Install protoc and the Go plugin:

```bash
# macOS
brew install protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Ubuntu
apt-get install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

Generate:

```bash
cd backend
protoc \
  --go_out=internal/proto \
  --go_opt=paths=source_relative \
  -I proto \
  proto/obscura.proto
```

Output lands in `backend/internal/proto/obscura.pb.go`.

Add `google.golang.org/protobuf` to `go.mod`:

```bash
go get google.golang.org/protobuf@latest
```

## Migration path

TODO: JSON → protobuf migration when all clients support binary framing.

1. Add `Content-Type: application/x-protobuf` detection in handlers.
2. Wrap `Envelope` marshalling behind a codec interface.
3. Keep JSON fallback for backward compatibility (negotiated via Accept header).
4. Flip default in a minor version bump.
