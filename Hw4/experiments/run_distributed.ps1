param(
    [string]$InputS3
)

# ===== HARD-CODE YOUR IPs HERE =====
$SplitterIP = "54.226.138.15"
$Mapper1IP  = "54.152.54.160"
$Mapper2IP  = "54.90.220.121"
$Mapper3IP  = "35.172.203.24"
$ReducerIP  = "100.27.3.254"

$sw = [System.Diagnostics.Stopwatch]::StartNew()

# Split
$splitUrl = "http://$($SplitterIP):8080/split?s3_url=$InputS3&chunks=3"
$splitResp = Invoke-WebRequest -UseBasicParsing $splitUrl
$splitJson = $splitResp.Content | ConvertFrom-Json

$chunk1 = $splitJson.chunks[0]
$chunk2 = $splitJson.chunks[1]
$chunk3 = $splitJson.chunks[2]

# Map in parallel
$jobs = @()
$jobs += Start-Job { param($u) Invoke-WebRequest -UseBasicParsing $u | Out-Null } -ArgumentList "http://$($Mapper1IP):8080/map?s3_url=$chunk1"
$jobs += Start-Job { param($u) Invoke-WebRequest -UseBasicParsing $u | Out-Null } -ArgumentList "http://$($Mapper2IP):8080/map?s3_url=$chunk2"
$jobs += Start-Job { param($u) Invoke-WebRequest -UseBasicParsing $u | Out-Null } -ArgumentList "http://$($Mapper3IP):8080/map?s3_url=$chunk3"

$jobs | Wait-Job | Remove-Job

# Reduce
# If your reducer expects mapper result URLs explicitly, adjust this to read them from S3 or mapper responses.
# Most implementations write predictable keys, so reducer can read fixed locations.
$reduceUrl = "http://$($ReducerIP):8080/reduce?url=s3://mapreduce-nithish-hw4/mapper-results/chunk1.json&url=s3://mapreduce-nithish-hw4/mapper-results/chunk2.json&url=s3://mapreduce-nithish-hw4/mapper-results/chunk3.json"
$reduceResp = Invoke-WebRequest -UseBasicParsing $reduceUrl

$sw.Stop()

$ms = $sw.Elapsed.TotalMilliseconds
Write-Output "DISTRIBUTED_TIME_MS=$ms"
Write-Output $reduceResp.Content
