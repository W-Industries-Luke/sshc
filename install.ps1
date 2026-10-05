# Installs the latest sshc release for this machine:
#   irm https://raw.githubusercontent.com/W-Industries-Luke/sshc/main/install.ps1 | iex
# Downloads the right binary, checks it against the release's SHA256SUMS, and
# runs "sshc --install", which copies it to %LOCALAPPDATA%\Programs\sshc and
# adds that to your user Path.
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $base = 'https://github.com/W-Industries-Luke/sshc/releases/latest/download'

    # A 32-bit PowerShell on 64-bit Windows reports the real one here.
    $cpu = $env:PROCESSOR_ARCHITEW6432
    if (-not $cpu) { $cpu = $env:PROCESSOR_ARCHITECTURE }
    switch ($cpu) {
        'AMD64' { $arch = 'amd64' }
        'ARM64' { $arch = 'arm64' }
        default { throw "sshc: no prebuilt binary for $cpu; build from source instead" }
    }
    $file = "sshc-windows-$arch.exe"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('sshc-install-' + [guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $file ..."
        Invoke-WebRequest "$base/$file" -OutFile (Join-Path $tmp $file) -UseBasicParsing
        Invoke-WebRequest "$base/SHA256SUMS" -OutFile (Join-Path $tmp 'SHA256SUMS') -UseBasicParsing

        $want = Get-Content (Join-Path $tmp 'SHA256SUMS') |
            ForEach-Object { $hash, $name = $_ -split '\s+', 2; if ($name -eq $file) { $hash } }
        $got = (Get-FileHash (Join-Path $tmp $file) -Algorithm SHA256).Hash.ToLower()
        if (-not $want -or $want -ne $got) {
            throw "sshc: checksum mismatch for $file; not installing"
        }

        & (Join-Path $tmp $file) --install
        if ($LASTEXITCODE -ne 0) { throw "sshc: --install failed" }
    }
    finally {
        Remove-Item -Recurse -Force $tmp
    }
}
