cmake_minimum_required(VERSION 3.20)
project(grpc_object_storage LANGUAGES CXX)

if(NOT CMAKE_SYSTEM_NAME STREQUAL "Linux")
    message(FATAL_ERROR "LocalStore uses Linux file-descriptor APIs")
endif()

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

find_package(Protobuf REQUIRED)
find_package(gRPC CONFIG QUIET)
if(NOT TARGET gRPC::grpc++)
    # Some distribution packages expose grpc++ through pkg-config only.
    find_package(PkgConfig REQUIRED)
    pkg_check_modules(GRPC REQUIRED IMPORTED_TARGET grpc++)
    add_library(gRPC::grpc++ ALIAS PkgConfig::GRPC)
endif()
find_program(PROTOC_EXECUTABLE protoc REQUIRED)
find_program(GRPC_CPP_PLUGIN_EXECUTABLE grpc_cpp_plugin REQUIRED)

set(PROTO_FILE "${CMAKE_CURRENT_SOURCE_DIR}/proto/object_storage.proto")
if(NOT EXISTS "${PROTO_FILE}")
    message(FATAL_ERROR
            "Missing ${PROTO_FILE} on the build host. Upload proto/object_storage.proto "
            "from the local grpcService/disk project before configuring.")
endif()
set(GENERATED_DIR "${CMAKE_CURRENT_BINARY_DIR}/generated")
file(MAKE_DIRECTORY "${GENERATED_DIR}")

add_custom_command(
    OUTPUT "${GENERATED_DIR}/object_storage.pb.cc"
           "${GENERATED_DIR}/object_storage.pb.h"
           "${GENERATED_DIR}/object_storage.grpc.pb.cc"
           "${GENERATED_DIR}/object_storage.grpc.pb.h"
    COMMAND "${PROTOC_EXECUTABLE}"
    ARGS --proto_path=${CMAKE_CURRENT_SOURCE_DIR}/proto
         --cpp_out=${GENERATED_DIR}
         --grpc_out=${GENERATED_DIR}
         --plugin=protoc-gen-grpc=${GRPC_CPP_PLUGIN_EXECUTABLE}
         "${PROTO_FILE}"
    DEPENDS "${PROTO_FILE}"
    VERBATIM)

add_library(object_storage_proto
    "${GENERATED_DIR}/object_storage.pb.cc"
    "${GENERATED_DIR}/object_storage.grpc.pb.cc")
target_include_directories(object_storage_proto PUBLIC "${GENERATED_DIR}")
target_link_libraries(object_storage_proto PUBLIC protobuf::libprotobuf gRPC::grpc++)

add_library(object_storage_files src/storage/local_store.cpp)
target_include_directories(object_storage_files PUBLIC "${CMAKE_CURRENT_SOURCE_DIR}/src")

add_executable(grpc_storage_node src/main.cpp src/object_storage_service.cpp)
add_dependencies(grpc_storage_node object_storage_proto)
target_link_libraries(grpc_storage_node PRIVATE object_storage_proto object_storage_files)
