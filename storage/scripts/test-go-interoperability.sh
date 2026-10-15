#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: bash scripts/test-go-interoperability.sh /path/to/go-project" >&2
  exit 2
fi

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
go_project="$1"
build_dir="${project_dir}/build-test"
cmake -S "${project_dir}" -B "${build_dir}"
cmake --build "${build_dir}" --target grpc_storage_node -j

cd "${go_project}"
STORAGE_TEST_GRPC_BINARY="${build_dir}/grpc_storage_node" \
  go test ./internal/storage/grpcstore -run '^TestCppServerInteroperability$' -count=1 -v
STORAGE_TEST_GRPC_BINARY="${build_dir}/grpc_storage_node" STORAGE_TEST_GRPC_TLS=1 \
  go test ./internal/storage/grpcstore -run '^TestCppServerInteroperability$' -count=1 -v
