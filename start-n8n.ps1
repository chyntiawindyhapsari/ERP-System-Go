# Menjalankan n8n TANPA Docker. Membaca .env lalu menjalankan n8n di http://localhost:5678
# Cara pakai (PowerShell, di folder project):  .\start-n8n.ps1
# Syarat: Node.js LTS terpasang (https://nodejs.org)

Get-Content .env | ForEach-Object {
    $line = $_.Trim()
    if ($line -eq "" -or $line.StartsWith("#") -or -not $line.Contains("=")) { return }
    $name, $value = $line.Split("=", 2)
    $value = ($value -replace "\s+#.*$", "").Trim()
    # Hanya ambil variabel yang dibutuhkan n8n (variabel lain bisa mengganggu n8n)
    if (@("INTEGRATION_API_KEY","OWNER_WA","FONNTE_TOKEN","APP_PORT") -contains $name.Trim()) {
        [Environment]::SetEnvironmentVariable($name.Trim(), $value, "Process")
    }
}

$env:APP_URL = "http://localhost:$(if ($env:APP_PORT) { $env:APP_PORT } else { '8080' })"
$env:N8N_BLOCK_ENV_ACCESS_IN_NODE = "false"
$env:GENERIC_TIMEZONE = "Asia/Jakarta"
$env:TZ = "Asia/Jakarta"

npx n8n
