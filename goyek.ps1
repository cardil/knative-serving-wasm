Push-Location "$PSScriptRoot" -ErrorAction Stop
& go run ./build $args
Pop-Location
exit $global:LASTEXITCODE
