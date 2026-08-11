# Run as Administrator.
# Forwards Windows :3000 -> current WSL IP :3000 and opens firewall.

$ErrorActionPreference = "Stop"

function Get-WslIp {
  $line = (wsl -e sh -c "hostname -I" 2>$null)
  if (-not $line) { throw "Cannot get WSL IP (is WSL running?)" }
  return ($line.Trim() -split "\s+")[0]
}

$wslIp = Get-WslIp
Write-Host "WSL IP: $wslIp"

netsh interface portproxy delete v4tov4 listenaddress=0.0.0.0 listenport=3000 2>$null | Out-Null
netsh interface portproxy add v4tov4 listenaddress=0.0.0.0 listenport=3000 connectaddress=$wslIp connectport=3000

Remove-NetFirewallRule -DisplayName "Lampa Sync 3000" -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName "Lampa Sync 3000" `
  -Direction Inbound -Protocol TCP -LocalPort 3000 `
  -Action Allow -Profile Any | Out-Null

Write-Host "Portproxy:"
netsh interface portproxy show v4tov4

$lan = Get-NetIPAddress -AddressFamily IPv4 |
  Where-Object { $_.InterfaceAlias -match 'Wi-Fi|Ethernet|WLAN' -and $_.IPAddress -notlike '169.254.*' } |
  Select-Object -ExpandProperty IPAddress -First 1

if ($lan) {
  Write-Host ""
  Write-Host "Phone URL: http://$($lan):3000"
  Write-Host "Health:    http://$($lan):3000/health"
} else {
  Write-Host "Could not detect LAN IP — check ipconfig"
}
