param(
    [string]$CompilerRoot = 'E:\mingw64',
    [string]$BuildDirectory = 'build/mingw-debug',
    [switch]$Test
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    $compiler = Join-Path $CompilerRoot 'bin/g++.exe'
    if (-not (Test-Path -LiteralPath $compiler)) { throw 'Specify -CompilerRoot for the MinGW toolchain used to build gRPC.' }
    $env:PATH = (Join-Path $CompilerRoot 'bin') + ';' + $env:PATH
    $temporaryDirectory = Join-Path $projectRoot 'build/tmp'
    New-Item -ItemType Directory -Path $temporaryDirectory -Force | Out-Null
    $env:TEMP = $temporaryDirectory
    $env:TMP = $temporaryDirectory
    $env:TMPDIR = $temporaryDirectory
    cmake -S . -B $BuildDirectory -G Ninja -DCMAKE_BUILD_TYPE=Debug -DBUILD_TESTING=ON "-DCMAKE_CXX_COMPILER=$compiler" "-DCMAKE_C_COMPILER=$(Join-Path $CompilerRoot 'bin/gcc.exe')"
    if ($LASTEXITCODE -ne 0) { throw 'CMake configuration failed.' }
    cmake --build $BuildDirectory --parallel 2
    if ($LASTEXITCODE -ne 0) { throw 'C++ build failed.' }
    if ($Test) {
        ctest --test-dir $BuildDirectory --output-on-failure
        if ($LASTEXITCODE -ne 0) { throw 'Transport regression test failed.' }
    }
} finally {
    Pop-Location
}
