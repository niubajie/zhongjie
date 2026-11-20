param(
  [string]$ApiBase = "http://127.0.0.1:8080",
  [string]$DevOpenid = "dev-tenant-1"
)

$ErrorActionPreference = "Stop"

Write-Host "health..."
$health = Invoke-RestMethod -Uri "$ApiBase/healthz" -TimeoutSec 5
if ($health.status -ne "ok") { throw "health check failed" }

Write-Host "login..."
$login = Invoke-RestMethod -Method Post -Uri "$ApiBase/v1/auth/wechat/login" `
  -ContentType "application/json" -Body (@{ devOpenid = $DevOpenid } | ConvertTo-Json -Compress)
if ($login.code -ne 0) { throw "login failed: $($login.message)" }
$headers = @{ Authorization = "Bearer $($login.data.token)" }

Write-Host "me..."
$me = Invoke-RestMethod -Uri "$ApiBase/v1/me" -Headers $headers
Write-Host "  user=$($me.data.uuid) role=$($me.data.role) complete=$($me.data.profileCompleted)"

Write-Host "map..."
$map = Invoke-RestMethod -Uri "$ApiBase/v1/properties/map?cityCode=440300&swLat=22.45&swLng=113.85&neLat=22.60&neLng=114.15&zoom=13"
Write-Host "  clusters=$($map.data.clusters.Count) items=$($map.data.items.Count)"

Write-Host "detail..."
$detailMap = Invoke-RestMethod -Uri "$ApiBase/v1/properties/map?cityCode=440300&swLat=22.45&swLng=113.85&neLat=22.60&neLng=114.15&zoom=16"
if ($detailMap.data.items.Count -eq 0) { throw "no listing to check" }
$uuid = $detailMap.data.items[0].uuid
$detail = Invoke-RestMethod -Uri "$ApiBase/v1/properties/$uuid"
Write-Host "  $($detail.data.title) / $($detail.data.areaSqm)sqm / $($detail.data.rentCents)cents"

Write-Host "smoke OK"
