param(
    [Parameter(Mandatory = $true)][string]$DiskSource,
    [string]$GinSource = '',
    [string]$MouduSource = '',
    [string]$CompilerRoot = 'E:/mingw64',
    [string]$BoostRoot = 'E:/Libraries/boost_1_92_0',
    [string]$K6 = '',
    [string]$Distro = 'Ubuntu',
    [ValidateRange(1, 86400)][int]$Duration = 60,
    [ValidateRange(0, 3600)][int]$Warmup = 30,
    [ValidateRange(1, 20)][int]$Repeats = 3,
    [switch]$SkipBuild,
    [switch]$SkipIntegration
)
$ErrorActionPreference = 'Stop'
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$DiskSource = (Resolve-Path -LiteralPath $DiskSource).Path
if (-not $GinSource) { $GinSource = Join-Path (Split-Path -Parent $DiskSource) 'gin_ cpp' }
if (-not $MouduSource) { $MouduSource = Join-Path (Split-Path -Parent $DiskSource) 'MouduAsio' }
$GinSource = (Resolve-Path -LiteralPath $GinSource).Path
$MouduSource = (Resolve-Path -LiteralPath $MouduSource).Path
if (-not $K6) { $K6 = Join-Path $projectRoot 'build/tools/k6-v2.4.0-windows-amd64/k6.exe' }
if (-not (Test-Path -LiteralPath $K6)) { throw 'Set -K6 to a k6 executable.' }
$buildRoot = Join-Path $projectRoot 'build/performance-build'
$apiBuild = Join-Path $buildRoot 'disk-release'
$httpBuild = Join-Path $buildRoot 'gin-release'
$storageBuild = Join-Path $buildRoot 'storage-release'
$runDirectory = Join-Path $projectRoot ('build/performance/resume-baseline-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
$env:PATH = (Join-Path $CompilerRoot 'bin') + ';' + $env:PATH
$env:TEMP = Join-Path $projectRoot 'build/tmp'
$env:TMP = $env:TEMP
New-Item -ItemType Directory -Path $env:TEMP -Force | Out-Null

function Invoke-Checked([string]$Program, [string[]]$Arguments) {
    & $Program @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Program failed with exit code $LASTEXITCODE" }
}
function Convert-ToWslPath([string]$Path) {
    $absolute = [IO.Path]::GetFullPath($Path)
    if ($absolute -notmatch '^([A-Za-z]):\\') { throw 'WSL paths must be on a Windows drive.' }
    return '/mnt/' + $Matches[1].ToLowerInvariant() + '/' + $absolute.Substring(3).Replace('\', '/')
}

if (-not $SkipBuild) {
    $diskConfigure = @('-S', $DiskSource, '-B', $apiBuild, '-G', 'Ninja',
        '-DCMAKE_BUILD_TYPE=Release', '-DBUILD_TESTING=ON', "-DGIN_CPP_ROOT=$GinSource",
        "-DMOUDU_ASIO_ROOT=$MouduSource", "-DBoost_ROOT=$BoostRoot",
        "-DCMAKE_CXX_COMPILER=$CompilerRoot/bin/g++.exe", "-DCMAKE_C_COMPILER=$CompilerRoot/bin/gcc.exe")
    foreach ($dependency in @(@('BOTAN_SOURCE_DIR', 'botan-3.10.0'), @('JWT_SOURCE_DIR', 'jwt-cpp-0.7.1'))) {
        $source = Join-Path $projectRoot ('build/sources/' + $dependency[1])
        if (Test-Path -LiteralPath $source) { $diskConfigure += "-D$($dependency[0])=$source" }
    }
    Invoke-Checked 'cmake' $diskConfigure
    Invoke-Checked 'cmake' @('--build', $apiBuild, '--parallel', '2')
    Invoke-Checked 'cmake' @('-S', (Join-Path $PSScriptRoot 'gin'), '-B', $httpBuild, '-G', 'Ninja',
        '-DCMAKE_BUILD_TYPE=Release', '-DBUILD_TESTING=ON', "-DGIN_CPP_ROOT=$GinSource", '-DGIN_USE_ASIO=ON',
        "-DMOUDU_ASIO_ROOT=$MouduSource", "-DBoost_ROOT=$BoostRoot", "-DCMAKE_CXX_COMPILER=$CompilerRoot/bin/g++.exe")
    Invoke-Checked 'cmake' @('--build', $httpBuild, '--parallel', '2')
    Invoke-Checked 'wsl' @('-d', $Distro, '--exec', 'cmake', '-S', (Convert-ToWslPath (Join-Path $DiskSource 'storage')),
        '-B', (Convert-ToWslPath $storageBuild), '-DCMAKE_BUILD_TYPE=Release')
    Invoke-Checked 'wsl' @('-d', $Distro, '--exec', 'cmake', '--build', (Convert-ToWslPath $storageBuild), '--parallel', '2')
}
Invoke-Checked 'ctest' @('--test-dir', $apiBuild, '--output-on-failure', '--no-tests=error')
Invoke-Checked 'ctest' @('--test-dir', $httpBuild, '--output-on-failure', '--no-tests=error')
$runArguments = @((Join-Path $PSScriptRoot 'run.py'), '--source', $DiskSource, '--framework-source', $GinSource,
    '--build', $apiBuild, '--http-build', $httpBuild, '--storage-build', $storageBuild, '--k6', $K6,
    '--distro', $Distro, '--output', $runDirectory, '--stages', 'http,api,files',
    '--duration', "$Duration", '--warmup', "$Warmup", '--repeats', "$Repeats", '--connections', '1,16',
    '--api-vus', '1,8', '--cases', 'profile,contents,search', '--accounts', '4', '--dataset-size', '100',
    '--file-bytes', '1048576', '--rates', '100', '--recovery', '5')
if ($SkipIntegration) { $runArguments += '--skip-integration' }
try {
    Invoke-Checked 'python' $runArguments
} finally {
    if (Test-Path -LiteralPath (Join-Path $runDirectory 'results.json')) {
        Invoke-Checked 'python' @((Join-Path $PSScriptRoot 'summarize.py'), $runDirectory)
    }
}
