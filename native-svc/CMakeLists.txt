# native-svc — out-of-tree LLVM obfuscation service (issue #82, ADR 0004).
#
# Builds a standalone executable that reads LLVM bitcode/IR on stdin, applies
# the selected obfuscation passes, and writes bitcode on stdout. It is NEVER
# linked into the Go build; the SHIELD engine stays stdlib-only and CGO-free.
#
#   cmake -S native-svc -B native-svc/build -G Ninja \
#         -DLLVM_DIR=/usr/lib/llvm-18/lib/cmake/llvm
#   cmake --build native-svc/build
cmake_minimum_required(VERSION 3.20)
project(native-svc C CXX) # C enabled too: LLVM's config runs C try_compile checks

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Pin the toolchain via -DLLVM_DIR=/usr/lib/llvm-18/lib/cmake/llvm; we accept the
# whole 18.x line (LLVM's config version check is exact, so don't request "18").
find_package(LLVM REQUIRED CONFIG)
if(LLVM_PACKAGE_VERSION VERSION_LESS 18)
  message(FATAL_ERROR "native-svc needs LLVM >= 18, found ${LLVM_PACKAGE_VERSION}")
endif()
message(STATUS "native-svc: using LLVM ${LLVM_PACKAGE_VERSION} (${LLVM_DIR})")

# LLVMSupport's exported link interface references these imported targets; define
# them so the export can be consumed from an out-of-tree project.
find_package(ZLIB REQUIRED)
find_package(zstd QUIET)

include_directories(SYSTEM ${LLVM_INCLUDE_DIRS})
separate_arguments(LLVM_DEFINITIONS_LIST NATIVE_COMMAND "${LLVM_DEFINITIONS}")
add_definitions(${LLVM_DEFINITIONS_LIST})

# LLVM on Ubuntu is built without RTTI; match it to avoid link errors.
if(NOT LLVM_ENABLE_RTTI)
  add_compile_options(-fno-rtti)
endif()

add_executable(native-svc
  src/main.cpp
  src/Flatten.cpp
  src/MBA.cpp
  src/Opaque.cpp
  src/Rasp.cpp
  src/Strings.cpp
  src/Tamper.cpp
)

llvm_map_components_to_libnames(llvm_libs
  core support irreader bitwriter bitreader transformutils analysis)

target_link_libraries(native-svc PRIVATE ${llvm_libs})
