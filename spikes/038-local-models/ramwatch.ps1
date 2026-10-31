param([string]$Out, [string]$Stop, [int]$Every = 3)
# SPIKE-038: samples the local Ollama's memory every few seconds until
# $Stop exists. Read-only: /api/ps, and the working set of the ollama
# processes. One CSV line per sample.
if (-not (Test-Path $Out)) { "time,model,ps_size_mib,ps_vram_mib,runner_ws_mib,runner_peak_ws_mib,runner_private_mib,available_mib" | Out-File -Encoding utf8 $Out }
while (-not (Test-Path $Stop)) {
  $t = Get-Date -Format s
  $model = ""; $size = 0; $vram = 0
  try {
    $ps = Invoke-RestMethod -Uri http://127.0.0.1:11434/api/ps -TimeoutSec 5
    foreach ($m in $ps.models) { $model += $m.name + ";"; $size += $m.size; $vram += $m.size_vram }
  } catch {}
  $procs = Get-Process -Name "llama-server","ollama*" -ErrorAction SilentlyContinue
  $ws = ($procs | Measure-Object -Property WorkingSet64 -Sum).Sum
  $peak = ($procs | Measure-Object -Property PeakWorkingSet64 -Sum).Sum
  $priv = ($procs | Measure-Object -Property PrivateMemorySize64 -Sum).Sum
  $free = (Get-Counter '\Memory\Available MBytes').CounterSamples[0].CookedValue * 1MB
  "{0},{1},{2:F0},{3:F0},{4:F0},{5:F0},{6:F0},{7:F0}" -f $t, $model, ($size/1MB), ($vram/1MB), ($ws/1MB), ($peak/1MB), ($priv/1MB), ($free/1MB) | Out-File -Append -Encoding utf8 $Out
  Start-Sleep -Seconds $Every
}
