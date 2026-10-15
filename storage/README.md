# gRPC object storage node

This Linux C++ service implements the same byte-storage role as the existing
`objectRestore` TCP node. The `LocalStore` implementation and its key validation
live in this project's `src/storage` directory.
Go owns user authorization, metadata,
SHA-256 deduplication and database transactions. The node sees only opaque
storage keys and object bytes.

## Build and run

Install the C++ gRPC and Protobuf development packages, `protoc`, and
`grpc_cpp_plugin` on the Linux build host. The C++ storage sources are included
in this project, so the build does not require an `objectRestore` checkout:

```sh
cmake -S . -B build
cmake --build build -j
mkdir -p /tmp/grpc-object-data
./build/grpc_storage_node 127.0.0.1:50051 /tmp/grpc-object-data
```

Plaintext gRPC can also listen on `0.0.0.0:50051` for container port mapping.
The root must be on a Linux filesystem supported by `LocalStore`.

For an encrypted loopback connection, provide a server certificate and private
key:

```sh
./build/grpc_storage_node 127.0.0.1:50051 /srv/object-data server.pem server.key
```

To require and verify a client certificate (mutual TLS), also provide a client
CA certificate:

```sh
./build/grpc_storage_node 0.0.0.0:50051 /srv/object-data server.pem server.key client-ca.pem
```

Keep private keys outside the repository. The server certificate must cover
the DNS name configured by Go's `STORAGE_TLS_SERVER_NAME`. The standard gRPC
`grpc.health.v1.Health/Check` service reports serving status and switches to
not serving during shutdown.

CLion's remote deployment must include `proto/object_storage.proto` as well as
all of `src/` and `CMakeLists.txt`. If `make` reports that it has no rule for the
`.proto` file, upload that file from the local project to
`/home/ubuntu/rpcdisk/proto/object_storage.proto`, then rerun CMake and build.

## API contract

`proto/object_storage.proto` is the source of truth. `Put` receives a key as
its first message and then chunks of at most 64 KiB. The server commits only
after a normal stream finish. `Get` sends the declared size first and then
object chunks. `Delete` is idempotent. `List` streams stored objects for
maintenance; it is not a user-facing folder listing. A positive
`ListRequest.limit` up to 1000 enables keyset pagination; `after_key` is the
last key from the previous page. A short or empty page ends the scan. Zero
limit retains the old unpaged behavior for existing clients.

Keys are relative UTF-8 paths of at most 500 bytes. `LocalStore` validates
them again and prevents symlink traversal. An object is limited to 1 GiB.
Cancellation before commit discards the temporary upload. A connection loss
after commit may leave the client uncertain about the outcome, so Go must
retain compensation and eventually reconcile orphaned objects.

The Go client and generated Go protobuf files live in the sibling Go project
under `internal/storage/grpcstore`. After changing the `.proto`, regenerate the
Go files from that project's root when the directories have the workspace
layout shown here:

```sh
protoc -I ../storage/proto --go_out=internal/storage/grpcstore/pb --go_opt=paths=source_relative --go-grpc_out=internal/storage/grpcstore/pb --go-grpc_opt=paths=source_relative ../storage/proto/object_storage.proto
```

The CMake build generates C++ protobuf files automatically.

## Cross-language test

On a Linux host with both projects available, this command builds the C++ node,
starts it against a temporary root, and runs the Go health and object lifecycle
test once over loopback plaintext and once with mutual TLS:

```sh
bash scripts/test-go-interoperability.sh ../backend
```
