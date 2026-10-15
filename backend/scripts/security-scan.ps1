$ErrorActionPreference = "Stop"

Write-Host "Running Go vulnerability scan..."
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Write-Host "Running repository secret scan..."
go run github.com/zricethezav/gitleaks/v8@v8.30.1 detect --source . --redact --no-banner
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
