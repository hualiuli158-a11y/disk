cmake_minimum_required(VERSION 3.24)
project(DiskCpp VERSION 0.1.0 LANGUAGES C CXX)
if(POLICY CMP0144)
    cmake_policy(SET CMP0144 NEW)
endif()
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
if(MINGW AND CMAKE_CXX_COMPILER_VERSION VERSION_GREATER_EQUAL 16 AND CMAKE_CXX_COMPILER_VERSION VERSION_LESS 17)
    add_compile_definitions(_GLIBCXX_ATOMIC_WORD_BUILTINS=1)
endif()
set(MOUDU_ASIO_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../MouduAsio" CACHE PATH "MouduAsio source checkout")
set(GRPC_ROOT "" CACHE PATH "gRPC installation prefix")
if(WIN32 AND NOT GRPC_ROOT AND EXISTS "E:/Libraries/grpc-mingw-debug")
    set(GRPC_ROOT "E:/Libraries/grpc-mingw-debug")
endif()
if(GRPC_ROOT)
    list(PREPEND CMAKE_PREFIX_PATH "${GRPC_ROOT}")
endif()
if(WIN32 AND NOT DEFINED Boost_ROOT AND EXISTS "E:/Libraries/boost_1_92_0")
    set(Boost_ROOT "E:/Libraries/boost_1_92_0" CACHE PATH "Boost installation")
endif()
find_package(Boost 1.88 CONFIG REQUIRED)
find_package(Threads REQUIRED)
find_package(OpenSSL CONFIG QUIET)
if(NOT OpenSSL_FOUND)
    find_package(OpenSSL REQUIRED)
endif()
find_package(Protobuf CONFIG QUIET)
if(NOT Protobuf_FOUND)
    find_package(Protobuf REQUIRED)
endif()
find_package(gRPC CONFIG REQUIRED)
if(MINGW)
    # This workstation's gRPC Debug archive contains over 1 GiB of DWARF data.
    # Strip a build-local copy so GNU ld can link within available memory.
    get_target_property(grpc_debug_archive gRPC::grpc IMPORTED_LOCATION_DEBUG)
    if(grpc_debug_archive AND CMAKE_OBJCOPY)
        set(grpc_link_archive "${CMAKE_BINARY_DIR}/grpc/libgrpc.a")
        add_custom_command(OUTPUT "${grpc_link_archive}"
            COMMAND "${CMAKE_COMMAND}" -E make_directory "${CMAKE_BINARY_DIR}/grpc"
            COMMAND "${CMAKE_COMMAND}" -E copy "${grpc_debug_archive}" "${grpc_link_archive}"
            COMMAND "${CMAKE_OBJCOPY}" --strip-debug "${grpc_link_archive}"
            DEPENDS "${grpc_debug_archive}" VERBATIM)
        add_custom_target(disk_grpc_link_archive DEPENDS "${grpc_link_archive}")
        set_property(TARGET gRPC::grpc PROPERTY IMPORTED_LOCATION_DEBUG "${grpc_link_archive}")
    endif()
endif()
add_subdirectory("${MOUDU_ASIO_ROOT}/net" moudu-net)
set(JSONCPP_WITH_TESTS OFF CACHE BOOL "" FORCE)
set(JSONCPP_WITH_POST_BUILD_UNITTEST OFF CACHE BOOL "" FORCE)
add_subdirectory("${MOUDU_ASIO_ROOT}/backend/third_party/jsoncpp" jsoncpp EXCLUDE_FROM_ALL)
add_library(disk_charconv STATIC
    "${MOUDU_ASIO_ROOT}/backend/third_party/boost-charconv/src/from_chars.cpp"
    "${MOUDU_ASIO_ROOT}/backend/third_party/boost-charconv/src/to_chars.cpp")
target_link_libraries(disk_charconv PUBLIC Boost::headers)
target_compile_definitions(disk_charconv PUBLIC BOOST_CHARCONV_STATIC_LINK BOOST_CHARCONV_NO_LIB PRIVATE BOOST_CHARCONV_SOURCE)
include(cmake/Dependencies.cmake)
include("${MOUDU_ASIO_ROOT}/backend/cmake/GrpcProto.cmake")
add_library(disk_proto STATIC)
target_grpc_proto(disk_proto "${CMAKE_CURRENT_SOURCE_DIR}/storage/proto/object_storage.proto")
target_include_directories(disk_proto PUBLIC "${CMAKE_CURRENT_BINARY_DIR}/generated/disk_proto")
target_link_libraries(disk_proto PUBLIC gRPC::grpc++ protobuf::libprotobuf)
add_library(disk_core STATIC
    src/config.cpp src/database.cpp src/auth.cpp src/storage.cpp src/application.cpp
    src/common.cpp src/http_types.cpp
    src/repositories/user_repository.cpp src/repositories/folder_repository.cpp src/repositories/file_repository.cpp
    src/services/account_service.cpp src/services/folder_service.cpp src/services/file_service.cpp
    src/repositories/repository_support.cpp src/repositories/search_repository.cpp src/repositories/trash_repository.cpp
    src/services/search_service.cpp src/services/trash_service.cpp
    src/gin/tree.cpp src/gin/context.cpp src/gin/routergroup.cpp src/gin/engine.cpp
    src/handlers/auth_handler.cpp src/handlers/user_handler.cpp src/handlers/folder_handler.cpp
    src/handlers/file_handler.cpp src/handlers/search_handler.cpp src/handlers/trash_handler.cpp
    src/router/router.cpp
    src/http_server.cpp "${MOUDU_ASIO_ROOT}/backend/src/common/BusinessPool.cpp")
target_include_directories(disk_core PUBLIC src "${CMAKE_CURRENT_SOURCE_DIR}/third_party" "${MOUDU_ASIO_ROOT}/backend/src" "${JWT_SOURCE_DIR}/include")
target_link_libraries(disk_core PUBLIC moudu::net JsonCpp::JsonCpp disk_charconv disk_proto disk_botan disk_multipart OpenSSL::SSL OpenSSL::Crypto)
target_compile_definitions(disk_core PUBLIC BOOST_ASIO_NO_DEPRECATED)
if(MINGW)
    target_compile_options(disk_core PRIVATE -Wa,-mbig-obj)
endif()
add_executable(disk_api src/main.cpp)
target_link_libraries(disk_api PRIVATE disk_core)
if(TARGET disk_grpc_link_archive)
    add_dependencies(disk_api disk_grpc_link_archive)
endif()
include(CTest)
if(BUILD_TESTING)
    add_executable(gin_router_test tests/gin_router_test.cpp)
    target_link_libraries(gin_router_test PRIVATE disk_core)
    add_test(NAME gin_router COMMAND gin_router_test)

    add_executable(disk_transport_test tests/transport_test.cpp)
    target_link_libraries(disk_transport_test PRIVATE disk_core)
    if(TARGET disk_grpc_link_archive)
        add_dependencies(disk_transport_test disk_grpc_link_archive)
    endif()
    add_test(NAME disk_transport COMMAND "${Python3_EXECUTABLE}" "${CMAKE_CURRENT_SOURCE_DIR}/tests/transport_test.py" "$<TARGET_FILE:disk_transport_test>")
    if(WIN32 AND CMAKE_CXX_COMPILER)
        get_filename_component(_compiler_bin_dir "${CMAKE_CXX_COMPILER}" DIRECTORY)
        set_property(TEST disk_transport APPEND PROPERTY
            ENVIRONMENT_MODIFICATION "PATH=path_list_prepend:${_compiler_bin_dir}")
    endif()
    set_tests_properties(disk_transport PROPERTIES TIMEOUT 120)
endif()
