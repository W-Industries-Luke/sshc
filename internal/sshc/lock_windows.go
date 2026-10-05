package sshc

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf16"
)

const verifyMethod = "for your Windows Hello PIN, fingerprint or face"

// Windows Hello is reached through the WinRT class UserConsentVerifier. The
// simplest dependable way to call WinRT from a plain executable is Windows
// PowerShell 5.1, which every Windows 10 and 11 has. The script prints the
// outcome; %s is "check" or "verify".
const helloScript = `
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Security.Credentials.UI.UserConsentVerifier, Windows.Security.Credentials.UI, ContentType = WindowsRuntime]
$asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1'
})[0]
function Await($operation, $resultType) {
    $task = $asTask.MakeGenericMethod($resultType).Invoke($null, @($operation))
    $null = $task.Wait(-1)
    $task.Result
}
$available = Await ([Windows.Security.Credentials.UI.UserConsentVerifier]::CheckAvailabilityAsync()) ([Windows.Security.Credentials.UI.UserConsentVerifierAvailability])
if ("$available" -ne 'Available' -or '%s' -eq 'check') { Write-Output "availability:$available"; exit 0 }
$result = Await ([Windows.Security.Credentials.UI.UserConsentVerifier]::RequestVerificationAsync('sshc: switch stored credentials back on')) ([Windows.Security.Credentials.UI.UserConsentVerificationResult])
Write-Output "result:$result"
`

// hello runs the script in the given mode and returns the line it prints.
func hello(mode string) (string, error) {
	script := fmt.Sprintf(helloScript, mode)
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 0, 2*len(units))
	for _, u := range units {
		raw = append(raw, byte(u), byte(u>>8))
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not ask Windows Hello: %v", err)
	}
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if len(lines) == 0 {
		return "", errors.New("Windows Hello gave no answer")
	}
	return lines[len(lines)-1], nil
}

func helloUnavailable(answer string) error {
	switch answer {
	case "availability:DeviceNotPresent", "availability:NotConfiguredForUser":
		return errors.New("Windows Hello is not set up for your account (set a PIN under Settings > Accounts > Sign-in options)")
	case "availability:DisabledByPolicy":
		return errors.New("Windows Hello is disabled by policy on this computer")
	}
	return fmt.Errorf("Windows Hello is not available (%s)", strings.TrimPrefix(answer, "availability:"))
}

func canVerifyUser() error {
	answer, err := hello("check")
	if err != nil {
		return err
	}
	if answer != "availability:Available" {
		return helloUnavailable(answer)
	}
	return nil
}

func verifyUser() error {
	answer, err := hello("verify")
	if err != nil {
		return err
	}
	switch {
	case answer == "result:Verified":
		return nil
	case strings.HasPrefix(answer, "availability:"):
		return helloUnavailable(answer)
	case answer == "result:Canceled":
		return errors.New("the Windows Hello prompt was cancelled")
	}
	return fmt.Errorf("Windows Hello did not verify you (%s)", strings.TrimPrefix(answer, "result:"))
}
