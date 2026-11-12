FROM ubuntu:22.04 AS builder

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        cmake \
        pkg-config \
        libgrpc++-dev \
        libprotobuf-dev \
        protobuf-compiler \
        protobuf-compiler-grpc \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY CMakeLists.txt ./
COPY proto/ ./proto/
COPY src/ ./src/

RUN cmake -S . -B /build -DCMAKE_BUILD_TYPE=Release \
    && cmake --build /build --target grpc_storage_node --parallel 2

FROM ubuntu:22.04 AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        libgrpc++1 \
        libprotobuf23 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 storage \
    && useradd --uid 10001 --gid storage --no-create-home \
        --shell /usr/sbin/nologin storage \
    && mkdir -p /data \
    && chown storage:storage /data

COPY --from=builder /build/grpc_storage_node /usr/local/bin/grpc_storage_node

USER storage:storage
WORKDIR /data
VOLUME ["/data"]
EXPOSE 50051

# Optional TLS: append server certificate, private key, and optionally client CA
# after the listen address and storage root when overriding command arguments.
ENTRYPOINT ["/usr/local/bin/grpc_storage_node"]
CMD ["0.0.0.0:50051", "/data"]
