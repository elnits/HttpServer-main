$env:ARLIAI_API_KEY = '597dbe7e-16ca-4803-ab17-5fa084909f37'
Set-Location 'E:\HttpServer'
Write-Host "Starting Go server on port 9999..."
Write-Host "AI normalization enabled"
go run main.go
